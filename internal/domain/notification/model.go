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

// IsAdminOnly returns true for notification types that describe operational
// or infrastructure events admins need to know about, but that should not
// pollute the personal notification feed (no badge, no dropdown listing).
//
// They still land in the same `notifications` table — querying with
// AdminOnly() filters them in/out at read time. A future /admin/notifications
// endpoint can surface them dedicated to admins.
func (t Type) IsAdminOnly() bool {
	switch t {
	case TypeBackupFailed, TypePluginError:
		return true
	}
	return false
}

// AdminOnlyTypes returns the slice of admin-only notification types so
// callers can pass them to repository List/Count exclude filters without
// hardcoding the values.
func AdminOnlyTypes() []Type {
	return []Type{TypeBackupFailed, TypePluginError}
}

type Level string

const (
	LevelDisabled       Level = "disabled"
	LevelAll            Level = "all"
	LevelRelevant       Level = "relevant"
	LevelHighlyRelevant Level = "highly_relevant"
)

func (l Level) IsValid() bool {
	switch l {
	case LevelDisabled, LevelAll, LevelRelevant, LevelHighlyRelevant:
		return true
	}
	return false
}

type Notification struct {
	ID             string
	ProfileID      shared.ProfileID
	Type           Type
	Title          string
	Message        string
	MediaItemID    shared.MediaItemID
	RelatedMediaID shared.MediaItemID // series ID for new_episode/new_season grouping
	EpisodeCount   int               // accumulated episode count for grouped new_episode notifs
	Level          Level
	CreatedAt      time.Time
	ExpiresAt      *time.Time
	DeliveredAt    *time.Time
	ReadAt         *time.Time
}

func (n *Notification) Validate() error {
	if n.ID == "" {
		return errors.New("notification ID is required")
	}
	if n.ProfileID == "" {
		return errors.New("notification profile ID is required")
	}
	if !n.Type.IsValid() {
		return errors.New("notification: invalid type")
	}
	if n.Title == "" {
		return errors.New("notification title is required")
	}
	if n.Level != "" && !n.Level.IsValid() {
		return errors.New("notification: invalid level")
	}
	return nil
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

type NotificationOption func(*Notification)

func WithNotificationMessage(msg string) NotificationOption {
	return func(n *Notification) { n.Message = msg }
}

func WithNotificationMediaItem(id shared.MediaItemID) NotificationOption {
	return func(n *Notification) { n.MediaItemID = id }
}

func WithRelatedMediaID(id shared.MediaItemID) NotificationOption {
	return func(n *Notification) { n.RelatedMediaID = id }
}

func WithEpisodeCount(count int) NotificationOption {
	return func(n *Notification) { n.EpisodeCount = count }
}

func WithNotificationLevel(l Level) NotificationOption {
	return func(n *Notification) { n.Level = l }
}

func WithNotificationExpiresAt(t *time.Time) NotificationOption {
	return func(n *Notification) { n.ExpiresAt = t }
}

// NewNotification is the canonical constructor. Stamps CreatedAt and
// validates. Required identity/intent fields (id, profile, type, title)
// are surfaced as params so construction never succeeds with them
// missing.
func NewNotification(id string, profileID shared.ProfileID, notifType Type, title string, opts ...NotificationOption) (*Notification, error) {
	n := &Notification{
		ID:        id,
		ProfileID: profileID,
		Type:      notifType,
		Title:     title,
		CreatedAt: time.Now(),
	}
	for _, opt := range opts {
		opt(n)
	}
	if err := n.Validate(); err != nil {
		return nil, err
	}
	return n, nil
}
