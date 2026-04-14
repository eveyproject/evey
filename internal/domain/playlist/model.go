package playlist

import (
	"errors"
	"time"

	"github.com/evey-project/evey/internal/pkg/id"
)

type PlaylistType string

const (
	Manual PlaylistType = "manual"
	Smart  PlaylistType = "smart"
)

type Playlist struct {
	ID          string
	Name        string
	Description string
	UserID      string
	Type        PlaylistType
	Rules       []Rule
	SortBy      string
	SortOrder   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type PlaylistItem struct {
	PlaylistID  string
	MediaItemID string
	SortOrder   int
	AddedAt     time.Time
}

type Rule struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

func (p *Playlist) Validate() error {
	if p.Name == "" {
		return errors.New("playlist name is required")
	}
	if p.UserID == "" {
		return errors.New("playlist user ID is required")
	}
	if p.Type != Manual && p.Type != Smart {
		return errors.New("playlist type must be 'manual' or 'smart'")
	}
	return nil
}

// NewPlaylist constructs a Playlist with invariants enforced.
func NewPlaylist(name, userID string, ptype PlaylistType) (*Playlist, error) {
	now := time.Now()
	p := &Playlist{
		ID:        id.New("pl-"),
		Name:      name,
		UserID:    userID,
		Type:      ptype,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Playlist) IsOwner(userID string) bool {
	return p.UserID == userID
}

func (p *Playlist) IsSmart() bool {
	return p.Type == Smart
}
