package procfork

import (
	"context"
	"errors"
	"testing"
	"time"
)

func fakeAdmission(available int64, budget int64) (*admission, *int) {
	sleeps := 0
	a := newAdmission(
		func() (int64, bool) { return available, true },
		func() (int64, bool) { return budget, true },
	)
	clock := time.Unix(0, 0)
	a.now = func() time.Time { return clock }
	a.sleep = func(_ context.Context, d time.Duration) error { sleeps++; clock = clock.Add(d); return nil }
	a.wait = 3 * a.poll
	return a, &sleeps
}

func TestAdmit_ProductionFailsFastWhenCapsWouldNotFit(t *testing.T) {
	t.Parallel()
	// 16 GiB budget: reserve 1 GiB. 9 GiB free holds two 4 GiB productions, not three.
	a, sleeps := fakeAdmission(9*gib, 16*gib)
	need := RSSCap(Production, 16*gib)
	r1, err := a.admit(context.Background(), Production, "t", need)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := a.admit(context.Background(), Production, "t", need)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.admit(context.Background(), Production, "t", need); !errors.Is(err, ErrMemoryPressure) {
		t.Fatalf("third production must be refused, got %v", err)
	}
	if *sleeps != 0 {
		t.Fatalf("production must not wait, slept %d times", *sleeps)
	}
	r1()
	r1() // idempotent
	if _, err := a.admit(context.Background(), Production, "t", need); err != nil {
		t.Fatalf("after one release a production fits again: %v", err)
	}
	r2()
}

func TestAdmit_ScanWaitsThenGivesUp(t *testing.T) {
	t.Parallel()
	a, sleeps := fakeAdmission(gib, 16*gib)
	_, err := a.admit(context.Background(), Scan, "t", RSSCap(Scan, 16*gib))
	if !errors.Is(err, ErrMemoryPressure) {
		t.Fatalf("got %v", err)
	}
	if *sleeps != 4 {
		t.Fatalf("a scan must poll until the wait is over, slept %d times", *sleeps)
	}
}

func TestAdmit_ScanStartsOnceMemoryFrees(t *testing.T) {
	t.Parallel()
	avail := gib
	a := newAdmission(
		func() (int64, bool) { return avail, true },
		func() (int64, bool) { return 16 * gib, true },
	)
	a.sleep = func(context.Context, time.Duration) error { avail = 8 * gib; return nil }
	a.wait = time.Hour
	release, err := a.admit(context.Background(), Scan, "t", RSSCap(Scan, 16*gib))
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestAdmit_ScanWaitHonoursContext(t *testing.T) {
	t.Parallel()
	a := newAdmission(
		func() (int64, bool) { return 0, true },
		func() (int64, bool) { return 16 * gib, true },
	)
	a.wait = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.admit(ctx, Scan, "t", gib); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestAdmit_UnreadableHostAdmits(t *testing.T) {
	t.Parallel()
	a := newAdmission(
		func() (int64, bool) { return 0, false },
		func() (int64, bool) { return 0, false },
	)
	if _, err := a.admit(context.Background(), Production, "t", 4*gib); err != nil {
		t.Fatalf("blind host must admit (the cap still bounds the process): %v", err)
	}
}

func TestAdmit_ProbeBypasses(t *testing.T) {
	t.Parallel()
	a, _ := fakeAdmission(0, 16*gib)
	if _, err := a.admit(context.Background(), Probe, "t", gib); err != nil {
		t.Fatal(err)
	}
}
