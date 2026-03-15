package activity

import (
	"context"
	"time"
)

type LoginHistoryRepository interface {
	Create(ctx context.Context, entry *LoginEntry) error
	GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*LoginEntry, error)
}

type ServerMetricRepository interface {
	Create(ctx context.Context, metric *ServerMetric) error
	GetByTimeRange(ctx context.Context, from, to time.Time) ([]*ServerMetric, error)
	Purge(ctx context.Context, before time.Time) error
}

type AuditLogFilter struct {
	UserID       *string
	Action       *string
	ResourceType *string
	Limit        int
	Offset       int
}

type AuditLogRepository interface {
	Create(ctx context.Context, entry *AuditLogEntry) error
	List(ctx context.Context, filter AuditLogFilter) ([]*AuditLogEntry, error)
}

type PlaySessionStatsReader interface {
	GetUserStats(ctx context.Context, profileID string) (*UserStats, error)
	GetServerStats(ctx context.Context) (*ServerStatsResult, error)
	GetMostPopular(ctx context.Context, limit int) ([]MediaPlayCount, error)
}

type PlayHistoryFilter struct {
	ProfileID   string
	MediaItemID string
	From        *time.Time
	To          *time.Time
	Limit       int
	Offset      int
}

type PlayHistoryRepository interface {
	List(ctx context.Context, filter PlayHistoryFilter) ([]*PlayHistoryEntry, error)
}

type AnalyticsReader interface {
	GetAnalytics(ctx context.Context) (*AnalyticsData, error)
}
