package mkvgo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
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
// ffmpeg pipeline together) but must NOT be cached: the next request retries.
func TestOnDemandPlan_ErrorSharedButNotCached(t *testing.T) {
	p := NewOnDemandPackager()
	defer p.Close()
	src := stampedeSrc(t)

	var builds atomic.Int32
	boom := errors.New("unsupported codec")
	p.build = func(context.Context, string, apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
		builds.Add(1)
		return nil, boom
	}
	if _, err := p.Plan(context.Background(), src, apptranscoding.OnDemandOptions{}); !errors.Is(err, boom) {
		t.Fatalf("want the build error, got %v", err)
	}
	if _, err := p.Plan(context.Background(), src, apptranscoding.OnDemandOptions{}); !errors.Is(err, boom) {
		t.Fatalf("want the build error on retry, got %v", err)
	}
	if got := builds.Load(); got != 2 {
		t.Fatalf("errors must not be cached: builds=%d, want 2", got)
	}
}

// The flight's winner must build under a cancel-detached context: the first
// caller bailing out (seek away, tab close) must not abort the walk every
// follower is waiting on. Same contract as the remux singleflight.
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

	if err := <-done; err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if <-sawCancel {
		t.Fatal("build ran under the caller's cancelable ctx - a bailing caller would abort every follower's plan")
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
