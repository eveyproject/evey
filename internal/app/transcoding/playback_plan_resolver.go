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
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
	pkgcache "github.com/evey-project/evey/internal/pkg/cache"
)

// PlanResolver resolves a PlaybackRequest into an immutable PlaybackPlan. It
// implements PlaybackPlanResolver. Zero value is usable: every dependency is
// optional and its absence reproduces the safe fallback the http handler had
// (720p software capability, no admission, no copy rung, no complexity class).
type PlanResolver struct {
	// Capability reports the host's calibrated encode ceiling; nil → SafeFallback.
	Capability func() transcoding.EncodeCapability
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
	decision := pr.Decide(ctx, req)
	profiles := pr.ShapeLadder(ctx, req, decision)
	fp := pr.Fingerprint(ctx, req, decision, profiles)
	isCopy := pr.IsCopy(decision, req.Streams, req.SourceBytes)
	packager := PackagerTranscodeFFmpeg
	if isCopy {
		packager = PackagerCopyFFmpeg
	}
	return PlaybackPlan{
		Decision: decision,
		// The class the spawn will be billed as - read from the fingerprint,
		// whose hw fields already follow the pinned route (a spilled stream
		// is tagged software).
		Class:           LadderClassOf(fp),
		Ladder:          profiles,
		VideoCodec:      fp.VideoCodec,
		HWAccel:         fp.HWAccel,
		Packager:        packager,
		FingerprintHash: fp.Hash(),
		WasmAudio:       req.WasmAudio,
		Fingerprint:     fp,
	}, nil
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
	servedAsRawCopy := d.VideoDecision == "copy" && pr.CopyProducer != nil && SourceCopyVideoCodec(req.Streams) != ""
	if !servedAsRawCopy &&
		!slices.Contains(d.TranscodeReasons, "hdr_tonemapping_needed") &&
		SourceIsHDR(req.Streams) {
		// Judge the passthrough on the transcode this will be, not on the copy
		// verdict it started as: a copy-blocked title (keyframes pending, VFR,
		// HDR truth unknown) on an HDR client and a Main10 GPU keeps its HDR.
		asTranscode := *d
		asTranscode.VideoDecision = "transcode"
		if pr.HDRPassthroughWanted(asTranscode, req.Streams) {
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

// capabilityOrFallback resolves the calibrated capability, nil-safe.
func (pr *PlanResolver) capabilityOrFallback() transcoding.EncodeCapability {
	if pr.Capability == nil {
		return transcoding.SafeFallback()
	}
	return pr.Capability()
}

// hwSource is what the GPU decode gate reads about the source.
type hwSource struct {
	Codec       string
	TenBit      bool
	PixelFormat string
}

// ResolveHW decides the hardware encode backend for a chosen output codec.
// Returns ("", "", false) - the software path - unless the flag is on, the
// host calibrated a HW backend, and that backend can encode this codec. The
// boolean is both the ladder's hwAccelUsable (taller HW ceiling) and the
// signal that tags the fingerprint so the orchestrator runs the GPU pipeline.
// HDR tonemap / bitmap burn-in are not gated here: the orchestrated VOD path
// does neither today, and the ffmpeg builder independently falls back to
// software for those, so the two stay consistent.
func (pr *PlanResolver) ResolveHW(outputCodec, sourceCodec string, sourceTenBit, hwTonemap bool) (accel, device string, features []string, usable, softwareDecode bool) {
	return pr.resolveHW(outputCodec, hwSource{Codec: sourceCodec, TenBit: sourceTenBit}, hwTonemap)
}

// resolveHW is ResolveHW with the full source facts. Every GPU shape it can
// answer with has to be PROVEN by the calibration's route probes
// (EncodeCapability.HWRoutes): the filter-level facts say what the GPU can
// do, the route proof says the builder's session for it actually runs here.
func (pr *PlanResolver) resolveHW(outputCodec string, src hwSource, hwTonemap bool) (accel, device string, features []string, usable, softwareDecode bool) {
	if pr.HWAccelEnabled == nil || !pr.HWAccelEnabled() || pr.Capability == nil {
		return "", "", nil, false, false
	}
	cap := pr.Capability()
	// Choose the backend for THIS route (not one global "best host backend"):
	// the cheapest accelerator that is correct, proves the route and supports
	// the codec. On a box with both, HDR->SDR takes VAAPI's OpenCL tone-map
	// while HEVC/SDR take a faster QSV.
	bc, ok := pr.pickBackend(outputCodec, hwTonemap)
	if !ok {
		return "", "", nil, false, false
	}
	backend := transcoding.HWAccelType(bc.Backend)
	// The zero-copy pipeline HW-decodes the source. If this backend can't decode
	// it (chroma/depth/codec), the source takes the DECOUPLED path (software
	// decode + hwupload + GPU scale/encode), unless that is not expressible or
	// honest: no upload chain / unproven decoupled route, a GPU tone-map (needs
	// the 10-bit surface the upload folds away), or a 10-bit source (its
	// software decode is the expensive half - price it as CPU, not GPU).
	decodable := bc.CanDecodeSource(src.Codec, src.TenBit, src.PixelFormat, cap.ProbeVersion)
	softwareDecode = src.Codec != "" && !decodable
	if softwareDecode {
		if hwTonemap || src.TenBit || (backend != transcoding.VAAPI && backend != transcoding.QSV) ||
			!bc.Proves(transcoding.HWRouteDecoupled) {
			return "", "", nil, false, false
		}
	}
	// A 10-bit source needs the HW VPP to fold p010->nv12 for the 8-bit encoder;
	// some drivers abort. Unless the 10-bit VPP is proven (and its builder route
	// ran), software-transcode. Exception: a GPU tone-map folds p010 itself.
	if src.TenBit && !hwTonemap && (!bc.HW10Bit || !bc.Proves(transcoding.HWRouteSource10Bit)) {
		return "", "", nil, false, false
	}
	// A source layout outside 4:2:0 8/10-bit rides the zero-copy chain only once
	// that chain ran on it end-to-end for this backend.
	if !softwareDecode && !transcoding.CanonicalDecodeFormat(src.PixelFormat) && !bc.Proves(transcoding.HWRouteSourceFormat(src.PixelFormat)) {
		return "", "", nil, false, false
	}
	return bc.Backend, bc.Device, pr.effectiveHWFeatures(bc.Features), true, softwareDecode
}

// pickBackend chooses the accelerator for a route: among the certified backends
// that prove the route's encode path and support the codec, the CHEAPEST by
// route cost (RouteCosts), with a <5% hysteresis tie-break (the primary, then
// alphabetical) so the pick does not flip on measurement noise across
// recalibrations. A tone-map route needs a backend proving HWRouteTonemap (only
// a GPU tone-mapper - QSV's vpp is ineligible - so HDR lands on VAAPI/OpenCL);
// everything else needs SDR (+HEVC / +AV1 for that output). nil RouteCosts or a
// pre-Backends capability falls back to the primary backend.
func (pr *PlanResolver) pickBackend(outputCodec string, hwTonemap bool) (transcoding.HWBackendCap, bool) {
	cap := pr.Capability()
	caps := cap.BackendCaps()
	if len(caps) == 0 {
		return transcoding.HWBackendCap{}, false
	}
	hevc := transcoding.CanonicalVideoCodec(outputCodec) == "hevc"
	av1 := transcoding.CanonicalVideoCodec(outputCodec) == "av1"
	costRoute := transcoding.RouteCostSDR
	qualifies := func(b transcoding.HWBackendCap) bool {
		if !b.SupportsCodec(outputCodec) || !b.Proves(transcoding.HWRouteSDR) {
			return false
		}
		if hevc && !b.Proves(transcoding.HWRouteHEVC) {
			return false
		}
		if av1 && !b.Proves(transcoding.HWRouteAV1) {
			return false
		}
		if hwTonemap && !b.Proves(transcoding.HWRouteTonemap) {
			return false
		}
		return true
	}
	if hwTonemap {
		costRoute = transcoding.RouteCostTonemap
	}
	var model *RouteCostModel
	if pr.RouteCosts != nil {
		model = pr.RouteCosts()
	}
	// cost of a backend's route at a reference rung (1080) and source fps (24):
	// the RELATIVE order of the backends is what matters, and it is stable
	// across rungs. Missing evidence => +inf (never preferred on cost).
	const refRung, refFPS = 1080, 24.0
	costOf := func(b transcoding.HWBackendCap) float64 {
		if model == nil {
			return 0 // no cost model: every qualifying backend ties, tie-break decides
		}
		enc := transcoding.RouteEncoderName(outputCodec, b.Backend)
		if c, ok := model.Cost(transcoding.RouteCostKeyFor(transcoding.HWAccelType(b.Backend), enc, costRoute), refRung, refFPS); ok {
			return c.Accel
		}
		return 1e9
	}
	var best transcoding.HWBackendCap
	var bestCost float64
	found := false
	for _, b := range caps {
		if !qualifies(b) {
			continue
		}
		c := costOf(b)
		if !found || betterBackend(b, c, best, bestCost, cap.HWBackend) {
			best, bestCost, found = b, c, true
		}
	}
	return best, found
}

// betterBackend is pickBackend's ordering: strictly cheaper wins beyond a 5%
// hysteresis band; within the band the PRIMARY backend wins, then the
// alphabetically-smaller one - both stable across recalibrations.
func betterBackend(cand transcoding.HWBackendCap, candCost float64, cur transcoding.HWBackendCap, curCost float64, primary string) bool {
	if hi := max(candCost, curCost); hi > 0 && absFloat(candCost-curCost)/hi >= 0.05 {
		return candCost < curCost
	}
	if strings.EqualFold(cand.Backend, primary) != strings.EqualFold(cur.Backend, primary) {
		return strings.EqualFold(cand.Backend, primary)
	}
	return cand.Backend < cur.Backend
}

func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// burnInStaysSoftware
// burnInStaysSoftware mirrors the builder's own refusal (hwUsable): a bitmap
// subtitle burn-in without a probed GPU compositor is built as a SOFTWARE
// encode, so the route, the class and the pool must say software too - or a
// CPU x264 session runs under the GPU orchestrator, priced on the GPU pool,
// capped at the GPU ceiling and blamed on the GPU when it fails.
func (pr *PlanResolver) burnInStaysSoftware(decision transcoding.PlaybackDecision) bool {
	return decision.SubtitleDecision == "burn" && !pr.HWOverlayWanted(decision)
}

// titleBlocked reads the per-title GPU route block (nil-safe).
func (pr *PlanResolver) titleBlocked(mediaID string) bool {
	return pr.HWRouteBlocked != nil && mediaID != "" && pr.HWRouteBlocked(mediaID)
}

// effectiveHWFeatures drops the low-power features unless the admin opted in.
func (pr *PlanResolver) effectiveHWFeatures(features []string) []string {
	if pr.HWLowPowerEnabled != nil && pr.HWLowPowerEnabled() {
		return features
	}
	out := make([]string, 0, len(features))
	for _, f := range features {
		if !strings.HasPrefix(f, "low_power_") {
			out = append(out, f)
		}
	}
	return out
}

// EffectiveVideoCodec applies the capability gate to the decision's chosen
// output codec and returns the codec the segments will ACTUALLY carry together
// with the HW resolution for that codec. AV1 software (libsvtav1) is several
// times heavier than H.264 and the weak-NAS target can't sustain it, so when AV1
// is requested but the host has no HW AV1 encoder AND no admin opt-in, fall
// back to H.264 rather than starve. Shared by the fingerprint (the cache key)
// and the manifest builder (the advertised CODECS) so the master playlist can
// never advertise a codec the segments don't carry. Re-resolves HW for the
// fallback codec (H.264 may be HW-encodable where AV1 was not).
func (pr *PlanResolver) EffectiveVideoCodec(decision transcoding.PlaybackDecision, streams []*library.MediaStream) (codec, accel, device string, features []string, usable, softwareDecode bool) {
	dims := SourceDimsOf(streams)
	hwTonemap := pr.HWTonemapWanted(decision, dims.HDRRange)
	// CPU tone-map bridged between GPU decode and encode: the GPU never sees the
	// 10-bit surface, but it must be the one decoding (the bridge downloads).
	bridge := !hwTonemap && slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed")
	resolve := func(c string) {
		accel, device, features, usable, softwareDecode = pr.resolveHW(c, hwSource{Codec: dims.Codec, TenBit: dims.TenBit && !bridge, PixelFormat: dims.PixelFormat}, hwTonemap)
		if pr.hwRouteRefused(decision, dims, bridge, softwareDecode) {
			accel, device, features, usable, softwareDecode = "", "", nil, false, false
		}
	}
	codec = pr.codecWithPassthrough(pr.outputCodecFor(decision, streams), decision, streams)
	resolve(codec)
	if codec == "hevc" && decision.VideoCodec != "hevc" && !usable {
		codec = "h264"
		resolve(codec)
	}
	if strings.EqualFold(codec, "av1") && ((pr.AV1Enabled != nil && !pr.AV1Enabled()) || !usable) {
		codec = "h264"
		resolve(codec)
	}
	return codec, accel, device, features, usable, softwareDecode
}

// hwRouteRefused gathers the whole-session reasons a resolved GPU encode still
// stays on the CPU, shared by EffectiveVideoCodec and HWRouteUsable so the
// fingerprint, the class and the producer can never disagree: inverse
// telecine (a software-only filter), a bridge whose decode would be software
// (the download needs a GPU frame) or whose route the calibration did not
// prove, and a burn-in the builder refuses to composite on the GPU.
func (pr *PlanResolver) hwRouteRefused(decision transcoding.PlaybackDecision, src SourceDims, bridge, softwareDecode bool) bool {
	if src.Telecine || pr.burnInStaysSoftware(decision) {
		return true
	}
	if bridge && (softwareDecode || !pr.capabilityOrFallback().HWRouteProven(transcoding.HWRouteBridge)) {
		return true
	}
	return false
}

// HWRouteUsable is the selector's routing gate: does the HARDWARE producer
// take this request? It must answer exactly like EffectiveVideoCodec tags the
// fingerprint - the route decides which physical resource the session burns
// (GPU vs CPU pools), so a gate/fingerprint disagreement makes admission price
// one pool while the spawn consumes the other. In particular it replays the
// AV1 fallback: an AV1-preferring client on an AV1-disabled (or AV1-incapable)
// host is ENCODED as H.264, so the route must be resolved for H.264 too.
func (pr *PlanResolver) HWRouteUsable(decision transcoding.PlaybackDecision, src SourceDims) bool {
	hwTonemap := pr.HWTonemapWanted(decision, src.HDRRange)
	// No GPU tone-mapper: the CPU tone-maps between the GPU decode and encode
	// (hwTonemapBridge). That needs the GPU to decode the source; a software
	// decode keeps the whole job on the CPU. Same answer as EffectiveVideoCodec.
	bridge := !hwTonemap && slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed")
	resolve := func(c string) bool {
		_, _, _, usable, softwareDecode := pr.resolveHW(c, hwSource{Codec: src.Codec, TenBit: src.TenBit && !bridge, PixelFormat: src.PixelFormat}, hwTonemap)
		return usable && !pr.hwRouteRefused(decision, src, bridge, softwareDecode)
	}
	codec := pr.outputCodecFor(decision, nil)
	usable := resolve(codec)
	if strings.EqualFold(codec, "av1") && ((pr.AV1Enabled != nil && !pr.AV1Enabled()) || !usable) {
		usable = resolve("h264")
	}
	if codec == "hevc" && decision.VideoCodec != "hevc" && !usable {
		usable = resolve("h264")
	}
	return usable
}

// outputCodecFor upgrades the decision's H.264 to HEVC when the client decodes it
// and the GPU encodes it; an explicit preference is never overridden. streams may be nil.
func (pr *PlanResolver) outputCodecFor(decision transcoding.PlaybackDecision, streams []*library.MediaStream) string {
	codec := decision.VideoCodec
	if decision.VideoDecision != "transcode" || slices.Contains(decision.TranscodeReasons, "preferred_video_codec") {
		return codec
	}
	if codec != "" && !strings.EqualFold(codec, transcoding.DefaultVideoCodec) {
		return codec
	}
	if !pr.clientDecodes(decision, "hevc") || pr.HWAccelEnabled == nil || !pr.HWAccelEnabled() || pr.Capability == nil {
		return codec
	}
	if pr.Capability().AnyBackendSupports("hevc") {
		return "hevc"
	}
	return codec
}

// outputCodecForPassthrough: the HDR passthrough is HEVC by construction.
func (pr *PlanResolver) codecWithPassthrough(codec string, decision transcoding.PlaybackDecision, streams []*library.MediaStream) string {
	if pr.HDRPassthroughWanted(decision, streams) {
		return "hevc"
	}
	return codec
}

func (pr *PlanResolver) clientDecodes(decision transcoding.PlaybackDecision, codec string) bool {
	for _, c := range decision.ClientVideoCodecs {
		if transcoding.CanonicalVideoCodec(c) == codec {
			return true
		}
	}
	return false
}

// HDRPassthroughWanted: an HDR source transcoded for an HDR client that decodes
// HEVC, on a GPU probed for Main10 encoding - keep the HDR instead of tone-mapping.
func (pr *PlanResolver) HDRPassthroughWanted(decision transcoding.PlaybackDecision, streams []*library.MediaStream) bool {
	if decision.VideoDecision != "transcode" || slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed") {
		return false
	}
	if SourceTranscodeVideoRange(streams) == "" || !pr.clientDecodes(decision, "hevc") {
		return false
	}
	if pr.HWAccelEnabled == nil || !pr.HWAccelEnabled() || pr.Capability == nil {
		return false
	}
	if _, ok := pr.Capability().PassthroughBackend(); !ok {
		return false
	}
	if decision.VideoCodec != "" && !strings.EqualFold(decision.VideoCodec, "h264") && !strings.EqualFold(decision.VideoCodec, "hevc") {
		return false
	}
	return true
}

// HWTonemapWanted reports whether this request should tone-map HDR->SDR on the
// GPU: the decision needs tone-mapping, the toggle is on, the host probed
// HWTonemap, and the backend has a wired HW tone-map filter (VAAPI OpenCL/VPP,
// NVENC tonemap_cuda; QSV's vpp is ineligible). When false the software
// tonemap path runs, byte-identical to a host with no GPU tone-mapping.
func (pr *PlanResolver) HWTonemapWanted(decision transcoding.PlaybackDecision, hdrRange string) bool {
	if pr.HWTonemapEnabled != nil && !pr.HWTonemapEnabled() {
		return false
	}
	if pr.HWAccelEnabled == nil || !pr.HWAccelEnabled() || pr.Capability == nil {
		return false
	}
	// A backend that tone-maps this range on the GPU (VAAPI OpenCL / NVENC CUDA
	// for any range, PQ-only for the fixed VPP; QSV vpp is ineligible). Picked PER ROUTE, so a
	// QSV-primary host still routes HDR->SDR to VAAPI's OpenCL.
	if _, ok := pr.Capability().TonemapBackend(hdrRange); !ok {
		return false
	}
	return slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed")
}

// HWOverlayWanted reports whether this request should burn its bitmap subtitle
// in ON THE GPU: the decision actually burns one, HW accel is on, and the host
// probed a working overlay compositor for its backend.
//
// It exists because a burn-in used to disqualify the hardware path outright, so
// one flagged-default forced subtitle sent an entire 4K HDR session - decode,
// tone-map and four encodes - to the CPU on a host whose GPU did all three
// (prod, 2026-09-09). Every backend is eligible here: unlike the tone-map, the
// compositor is wired for vaapi, qsv AND cuda, and the calibration probe is what
// says whether this host's driver honours it.
func (pr *PlanResolver) HWOverlayWanted(decision transcoding.PlaybackDecision) bool {
	if pr.HWAccelEnabled == nil || !pr.HWAccelEnabled() || pr.Capability == nil {
		return false
	}
	if _, ok := pr.Capability().OverlayBackend(); !ok {
		return false
	}
	return decision.SubtitleDecision == "burn"
}

// PinKey identifies a stream for the admission shaper. Coarser than the
// decision-fingerprint hash on purpose: burn-in/audio variants of the same
// title share one pinned ceiling (their video ladder is the same shape), while
// distinct titles or codecs never collide. It must be derivable identically at
// manifest- and segment-time from the request alone.
//
// It also carries every REQUEST-scoped ladder input (screen height, bitrate
// ceiling): the frozen ladder is read back by key, so two viewers of one title
// with different caps must not share a pin or the second inherits the first's.
func (pr *PlanResolver) PinKey(req PlaybackRequest, decision transcoding.PlaybackDecision) string {
	ec, _, _, _, _, _ := pr.EffectiveVideoCodec(decision, req.Streams)
	key := fmt.Sprintf("%s|%s|%s|sh=%d|mh=%d|bw=%d|lvl=%d", req.MediaID, ec, decision.VideoDecision,
		req.ScreenHeight, req.Client.MaxHeight, pr.videoBitrateCeilingKbps(req, decision), req.Client.MaxH264Level)
	// A title whose GPU route just failed gets a fresh pin: the ladder frozen
	// for its GPU session must not be served to its CPU session.
	if pr.titleBlocked(req.MediaID) {
		key += "|rb=1"
	}
	return key
}

// ShapeLadder picks the ABR ladder for a given (decision, media, streams)
// triple. Reads the per-title complexity class persisted by the background
// ComplexityWorker; falls back to LadderFilmNormal when the item hasn't been
// classified yet (safe: matches pre-refactor behaviour). Caps the ladder to
// the source resolution so we never advertise a 4K rung for a 1080p source.
func (pr *PlanResolver) ShapeLadder(ctx context.Context, req PlaybackRequest, decision transcoding.PlaybackDecision) []transcoding.HLSProfile {
	streams := req.Streams
	// Read-verbatim (Étape 2): a live stream's ABR ladder is shaped ONCE and
	// frozen on its admission pin; return it unchanged so the manifest and every
	// segment shape the SAME ladder BY CONSTRUCTION - immune to a mid-stream drift
	// of the mutable inputs below (complexity class, capability, CMCD/av1 caps)
	// that would otherwise desync a player's URLs from the cached bytes. Copy
	// rungs take no admission pin (never frozen); their single source-res rung is
	// stable regardless. streamKey is set only on the freezable (transcode) path,
	// and gates the freeze at the tail.
	isCopy := pr.IsCopy(decision, streams, req.SourceBytes)
	var streamKey string
	if !isCopy && pr.AdmissionFrozenLadder != nil {
		streamKey = pr.PinKey(req, decision)
		if frozen, ok := pr.AdmissionFrozenLadder(streamKey); ok {
			return frozen
		}
	}

	complexity := pr.complexityOf(ctx, req.MediaID)
	in := transcoding.LadderInput{
		Source:     pr.sourceFacts(req),
		Complexity: complexity,
		Route:      pr.routeFor(req, decision, streams, isCopy),
		Delivery:   pr.deliveryConstraints(req, decision),
	}
	laddered := pr.ladderBuilder().Build(in)

	if pr.AdmissionRoute != nil && len(laddered) > 0 && !isCopy {
		natural := pr.LadderClassFor(decision, streams)
		if natural == LadderClassHW && pr.titleBlocked(req.MediaID) {
			// The title's GPU route is blocked: it IS a software stream now, the
			// ladder above already has the CPU shape (routeFor), so admission
			// prices the CPU pool and never offers the GPU as a fallback.
			natural = softwareClassFor(in.Route)
		}
		// The spill ladder (GPU pool full, CPU pool free) is the same request
		// shaped for the CPU route - every delivery cap included. It used to be
		// the bare producer shape, so a spilled viewer lost the screen, level and
		// bitrate caps the master had negotiated.
		softwareShape := func() []transcoding.HLSProfile {
			if pr.Selector == nil || pr.Selector.Software() == nil {
				return nil
			}
			sw := in
			sw.Route = spillRoute(in.Route, decision)
			return pr.ladderBuilder().Build(sw)
		}
		var fallback func() []int
		if natural == LadderClassHW && pr.Selector != nil {
			fallback = func() []int { return profileHeights(softwareShape()) }
		}
		srcClass := transcoding.SourceClass{
			Codec:  SourceVideoCodec(streams),
			Height: SourceVideoHeight(streams),
		}
		g := pr.AdmissionRoute(pr.PinKey(req, decision), RouteAdmission{
			Natural:         natural,
			Heights:         profileHeights(laddered),
			Src:             srcClass,
			Route:           in.Route.CostKey(),
			SourceFPS:       SourceFrameRate(streams),
			FallbackHeights: fallback,
			FallbackRoute:   spillRoute(in.Route, decision).CostKey(),
		})
		if g.OK {
			if g.Class != natural && g.Class == LadderClassSoftware {
				if swLadder := softwareShape(); len(swLadder) > 0 {
					laddered = swLadder
				}
			}
			if g.Ceiling > 0 {
				laddered = transcoding.CapLadderToHeight(laddered, g.Ceiling)
			}
		}
	}
	// Freeze this shaped ladder on the admission pin (created by AdmissionRoute
	// just above) so every later manifest-/segment-time call reads it verbatim.
	// No-op when no pin was created (nothing fit ⇒ refused, or admission unwired):
	// FreezeLadder finds no pin and does nothing, and the next call re-derives.
	if streamKey != "" && pr.AdmissionFreezeLadder != nil {
		pr.AdmissionFreezeLadder(streamKey, laddered)
	}
	return laddered
}

// profileHeights lists the heights of a shaped ladder, in order.
// videoBitrateCeilingKbps is the tightest of the request cap, the profile cap,
// a bitrate_exceeded decision and the server setting; 0 = follow the source.
func (pr *PlanResolver) videoBitrateCeilingKbps(req PlaybackRequest, decision transcoding.PlaybackDecision) int64 {
	var ceiling int64
	tighten := func(v int64) {
		if v > 0 && (ceiling == 0 || v < ceiling) {
			ceiling = v
		}
	}
	tighten(int64(req.MaxBitrateKbps))
	tighten(int64(req.Prefs.EffectiveMaxBitrateKbps()))
	if slices.Contains(decision.TranscodeReasons, "bitrate_exceeded") {
		tighten(decision.VideoBitrate)
	}
	if pr.MaxVideoBitrateKbps != nil {
		tighten(int64(pr.MaxVideoBitrateKbps()))
	}
	return ceiling
}

func profileHeights(profiles []transcoding.HLSProfile) []int {
	out := make([]int, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, p.Height)
	}
	return out
}

// LadderClassFor derives, from the request alone, the LadderClass this
// stream's session will be billed as. It MUST agree with LadderClassOf on the
// fingerprint the same request builds - the producers charge LadderClassOf(fp)
// at spawn, so a disagreement here makes admission price one resource pool
// while the spawn consumes another. TestLadderClassFor_AgreesWithFingerprint
// locks the equivalence.
func (pr *PlanResolver) LadderClassFor(decision transcoding.PlaybackDecision, streams []*library.MediaStream) LadderClass {
	ec, accel, _, _, usable, _ := pr.EffectiveVideoCodec(decision, streams)
	switch {
	case usable && accel != "":
		return LadderClassHW
	case strings.EqualFold(ec, "av1"):
		return LadderClassSoftwareAV1
	case slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed"):
		return LadderClassSoftwareHDR
	default:
		return LadderClassSoftware
	}
}

// spillRoute is the CPU route a hardware session falls back to: the same
// codec the software encoder will carry (AV1 rides hardware only; an
// auto-upgraded HEVC goes back to H.264 - Fingerprint applies the same rule),
// no GPU, the tone-map in software.
func spillRoute(hw transcoding.EncodeRoute, decision transcoding.PlaybackDecision) transcoding.EncodeRoute {
	r := hw
	r.HWUsable, r.Accel, r.SoftwareDecode, r.HWTonemap = false, "", false, false
	if strings.EqualFold(r.Codec, "av1") || (strings.EqualFold(r.Codec, "hevc") && !strings.EqualFold(decision.VideoCodec, "hevc")) {
		r.Codec = "h264"
	}
	return r
}

// routeFor is encodeRoute with the per-title GPU block applied: a blocked
// title is shaped for the CPU route exactly like an admission spill.
func (pr *PlanResolver) routeFor(req PlaybackRequest, decision transcoding.PlaybackDecision, streams []*library.MediaStream, isCopy bool) transcoding.EncodeRoute {
	route := pr.encodeRoute(decision, streams, isCopy)
	if route.HWUsable && pr.titleBlocked(req.MediaID) {
		return spillRoute(route, decision)
	}
	return route
}

// softwareClassFor is the admission class of a CPU route (the spill's, the
// blocked title's): the same precedence LadderClassOf applies to a fingerprint.
func softwareClassFor(route transcoding.EncodeRoute) LadderClass {
	switch {
	case strings.EqualFold(route.Codec, "av1"):
		return LadderClassSoftwareAV1
	case route.Tonemap:
		return LadderClassSoftwareHDR
	default:
		return LadderClassSoftware
	}
}

func (pr *PlanResolver) ladderBuilder() transcoding.LadderBuilder {
	if pr.Ladder != nil {
		return pr.Ladder
	}
	return transcoding.StaticLadderBuilder{}
}

// complexityOf reads the title's persisted complexity (class + metrics, the
// cost model's input), through the short-TTL cache on the segment hot path.
// An unclassified title resolves to the film_normal class and no metrics.
func (pr *PlanResolver) complexityOf(ctx context.Context, mediaID string) transcoding.ComplexityVector {
	entry := transcoding.ComplexityEntry{Class: transcoding.ClassFilmNormal}
	if pr.ComplexityRepo == nil || mediaID == "" {
		return transcoding.ComplexityVector{Class: entry.Class, Metrics: entry.Metrics}
	}
	if pr.ClassLookup != nil {
		if e, ok := pr.ClassLookup.Get(mediaID); ok {
			return transcoding.ComplexityVector{Class: e.Class, Metrics: e.Metrics}
		}
	}
	if e, found, err := pr.ComplexityRepo.Get(ctx, mediaID); err == nil && found {
		if e.Class.IsValid() {
			entry.Class = e.Class
		}
		entry.Metrics = e.Metrics
	}
	if pr.ClassLookup != nil {
		// Cache the RESOLVED entry (default included) so an unclassified
		// title doesn't re-query on every segment either.
		pr.ClassLookup.Set(mediaID, entry, pr.ClassLookupTTL)
	}
	return transcoding.ComplexityVector{Class: entry.Class, Metrics: entry.Metrics}
}

// QualityPolicy is the rate-control policy the encoder plans read.
func (pr *PlanResolver) QualityPolicy() transcoding.QualityPolicy {
	if pr.Quality != nil {
		return pr.Quality
	}
	return transcoding.StaticQualityPolicy{}
}

// sourceFacts projects the request's streams onto the ladder engine's input.
func (pr *PlanResolver) sourceFacts(req PlaybackRequest) transcoding.SourceFacts {
	d := SourceDimsOf(req.Streams)
	return transcoding.SourceFacts{
		Width: d.Width, Height: d.Height, Codec: d.Codec, TenBit: d.TenBit,
		HDRRange: d.HDRRange, Telecine: d.Telecine, BitrateKbps: req.SourceBitrateKbps,
		FPS: SourceFrameRate(req.Streams),
	}
}

// encodeRoute is the pipeline the ladder is shaped for: the effective codec
// and the GPU/CPU split EffectiveVideoCodec resolves (the same answer the
// fingerprint and the producer selector give), plus the tone-map placement.
func (pr *PlanResolver) encodeRoute(decision transcoding.PlaybackDecision, streams []*library.MediaStream, isCopy bool) transcoding.EncodeRoute {
	codec, accel, _, _, usable, softwareDecode := pr.EffectiveVideoCodec(decision, streams)
	tonemap := slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed")
	return transcoding.EncodeRoute{
		Codec:          codec,
		Accel:          transcoding.HWAccelType(accel),
		HWUsable:       usable && accel != "",
		SoftwareDecode: softwareDecode,
		Tonemap:        tonemap,
		HWTonemap:      tonemap && pr.HWTonemapWanted(decision, SourceTranscodeVideoRange(streams)),
		Copy:           isCopy,
		HDRPassthrough: usable && accel != "" && pr.HDRPassthroughWanted(decision, streams),
	}
}

// deliveryConstraints gathers every cap the request, the operator, the QoE
// feedback and the host put on the ladder.
func (pr *PlanResolver) deliveryConstraints(req PlaybackRequest, decision transcoding.PlaybackDecision) transcoding.DeliveryConstraints {
	dc := transcoding.DeliveryConstraints{
		ScreenHeight:            req.ScreenHeight,
		MaxHeight:               req.Client.MaxHeight,
		MaxH264Level:            req.Client.MaxH264Level,
		VideoBitrateCeilingKbps: pr.videoBitrateCeilingKbps(req, decision),
		Capability:              pr.capabilityOrFallback(),
	}
	if pr.CapOverride != nil {
		if ov, ok := pr.CapOverride(SourceVideoCodec(req.Streams), SourceVideoHeight(req.Streams)); ok {
			dc.QoEMaxHeight = ov.MaxHeight
		}
	}
	if pr.AV1MaxHeight != nil {
		dc.AV1MaxHeight = pr.AV1MaxHeight()
	}
	return dc
}

// Fingerprint assembles the DecisionFingerprint - the cache-key object - from
// the resolved (decision, profiles) pair. Must be byte-identical between the
// manifest-time and segment-time resolutions of the same request shape, or
// the cache lookup misses on every segment.
func (pr *PlanResolver) Fingerprint(ctx context.Context, req PlaybackRequest, decision transcoding.PlaybackDecision, profiles []transcoding.HLSProfile) DecisionFingerprint {
	streams := req.Streams
	audioTrack := req.AudioTrack
	subBurnIn := req.SubtitleBurnInTrack

	// When the audio-rendition feature is on, the audio is served as a SEPARATE
	// HLS rendition (audio_T.m3u8 / audio_T_*.m4s) and the video segments MUST be
	// video-only. Otherwise stream_0 keeps muxing the audio track while the master
	// also advertises the separate audio group with DEFAULT=YES - hls.js then
	// appends audio twice (muxed-in-stream_0 + audio_0), which throws
	// bufferAppendError on any seek. Signalling "no audio" with track -1 reuses the
	// tested video-only ladder (hasAudio) and forks the cache (atrack=-1 in Hash())
	// so muxed and video-only segments for the same media never alias.
	videoAudioTrack := audioTrack
	// Same video-only requirement for the wasm-audio path: the AC-3/E-AC-3 audio
	// is served as a SEPARATE rendition (mkvgo od_audio* or ffmpeg audio_*) and
	// decoded client-side, so the ffmpeg video segments must carry no audio track
	// (atrack=-1 forks the cache off the muxed ladder). Harmless when the video is
	// served by mkvgo instead (this ffmpeg fingerprint is then unused).
	//
	// GATED ON THE TRACK CODEC, like the master and the audio handler: since
	// wasm ON, the cast receiver rides ?wasmAudio=1 on EVERY cast, and for a
	// plain-AAC track the server refuses the wasm master and serves the NORMAL
	// muxed one - stripping the audio here anyway shipped audio-free segments
	// under a master advertising audio (CAF 104/905, prod 2026-07-29).
	// Same predicate as the master handler and the decision: the codec alone is
	// not the condition, the channel gate is part of it.
	wasmAudioApplies := req.WasmAudio && WasmAudioApplies(streams, audioTrack, req.Prefs.MaxAudioChannels)
	renditionAudio := pr.AudioRenditionEnabled && !SourceTelecine(streams) &&
		(decision.VideoDecision == "copy" || (decision.VideoDecision == "transcode" && pr.TranscodeAudioRendition))
	if renditionAudio || wasmAudioApplies {
		videoAudioTrack = -1
	}

	fpProfiles := make([]ProfileFingerprint, len(profiles))
	for i, p := range profiles {
		// Single, reflection-guarded projection (TestProfileFingerprintOf_CopiesEveryField).
		fpProfiles[i] = ProfileFingerprintOf(p)
	}
	// Each rung's rate control, from the policy the ladder was shaped under.
	route := pr.routeFor(req, decision, streams, pr.IsCopy(decision, streams, req.SourceBytes))
	quality := make([]transcoding.QualityTarget, len(profiles))
	for i, p := range profiles {
		quality[i] = pr.QualityPolicy().TargetFor(p, route)
	}

	hwTonemap := pr.HWTonemapWanted(decision, SourceTranscodeVideoRange(streams))
	// Capability gate: AV1 falls back to H.264 when the host can't encode it (no
	// HW AV1, no admin opt-in). videoCodec is what the segments actually carry -
	// the cache key MUST use it, not the raw decision codec.
	videoCodec, hwAccel, hwDevice, hwFeatures, hwUsable, hwSoftwareDecode := pr.EffectiveVideoCodec(decision, streams)
	// Admission spill: a hw stream pinned to the software route (GPU pool was
	// full) must be FINGERPRINTED as software - the fingerprint describes the
	// bytes the software producer will make, and LadderClassOf(fp) is what the
	// spawn bills (CPU pool, not GPU). AV1 rides hardware only, so a spilled
	// AV1 stream carries the H.264 the software encoder will actually produce.
	// A title whose GPU route just failed is spilled the same way, by the
	// per-title block instead of the pool.
	if hwUsable && pr.spilledToSoftware(req, decision) {
		hwAccel, hwDevice, hwFeatures, hwUsable, hwSoftwareDecode = "", "", nil, false, false
		if strings.EqualFold(videoCodec, "av1") {
			videoCodec = "h264"
		}
	}
	// The HDR passthrough only exists on the GPU: a spill to software tone-maps.
	hdrPassWanted := pr.HDRPassthroughWanted(decision, streams)
	if !hwUsable && strings.EqualFold(videoCodec, "hevc") && decision.VideoCodec != "hevc" {
		videoCodec = "h264"
	}
	normalize := pr.AudioNormalize != nil && pr.AudioNormalize(ctx)
	srcAudio := SourceAudioCodec(streams, audioTrack)
	audioCopy := AudioCopyEligible(decision, normalize, len(fpProfiles), srcAudio, req.Client)
	fp := DecisionFingerprint{
		MediaID:    req.MediaID,
		SourcePath: req.SourcePath,
		VideoCodec: videoCodec,
		// The SOURCE codec (excluded from Hash - derivable from MediaID): the
		// copy builder tags the fMP4 sample entry from it (-tag:v hvc1).
		SourceVideoCodec: SourceVideoCodec(streams),
		// The decode workload admission prices (excluded from Hash, as the codec).
		SourceVideoHeight: SourceVideoHeight(streams),
		SourceFPS:         SourceFrameRate(streams),
		// Legacy-codec corrections (B3): idet content verdict + anamorphic SAR,
		// both hashed conditionally so progressive/square titles keep their
		// cache. No-ops on modern progressive sources.
		DeinterlaceAll: SourceDeinterlaceAll(streams),
		Telecine:       SourceTelecine(streams),
		SourceSAR:      SourceSAR(streams),
		// The codec the segments actually carry (source codec on passthrough).
		// The builder only reads it on the transcode branch; here it keeps the
		// hash truthful per served bytes.
		AudioCodec:          ServedAudioCodec(audioCopy, srcAudio, decision),
		AudioTrack:          videoAudioTrack,
		SubtitleBurnInTrack: subBurnIn,
		SubtitleBurnInCodec: SourceSubtitleCodec(streams, subBurnIn),
		// HDR->SDR tonemap is needed whenever the decision flagged it (HDR source +
		// non-HDR client, or Dolby Vision). Baked into the fingerprint so it (a)
		// enters the cache hash - tonemapped vs not are different bytes - and (b)
		// reaches the orchestrator/builder, which has no source streams to detect it.
		HDRTonemap:       slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed") || (hdrPassWanted && !hwUsable),
		HDRPassthrough:   hdrPassWanted && hwUsable,
		HDRTransfer:      strings.ToLower(SourceTranscodeVideoRange(streams)),
		MaxH264Level:     req.Client.MaxH264Level,
		AudioNormalize:   normalize,
		AudioEncoder:     pr.audioEncoderID(),
		MaxAudioChannels: decision.MaxAudioChannels,
		// Audio passthrough eligibility - see AudioCopyEligible (shared with
		// the manifest input so the master CODECS always matches the bytes).
		AudioCopy: audioCopy,
		// User audio resync (ms) - hashed conditionally (non-zero only) and
		// threaded to the builder as a net input shift with the source-delay
		// cancel. Decide already forced audio transcode when non-zero.
		AudioOffsetMs:    req.AudioOffsetMs,
		ForceSidecarSubs: decision.SubtitleDecision == "external",
		UseFMP4:          pr.UseFMP4,
		HWAccel:          hwAccel,
		HWDevice:         hwDevice,
		HWFeatures:       hwFeatures,
		// Decoupled route (software decode + hwupload), only when the HW path is
		// actually taken. Excluded from Hash() like HWAccel.
		HWSoftwareDecode: hwSoftwareDecode && hwUsable,
		// GPU tone-mapping for this request (only when the HW path is actually
		// taken). Excluded from Hash() like HWAccel - a HW- vs software-tonemapped
		// segment is interchangeable in the addressable cache.
		HWTonemap: hwTonemap && hwUsable,
		// GPU subtitle compositing, same contract as HWTonemap: only when the HW
		// path is actually taken, and excluded from Hash() - the same subtitle on
		// the same frames, whoever blended it.
		HWOverlay:      pr.HWOverlayWanted(decision) && hwUsable,
		Profiles:       fpProfiles,
		QualityTargets: quality,
	}
	// Tell the admission shaper which session this pin shaped: while that
	// session holds a limiter slot, the pinned ceiling must not move (see
	// VideoAdmissionShaper - the cache-hash invariant, live edition).
	if pr.AdmissionBind != nil {
		pr.AdmissionBind(pr.PinKey(req, decision), fp.Hash())
	}
	return fp
}

// IsCopy reports whether this request should be served as a video-COPY
// (remux) rung rather than transcoded. Three conditions, all required: the
// copy path is wired (CopyProducer non-nil), the decision did NOT require a
// video transcode, and the source is copy-eligible (SourceCopyVideoCodec).
// When false, the request routes through the selector exactly as before.
func (pr *PlanResolver) IsCopy(decision transcoding.PlaybackDecision, streams []*library.MediaStream, sourceBytes int64) bool {
	if pr.CopyProducer == nil || !VideoCopyEligible(decision, streams) {
		return false
	}
	// Disk guard: the copy rung's segments duplicate ~the source size on disk over
	// the session (video copied 1:1 + a smaller AAC track), and the unpaced copy
	// produces them well ahead of the viewer. If the cache filesystem can't fit it
	// with headroom, fall back to the segment-bounded transcode ladder rather than
	// risk ENOSPC on a small box. Fail-OPEN when the size is unknown or
	// statfs fails - keeps today's behaviour and the mux still fails gracefully
	// downstream. (master- and segment-time both reach this with the same
	// item.File.Size; a free-space flip between them is a rare, bounded 503-retry,
	// not a cache aliasing - the copy hash never collides with a transcode hash.)
	if sourceBytes > 0 && pr.FreeBytes != nil {
		if free, err := pr.FreeBytes(); err == nil && free < sourceBytes+sourceBytes/10 {
			return false
		}
	}
	return true
}

// ProducerFor picks the producer for an already-resolved decision, so the
// producer chosen for a segment matches the one the ladder was shaped for
// (and thus the cache hash). Falls back to the orchestrator when no selector
// is wired (hand-built test handlers); returns nil only when neither is set.
func (pr *PlanResolver) ProducerFor(req PlaybackRequest, decision transcoding.PlaybackDecision, isCopy bool) StreamProducer {
	if pr.Selector != nil {
		if isCopy {
			return pr.CopyProducer
		}
		// Admission spill (pool full) or per-title block (GPU route just failed):
		// the stream spawns on the software producer regardless of the hw gate -
		// the producer must be the one the ladder was shaped for (see ShapeLadder).
		if pr.spilledToSoftware(req, decision) {
			if sw := pr.Selector.Software(); sw != nil {
				return sw
			}
		}
		return pr.Selector.For(decision, SourceDimsOf(req.Streams))
	}
	return pr.Orchestrator
}

// spilledToSoftware reports whether a GPU-eligible request is served from the
// CPU anyway: admission pinned it to the software route (GPU pool full), or
// the title's GPU route is blocked after a failure.
func (pr *PlanResolver) spilledToSoftware(req PlaybackRequest, decision transcoding.PlaybackDecision) bool {
	if pr.titleBlocked(req.MediaID) {
		return true
	}
	if pr.AdmissionPinnedClass != nil {
		if cls, ok := pr.AdmissionPinnedClass(pr.PinKey(req, decision)); ok && cls != LadderClassHW {
			return true
		}
	}
	return false
}

// audioEncoderID is the nil-safe read of the injected probe: an unwired
// resolver keeps the native encoder, which is exactly the pre-feature
// behaviour, bytes and hash alike.
func (pr *PlanResolver) audioEncoderID() string {
	if pr.AudioEncoderID == nil {
		return ""
	}
	return pr.AudioEncoderID()
}
