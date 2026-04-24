package library

import "time"

// Ticker abstracts time.Ticker so tests can drive ticks deterministically
// instead of waiting on wall-clock duration.
type Ticker interface {
	// C returns the channel on which ticks are delivered.
	C() <-chan time.Time
	// Stop releases the ticker's resources.
	Stop()
}

// Clock abstracts the wall clock. Production code uses RealClock(); tests
// inject a fake to control Now() and NewTicker() deterministically.
type Clock interface {
	Now() time.Time
	NewTicker(d time.Duration) Ticker
}

// RealClock returns a Clock backed by the standard library time package.
func RealClock() Clock {
	return realClock{}
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) NewTicker(d time.Duration) Ticker {
	return &realTicker{t: time.NewTicker(d)}
}

type realTicker struct {
	t *time.Ticker
}

func (r *realTicker) C() <-chan time.Time { return r.t.C }
func (r *realTicker) Stop()               { r.t.Stop() }
