package syncplay

import (
	"crypto/rand"
	"math/big"
	"time"
)

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func GenerateCode() string {
	b := make([]byte, 6)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(codeAlphabet))))
		b[i] = codeAlphabet[n.Int64()]
	}
	return string(b)
}

type Room struct {
	ID        string
	Code      string
	Name      string
	HostID    string
	MediaID   string
	Members   []Member
	State     PlaybackState
	CreatedAt time.Time
}

type Member struct {
	UserID   string
	Username string
	JoinedAt time.Time
}

type PlaybackState struct {
	Playing    bool
	PositionMs int64
	UpdatedAt  time.Time
	UpdatedBy  string
}

type SyncCommand struct {
	Type       string `json:"type"`
	PositionMs int64  `json:"positionMs,omitempty"`
	UserID     string `json:"userId,omitempty"`
	Username   string `json:"username,omitempty"`
	RoomID     string `json:"roomId,omitempty"`
	Playing    bool   `json:"playing,omitempty"`
}

// MaskUsername returns first rune + "***" to avoid leaking login usernames.
func MaskUsername(name string) string {
	if len(name) == 0 {
		return "***"
	}
	return string([]rune(name)[:1]) + "***"
}

func (r *Room) IsHost(userID string) bool {
	return r.HostID == userID
}

func (r *Room) IsMember(userID string) bool {
	if r.HostID == userID {
		return true
	}
	for _, m := range r.Members {
		if m.UserID == userID {
			return true
		}
	}
	return false
}
