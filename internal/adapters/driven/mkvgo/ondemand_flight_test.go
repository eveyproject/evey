package mkvgo

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

// countingBuilds makes p's builds count per source and wait on gate, and
// signals started each time one begins.
func countingBuilds(p *OnDemandPackager, gate <-chan struct{}) (map[string]*atomic.Int32, *sync.Mutex, chan string) {
	var mu sync.Mutex
	counts := map[string]*atomic.Int32{}
	started := make(chan string, 16)
	p.build = func(_ context.Context, src string, _ apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
		mu.Lock()
		if counts[src] == nil {
			counts[src] = &atomic.Int32{}
		}
		counts[src].Add(1)
		mu.Unlock()
		started <- src
		<-gate
		return &stubPlan{}, nil
	}
	return counts, &mu, started
}

func builtCount(counts map[string]*atomic.Int32, mu *sync.Mutex, src string) int32 {
	mu.Lock()
	defer mu.Unlock()
	if c := counts[src]; c != nil {
		return c.Load()
	}
	return 0
}

// A build every caller left while it queued for a slot is never run, and the
// next viewer of that source starts a fresh one; while one caller still waits,
// the others leaving changes nothing.
func TestOnDemandPlan_FlightLeftBeforeItsSlotIsNotBuilt(t *testing.T) {
	p := NewOnDemandPackager().WithMaxBuilds(1)
	defer p.Close()
	gate := make(chan struct{})
	counts, mu, started := countingBuilds(p, gate)
	busy, left, kept := stampedeSrc(t), stampedeSrc(t), stampedeSrc(t)

	// The only slot is taken.
	go func() { _, _ = p.Plan(context.Background(), busy, apptranscoding.OnDemandOptions{}) }()
	if got := <-started; got != busy {
		t.Fatalf("first build = %s, want the busy source", got)
	}

	// Two callers queue for `left` and both leave.
	var wg sync.WaitGroup
	for range 2 {
		ctx, cancel := context.WithCancel(context.Background())
		wg.Go(func() {
			_, err := p.Plan(ctx, left, apptranscoding.OnDemandOptions{})
			if !errors.Is(err, context.Canceled) {
				t.Errorf("a caller that left got %v, want context.Canceled", err)
			}
		})
		time.AfterFunc(50*time.Millisecond, cancel)
	}
	// Two callers queue for `kept`; one leaves, the other stays.
	keptCtx, keptCancel := context.WithCancel(context.Background())
	wg.Go(func() { _, _ = p.Plan(keptCtx, kept, apptranscoding.OnDemandOptions{}) })
	var keptErr error
	var keptWG sync.WaitGroup
	keptWG.Go(func() { _, keptErr = p.Plan(context.Background(), kept, apptranscoding.OnDemandOptions{}) })
	time.AfterFunc(50*time.Millisecond, keptCancel)

	wg.Wait() // every caller that left is back, the slot never freed
	close(gate)
	keptWG.Wait()
	if keptErr != nil {
		t.Fatalf("the caller that stayed got %v", keptErr)
	}
	// The slot is free from here on: an abandoned flight still queued would
	// take it within moments.
	for deadline := time.Now().Add(200 * time.Millisecond); time.Now().Before(deadline); {
		if got := builtCount(counts, mu, left); got != 0 {
			t.Fatalf("a build every caller left was run %d times", got)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := builtCount(counts, mu, kept); got != 1 {
		t.Fatalf("a build one caller still waited on ran %d times, want 1", got)
	}

	// Not remembered as a failure: the next viewer gets the plan.
	if _, err := p.Plan(context.Background(), left, apptranscoding.OnDemandOptions{}); err != nil {
		t.Fatalf("the next viewer of an abandoned source got %v", err)
	}
	if got := builtCount(counts, mu, left); got != 1 {
		t.Fatalf("the next viewer built %d times, want 1", got)
	}
}

// A build that got its slot runs to the end even if every caller left, and
// its plan is cached for the next viewer.
func TestOnDemandPlan_FlightLeftAfterItsSlotIsCached(t *testing.T) {
	p := NewOnDemandPackager().WithMaxBuilds(1)
	defer p.Close()
	gate := make(chan struct{})
	counts, mu, started := countingBuilds(p, gate)
	src := stampedeSrc(t)

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		_, err := p.Plan(ctx, src, apptranscoding.OnDemandOptions{})
		errc <- err
	}()
	<-started
	cancel()
	select {
	case err := <-errc:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("the caller that left got %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("a caller that left kept waiting on the build")
	}

	close(gate)
	pl, err := p.Plan(context.Background(), src, apptranscoding.OnDemandOptions{})
	if err != nil || pl == nil {
		t.Fatalf("next viewer: plan %v, err %v", pl, err)
	}
	if got := builtCount(counts, mu, src); got != 1 {
		t.Fatalf("built %d times, want the one build left running", got)
	}
	if _, ok := p.plans.Get(onDemandCacheKey(src, apptranscoding.OnDemandOptions{})); !ok {
		t.Fatal("the plan of a build every caller left is not cached")
	}
}
