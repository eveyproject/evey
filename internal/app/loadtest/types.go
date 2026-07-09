// Package loadtest simulates real HLS viewers against Evey's own HTTP stack so
// an admin can measure how many concurrent streams the running server (on its
// actual hardware) can sustain, without touching a third-party tool or writing
// to disk. It is deliberately read-only from the server's point of view: every
// request a simulated viewer makes is indistinguishable from a real player
// hitting the same endpoints with a real bearer token.
package loadtest

import (
	"context"
	"errors"
	"time"
)

// ErrAlreadyRunning is returned by Start when a load test is already in
// progress; only one run is allowed at a time.
var ErrAlreadyRunning = errors.New("loadtest: already running")

// ErrStreamsActive is returned by Start when real viewers are currently
// streaming. Load testing on top of real traffic would both skew the
// measurement and steal capacity from real users, so it is refused outright.
var ErrStreamsActive = errors.New("loadtest: real streams are active")

// MediaCandidate is one piece of media the runner may point simulated
// viewers at.
type MediaCandidate struct {
	ID          string
	DurationSec int
}

// MediaLister supplies the pool of media the load test can pick viewers'
// streams from. The parent wires this to its media query service.
type MediaLister interface {
	ListLoadTestCandidates(ctx context.Context, limit int) ([]MediaCandidate, error)
}

// Scenario selects how a simulated viewer negotiates playback.
const (
	// ScenarioTranscode forces every viewer onto the transcode path
	// (vd=transcode&ad=transcode), the worst case for CPU/GPU capacity.
	ScenarioTranscode = "transcode"
	// ScenarioNatural lets each viewer negotiate playback the way a real
	// client would (no forced decision), and never shares a media item
	// across two simulated viewers so direct-play/remux cache hits can't
	// flatter the numbers.
	ScenarioNatural = "natural"
)

// Config configures one load test run.
type Config struct {
	// BaseURL is the server's own loopback origin (e.g. http://127.0.0.1:8080),
	// supplied by the parent. Viewers hit real HTTP endpoints on it.
	BaseURL string
	// Token is the bearer token used by every simulated viewer. It is a real
	// credential exercising the real auth path, not a bypass.
	Token string
	// Scenario is ScenarioTranscode or ScenarioNatural.
	Scenario string
	// MaxViewers caps how far the ramp will go. Defaults to 12: a weak NAS
	// is the target hardware, so the default stays conservative and an
	// admin who wants more has to ask for it explicitly.
	MaxViewers int
	// RampEvery is both the cadence at which a new viewer joins and the
	// minimum observation window given to each tier before advancing.
	RampEvery time.Duration
	// SegmentSLOMs is the segment-fetch latency budget. Zero means auto:
	// the detected #EXT-X-TARGETDURATION of the stream, in milliseconds
	// (a segment that takes longer to arrive than its own playback
	// duration means a real player would stall).
	SegmentSLOMs int
	// MaxDuration is a hard wall on the whole run, regardless of ramp
	// progress. Defaults to 4 minutes.
	MaxDuration time.Duration
}

// Defaults applied when the corresponding Config field is left at its zero
// value.
const (
	DefaultMaxViewers  = 12
	DefaultRampEvery   = 8 * time.Second
	DefaultMaxDuration = 4 * time.Minute
)

func (c Config) withDefaults() Config {
	if c.MaxViewers <= 0 {
		c.MaxViewers = DefaultMaxViewers
	}
	if c.RampEvery <= 0 {
		c.RampEvery = DefaultRampEvery
	}
	if c.MaxDuration <= 0 {
		c.MaxDuration = DefaultMaxDuration
	}
	if c.Scenario == "" {
		c.Scenario = ScenarioNatural
	}
	return c
}

// StopReason names why a run ended.
type StopReason string

const (
	// StopSLOBreach: the rolling p95 segment-fetch latency crossed the SLO,
	// meaning a real player at that viewer count would stall.
	StopSLOBreach StopReason = "slo_breach"
	// StopAdmissionFull: the server itself refused a stream with
	// X-Evey-Error: too_many_streams. This is the server's own capacity
	// verdict, not a load test error.
	StopAdmissionFull StopReason = "admission_full"
	// StopLibraryLimit: the media pool ran out of distinct items to point
	// another viewer at (natural scenario), or ran out entirely
	// (transcode scenario with zero candidates).
	StopLibraryLimit StopReason = "library_limit"
	// StopMaxViewers: the ramp reached Config.MaxViewers cleanly.
	StopMaxViewers StopReason = "max_viewers"
	// StopCancelled: Cancel was called, or the caller's context was
	// cancelled, while viewers were still healthy.
	StopCancelled StopReason = "cancelled"
	// StopTokenExpired: a viewer got a 401; the admin's token expired
	// mid-run.
	StopTokenExpired StopReason = "token_expired"
	// StopError: a viewer failed repeatedly (3 consecutive non-admission
	// failures) or another unrecoverable error occurred.
	StopError StopReason = "error"
)

// Report is the outcome of a completed (or cancelled) run.
type Report struct {
	Scenario         string
	SustainedViewers int
	StopReason       StopReason
	StopDetail       string

	TTFFMsP50, TTFFMsP95       int
	SegmentMsP50, SegmentMsP95 int
	SegmentSLOMs               int
	Samples                    int

	// SharedCacheViewers counts viewers that reused an already-in-flight
	// media item (via a forked audio-resync param) because the library ran
	// out of distinct candidates. Only non-zero in the transcode scenario.
	SharedCacheViewers int

	StartedAt, EndedAt time.Time
}

// Status is a live snapshot of the current (or most recent) run.
type Status struct {
	Running    bool
	Phase      string
	Viewers    int
	Elapsed    time.Duration
	LastReport *Report
}
