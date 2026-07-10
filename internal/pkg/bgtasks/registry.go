// Package bgtasks is a tiny, dependency-free registry that background workers
// report their live status into, so the admin dashboard can show WHAT is
// running (and why the CPU isn't idle) without each worker growing its own
// status plumbing. Observability only — never on a hot path, never affects
// behaviour. Mirrors the log/metrics "package-level default + injectable type"
// shape so a worker just calls bgtasks.Begin(...) with no wiring.
package bgtasks

import (
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
}

type entry struct {
	state      State
	detail     string
	processed  int64
	lastActive time.Time
}

// Registry tracks per-worker status. Safe for concurrent use.
type Registry struct {
	mu sync.Mutex
	m  map[string]*entry
}

func New() *Registry { return &Registry{m: make(map[string]*entry)} }

// Register pre-seeds an idle entry so a worker shows in the dashboard even
// before it processes its first item. Idempotent; call once at startup.
func (r *Registry) Register(name string) {
	r.mu.Lock()
	if r.m[name] == nil {
		r.m[name] = &entry{state: StateIdle}
	}
	r.mu.Unlock()
}

// Begin marks name as running on detail and returns a done func that flips it
// back to idle and bumps the processed counter. Idiomatic one-liner per work
// item: defer reg.Begin("keyframe_backfill", mediaID)().
func (r *Registry) Begin(name, detail string) func() {
	r.mu.Lock()
	e := r.m[name]
	if e == nil {
		e = &entry{}
		r.m[name] = e
	}
	e.state = StateRunning
	e.detail = detail
	e.lastActive = time.Now()
	r.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			e.state = StateIdle
			e.detail = ""
			e.processed++
			e.lastActive = time.Now()
			r.mu.Unlock()
		})
	}
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
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// --- package-level default (so workers need zero wiring) ---

var std = New()

// Register / Begin / Snapshot delegate to the default registry.
func Register(name string)             { std.Register(name) }
func Begin(name, detail string) func() { return std.Begin(name, detail) }
func Snapshot() []Task                 { return std.Snapshot() }
