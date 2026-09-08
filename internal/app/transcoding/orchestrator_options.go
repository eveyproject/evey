package transcoding

// orchestrator_options.go holds SegmentOrchestrator's configuration type
// (OrchestratorOptions) and its source-resolution seam (moved out of
// segment_orchestrator.go for readability; pure move, no behaviour change).

import (
	"context"
	"time"
)

// OrchestratorOptions configures the orchestrator's lifecycle policy.
type OrchestratorOptions struct {
	// LookAheadSegments is the size of each ffmpeg rafale: when a request arrives
	// for segment X and no live session covers it, a fresh ffmpeg is spawned at
	// X and runs until it has encoded X + LookAheadSegments. Larger = fewer
	// ffmpeg restarts but more wasted work on seeks; default 30 (90 s at 3 s
	// segments) which empirically covers small rewinds + tight ABR switches
	// without paying for segments the user won't watch.
	LookAheadSegments int

	// InactivityTimeout is how long an idle session lingers before the GC kills
	// it. "Idle" = no segment request since LastAccess. Default 60 s, matching
	// hls.js' default fragment retry window.
	InactivityTimeout time.Duration

	// UnreadTimeout is the (much shorter) idle budget of a rafale NO client has
	// ever read a segment from. serveVariant pre-warms segment 0 on every
	// variant-playlist fetch, and no client tells the server where it is about
	// to start, so a resume opens an encoder at 0 that the player never touches.
	// On InactivityTimeout that bet kept a full ABR ladder running next to the
	// real rafale for ~55 s of the exact window the viewer spends buffering. A
	// pre-warm that pays off is read within a round-trip (that is its entire
	// purpose), so this can be tight without ever catching a live viewer.
	// Default 20 s; clamped to InactivityTimeout when set higher.
	UnreadTimeout time.Duration

	// GCInterval is the period of the inactivity sweeper. Default 15 s.
	GCInterval time.Duration

	// SegmentSeconds - keep in sync with ManifestSegmentSeconds and the ffmpeg
	// force_key_frames cadence. Exposed for tests.
	SegmentSeconds float64

	// ThrottleEnabled toggles the pause-when-ahead policy.
	// When ffmpeg has produced ThrottlePauseAhead segments more than the
	// client's high-water request, SIGSTOP the process; SIGCONT when the
	// client catches back up to within ThrottleResumeBelow segments.
	// Saves CPU + thermals + disk write churn on calm linear playback
	// without affecting seek latency (cancel-on-seek tears the session
	// down before throttle gets involved).
	ThrottleEnabled bool

	// ThrottlePauseAhead is the buffer (in segments) above which ffmpeg
	// gets paused. Default 10 = 30s buffer at 3s segments. Field reports cite
	// 30-45s as the empirical sweet spot.
	ThrottlePauseAhead int

	// ThrottleResumeBelow is the buffer (in segments) below which a paused
	// session resumes. Hysteresis vs ThrottlePauseAhead avoids flapping.
	// Default 5 = 15s.
	ThrottleResumeBelow int

	// ThrottleTickInterval is how often the throttle policy is evaluated.
	// Default 1s - coarse enough to be free, tight enough that a seek-out
	// that lands on a paused session resumes within one tick.
	ThrottleTickInterval time.Duration

	// MaxRafalesPerHash bounds the number of concurrent ffmpeg rafales
	// that can be active for a single fingerprint. Each rafale serves a
	// disjoint span (one client per rafale, in practice). Default 3
	// is enough for "two browser tabs + one resume" type traffic on a
	// single film without melting the CPU on a server hosting 10
	// different films. Set to 1 to recover the legacy single-rafale
	// behaviour (kill-on-conflict).
	MaxRafalesPerHash int

	// MaxActiveSessions caps the TOTAL number of concurrent ffmpeg rafales the
	// orchestrator runs across ALL fingerprints, independent of
	// MaxRafalesPerHash (which only bounds rafales for a single media). It is the
	// global CPU backstop: without it an authenticated client can saturate a weak
	// host by requesting many distinct media - or diversifying one media's
	// decision hash (audio/sub/hdr query params) - to spawn N software x264
	// sessions. At the cap, spawnSession returns ErrCapacityExceeded (HTTP 503)
	// instead of starting ffmpeg. Zero = unlimited (no cap), preserving legacy
	// behaviour for tests and callers that don't set it.
	MaxActiveSessions int

	// HWFirstSegmentTimeout is the fast-fail watchdog for hardware sessions: if
	// a HW-encode rafale hasn't written its fMP4 init segment (the first sign the
	// encoder is actually producing output) within this window, it's treated as
	// a stuck/dead GPU pipeline - demoted to software and killed - instead of
	// letting the client wait out the full ~25 s segment timeout. The init
	// (vs a full segment) is the signal because a working-but-slow HW pipeline
	// writes it within ~1-3 s while a TDR-dead one never does, so a tight 5 s
	// default won't false-fail a slow weak-NAS GPU. Software sessions are
	// unaffected; zero disables the watchdog.
	HWFirstSegmentTimeout time.Duration

	// FFmpegProgressLogDir returns the directory this orchestrator's sessions
	// write their ffmpeg `-progress` stream into, or "" to record nothing. Same
	// contract as the software producer's: read per session so an operator's
	// toggle applies to the next encoder, nil = off. The hardware path carries
	// it too on purpose - a diagnostic that only covers software transcodes
	// would go missing on exactly the boxes that transcode fastest.
	FFmpegProgressLogDir func(context.Context) string

	// WaitBudgetDur is how long the segment handler blocks on cache.Wait for an
	// orchestrated segment (returned by WaitBudget()). Default 25 s - a generous
	// HW cold-start window. Configurable so unit tests fail fast instead of summing
	// 25 s waits, mirroring the software producer's WaitBudgetDur.
	WaitBudgetDur time.Duration

	// HWPerVariant routes the HW transcode path through one single-output ffmpeg
	// per ABR rung (forwarded to HLSProcessOpts.PerVariantHW). Off by default;
	// software is always single-process multi-variant.
	HWPerVariant bool

	// SingleVariant, when true, makes the orchestrator spawn one ffmpeg rafale
	// per (fingerprint, variantIdx) pair instead of one multi-variant rafale per
	// fingerprint. Each spawn produces only the requested ABR rung (1 encode),
	// saving the N-1 encodes of rungs the client never watches. Like
	// the on-demand single-variant model mature media servers converge on.
	//
	// FLAG OFF (default): behaviour is byte-identical to the pre-flag multi-variant
	// path - the existing tests prove this invariant.
	// FLAG ON: the per-rung sessions are all keyed under the same DecisionHash;
	// the client ABR-switches freely and each new rung is produced lazily.
	SingleVariant bool
}

func (o *OrchestratorOptions) withDefaults() OrchestratorOptions {
	out := *o
	if out.LookAheadSegments <= 0 {
		out.LookAheadSegments = 30
	}
	if out.InactivityTimeout <= 0 {
		out.InactivityTimeout = 60 * time.Second
	}
	if out.UnreadTimeout <= 0 {
		out.UnreadTimeout = 20 * time.Second
	}
	if out.UnreadTimeout > out.InactivityTimeout {
		// An unread rafale must never outlive a read one, whatever an operator
		// configures - the whole point is that it is the cheaper thing to drop.
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
		// Hysteresis invariant: resume threshold must be strictly below pause
		// threshold or the session will flap every tick.
		out.ThrottleResumeBelow = out.ThrottlePauseAhead / 2
	}
	if out.ThrottleTickInterval <= 0 {
		out.ThrottleTickInterval = time.Second
	}
	if out.MaxRafalesPerHash <= 0 {
		out.MaxRafalesPerHash = 3
	}
	if out.HWFirstSegmentTimeout <= 0 {
		out.HWFirstSegmentTimeout = 5 * time.Second
	}
	if out.WaitBudgetDur <= 0 {
		out.WaitBudgetDur = 25 * time.Second
	}
	return out
}

// SourceResolver bridges the orchestrator (which works with DecisionFingerprint)
// to whatever the rest of the app uses to locate a media file and its streams.
// Kept as a small interface so the orchestrator stays testable without bringing
// the whole library service into its tests.
type SourceResolver interface {
	// ResolveSource returns the absolute file path, media streams (codec/lang/
	// resolution metadata used by the ffmpeg adapter), and a "needs HDR tonemap"
	// flag for the given media. Errors propagate to the handler as 5xx.
	ResolveSource(ctx context.Context, fp DecisionFingerprint) (path string, streams []SourceStream, err error)
}

// SourceResolverFunc adapts a plain function to the SourceResolver interface,
// letting wire.go inline the bridge without declaring a one-method struct.
type SourceResolverFunc func(ctx context.Context, fp DecisionFingerprint) (string, []SourceStream, error)

func (f SourceResolverFunc) ResolveSource(ctx context.Context, fp DecisionFingerprint) (string, []SourceStream, error) {
	return f(ctx, fp)
}

// SourceStream is the minimal stream metadata the orchestrator passes to the
// ffmpeg adapter. Defined here (rather than re-using library.MediaStream
// directly) to avoid pulling the library package into the orchestrator.
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
