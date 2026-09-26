package bgtasks

import (
	"context"
	"log/slog"
	"maps"
	"sync"
	"time"
)

// Coverage is how far a task is through the whole library, in items and in
// media duration (a 3 h film weighs more than a 20 min episode), and once its
// pace is measured, how long the rest should take.
type Coverage struct {
	DoneItems    int64   `json:"doneItems"`
	TotalItems   int64   `json:"totalItems"`
	DoneSeconds  float64 `json:"doneSeconds"`
	TotalSeconds float64 `json:"totalSeconds"`
	// EtaSeconds is the remaining media duration at the pace measured over
	// the last coverageWindow. Absent until that pace spans coverageMinSpan
	// with progress in it, and while nothing moves (paused, budget at 0).
	EtaSeconds *float64 `json:"etaSeconds,omitempty"`
}

// CoverageFunc counts a task's library-wide progress. It is the twin of the
// query the worker takes its work from, so the two cannot disagree.
type CoverageFunc func(ctx context.Context) (Coverage, error)

const (
	coverageWindow  = 30 * time.Minute
	coverageMinSpan = 5 * time.Minute
	coverageTimeout = 20 * time.Second
)

type covSample struct {
	at   time.Time
	done float64
}

type coverageState struct {
	mu      sync.Mutex
	funcs   map[string]CoverageFunc
	latest  map[string]Coverage
	samples map[string][]covSample
}

func newCoverageState() *coverageState {
	return &coverageState{
		funcs:   map[string]CoverageFunc{},
		latest:  map[string]Coverage{},
		samples: map[string][]covSample{},
	}
}

// SetCoverage gives name a library-wide progress count; the task shows up in
// the snapshot from then on, even before its first item.
func (r *Registry) SetCoverage(name string, fn CoverageFunc) {
	r.mu.Lock()
	r.get(name)
	r.mu.Unlock()
	r.cov.mu.Lock()
	r.cov.funcs[name] = fn
	r.cov.mu.Unlock()
}

// SampleCoverage runs every coverage count once and updates the paces.
func (r *Registry) SampleCoverage(ctx context.Context, now time.Time) {
	r.cov.mu.Lock()
	funcs := make(map[string]CoverageFunc, len(r.cov.funcs))
	maps.Copy(funcs, r.cov.funcs)
	r.cov.mu.Unlock()
	for name, fn := range funcs {
		cctx, cancel := context.WithTimeout(ctx, coverageTimeout)
		c, err := fn(cctx)
		cancel()
		if err != nil {
			slog.WarnContext(ctx, "bgtasks: coverage count failed", slog.String("task", name), slog.Any("err", err))
			continue
		}
		r.cov.mu.Lock()
		r.cov.latest[name] = r.cov.record(name, c, now)
		r.cov.mu.Unlock()
	}
}

// record appends the sample and returns c with its ETA. Done going DOWN is a
// reset (an algorithm version bump re-opens items, media were removed): the
// pace before it measured another amount of work. The caller holds mu.
func (s *coverageState) record(name string, c Coverage, now time.Time) Coverage {
	prev := s.samples[name]
	if n := len(prev); n > 0 && prev[n-1].done > c.DoneSeconds {
		prev = nil
	}
	kept := make([]covSample, 0, len(prev)+1)
	for _, p := range prev {
		if now.Sub(p.at) <= coverageWindow {
			kept = append(kept, p)
		}
	}
	kept = append(kept, covSample{at: now, done: c.DoneSeconds})
	s.samples[name] = kept
	c.EtaSeconds = nil
	oldest := kept[0]
	span := now.Sub(oldest.at)
	if span >= coverageMinSpan && c.DoneSeconds > oldest.done && c.TotalSeconds > c.DoneSeconds {
		rate := (c.DoneSeconds - oldest.done) / span.Seconds()
		eta := (c.TotalSeconds - c.DoneSeconds) / rate
		c.EtaSeconds = &eta
	}
	return c
}

func (r *Registry) coverageOf(name string) *Coverage {
	r.cov.mu.Lock()
	defer r.cov.mu.Unlock()
	c, ok := r.cov.latest[name]
	if !ok {
		return nil
	}
	return &c
}

// RunCoverageSampler samples every coverage count now, then every interval,
// until ctx ends. A handful of COUNT queries a minute: cheap on a small host.
func (r *Registry) RunCoverageSampler(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		r.SampleCoverage(ctx, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func SetCoverage(name string, fn CoverageFunc) { std.SetCoverage(name, fn) }
func RunCoverageSampler(ctx context.Context, interval time.Duration) {
	std.RunCoverageSampler(ctx, interval)
}
