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
	// interactiveReserved is the part of reserved held by Interactive jobs,
	// the one reservation an interactive admission does subtract.
	interactiveReserved int64
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
		avail, ok, free := a.tryReserve(need, reserve, c == Interactive)
		if ok {
			if waited {
				slog.Info("procfork: memory pressure cleared, starting", slog.String("site", site))
			}
			return a.releaseFn(need, c == Interactive), nil
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
//
// againstHost admits on what the host really has free rather than on the sum
// of the in-flight caps. Those caps are ceilings the encoders never reach (a
// production ffmpeg capped at 3.9 GiB sat at 681 MiB, prod 2026-09), and the
// paper sum refused a 1 GiB album-art resize a viewer was waiting on while
// 12.6 GiB were free. A short interactive job cannot push the host over on
// its own; the heavy classes keep the conservative rule. Its cap is still
// added to the sum, so the encoders admitted after it see it. What it does
// subtract is the other interactive jobs in flight: MemAvailable lags their
// allocations, so a burst read against the same figure would all get in.
func (a *admission) tryReserve(need, reserve int64, interactive bool) (avail int64, ok bool, free int64) {
	avail, readable := a.available()
	a.mu.Lock()
	defer a.mu.Unlock()
	if !readable {
		a.take(need, interactive)
		return 0, true, 0
	}
	free = avail - a.reserved
	headroom := free
	if interactive {
		headroom = avail - a.interactiveReserved
	}
	if headroom < need+reserve {
		return avail, false, free
	}
	a.take(need, interactive)
	return avail, true, free
}

// take books need; the caller holds mu. A negative need gives it back.
func (a *admission) take(need int64, interactive bool) {
	a.reserved += need
	if interactive {
		a.interactiveReserved += need
	}
}

func (a *admission) releaseFn(need int64, interactive bool) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			a.mu.Lock()
			a.take(-need, interactive)
			a.mu.Unlock()
		})
	}
}
