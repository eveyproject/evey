package syncplay

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

func GenerateCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(90000))
	if err != nil {
		return "", fmt.Errorf("crypto/rand failed: %w", err)
	}
	return fmt.Sprintf("%d", 10000+n.Int64()), nil
}

type Room struct {
	ID        string
	Code      string
	Name      string
	HostID    shared.UserID
	MediaID   shared.MediaItemID
	MediaType string // "video" (default) or "music"
	Members   []Member
	State     PlaybackState
	CreatedAt time.Time
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
}

// MaskUsername returns first rune + "***" to avoid leaking login usernames.
func MaskUsername(name string) string {
	if len(name) == 0 {
		return "***"
	}
	return string([]rune(name)[:1]) + "***"
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
