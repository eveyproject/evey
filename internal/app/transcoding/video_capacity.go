package transcoding

import (
	"context"
	"log/slog"
	"math"
	"strconv"
	"strings"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// MaxVideoSessionsConfigKey is the server_config key an admin uses to REDUCE
// (never raise) the auto-derived video-session capacity (admin panel only -
// never an env var). Empty/absent/"auto" means
// "use the auto capacity"; a positive integer is clamped to at most the auto
// capacity by ResolveVideoSessionLimit.
const MaxVideoSessionsConfigKey = "transcoding.max_video_sessions"

// VideoCapacityEvidence is the human-readable "why" behind
// AutoVideoSessionCapacity's answer - the admin panel surfaces it verbatim so
// an operator can see the bench rung the number was derived from instead of
// trusting a bare integer.
type VideoCapacityEvidence struct {
	// Height is the resolution rung the capacity was derived from (0 when
	// Derived is false).
	Height int
	// FPS is that rung's measured encode throughput.
	FPS float64
	// RequiredFPS is the realtime threshold the rung had to clear.
	RequiredFPS float64
	// Ratio is FPS / RequiredFPS - "how many realtimes" one encoder of this
	// rung sustains, the basis for the session count.
	Ratio float64
	// CPUBudget is the host's usable core count (sysinfo.CPUBudget), the
	// hard ceiling capacity is clamped to.
	CPUBudget int
	// Derived is false when no usable calibration evidence existed and
	// capacity fell back to the conservative default of 1.
	Derived bool
}

// AutoVideoSessionCapacity derives the number of concurrent VIDEO transcode
// sessions (i.e. sessions that actually ENCODE video - never a video-copy/
// remux rung) a host can sustain, from the boot benchmark's software rungs.
//
// It picks the tallest PASSING "software" ladder rung, takes the ratio of
// its measured fps over the realtime fps it had to clear, and floors that
// ratio to an integer session count: the same reasoning an operator would
// use by hand ("this box does 2.4x realtime at the top resolution it
// passed, so about 2 full-speed encodes fit before either drops below
// realtime"). The result is bounded to [1, cpuBudget] - never zero (a
// hard-zero cap would wedge every viewer) and never more sessions than the
// host has cores for (a software encode is ~1 full core).
//
// Fail-safe: no samples, no passing "software" rung, or a non-positive
// RequiredFPS all fall back to capacity=1 with Derived=false - the most
// conservative answer available when the evidence doesn't support anything
// better. Hardware ("hw") and HDR ("software-hdr") rungs are ignored - this
// is a bound on SOFTWARE video-encode concurrency only.
func AutoVideoSessionCapacity(samples []transcoding.BenchRungSample, cpuBudget int) (int, VideoCapacityEvidence) {
	cpuBudget = max(cpuBudget, 1)

	var best *transcoding.BenchRungSample
	for i := range samples {
		s := &samples[i]
		if s.Ladder != "software" || !s.Passing {
			continue
		}
		if best == nil || s.Height > best.Height {
			best = s
		}
	}

	if best == nil || best.RequiredFPS <= 0 {
		return 1, VideoCapacityEvidence{CPUBudget: cpuBudget, Derived: false}
	}

	ratio := best.FPS / best.RequiredFPS
	capacity := max(int(math.Floor(ratio)), 1)
	capacity = min(capacity, cpuBudget)

	return capacity, VideoCapacityEvidence{
		Height:      best.Height,
		FPS:         best.FPS,
		RequiredFPS: best.RequiredFPS,
		Ratio:       ratio,
		CPUBudget:   cpuBudget,
		Derived:     true,
	}
}

// ResolveVideoSessionLimit reads MaxVideoSessionsConfigKey from server_config
// and combines it with the auto-derived capacity: the admin can only REDUCE
// the ceiling, never exceed it. An empty value, an absent row, a nil repo, the
// literal "auto", or a value that fails to parse as a positive integer all
// resolve to auto unchanged (a parse failure logs a Warn so a typo doesn't
// silently do nothing). A parsed value is clamped to [1, auto] - min(n, auto).
func ResolveVideoSessionLimit(ctx context.Context, cfgRepo shared.ServerConfigRepository, auto int) int {
	if auto < 1 {
		auto = 1
	}
	if cfgRepo == nil {
		return auto
	}
	entry, err := cfgRepo.Get(ctx, MaxVideoSessionsConfigKey)
	if err != nil || entry == nil {
		return auto
	}
	val := strings.TrimSpace(entry.Value)
	if val == "" || strings.EqualFold(val, "auto") {
		return auto
	}
	n, err := strconv.Atoi(val)
	if err != nil || n <= 0 {
		slog.Warn("transcoding: invalid transcoding.max_video_sessions, falling back to auto",
			slog.String("value", entry.Value))
		return auto
	}
	if n > auto {
		return auto
	}
	return n
}
