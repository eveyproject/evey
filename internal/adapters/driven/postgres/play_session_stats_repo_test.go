package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlaySessionStatsRepo_GetUserStats_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPlaySessionStatsReader(db)
	ctx := context.Background()

	stats, err := repo.GetUserStats(ctx, "nonexistent-profile")
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats.TotalWatchTimeMs)
	assert.Equal(t, 0, stats.TotalPlayCount)
}

func TestPlaySessionStatsRepo_GetUserStats_WithData(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	statsRepo := postgres.NewPlaySessionStatsReader(db)
	ctx := context.Background()

	u := newTestUser("pss-u1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := &user.Profile{ID: "pss-p1", UserID: shared.UserID(u.ID), Name: "Default", Preferences: map[string]string{}, IsDefault: true, CreatedAt: time.Now().UTC()}
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("pss-lib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("pss-mi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	session := newTestPlaySession("pss-ps1", string(p.ID), string(item.ID))
	dur, _ := shared.NewDurationMs(60000)
	session.Duration = &dur
	require.NoError(t, sessionRepo.Create(ctx, session))

	stats, err := statsRepo.GetUserStats(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.TotalPlayCount)
	assert.Equal(t, int64(60000), stats.TotalWatchTimeMs)
}

func TestPlaySessionStatsRepo_GetServerStats(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPlaySessionStatsReader(db)
	ctx := context.Background()

	stats, err := repo.GetServerStats(ctx)
	require.NoError(t, err)
	assert.NotNil(t, stats)
	assert.GreaterOrEqual(t, stats.TotalUsers, 0)
}

func TestPlaySessionStatsRepo_GetMostPopular_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPlaySessionStatsReader(db)
	ctx := context.Background()

	result, err := repo.GetMostPopular(ctx, "", 10)
	require.NoError(t, err)
	assert.Empty(t, result)
}

// TestPlaySessionStatsRepo_WatchTimeDedup_Resume proves that resuming the same
// media creates two sessions but TotalWatchTimeMs takes MAX, not SUM: the
// re-watched span must not be double-counted.
func TestPlaySessionStatsRepo_WatchTimeDedup_Resume(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	statsRepo := postgres.NewPlaySessionStatsReader(db)
	ctx := context.Background()

	u := newTestUser("wt1u1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("wt1p1", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("wt1lib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("wt1mi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	// Session A: first watch, 30 min of progress.
	sessA := newTestPlaySession("wt1ps1a", string(p.ID), string(item.ID))
	posA, err := shared.NewDurationMs(1_800_000)
	require.NoError(t, err)
	sessA.Position = &posA
	require.NoError(t, sessionRepo.Create(ctx, sessA))

	// Session B: resume, 60 min of progress (further into the same title).
	sessB := newTestPlaySession("wt1ps1b", string(p.ID), string(item.ID))
	posB, err := shared.NewDurationMs(3_600_000)
	require.NoError(t, err)
	sessB.Position = &posB
	require.NoError(t, sessionRepo.Create(ctx, sessB))

	stats, err := statsRepo.GetUserStats(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, stats.TotalPlayCount, "both sessions must be counted")
	// MAX(1_800_000, 3_600_000) = 3_600_000; SUM would give 5_400_000 (wrong).
	assert.Equal(t, int64(3_600_000), stats.TotalWatchTimeMs,
		"resume must not double-count the re-watched span: want MAX not SUM")
}

// TestPlaySessionStatsRepo_WatchTimeDedup_StaleSessionPositionFallback proves
// that sessions closed by CloseStale (duration_ms NULL, position_ms set) still
// contribute to watch-time via the COALESCE(position_ms, duration_ms, 0)
// formula instead of silently counting as 0.
func TestPlaySessionStatsRepo_WatchTimeDedup_StaleSessionPositionFallback(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	statsRepo := postgres.NewPlaySessionStatsReader(db)
	ctx := context.Background()

	u := newTestUser("wt2u1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("wt2p1", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("wt2lib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item1 := newTestMediaItem("wt2mi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item1))
	item2 := newTestMediaItem("wt2mi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item2))

	// Stale session: position_ms=2_400_000, duration_ms NULL (CloseStale path).
	stale := newTestPlaySession("wt2ps1", string(p.ID), string(item1.ID))
	pos, err := shared.NewDurationMs(2_400_000)
	require.NoError(t, err)
	stale.Position = &pos
	// Duration left nil → duration_ms NULL persisted.
	require.NoError(t, sessionRepo.Create(ctx, stale))

	// Normal session: position_ms NULL, duration_ms=60_000.
	normal := newTestPlaySession("wt2ps2", string(p.ID), string(item2.ID))
	dur, err := shared.NewDurationMs(60_000)
	require.NoError(t, err)
	normal.Duration = &dur
	require.NoError(t, sessionRepo.Create(ctx, normal))

	stats, err := statsRepo.GetUserStats(ctx, p.ID)
	require.NoError(t, err)
	// COALESCE(2_400_000, NULL, 0)=2_400_000 + COALESCE(NULL, 60_000, 0)=60_000
	assert.Equal(t, int64(2_460_000), stats.TotalWatchTimeMs,
		"stale session (position_ms) and normal session (duration_ms) must both contribute")
}

// TestPlaySessionStatsRepo_WatchTimeDedup_TwoDistinctMedia proves that the
// per-media MAX values are SUMMED across different titles: 1_000_000 + 2_000_000.
func TestPlaySessionStatsRepo_WatchTimeDedup_TwoDistinctMedia(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	statsRepo := postgres.NewPlaySessionStatsReader(db)
	ctx := context.Background()

	u := newTestUser("wt3u1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("wt3p1", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("wt3lib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item1 := newTestMediaItem("wt3mi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item1))
	item2 := newTestMediaItem("wt3mi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item2))

	sess1 := newTestPlaySession("wt3ps1", string(p.ID), string(item1.ID))
	pos1, err := shared.NewDurationMs(1_000_000)
	require.NoError(t, err)
	sess1.Position = &pos1
	require.NoError(t, sessionRepo.Create(ctx, sess1))

	sess2 := newTestPlaySession("wt3ps2", string(p.ID), string(item2.ID))
	pos2, err := shared.NewDurationMs(2_000_000)
	require.NoError(t, err)
	sess2.Position = &pos2
	require.NoError(t, sessionRepo.Create(ctx, sess2))

	stats, err := statsRepo.GetUserStats(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, stats.UniqueItemsWatched)
	// SUM over media of MAX-per-media: 1_000_000 + 2_000_000 = 3_000_000.
	assert.Equal(t, int64(3_000_000), stats.TotalWatchTimeMs)
}

// TestPlaySessionStatsRepo_GetServerStats_WatchTimeDedup proves that
// GetServerStats.TotalPlayTimeMs groups by (profile_id, media_item_id) and
// takes MAX so resume sessions are not double-counted at the server level.
func TestPlaySessionStatsRepo_GetServerStats_WatchTimeDedup(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	statsRepo := postgres.NewPlaySessionStatsReader(db)
	ctx := context.Background()

	u := newTestUser("wt4u1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("wt4p1", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("wt4lib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("wt4mi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	sessA := newTestPlaySession("wt4ps1a", string(p.ID), string(item.ID))
	posA, err := shared.NewDurationMs(1_800_000)
	require.NoError(t, err)
	sessA.Position = &posA
	require.NoError(t, sessionRepo.Create(ctx, sessA))

	sessB := newTestPlaySession("wt4ps1b", string(p.ID), string(item.ID))
	posB, err := shared.NewDurationMs(3_600_000)
	require.NoError(t, err)
	sessB.Position = &posB
	require.NoError(t, sessionRepo.Create(ctx, sessB))

	srvStats, err := statsRepo.GetServerStats(ctx)
	require.NoError(t, err)
	// MAX(1_800_000, 3_600_000) = 3_600_000; SUM would give 5_400_000 (wrong).
	assert.Equal(t, int64(3_600_000), srvStats.TotalPlayTimeMs,
		"server watch-time must de-dup resumes per (profile, media): want MAX not SUM")
}

// The server stats are cached for 5 minutes, and the live counters were
// cached with them: the admin dashboard, polling every 3 s, showed "0 active
// streams" beside two Now Playing cards. A playback that starts after the
// cache was filled must be counted on the very next read.
func TestPlaySessionStatsRepo_GetServerStats_ActiveStreamsAreNeverCached(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	statsRepo := postgres.NewPlaySessionStatsReader(db)
	ctx := context.Background()

	u := newTestUser("livu1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("livp1", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("livlib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("livmi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	require.NoError(t, sessionRepo.Create(ctx, newTestPlaySession("livps1", string(p.ID), string(item.ID))))
	first, err := statsRepo.GetServerStats(ctx) // fills the rollup cache
	require.NoError(t, err)
	require.Equal(t, 1, first.ActiveStreams)

	require.NoError(t, sessionRepo.Create(ctx, newTestPlaySession("livps2", string(p.ID), string(item.ID))))
	second, err := statsRepo.GetServerStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, second.ActiveStreams, "a playback started after the cache was filled must be counted at once")
}

// The load test's client mix counts recent playbacks per recorded device type,
// and nothing older than the window.
func TestPlaySessionStatsRepo_CountByDeviceTypeSince(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	statsRepo := postgres.NewPlaySessionStatsReader(db)
	ctx := context.Background()

	u := newTestUser("pss-dt-u1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := &user.Profile{ID: "pss-dt-p1", UserID: u.ID, Name: "Default", Preferences: map[string]string{}, IsDefault: true, CreatedAt: time.Now().UTC()}
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("pss-dt-lib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("pss-dt-mi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	now := time.Now().UTC()
	for i, s := range []struct {
		device string
		age    time.Duration
	}{
		{"Evey TV", time.Hour}, {"Evey TV", 2 * time.Hour}, {"Evey Web", time.Hour}, {"Evey Web", 40 * 24 * time.Hour},
	} {
		ps := newTestPlaySession(fmt.Sprintf("pss-dt-ps%d", i), string(p.ID), string(item.ID))
		ps.DeviceType = s.device
		ps.StartedAt = now.Add(-s.age).Truncate(time.Millisecond)
		require.NoError(t, sessionRepo.Create(ctx, ps))
	}

	got, err := statsRepo.CountByDeviceTypeSince(ctx, now.Add(-30*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"Evey TV": 2, "Evey Web": 1}, got)
}
