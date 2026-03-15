package notification

import "time"

type DeviceToken struct {
	ID        string
	UserID    string
	ProfileID string
	Token     string
	Platform  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Type string

const (
	TypeNewMovie       Type = "new_movie"
	TypeNewEpisode     Type = "new_episode"
	TypeNewSeason      Type = "new_season"
	TypeNewSeries      Type = "new_series"
	TypeNewAlbum       Type = "new_album"
	TypeLibraryScanned Type = "library_scanned"
	TypePluginError    Type = "plugin_error"
	TypeCustom         Type = "custom"
)

type Level string

const (
	LevelDisabled       Level = "disabled"
	LevelAll            Level = "all"
	LevelRelevant       Level = "relevant"
	LevelHighlyRelevant Level = "highly_relevant"
)

type Notification struct {
	ID          string
	ProfileID   string
	Type        Type
	Title       string
	Message     string
	MediaItemID string
	Level       Level
	CreatedAt   time.Time
	ExpiresAt   *time.Time
	DeliveredAt *time.Time
	ReadAt      *time.Time
}

type Settings struct {
	ProfileID   string
	MovieLevel  Level
	SeriesLevel Level
	MusicLevel  Level
}

func (n *Notification) IsRead() bool {
	return n.ReadAt != nil
}

func (n *Notification) IsDelivered() bool {
	return n.DeliveredAt != nil
}

func (n *Notification) IsExpired() bool {
	return n.ExpiresAt != nil && time.Now().After(*n.ExpiresAt)
}

func (s *Settings) LevelForType(t Type) Level {
	switch t {
	case TypeNewMovie:
		return s.MovieLevel
	case TypeNewEpisode, TypeNewSeason, TypeNewSeries:
		return s.SeriesLevel
	case TypeNewAlbum:
		return s.MusicLevel
	}
	return LevelAll
}
