package postgres_test

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	appplayback "github.com/eveyproject/evey/internal/app/playback"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/require"
)

// Regression guard: N concurrent StartStream calls on a user with
// MaxStreams=1 must leave exactly one active session. Without the
// tx-scoped advisory lock the quota check + insert pair races and
// multiple sessions can be created past the cap.
func TestPlaySession_StartStream_TOCTOU_MaxStreams(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	watchProgressRepo := postgres.NewWatchProgressRepository(db)
	ctx := context.Background()

	u := newTestUser("toc-u")
	max := 1
	u.MaxStreams = &max
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("toc-p", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("toc-lib")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("toc-mi", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	svc := appplayback.NewService(
		watchProgressRepo, watchProgressRepo,
		sessionRepo, sessionRepo,
		userRepo, profileRepo,
		sharedfakes.NewFakeEventBus(),
		appplayback.WithUnitOfWork(postgres.NewTxUnit(db)),
	)

	const N = 20
	var wg sync.WaitGroup
	wg.Add(N)
	results := make(chan error, N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			sess := newTestPlaySession("toc-s"+strconv.Itoa(i), string(p.ID), string(item.ID))
			// Distinct device per goroutine: these must be genuinely separate
			// streams that each hit the MaxStreams quota, not the same
			// (profile, media, device) tuple the idempotency dedup would
			// collapse into one replayed session (which legitimately returns
			// nil for every caller and would never exercise the quota).
			sess.DeviceName = "toc-dev-" + strconv.Itoa(i)
			results <- svc.CommandService().StartStream(ctx, sess, u.ID)
		}(i)
	}
	wg.Wait()
	close(results)

	ok, rejected, other := 0, 0, 0
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, appplayback.ErrMaxStreams):
			rejected++
		default:
			t.Logf("unexpected error: %v", err)
			other++
		}
	}
	require.Equalf(t, 1, ok, "expected exactly 1 successful StartStream, got ok=%d rejected=%d other=%d", ok, rejected, other)
	require.Equal(t, N-1, rejected, "remaining calls must hit ErrMaxStreams")
}

// Regression guard for the OTHER race the per-user advisory lock serializes:
// N concurrent StartStream calls for the SAME (profile, media, device) tuple
// (a double-click / retry storm) must collapse into exactly ONE active session
// row via idempotency dedup, never N. Without the lock two callers can both
// miss the not-yet-committed session and both INSERT.
func TestPlaySession_StartStream_ConcurrentDedupSameDevice(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	watchProgressRepo := postgres.NewWatchProgressRepository(db)
	ctx := context.Background()

	// No stream cap here: the dedup, not the quota, must bound the row count.
	u := newTestUser("dedup-u")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("dedup-p", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("dedup-lib")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("dedup-mi", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	svc := appplayback.NewService(
		watchProgressRepo, watchProgressRepo,
		sessionRepo, sessionRepo,
		userRepo, profileRepo,
		sharedfakes.NewFakeEventBus(),
		appplayback.WithUnitOfWork(postgres.NewTxUnit(db)),
	)

	const N = 20
	var wg sync.WaitGroup
	wg.Add(N)
	results := make(chan error, N)
	for i := range N {
		go func(i int) {
			defer wg.Done()
			// Same device on purpose: every call is the same logical stream.
			sess := newTestPlaySession("dedup-s"+strconv.Itoa(i), string(p.ID), string(item.ID))
			results <- svc.CommandService().StartStream(ctx, sess, u.ID)
		}(i)
	}
	wg.Wait()
	close(results)

	for err := range results {
		require.NoError(t, err, "every concurrent same-device StartStream must succeed (dedup replay)")
	}
	count, err := sessionRepo.CountActiveByUser(ctx, string(u.ID))
	require.NoError(t, err)
	require.Equal(t, 1, count, "concurrent same-device starts must create exactly one active session row")
}
