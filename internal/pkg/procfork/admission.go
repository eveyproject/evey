package procfork

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// ErrMemoryPressure is returned by Command when the free memory cannot hold
// the process at its cap on top of what is already running. Production and
// interactive callers fail fast; scans wait scanAdmissionWait first.
var ErrMemoryPressure = errors.New("procfork: not enough free memory to start")

const (
	scanAdmissionWait = 10 * time.Minute
	admissionPoll     = 5 * time.Second
)

// admission keeps the sum of the caps of the processes in flight, so that all
// of them reaching their cap at once still leaves reserveBytes free.
type admission struct {
	available func() (int64, bool)
	budget    func() (int64, bool)
	sleep     func(ctx context.Context, d time.Duration) error
	now       func() time.Time
	wait      time.Duration
	poll      time.Duration

	mu       sync.Mutex
	reserved int64
}

func newAdmission(available, budget func() (int64, bool)) *admission {
	return &admission{
		available: available,
		budget:    budget,
		sleep:     sleepCtx,
		now:       time.Now,
		wait:      scanAdmissionWait,
		poll:      admissionPoll,
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// admit reserves need bytes. A host whose free memory is unreadable admits
// everything: the cap on the process still bounds it.
func (a *admission) admit(ctx context.Context, c Class, site string, need int64) (release func(), err error) {
	if c == Probe || need <= 0 {
		return func() {}, nil
	}
	budget, _ := a.budget()
	reserve := reserveBytes(budget)
	deadline := a.now().Add(a.wait)
	waited := false
	for {
		avail, ok, free := a.tryReserve(need, reserve)
		if ok {
			if waited {
				slog.Info("procfork: memory pressure cleared, starting", slog.String("site", site))
			}
			return a.releaseFn(need), nil
		}
		if c != Scan || a.now().After(deadline) {
			slog.Warn("procfork: refused for memory pressure",
				slog.String("site", site), slog.String("class", c.String()),
				slog.Int64("need_mb", need>>20), slog.Int64("free_mb", free>>20),
				slog.Int64("available_mb", avail>>20), slog.Int64("reserve_mb", reserve>>20))
			return nil, ErrMemoryPressure
		}
		if !waited {
			waited = true
			slog.Info("procfork: waiting for free memory",
				slog.String("site", site), slog.Int64("need_mb", need>>20), slog.Int64("free_mb", free>>20))
		}
		if serr := a.sleep(ctx, a.poll); serr != nil {
			return nil, serr
		}
	}
}

// tryReserve returns the raw MemAvailable, whether the reservation was taken,
// and the free memory net of the in-flight caps.
func (a *admission) tryReserve(need, reserve int64) (avail int64, ok bool, free int64) {
	avail, readable := a.available()
	a.mu.Lock()
	defer a.mu.Unlock()
	if !readable {
		a.reserved += need
		return 0, true, 0
	}
	free = avail - a.reserved
	if free < need+reserve {
		return avail, false, free
	}
	a.reserved += need
	return avail, true, free
}

func (a *admission) releaseFn(need int64) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			a.mu.Lock()
			a.reserved -= need
			a.mu.Unlock()
		})
	}
}
