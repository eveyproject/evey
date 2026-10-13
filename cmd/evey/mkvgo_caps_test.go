package main

import (
	"io"
	"log/slog"
	"testing"

	mkvgoadapter "github.com/eveyproject/evey/internal/adapters/driven/mkvgo"
)

// Unset, empty or unusable leaves both defaults alone; a positive integer
// replaces them.
func TestMkvgoCapsFromEnv(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	def := mkvgoadapter.ProcessBudget{MaxWalks: 7, TableCacheBytes: 1}
	defBuilds := mkvgoadapter.NewOnDemandPackager().MaxBuilds()

	for _, tc := range []struct {
		value             string
		walks, planBuilds int
	}{
		{"", 7, defBuilds},
		{"0", 7, defBuilds},
		{"-2", 7, defBuilds},
		{"four", 7, defBuilds},
		{"2", 2, 2},
	} {
		t.Setenv(envMkvgoMaxWalks, tc.value)
		t.Setenv(envMkvgoMaxPlanBuilds, tc.value)
		if got := mkvgoBudgetFromEnv(log, def); got.MaxWalks != tc.walks || got.TableCacheBytes != 1 {
			t.Errorf("%q: budget = %+v, want MaxWalks %d", tc.value, got, tc.walks)
		}
		p := mkvgoadapter.NewOnDemandPackager()
		applyMkvgoMaxPlanBuilds(log, p)
		if got := p.MaxBuilds(); got != tc.planBuilds {
			t.Errorf("%q: plan builds = %d, want %d", tc.value, got, tc.planBuilds)
		}
		p.Close()
	}
}
