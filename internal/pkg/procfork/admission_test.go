package procfork

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
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

// Prod 2026-09: three encoders at their 3.9 GiB caps made the paper sum say
// 1.4 GiB free while the host had 12.6 GiB, and a 1 GiB album-art resize a
// viewer was waiting on was refused. An interactive job is admitted on what
// the host really has; the heavy classes still see the paper sum.
func TestAdmit_InteractiveIsAdmittedOnHostMemoryNotOnPaperCaps(t *testing.T) {
	t.Parallel()
	// 16 GiB budget: reserve 1 GiB, production cap 4 GiB, interactive cap 1 GiB.
	// 12 GiB available holds two productions on paper (8 GiB reserved, 4 GiB
	// "free"); a third production is refused, an interactive job is not.
	a, _ := fakeAdmission(12*gib, 16*gib)
	prod := RSSCap(Production, 16*gib)
	for range 2 {
		if _, err := a.admit(context.Background(), Production, "enc", prod); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.admit(context.Background(), Production, "enc", prod); !errors.Is(err, ErrMemoryPressure) {
		t.Fatalf("a third production must still be refused on the paper sum, got %v", err)
	}
	release, err := a.admit(context.Background(), Interactive, "art", RSSCap(Interactive, 16*gib))
	if err != nil {
		t.Fatalf("an interactive job must be admitted on the host's 12 GiB, got %v", err)
	}
	release()
	// The host itself short: the interactive job is refused like any other.
	tight, _ := fakeAdmission(gib, 16*gib)
	if _, err := tight.admit(context.Background(), Interactive, "art", RSSCap(Interactive, 16*gib)); !errors.Is(err, ErrMemoryPressure) {
		t.Fatalf("with 1 GiB on the host the interactive job must be refused, got %v", err)
	}
}

// MemAvailable lags the allocations of the jobs just started, so a burst of
// interactive jobs read against the same figure must see each other: with
// 4 GiB free, a 1 GiB reserve and 1 GiB caps, three fit and the fourth waits
// for a release - whatever the encoders hold on paper.
func TestAdmit_InteractiveBurstSubtractsInteractiveReservations(t *testing.T) {
	t.Parallel()
	a, _ := fakeAdmission(4*gib, 16*gib)
	art := RSSCap(Interactive, 16*gib)
	var releases []func()
	for i := range 3 {
		release, err := a.admit(context.Background(), Interactive, "art", art)
		if err != nil {
			t.Fatalf("interactive job %d refused: %v", i, err)
		}
		releases = append(releases, release)
	}
	if _, err := a.admit(context.Background(), Interactive, "art", art); !errors.Is(err, ErrMemoryPressure) {
		t.Fatalf("a 4th interactive job on the same 4 GiB must be refused, got %v", err)
	}
	releases[0]()
	releases[0]() // idempotent: a double release must not free twice
	if _, err := a.admit(context.Background(), Interactive, "art", art); err != nil {
		t.Fatalf("a released slot must admit the next interactive job, got %v", err)
	}
	if _, err := a.admit(context.Background(), Interactive, "art", art); !errors.Is(err, ErrMemoryPressure) {
		t.Fatalf("one release frees one slot, not two, got %v", err)
	}
}

// The subtraction is the interactive jobs only: two productions booking 8 GiB
// on paper out of 12 still leave the interactive class the host's 12 GiB,
// eleven 1 GiB jobs above the 1 GiB reserve.
func TestAdmit_InteractiveIgnoresEncoderCapsButNotItsOwn(t *testing.T) {
	t.Parallel()
	a, _ := fakeAdmission(12*gib, 16*gib)
	for range 2 {
		if _, err := a.admit(context.Background(), Production, "enc", RSSCap(Production, 16*gib)); err != nil {
			t.Fatal(err)
		}
	}
	art := RSSCap(Interactive, 16*gib)
	for i := range 11 {
		if _, err := a.admit(context.Background(), Interactive, "art", art); err != nil {
			t.Fatalf("interactive job %d refused with the encoders on paper: %v", i, err)
		}
	}
	if _, err := a.admit(context.Background(), Interactive, "art", art); !errors.Is(err, ErrMemoryPressure) {
		t.Fatalf("the 12th interactive job must be refused, got %v", err)
	}
}

// Concurrent admissions race on the same MemAvailable reading: exactly as
// many get in as fit, under -race.
func TestAdmit_ConcurrentInteractiveBurstAdmitsOnlyWhatFits(t *testing.T) {
	t.Parallel()
	a, _ := fakeAdmission(4*gib, 16*gib)
	art := RSSCap(Interactive, 16*gib)
	const burst = 16
	var wg sync.WaitGroup
	var admitted atomic.Int32
	start := make(chan struct{})
	for range burst {
		wg.Go(func() {
			<-start
			if _, err := a.admit(context.Background(), Interactive, "art", art); err == nil {
				admitted.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()
	if got := admitted.Load(); got != 3 {
		t.Fatalf("%d of %d concurrent interactive jobs admitted on 4 GiB, want 3", got, burst)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.interactiveReserved != 3*art || a.reserved != 3*art {
		t.Fatalf("booked interactive=%d total=%d, want %d each", a.interactiveReserved, a.reserved, 3*art)
	}
}
