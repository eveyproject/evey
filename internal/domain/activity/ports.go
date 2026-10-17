package activity

import (
	"context"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

// LoginHistoryCursor keysets by (started_at, id) DESC. Empty = first
// page; non-empty resumes strictly after the referenced entry.
type LoginHistoryCursor struct {
	StartedAt time.Time
	EntryID   string
}

type LoginHistoryRepository interface {
	Create(ctx context.Context, entry *LoginEntry) error
	GetByUserID(ctx context.Context, userID shared.UserID, limit, offset int) ([]*LoginEntry, error)
	// ListByUserIDCursor returns up to `limit` entries strictly older than
	// the cursor, ordered by started_at DESC. Empty cursor returns the
	// most recent page.
	ListByUserIDCursor(ctx context.Context, userID shared.UserID, successOnly bool, from, to *time.Time, cursor *LoginHistoryCursor, limit int) ([]*LoginEntry, error)
}

type ServerMetricRepository interface {
	Create(ctx context.Context, metric *ServerMetric) error
	GetByTimeRange(ctx context.Context, from, to time.Time) ([]*ServerMetric, error)
	// GetBucketed folds the samples of [from, to] into buckets of the given
	// width: the percentages and rates are averaged, the counts, the disk
	// wait and the storage figures keep their maximum, and the busiest disk
	// is the one behind that maximum wait. A week of one-minute samples
	// becomes a few hundred points instead of ten thousand rows.
	GetBucketed(ctx context.Context, from, to time.Time, bucket time.Duration) ([]*ServerMetric, error)
	// IdleDiskAwaitMs is the lowest disk wait recorded since the given time
	// by a sample whose disk was nearly idle: the disk's wait at rest, the
	// reference the saturation line is drawn from. 0 when no sample qualifies.
	IdleDiskAwaitMs(ctx context.Context, since time.Time) (float64, error)
	Purge(ctx context.Context, before time.Time) error
	// TotalBandwidthBytes integrates the sampled streaming rate over the whole
	// metric history: the real bytes served, bounded by the retention window.
	TotalBandwidthBytes(ctx context.Context) (int64, error)
}

type AuditLogFilter struct {
	UserID       *string
	ProfileID    *string
	Action       *string
	ResourceType *string
	Limit        int
	Offset       int
}

type AuditLogRepository interface {
	Create(ctx context.Context, entry *AuditLogEntry) error
	List(ctx context.Context, filter AuditLogFilter) ([]*AuditLogEntry, error)
	// Count returns the total number of entries matching the filter's
	// predicates (ignoring Limit/Offset), for pagination totals.
	Count(ctx context.Context, filter AuditLogFilter) (int, error)
	// Purge deletes entries strictly older than before. Returns the number
	// of rows removed so a retention runner can log/observe its work.
	Purge(ctx context.Context, before time.Time) (int64, error)
}

type PlaySessionStatsReader interface {
	GetUserStats(ctx context.Context, profileID shared.ProfileID) (*UserStats, error)
	GetServerStats(ctx context.Context) (*ServerStatsResult, error)
	GetMostPopular(ctx context.Context, profileID string, limit int) ([]MediaPlayCount, error)
	// GetActiveSessionsBitrate sums the source bitrate of every active play
	// session - the dashboard's "theoretical outgoing rate". Uncached: the
	// dashboard polls every few seconds and the active set is small.
	GetActiveSessionsBitrate(ctx context.Context) (int64, error)
}

// PlayHistoryCursor keysets by (started_at, id) DESC. Same shape as
// LoginHistoryCursor - history feeds are always time-ordered.
type PlayHistoryCursor struct {
	StartedAt time.Time
	SessionID string
}

type PlayHistoryFilter struct {
	ProfileID shared.ProfileID
	// UserID narrows to every profile of one account. Both it and ProfileID
	// empty means "the whole server", which only an admin caller may ask for -
	// the handler, not this filter, is what enforces that.
	UserID      shared.UserID
	MediaItemID shared.MediaItemID
	From        *time.Time
	To          *time.Time
	Limit       int
	Offset      int
	// Cursor, when non-nil, overrides Offset: returns entries strictly
	// older than the cursor's (started_at, id) in DESC order.
	Cursor *PlayHistoryCursor
}

type PlayHistoryRepository interface {
	List(ctx context.Context, filter PlayHistoryFilter) ([]*PlayHistoryEntry, error)
	// Count returns the total number of entries matching the filter's
	// predicates (ignoring Limit/Offset/Cursor), for pagination totals.
	Count(ctx context.Context, filter PlayHistoryFilter) (int, error)
}

type AnalyticsReader interface {
	GetAnalytics(ctx context.Context) (*AnalyticsData, error)
}

// QoEEventPurger bounds the playback_qoe_events table. Kept as its own
// single-method port rather than folded into an existing repository: QoE is
// another bounded context, and retention is the ONLY thing this service needs
// from it. Optional - a nil purger disables the sweep.
// WebhookDeliveryPurger bounds the webhook delivery log. Same shape and same
// reasoning as QoEEventPurger: another context's table whose only cross-context
// need is retention.
type WebhookDeliveryPurger interface {
	Purge(ctx context.Context, before time.Time) (int64, error)
}

type QoEEventPurger interface {
	// Purge deletes events strictly older than before, returning the row count
	// so the retention runner can log its work.
	Purge(ctx context.Context, before time.Time) (int64, error)
}
