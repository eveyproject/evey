package ratelimit_test

import (
	"sync"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

func TestFailureGate_ClosesOnThresholdAndReopensAfterCooldown(t *testing.T) {
	t.Parallel()
	now := time.Now()
	clock := func() time.Time { return now }
	g := ratelimit.NewFailureGateForTest("test", 3, time.Minute, 5*time.Minute, clock)

	g.Fail()
	g.Fail()
	if !g.Allow() {
		t.Fatal("gate closed below its threshold")
	}

	g.Fail()
	if g.Allow() {
		t.Fatal("gate stayed open at its threshold - a distributed enumeration walks straight through")
	}

	now = now.Add(4 * time.Minute)
	if g.Allow() {
		t.Fatal("gate reopened before the cooldown elapsed")
	}
	now = now.Add(2 * time.Minute)
	if !g.Allow() {
		t.Fatal("gate never reopened - honest guests are locked out for good")
	}
}

// Failures spread thinner than the window must not accumulate: a household
// where someone mistypes a code once an hour would otherwise eventually shut
// the door on itself.
func TestFailureGate_WindowResets(t *testing.T) {
	t.Parallel()
	now := time.Now()
	clock := func() time.Time { return now }
	g := ratelimit.NewFailureGateForTest("test", 3, time.Minute, 5*time.Minute, clock)

	for range 10 {
		g.Fail()
		now = now.Add(2 * time.Minute)
	}
	if !g.Allow() {
		t.Fatal("scattered failures closed the gate")
	}
}

func TestFailureGate_SucceedClearsTheCount(t *testing.T) {
	t.Parallel()
	now := time.Now()
	clock := func() time.Time { return now }
	g := ratelimit.NewFailureGateForTest("test", 3, time.Minute, 5*time.Minute, clock)

	g.Fail()
	g.Fail()
	g.Succeed()
	g.Fail()
	g.Fail()
	if !g.Allow() {
		t.Fatal("a success did not clear the running count")
	}
}

func TestFailureGate_NilIsOpen(t *testing.T) {
	t.Parallel()
	var g *ratelimit.FailureGate
	if !g.Allow() {
		t.Fatal("an unconfigured gate must not refuse traffic")
	}
	g.Fail()
	g.Succeed()
}

func TestFailureGate_Concurrent(t *testing.T) {
	t.Parallel()
	g := ratelimit.NewFailureGate("test", 1000, time.Minute, time.Minute)
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			for range 20 {
				g.Fail()
				g.Allow()
				g.Succeed()
			}
		})
	}
	wg.Wait()
}
