package transcoding

// orchestrator_options.go holds SegmentOrchestrator's configuration type and
// its source-resolution seam.

import (
	"context"
	"time"
)

// OrchestratorOptions configures the orchestrator's lifecycle policy.
type OrchestratorOptions struct {
	// LookAheadSegments is the size of each ffmpeg rafale: a fresh ffmpeg
	// spawned at segment X runs until X + LookAheadSegments. Default 30 (90 s),
	// which covers small rewinds and ABR switches without encoding unwatched content.
	LookAheadSegments int

	// SeekReachWindowSegs bounds how far past a rafale's production point a
	// request may wait on it instead of spawning at its own position (a request
	// at 23 used to wait on a rafale at 0, prod 2026-09-09). Default 8.
	SeekReachWindowSegs int

	// InactivityTimeout is how long an idle session (no segment request)
	// lingers before the GC kills it. Default 60 s, hls.js' fragment retry window.
	InactivityTimeout time.Duration

	// UnreadTimeout is the shorter idle budget of a rafale NO client ever read
	// from: the segment-0 pre-warm of a resume the player never touches. A
	// pre-warm that pays off is read within a round-trip. Default 20 s,
	// clamped to InactivityTimeout.
	UnreadTimeout time.Duration

	// GCInterval is the period of the inactivity sweeper. Default 15 s.
	GCInterval time.Duration

	// SegmentSeconds - keep in sync with ManifestSegmentSeconds and the ffmpeg
	// force_key_frames cadence. Exposed for tests.
	SegmentSeconds float64

	// ThrottleEnabled toggles the pause-when-ahead policy: SIGSTOP ffmpeg when
	// it is ThrottlePauseAhead segments past the client's high-water request,
	// SIGCONT when the client is back within ThrottleResumeBelow.
	ThrottleEnabled bool

	// ThrottlePauseAhead: default 10 (30 s at 3 s segments).
	ThrottlePauseAhead int

	// ThrottleResumeBelow: default 5; hysteresis vs ThrottlePauseAhead avoids flapping.
	ThrottleResumeBelow int

	// ThrottleTickInterval is how often the throttle policy is evaluated. Default 1 s.
	ThrottleTickInterval time.Duration

	// MaxRafalesPerHash bounds the concurrent rafales for one fingerprint (each
	// serves a disjoint span). Default 3; 1 = the legacy kill-on-conflict.
	MaxRafalesPerHash int

	// MaxActiveSessions caps the TOTAL concurrent rafales across every
	// fingerprint: the global CPU backstop against a client requesting many
	// distinct media or diversifying one media's hash. At the cap spawnSession
	// returns ErrCapacityExceeded (503). Zero = unlimited.
	MaxActiveSessions int

	// HWFirstSegmentTimeout is the fast-fail watchdog of a hardware session: no
	// fMP4 init within this window = a stuck GPU pipeline, demoted to software
	// and killed. A working-but-slow pipeline writes the init within ~1-3 s.
	// Default 10 s; zero disables.
	HWFirstSegmentTimeout time.Duration

	// FFmpegProgressLogDir returns the directory sessions write their ffmpeg
	// `-progress` stream into, or "" to record nothing (read per session, nil = off).
	FFmpegProgressLogDir func(context.Context) string

	// WaitBudgetDur is how long the segment handler blocks on cache.Wait for an
	// orchestrated segment. Default 25 s; tests set it low.
	WaitBudgetDur time.Duration

	// HWPerVariant routes the HW transcode through one single-output ffmpeg per
	// ABR rung (HLSProcessOpts.PerVariantHW). Off by default.
	HWPerVariant bool

	// SingleVariant spawns one rafale per (fingerprint, variantIdx) producing
	// only the requested rung, instead of one multi-variant rafale per
	// fingerprint. Off = byte-identical to the historical multi-variant path.
	SingleVariant bool
}

func (o *OrchestratorOptions) withDefaults() OrchestratorOptions {
	out := *o
	if out.LookAheadSegments <= 0 {
		out.LookAheadSegments = 30
	}
	if out.SeekReachWindowSegs <= 0 {
		out.SeekReachWindowSegs = 8
	}
	if out.InactivityTimeout <= 0 {
		out.InactivityTimeout = 60 * time.Second
	}
	if out.UnreadTimeout <= 0 {
		out.UnreadTimeout = 20 * time.Second
	}
	if out.UnreadTimeout > out.InactivityTimeout {
		// An unread rafale must never outlive a read one.
		out.UnreadTimeout = out.InactivityTimeout
	}
	if out.GCInterval <= 0 {
		out.GCInterval = 15 * time.Second
	}
	if out.SegmentSeconds <= 0 {
		out.SegmentSeconds = ManifestSegmentSeconds
	}
	if out.ThrottlePauseAhead <= 0 {
		out.ThrottlePauseAhead = 10
	}
	if out.ThrottleResumeBelow <= 0 {
		out.ThrottleResumeBelow = 5
	}
	if out.ThrottleResumeBelow >= out.ThrottlePauseAhead {
		// Hysteresis invariant, or the session flaps every tick.
		out.ThrottleResumeBelow = out.ThrottlePauseAhead / 2
	}
	if out.ThrottleTickInterval <= 0 {
		out.ThrottleTickInterval = time.Second
	}
	if out.MaxRafalesPerHash <= 0 {
		out.MaxRafalesPerHash = 3
	}
	if out.HWFirstSegmentTimeout <= 0 {
		out.HWFirstSegmentTimeout = 10 * time.Second
	}
	if out.WaitBudgetDur <= 0 {
		out.WaitBudgetDur = 25 * time.Second
	}
	return out
}

// SourceResolver locates a media file and its streams for a fingerprint; a
// small interface so the orchestrator stays testable without the library service.
type SourceResolver interface {
	ResolveSource(ctx context.Context, fp DecisionFingerprint) (path string, streams []SourceStream, err error)
}

// SourceResolverFunc adapts a plain function to the SourceResolver interface.
type SourceResolverFunc func(ctx context.Context, fp DecisionFingerprint) (string, []SourceStream, error)

func (f SourceResolverFunc) ResolveSource(ctx context.Context, fp DecisionFingerprint) (string, []SourceStream, error) {
	return f(ctx, fp)
}

// SourceStream is the minimal stream metadata the orchestrator passes to the
// ffmpeg adapter (not library.MediaStream, to keep that package out of here).
type SourceStream struct {
	Index         int
	Type          string // "video", "audio", "subtitle"
	Codec         string
	Language      string
	IsDefault     bool
	IsExternal    bool
	ExternalPath  string
	Width, Height int
	FrameRate     float64
	PixelFormat   string
}
