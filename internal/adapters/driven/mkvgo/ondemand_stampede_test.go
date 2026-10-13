package mkvgo

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	extmatroska "github.com/gravity-zero/mkvgo/matroska"
	"github.com/gravity-zero/mkvgo/mp4"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

// stubPlan satisfies OnDemandPlan without touching mkvgo - the stampede tests
// only care about HOW MANY times the build ran, never about the plan's bytes.
type stubPlan struct{ id int32 }

func (s *stubPlan) Resource(context.Context, string) ([]byte, string, error) {
	return nil, "", errors.New("stub")
}
func (s *stubPlan) Resources() []string { return nil }
func (s *stubPlan) NumSegments() int    { return 0 }
func (s *stubPlan) HasDASH() bool       { return false }
func (s *stubPlan) SourceID() string    { return "" }

func stampedeSrc(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// N players hitting a COLD source at stream start must not walk the same
// multi-GB file N times in parallel: plan building can cluster-walk the whole
// source (SynthesizeIndex), and N concurrent walks on a weak NAS is the exact
// I/O-starvation shape of the 2026-07-19 incident. One flight builds, the
// followers wait and share.
func TestOnDemandPlan_ConcurrentMissesBuildOnce(t *testing.T) {
	p := NewOnDemandPackager()
	defer p.Close()
	src := stampedeSrc(t)

	var builds atomic.Int32
	gate := make(chan struct{})
	p.build = func(ctx context.Context, _ string, _ apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
		n := builds.Add(1)
		<-gate // hold every concurrent caller in the miss window
		return &stubPlan{id: n}, nil
	}

	const callers = 20
	var wg sync.WaitGroup
	results := make([]apptranscoding.OnDemandPlan, callers)
	errs := make([]error, callers)
	for i := range callers {
		wg.Go(func() {
			results[i], errs[i] = p.Plan(context.Background(), src, apptranscoding.OnDemandOptions{SegmentMs: 6000})
		})
	}
	time.Sleep(100 * time.Millisecond) // let every caller reach the flight
	close(gate)
	wg.Wait()

	if got := builds.Load(); got != 1 {
		t.Fatalf("20 concurrent cold Plan() calls built %d times, want exactly 1", got)
	}
	for i := range callers {
		if errs[i] != nil {
			t.Fatalf("caller %d failed: %v", i, errs[i])
		}
		if results[i] != results[0] {
			t.Fatalf("caller %d got a different plan instance than caller 0", i)
		}
	}

	// The plan must have been cached by the flight: a later call is a pure
	// cache hit, no new build.
	if _, err := p.Plan(context.Background(), src, apptranscoding.OnDemandOptions{SegmentMs: 6000}); err != nil {
		t.Fatal(err)
	}
	if got := builds.Load(); got != 1 {
		t.Fatalf("post-flight call rebuilt (builds=%d), the flight must populate the cache", got)
	}
}

// Distinct cache keys are distinct flights: two different sources must build
// independently, not serialize behind one another.
func TestOnDemandPlan_DistinctKeysBuildIndependently(t *testing.T) {
	p := NewOnDemandPackager()
	defer p.Close()
	srcA, srcB := stampedeSrc(t), stampedeSrc(t)

	var builds atomic.Int32
	p.build = func(context.Context, string, apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
		return &stubPlan{id: builds.Add(1)}, nil
	}
	if _, err := p.Plan(context.Background(), srcA, apptranscoding.OnDemandOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Plan(context.Background(), srcB, apptranscoding.OnDemandOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := builds.Load(); got != 2 {
		t.Fatalf("two distinct sources built %d times, want 2", got)
	}
}

// A failed build is shared by the waiting callers (they all fall back to the
// ffmpeg pipeline together) and remembered: the next requests fall back at once
// instead of paying the failing read again. A changed file is a new key and is
// planned straight away.
func TestOnDemandPlan_FailureRememberedUntilTheFileChanges(t *testing.T) {
	p := NewOnDemandPackager()
	defer p.Close()
	src := stampedeSrc(t)

	var builds atomic.Int32
	boom := errors.New("unsupported codec")
	p.build = func(context.Context, string, apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
		builds.Add(1)
		return nil, boom
	}
	for range 3 {
		if _, err := p.Plan(context.Background(), src, apptranscoding.OnDemandOptions{}); !errors.Is(err, boom) {
			t.Fatalf("want the build error, got %v", err)
		}
	}
	if got := builds.Load(); got != 1 {
		t.Fatalf("a failed source was rebuilt: builds=%d, want 1", got)
	}

	if err := os.WriteFile(src, []byte("repaired"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Plan(context.Background(), src, apptranscoding.OnDemandOptions{}); !errors.Is(err, boom) {
		t.Fatalf("want the build error, got %v", err)
	}
	if got := builds.Load(); got != 2 {
		t.Fatalf("a changed file must be planned again: builds=%d, want 2", got)
	}
}

// A build cut short by a cancellation says nothing about the file: it is not
// remembered as a failure.
func TestOnDemandPlan_CancellationIsNotRemembered(t *testing.T) {
	p := NewOnDemandPackager()
	defer p.Close()
	src := stampedeSrc(t)

	var builds atomic.Int32
	p.build = func(context.Context, string, apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
		builds.Add(1)
		return nil, context.Canceled
	}
	for range 2 {
		_, _ = p.Plan(context.Background(), src, apptranscoding.OnDemandOptions{})
	}
	if got := builds.Load(); got != 2 {
		t.Fatalf("a cancelled build was remembered as a failure: builds=%d, want 2", got)
	}
}

// However many sources an audience opens at once, at most onDemandMaxBuilds
// plans are built at the same time.
func TestOnDemandPlan_BuildsAreCapped(t *testing.T) {
	p := NewOnDemandPackager()
	defer p.Close()

	var running, peak atomic.Int32
	gate := make(chan struct{})
	p.build = func(context.Context, string, apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
		n := running.Add(1)
		for {
			m := peak.Load()
			if n <= m || peak.CompareAndSwap(m, n) {
				break
			}
		}
		<-gate
		running.Add(-1)
		return &stubPlan{}, nil
	}
	var wg sync.WaitGroup
	for range 3 * onDemandMaxBuilds {
		src := stampedeSrc(t)
		wg.Go(func() {
			_, _ = p.Plan(context.Background(), src, apptranscoding.OnDemandOptions{})
		})
	}
	time.Sleep(100 * time.Millisecond)
	if got := peak.Load(); got != onDemandMaxBuilds {
		t.Fatalf("%d builds ran at once, want the cap of %d", got, onDemandMaxBuilds)
	}
	close(gate)
	wg.Wait()
}

// The flight must build under a cancel-detached context: the first caller
// bailing out (seek away, tab close) must not abort the walk every follower is
// waiting on. That caller stops waiting; the plan still lands in the cache.
func TestOnDemandPlan_WinnerSurvivesFirstCallerCancel(t *testing.T) {
	p := NewOnDemandPackager()
	defer p.Close()
	src := stampedeSrc(t)

	canceled := make(chan struct{})
	sawCancel := make(chan bool, 1)
	p.build = func(ctx context.Context, _ string, _ apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
		<-canceled // the caller's ctx is canceled by now
		sawCancel <- ctx.Err() != nil
		return &stubPlan{}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := p.Plan(ctx, src, apptranscoding.OnDemandOptions{})
		done <- err
	}()
	time.Sleep(50 * time.Millisecond) // let the flight start
	cancel()
	close(canceled)

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("the caller that bailed got %v, want context.Canceled", err)
	}
	if <-sawCancel {
		t.Fatal("build ran under the caller's cancelable ctx - a bailing caller would abort every follower's plan")
	}
	if _, err := p.Plan(context.Background(), src, apptranscoding.OnDemandOptions{}); err != nil {
		t.Fatalf("the plan built for a caller that left is not served: %v", err)
	}
}

// A plan in use is kept: its expiry slides with every request, so a film
// watched past the TTL keeps the plan (and what mkvgo learned serving it).
// An idle plan still expires.
func TestOnDemandPlan_ExpirySlidesWithUse(t *testing.T) {
	p := NewOnDemandPackager()
	defer p.Close()
	p.ttl = 80 * time.Millisecond
	src := stampedeSrc(t)
	var builds atomic.Int32
	p.build = func(context.Context, string, apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
		return &stubPlan{id: builds.Add(1)}, nil
	}
	opts := apptranscoding.OnDemandOptions{SegmentMs: 6000}
	for range 6 { // 6 x 30 ms = 180 ms of use, more than twice the TTL
		if _, err := p.Plan(context.Background(), src, opts); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
	}
	if n := builds.Load(); n != 1 {
		t.Fatalf("a plan in use was rebuilt: %d builds", n)
	}
	time.Sleep(150 * time.Millisecond)
	if _, err := p.Plan(context.Background(), src, opts); err != nil {
		t.Fatal(err)
	}
	if n := builds.Load(); n != 2 {
		t.Fatalf("an idle plan must expire: %d builds", n)
	}
}

// A build that outlasts slowAfter is named while it still runs and again when
// it ends, with its file: the heap profile names the code holding a peak, only
// this line names the source. A quick build says nothing.
func TestOnDemandPlan_SlowBuildIsNamedWithItsFile(t *testing.T) {
	p := NewOnDemandPackager()
	defer p.Close()
	var logs bytes.Buffer
	p.logger = slog.New(slog.NewTextHandler(&logs, nil))
	p.slowAfter = 20 * time.Millisecond

	slow, quick := stampedeSrc(t), stampedeSrc(t)
	p.build = func(_ context.Context, path string, _ apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
		if path == slow {
			time.Sleep(80 * time.Millisecond)
		}
		return &stubPlan{}, nil
	}
	if _, err := p.Plan(context.Background(), quick, apptranscoding.OnDemandOptions{}); err != nil {
		t.Fatal(err)
	}
	if logs.Len() != 0 {
		t.Fatalf("a quick build was logged: %s", logs.String())
	}
	if _, err := p.Plan(context.Background(), slow, apptranscoding.OnDemandOptions{}); err != nil {
		t.Fatal(err)
	}
	out := logs.String()
	for _, want := range []string{"plan build still running", "slow plan build", "build_ms="} {
		if !strings.Contains(out, want) {
			t.Fatalf("log lacks %q:\n%s", want, out)
		}
	}
	if n := strings.Count(out, slow); n != 2 {
		t.Fatalf("the slow file is named %d time(s), want 2 (while running, at the end):\n%s", n, out)
	}
}

// statsPlan is a stub plan that reports fixed window counters.
type statsPlan struct {
	stubPlan
	st mp4.HLSPlanStats
}

func (s *statsPlan) Stats() mp4.HLSPlanStats { return s.st }

// WindowStats sums the counters of every plan the cache holds and says how
// many it summed; a plan with no counters is left out.
func TestOnDemandPackager_WindowStatsSumsTheCachedPlans(t *testing.T) {
	p := NewOnDemandPackager()
	defer p.Close()
	plans := []apptranscoding.OnDemandPlan{
		&statsPlan{st: mp4.HLSPlanStats{WindowBuilds: 10, WindowRebuilds: 2, Evictions: 1, ServedBytes: 100, DroppedBytes: 7}},
		&statsPlan{st: mp4.HLSPlanStats{WindowBuilds: 5, WindowRebuilds: 3, ServedBytes: 50, SubtitleWalks: 1, SubtitleIndexedBlocks: 40}},
		&stubPlan{},
	}
	i := 0
	p.build = func(context.Context, string, apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
		pl := plans[i]
		i++
		return pl, nil
	}
	for range plans {
		if _, err := p.Plan(context.Background(), stampedeSrc(t), apptranscoding.OnDemandOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	got := p.WindowStats()
	want := map[string]int64{"plans": 2, "window_builds": 15, "window_rebuilds": 5, "evictions": 1, "served_bytes": 150, "dropped_bytes": 7, "subtitle_walks": 1, "subtitle_indexed_blocks": 40}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %d, want %d (all: %v)", k, got[k], v, got)
		}
	}
}

// A Matroska source gets the subtitle index stored for it; a source with no
// index, a non-Matroska source and a packager with no lookup get none.
func TestOnDemandPackager_SubtitleIndexForAMatroskaSourceOnly(t *testing.T) {
	p := NewOnDemandPackager()
	defer p.Close()
	if p.subtitleIndexFor("/m/film.mkv") != nil {
		t.Fatal("no lookup wired, yet an index was found")
	}
	stored := &extmatroska.SubtitleIndex{}
	var asked []string
	p.WithSubtitleIndex(func(path string) (*extmatroska.SubtitleIndex, bool) {
		asked = append(asked, path)
		return stored, path == "/m/film.mkv"
	})
	if got := p.subtitleIndexFor("/m/film.mkv"); got != mp4.SubtitleBlockIndex(stored) {
		t.Fatalf("a Matroska source with a stored index got %v", got)
	}
	if p.subtitleIndexFor("/m/other.mkv") != nil {
		t.Fatal("a Matroska source with no stored index got one")
	}
	if p.subtitleIndexFor("/m/film.mp4") != nil || len(asked) != 2 {
		t.Fatalf("an MP4 source was looked up or given an index (lookups: %v)", asked)
	}
}
