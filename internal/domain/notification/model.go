package notification

import (
	"errors"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

// ErrSubscriptionGone signals that a push provider reported the subscription is
// permanently gone (HTTP 410). Callers should delete the subscription rather
// than retry it on every future notification.
var ErrSubscriptionGone = errors.New("push subscription gone (410)")

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
	// TypeMediaRequest pages operators that a user submitted a media request
	// (admin-only, like plugin_error). TypeRequestAnswered notifies the
	// requester of the operator's decision (a personal notification).
	TypeMediaRequest    Type = "media_request"
	TypeRequestAnswered Type = "request_answered"
)

func (t Type) IsValid() bool {
	switch t {
	case TypeNewMovie, TypeNewEpisode, TypeNewSeason, TypeNewSeries, TypeNewAlbum,
		TypeLibraryScanned, TypeBackupFailed, TypePluginError, TypeCustom,
		TypeMediaRequest, TypeRequestAnswered:
		return true
	}
	return false
}

// IsAdminOnly returns true for notification types that describe operational
// or infrastructure events. Their rows are only ever created on ADMIN
// profiles (the notification event handlers target ListAdminProfiles), and
// since 2026-08-26 they appear in the owner's regular feed and unread badge
// like any other row: the old "keep them out of the personal feed, a
// dedicated endpoint will surface them" design left them unreachable - no
// client ever called ListAdminNotifications, and media requests sat
// unnoticed in prod for weeks. The classification still powers
// ListAdminNotifications (a filtered admin view) and client-side type
// filters (the web bell hides plugin_error, which has its own dashboard).
func (t Type) IsAdminOnly() bool {
	switch t {
	case TypeBackupFailed, TypePluginError, TypeMediaRequest:
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

func (l Level) IsValid() bool {
	switch l {
	case LevelDisabled, LevelAll, LevelRelevant, LevelHighlyRelevant:
		return true
	}
	return false
}

// SeriesKey is the opaque per-library deduplication key grouping the
// new_episode/new_season notifications of one series: "<libraryID>:<title>".
// It is NOT a MediaItemID - it used to be typed as one (and still persists in
// the legacy related_media_id column), which let the two mix silently.
type SeriesKey string

// NewSeriesKey builds the key; an empty title yields the empty key (no
// grouping), mirroring the historical behaviour.
func NewSeriesKey(libraryID, seriesTitle string) SeriesKey {
	if seriesTitle == "" {
		return ""
	}
	return SeriesKey(libraryID + ":" + seriesTitle)
}

type Notification struct {
	ID          string
	ProfileID   shared.ProfileID
	Type        Type
	Title       string
	Message     string
	MediaItemID shared.MediaItemID
	// SeriesKey groups new_episode/new_season notifications of one series.
	// Persisted in the legacy related_media_id column.
	SeriesKey SeriesKey
	// Subject is the bare name the notification is about ("Some Series"), without
	// any count or wording. Title stays the pre-rendered English label for
	// clients that predate this; a client that sees a Subject builds the whole
	// sentence itself and never reads Title. Empty on rows written before.
	Subject      string
	SeasonCount  int // seasons that came with the batch, 0 when none
	EpisodeCount int // accumulated episode count for grouped new_episode notifs
	Level        Level
	CreatedAt    time.Time
	ExpiresAt    *time.Time
	ReadAt       *time.Time
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

func WithSeriesKey(k SeriesKey) NotificationOption {
	return func(n *Notification) { n.SeriesKey = k }
}

// WithLabelParts records what a client needs to word the notification itself:
// the bare subject and the counts.
func WithLabelParts(subject string, seasonCount, episodeCount int) NotificationOption {
	return func(n *Notification) {
		n.Subject = subject
		n.SeasonCount = seasonCount
		n.EpisodeCount = episodeCount
	}
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
