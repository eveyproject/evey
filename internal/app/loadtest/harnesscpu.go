package loadtest

import (
	"bytes"
	"context"
	"fmt"
	"runtime/pprof"
	"slices"
	"time"

	pprofprofile "github.com/google/pprof/profile"
)

// harnesscpu.go answers a question the cgroup cannot: of the CPU this server
// burned, how much was SERVING the films, and how much was PRETENDING to watch
// them?
//
// The simulated viewers are goroutines inside the server process. They open the
// connections, drain 60-plus MB/s of loopback traffic and throw every byte away
// - and the kernel bills all of it to the same cgroup as the packager. Reading
// cpu.stat and calling the result "what a viewer costs" therefore charges the
// server for its own test harness.
//
// It is not a rounding error. A run measured 25% CPU for 21 viewers - 1.2% each.
// An operator sizing a NAS on that figure buys several times the machine he
// needs, and a run that stops on CPU saturation would be stopping on the load
// generator, not on the load.
//
// So the harness is measured rather than assumed. Every viewer goroutine runs
// under a pprof label; a CPU profile taken during the measurement window is then
// split by that label. Server CPU is what is left - which is the only figure that
// means anything to someone buying hardware.

// harnessLabel tags every goroutine that belongs to the load generator rather
// than to the server. Child goroutines inherit it, so a viewer's parallel audio
// fetch is tagged too, without a second call.
const (
	harnessLabelKey = "evey_role"
	harnessLabelVal = "loadtest_harness"

	// harnessProfileWindow is how long the CPU profile runs. Long enough for the
	// sampler (100 Hz) to collect thousands of samples, short enough that its own
	// overhead never shapes the run it is measuring.
	harnessProfileWindow = 10 * time.Second
	// harnessProfileMinViewers is the audience the run waits for before
	// profiling: the split is meaningless on an idle server, where the only CPU
	// being burned is the sampler's own.
	harnessProfileMinViewers = 4
)

// withHarnessLabel runs fn on a goroutine tagged as the load generator's. The
// tag is what lets the CPU profile tell the client's cost from the server's.
func withHarnessLabel(ctx context.Context, fn func(context.Context)) {
	pprof.Do(ctx, pprof.Labels(harnessLabelKey, harnessLabelVal), fn)
}

// cpuSplit is how the CPU burned during a profiling window divides between the
// server doing its job and the harness pretending to be an audience.
type cpuSplit struct {
	// HarnessFraction is the share of all CPU samples that carried the harness
	// label: 0.0 means the load generator is free, 1.0 means the run measured
	// nothing but itself.
	HarnessFraction float64
	// Measured is false when no profile could be taken or it held no samples (a
	// window too short, a machine too idle). The report then says nothing rather
	// than dividing by a guess.
	Measured bool
}

// serverShare is the fraction of measured CPU that is genuinely the server's.
func (s cpuSplit) serverShare() float64 {
	if !s.Measured {
		return 1 // unmeasured: attribute everything to the server, the pessimistic reading
	}
	return 1 - s.HarnessFraction
}

// profileCPUSplit runs a CPU profile for d and reports how it divided between
// harness and server.
//
// Only ONE profile may run in a process at a time, and the server may well be
// exposing /debug/pprof: a refusal to start is expected, not exceptional, and
// costs the run nothing but this one figure.
func profileCPUSplit(ctx context.Context, d time.Duration) (cpuSplit, error) {
	var buf bytes.Buffer
	if err := pprof.StartCPUProfile(&buf); err != nil {
		return cpuSplit{}, fmt.Errorf("loadtest: start cpu profile: %w", err)
	}

	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
	pprof.StopCPUProfile()

	prof, err := pprofprofile.Parse(&buf)
	if err != nil {
		return cpuSplit{}, fmt.Errorf("loadtest: parse cpu profile: %w", err)
	}
	return splitByLabel(prof), nil
}

// splitByLabel adds up the CPU-time of every sample and works out how much of it
// carried the harness label.
//
// The value index matters: a Go CPU profile carries two per sample - the sample
// COUNT and the CPU NANOSECONDS. Summing counts would weight every sample the
// same regardless of how long it represents; the profile says which column is
// which, so it is asked rather than guessed.
func splitByLabel(prof *pprofprofile.Profile) cpuSplit {
	idx := -1
	for i, st := range prof.SampleType {
		if st.Type == "cpu" && st.Unit == "nanoseconds" {
			idx = i
			break
		}
	}
	if idx < 0 || len(prof.Sample) == 0 {
		return cpuSplit{}
	}

	var total, harness float64
	for _, s := range prof.Sample {
		if idx >= len(s.Value) {
			continue
		}
		v := float64(s.Value[idx])
		if v <= 0 {
			continue
		}
		total += v
		if slices.Contains(s.Label[harnessLabelKey], harnessLabelVal) {
			harness += v
		}
	}
	if total <= 0 {
		return cpuSplit{}
	}
	return cpuSplit{HarnessFraction: harness / total, Measured: true}
}
