package notification

import (
	"errors"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type PushSubscription struct {
	ID        string
	UserID    shared.UserID
	ProfileID shared.ProfileID
	Endpoint  string
	P256dh    string
	Auth      string
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
	TypeBackupFailed   Type = "backup_failed"
	TypePluginError    Type = "plugin_error"
	TypeCustom         Type = "custom"
)

func (t Type) IsValid() bool {
	switch t {
	case TypeNewMovie, TypeNewEpisode, TypeNewSeason, TypeNewSeries, TypeNewAlbum,
		TypeLibraryScanned, TypeBackupFailed, TypePluginError, TypeCustom:
		return true
	}
	return false
}

type Level string

const (
	LevelDisabled       Level = "disabled"
	LevelAll            Level = "all"
	LevelRelevant       Level = "relevant"
	LevelHighlyRelevant Level = "highly_relevant"
)

type Notification struct {
	ID          string
	ProfileID   shared.ProfileID
	Type        Type
	Title       string
	Message     string
	MediaItemID shared.MediaItemID
	Level       Level
	CreatedAt   time.Time
	ExpiresAt   *time.Time
	DeliveredAt *time.Time
	ReadAt      *time.Time
}

type Settings struct {
	ProfileID   shared.ProfileID
	MovieLevel  Level
	SeriesLevel Level
	MusicLevel  Level
}

var validLevels = map[Level]bool{
	LevelDisabled:       true,
	LevelAll:            true,
	LevelRelevant:       true,
	LevelHighlyRelevant: true,
}

func (s *Settings) Validate() error {
	if !validLevels[s.MovieLevel] {
		return errors.New("invalid movie notification level")
	}
	if !validLevels[s.SeriesLevel] {
		return errors.New("invalid series notification level")
	}
	if !validLevels[s.MusicLevel] {
		return errors.New("invalid music notification level")
	}
	return nil
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
