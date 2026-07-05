package activity

import (
	"context"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// bandwidthCacheTTL bounds how often the dashboard recomputes total served
// bandwidth — a LAG window over the whole server_metrics history, too heavy to
// run on every dashboard poll.
const bandwidthCacheTTL = 5 * time.Minute

type QueryService struct {
	loginHistory    activity.LoginHistoryRepository
	metrics         activity.ServerMetricRepository
	auditLog        activity.AuditLogRepository
	statsReader     activity.PlaySessionStatsReader
	playHistory     activity.PlayHistoryRepository
	analyticsReader activity.AnalyticsReader

	bwMu       sync.Mutex
	bwCached   int64
	bwCachedAt time.Time
	bwHasValue bool
}

type Option func(*QueryService)

func WithAnalyticsReader(r activity.AnalyticsReader) Option {
	return func(s *QueryService) { s.analyticsReader = r }
}

func NewQueryService(
	loginHistory activity.LoginHistoryRepository,
	metrics activity.ServerMetricRepository,
	auditLog activity.AuditLogRepository,
	statsReader activity.PlaySessionStatsReader,
	playHistory activity.PlayHistoryRepository,
	opts ...Option,
) *QueryService {
	s := &QueryService{
		loginHistory: loginHistory,
		metrics:      metrics,
		auditLog:     auditLog,
		statsReader:  statsReader,
		playHistory:  playHistory,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *QueryService) GetLoginHistory(ctx context.Context, userID string, limit, offset int) ([]*activity.LoginEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.GetLoginHistory")
	defer span.End()
	return s.loginHistory.GetByUserID(ctx, shared.UserID(userID), limit, offset)
}

func (s *QueryService) GetLoginHistoryCursor(ctx context.Context, userID string, successOnly bool, from, to *time.Time, cursor *activity.LoginHistoryCursor, limit int) ([]*activity.LoginEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.GetLoginHistoryCursor")
	defer span.End()
	return s.loginHistory.ListByUserIDCursor(ctx, shared.UserID(userID), successOnly, from, to, cursor, limit)
}

func (s *QueryService) GetServerMetrics(ctx context.Context, from, to time.Time) ([]*activity.ServerMetric, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.GetServerMetrics")
	defer span.End()
	return s.metrics.GetByTimeRange(ctx, from, to)
}

func (s *QueryService) GetAuditLog(ctx context.Context, filter activity.AuditLogFilter) ([]*activity.AuditLogEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.GetAuditLog")
	defer span.End()
	return s.auditLog.List(ctx, filter)
}

func (s *QueryService) CountAuditLog(ctx context.Context, filter activity.AuditLogFilter) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.CountAuditLog")
	defer span.End()
	return s.auditLog.Count(ctx, filter)
}

func (s *QueryService) GetUserStats(ctx context.Context, profileID string) (*activity.UserStats, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.GetUserStats")
	defer span.End()
	return s.statsReader.GetUserStats(ctx, shared.ProfileID(profileID))
}

func (s *QueryService) GetServerStats(ctx context.Context) (*activity.ServerStatsResult, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.GetServerStats")
	defer span.End()
	stats, err := s.statsReader.GetServerStats(ctx)
	if err != nil {
		return nil, err
	}
	// Consumed bandwidth comes from the metric history (real sampled rate),
	// not from play sessions — best effort: stats stay useful without it.
	if s.metrics != nil {
		if bw, ok := s.cachedTotalBandwidth(ctx); ok {
			stats.TotalBandwidthBytes = bw
		}
	}
	return stats, nil
}

// cachedTotalBandwidth memoizes TotalBandwidthBytes for bandwidthCacheTTL. That
// query LAG-scans the entire server_metrics table; the dashboard polls every
// few seconds, so without this each poll re-ran a full ordered window aggregate.
// The lock also single-flights the refresh — concurrent dashboard loads after
// expiry wait for one scan instead of each launching their own.
func (s *QueryService) cachedTotalBandwidth(ctx context.Context) (int64, bool) {
	s.bwMu.Lock()
	defer s.bwMu.Unlock()
	if s.bwHasValue && time.Since(s.bwCachedAt) < bandwidthCacheTTL {
		return s.bwCached, true
	}
	bw, err := s.metrics.TotalBandwidthBytes(ctx)
	if err != nil {
		// Serve the last good value if we have one; otherwise signal absence so
		// the dashboard simply omits bandwidth rather than showing a wrong 0.
		return s.bwCached, s.bwHasValue
	}
	s.bwCached = bw
	s.bwCachedAt = time.Now()
	s.bwHasValue = true
	return bw, true
}

// GetActiveSessionsBitrate is the summed source bitrate of the active play
// sessions — the dashboard's theoretical outgoing rate.
func (s *QueryService) GetActiveSessionsBitrate(ctx context.Context) (int64, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.GetActiveSessionsBitrate")
	defer span.End()
	return s.statsReader.GetActiveSessionsBitrate(ctx)
}

func (s *QueryService) GetPlayHistory(ctx context.Context, filter activity.PlayHistoryFilter) ([]*activity.PlayHistoryEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.GetPlayHistory")
	defer span.End()
	return s.playHistory.List(ctx, filter)
}

func (s *QueryService) CountPlayHistory(ctx context.Context, filter activity.PlayHistoryFilter) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.CountPlayHistory")
	defer span.End()
	return s.playHistory.Count(ctx, filter)
}

func (s *QueryService) GetMostPopular(ctx context.Context, profileID string, limit int) ([]activity.MediaPlayCount, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.GetMostPopular")
	defer span.End()
	return s.statsReader.GetMostPopular(ctx, profileID, limit)
}

func (s *QueryService) GetAnalytics(ctx context.Context) (*activity.AnalyticsData, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.GetAnalytics")
	defer span.End()
	if s.analyticsReader == nil {
		return &activity.AnalyticsData{
			TopMedia:       []activity.TopMediaItem{},
			RecentActivity: []activity.RecentActivityEntry{},
		}, nil
	}
	return s.analyticsReader.GetAnalytics(ctx)
}
