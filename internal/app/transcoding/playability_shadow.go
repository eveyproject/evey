package transcoding

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// playability_shadow.go - SHADOW MODE ONLY. mkvgo v0.17.0 ships
// ops.Playability, a head-only per-track playability verdict against a named
// playback target. Evey's own decision engine (decision.go / copy_eligibility.go)
// is mature (green-frames, HDR tonemap, per-track pinning, HW/SW selection) and
// is NOT replaced here: PlayabilityShadow computes mkvgo's independent verdict
// alongside the real decision, purely to observe how often they agree, and logs
// only the divergences. Nothing in this file feeds back into
// PlaybackDecision - see decision.go's call to (*PlayabilityShadow).Observe,
// which runs after d is fully computed and never mutates it.

// PlayabilityShadowConfigKey is the server_config key gating the shadow
// comparison (admin panel, never an env var - see a project note).
// "on" enables it; any other value, or the key being absent (the boot
// default), keeps it off.
const PlayabilityShadowConfigKey = "transcoding.playability_shadow"

// playabilityShadowThrottleTTL bounds how often the SAME (media path, target)
// divergence is re-logged: a hot title re-fetching its HLS master playlist
// every few seconds must not spam a WARN per request.
const playabilityShadowThrottleTTL = time.Hour

// playabilityShadowSeenMax triggers an opportunistic sweep of expired throttle
// entries once the in-memory map grows past this size, so a large library with
// unique paths does not grow the map unbounded over a long uptime.
const playabilityShadowSeenMax = 4096

// playabilityShadowTimeout bounds the detached goroutine's own work (head-only
// metadata read, no block walk) - generous, since it never blocks a caller.
const playabilityShadowTimeout = 30 * time.Second

// playabilityShadowGateTTL bounds how stale the cached on/off flag may be.
// 30s is short enough that an operator flipping the switch sees it take effect
// while still watching, and long enough that the lookup is negligible next to
// one-per-segment.
const playabilityShadowGateTTL = 30 * time.Second

// playabilityShadowGateTimeout bounds the gate lookup itself. It runs inline on
// the decision path, so it must never be the thing that stalls a segment: if
// the config store is slow, fail closed and try again after the TTL.
const playabilityShadowGateTimeout = 2 * time.Second

// PlayabilityTarget is the app-owned mirror of the handful of mkvgo
// ops.Target fields transcoding.ClientProfile can reliably supply (see
// mapClientProfileToTarget). Kept as plain primitives here - rather than
// importing mkvgo's ops.Target directly - so this package stays free of the
// mkvgo dependency; the mkvgo adapter's Playability method (the
// PlayabilityAnalyzer implementation) does the ops.Target conversion.
type PlayabilityTarget struct {
	Name        string
	Containers  []string
	VideoCodecs []string
	AudioCodecs []string
	MaxWidth    int
	MaxHeight   int
	HDR         bool
}

// PlayabilityTrackVerdict is one track's mkvgo playability verdict, mirroring
// ops.TrackVerdict.
type PlayabilityTrackVerdict struct {
	TrackID uint64
	Type    string // "video" | "audio" | "subtitle"
	Verdict string // "direct-play" | "remux" | "transcode"
	Reasons []string
}

// PlayabilityReport is the whole-file mkvgo playability verdict, mirroring
// ops.PlayabilityReport.
type PlayabilityReport struct {
	Target         string
	OverallVerdict string
	RemuxContainer string
	Tracks         []PlayabilityTrackVerdict
}

// PlayabilityAnalyzer is the narrow port onto mkvgo's ops.Playability: a
// head-only, per-track playability verdict against target. Implemented by the
// mkvgo adapter's Playability method. A nil report with a nil error means the
// container is not one ops.Playability can evaluate head-only (not
// MKV/WebM/MP4/MOV) - a legitimate "nothing to compare" outcome, not a failure.
type PlayabilityAnalyzer interface {
	Playability(ctx context.Context, path string, target PlayabilityTarget) (*PlayabilityReport, error)
}

// PlayabilityShadow runs mkvgo's ops.Playability verdict SIDE BY SIDE with the
// real decision engine, purely to observe agreement - it never influences
// VideoDecision/AudioDecision/PlayMethod. Nil-safe throughout: a nil
// *PlayabilityShadow makes every Observe call an inert no-op, so wiring it via
// decision.go's SetPlayabilityShadow is strictly opt-in.
type PlayabilityShadow struct {
	analyzer PlayabilityAnalyzer
	config   shared.ServerConfigRepository
	logger   *slog.Logger

	mu   sync.Mutex
	seen map[string]time.Time // throttleKey -> last time a divergence was logged

	// wg tracks in-flight Observe goroutines. Only Wait (test-only - production
	// callers never need this, Observe is deliberately fire-and-forget) reads it.
	wg sync.WaitGroup

	// gate caches the on/off config lookup. Observe runs once per transcode
	// decision - once per HLS segment, so ~1200 times an hour per viewer - and
	// the lookup is an uncached SELECT, so reading it per observation put a
	// query on the hot path for a feature that is off by default. Its own mutex
	// rather than mu: shouldLog holds mu on the divergence path and the two
	// have nothing to do with each other.
	gateMu        sync.Mutex
	gateOn        bool
	gateCheckedAt time.Time
}

// NewPlayabilityShadow builds a shadow comparer. A nil logger falls back to
// slog.Default().
func NewPlayabilityShadow(analyzer PlayabilityAnalyzer, config shared.ServerConfigRepository, logger *slog.Logger) *PlayabilityShadow {
	if logger == nil {
		logger = slog.Default()
	}
	return &PlayabilityShadow{
		analyzer: analyzer,
		config:   config,
		logger:   logger,
		seen:     make(map[string]time.Time),
	}
}

// Observe compares decision (the real, already-rendered PlaybackDecision)
// against mkvgo's Playability verdict for path/client, entirely off the
// decision path: it spawns a detached goroutine (panic-recovered) and returns
// immediately. Nil-safe (a nil *PlayabilityShadow, nil analyzer, empty path or
// nil decision all no-op) so decision.go's call site never has to guard first.
func (s *PlayabilityShadow) Observe(ctx context.Context, path string, decision *transcoding.PlaybackDecision, client transcoding.ClientProfile) {
	if s == nil || s.analyzer == nil || path == "" || decision == nil {
		return
	}
	// Gate BEFORE spawning. This check used to live inside the goroutine, so a
	// disabled feature still cost one goroutine plus one uncached SELECT for
	// every segment of every stream. Off is the default, so that was the normal
	// case, not the edge case.
	if !s.gateAllows(ctx) {
		return
	}
	// Detached from the request context on purpose (same reasoning as
	// keyframe_ondemand's background probe): this comparison must survive the
	// request finishing, so a plain ctx would risk being cancelled mid-read.
	// context.Background() would drop the caller's trace/values though, so
	// this propagates from ctx via WithoutCancel instead of starting fresh -
	// satisfying "propagate from caller" without inheriting cancellation.
	bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), playabilityShadowTimeout)
	s.wg.Go(func() {
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("playability shadow: recovered panic",
					slog.Any("panic", r), slog.String("path", path))
			}
		}()
		s.observeOnce(bgCtx, path, decision, client)
	})
}

// Wait blocks until every in-flight Observe goroutine has finished. Test-only
// synchronization hook - production callers never need this, the whole point
// of Observe is to be fire-and-forget. Nil-safe.
func (s *PlayabilityShadow) Wait() {
	if s == nil {
		return
	}
	s.wg.Wait()
}

// observeOnce is Observe's synchronous body, factored out so it runs entirely
// on the detached goroutine: gate check, target mapping, analyzer call, then
// the comparison log. Any of these failing (gate off, analyze error, no video
// track) is a silent no-op - this feature must never surface as an error to
// anything downstream.
func (s *PlayabilityShadow) observeOnce(ctx context.Context, path string, decision *transcoding.PlaybackDecision, client transcoding.ClientProfile) {
	// No gate check here: Observe already refused to spawn us if the feature is
	// off. Re-reading it would put a second SELECT per segment on the enabled
	// path, which is the one that matters once an operator turns this on.
	target := mapClientProfileToTarget(client)
	report, err := s.analyzer.Playability(ctx, path, target)
	if err != nil {
		s.logger.Debug("playability shadow: analyze failed", slog.String("path", path), slog.Any("err", err))
		return
	}
	if report == nil {
		return // container mkvgo can't evaluate head-only - nothing to compare
	}

	mkvgoVerdict, mkvgoReasons := videoTrackVerdict(report)
	if mkvgoVerdict == "" {
		return // report carries no video track
	}
	eveyVerdict := eveyVideoVerdict(decision)

	if eveyVerdict == mkvgoVerdict {
		s.logger.Debug("playability shadow: convergent",
			slog.String("path", path), slog.String("target", target.Name),
			slog.String("verdict", eveyVerdict))
		return
	}

	key := path + "::" + targetFingerprint(target)
	if !s.shouldLog(key) {
		return
	}
	s.logger.Warn("playability shadow: divergent verdict",
		slog.String("path", path),
		slog.String("target", target.Name),
		slog.String("evey_verdict", eveyVerdict),
		slog.String("mkvgo_verdict", mkvgoVerdict),
		slog.String("mkvgo_overall", report.OverallVerdict),
		slog.Any("mkvgo_reasons", mkvgoReasons))
}

// enabled reads the PlayabilityShadowConfigKey server_config row. Any failure
// to read it (nil repo, missing row, store error) defaults to off - the same
// "must never break the caller" posture the rest of this file follows.
// gateAllows reports whether the shadow is switched on, re-reading the config
// at most once per playabilityShadowGateTTL. The read is deliberately
// synchronous: it is one query per TTL rather than one per segment, and doing
// it inline keeps "operator flips the switch, next observation past the TTL
// picks it up" true without a second layer of background refresh.
//
// The config read is detached from the caller's cancellation: a viewer closing
// the tab mid-decision must not cache a spurious "off" for the whole TTL.
func (s *PlayabilityShadow) gateAllows(ctx context.Context) bool {
	s.gateMu.Lock()
	defer s.gateMu.Unlock()
	if !s.gateCheckedAt.IsZero() && time.Since(s.gateCheckedAt) < playabilityShadowGateTTL {
		return s.gateOn
	}
	gateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), playabilityShadowGateTimeout)
	defer cancel()
	s.gateOn = s.enabled(gateCtx)
	s.gateCheckedAt = time.Now()
	return s.gateOn
}

// expireGateCache forces the next gateAllows to re-read the config. Test-only
// hook so a test does not have to sleep out the TTL.
func (s *PlayabilityShadow) expireGateCache() {
	s.gateMu.Lock()
	s.gateCheckedAt = time.Time{}
	s.gateMu.Unlock()
}

func (s *PlayabilityShadow) enabled(ctx context.Context) bool {
	if s.config == nil {
		return false
	}
	entry, err := s.config.Get(ctx, PlayabilityShadowConfigKey)
	if err != nil || entry == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(entry.Value), "on")
}

// shouldLog reports whether key's divergence has not been logged within
// playabilityShadowThrottleTTL, and records "now" as the last-logged time when
// it returns true. It also opportunistically sweeps expired entries once the
// map grows large, so a long-uptime server with a big, mostly-unique-path
// library does not grow this map without bound.
func (s *PlayabilityShadow) shouldLog(key string) bool {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if last, ok := s.seen[key]; ok && now.Sub(last) < playabilityShadowThrottleTTL {
		return false
	}
	s.seen[key] = now
	if len(s.seen) > playabilityShadowSeenMax {
		for k, t := range s.seen {
			if now.Sub(t) >= playabilityShadowThrottleTTL {
				delete(s.seen, k)
			}
		}
	}
	return true
}

// mapClientProfileToTarget builds a PlayabilityTarget directly from the
// fields transcoding.ClientProfile already carries (VideoCodecs, AudioCodecs,
// Containers, MaxWidth/MaxHeight, SupportsHDR) - the SAME struct the real
// decision engine judges the stream against, so this is a faithful mapping,
// not a guess at one of mkvgo's built-in named targets (safari/chrome/...).
//
// LIMITATION: ClientProfile carries no H.264/HEVC level ceiling, no Dolby
// Vision signal, and no HEVC-Main10 / VP9-Profile2 (10-bit) signal, so those
// ops.Target fields are left at their zero value: no level ceiling, DoVi and
// 10-bit unsupported. That is the conservative choice mkvgo's own strictest
// built-in targets (e.g. mse-generic) also make, so an unmapped-signal
// divergence can only make mkvgo's verdict LOOK MORE PERMISSIVE than reality
// (never the reverse) - worth knowing about should this shadow's divergence
// log ever motivate widening ClientProfile with those signals.
func mapClientProfileToTarget(client transcoding.ClientProfile) PlayabilityTarget {
	return PlayabilityTarget{
		Name:        "client-profile",
		Containers:  client.Containers,
		VideoCodecs: client.VideoCodecs,
		AudioCodecs: client.AudioCodecs,
		MaxWidth:    client.MaxWidth,
		MaxHeight:   client.MaxHeight,
		HDR:         client.SupportsHDR,
	}
}

// eveyVideoVerdict folds the real decision's VideoDecision/PlayMethod into the
// same three-value verdict space ops.Playability uses ("direct-play" /
// "remux" / "transcode"), so the two can be compared directly:
//   - VideoDecision == "transcode" -> "transcode" (matches regardless of PlayMethod)
//   - PlayMethod == "direct_stream" -> "remux" (video copied, container remuxed)
//   - otherwise -> "direct-play"
func eveyVideoVerdict(d *transcoding.PlaybackDecision) string {
	if d.VideoDecision == "transcode" {
		return "transcode"
	}
	if d.PlayMethod == "direct_stream" {
		return "remux"
	}
	return "direct-play"
}

// videoTrackVerdict returns the verdict and reasons of report's video track,
// or ("", nil) when the report carries none.
func videoTrackVerdict(report *PlayabilityReport) (string, []string) {
	for _, t := range report.Tracks {
		if t.Type == "video" {
			return t.Verdict, t.Reasons
		}
	}
	return "", nil
}

// targetFingerprint renders target's fields the mapping fills in into a
// stable throttle-key component, so the same media path with two materially
// different targets (e.g. two different clients) throttles independently.
func targetFingerprint(t PlayabilityTarget) string {
	return fmt.Sprintf("%s|%v|%v|%v|%dx%d|hdr=%v",
		t.Name, t.Containers, t.VideoCodecs, t.AudioCodecs, t.MaxWidth, t.MaxHeight, t.HDR)
}
