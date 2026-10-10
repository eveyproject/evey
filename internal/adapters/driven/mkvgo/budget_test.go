package mkvgo

import (
	"math"
	"testing"
)

func TestBudgetFor_OneWalkPerCPUWhenMemoryAllows(t *testing.T) {
	// 8 CPUs, 4 GiB heap limit: 1 GiB / 15 MiB = 68 walks fit, so the CPUs win.
	b := BudgetFor(8, 4<<30)
	if b.MaxWalks != 8 {
		t.Fatalf("MaxWalks = %d, want 8", b.MaxWalks)
	}
	if b.TableCacheBytes != tableCacheMax {
		t.Fatalf("TableCacheBytes = %d, want the max %d", b.TableCacheBytes, tableCacheMax)
	}
}

func TestBudgetFor_MemoryHoldsTheWalksDown(t *testing.T) {
	// 16 CPUs on a 256 MiB heap limit (the memlimit floor): 64 MiB / 15 MiB = 4.
	b := BudgetFor(16, 256<<20)
	if b.MaxWalks != 4 {
		t.Fatalf("MaxWalks = %d, want 4", b.MaxWalks)
	}
	if b.TableCacheBytes != tableCacheMin {
		t.Fatalf("TableCacheBytes = %d, want the floor %d", b.TableCacheBytes, tableCacheMin)
	}
}

func TestBudgetFor_NeverBelowOneWalk(t *testing.T) {
	b := BudgetFor(2, 8<<20)
	if b.MaxWalks != 1 {
		t.Fatalf("MaxWalks = %d, want 1", b.MaxWalks)
	}
	if b := BudgetFor(0, math.MaxInt64); b.MaxWalks != 1 {
		t.Fatalf("MaxWalks with 0 CPUs = %d, want 1", b.MaxWalks)
	}
}

func TestBudgetFor_NoHeapLimitMeansCPUsOnly(t *testing.T) {
	b := BudgetFor(6, math.MaxInt64)
	if b.MaxWalks != 6 || b.TableCacheBytes != tableCacheMax {
		t.Fatalf("got %+v, want 6 walks and the default table cache", b)
	}
}

func TestBudgetFor_TableCacheScalesWithTheLimit(t *testing.T) {
	// 1 GiB / 16 = 64 MiB, inside the clamp.
	if b := BudgetFor(4, 1<<30); b.TableCacheBytes != 64<<20 {
		t.Fatalf("TableCacheBytes = %d, want 64 MiB", b.TableCacheBytes)
	}
}
