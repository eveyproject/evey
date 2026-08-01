package ratelimit

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// The visitor cap (2026-08-01) closed an unbounded leak but opened a contention
// amplifier in its place: at the cap with all-fresh entries - exactly the
// rotating-address case the cap targets - the retention pass deletes nothing,
// one coldest key goes, the table returns to the cap, and the NEXT request
// scans all 50k again under l.mu, the lock every legitimate call takes. One
// packet for the attacker, 50k iterations for the server.

func fillToCap(t *testing.T, l *Limiter) {
	t.Helper()
	for i := 0; len(l.visitors) < maxVisitors; i++ {
		l.Allow(fmt.Sprintf("10.%d.%d.%d", i>>16&0xff, i>>8&0xff, i&0xff))
		if i > maxVisitors*4 {
			t.Fatalf("could not fill the table (%d entries)", len(l.visitors))
		}
	}
}

func TestEviction_CostDoesNotScaleWithTableSize(t *testing.T) {
	l := New(1000)
	defer l.Stop()

	fillToCap(t, l)

	l.mu.Lock()
	evictScanned = 0
	l.mu.Unlock()

	// An unseen address: the table is at the cap, so this call evicts.
	l.Allow("203.0.113.7")

	l.mu.Lock()
	scanned, size := evictScanned, len(l.visitors)
	l.mu.Unlock()

	// <= 0, not == 0: a counter that runs backwards would never reach the sample
	// bound, so the loop would scan the whole table while the ceiling assertion
	// below still passed on a negative number.
	if scanned <= 0 {
		t.Fatalf("eviction reported %d entries examined: either none happened, or the "+
			"counter that bounds the loop is not counting up", scanned)
	}
	if scanned > evictSample {
		t.Fatalf("one eviction examined %d entries out of %d (allowed: %d). Cost tracks "+
			"table size, so every packet from a rotating attacker buys a full scan "+
			"under the lock legitimate connections also take.", scanned, size, evictSample)
	}
	if size > maxVisitors {
		t.Fatalf("table grew past its cap: %d > %d", size, maxVisitors)
	}
}

// Evicting a caller in debt hands its allowance back: the next request creates a
// fresh entry with a full bucket, undoing the throttling just applied. The
// "an active caller is never evicted" guarantee only covers the one that
// RETRIES (each attempt refreshes lastCheck); the one refused once and then
// quiet becomes the coldest entry as soon as the table fills.
//
// Under a hard cap something must go, so the contract is "last", not "never":
// while entries that would gain nothing from eviction remain (non-empty bucket,
// they would be allowed anyway), those go first.
//
// Beware the test that only looks convincing: targeting ONE indebted caller
// among 50k proves nothing, since a 64-entry sample almost never contains it -
// it survives by luck, not by policy. So measure the policy on a population.
func TestEviction_PrefersCallersThatOweNothing(t *testing.T) {
	l := NewWithBurst(5, 5)
	defer l.Stop()

	const evictions = 300
	now := time.Now()

	l.mu.Lock()
	// Every entry is recent (none stale), so bucket state is the only thing that
	// can decide between two candidates.
	for i := range maxVisitors {
		tokens := 3.0 // solvent: evicting it hands back nothing
		if i%2 == 0 {
			tokens = 0 // in debt: evicting it undoes the throttling
		}
		l.visitors[fmt.Sprintf("k%d", i)] = &entry{
			tokens:    tokens,
			lastCheck: now.Add(-time.Duration(i%60) * time.Second),
		}
	}
	l.mu.Unlock()

	for i := range evictions {
		l.Allow(fmt.Sprintf("newcomer-%d", i))
	}

	l.mu.Lock()
	starvedLeft := 0
	for k, e := range l.visitors {
		if strings.HasPrefix(k, "k") && e.tokens < 1 {
			starvedLeft++
		}
	}
	l.mu.Unlock()

	const starvedAtStart = maxVisitors / 2
	starvedDropped := starvedAtStart - starvedLeft

	// With no preference victims split evenly: ~150 of 300 would hit an indebted
	// caller. With it, near zero. The threshold leaves room for sampling noise
	// without letting a missing policy through.
	if starvedDropped > evictions/10 {
		t.Fatalf("%d of %d evictions hit a caller in debt while the table held just as "+
			"many solvent entries. Each of those hands a full bucket back to someone "+
			"just refused: that is the throttling being undone, not memory saved.",
			starvedDropped, evictions)
	}
}

// The last-resort branch: when every sampled entry is in debt, one still has to
// go, or the table would grow past its cap and the memory bound the whole
// mechanism exists for would be gone. Mutation (2026-08-01) showed nothing
// exercised it - the fallback candidate could be disabled entirely and the
// suite stayed green, because a solvent entry was always available.
func TestEviction_StillFreesASlotWhenEveryoneIsInDebt(t *testing.T) {
	l := NewWithBurst(5, 5)
	defer l.Stop()

	now := time.Now()
	l.mu.Lock()
	for i := range maxVisitors {
		l.visitors[fmt.Sprintf("k%d", i)] = &entry{
			tokens:    0, // everyone refused, nobody solvent
			lastCheck: now.Add(-time.Duration(i%60) * time.Second),
		}
	}
	l.mu.Unlock()

	l.Allow("203.0.113.42")

	l.mu.Lock()
	size := len(l.visitors)
	_, admitted := l.visitors["203.0.113.42"]
	l.mu.Unlock()

	if size > maxVisitors {
		t.Fatalf("table grew to %d past the %d cap: with no solvent candidate the "+
			"eviction gave up, so the memory bound no longer holds", size, maxVisitors)
	}
	if !admitted {
		t.Fatal("the newcomer got no bucket")
	}
}
