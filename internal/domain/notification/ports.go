package notification

import "context"

type DeviceTokenRepository interface {
	Upsert(ctx context.Context, userID, profileID, token, platform string) error
	Delete(ctx context.Context, userID, token string) error
	GetByProfile(ctx context.Context, profileID string) ([]string, error)
	GetByUser(ctx context.Context, userID string) ([]string, error)
	ListByUser(ctx context.Context, userID string) ([]DeviceToken, error)
}

type PushSender interface {
	SendToTokens(ctx context.Context, tokens []string, title, body string, data map[string]string) error
}

type Repository interface {
	Create(ctx context.Context, n *Notification) error
	ListByProfile(ctx context.Context, profileID string, unreadOnly bool, limit, offset int) ([]*Notification, error)
	MarkRead(ctx context.Context, id string) error
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
	HasWatchedSeries(ctx context.Context, profileID, mediaItemID string) (bool, error)
	IsCurrentlyWatchingSeries(ctx context.Context, profileID, mediaItemID string) (bool, error)
	CalculateRelevanceScore(ctx context.Context, profileID string, genres []string) (int, error)
}
