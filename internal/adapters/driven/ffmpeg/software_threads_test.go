package ffmpeg

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/eveyproject/evey/internal/pkg/sysinfo"
)

func TestParseSoftwareThreads(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"", 0},    // unset → no cap
		{"0", 0},   // <1 → no cap
		{"-3", 0},  // negative → no cap
		{"abc", 0}, // non-numeric → no cap
		{"2.5", 0}, // non-integer → no cap
		{"65", 0},  // above ceiling → no cap
		{"1", 1},   // boundary
		{"2", 2},   // typical weak-NAS value
		{"64", 64}, // boundary ceiling
		{" 4 ", 0}, // surrounding space is not trimmed (Atoi fails) → no cap
	}
	for _, c := range cases {
		if got := parseSoftwareThreads(c.raw); got != c.want {
			t.Errorf("parseSoftwareThreads(%q) = %d, want %d", c.raw, got, c.want)
		}
	}
}

func TestSoftwareThreadsArgsN(t *testing.T) {
	if got := softwareThreadsArgsN(0, ""); got != nil {
		t.Errorf("n=0 must emit no flag, got %v", got)
	}
	if got := softwareThreadsArgsN(-1, ":v:0"); got != nil {
		t.Errorf("n<0 must emit no flag, got %v", got)
	}
	single := softwareThreadsArgsN(2, "")
	if strings.Join(single, " ") != "-threads 2" {
		t.Errorf("single-output cap = %v, want [-threads 2]", single)
	}
	perRung := softwareThreadsArgsN(3, ":v:1")
	if strings.Join(perRung, " ") != "-threads:v:1 3" {
		t.Errorf("per-rung cap = %v, want [-threads:v:1 3]", perRung)
	}
}

// By default (env unset → no operator override) the software arg builders must
// still bound the encoder to the process's actual CPU budget (the cgroup quota
// when set, else NumCPU) - a software encode must never grab every HOST core
// under a container quota, which starves every other job (scanning, metadata,
// serving the UI) on a weak NAS. Hardware paths are untouched: -threads has a
// different meaning there and this test only exercises the software builders.
func TestBuildFFmpegArgs_ThreadsCapDefaultsToCPUBudget(t *testing.T) {
	if softwareTranscodeThreads() != 0 {
		t.Skip("EVEY_TRANSCODING_SOFTWARE_THREADS is set in this environment; default-CPU-budget assertion is moot")
	}
	budget := sysinfo.CPUBudget()
	base := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		VideoDecision: "transcode",
		AudioDecision: "transcode",
		VideoCodec:    "h264",
	}

	single := base
	single.Profiles = transcoding.DefaultHLSProfiles[:1]
	wantSingle := fmt.Sprintf("-threads %d", budget)
	if joined := strings.Join(BuildFFmpegArgs(single), " "); !strings.Contains(joined, wantSingle) {
		t.Errorf("single transcode must default -threads to the CPU budget (%s):\n%s", wantSingle, joined)
	}

	multi := base
	multi.Profiles = transcoding.DefaultHLSProfiles[:3]
	wantMulti := fmt.Sprintf("-threads:v:0 %d", budget)
	if joined := strings.Join(BuildFFmpegArgs(multi), " "); !strings.Contains(joined, wantMulti) {
		t.Errorf("multi-variant transcode must default -threads:v:i to the CPU budget (%s):\n%s", wantMulti, joined)
	}
}

// Positive end-to-end wiring check, env-gated: run with
// EVEY_TRANSCODING_SOFTWARE_THREADS=N to assert the cap actually reaches the
// software arg builders (single → -threads N, multi → -threads:v:i N).
func TestBuildFFmpegArgs_ThreadsCapWhenSet(t *testing.T) {
	n := softwareTranscodeThreads()
	if n <= 0 {
		t.Skip("set EVEY_TRANSCODING_SOFTWARE_THREADS to exercise the positive path")
	}
	base := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		VideoDecision: "transcode",
		AudioDecision: "transcode",
		VideoCodec:    "h264",
	}

	single := base
	single.Profiles = transcoding.DefaultHLSProfiles[:1]
	if joined := strings.Join(BuildFFmpegArgs(single), " "); !strings.Contains(joined, "-threads ") {
		t.Errorf("single transcode must emit -threads when capped:\n%s", joined)
	}

	multi := base
	multi.Profiles = transcoding.DefaultHLSProfiles[:3]
	if joined := strings.Join(BuildFFmpegArgs(multi), " "); !strings.Contains(joined, "-threads:v:0 ") {
		t.Errorf("multi-variant transcode must emit -threads:v:i when capped:\n%s", joined)
	}
}
