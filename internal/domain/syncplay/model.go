package syncplay

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

// codeAlphabet is Crockford base32: no I, L, O or U, so a code read off a
// television never folds onto another valid code and no draw spells a word.
const codeAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// codeLen of 6 over that alphabet is 2^30 codes. The previous 5 digits were
// 90 000: enumerable in minutes by an attacker rotating source addresses past
// the per-IP limiter, for a 3 h streaming token minted without any account.
const codeLen = 6

func GenerateCode() (string, error) {
	b := make([]byte, codeLen)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(codeAlphabet))))
		if err != nil {
			return "", fmt.Errorf("crypto/rand failed: %w", err)
		}
		b[i] = codeAlphabet[n.Int64()]
	}
	return string(b), nil
}

// NormalizeCode folds what a guest actually types into the stored form: case,
// the separators people add to a 6-character code, and the three characters the
// alphabet drops precisely because they are misread (I and L for 1, O for 0).
// Everything else is left alone so an unknown code stays unknown.
func NormalizeCode(code string) string {
	var b strings.Builder
	for _, c := range strings.ToUpper(code) {
		switch {
		case c == 'I' || c == 'L':
			b.WriteByte('1')
		case c == 'O':
			b.WriteByte('0')
		case strings.ContainsRune(codeAlphabet, c):
			b.WriteRune(c)
		}
	}
	return b.String()
}

type Room struct {
	ID         string
	Code       string
	Name       string
	HostID     shared.UserID
	MediaID    shared.MediaItemID
	MediaType  string // "video" (default) or "music"
	PlaylistID *string
	Queue      []shared.MediaItemID
	TrackIndex int
	Members    []Member
	State      PlaybackState
	CreatedAt  time.Time
}

type Member struct {
	UserID   shared.UserID
	Username string
	JoinedAt time.Time
}

type PlaybackState struct {
	Playing    bool
	PositionMs int64
	UpdatedAt  time.Time
	UpdatedBy  shared.UserID
}

type SyncCommand struct {
	Type       string `json:"type"`
	PositionMs int64  `json:"positionMs,omitempty"`
	UserID     string `json:"userId,omitempty"`
	Username   string `json:"username,omitempty"`
	RoomID     string `json:"roomId,omitempty"`
	Playing    bool   `json:"playing,omitempty"`
	TrackIndex int    `json:"trackIndex,omitempty"`
	MediaID    string `json:"mediaId,omitempty"`
}

// MaskUsername returns first rune + "***" to avoid leaking login usernames.
func MaskUsername(name string) string {
	if len(name) == 0 {
		return "***"
	}
	return string([]rune(name)[:1]) + "***"
}

// Validate checks the required invariants for a Room. An empty Code breaks
// room-lookup-by-code; an empty MediaID causes nil-media at first sync.
func (r *Room) Validate() error {
	if r.ID == "" {
		return errors.New("room requires an ID")
	}
	if r.Code == "" {
		return errors.New("room requires a code")
	}
	if r.Name == "" {
		return errors.New("room requires a name")
	}
	if r.HostID == "" {
		return errors.New("room requires a host ID")
	}
	if r.MediaID == "" {
		return errors.New("room requires a media ID")
	}
	return nil
}

// NewRoom is the canonical constructor. Stamps CreatedAt and State.UpdatedAt
// (UTC), defaults MediaType to "video", and validates required fields before
// returning. Contextual fields (Queue, PlaylistID, TrackIndex, Members) are
// set by the caller on the returned pointer.
func NewRoom(id, code, name string, hostID shared.UserID, mediaID shared.MediaItemID, mediaType string) (*Room, error) {
	if mediaType == "" {
		mediaType = "video"
	}
	now := time.Now().UTC()
	r := &Room{
		ID:        id,
		Code:      code,
		Name:      name,
		HostID:    hostID,
		MediaID:   mediaID,
		MediaType: mediaType,
		CreatedAt: now,
		State:     PlaybackState{UpdatedAt: now},
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return r, nil
}

// OldestMember returns the member with the earliest JoinedAt from a non-empty
// slice. Ties are broken by ascending UserID for a deterministic, stable result.
// Used to elect a new host when the current host disconnects.
func OldestMember(members []Member) Member {
	oldest := members[0]
	for _, m := range members[1:] {
		if m.JoinedAt.Before(oldest.JoinedAt) ||
			(m.JoinedAt.Equal(oldest.JoinedAt) && string(m.UserID) < string(oldest.UserID)) {
			oldest = m
		}
	}
	return oldest
}

func (r *Room) IsHost(userID string) bool {
	return string(r.HostID) == userID
}

func (r *Room) IsMember(userID string) bool {
	if string(r.HostID) == userID {
		return true
	}
	for _, m := range r.Members {
		if string(m.UserID) == userID {
			return true
		}
	}
	return false
}
