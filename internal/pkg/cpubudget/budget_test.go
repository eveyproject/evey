package cpubudget

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSlots_ShareOfCapacityNeverUnderOne(t *testing.T) {
	t.Parallel()
	cases := []struct {
		cores  float64
		pct    int
		expect int
	}{
		{8, 25, 2}, {8, 50, 4}, {8, 80, 6},
		{4, 25, 1}, {4, 50, 2}, {4, 80, 3},
		{6, 25, 1}, {2, 25, 1}, {1, 80, 1},
		{1.5, 50, 1}, {16, 25, 4}, {0.5, 25, 1},
	}
	for _, c := range cases {
		b := New(c.cores)
		b.SetPolicy(Policy{TaskPercent: map[string]int{"x": c.pct}})
		if got := b.Slots("x"); got != c.expect {
			t.Errorf("%v cores at %d%%: got %d slots, want %d", c.cores, c.pct, got, c.expect)
		}
	}
}

func TestPolicy_NormalizedClampsAndDefaults(t *testing.T) {
	t.Parallel()
	p := Policy{GlobalPercent: 90, TaskPercent: map[string]int{"a": 95, "b": 0, "": 50, "c": 40}}.Normalized()
	if p.GlobalPercent != BoostGlobalPercent {
		t.Fatalf("global must clamp to %d, got %d", BoostGlobalPercent, p.GlobalPercent)
	}
	if p.TaskPercent["a"] != MaxTaskPercent || p.TaskPercent["c"] != 40 {
		t.Fatalf("task shares: %v", p.TaskPercent)
	}
	if _, ok := p.TaskPercent["b"]; ok {
		t.Fatal("a zero task share must be dropped")
	}
	if _, ok := p.TaskPercent[""]; ok {
		t.Fatal("an unnamed task must be dropped")
	}
	if d := (Policy{}).Normalized(); d.GlobalPercent != DefaultGlobalPercent || d.TaskPercent != nil {
		t.Fatalf("empty policy must default to %d%%, got %+v", DefaultGlobalPercent, d)
	}
}

func TestCeiling_IsTheHighestShareNotTheSum(t *testing.T) {
	t.Parallel()
	b := New(10)
	if got := b.Ceiling(); got != 2 {
		t.Fatalf("default ceiling on 10 cores must be 2 (25%%), got %d", got)
	}
	b.SetPolicy(Policy{GlobalPercent: 25, TaskPercent: map[string]int{"trickplay": 80, "intro": 80}})
	if got := b.Ceiling(); got != 8 {
		t.Fatalf("two tasks at 80%% must cap at 8 of 10 cores, got %d", got)
	}
}

// acquire runs Acquire with a short deadline and reports whether it got in.
func acquire(t *testing.T, b *Budget, task string, n int) (func(), bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, release, err := b.Acquire(ctx, task, n)
	if err != nil {
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("unexpected error: %v", err)
		}
		return nil, false
	}
	return release, true
}

func TestAcquire_TasksWithoutOverrideShareTheGlobalSlots(t *testing.T) {
	t.Parallel()
	b := New(8) // 25 % = 2 slots for everyone together
	r1, ok := acquire(t, b, "scan", 1)
	if !ok {
		t.Fatal("first slot must be granted")
	}
	r2, ok := acquire(t, b, "complexity", 1)
	if !ok {
		t.Fatal("second slot must be granted")
	}
	if _, granted := acquire(t, b, "trickplay", 1); granted {
		t.Fatal("a third task must wait: the global share is 2 slots")
	}
	r1()
	r3, granted := acquire(t, b, "trickplay", 1)
	if !granted {
		t.Fatal("a released slot must be granted to the waiter")
	}
	r2()
	r3()
	if b.Snapshot().Held != 0 {
		t.Fatalf("everything released, held = %d", b.Snapshot().Held)
	}
}

func TestAcquire_OverrideHasItsOwnShareUnderTheCeiling(t *testing.T) {
	t.Parallel()
	b := New(8)
	b.SetPolicy(Policy{GlobalPercent: 25, TaskPercent: map[string]int{"trickplay": 50}})
	// trickplay: 4 slots of its own; the rest: 2 together; ceiling 4.
	rt, ok := acquire(t, b, "trickplay", 3)
	if !ok {
		t.Fatal("trickplay must get 3 of its 4 slots")
	}
	rs, ok := acquire(t, b, "scan", 1)
	if !ok {
		t.Fatal("the scan must get 1 slot: 3 + 1 = ceiling")
	}
	if _, granted := acquire(t, b, "trickplay", 1); granted {
		t.Fatal("trickplay's 4th slot must wait: the ceiling (4) is reached")
	}
	if _, granted := acquire(t, b, "complexity", 1); granted {
		t.Fatal("another default task must wait at the ceiling too")
	}
	rs()
	rt2, granted := acquire(t, b, "trickplay", 1)
	if !granted {
		t.Fatal("trickplay's 4th slot fits once the scan released")
	}
	if _, granted := acquire(t, b, "trickplay", 1); granted {
		t.Fatal("a 5th trickplay slot exceeds its own share")
	}
	rt()
	rt2()
}

func TestAcquire_TwoHeavyTasksNeverStack(t *testing.T) {
	t.Parallel()
	b := New(10)
	b.SetPolicy(Policy{TaskPercent: map[string]int{"a": 80, "b": 80}})
	ra, ok := acquire(t, b, "a", 8)
	if !ok {
		t.Fatal("a alone may take the whole ceiling")
	}
	if _, granted := acquire(t, b, "b", 1); granted {
		t.Fatal("b must wait: 8 + 1 exceeds the 8-core ceiling")
	}
	ra()
	rb, granted := acquire(t, b, "b", 8)
	if !granted {
		t.Fatal("b gets the ceiling once a released")
	}
	rb()
}

func TestAcquire_ClampsToTheTaskShare(t *testing.T) {
	t.Parallel()
	b := New(4) // global 25 % = 1 slot
	_, release, err := b.Acquire(context.Background(), "scan", 16)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if h := b.Snapshot().Held; h != 1 {
		t.Fatalf("a request above the share is clamped to it, held = %d", h)
	}
}

func TestAcquire_ContextAlreadyHoldingIsReentrant(t *testing.T) {
	t.Parallel()
	b := New(4) // 1 slot
	ctx, release, err := b.Acquire(context.Background(), "scan", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if !Held(ctx) {
		t.Fatal("the returned context must carry the slot")
	}
	// A fork under the holder's context must not queue behind its owner.
	inner, innerRelease, err := b.Acquire(ctx, "scan", 1)
	if err != nil {
		t.Fatalf("re-entrant acquire must not block: %v", err)
	}
	innerRelease()
	if !Held(inner) || b.Snapshot().Held != 1 {
		t.Fatalf("re-entry must neither take nor give back a slot, held = %d", b.Snapshot().Held)
	}
	if Held(context.Background()) {
		t.Fatal("a fresh context holds nothing")
	}
}

func TestAcquire_WaiterWakesOnPolicyChange(t *testing.T) {
	t.Parallel()
	b := New(8) // 2 slots
	r1, _ := acquire(t, b, "scan", 2)
	defer r1()
	got := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, release, err := b.Acquire(ctx, "intro", 1)
		if err == nil {
			release()
		}
		got <- err
	}()
	time.Sleep(20 * time.Millisecond)
	b.SetPolicy(Policy{GlobalPercent: 50}) // 4 slots: room for the waiter
	select {
	case err := <-got:
		if err != nil {
			t.Fatalf("the waiter must be admitted after the boost: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("the waiter was not woken by the policy change")
	}
}

func TestRelease_UsesTheGroupingTakenWith(t *testing.T) {
	t.Parallel()
	b := New(8)
	release, ok := acquire(t, b, "trickplay", 1) // taken under the global share
	if !ok {
		t.Fatal("must be granted")
	}
	b.SetPolicy(Policy{TaskPercent: map[string]int{"trickplay": 80}}) // now an override
	release()
	snap := b.Snapshot()
	if snap.Held != 0 || b.group != 0 {
		t.Fatalf("release must undo the group count it was taken with: held=%d group=%d", snap.Held, b.group)
	}
}

func TestSnapshot_ListsOverridesAndHolders(t *testing.T) {
	t.Parallel()
	b := New(8)
	b.SetPolicy(Policy{GlobalPercent: 50, TaskPercent: map[string]int{"trickplay": 80}})
	release, _ := acquire(t, b, "scan", 1)
	defer release()
	snap := b.Snapshot()
	if snap.GlobalSlots != 4 || snap.Ceiling != 6 || snap.Held != 1 || snap.CapacityCores != 8 {
		t.Fatalf("snapshot header: %+v", snap)
	}
	if len(snap.Tasks) != 2 || snap.Tasks[0].Task != "scan" || snap.Tasks[1].Task != "trickplay" {
		t.Fatalf("tasks must be name-sorted holders and overrides: %+v", snap.Tasks)
	}
	if snap.Tasks[0].Override || snap.Tasks[0].Held != 1 || snap.Tasks[0].Slots != 4 {
		t.Fatalf("scan follows the global share: %+v", snap.Tasks[0])
	}
	if !snap.Tasks[1].Override || snap.Tasks[1].Slots != 6 || snap.Tasks[1].Held != 0 {
		t.Fatalf("trickplay has its own: %+v", snap.Tasks[1])
	}
}

func TestNew_UnusableCapacityIsOneCore(t *testing.T) {
	t.Parallel()
	for _, c := range []float64{0, -3} {
		if got := New(c).Capacity(); got != 1 {
			t.Errorf("capacity %v must become 1, got %v", c, got)
		}
	}
}
