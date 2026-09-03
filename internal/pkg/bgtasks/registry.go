// Package bgtasks is a tiny, dependency-free registry that background workers
// report their live status into, so the admin dashboard can show WHAT is
// running (and why the CPU isn't idle) without each worker growing its own
// status plumbing. Mirrors the log/metrics "package-level default + injectable
// type" shape so a worker just calls bgtasks.Begin(...) with no wiring.
//
// Controllable workers additionally let an admin STOP them: Stop pauses the
// worker (every later Begin sees Paused, WaitResumed blocks) and cancels the
// context of the item in flight. A worker opts in with RegisterControllable
// and derives its per-item context from BeginCtx; a worker that only calls
// Begin keeps running as before and is reported as not controllable.
package bgtasks

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

type State string

const (
	StateIdle    State = "idle"
	StateRunning State = "running"
)

// Task is a point-in-time snapshot of one worker's status.
type Task struct {
	Name         string    `json:"name"`
	State        State     `json:"state"`
	Detail       string    `json:"detail,omitempty"`      // what it's working on right now (e.g. a media id)
	Processed    int64     `json:"processed"`             // items completed this process lifetime
	LastActiveAt time.Time `json:"lastActiveAt,omitzero"` // when it last did work
	StartedAt    time.Time `json:"startedAt,omitzero"`    // when the item in flight began
	// Controllable reports that Stop/Resume act on this worker.
	Controllable bool `json:"controllable"`
	// Paused is set by Stop and cleared by Resume: the worker skips every item
	// until an admin resumes it (or the server restarts).
	Paused bool `json:"paused"`
}

type entry struct {
	state        State
	detail       string
	processed    int64
	lastActive   time.Time
	startedAt    time.Time
	controllable bool
	paused       bool
	// running counts the items in flight (a worker may fan out); cancels holds
	// the context cancel of each one, so Stop reaches all of them.
	running int
	nextID  uint64
	cancels map[uint64]context.CancelFunc
}

// Registry tracks per-worker status. Safe for concurrent use.
type Registry struct {
	mu sync.Mutex
	m  map[string]*entry
	// pollInterval paces WaitResumed; shortened by tests.
	pollInterval time.Duration
}

func New() *Registry { return &Registry{m: make(map[string]*entry), pollInterval: time.Second} }

func (r *Registry) get(name string) *entry {
	e := r.m[name]
	if e == nil {
		e = &entry{state: StateIdle, cancels: make(map[uint64]context.CancelFunc)}
		r.m[name] = e
	}
	return e
}

// Register pre-seeds an idle entry so a worker shows in the dashboard even
// before it processes its first item. Idempotent; call once at startup.
func (r *Registry) Register(name string) {
	r.mu.Lock()
	r.get(name)
	r.mu.Unlock()
}

// RegisterControllable is Register for a worker that honours Stop/Resume:
// it gates its loop on WaitResumed (or Paused) and runs each item under the
// context BeginCtx hands out.
func (r *Registry) RegisterControllable(name string) {
	r.mu.Lock()
	r.get(name).controllable = true
	r.mu.Unlock()
}

// Begin marks name as running on detail and returns a done func that flips it
// back to idle and bumps the processed counter. Idiomatic one-liner per work
// item: defer reg.Begin("keyframe_backfill", mediaID)().
func (r *Registry) Begin(name, detail string) func() {
	_, done := r.BeginCtx(context.Background(), name, detail)
	return done
}

// BeginCtx is Begin with a per-item context: derived from ctx, tagged with the
// task name (TaskFromContext, which is how forked processes are attributed to
// the task), and cancelled by Stop. On a paused worker it comes back already
// cancelled, so a worker that skipped the WaitResumed gate still cannot do
// work while an admin believes it stopped. done cancels it too, so the
// derived context never outlives the item.
func (r *Registry) BeginCtx(ctx context.Context, name, detail string) (context.Context, func()) {
	ctx = WithTask(ctx, name)
	itemCtx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	e := r.get(name)
	e.state = StateRunning
	e.detail = detail
	e.lastActive = time.Now()
	if e.running == 0 {
		e.startedAt = e.lastActive
	}
	e.running++
	e.nextID++
	id := e.nextID
	e.cancels[id] = cancel
	paused := e.paused
	r.mu.Unlock()
	if paused {
		cancel()
	}
	var once sync.Once
	return itemCtx, func() {
		once.Do(func() {
			cancel()
			r.mu.Lock()
			delete(e.cancels, id)
			e.running--
			e.processed++
			e.lastActive = time.Now()
			if e.running == 0 {
				e.state = StateIdle
				e.detail = ""
				e.startedAt = time.Time{}
			}
			r.mu.Unlock()
		})
	}
}

// Stop pauses a controllable worker and cancels its item in flight. Returns
// false for an unknown or non-controllable worker, in which case nothing
// changes. Idempotent.
func (r *Registry) Stop(name string) bool {
	r.mu.Lock()
	e := r.m[name]
	if e == nil || !e.controllable {
		r.mu.Unlock()
		return false
	}
	e.paused = true
	cancels := make([]context.CancelFunc, 0, len(e.cancels))
	for _, c := range e.cancels {
		cancels = append(cancels, c)
	}
	r.mu.Unlock()
	for _, c := range cancels {
		c()
	}
	return true
}

// Resume lifts a Stop. Returns false for an unknown or non-controllable worker.
func (r *Registry) Resume(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.m[name]
	if e == nil || !e.controllable {
		return false
	}
	e.paused = false
	return true
}

// Paused reports whether an admin stopped name. Unknown names are not paused.
func (r *Registry) Paused(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.m[name]
	return e != nil && e.paused
}

// ErrStopped is what WaitResumed returns when the worker's own stop channel
// closed while it was waiting: shut down, do not run the item.
var ErrStopped = errors.New("bgtasks: worker stopped")

// WaitResumed blocks while name is paused. It returns nil once the worker may
// run, ctx.Err() when the caller is shutting down, and ErrStopped when stop
// (a worker's own stop channel, nil when it has none) closes - the one gate a
// controllable worker places at the top of its per-item loop.
func (r *Registry) WaitResumed(ctx context.Context, name string, stop <-chan struct{}) error {
	for r.Paused(name) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-stop:
			return ErrStopped
		case <-time.After(r.pollInterval):
		}
	}
	return nil
}

// Snapshot returns every known task's status, name-sorted for a stable display.
func (r *Registry) Snapshot() []Task {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Task, 0, len(r.m))
	for name, e := range r.m {
		out = append(out, Task{
			Name:         name,
			State:        e.state,
			Detail:       e.detail,
			Processed:    e.processed,
			LastActiveAt: e.lastActive,
			StartedAt:    e.startedAt,
			Controllable: e.controllable,
			Paused:       e.paused,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// --- task attribution through context ---

type taskKey struct{}

// WithTask tags ctx with the task doing the work, so anything forked under it
// (see procfork) can be charged to that task.
func WithTask(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, taskKey{}, name)
}

// TaskFromContext returns the task tag set by WithTask, or "".
func TaskFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	name, _ := ctx.Value(taskKey{}).(string)
	return name
}

// --- package-level default (so workers need zero wiring) ---

var std = New()

// Register / Begin / Snapshot / ... delegate to the default registry.
func Register(name string)             { std.Register(name) }
func RegisterControllable(name string) { std.RegisterControllable(name) }
func Begin(name, detail string) func() { return std.Begin(name, detail) }
func BeginCtx(ctx context.Context, name, detail string) (context.Context, func()) {
	return std.BeginCtx(ctx, name, detail)
}
func Stop(name string) bool   { return std.Stop(name) }
func Resume(name string) bool { return std.Resume(name) }
func Paused(name string) bool { return std.Paused(name) }
func WaitResumed(ctx context.Context, name string, stop <-chan struct{}) error {
	return std.WaitResumed(ctx, name, stop)
}
func Snapshot() []Task { return std.Snapshot() }
