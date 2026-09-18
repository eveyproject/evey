package transcoding

// playback_plan_resolver.go - the concrete PlaybackPlanResolver: the playback
// "brain" absorbed from the http adapter (decision pins, ladder shaping,
// hardware routing, fingerprint assembly, copy-rung routing, producer pick).
// The adapter now only marshals a *http.Request into a PlaybackRequest and
// serves off the returned PlaybackPlan; everything decision-shaped lives here,
// testable without net/http. Field-by-field the dependencies mirror what the
// handler carried - all nil-safe, so a partially-wired resolver (hand-built
// tests) degrades exactly like the partially-wired handler did.

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
	pkgcache "github.com/evey-project/evey/internal/pkg/cache"
)

// PlanResolver resolves a PlaybackRequest into an immutable PlaybackPlan. It
// implements PlaybackPlanResolver. Zero value is usable: every dependency is
// optional and its absence reproduces the safe fallback the http handler had
// (720p software capability, no admission, no copy rung, no complexity class).
type PlanResolver struct {
	// Capability reports the host's calibrated encode ceiling; nil → SafeFallback.
	// Capability is the host as the planner reads it: a snapshot (normalized
	// row), never the persisted row itself.
	Capability func() transcoding.HostCapabilitySnapshot
	// RouteCosts is the per-route cost model (nil-safe) the backend-per-route
	// pick reads to choose the CHEAPEST correct+proven backend for a route.
	// nil → the pick falls back to the primary backend (single-backend behaviour).
	RouteCosts func() *RouteCostModel
	// HWAccelEnabled / AV1Enabled / AV1MaxHeight / HWTonemapEnabled are the
	// operator toggles, read dynamically (admin changes apply without restart).
	HWAccelEnabled   func() bool
	AV1Enabled       func() bool
	AV1MaxHeight     func() int
	HWTonemapEnabled func() bool
	// HWLowPowerEnabled: admin opt-in for the Intel low-power (VDEnc) encoder.
	// Off (nil) keeps the probed low_power_* features out of the encode.
	HWLowPowerEnabled func() bool
	// Expressibility is the ffmpeg builder's own knowledge of the routes it
	// can emit (ADR-0030). The backend pick consults it per candidate: a
	// backend whose pipeline the builder cannot express for the topology and
	// the needs is not in the field. The resolver keeps no mirror of its
	// rules. nil = nothing refused (a resolver built without the port).
	Expressibility transcoding.RouteExpressibility
	// MaxVideoBitrateKbps caps the served video bitrate (transcoding.max_video_bitrate_kbps, 0 = source).
	MaxVideoBitrateKbps func() int
	// AudioNormalize reports the loudnorm admin setting (forces audio re-encode).
	AudioNormalize func(ctx context.Context) bool
	// CapOverride is the CMCD-feedback adaptive ceiling (never raises, only lowers).
	CapOverride func(sourceCodec string, sourceHeight int) (transcoding.CapOverride, bool)

	// ComplexityRepo + ClassLookup resolve the per-title ladder class;
	// ClassLookupTTL bounds the hot-path cache entry. It must be > 0 to cache at
	// all: TTLCache.Set stamps expiresAt = now + ttl and Get treats a passed
	// expiry as a miss, so 0 stores an already-expired entry and every segment
	// re-queries ComplexityRepo. (The http adapter wires 10s.)
	ComplexityRepo transcoding.ComplexityRepo
	ClassLookup    *pkgcache.TTLCache[transcoding.ComplexityEntry]
	ClassLookupTTL time.Duration

	// Selector routes a transcode to the software or hardware producer;
	// CopyProducer serves the ffmpeg copy rung (nil = rung off); Orchestrator
	// is the no-selector fallback (hand-built test handlers). FreeBytes is the
	// segment cache's disk probe for the copy-rung disk guard.
	Selector     *ProducerSelector
	CopyProducer StreamProducer
	Orchestrator StreamProducer
	FreeBytes    func() (int64, error)

	// Admission closures - all optional, TRUE-nil when the shaper/limiter is
	// not wired (see the adapter's admissionRouteOf and friends).
	// AdmissionRoute carries the source class so the shed probe prices the decode
	// like the spawn does.
	AdmissionRoute        func(key string, req RouteAdmission) RouteGrant
	AdmissionPinnedClass  func(key string) (LadderClass, bool)
	AdmissionBind         func(key, fingerprintHash string)
	AdmissionFrozenLadder func(key string) ([]transcoding.HLSProfile, bool)
	AdmissionFreezeLadder func(key string, ladder []transcoding.HLSProfile)
	// AdmissionNoteRoute hands the pin the identity of the route this Resolve
	// settled on (VideoAdmissionShaper.NoteRoute): a live pin whose route
	// moves is counted and logged, never re-frozen. Optional.
	AdmissionNoteRoute func(key, routeIdentity string)

	// HWRouteBlocked reports whether this title's GPU route failed recently
	// (HWRouteBlocklist): the title is then shaped, fingerprinted and produced
	// exactly like an admission spill - the CPU route, every delivery cap kept
	// - so the client's retry plays instead of replaying the failure. nil
	// blocks nothing.
	HWRouteBlocked func(mediaID string) bool

	// AudioEncoderID returns the concrete ffmpeg AAC implementation this host
	// will encode with ("aac" / "libfdk_aac"). nil → native, i.e. the historical
	// bytes. Threaded into the FINGERPRINT and from there into the process opts,
	// so the cache key and the command line read the SAME value.
	AudioEncoderID func() string

	// UseFMP4 selects the segment container (project default true);
	// AudioRenditionEnabled mirrors the audio-rendition feature flag.
	UseFMP4               bool
	AudioRenditionEnabled bool
	// TranscodeAudioRendition extends the audio rendition to transcoded ladders
	// (the http adapter sets it when it can compensate the encoder's delay).
	TranscodeAudioRendition bool
	// Ladder shapes the rungs (nil = the static class table); Quality turns
	// each rung into an encoder target (nil = the static policy).
	Ladder  transcoding.LadderBuilder
	Quality transcoding.QualityPolicy
}

var _ PlaybackPlanResolver = (*PlanResolver)(nil)

// Resolve computes the whole plan. Side-effect order is the contract: the
// decision first, then the ladder shaping (which creates/refreshes the
// admission pin and freezes the ladder), then the fingerprint (which binds
// its hash to that pin).
func (pr *PlanResolver) Resolve(ctx context.Context, req PlaybackRequest) (PlaybackPlan, error) {
	// The source facts and the operator's policy are read ONCE here; every
	// derivation below reads req.Source and req.Policy.
	req.Source = SnapshotOf(req)
	req.Policy = pr.hwPolicy()
	return pr.resolveDecided(ctx, req, pr.Decide(ctx, req)), nil
}

// resolveDecided is Resolve past the decision: the one derivation of the
// route and its projections for a request whose source and policy were
// snapshotted. Tests that hold a decision enter here; production enters
// through Resolve only.
func (pr *PlanResolver) resolveDecided(ctx context.Context, req PlaybackRequest, decision transcoding.PlaybackDecision) PlaybackPlan {
	isCopy := pr.IsCopy(decision, req.Streams, req.SourceBytes)
	// The route is resolved ONCE (ADR-0030): the natural route with the
	// per-title block applied, then admission may spill it to the CPU. The
	// ladder, the fingerprint, the class and the producer project it.
	route := pr.resolveRoute(req, decision, isCopy)
	pinKey := pr.pinKeyFor(req, decision, route)
	profiles, grant, spilled := pr.shapeLadderFor(ctx, req, decision, route, pinKey)
	if spilled {
		route = route.spill(SpillPoolFull, decision)
	}
	// The plan names its pin key even when no admission is wired (a bare
	// resolver): the key is a projection of the route, not of the shaper.
	grant.PinKey = pinKey
	fp := pr.fingerprintFor(ctx, req, decision, route, profiles, pinKey)
	if pr.AdmissionNoteRoute != nil && !isCopy {
		pr.AdmissionNoteRoute(pinKey, route.Identity())
	}
	packager := PackagerTranscodeFFmpeg
	if isCopy {
		packager = PackagerCopyFFmpeg
	}
	return PlaybackPlan{
		Decision:        decision,
		Class:           route.Class(),
		Ladder:          profiles,
		VideoCodec:      route.Codec,
		HWAccel:         route.Backend.Name,
		Packager:        packager,
		FingerprintHash: fp.Hash(),
		WasmAudio:       req.WasmAudio,
		Fingerprint:     fp,
		Route:           route,
		Admission:       grant,
	}
}

// Decide is the per-track copy/transcode decision with the request-time pins
// applied: the pure MakePlaybackDecisionWithPrefs core, then the audio-resync
// override, the vd=/ad= mid-play pins, and the HDR tonemap re-add for any HDR
// source that will not be served as a raw copy.
func (pr *PlanResolver) Decide(ctx context.Context, req PlaybackRequest) transcoding.PlaybackDecision {
	prefs := req.Prefs
	// Inject the selected subtitle track so the decision only burns (and so only
	// denies the copy rung) when the user actually plays this bitmap subtitle -
	// not whenever the file merely carries one. Mirrors the subtitleTrack pin
	// the fingerprint and the burn-in filter already consume.
	prefs.SubtitleBurnInTrack = req.SubtitleBurnInTrack
	// Inject the audio track the request will actually serve, so the audio
	// copy-vs-transcode decision is made on THAT track (the default may be DTS
	// while the user plays an AAC track, or vice-versa). Mirrors the audioTrack
	// resolution the fingerprint and the ffmpeg -map already use.
	prefs.AudioTrack = req.AudioTrack
	prefs.SourceBitrateKbps = req.SourceBitrateKbps
	d := MakePlaybackDecisionWithPrefs(ctx, req.Streams, req.Container, req.Client, req.MaxBitrateKbps, prefs, req.SourcePath)
	if d == nil {
		return transcoding.PlaybackDecision{}
	}
	// User audio resync: shifting audio requires re-encoding it from a
	// time-shifted input, so a non-zero offset forces the audio decision to
	// transcode (a copied track cannot be shifted). The offset itself rides
	// the fingerprint (cache fork) and every baked URL.
	if req.AudioOffsetMs != 0 && d.AudioDecision != "transcode" {
		d.AudioDecision = "transcode"
		d.TranscodeReasons = append(d.TranscodeReasons, "audio_resync_requested")
		if d.AudioCodec == "" {
			d.AudioCodec = "aac"
		}
	}
	// Mid-play decision pinning. The first playlist of a playback stamps the
	// then-current video decision into every child URL (vd=..., see
	// buildManifestInput/querySuffix); this honors it on the follow-up polls.
	// Background enrichment can flip copy-eligibility BETWEEN two polls of a
	// live EVENT playlist - measured in prod (2026-07-07): the
	// keyframe backfill indexed the file 22 min into a TRANSCODE playback,
	// the same playlist URL switched to the copy plan, and the source-h264
	// segments under the transcode init rendered as green garbage on the
	// Chromecast's hardware decoder. Pinning "transcode" always holds (a
	// re-encode is always serveable). Pinning "copy" cannot be forced - if
	// the fresh decision no longer says copy the plan is structurally gone
	// (keyframes wiped) and a re-encode is the only serveable output - so a
	// broken copy pin flips WITH a warning instead of 500ing the session.
	switch req.PinnedVideoDecision {
	case "transcode":
		if d.VideoDecision == "copy" {
			d.VideoDecision = "transcode"
			d.TranscodeReasons = append(d.TranscodeReasons, "decision_pinned")
			if d.VideoCodec == "" {
				d.VideoCodec = "h264"
			}
		}
	case "copy":
		if d.VideoDecision != "copy" {
			slog.WarnContext(ctx, "hls-vod: pinned copy decision no longer serveable, session flips to transcode mid-play (player may need a restart)",
				slog.String("media_id", req.MediaID),
				slog.String("reasons", strings.Join(d.TranscodeReasons, ",")))
		}
	}
	// Output-codec pin (vc=): the fallback rung of a hybrid copy master keeps
	// the copy rung's codec family, so the player's emergency drop is a rung
	// switch and not an H.264->HEVC decoder swap in the same buffer under
	// starvation. A codec this client cannot decode is ignored, never forced.
	// The reason is stamped even when the decision already says the pinned
	// codec: it is what stops outputCodecFor's HEVC upgrade downstream.
	if pin := transcoding.CanonicalVideoCodec(req.PinnedVideoCodec); (pin == "h264" || pin == "hevc" || pin == "av1") &&
		d.VideoDecision == "transcode" && pr.clientDecodes(*d, pin) {
		d.VideoCodec = pin
		d.TranscodeReasons = append(d.TranscodeReasons, "video_codec_pinned")
	}
	// Audio-side pin, same contract as vd: "transcode" always holds; a broken
	// "copy" pin (loudnorm toggled on, codec no longer passthrough-eligible)
	// flips WITH a warning - the session's audio codec changes mid-play and
	// the player may need a restart, but a re-encode is always serveable.
	switch req.PinnedAudioDecision {
	case "transcode":
		if d.AudioDecision != "transcode" {
			d.AudioDecision = "transcode"
			d.TranscodeReasons = append(d.TranscodeReasons, "decision_pinned")
			if d.AudioCodec == "" {
				d.AudioCodec = "aac"
			}
		}
	case "copy":
		if d.AudioDecision != "copy" {
			slog.WarnContext(ctx, "hls-vod: pinned audio copy decision no longer serveable, session flips to transcoded audio mid-play",
				slog.String("media_id", req.MediaID),
				slog.String("reasons", strings.Join(d.TranscodeReasons, ",")))
		}
	}
	// A transcoded rung is SDR BY CONTRACT - the master only advertises
	// VIDEO-RANGE on the COPY rung. An HDR-capable client (hdr=1) legitimately
	// suppresses the decision-level tonemap for the raw-serve paths, but when
	// the copy rung is UNAVAILABLE (keyframes/HDR truth not yet backfilled,
	// non-wired codec/profile, copy producer off) this request still
	// re-encodes - and a PQ source x264-encoded without tonemapping renders
	// washed-out (proven on prod bytes: SATAVG 3.1 untonemapped vs the same
	// segment tonemapped). Re-add the tonemap for any HDR source that will
	// NOT be served as a raw copy. The disk guard can still flip copy→ladder
	// at serve time without re-adding it - accepted bounded edge (hashing
	// FreeBytes would be worse).
	snap := snapshot(req)
	servedAsRawCopy := d.VideoDecision == "copy" && pr.CopyProducer != nil && snap.CopyVideoCodec != ""
	if !servedAsRawCopy &&
		!slices.Contains(d.TranscodeReasons, "hdr_tonemapping_needed") &&
		snap.IsHDR {
		// Judge the passthrough on the transcode this will be, not on the copy
		// verdict it started as: a copy-blocked title (keyframes pending, VFR,
		// HDR truth unknown) on an HDR client and a Main10 GPU keeps its HDR.
		asTranscode := *d
		asTranscode.VideoDecision = "transcode"
		if pr.hdrPassthroughWanted(pr.policy(req), asTranscode, req.Streams) {
			if d.VideoDecision != "transcode" {
				d.TranscodeReasons = append(d.TranscodeReasons, "hdr_copy_unavailable")
			}
		} else {
			d.TranscodeReasons = append(d.TranscodeReasons, "hdr_tonemapping_needed")
		}
		d.VideoDecision = "transcode"
		if d.VideoCodec == "" {
			d.VideoCodec = "h264"
		}
	}
	return *d
}

// snapshot returns the request's source snapshot, taking it when a caller
// reached a sub-derivation without going through Resolve.
func snapshot(req PlaybackRequest) SourceSnapshot {
	if req.Source.Taken() {
		return req.Source
	}
	return SnapshotOf(req)
}
