package playlist

import (
	"time"
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

func (p *Playlist) IsOwner(userID string) bool {
	return p.UserID == userID
}

func (p *Playlist) IsSmart() bool {
	return p.Type == Smart
}
