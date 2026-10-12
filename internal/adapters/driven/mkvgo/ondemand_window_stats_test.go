package mkvgo

import (
	"testing"

	"github.com/gravity-zero/mkvgo/mp4"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

// statPlan is a plan whose window counters are set by the test.
type statPlan struct {
	stubPlan
	st mp4.HLSPlanStats
}

func (s *statPlan) Stats() mp4.HLSPlanStats { return s.st }

// A plan the cache lets go keeps its counts in the totals: two snapshots of a
// run never differ by a negative (measured 2026-10-12: streamed_segments -377
// with the cache at its 256-plan cap).
func TestWindowStats_APlanLeavingTheCacheKeepsItsCounts(t *testing.T) {
	p := NewOnDemandPackager()
	t.Cleanup(p.Close)
	a := &statPlan{st: mp4.HLSPlanStats{StreamedSegments: 7, WindowBuilds: 3}}
	b := &statPlan{st: mp4.HLSPlanStats{StreamedSegments: 5}}
	p.plans.Set("a", apptranscoding.OnDemandPlan(a), p.ttl)
	p.plans.Set("b", apptranscoding.OnDemandPlan(b), p.ttl)
	before := p.WindowStats()
	if before["streamed_segments"] != 12 || before["plans"] != 2 {
		t.Fatalf("before = %v", before)
	}

	p.plans.Delete("a")
	after := p.WindowStats()
	if after["streamed_segments"] != 12 || after["window_builds"] != 3 {
		t.Fatalf("a plan let go took its counts with it: %v", after)
	}
	if after["plans"] != 1 || after["retired_plans"] != 1 {
		t.Fatalf("plans=%d retired=%d, want 1 and 1", after["plans"], after["retired_plans"])
	}
}
