package activity

import (
	"context"
	"time"
)

type LoginHistoryRepository interface {
	Create(ctx context.Context, entry *LoginEntry) error
	GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*LoginEntry, error)
	CountFailedSince(ctx context.Context, userID string, since time.Time) (int, error)
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
