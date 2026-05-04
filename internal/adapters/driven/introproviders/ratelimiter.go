package introproviders

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// headerRateLimiter reads X-RateLimit-Remaining / X-RateLimit-Reset from
// TIDB response headers and proactively pauses before the next request
// when remaining falls below lowMark. This avoids 429s on bulk scans by
// honouring the server's own pacing signal instead of waiting for a hard
// rejection.
//
// Thread-safe: all state is guarded by mu.
type headerRateLimiter struct {
	mu        sync.Mutex
	nextOK    time.Time // sleep until this moment if it is in the future
	remaining int       // last seen X-RateLimit-Remaining value
	lowMark   int       // pause threshold (default 3)
	nowFunc   func() time.Time
	sleepFunc func(ctx context.Context, d time.Duration) error
}

func newHeaderRateLimiter() *headerRateLimiter {
	rl := &headerRateLimiter{
		remaining: -1, // -1 = never seen a response yet
		lowMark:   3,
		nowFunc:   time.Now,
	}
	rl.sleepFunc = func(ctx context.Context, d time.Duration) error {
		select {
		case <-time.After(d):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return rl
}

// Wait blocks until it is safe to make the next TIDB request. Returns
// ctx.Err() if the context is cancelled while waiting.
func (r *headerRateLimiter) Wait(ctx context.Context) error {
	r.mu.Lock()
	wait := r.nextOK.Sub(r.nowFunc())
	r.mu.Unlock()

	if wait <= 0 {
		return nil
	}
	return r.sleepFunc(ctx, wait)
}

// Update ingests the rate-limit headers from a TIDB response. When
// Remaining drops below lowMark it sets nextOK so subsequent Wait calls
// pause until the window resets.
func (r *headerRateLimiter) Update(headers http.Header) {
	remaining := parseIntHeader(headers, "X-RateLimit-Remaining")
	resetSecs := parseIntHeader(headers, "X-RateLimit-Reset")

	r.mu.Lock()
	defer r.mu.Unlock()

	if remaining >= 0 {
		r.remaining = remaining
	}
	if r.remaining >= 0 && r.remaining < r.lowMark && resetSecs > 0 {
		candidate := r.nowFunc().Add(time.Duration(resetSecs) * time.Second)
		if candidate.After(r.nextOK) {
			r.nextOK = candidate
		}
	}
}

func parseIntHeader(h http.Header, name string) int {
	v := h.Get(name)
	if v == "" {
		return -1
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return -1
	}
	return n
}
