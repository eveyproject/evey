package notification

import "time"

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
