package grpcapi

import (
	"context"
	"testing"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appactivity "github.com/evey-project/evey/internal/app/activity"
	"github.com/evey-project/evey/internal/domain/activity"
	actfakes "github.com/evey-project/evey/internal/domain/activity/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func newActivityHandler() (*activityHandler, *actfakes.FakeLoginHistoryRepository, *actfakes.FakePlaySessionStatsReader, *actfakes.FakePlayHistoryRepository, *actfakes.FakeServerMetricRepository) {
	loginHistory := actfakes.NewFakeLoginHistoryRepository()
	metrics := actfakes.NewFakeServerMetricRepository()
	auditLog := actfakes.NewFakeAuditLogRepository()
	statsReader := actfakes.NewFakePlaySessionStatsReader()
	playHistory := actfakes.NewFakePlayHistoryRepository()

	svc := appactivity.NewQueryService(loginHistory, metrics, auditLog, statsReader, playHistory)
	h := &activityHandler{svc: svc}
	return h, loginHistory, statsReader, playHistory, metrics
}

func TestActivityHandler_GetPlayHistory_Success(t *testing.T) {
	h, _, _, playHistory, _ := newActivityHandler()

	now := time.Now()
	// ph1 is newer than ph2 so it sorts first under the DESC timestamp
	// order the cursor pagination uses.
	playHistory.Add(
		&activity.PlayHistoryEntry{
			ID: "ph1", ProfileID: "profile-1", MediaItemID: "m1",
			MediaTitle: "Movie 1", MediaType: "movie", DurationMs: 120000,
			PositionMs: 60000, StartedAt: now,
		},
		&activity.PlayHistoryEntry{
			ID: "ph2", ProfileID: "profile-1", MediaItemID: "m2",
			MediaTitle: "Movie 2", MediaType: "movie", DurationMs: 90000,
			PositionMs: 90000, Completed: true, StartedAt: now.Add(-time.Minute),
		},
	)

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.GetPlayHistory(ctx, &eveyv1.GetPlayHistoryRequest{})

	require.NoError(t, err)
	require.Len(t, resp.Entries, 2)
	assert.Equal(t, "ph1", resp.Entries[0].Id)
	assert.Equal(t, "m1", resp.Entries[0].MediaItemId)
	assert.Equal(t, int64(60000), resp.Entries[0].PositionMs)
	assert.True(t, resp.Entries[1].Completed)
}

func TestActivityHandler_GetPlayHistory_Unauthenticated(t *testing.T) {
	h, _, _, _, _ := newActivityHandler()

	_, err := h.GetPlayHistory(context.Background(), &eveyv1.GetPlayHistoryRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestActivityHandler_GetPlayHistory_WithPagination(t *testing.T) {
	h, _, _, playHistory, _ := newActivityHandler()

	now := time.Now()
	for i := range 5 {
		playHistory.Add(&activity.PlayHistoryEntry{
			ID: "ph" + string(rune('0'+i)), ProfileID: "profile-1",
			MediaItemID: "m1", MediaTitle: "Movie", MediaType: "movie",
			StartedAt: now,
		})
	}

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.GetPlayHistory(ctx, &eveyv1.GetPlayHistoryRequest{
		Pagination: &eveyv1.PaginationRequest{Page: 1, PageSize: 2},
	})

	require.NoError(t, err)
	assert.Len(t, resp.Entries, 2)
}

func TestActivityHandler_GetUserStats_Success(t *testing.T) {
	h, _, statsReader, _, _ := newActivityHandler()

	statsReader.UserStatsResult = &activity.UserStats{
		ProfileID:        "profile-1",
		TotalWatchTimeMs: 3600000,
		TotalPlayCount:   15,
		GenreBreakdown:   map[string]activity.GenreStats{"action": {TotalTimeMs: 1200000, PlayCount: 5}, "comedy": {TotalTimeMs: 2400000, PlayCount: 10}},
		MostPlayedItems: []activity.MediaPlayCount{
			{MediaItemID: "m1", Title: "Top Movie", PlayCount: 5},
		},
	}

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.GetUserStats(ctx, &eveyv1.GetUserStatsRequest{})

	require.NoError(t, err)
	assert.Equal(t, "profile-1", resp.ProfileId)
	assert.Equal(t, int64(3600000), resp.TotalWatchTimeMs)
	assert.Equal(t, int32(15), resp.TotalPlays)
	assert.NotEmpty(t, resp.Genres)
	require.Len(t, resp.MostPlayed, 1)
	assert.Equal(t, "Top Movie", resp.MostPlayed[0].Title)
	assert.Equal(t, int32(5), resp.MostPlayed[0].PlayCount)
}

func TestActivityHandler_GetServerStats_Success(t *testing.T) {
	h, _, statsReader, _, _ := newActivityHandler()

	statsReader.ServerStatsResult = &activity.ServerStatsResult{
		TotalUsers: 10,
		TotalItems: 500,
	}

	resp, err := h.GetServerStats(context.Background(), &eveyv1.GetServerStatsRequest{})

	require.NoError(t, err)
	assert.Equal(t, int32(10), resp.TotalUsers)
	assert.Equal(t, int32(500), resp.TotalMediaItems)
}

func TestActivityHandler_GetLoginHistory_OwnHistory(t *testing.T) {
	h, loginRepo, _, _, _ := newActivityHandler()

	now := time.Now()
	_ = loginRepo.Create(context.Background(), &activity.LoginEntry{
		ID: "l1", UserID: "user-1", ClientIP: "192.168.1.1",
		UserAgent: "Firefox", Success: true, CreatedAt: now,
	})
	_ = loginRepo.Create(context.Background(), &activity.LoginEntry{
		ID: "l2", UserID: "user-2", ClientIP: "10.0.0.1",
		UserAgent: "Chrome", Success: true, CreatedAt: now,
	})

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.GetLoginHistory(ctx, &eveyv1.GetLoginHistoryRequest{})

	require.NoError(t, err)
	require.Len(t, resp.Entries, 1)
	assert.Equal(t, "l1", resp.Entries[0].Id)
	assert.Equal(t, "192.168.1.1", resp.Entries[0].IpAddress)
	assert.True(t, resp.Entries[0].Success)
}

func TestActivityHandler_GetLoginHistory_AdminCanViewOther(t *testing.T) {
	h, loginRepo, _, _, _ := newActivityHandler()

	now := time.Now()
	_ = loginRepo.Create(context.Background(), &activity.LoginEntry{
		ID: "l1", UserID: "user-2", ClientIP: "10.0.0.1",
		UserAgent: "Chrome", Success: true, CreatedAt: now,
	})

	ctx := ctxWithClaims("admin-1", "profile-admin", true)
	resp, err := h.GetLoginHistory(ctx, &eveyv1.GetLoginHistoryRequest{UserId: "user-2"})

	require.NoError(t, err)
	require.Len(t, resp.Entries, 1)
	assert.Equal(t, "user-2", resp.Entries[0].UserId)
}

func TestActivityHandler_GetLoginHistory_NonAdminCannotViewOther(t *testing.T) {
	h, _, _, _, _ := newActivityHandler()

	ctx := ctxWithClaims("user-1", "profile-1", false)
	_, err := h.GetLoginHistory(ctx, &eveyv1.GetLoginHistoryRequest{UserId: "user-2"})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestActivityHandler_ListServerMetrics_Success(t *testing.T) {
	h, _, _, _, metricsRepo := newActivityHandler()

	now := time.Now()
	_ = metricsRepo.Create(context.Background(), &activity.ServerMetric{
		ID: "sm1", CPUPercent: 45.5, MemoryUsedBytes: 4000000000,
		MemoryTotalBytes: 8000000000, ActiveStreams: 3,
		ActiveTranscodes: 1, RecordedAt: now,
	})

	from := timestamppb.New(now.Add(-1 * time.Hour))
	to := timestamppb.New(now.Add(1 * time.Hour))

	resp, err := h.ListServerMetrics(context.Background(), &eveyv1.ListServerMetricsRequest{
		From: from, To: to,
	})

	require.NoError(t, err)
	require.Len(t, resp.Metrics, 1)
	assert.InEpsilon(t, 45.5, resp.Metrics[0].CpuPercent, 1e-6)
	assert.Equal(t, int32(3), resp.Metrics[0].ActiveStreams)
	assert.Equal(t, int32(1), resp.Metrics[0].ActiveTranscodes)
	assert.InDelta(t, 50.0, resp.Metrics[0].MemoryPercent, 0.1)
}

func TestActivityHandler_GetLoginHistory_Unauthenticated(t *testing.T) {
	h, _, _, _, _ := newActivityHandler()

	_, err := h.GetLoginHistory(context.Background(), &eveyv1.GetLoginHistoryRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}
