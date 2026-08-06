package notification

import (
	"context"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type PushSubscriptionRepository interface {
	Upsert(ctx context.Context, userID shared.UserID, profileID shared.ProfileID, endpoint, p256dh, auth string) error
	DeleteByEndpoint(ctx context.Context, userID shared.UserID, endpoint string) error
	DeleteByUser(ctx context.Context, userID shared.UserID) error
	GetByProfile(ctx context.Context, profileID shared.ProfileID) ([]PushSubscription, error)
	ListByUser(ctx context.Context, userID shared.UserID) ([]PushSubscription, error)
}

type PushSender interface {
	Send(ctx context.Context, sub PushSubscription, title, body string, data map[string]string) error
}

// CommandRepo is the write side of the notification store (ADR-0009): the
// mutations the CommandService performs. Reads needed by command flows live in
// QueryRepo - the concrete repository satisfies both, the split only narrows
// what each call site can reach.
type CommandRepo interface {
	Create(ctx context.Context, n *Notification) error
	// Update persists title and episode_count changes on an existing notification.
	Update(ctx context.Context, n *Notification) error
	MarkRead(ctx context.Context, id string, profileID shared.ProfileID) error
	MarkAllRead(ctx context.Context, profileID shared.ProfileID) error
	DeleteAll(ctx context.Context, profileID shared.ProfileID) error
	CleanupExpired(ctx context.Context) error
}

// QueryRepo is the read side of the notification store.
type QueryRepo interface {
	ListByProfile(ctx context.Context, profileID shared.ProfileID, unreadOnly bool, limit, offset int) ([]*Notification, error)
	// FindRecentByProfilesTypeAndSeries is the batched form of the per-profile dedup
	// lookup: it returns, for every profile in profileIDs, the notifications matching
	// one of types and seriesKey created after since, in a single query. Used during
	// episode notification fan-out to avoid one round-trip per profile per series.
	FindRecentByProfilesTypeAndSeries(ctx context.Context, profileIDs []shared.ProfileID, types []Type, seriesKey SeriesKey, since time.Time) (map[shared.ProfileID][]*Notification, error)
	UnreadCount(ctx context.Context, profileID shared.ProfileID) (int, error)
	// UnreadCountExcludingTypes is UnreadCount minus the given types - used for
	// the personal bell badge, which hides admin-only operational notifications.
	UnreadCountExcludingTypes(ctx context.Context, profileID shared.ProfileID, excludeTypes []Type) (int, error)
}

// Repository is the composite the concrete adapters implement; services hold
// the narrowed halves, never this.
type Repository interface {
	CommandRepo
	QueryRepo
}

type SettingsRepository interface {
	GetByProfile(ctx context.Context, profileID shared.ProfileID) (*Settings, error)
	GetByProfiles(ctx context.Context, profileIDs []shared.ProfileID) (map[shared.ProfileID]*Settings, error)
	Upsert(ctx context.Context, s *Settings) error
	GetGlobalDefaults(ctx context.Context) (*Settings, error)
	UpsertGlobalDefaults(ctx context.Context, s *Settings) error
}

type WatchHistoryAnalyzer interface {
	GetFavoriteGenres(ctx context.Context, profileID shared.ProfileID, minCount int) ([]string, error)
	GetFavoriteGenresBatch(ctx context.Context, profileIDs []shared.ProfileID, minCount int) (map[shared.ProfileID][]string, error)
	HasWatchedSeries(ctx context.Context, profileID shared.ProfileID, mediaItemID string) (bool, error)
	IsCurrentlyWatchingSeries(ctx context.Context, profileID shared.ProfileID, mediaItemID string) (bool, error)
	CalculateRelevanceScore(ctx context.Context, profileID shared.ProfileID, genres []string) (int, error)
}
