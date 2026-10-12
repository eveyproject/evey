package playback

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/playback"
	pbfakes "github.com/eveyproject/evey/internal/domain/playback/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

// captureSlogBuffer redirects slog output to a buffer.
func captureSlogBuffer(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return &buf, func() { slog.SetDefault(prev) }
}

func newCmdSvc() (
	*CommandService,
	*pbfakes.FakeWatchProgressRepository,
	*pbfakes.FakePlaySessionRepository,
	*userfakes.FakeUserRepository,
	*userfakes.FakeProfileRepository,
	*sharedfakes.FakeEventBus,
) {
	pr := pbfakes.NewFakeWatchProgressRepository()
	sr := pbfakes.NewFakePlaySessionRepository()
	ur := userfakes.NewFakeUserRepository()
	pfr := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmd := NewCommandService(pr, sr, sr, ur, pfr, bus)
	return cmd, pr, sr, ur, pfr, bus
}

func seed(ctx context.Context, ur *userfakes.FakeUserRepository, pfr *userfakes.FakeProfileRepository) {
	_ = ur.Create(ctx, &user.User{ID: "u1", Username: "alice", Enabled: true})
	_ = pfr.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})
}

// --- Lines 82, 106, 174: assert slog.Warn emitted on event publish error ---

func TestStartStream_PublishError_LogsWarn(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	cmd, _, _, ur, pfr, bus := newCmdSvc()
	ctx := context.Background()
	seed(ctx, ur, pfr)
	bus.PublishErr = errors.New("bus down")

	if err := cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"); err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "publish event") || !strings.Contains(out, "playback.started") {
		t.Fatalf("expected warn for playback.started, got %q", out)
	}
}

func TestStartStream_PublishOK_NoWarn(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	cmd, _, _, ur, pfr, _ := newCmdSvc()
	ctx := context.Background()
	seed(ctx, ur, pfr)

	if err := cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "publish event") {
		t.Fatalf("did not expect warn, got %q", buf.String())
	}
}

func TestStopStream_PublishError_LogsWarn(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	cmd, _, _, ur, pfr, bus := newCmdSvc()
	ctx := context.Background()
	seed(ctx, ur, pfr)
	if err := cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"); err != nil {
		t.Fatal(err)
	}
	bus.PublishErr = errors.New("bus down")

	if err := cmd.StopStream(ctx, "s1"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "publish event") || !strings.Contains(out, "playback.stopped") {
		t.Fatalf("expected warn for playback.stopped, got %q", out)
	}
}

func TestStopStream_PublishOK_NoWarn(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	cmd, _, _, ur, pfr, _ := newCmdSvc()
	ctx := context.Background()
	seed(ctx, ur, pfr)
	if err := cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.StopStream(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "publish event") {
		t.Fatalf("did not expect warn, got %q", buf.String())
	}
}

func TestMarkAsWatched_PublishError_LogsWarn(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	cmd, _, _, _, pfr, bus := newCmdSvc()
	ctx := context.Background()
	_ = pfr.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})
	bus.PublishErr = errors.New("bus down")

	if err := cmd.MarkAsWatched(ctx, "p1", "m1"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "publish event") || !strings.Contains(out, "media.marked.watched") {
		t.Fatalf("expected warn for media.marked.watched, got %q", out)
	}
}

func TestMarkAsWatched_PublishOK_NoWarn(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	cmd, _, _, _, pfr, _ := newCmdSvc()
	ctx := context.Background()
	_ = pfr.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	if err := cmd.MarkAsWatched(ctx, "p1", "m1"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "publish event") {
		t.Fatalf("did not expect warn, got %q", buf.String())
	}
}

// --- Lines 222 (err != nil) & 224 (n > 0): runCleanup branch logs ---

func TestRunCleanup_CloseStaleError_LogsWarn(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	cmd, _, sr, _, _, _ := newCmdSvc()
	sr.Err = errors.New("db down")
	cmd.runCleanup(context.Background(), time.Hour)

	out := buf.String()
	if !strings.Contains(out, "stale session cleanup") {
		t.Fatalf("expected warn 'stale session cleanup', got %q", out)
	}
	if strings.Contains(out, "cleaned stale sessions") {
		t.Fatalf("did not expect info 'cleaned stale sessions', got %q", out)
	}
}

func TestRunCleanup_NPositive_LogsInfo(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	cmd, _, sr, _, _, _ := newCmdSvc()
	ctx := context.Background()
	_ = sr.Create(ctx, &playback.PlaySession{
		ID: "s-stale", ProfileID: "p1", MediaItemID: "m1",
		StartedAt: time.Now().Add(-3 * time.Hour),
	})
	cmd.runCleanup(ctx, time.Hour)

	out := buf.String()
	if !strings.Contains(out, "cleaned stale sessions") {
		t.Fatalf("expected info 'cleaned stale sessions', got %q", out)
	}
	if !strings.Contains(out, "count=1") {
		t.Fatalf("expected count=1 in log, got %q", out)
	}
	if strings.Contains(out, "stale session cleanup") {
		t.Fatalf("did not expect warn, got %q", out)
	}
}

// The retention purge deletes rows, so its window is pinned: a finished
// session older than the retention goes, a recent one and an open one stay,
// and the purge runs once a day, not on every 30s sweep. Neither the cutoff
// nor the daily gate had a test (mutation 2026-09-28): a sign slip on the
// cutoff would delete the recent history instead.
func TestRunCleanup_RetentionPurgeWindow(t *testing.T) {
	cmd, _, sr, _, _, _ := newCmdSvc()
	cmd.sessionRetention = func() int { return 30 }
	ctx := context.Background()
	ended := func(daysAgo int) *time.Time { t := time.Now().AddDate(0, 0, -daysAgo); return &t }
	_ = sr.Create(ctx, &playback.PlaySession{ID: "s-old", ProfileID: "p1", MediaItemID: "m1", StartedAt: time.Now().AddDate(0, 0, -41), EndedAt: ended(40)})
	_ = sr.Create(ctx, &playback.PlaySession{ID: "s-recent", ProfileID: "p1", MediaItemID: "m1", StartedAt: time.Now().AddDate(0, 0, -11), EndedAt: ended(10)})
	_ = sr.Create(ctx, &playback.PlaySession{ID: "s-open", ProfileID: "p1", MediaItemID: "m1", StartedAt: time.Now()})

	cmd.runCleanup(ctx, time.Hour)
	if s, _ := sr.GetByID(ctx, "s-old"); s != nil {
		t.Error("a session finished 40 days ago must be purged under a 30-day retention")
	}
	for _, id := range []string{"s-recent", "s-open"} {
		if s, _ := sr.GetByID(ctx, id); s == nil {
			t.Errorf("%s must survive the purge", id)
		}
	}

	// A second sweep the same day does not purge again.
	_ = sr.Create(ctx, &playback.PlaySession{ID: "s-old-2", ProfileID: "p1", MediaItemID: "m1", StartedAt: time.Now().AddDate(0, 0, -41), EndedAt: ended(40)})
	cmd.runCleanup(ctx, time.Hour)
	if s, _ := sr.GetByID(ctx, "s-old-2"); s == nil {
		t.Error("the purge runs once a day: the next 30s sweep must not purge again")
	}
}

func TestRunCleanup_NZero_NoLog(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	cmd, _, _, _, _, _ := newCmdSvc()
	cmd.runCleanup(context.Background(), time.Hour)

	out := buf.String()
	if strings.Contains(out, "cleaned stale sessions") {
		t.Fatalf("did not expect info when n=0, got %q", out)
	}
	if strings.Contains(out, "stale session cleanup") {
		t.Fatalf("did not expect warn when no err, got %q", out)
	}
}

// --- Line 203: defer recover() catches panic in cleanup goroutine ---

type panicSessionRepo struct {
	*pbfakes.FakePlaySessionRepository
	panicked atomic.Bool
	done     chan struct{}
}

func (r *panicSessionRepo) CloseStale(_ context.Context, _ time.Duration) (int, error) {
	if r.panicked.CompareAndSwap(false, true) {
		close(r.done)
		panic("boom cleanup")
	}
	return 0, nil
}

func TestStartCleanupWithInterval_NoPanic_NoRecoverLog(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	cmd, _, _, _, _, _ := newCmdSvc()
	ctx, cancel := context.WithCancel(context.Background())
	cmd.StartCleanupWithInterval(ctx, time.Hour, time.Hour)
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	if strings.Contains(buf.String(), "panic in playback cleanup") {
		t.Fatalf("did not expect panic recover log when no panic, got %q", buf.String())
	}
}

func TestStartCleanupWithInterval_RecoversFromPanic(t *testing.T) {
	pr := pbfakes.NewFakeWatchProgressRepository()
	inner := pbfakes.NewFakePlaySessionRepository()
	sr := &panicSessionRepo{FakePlaySessionRepository: inner, done: make(chan struct{})}
	ur := userfakes.NewFakeUserRepository()
	pfr := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmd := NewCommandService(pr, sr, inner, ur, pfr, bus)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd.StartCleanupWithInterval(ctx, time.Hour, time.Hour)

	select {
	case <-sr.done:
	case <-time.After(2 * time.Second):
		t.Fatal("CloseStale never called")
	}
	// give goroutine time to recover and exit; if recover didn't fire, test process would crash
	time.Sleep(50 * time.Millisecond)
	if !sr.panicked.Load() {
		t.Fatal("expected panic to have occurred")
	}
}
