// Package cpubudget bounds the CPU the background tasks may take together. It
// hands out slots (one slot = one core of the host's capacity) under one
// policy the admin sets live: a global share every background task draws
// from (25 % by default, 50 % when the admin allows the boost), and, for the
// heavy tasks, an optional share of their own (up to 80 %) that replaces the
// global one for that task. The shares never stack: the sum of every slot
// held at once stays under the highest share in the policy, so two tasks at
// 80 % still make 80 %, not 160 %.
//
// The library scan used to fan out over every core the moment a file
// appeared, and each analysis worker sized itself on its own; this is the one
// place the number now comes from. It is dependency-free like bgtasks so any
// worker can draw on it.
package cpubudget

import (
	"context"
	"maps"
	"math"
	"sort"
	"sync"
)

const (
	// DefaultGlobalPercent is the share every background task shares when the
	// admin has decided nothing.
	DefaultGlobalPercent = 25
	// BoostGlobalPercent is the highest share the global setting accepts.
	BoostGlobalPercent = 50
	// MaxTaskPercent is the highest share one task may be granted on its own.
	MaxTaskPercent = 80
)

// Policy is what the admin decided. TaskPercent maps a task name (its bgtasks
// name) to the share that replaces the global one for that task; a task that
// is not listed follows GlobalPercent.
type Policy struct {
	GlobalPercent int
	TaskPercent   map[string]int
}

// Normalized clamps the policy into the accepted ranges: the global share in
// [1, BoostGlobalPercent] (DefaultGlobalPercent when unset), a task share in
// [1, MaxTaskPercent] (dropped when unset).
func (p Policy) Normalized() Policy {
	out := Policy{GlobalPercent: p.GlobalPercent}
	if out.GlobalPercent <= 0 {
		out.GlobalPercent = DefaultGlobalPercent
	}
	out.GlobalPercent = min(out.GlobalPercent, BoostGlobalPercent)
	for task, pct := range p.TaskPercent {
		if task == "" || pct <= 0 {
			continue
		}
		if out.TaskPercent == nil {
			out.TaskPercent = make(map[string]int, len(p.TaskPercent))
		}
		out.TaskPercent[task] = min(pct, MaxTaskPercent)
	}
	return out
}

// Budget is the live allocator. Safe for concurrent use.
type Budget struct {
	capacity float64

	mu     sync.Mutex
	policy Policy
	// held counts the slots each task holds; group is the sum over the tasks
	// that follow the global share; total is the sum over everything.
	held  map[string]int
	group int
	total int
	// changed is closed and replaced whenever a slot frees or the policy
	// moves, so every waiter re-evaluates.
	changed chan struct{}
}

// New builds a budget over capacityCores (sysinfo.CPUCapacity: the cgroup
// quota when one is set, else the host's cores). Anything under one core is
// treated as one: a task always gets at least one slot.
func New(capacityCores float64) *Budget {
	if capacityCores < 1 || math.IsNaN(capacityCores) || math.IsInf(capacityCores, 0) {
		capacityCores = 1
	}
	return &Budget{
		capacity: capacityCores,
		policy:   Policy{}.Normalized(),
		held:     make(map[string]int),
		changed:  make(chan struct{}),
	}
}

// Capacity is the cores the budget was built over.
func (b *Budget) Capacity() float64 { return b.capacity }

// SetPolicy replaces the policy at once. Slots already held are not taken
// back - the processes holding them finish - but nothing new starts above the
// new shares, and waiters see the new room immediately.
func (b *Budget) SetPolicy(p Policy) {
	b.mu.Lock()
	b.policy = p.Normalized()
	b.wakeLocked()
	b.mu.Unlock()
}

// Policy returns a copy of the policy in force.
func (b *Budget) Policy() Policy {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.policy.copy()
}

func (p Policy) copy() Policy {
	out := Policy{GlobalPercent: p.GlobalPercent}
	if len(p.TaskPercent) > 0 {
		out.TaskPercent = make(map[string]int, len(p.TaskPercent))
		maps.Copy(out.TaskPercent, p.TaskPercent)
	}
	return out
}

// slotsFor turns a share into whole cores, never under one: 25 % of 4 cores
// is 1, 25 % of 6 is 1 (1.5 rounded down), 80 % of 8 is 6.
func (b *Budget) slotsFor(percent int) int {
	return max(1, int(math.Floor(b.capacity*float64(percent)/100+1e-9)))
}

// Percent is the share that governs task: its own when it has one, else the
// global share.
func (b *Budget) Percent(task string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	pct, _ := b.percentLocked(task)
	return pct
}

func (b *Budget) percentLocked(task string) (percent int, override bool) {
	if pct, ok := b.policy.TaskPercent[task]; ok {
		return pct, true
	}
	return b.policy.GlobalPercent, false
}

// Slots is how many cores task may hold at once: what a worker sizes its
// parallelism (or a decoder its threads) on.
func (b *Budget) Slots(task string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	pct, _ := b.percentLocked(task)
	return b.slotsFor(pct)
}

// Ceiling is the most cores every background task may hold together: the
// highest share in the policy. Task shares replace the global one, they
// never add to it.
func (b *Budget) Ceiling() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ceilingLocked()
}

func (b *Budget) ceilingLocked() int {
	pct := b.policy.GlobalPercent
	for _, v := range b.policy.TaskPercent {
		pct = max(pct, v)
	}
	return b.slotsFor(pct)
}

type heldKey struct{}

// Held reports whether ctx already carries a slot from Acquire. A worker that
// holds a slot for a whole item forks under that context; the fork must not
// queue for a second slot behind the one its owner holds, or a one-slot task
// would wait on itself forever.
func Held(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	held, _ := ctx.Value(heldKey{}).(bool)
	return held
}

// Acquire blocks until task may hold n more slots (n is clamped to the task's
// share, and to one at least), then returns a context marked as holding them
// (see Held) and the release. It returns ctx.Err() when the context ends
// first. A context that already holds returns at once with a no-op release.
func (b *Budget) Acquire(ctx context.Context, task string, n int) (context.Context, func(), error) {
	if Held(ctx) {
		return ctx, func() {}, nil
	}
	n = max(n, 1)
	for {
		b.mu.Lock()
		pct, override := b.percentLocked(task)
		limit := b.slotsFor(pct)
		n = min(n, limit)
		fits := b.total+n <= b.ceilingLocked()
		if override {
			fits = fits && b.held[task]+n <= limit
		} else {
			fits = fits && b.group+n <= limit
		}
		if fits {
			b.held[task] += n
			b.total += n
			if !override {
				b.group += n
			}
			b.mu.Unlock()
			return context.WithValue(ctx, heldKey{}, true), b.releaseFn(task, n, override), nil
		}
		wait := b.changed
		b.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx, func() {}, ctx.Err()
		case <-wait:
		}
	}
}

// releaseFn gives the slots back under the grouping they were taken with: a
// policy change in between must not leave the group count skewed.
func (b *Budget) releaseFn(task string, n int, override bool) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			b.held[task] -= n
			if b.held[task] <= 0 {
				delete(b.held, task)
			}
			b.total -= n
			if !override {
				b.group -= n
			}
			b.wakeLocked()
			b.mu.Unlock()
		})
	}
}

func (b *Budget) wakeLocked() {
	close(b.changed)
	b.changed = make(chan struct{})
}

// TaskUsage is one task's share and what it holds right now.
type TaskUsage struct {
	Task string `json:"task"`
	// Percent is the share governing the task; Override says it is its own.
	Percent  int  `json:"percent"`
	Override bool `json:"override"`
	Slots    int  `json:"slots"`
	Held     int  `json:"held"`
}

// Snapshot is the budget as the dashboard shows it.
type Snapshot struct {
	CapacityCores float64     `json:"capacityCores"`
	GlobalPercent int         `json:"globalPercent"`
	GlobalSlots   int         `json:"globalSlots"`
	Ceiling       int         `json:"ceiling"`
	Held          int         `json:"held"`
	Tasks         []TaskUsage `json:"tasks"`
}

// Snapshot lists every task that has a share of its own or holds a slot,
// name-sorted.
func (b *Budget) Snapshot() Snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	names := make(map[string]struct{}, len(b.held)+len(b.policy.TaskPercent))
	for t := range b.held {
		names[t] = struct{}{}
	}
	for t := range b.policy.TaskPercent {
		names[t] = struct{}{}
	}
	snap := Snapshot{
		CapacityCores: b.capacity,
		GlobalPercent: b.policy.GlobalPercent,
		GlobalSlots:   b.slotsFor(b.policy.GlobalPercent),
		Ceiling:       b.ceilingLocked(),
		Held:          b.total,
		Tasks:         make([]TaskUsage, 0, len(names)),
	}
	for t := range names {
		pct, override := b.percentLocked(t)
		snap.Tasks = append(snap.Tasks, TaskUsage{
			Task: t, Percent: pct, Override: override, Slots: b.slotsFor(pct), Held: b.held[t],
		})
	}
	sort.Slice(snap.Tasks, func(i, j int) bool { return snap.Tasks[i].Task < snap.Tasks[j].Task })
	return snap
}
