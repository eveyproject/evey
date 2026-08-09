package ratelimit

import (
	"log/slog"
	"sync"
	"time"
)

// FailureGate counts FAILURES across every caller at once, with no key, and
// shuts a route for a cooldown once they pass a threshold.
//
// It answers the hole the per-key Limiter leaves open and documents at
// maxVisitors: a limiter keyed on the source address bounds one attacker, not a
// thousand rotating ones, and guessing a secret is exactly the shape of attack
// that rotates. A global counter is the standard answer because the resource
// being protected - the space of valid codes - is itself global.
//
// Only failures count, so a route with real traffic never trips: honest callers
// succeed. It is deliberately blunt (all callers are refused during a cooldown,
// including honest ones), which is why it belongs on routes where the cost of a
// few minutes' refusal is far below the cost of an enumeration succeeding.
type FailureGate struct {
	mu           sync.Mutex
	failures     int
	windowStart  time.Time
	blockedUntil time.Time

	threshold int
	window    time.Duration
	cooldown  time.Duration
	name      string
	now       func() time.Time
}

func NewFailureGate(name string, threshold int, window, cooldown time.Duration) *FailureGate {
	return NewFailureGateForTest(name, threshold, window, cooldown, time.Now)
}

// NewFailureGateForTest injects the clock so the window and the cooldown can be
// exercised without sleeping.
func NewFailureGateForTest(name string, threshold int, window, cooldown time.Duration, now func() time.Time) *FailureGate {
	return &FailureGate{
		threshold: threshold,
		window:    window,
		cooldown:  cooldown,
		name:      name,
		now:       now,
	}
}

// Allow reports whether the route is currently open.
func (g *FailureGate) Allow() bool {
	if g == nil {
		return true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return !g.now().Before(g.blockedUntil)
}

// Fail records one failed attempt and closes the gate when the threshold is
// reached inside the window. Call it ONLY on a genuine failure - a wrong code,
// not a malformed request - or honest client bugs would close the route.
func (g *FailureGate) Fail() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	if now.Sub(g.windowStart) > g.window {
		g.windowStart = now
		g.failures = 0
	}
	g.failures++
	if g.failures < g.threshold {
		return
	}
	g.blockedUntil = now.Add(g.cooldown)
	g.failures = 0
	g.windowStart = now
	// Refusing without a trace is how the 503 of 2026-07-31 stayed invisible for
	// days: an operator must be able to see this from the logs alone.
	slog.Warn("ratelimit: failure gate closed",
		slog.String("gate", g.name),
		slog.Int("threshold", g.threshold),
		slog.Duration("window", g.window),
		slog.Duration("cooldown", g.cooldown))
}

// Succeed clears the running count: a success proves the traffic is not an
// enumeration, so scattered wrong codes over a long window never accumulate
// into a refusal.
func (g *FailureGate) Succeed() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.failures = 0
	g.windowStart = g.now()
}
