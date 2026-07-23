package coalesce

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A trigger must run fn. Bursts arriving WHILE fn runs must collapse into
// exactly one follow-up run - the follow-up starts after the current run ends,
// so its snapshot is at least as fresh as the last trigger. This is the lost
// update the video-session re-arm had: two concurrent triggers interleaved
// read/compute/apply and the staler apply could land last.
func TestRunner_CoalescesBurstIntoOneFollowUpRun(t *testing.T) {
	runs := make(chan struct{}, 16)
	release := make(chan struct{})
	first := make(chan struct{})
	var once sync.Once
	r := Start(t.Context(), func(context.Context) {
		runs <- struct{}{}
		once.Do(func() { close(first) })
		<-release
	})

	r.Trigger()
	<-runs // first run started and now blocks in fn
	<-first
	for range 5 {
		r.Trigger() // burst while fn is running
	}
	close(release) // let every run through from here on

	select {
	case <-runs: // exactly one coalesced follow-up
	case <-time.After(2 * time.Second):
		t.Fatal("burst during a run must schedule one follow-up run")
	}
	select {
	case <-runs:
		t.Fatal("5 triggers during one run must coalesce into ONE follow-up, got a third run")
	case <-time.After(150 * time.Millisecond):
	}
}

// Runs must never overlap: fn does read-compute-apply against shared state and
// two interleaved runs are exactly the lost update this type exists to prevent.
func TestRunner_RunsNeverOverlap(t *testing.T) {
	var inFlight, maxSeen atomic.Int32
	var wg sync.WaitGroup
	r := Start(t.Context(), func(context.Context) {
		cur := inFlight.Add(1)
		for {
			m := maxSeen.Load()
			if cur <= m || maxSeen.CompareAndSwap(m, cur) {
				break
			}
		}
		time.Sleep(time.Millisecond)
		inFlight.Add(-1)
	})

	for range 8 {
		wg.Go(func() {
			for range 50 {
				r.Trigger()
			}
		})
	}
	wg.Wait()
	time.Sleep(100 * time.Millisecond)
	if got := maxSeen.Load(); got > 1 {
		t.Fatalf("runs overlapped: max in-flight = %d, want <= 1", got)
	}
}

// Trigger must NEVER block, whatever the state: it is called synchronously
// from notifyCapabilityChange while the calibration service holds demoteMu +
// currentMu, on the encode-failure path. A blocking trigger would serialize
// demotions on fn's duration (a Postgres read in the real wiring).
func TestRunner_TriggerNeverBlocks(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	r := Start(t.Context(), func(context.Context) { <-block })

	done := make(chan struct{})
	go func() {
		for range 1000 {
			r.Trigger()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Trigger blocked while fn was running and the pending slot was full")
	}
}

func TestRunner_StopsWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	runs := make(chan struct{}, 4)
	r := Start(ctx, func(context.Context) { runs <- struct{}{} })

	r.Trigger()
	select {
	case <-runs:
	case <-time.After(2 * time.Second):
		t.Fatal("runner never ran")
	}

	cancel()
	time.Sleep(50 * time.Millisecond) // let the goroutine observe ctx.Done
	r.Trigger()
	select {
	case <-runs:
		t.Fatal("runner must not run after its context ended")
	case <-time.After(150 * time.Millisecond):
	}
}
