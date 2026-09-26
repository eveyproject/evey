package bgtasks

import (
	"context"
	"errors"
	"testing"
	"time"
)

// A counted task at a steady pace: no ETA before coverageMinSpan of history,
// then the remaining duration at the measured pace.
func TestCoverage_EtaFollowsTheMeasuredPace(t *testing.T) {
	r := New()
	done := 0.0
	r.SetCoverage("intro", func(context.Context) (Coverage, error) {
		return Coverage{DoneItems: int64(done / 60), TotalItems: 100, DoneSeconds: done, TotalSeconds: 6000}, nil
	})
	start := time.Unix(1_000_000, 0)
	for m := range 11 {
		done = float64(m) * 60 // one media minute per wall minute
		r.SampleCoverage(context.Background(), start.Add(time.Duration(m)*time.Minute))
		c := r.Snapshot()[0].Coverage
		if c == nil {
			t.Fatal("a counted task must carry its coverage")
		}
		if m < 5 && c.EtaSeconds != nil {
			t.Fatalf("minute %d: an ETA before %v of history", m, coverageMinSpan)
		}
		if m == 10 {
			if c.EtaSeconds == nil {
				t.Fatal("no ETA after 10 minutes of steady progress")
			}
			// 6000 - 600 media seconds left at 1 media second per second.
			if *c.EtaSeconds < 5399 || *c.EtaSeconds > 5401 {
				t.Fatalf("ETA %.0f s, want 5400", *c.EtaSeconds)
			}
		}
	}
}

// The pace is the last coverageWindow: a fast start long ago does not make a
// slow present look fast.
func TestCoverage_PaceForgetsOutsideTheWindow(t *testing.T) {
	r := New()
	done := 0.0
	r.SetCoverage("t", func(context.Context) (Coverage, error) {
		return Coverage{DoneSeconds: done, TotalSeconds: 100_000}, nil
	})
	start := time.Unix(0, 0)
	for m := range 61 {
		now := start.Add(time.Duration(m) * time.Minute)
		if m <= 20 {
			done = float64(m) * 600 // 10 media s per s, early on
		} else {
			done = 12_000 + float64(m-20)*60 // then 1 media s per s
		}
		r.SampleCoverage(context.Background(), now)
	}
	c := r.Snapshot()[0].Coverage
	if c.EtaSeconds == nil {
		t.Fatal("no ETA")
	}
	left := 100_000 - done
	if *c.EtaSeconds < left*0.99 || *c.EtaSeconds > left*1.01 {
		t.Fatalf("ETA %.0f s, want ~%.0f (the last 30 min pace only)", *c.EtaSeconds, left)
	}
}

// Nothing moving (paused, CPU budget at 0, nothing it can do): no ETA, rather
// than an infinite or stale one.
func TestCoverage_NoEtaWithoutProgress(t *testing.T) {
	r := New()
	r.SetCoverage("t", func(context.Context) (Coverage, error) {
		return Coverage{DoneSeconds: 50, TotalSeconds: 100}, nil
	})
	start := time.Unix(0, 0)
	for m := range 21 {
		r.SampleCoverage(context.Background(), start.Add(time.Duration(m)*time.Minute))
	}
	if c := r.Snapshot()[0].Coverage; c.EtaSeconds != nil {
		t.Fatalf("ETA %.0f with no progress", *c.EtaSeconds)
	}
}

// Done going down (an algorithm version bump re-opens items) restarts the
// pace: the old history measured another amount of work.
func TestCoverage_DoneGoingDownResetsThePace(t *testing.T) {
	r := New()
	done := 0.0
	r.SetCoverage("t", func(context.Context) (Coverage, error) {
		return Coverage{DoneSeconds: done, TotalSeconds: 10_000}, nil
	})
	start := time.Unix(0, 0)
	for m := range 11 {
		done = float64(m) * 60
		r.SampleCoverage(context.Background(), start.Add(time.Duration(m)*time.Minute))
	}
	done = 0
	r.SampleCoverage(context.Background(), start.Add(11*time.Minute))
	if c := r.Snapshot()[0].Coverage; c.EtaSeconds != nil {
		t.Fatal("an ETA survived a reset")
	}
	if n := len(r.cov.samples["t"]); n != 1 {
		t.Fatalf("%d samples kept across a reset, want 1", n)
	}
}

// A failing count keeps the last good reading; a task without a count has no
// coverage at all.
func TestCoverage_FailedCountKeepsTheLastReading(t *testing.T) {
	r := New()
	fail := false
	r.SetCoverage("counted", func(context.Context) (Coverage, error) {
		if fail {
			return Coverage{}, errors.New("db down")
		}
		return Coverage{DoneItems: 3, TotalItems: 4}, nil
	})
	r.Register("uncounted")
	r.SampleCoverage(context.Background(), time.Unix(0, 0))
	fail = true
	r.SampleCoverage(context.Background(), time.Unix(60, 0))
	for _, task := range r.Snapshot() {
		switch task.Name {
		case "counted":
			if task.Coverage == nil || task.Coverage.DoneItems != 3 {
				t.Fatalf("counted: %+v", task.Coverage)
			}
		case "uncounted":
			if task.Coverage != nil {
				t.Fatalf("uncounted task got coverage %+v", task.Coverage)
			}
		}
	}
}
