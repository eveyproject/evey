package main

import (
	"log/slog"
	"os"
	"strconv"

	mkvgoadapter "github.com/eveyproject/evey/internal/adapters/driven/mkvgo"
)

// The two caps of the on-demand packager, overridable for a measured run:
// the plans built at once (16 by default) and the source windows read at once
// (one per GOMAXPROCS by default, fewer when the heap limit cannot hold them).
// Unset, empty or not a positive integer keeps the default.
const (
	envMkvgoMaxPlanBuilds = "EVEY_MKVGO_MAX_PLAN_BUILDS"
	envMkvgoMaxWalks      = "EVEY_MKVGO_MAX_WALKS"
)

// positiveIntEnv reads name as a positive integer; ok is false when unset or
// unusable, the latter logged so a typo does not pass for the default.
func positiveIntEnv(log *slog.Logger, name string) (int, bool) {
	v := os.Getenv(name)
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		log.Warn("not a positive integer, keeping the default", slog.String("env", name), slog.String("value", v))
		return 0, false
	}
	return n, true
}

// mkvgoBudgetFromEnv is budget with EVEY_MKVGO_MAX_WALKS applied.
func mkvgoBudgetFromEnv(log *slog.Logger, budget mkvgoadapter.ProcessBudget) mkvgoadapter.ProcessBudget {
	if n, ok := positiveIntEnv(log, envMkvgoMaxWalks); ok {
		budget.MaxWalks = n
	}
	return budget
}

// applyMkvgoMaxPlanBuilds sets EVEY_MKVGO_MAX_PLAN_BUILDS on the packager.
func applyMkvgoMaxPlanBuilds(log *slog.Logger, p *mkvgoadapter.OnDemandPackager) {
	if n, ok := positiveIntEnv(log, envMkvgoMaxPlanBuilds); ok {
		p.WithMaxBuilds(n)
	}
}
