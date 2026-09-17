package httpapi

// transcode_vod_admission.go holds the shed/admission plumbing: fingerprint
// heights, host-full probing, and the admission-shaper adapter closures.

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// hlsVODServe wires the new pipeline: a static, content-addressable HLS VOD
// manifest (no live-edge snap, no drift), an on-disk segment cache shared
// across sessions with the same DecisionHash, and an orchestrator that spawns
// ffmpeg rafales on cache miss. Replaces hlsMasterServe / hlsSegmentHandler
// once the runtime validation step lands.
// ErrCodeFileNotReady is the stable reason a request is shed because the
// SOURCE is not ready to be served cheaply yet - its keyframe index is still
// being built, which is the one and only thing standing between it and a
// zero-cost copy/remux. Telling such a viewer "too many people are watching"
// would be a lie: nobody else is in their way, their file is.
const ErrCodeFileNotReady = "file_not_ready"

// ErrCodeTooManyStreams is the stable, machine-readable reason a playback
// request is shed when the host cannot prepare one more video stream. Clients
// key their user-facing wording on this code and never on the English message
// below - a viewer must read "too many things playing at once", not
// "transcoding capacity".
const ErrCodeTooManyStreams = "too_many_streams"

// ErrCodeCalibrating is the reason a playback request is shed while the
// server is benchmarking its own encode capacity: the bench pins the host for
// ~2 minutes, and only ever starts on an idle host, so a viewer arriving in
// that window is told to wait a moment rather than blamed on other viewers.
const ErrCodeCalibrating = "calibrating"

// ErrCodeFileNeedsRemux is the reason a request is refused because the FILE
// cannot be played by anyone: its tracks sit in separate blocks of minutes
// (see library.MediaFile.NeedsRemux) and every sequential reader - a phone,
// the HLS packager - stalls at the first block. Nothing to retry: an admin
// re-muxes it from the media-health page.
const ErrCodeFileNeedsRemux = "file_needs_remux"

// checkPlayable refuses, with the shed contract, a file no player can get
// through. Sits next to the parental check on every route that serves bytes
// of the file, so a client that skipped stream-info cannot stall on it.
func checkPlayable(w http.ResponseWriter, item *domainlibrary.MediaItem) bool {
	if item == nil || !item.File.NeedsRemux() {
		return true
	}
	writeShed(w, ErrCodeFileNeedsRemux, 0, "tracks stored in one-track blocks; re-mux the file from the media-health page")
	return false
}

// calibratingRetryAfterSec is the Retry-After of a calibrating shed: the bench
// lasts ~2-3 min on a small host; a retry sooner just meets it again.
const calibratingRetryAfterSec = 120

// fingerprintHeights lists the distinct positive heights this fingerprint's
// ladder offers, unordered. The shed probe tries each: a request is refused
// only when NO height admits - never because one particular rung does not.
func fingerprintHeights(fp apptranscoding.DecisionFingerprint) []int {
	seen := make(map[int]bool, len(fp.Profiles))
	out := make([]int, 0, len(fp.Profiles))
	for _, p := range fp.Profiles {
		if p.Height > 0 && !seen[p.Height] {
			seen[p.Height] = true
			out = append(out, p.Height)
		}
	}
	return out
}

// shedIfHostFull probes the limiter and, when this stream cannot be admitted,
// writes the shed response and reports true. probeHeight says how generous the
// probe is. It never sheds a request the copy rung would serve for free, and
// it tells a still-indexing source apart from a genuinely full host. Reads
// the caller's resolved plan - the same decision/fingerprint the manifest is
// about to be built from.
//
// Advisory by construction: two viewers can both pass the probe and race at
// spawn. That is fine - the loser is shed at the segment, which remains the
// authoritative gate.
func (h *hlsVODServe) shedIfHostFull(w http.ResponseWriter, r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, rp *requestPlan, probeHeights func(apptranscoding.DecisionFingerprint) []int) bool {
	if h.videoAdmission == nil {
		return false
	}
	if rp.IsCopy {
		return false // remuxed, never encoded: no capacity slot is taken
	}
	// A running calibration bench owns the host for a couple of minutes: an
	// encode started under it stalls and skews the measure. Say so honestly.
	if h.calibrating != nil && h.calibrating() {
		slog.Info("hls-vod: shedding, calibration benchmark in progress", slog.String("media_id", mediaID))
		writeShed(w, ErrCodeCalibrating, calibratingRetryAfterSec, "server is benchmarking its encode capacity; retry once it is done")
		return true
	}
	// Price in the class this session will actually be billed as (GPU vs CPU
	// pool, HDR/AV1 software tables). Probe EVERY candidate height rather than
	// a single "cheapest" one: hw costs are not monotonic in height (a 720p
	// NVENC rung can cost more than 1080p), so no single height is safely the
	// cheapest a priori - the limiter prices each and one admit suffices.
	// The route is read from the fingerprint itself: its hw fields already
	// follow the pinned route (a spilled stream is tagged software), so this
	// is exactly what the spawn's TryAcquireRoute will charge.
	for _, ph := range probeHeights(rp.FP) {
		if h.videoAdmission(rp.FPHash, rp.FP, ph) {
			return false
		}
	}
	if copyBlockedOnlyByMissingIndex(rp.Decision, streams) {
		if h.keyframeWarmer != nil {
			go h.keyframeWarmer(context.WithoutCancel(r.Context()), mediaID, sourcePathOf(item))
		}
		writeShed(w, ErrCodeFileNotReady, 10, "source keyframe index still being built; the copy rung needs no encode slot")
		return true
	}
	slog.Info("hls-vod: shedding, host at video capacity", slog.String("media_id", mediaID))
	writeTooManyStreams(w, 5, "server at video transcoding capacity")
	return true
}

// writeShed sheds a request with everything a client needs to react well: a
// machine-readable code (in a header AND the body, since a media element only
// exposes the status line and headers), a Retry-After, and a technical message
// meant for logs and operators, never for the screen.
func writeShed(w http.ResponseWriter, code string, retryAfterSec int, technicalMsg string) {
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSec))
	w.Header().Set("X-Evey-Error", code)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write(fmt.Appendf(nil, `{"code":%q,"retryAfterSec":%d,"message":%q}`,
		code, retryAfterSec, technicalMsg))
}

// copyBlockedOnlyByMissingIndex reports whether this source WOULD be served by
// the zero-cost copy rung (no video encode, no capacity slot) if only its
// keyframe index existed. Everything else about it - codec, profile, level,
// bit depth, HDR/DoVi, frame-rate mode - already passes the copy gate.
//
// This is the difference between an honest "the host is full" and a wrong one:
// such a viewer is not competing with anybody, their file simply has not been
// indexed yet. The index lands on its own (on-demand walk at negotiation, or
// the paced backfill worker), so the right answer is "try again shortly", not
// "too many streams".
func copyBlockedOnlyByMissingIndex(decision domaintranscoding.PlaybackDecision, streams []*domainlibrary.MediaStream) bool {
	if decision.VideoDecision != "copy" {
		return false // the client genuinely cannot take this video as-is
	}
	for _, s := range streams {
		if s == nil || s.StreamType != "video" {
			continue
		}
		if len(s.Keyframes) > 0 {
			return false // indexed already; copy was declined for another reason
		}
		return apptranscoding.CopyVideoCodecString(apptranscoding.CopySource{
			Codec:         s.Codec,
			Profile:       s.Profile,
			Level:         s.Level,
			TenBit:        streamIsTenBit(s),
			ColorTransfer: s.ColorTransfer,
			DoViProfile:   s.DoViProfile,
			DoViBLCompat:  s.DoViBLCompat,
			FrameRateMode: s.FrameRateMode,
		}) != ""
	}
	return false
}

// writeTooManyStreams sheds a request with everything a client needs to react
// well: the code (in a header AND the body, since a media element only exposes
// the status line and headers), a Retry-After, and a technical message meant
// for logs and operators, not for the screen.
func writeTooManyStreams(w http.ResponseWriter, retryAfterSec int, technicalMsg string) {
	writeShed(w, ErrCodeTooManyStreams, retryAfterSec, technicalMsg)
}

// calibratingOf adapts the calibration service's bench flag, TRUE-nil when it
// is not wired (tests, hand-built servers).
func calibratingOf(c *apptranscoding.CalibrationService) func() bool {
	if c == nil {
		return nil
	}
	return c.IsBenchmarking
}

// videoAdmissionOf adapts the limiter to the handler's narrow probe, keeping a
// TRUE-nil func when no limiter is wired (tests, hand-built servers).
func videoAdmissionOf(l *apptranscoding.VideoSessionLimiter) func(string, apptranscoding.DecisionFingerprint, int) bool {
	if l == nil {
		return nil
	}
	// The SAME price as the spawn's (SessionCostOf, one derivation): a probe
	// priced on a zero route would ask the table, not the route evidence the
	// spawn uses.
	return func(key string, fp apptranscoding.DecisionFingerprint, topHeight int) bool {
		return l.CanAdmitSession(key, apptranscoding.SessionCostOf(fp, topHeight))
	}
}

// admissionCeilingOf / admissionBindOf adapt the shaper the same way: TRUE-nil
// funcs when it is not wired, so the handler's nil-checks stay meaningful.
func admissionRouteOf(s *apptranscoding.VideoAdmissionShaper) func(string, apptranscoding.RouteAdmission) apptranscoding.RouteGrant {
	if s == nil {
		return nil
	}
	return s.GrantRoute
}

func pinnedClassOf(s *apptranscoding.VideoAdmissionShaper) func(string) (apptranscoding.LadderClass, bool) {
	if s == nil {
		return nil
	}
	return s.PinnedClass
}

func admissionBindOf(s *apptranscoding.VideoAdmissionShaper) func(string, string) {
	if s == nil {
		return nil
	}
	return s.Bind
}

// frozenLadderOf / freezeLadderOf adapt the shaper's read-verbatim ladder freeze
// the same TRUE-nil way, so a handler built without a shaper keeps re-deriving.
func frozenLadderOf(s *apptranscoding.VideoAdmissionShaper) func(string) ([]domaintranscoding.HLSProfile, bool) {
	if s == nil {
		return nil
	}
	return s.FrozenLadder
}

func freezeLadderOf(s *apptranscoding.VideoAdmissionShaper) func(string, []domaintranscoding.HLSProfile) {
	if s == nil {
		return nil
	}
	return s.FreezeLadder
}
