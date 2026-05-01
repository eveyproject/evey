package notification

import (
	"context"
	"time"
)

type PushSubscriptionRepository interface {
	Upsert(ctx context.Context, userID, profileID, endpoint, p256dh, auth string) error
	DeleteByEndpoint(ctx context.Context, userID, endpoint string) error
	DeleteByUser(ctx context.Context, userID string) error
	GetByProfile(ctx context.Context, profileID string) ([]PushSubscription, error)
	ListByUser(ctx context.Context, userID string) ([]PushSubscription, error)
}

type PushSender interface {
	Send(ctx context.Context, sub PushSubscription, title, body string, data map[string]string) error
}

type Repository interface {
	Create(ctx context.Context, n *Notification) error
	ListByProfile(ctx context.Context, profileID string, unreadOnly bool, limit, offset int) ([]*Notification, error)
	// FindRecentByProfileTypeAndSeries returns notifications matching profileID, one of types,
	// and relatedMediaID (series ID), created after since. Used for deduplication.
	FindRecentByProfileTypeAndSeries(ctx context.Context, profileID string, types []Type, relatedMediaID string, since time.Time) ([]*Notification, error)
	// Update persists title and episode_count changes on an existing notification.
	Update(ctx context.Context, n *Notification) error
	MarkRead(ctx context.Context, id, profileID string) error
	MarkAllRead(ctx context.Context, profileID string) error
	UnreadCount(ctx context.Context, profileID string) (int, error)
	DeleteAll(ctx context.Context, profileID string) error
	CleanupExpired(ctx context.Context) error
}

type SettingsRepository interface {
	GetByProfile(ctx context.Context, profileID string) (*Settings, error)
	GetByProfiles(ctx context.Context, profileIDs []string) (map[string]*Settings, error)
	Upsert(ctx context.Context, s *Settings) error
	GetGlobalDefaults(ctx context.Context) (*Settings, error)
	UpsertGlobalDefaults(ctx context.Context, s *Settings) error
}

type WatchHistoryAnalyzer interface {
	GetFavoriteGenres(ctx context.Context, profileID string, minCount int) ([]string, error)
	GetFavoriteGenresBatch(ctx context.Context, profileIDs []string, minCount int) (map[string][]string, error)
	HasWatchedSeries(ctx context.Context, profileID, mediaItemID string) (bool, error)
	IsCurrentlyWatchingSeries(ctx context.Context, profileID, mediaItemID string) (bool, error)
	CalculateRelevanceScore(ctx context.Context, profileID string, genres []string) (int, error)
}
