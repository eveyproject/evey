package activity_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appactivity "github.com/eveyproject/evey/internal/app/activity"
	"github.com/eveyproject/evey/internal/domain/activity"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// The admin tables page with a total and a keyset cursor; each query service
// entry point must reach its repository with the caller's filter intact and
// surface a storage failure rather than an empty page.
func TestQueryService_CountsAndCursor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, querySvc, loginRepo, _, auditRepo, statsReader, playHistoryRepo := newTestServices()

	base := time.Date(2026, 9, 16, 3, 0, 0, 0, time.UTC)
	for i, ok := range []bool{true, false, true} {
		require.NoError(t, loginRepo.Create(ctx, &activity.LoginEntry{
			ID: fmt.Sprintf("l%d", i+1), UserID: "u1", Success: ok, CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}))
	}
	require.NoError(t, loginRepo.Create(ctx, &activity.LoginEntry{ID: "l9", UserID: "u2", Success: true, CreatedAt: base}))

	page, err := querySvc.GetLoginHistoryCursor(ctx, "u1", true, nil, nil, nil, 10)
	require.NoError(t, err)
	require.Len(t, page, 2, "successOnly drops the failed attempt, the other user is never seen")
	assert.Equal(t, "l3", page[0].ID, "newest first")
	next, err := querySvc.GetLoginHistoryCursor(ctx, "u1", true, nil, nil,
		&activity.LoginHistoryCursor{StartedAt: page[0].CreatedAt, EntryID: page[0].ID}, 10)
	require.NoError(t, err)
	require.Len(t, next, 1)
	assert.Equal(t, "l1", next[0].ID)

	action := "login"
	require.NoError(t, auditRepo.Create(ctx, &activity.AuditLogEntry{ID: "a1", UserID: "u1", Action: "login"}))
	require.NoError(t, auditRepo.Create(ctx, &activity.AuditLogEntry{ID: "a2", UserID: "u1", Action: "delete"}))
	n, err := querySvc.CountAuditLog(ctx, activity.AuditLogFilter{Action: &action})
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	playHistoryRepo.Add(
		&activity.PlayHistoryEntry{ID: "h1", ProfileID: "p1", MediaItemID: "m1"},
		&activity.PlayHistoryEntry{ID: "h2", ProfileID: "p2", MediaItemID: "m2"},
	)
	n, err = querySvc.CountPlayHistory(ctx, activity.PlayHistoryFilter{ProfileID: "p1"})
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	statsReader.ActiveSessionsBps = 12_000_000
	bps, err := querySvc.GetActiveSessionsBitrate(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(12_000_000), bps)

	statsReader.Err = errors.New("db down")
	auditRepo.Err = statsReader.Err
	playHistoryRepo.Err = statsReader.Err
	loginRepo.Err = statsReader.Err
	_, err = querySvc.GetActiveSessionsBitrate(ctx)
	require.Error(t, err)
	_, err = querySvc.CountAuditLog(ctx, activity.AuditLogFilter{})
	require.Error(t, err)
	_, err = querySvc.CountPlayHistory(ctx, activity.PlayHistoryFilter{})
	require.Error(t, err)
	_, err = querySvc.GetLoginHistoryCursor(ctx, "u1", false, nil, nil, nil, 10)
	require.Error(t, err)
}

// The server language setting decides how merged genres are labelled in the
// stats. Unset, unreadable or blank all mean "merge, do not translate".
func TestGetUserStats_GenreLabelFollowsServerLanguage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	breakdown := func() map[string]activity.GenreStats {
		return map[string]activity.GenreStats{
			"Drama": {TotalTimeMs: 1000, PlayCount: 2},
			"Drame": {TotalTimeMs: 500, PlayCount: 1},
		}
	}
	_, _, loginRepo, metricRepo, auditRepo, statsReader, playHistoryRepo := newTestServices()
	cfg := sharedfakes.NewFakeServerConfigRepository()
	querySvc := appactivity.NewQueryService(loginRepo, metricRepo, auditRepo, statsReader, playHistoryRepo,
		appactivity.WithServerConfig(cfg))

	require.NoError(t, cfg.Set(ctx, "server.language", " FR "))
	statsReader.UserStatsResult = &activity.UserStats{ProfileID: "p1", GenreBreakdown: breakdown()}
	stats, err := querySvc.GetUserStats(ctx, "p1")
	require.NoError(t, err)
	require.Len(t, stats.GenreBreakdown, 1)
	assert.Equal(t, int64(1500), stats.GenreBreakdown["Drame"].TotalTimeMs, "case and padding of the setting are tolerated")

	require.NoError(t, cfg.Set(ctx, "server.language", "en"))
	statsReader.UserStatsResult = &activity.UserStats{ProfileID: "p1", GenreBreakdown: breakdown()}
	stats, err = querySvc.GetUserStats(ctx, "p1")
	require.NoError(t, err)
	assert.Equal(t, int64(1500), stats.GenreBreakdown["Drama"].TotalTimeMs)

	cfg.Err = errors.New("db down")
	statsReader.UserStatsResult = &activity.UserStats{ProfileID: "p1", GenreBreakdown: breakdown()}
	stats, err = querySvc.GetUserStats(ctx, "p1")
	require.NoError(t, err, "a stats page is not worth failing over the language setting")
	assert.Len(t, stats.GenreBreakdown, 1, "still merged, just not translated")

	statsReader.UserStatsResult = nil
	stats, err = querySvc.GetUserStats(ctx, "p1")
	require.NoError(t, err)
	assert.Nil(t, stats, "a nil result passes through without a fold panic")
}
