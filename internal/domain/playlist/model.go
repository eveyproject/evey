package playlist

import (
	"errors"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/id"
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
	ProfileID   shared.ProfileID
	Type        PlaylistType
	Rules       []Rule
	SortBy      string
	SortOrder   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const (
	MediaTypeVideo = "video"
	MediaTypeMusic = "music"
)

type PlaylistItem struct {
	PlaylistID  string
	MediaItemID shared.MediaItemID
	SortOrder   int
	AddedAt     time.Time
	MediaType   string
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
	if p.ProfileID == "" {
		return errors.New("playlist profile ID is required")
	}
	if p.Type != Manual && p.Type != Smart {
		return errors.New("playlist type must be 'manual' or 'smart'")
	}
	return nil
}

// NewPlaylist constructs a Playlist with invariants enforced.
func NewPlaylist(name, profileID string, ptype PlaylistType) (*Playlist, error) {
	now := time.Now()
	p := &Playlist{
		ID:        id.New("pl-"),
		Name:      name,
		ProfileID: shared.ProfileID(profileID),
		Type:      ptype,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

// IsOwnedBy reports whether the playlist belongs to the given profile.
func (p *Playlist) IsOwnedBy(profileID shared.ProfileID) bool {
	return p.ProfileID == profileID
}

func (p *Playlist) IsSmart() bool {
	return p.Type == Smart
}
