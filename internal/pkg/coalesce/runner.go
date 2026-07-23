// Package coalesce serializes re-arm work triggered from hot notification
// paths. Born from two audit findings on the video-session capacity re-arm:
// (1) the capability-change callback ran a Postgres read SYNCHRONOUSLY while
// the calibration service held demoteMu+currentMu (the encode-failure path
// serialized on the DB), and (2) two concurrent triggers (capability change +
// server-config change) interleaved read-compute-apply, so a re-arm computed
// from a STALE capability could apply last and win.
package coalesce

import "context"

// Runner runs fn on a single goroutine, one run at a time. Triggers arriving
// while fn runs collapse into ONE follow-up run that starts after the current
// one finishes - so the final state always derives from a read taken AFTER the
// last trigger, and callers' notification paths never block.
type Runner struct {
	pending chan struct{}
}

// Start spawns the runner goroutine, bound to ctx (service lifetime - the
// work is service-scoped re-arming, not request-scoped). fn receives that
// same ctx.
func Start(ctx context.Context, fn func(context.Context)) *Runner {
	r := &Runner{pending: make(chan struct{}, 1)}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-r.pending:
				fn(ctx)
			}
		}
	}()
	return r
}

// Trigger requests a run and never blocks: the capacity-1 channel remembers at
// most one pending request, which is enough - a run started after the last
// trigger sees everything that trigger saw.
func (r *Runner) Trigger() {
	select {
	case r.pending <- struct{}{}:
	default:
	}
}
