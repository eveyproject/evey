package httpapi

import (
	"context"
	"errors"
	"fmt"
	domainplayback "github.com/evey-project/evey/internal/domain/playback"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	appmedia "github.com/evey-project/evey/internal/app/media"
	appqoe "github.com/evey-project/evey/internal/app/qoe"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	pkgcache "github.com/evey-project/evey/internal/pkg/cache"
	"github.com/evey-project/evey/internal/pkg/metrics"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type hlsVODServe struct {
	libSvc        MediaStreamsGetter
	libraryAccess LibraryAccessChecker
	parental      ParentalChecker
	// streamTokens mints the next episode's cast token in serveCastNext.
	// nil = cast-next answers 503 (fail closed).
	streamTokens *appmedia.StreamTokenStore
	profilePrefs ProfilePrefsGetter
	// probeSignals records a cost-modelled rung shipping at its advertised
	// ceiling, for the background P3 probe worker. Nil-safe; never runs a probe.
	probeSignals         *apptranscoding.ProbeSignalStore
	audioNormalizeGetter func(ctx context.Context) bool
	// audioEncoderIDGetter: see ServerConfig.AudioEncoderIDGetter.
	audioEncoderIDGetter func() string
	userMaxStreams       UserMaxStreamsGetter
	// subtitlePrewarm fills the subtitle cache in the background from playback
	// start, so the subtitle menu is not the thing that pays for a full read of
	// the container. nil = disabled. See subtitle_prewarm.go.
	subtitlePrewarm *subtitlePrewarmer
	startHints      startPositionHints // master's ?startSec= -> variant pre-warm

	// compositionDelayProbe measures how late a source's video is presented
	// once packaged as MP4 (has_b_frames/fps). Injected so this adapter keeps
	// no ffprobe dependency of its own. nil = no compensation, the pre-fix
	// behaviour. See transcode_audio_sync.go for what it is used for.
	compositionDelayProbe func(ctx context.Context, path string) float64
	// servingRecord: see SessionServingRecord; nil = not wired (tests).
	servingRecord SessionServingRecord
	// frameRateProbe + encoderReorderProbe: the same shift for a TRANSCODED
	// video (the encoder's reorder delay, not the source's).
	frameRateProbe      func(ctx context.Context, path string) float64
	encoderReorderProbe func(ctx context.Context, codec, accel, device string, features []string) (int, bool)

	manifestGen *apptranscoding.ManifestGenerator
	cache       *apptranscoding.SegmentCache

	// selector routes each request to the software or hardware StreamProducer.
	// The software producer is the long-lived, input-paced, single-rendition
	// paced single-encoder model (sane on a weak NAS with hw_accel off); the hardware
	// producer is the multi-session burst-ahead orchestrator (a GPU
	// parallelises rungs cheaply). The selector's For(decision, src) decides -
	// it reuses resolveHW so the manifest, the fingerprint, and the routing
	// never disagree on which path a request takes. Nil in hand-built test
	// handlers, which then fall back to the orchestrator field below.
	selector *apptranscoding.ProducerSelector

	// orchestrator is the hardware StreamProducer (the multi-session rafale
	// machine). Production wires it INSIDE the selector; this field is the
	// no-selector fallback so tests that construct the handler by hand keep a
	// working producer. When the selector is set it takes precedence.
	orchestrator *apptranscoding.SegmentOrchestrator

	// copyProducer serves the H.264 video-COPY (remux) rung when wired (behind
	// EVEY_REMUX_COPY=on): a dedicated SoftwareProducer in copy mode - the
	// paced remux, one `-c:v copy` + AAC session, seek = restart. nil =
	// the copy path is off and every request routes through the selector
	// (transcode) exactly as before. Interface-typed because both producers
	// satisfy it; only copy requests (isCopyRequest) are routed here.
	copyProducer apptranscoding.StreamProducer
	// keyframeWarmer nudges the bounded on-demand index walk. Optional (nil =
	// no nudge); see copyBlockedOnlyByMissingIndex for why it matters here.
	keyframeWarmer func(ctx context.Context, mediaID, path string) bool
	// videoAdmission is the limiter's read-only probe, used to shed a variant
	// playlist before a player commits to a stream. Optional (nil = no early
	// shed; the segment route still gates authoritatively). Takes the resolved
	// fingerprint so the probe prices the very route the spawn will charge.
	videoAdmission func(key string, fp apptranscoding.DecisionFingerprint, topHeight int) bool
	// calibrating reports a calibration benchmark in flight: its encodes pin
	// the host, so a NEW encoded stream would stall and pollute the measure.
	// Such a request is shed with ErrCodeCalibrating (a ~2 min Retry-After)
	// instead of "too many streams". Optional (nil = never calibrating).
	calibrating func() bool
	// admissionRoute asks the admission shaper for this stream's pinned route
	// and ladder ceiling (see profilesForDecision's third cap): a hw stream on
	// a full GPU pool spills to the software route when the CPU pool has room.
	// Optional: nil keeps the natural shape and viewers over budget are shed.
	admissionRoute func(key string, req apptranscoding.RouteAdmission) apptranscoding.RouteGrant
	// admissionPinnedClass reads the class a live pin was granted under, so
	// the fingerprint's hw fields and the spawn's producer follow the pinned
	// route rather than the natural one. Optional.
	admissionPinnedClass func(key string) (apptranscoding.LadderClass, bool)
	// admissionBind reports the fingerprint hash built under a pin so the
	// shaper can freeze the ceiling while that session holds a slot. Optional.
	admissionBind func(key, fingerprintHash string)

	// streamHashes remembers which decision hash each PLAYBACK STREAM (one
	// player instance, keyed by its stream token) currently sits on, so a
	// track/subtitle/offset switch - which re-fingerprints the SAME viewing
	// session - can be told apart from a genuine second viewer. Optional: nil
	// disables the reclaim below entirely (prior behaviour).
	streamHashes *apptranscoding.StreamHashRegistry
	// reclaimHash kills the encoders of a hash no stream is on anymore and
	// returns how many died. Paired with streamHashes: without it the registry
	// would observe supersessions and do nothing. Optional.
	reclaimHash func(hash, reason string) int
	// admissionFrozenLadder / admissionFreezeLadder read-verbatim the shaped ABR
	// ladder EMITTED ONCE for a stream: profilesForDecision freezes its output on
	// the admission pin and every later manifest-/segment-time call reads it back
	// verbatim, so a mid-stream drift of the mutable shaping inputs (complexity
	// class, capability, CMCD/av1 caps) cannot desync the manifest's ladder from
	// the segments' - the cache-hash invariant made structural rather than
	// discipline-held. Both optional (nil ⇒ always re-derive, the prior behaviour).
	admissionFrozenLadder func(key string) ([]domaintranscoding.HLSProfile, bool)
	admissionFreezeLadder func(key string, ladder []domaintranscoding.HLSProfile)

	// onDemand serves the copy rung straight from mkvgo's mp4.PlanHLS -
	// zero-storage, stateless, byte-identical HLS + DASH - when wired (behind
	// transcoding.hls_ondemand). It supersedes copyProducer for video+audio
	// copy-eligible requests whose source packages verbatim (see planHLSEligible
	// / transcode_ondemand.go); everything else falls through to the ffmpeg
	// pipeline unchanged. nil = feature off, no behaviour change.
	onDemand apptranscoding.OnDemandPackager

	// playbackActivity is how this stateless path reports "someone is watching".
	// The packager spawns no ffmpeg session, so without a touch on every served
	// resource the server's whole anti-starvation machinery reads an mkvgo
	// viewer as an idle box and lets background workers fight them for the disk.
	// nil = not wired (no behaviour change).
	playbackActivity *apptranscoding.PlaybackActivity

	// castProgress derives watch progress from segments fetched on a CAST
	// token - the only playback signal once the sender's browser closes
	// (see cast_observe.go). nil = not wired.
	castProgress CastFetchObserver

	// onDemandEncrypt AES-128-encrypts the on-demand rung's media segments
	// (behind transcoding.hls_ondemand_encrypt). Off by default; requires
	// hlsAESSecret. When on, no DASH manifest is emitted (AES-128 is HLS-only).
	onDemandEncrypt bool
	// hlsAESSecret derives per-media AES-128 keys (HMAC(secret, mediaID)); the
	// /hls/{id}/key endpoint serves the derived key to authorised clients.
	hlsAESSecret []byte
	// hlsKeyRotateSegments, when > 0 (transcoding.hls_key_rotation_segments),
	// rotates the AES-128 key every N segments for forward secrecy. Each period
	// gets its own derived key, advertised as key?i=<n> and served by the key
	// endpoint. 0 = single key per media (the default).
	hlsKeyRotateSegments int

	// onDemandConcat serves a gapless multi-episode HLS session (mkvgo's
	// mp4.PlanConcat) when a master request opts in with ?gapless=1 (behind
	// transcoding.hls_ondemand_concat). nil = feature off, no behaviour
	// change - concat is purely additive on top of the single-episode
	// on-demand rung. See transcode_concat.go.
	onDemandConcat apptranscoding.ConcatPackager

	// onDemandTrackTrim (transcoding.hls_ondemand_track_trim) restricts the
	// on-demand master's audio to the single track ?audioTrack=/the profile's
	// language preference would resolve, instead of packaging every audio
	// track - a lighter master.m3u8 for a "MULTI" release with many dubs.
	// Video and subtitles are always kept in full; an explicit ?audioTrack=
	// switch still re-resolves and re-plans normally. Off by default, no
	// behaviour change.
	onDemandTrackTrim bool

	// qoeSink ingests CMCD telemetry from segment requests. Nil when QoE is
	// disabled (tests, dev without postgres). Record is non-blocking.
	qoeSink appqoe.EventSink

	// complexityRepo reads the per-title classification used to pick the
	// ABR ladder. Nil-safe - profilesForDecision falls back to
	// LadderFilmNormal when the repo isn't wired (dev w/o postgres) or
	// the row hasn't been classified yet.
	complexityRepo domaintranscoding.ComplexityRepo

	// capability reports the host's calibrated encode ceiling. Nil-safe:
	// profilesForDecision falls back to the safe 720p floor when the
	// service isn't wired (tests). Returns a fresh snapshot per call so
	// recalibration via the admin endpoint takes effect immediately -
	// no need to restart the server.
	capability func() domaintranscoding.EncodeCapability

	// capOverride is the CMCD-feedback adaptive override service.
	// Returns (override, true) when an active override applies to the
	// source-class bucket - handler uses MIN(benchmark, override) as
	// the effective cap. Nil-safe so tests without the worker still
	// pass.
	capOverride func(sourceCodec string, sourceHeight int) (domaintranscoding.CapOverride, bool)

	// hwAccelEnabled reports whether the hardware-transcode flag
	// (transcoding.hw_accel_enabled) is on. Read dynamically so an admin
	// toggle takes effect without a restart. Nil-safe (tests → software).
	hwAccelEnabled func() bool
	// av1Enabled gates AV1 as an OUTPUT codec (transcoding.av1_enabled). AV1 is
	// hardware-only for streaming, so even with HW the admin must opt in; nil or
	// false ⇒ a profile that prefers AV1 still transcodes to H.264.
	av1Enabled func() bool
	// av1MaxHeight caps the AV1 output ladder (transcoding.av1_max_height, 0 = no
	// cap). Applied to the shaped ladder when AV1 is the effective codec.
	av1MaxHeight func() int

	// hwTonemapEnabled gates GPU HDR->SDR tone-mapping (still requires the host to
	// have probed EncodeCapability.HWTonemap). Default on; EVEY_TRANSCODING_HW_TONEMAP=0
	// forces the software tonemap path. Nil-safe (nil/true → allowed).
	hwTonemapEnabled func() bool
	// hwLowPowerEnabled: admin opt-in for the Intel low-power encoder (transcoding.hw_low_power).
	hwLowPowerEnabled func() bool
	// maxVideoBitrateRemoteKbps: the ceiling for a public-IP client (0 = none);
	// userBitrate: the admin per-user limit, cached per user like the prefs.
	maxVideoBitrateRemoteKbps func() int
	userBitrate               UserBitrateGetter
	userBitrateLookup         *pkgcache.TTLCache[int]
	// maxVideoBitrateKbps: server ceiling on the served video bitrate (0 = source).
	maxVideoBitrateKbps func() int

	// useFMP4 controls the segment container. fMP4 is the project default -
	// segments are ~30% smaller and the codecs string in the master playlist
	// matches what hls.js expects on modern Chromium/Firefox.
	useFMP4 bool

	// audioRenditionEnabled mirrors Cfg.Transcoding.AudioRenditionEnabled.
	// When false (default), the master playlist stays muxed audio+video
	// bit-for-bit and the audio rendition handlers respond 404. When true,
	// the master advertises EXT-X-MEDIA TYPE=AUDIO renditions per source
	// audio track and the dedicated audio pipeline serves them.
	audioRenditionEnabled bool

	// castWasmAudioEnabled gates the wasm-audio path FOR CAST RECEIVERS only
	// (browsers keep it: it is validated there and it is what plays Dolby
	// without a transcode). Default OFF.
	//
	// A low-end operator box died twice mid-film, ~2min30 in, with the sound working and
	// both memory figures we can read flat and tiny (2026-07-30). The decoder
	// measured 3.13x real time on that box, so the path buys it NOTHING anyway -
	// the AAC rendition plays the same film with sound from the first second.
	// Until the death is explained, a TV does not run our wasm decoder, and the
	// operator can flip this back on without a deploy.
	castWasmAudioEnabled bool

	// copyFallbackRung mirrors Cfg.Transcoding.CopyFallbackRungEnabled: the
	// hybrid on-demand master advertises one low transcoded rung next to the
	// copy variant, so a player whose throughput collapses can drop to it
	// instead of freezing (the copy path has no other rung below the source
	// bitrate). Advisory-only until a player actually fetches that rung's
	// segments - only then does an encode session spawn.
	copyFallbackRung bool

	// audio handles the audio rendition routes (audio_T.m3u8 / init /
	// segment). Routed through the same /hls/{file} entry point so the
	// relative URLs the master playlist emits resolve correctly. Nil-safe
	// - dispatched paths 404 when unset (tests, no AudioOrch wired).
	audio *audioServe

	// segmentWaitTimeout caps how long the segment handler will block waiting
	// for ffmpeg to land a segment. Larger than expected encode time but
	// smaller than the user's patience for a 504. Default 15s.
	segmentWaitTimeout time.Duration

	// serverLang returns the server-level metadata language (cfg.Metadata.Language,
	// e.g. "en"). Used as the lowest-priority fallback for default audio track
	// resolution when neither the profile's audio_languages nor its locale
	// match any stream. Nil-safe - falls back to "en".
	serverLang func() string

	// mediaLookup / classLookup are SHORT-TTL caches for the per-segment hot
	// path: every segment request used to pay three DB round-trips
	// (GetMediaItem + GetMediaStreams + complexity Get) for values that are
	// stable across a playback burst. 10 s of staleness is harmless - the
	// access/parental checks still run live per request, and a mid-playback
	// metadata change converging one burst later is the same tolerance the
	// manifest/segment pair already has. Cached values are SHARED between
	// requests: treat them as immutable. Nil-safe (hand-built test handlers
	// keep the direct fetch path).
	mediaLookup *pkgcache.TTLCache[cachedMediaLookup]
	classLookup *pkgcache.TTLCache[domaintranscoding.ComplexityEntry]
	// prefsLookup caches the per-profile transcoding preferences (up to 2 DB
	// round-trips per load) for the same burst window: during playback every
	// segment request re-resolves the plan, and the prefs behind it are
	// stable across a burst. Same 10 s staleness tolerance as mediaLookup -
	// a preference change converges one burst later. Keyed by profile ID;
	// cached values are SHARED between requests: treat them as immutable
	// (consumers mutate their own value copy only).
	prefsLookup *pkgcache.TTLCache[apptranscoding.TranscodingPreferences]
}

// profilePrefsFor resolves the profile's transcoding preferences through the
// short-TTL cache. Nil-safe (hand-built test handlers keep the direct fetch);
// an empty profile ID short-circuits to defaults without touching the cache.
// userPrefs loads the caller's preferences, nil claims included (an
// unauthenticated context yields the defaults rather than a panic).
func (h *hlsVODServe) userPrefs(ctx context.Context) apptranscoding.TranscodingPreferences {
	profileID := ""
	if claims := HTTPClaimsFromContext(ctx); claims != nil {
		profileID = claims.ProfileID
	}
	return h.profilePrefsFor(ctx, profileID)
}

// subtitlePrewarmLang is the language to extract FIRST: the caller's own
// subtitle preference when they have one, else their audio preference, which is
// the language whose forced track a file usually carries. Empty means "file
// order", which is what the subtitle menu shows anyway.
func (h *hlsVODServe) subtitlePrewarmLang(ctx context.Context) string {
	claims := HTTPClaimsFromContext(ctx)
	if claims == nil || claims.ProfileID == "" || h.profilePrefs == nil {
		return ""
	}
	prefs, err := h.profilePrefs.GetProfilePreferences(ctx, claims.ProfileID)
	if err != nil || prefs == nil {
		return ""
	}
	for _, key := range []string{"subtitle_languages", "audio_languages"} {
		if v := strings.TrimSpace(strings.Split(prefs[key], ",")[0]); v != "" {
			return v
		}
	}
	return ""
}

func (h *hlsVODServe) profilePrefsFor(ctx context.Context, profileID string) apptranscoding.TranscodingPreferences {
	if profileID == "" || h.prefsLookup == nil {
		return loadProfilePrefs(ctx, h.profilePrefs, profileID)
	}
	if p, ok := h.prefsLookup.Get(profileID); ok {
		return p
	}
	p := loadProfilePrefs(ctx, h.profilePrefs, profileID)
	h.prefsLookup.Set(profileID, p, mediaLookupTTL)
	return p
}

// cachedMediaLookup is one mediaLookup entry: the item and its streams as
// resolved together (never cache one without the other - the fingerprint
// derives from both).
type cachedMediaLookup struct {
	item    *domainlibrary.MediaItem
	streams []*domainlibrary.MediaStream
}

const (
	mediaLookupTTL = 10 * time.Second
	mediaLookupMax = 256
)

// ceilingHitContainerMargin lifts the P3 ceiling-hit threshold above the sum of
// the rung's video + audio bitrates to absorb the segment container overhead
// (fMP4/TS muxing), so overhead alone never counts as the video encoder pinning
// its ceiling. See serveSegment.
const ceilingHitContainerMargin = 1.05

// vodSegmentNameRE parses the on-the-wire segment file name and returns the
// (variantIdx, segmentIdx, isInit, ext) tuple. Strict: any other shape 404s.
// The index is \d+ (not \d{3}): ffmpeg's hls_segment_filename only zero-pads to
// a 3-digit MINIMUM, so segment 1000+ (≥ ~50 min at 3s/seg) is 4 digits - a
// \d{3} cap 404'd all deep-resume / long-playback segments.
var vodSegmentNameRE = regexp.MustCompile(`^stream_(\d+)(?:_(\d+)|_init)\.(ts|m4s|mp4)$`)

// vodVariantNameRE matches the sub-playlist file name.
var vodVariantNameRE = regexp.MustCompile(`^stream_(\d+)\.m3u8$`)

// audioPlaylistNameRE matches the audio rendition sub-playlist
// (audio_T.m3u8) the master references.
var audioPlaylistNameRE = regexp.MustCompile(`^audio_(\d+)\.m3u8$`)

// audioSegmentNameRE matches an audio rendition segment file
// (audio_T_NNN.m4s) or its init segment (audio_T_init.mp4). Index is \d+ for
// the same reason as vodSegmentNameRE: deep offsets exceed 3 digits.
var audioSegmentNameRE = regexp.MustCompile(`^audio_(\d+)(?:_(\d+)|_init)\.(m4s|mp4)$`)

// serveHTTP is the single dispatch entry point for `/hls/{file}`. It splits
// master vs sub-playlist vs segment by file name and delegates.
func (h *hlsVODServe) serveHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.transcode.HLSVOD")
	defer span.End()
	r = r.WithContext(ctx)

	mediaID := r.PathValue("id")
	file := r.PathValue("file")
	span.SetAttributes(attribute.String("media.id", mediaID), attribute.String("hls.file", file))

	// commonAuth resolves the media item (and its streams) on its way through
	// the access checks. Passing them down to the per-file handlers prevents
	// each downstream function from re-fetching - one master + one variant +
	// one segment request used to fan out to 9 DB hits, now down to 3.
	item, streams, ok := h.commonAuth(w, r, mediaID, span)
	if !ok {
		return
	}

	switch {
	case file == "master.m3u8":
		h.serveMaster(w, r, mediaID, item, streams, span)
	case file == "key":
		// AES-128 key for the on-demand encrypted rung (EXT-X-KEY URI="key").
		// Access already enforced by commonAuth above.
		h.serveHLSKey(w, r, mediaID, span)
	case file == "bitrate-test":
		// Opaque bytes the cast receiver times to measure its own stack
		// before asking for a fitted master (?maxKbps=).
		h.serveBitrateTest(w, r)
	case strings.HasPrefix(file, concatPrefix):
		// Gapless multi-episode session (mp4.PlanConcat): every prefixed
		// sub-resource (cc_playlist.m3u8, cc_p0/seg00001.m4s, cc_audio1.m3u8...)
		// the concatenated master references. 404 when the feature is off / the
		// chain no longer resolves.
		h.serveConcatResource(w, r, mediaID, file, item, span)
	case file == "manifest.mpd" || strings.HasPrefix(file, onDemandPrefix):
		// On-demand copy rung (mp4.PlanHLS): the DASH manifest entry point and
		// every prefixed sub-resource (od_playlist.m3u8, od_seg00001.m4s,
		// od_audio1.m3u8, od_init.mp4, od_sub1.vtt …) the plan's manifests
		// reference. 404 when the feature is off / the source can't be packaged.
		h.serveOnDemandResource(w, r, mediaID, file, item, streams, span)
	case vodVariantNameRE.MatchString(file):
		h.serveVariant(w, r, mediaID, file, item, streams, span)
	case vodSegmentNameRE.MatchString(file):
		h.serveSegment(w, r, mediaID, file, item, streams, span)
	case audioPlaylistNameRE.MatchString(file):
		h.dispatchAudioPlaylist(w, r, mediaID, file, item, streams, span)
	case audioSegmentNameRE.MatchString(file):
		h.dispatchAudioSegment(w, r, mediaID, file, item, streams, span)
	default:
		span.SetStatus(codes.Error, "unknown hls path")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
	}
}

// dispatchAudioPlaylist + dispatchAudioSegment are tiny adapters: they parse
// the file name the master playlist emitted (audio_T.m3u8 / audio_T_NNN.m4s
// / audio_T_init.mp4) and forward to the audio handler with explicit ints.
// We can't register `/api/v1/media/{id}/audio_{trackIdx}.m3u8` as a separate
// route - Go 1.22+ ServeMux refuses wildcards that don't cover a whole
// path segment - and we don't want to either, because the URLs the manifest
// emits are *relative* to /hls/master.m3u8 and thus resolve into /hls/{file}.
func (h *hlsVODServe) dispatchAudioPlaylist(w http.ResponseWriter, r *http.Request, mediaID, file string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, span trace.Span) {
	if h.audio == nil {
		span.SetStatus(codes.Error, "audio rendition not wired")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	m := audioPlaylistNameRE.FindStringSubmatch(file)
	trackIdx := atoiIndex(m[1])
	h.audio.servePlaylistFile(w, r, mediaID, trackIdx, h.videoCompositionDelaySec(r, mediaID, item, streams), h.audioRenditionCopyCodec(r, item, streams, trackIdx))
}

func (h *hlsVODServe) dispatchAudioSegment(w http.ResponseWriter, r *http.Request, mediaID, file string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, span trace.Span) {
	if h.audio == nil {
		span.SetStatus(codes.Error, "audio rendition not wired")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	m := audioSegmentNameRE.FindStringSubmatch(file)
	trackIdx := atoiIndex(m[1])
	if m[2] == "" {
		// _init.mp4
		h.audio.serveInitFile(w, r, mediaID, trackIdx, h.videoCompositionDelaySec(r, mediaID, item, streams), h.audioRenditionCopyCodec(r, item, streams, trackIdx))
		return
	}
	segIdx := atoiIndex(m[2])
	h.audio.serveSegmentFile(w, r, mediaID, trackIdx, segIdx, h.videoCompositionDelaySec(r, mediaID, item, streams), h.audioRenditionCopyCodec(r, item, streams, trackIdx))
}

// commonAuth runs auth + access + parental and resolves the media item +
// streams. Returns (item, streams, true) to continue, (nil, nil, false) after
// writing an error response to abort. Resolving the media item here lets the
// per-file handlers avoid re-fetching downstream.
func (h *hlsVODServe) commonAuth(w http.ResponseWriter, r *http.Request, mediaID string, span trace.Span) (*domainlibrary.MediaItem, []*domainlibrary.MediaStream, bool) {
	if st, ok := StreamTokenFromContext(r.Context()); ok {
		if st.MediaItemID != shared.MediaItemID(mediaID) {
			span.SetStatus(codes.Error, "stream token media mismatch")
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return nil, nil, false
		}
	}
	claims := HTTPClaimsFromContext(r.Context())

	// Hot-path cache: segment requests arrive every few seconds during
	// playback and each used to re-fetch the item + streams from the DB. The
	// ACCESS checks below always run live - only the media lookup is cached.
	var item *domainlibrary.MediaItem
	var streams []*domainlibrary.MediaStream
	cached := false
	if h.mediaLookup != nil {
		if c, ok := h.mediaLookup.Get(mediaID); ok {
			item, streams, cached = c.item, c.streams, true
		}
	}
	if !cached {
		var err error
		item, err = h.libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil || item.File == nil || item.File.Path == "" {
			if err != nil {
				span.RecordError(err)
			}
			span.SetStatus(codes.Error, "media item not found")
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return nil, nil, false
		}
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		span.SetStatus(codes.Error, "library access denied")
		return nil, nil, false
	}
	if !checkParental(r.Context(), w, claims, item, h.parental) {
		span.SetStatus(codes.Error, "parental restricted")
		return nil, nil, false
	}
	if !checkPlayable(w, item) {
		span.SetStatus(codes.Error, "file needs remux")
		return nil, nil, false
	}
	if !cached {
		var err error
		streams, err = h.libSvc.GetMediaStreams(r.Context(), mediaID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "streams resolve failed")
			logInternal(r.Context(), "hls: get media streams", err)
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return nil, nil, false
		}
		if h.mediaLookup != nil {
			h.mediaLookup.Set(mediaID, cachedMediaLookup{item: item, streams: streams}, mediaLookupTTL)
		}
	}
	return item, streams, true
}

func (h *hlsVODServe) serveMaster(w http.ResponseWriter, r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, span trace.Span) {
	// Catches the startup CMCD payload (?CMCD=su,sid=...,ot=m) - the first
	// thing every player sends. Useful for "time to first frame" admin views.
	h.ingestCMCD(r, mediaID, 0, 0, false, true, false)

	if seg, ok := startSegmentHint(r); ok {
		h.startHints.remember(mediaID, seg, time.Now())
	}

	// Playback is starting: begin filling the subtitle cache. Deliberately
	// here, on the master playlist, because that is the earliest moment the
	// server knows a specific film is about to be watched - and it is a request
	// every client makes exactly once per session. The prewarmer waits for the
	// picture to have its head start before touching the disk, and drops the
	// job rather than delay this response.
	// Not for a browser on a file the packager subtitles: it reads the
	// renditions segment by segment and would never open the cache this full
	// read fills - a full read next to its own playback. The native apps and
	// the cast receiver still fetch /subtitle, so they keep the head start.
	if h.subtitlePrewarm != nil && item != nil && item.File != nil {
		packagerServes := browserUserAgent(r.UserAgent()) && len(h.planSubtitleRenditions(r, mediaID, item, streams)) > 0
		if !packagerServes {
			h.subtitlePrewarm.Enqueue(mediaID, item.File.Path, streams, h.subtitlePrewarmLang(r.Context()))
		}
	}

	// Gapless multi-episode session: only when the request opts in
	// (?gapless=1). Falls through (feature off / not requested / chain too
	// short / incompatible episode) to the normal single-episode paths below,
	// unchanged.
	if h.serveConcatMaster(w, r, mediaID, item, streams, span) {
		return
	}

	// Browser wasm-audio path (?wasmAudio=1 + selected AC-3/E-AC-3 track): a
	// VIDEO-ONLY master whose audio the client decodes itself (ac3go wasm) from an
	// out-of-band rendition. Checked before the plan master so the AC-3 audio
	// stays out of MSE. Falls through when not applicable.
	if h.serveWasmAudioMaster(w, r, mediaID, item, streams, span) {
		return
	}

	// On-demand copy rung: when the request is video+audio copy-eligible and the
	// source packages verbatim, serve the whole presentation from mp4.PlanHLS
	// (stateless, byte-identical, DASH included) instead of the ffmpeg pipeline.
	// Falls through (feature off / not eligible / plan build failed) unchanged.
	if h.tryServePlanMaster(w, r, mediaID, item, streams, span) {
		return
	}

	// From here on the ffmpeg ladder serves this master: resolve the playback
	// plan ONCE - decision, shaped ladder, fingerprint - and let the shed
	// probe and the manifest input read the same resolution.
	rp, err := h.resolvePlan(r, mediaID, item, streams)
	if err != nil {
		h.writeManifestErr(r.Context(), w, span, err, mediaID)
		return
	}

	// Tell a viewer the host is full BEFORE their player parses a ladder it can
	// never fetch - the only signal a native mobile/TV player ever sees. Probed
	// against the CHEAPEST rung: a master is refused only when no quality at
	// all fits, never merely because the tallest one does not.
	if h.shedIfHostFull(w, r, mediaID, item, streams, rp, fingerprintHeights) {
		return
	}

	in, err := h.buildManifestInputWith(r, mediaID, item, streams, rp)
	if err != nil {
		h.writeManifestErr(r.Context(), w, span, err, mediaID)
		return
	}
	// Even under an encoder the text subtitles come from the packager: the
	// transcode timeline is the source's, so the windowed cues line up.
	in.SubtitleRenditions = h.planSubtitleRenditions(r, mediaID, item, streams)
	if !rp.IsCopy {
		// One line and one counter per transcode master: what a copy would have
		// needed (the reason list) is the backlog of the free path.
		cause := apptranscoding.TranscodeCause(rp.Decision, streams)
		metrics.HLSVODTranscodeSessions.WithLabelValues(cause).Inc()
		slog.InfoContext(r.Context(), "hls-vod: transcode master served",
			slog.String("media_id", mediaID),
			slog.String("cause", cause),
			slog.String("reasons", strings.Join(rp.Decision.TranscodeReasons, ",")),
			slog.String("video_codec", rp.Plan.VideoCodec),
			slog.String("hw_accel", rp.Plan.HWAccel))
	}

	out, err := h.manifestGen.Master(in)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "manifest master")
		logInternal(r.Context(), "hls: master manifest", err)
		http.Error(w, `{"message":"manifest error"}`, http.StatusInternalServerError)
		return
	}

	{
		p := domainplayback.ServingPath{VideoDecision: "transcode", VideoEngine: "ffmpeg", VideoBackend: videoBackendLabel(rp.Plan.HWAccel), AudioDecision: "transcode", AudioEngine: "ffmpeg"}
		if rp.IsCopy {
			p.VideoDecision, p.VideoBackend = "copy", ""
		}
		if rp.FP.AudioCopy {
			p.AudioDecision = "copy"
		}
		h.recordServing(r, mediaID, p)
	}

	setCORSHeaders(w, r, "")
	// The ffmpeg pipeline family serves this master (its ladder may mix copy
	// and transcode rungs - segments carry the precise value). Counterpart of
	// the mkvgo tag in servePlanResourceData; lets the load test and a curl
	// tell the serving machinery at a glance.
	w.Header().Set("X-Evey-Served-By", "ffmpeg")
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(out))

	// The decision hash rides the log line: it names the ffmpeg sessions and
	// the cache directory this master's segments resolve to. During the
	// 2026-07-15 incident, correlating "master served, 3 profiles" with the
	// sessions that actually ran took inference - one field ends that. Same
	// value the manifest just sealed into its URIs.
	slog.Info("hls-vod: master served",
		slog.String("media_id", mediaID),
		slog.Int("profiles", len(in.Profiles)),
		slog.Int("segments_per_variant", in.SegmentCount()),
		slog.String("fp", in.FingerprintHash),
	)
}

// browserUserAgent: a web browser, as opposed to the native apps (EveyMobile/,
// EveyTV/) and the cast receiver (CrKey), which all present a Mozilla/ UA too.
func browserUserAgent(ua string) bool {
	return strings.Contains(ua, "Mozilla/") && !strings.Contains(ua, "CrKey") &&
		!strings.Contains(ua, "EveyMobile") && !strings.Contains(ua, "EveyTV")
}

// recordServing tells the (profile, media) play session what this master
// hands the player. The profile comes from the stream token, so a cast
// receiver's master lands on the same session as the phone that started it.
func (h *hlsVODServe) recordServing(r *http.Request, mediaID string, p domainplayback.ServingPath) {
	if h.servingRecord == nil {
		return
	}
	claims := HTTPClaimsFromContext(r.Context())
	if claims == nil || claims.ProfileID == "" {
		return
	}
	h.servingRecord(r.Context(), claims.ProfileID, mediaID, p)
}

// videoBackendLabel names the encoder backend of a transcode rung.
func videoBackendLabel(hwAccel string) string {
	if hwAccel == "" {
		return "software"
	}
	return hwAccel
}

// refuseStaleFP compares the fp the manifest sealed into this URL against the
// decision hash of THIS moment. A mismatch means the ladder reshaped since the
// player fetched its master (capability flap, admission pin gone cold): the
// URL's index now names a DIFFERENT rendition, and serving it would hand the
// decoder a stranger's bytes - the 2026-07-15 rainbow screen. Refuse with 410;
// the player refetches the master and rebuilds coherently. A URL with no fp
// (pre-feature client, cached link) is served as before: degrade open.
//
// The WARN is as valuable as the refusal: it is the direct detector of a
// ladder that moved under a live player, which previously left no trace.
func (h *hlsVODServe) refuseStaleFP(w http.ResponseWriter, r *http.Request, currentFP, mediaID, file string) bool {
	sealed := r.URL.Query().Get("fp")
	if sealed == "" || currentFP == "" || sealed == currentFP {
		return false
	}
	slog.WarnContext(r.Context(), "hls-vod: stale rendition URL refused (the ladder moved under a live player)",
		slog.String("media_id", mediaID),
		slog.String("file", file),
		slog.String("sealed_fp", sealed),
		slog.String("current_fp", currentFP),
	)
	w.Header().Set("X-Evey-Error", "stale_rendition")
	http.Error(w, `{"message":"this URL belongs to a reshaped playback session; refetch the master playlist"}`, http.StatusGone)
	return true
}

// atoiIndex parses a numeric URL segment that has already matched a `\d+`
// route pattern, so the only way it can fail is OVERFLOW - a caller sending
// stream_99999999999999999999.m3u8. strconv.Atoi returns its range error along
// with 0, and every call site here used to discard it, so an overflowing index
// silently became index 0: the bounds check downstream passed and the server
// served rendition 0 instead of answering 404.
//
// Returns -1 on failure so a bounds check rejects it, whatever the caller's
// upper limit is.
func atoiIndex(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return n
}

func (h *hlsVODServe) serveVariant(w http.ResponseWriter, r *http.Request, mediaID, file string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, span trace.Span) {
	m := vodVariantNameRE.FindStringSubmatch(file)
	profileIdx := atoiIndex(m[1])

	// Catches CMCD on variant playlist fetches (typical when the player ABR
	// switches renditions). Helps the dashboard count switch frequency.
	h.ingestCMCD(r, mediaID, profileIdx, 0, false, false, true)

	// One plan resolution serves everything below: the stale-URL seal, the
	// copy-rung gate, the producer pick, the shed probe and the pre-warm all
	// read the same decision/fingerprint by construction.
	rp, err := h.resolvePlan(r, mediaID, item, streams)
	if err != nil {
		h.writeManifestErr(r.Context(), w, span, err, mediaID)
		return
	}

	in, err := h.buildManifestInputWith(r, mediaID, item, streams, rp)
	if err != nil {
		h.writeManifestErr(r.Context(), w, span, err, mediaID)
		return
	}
	if h.refuseStaleFP(w, r, rp.FPHash, mediaID, file) {
		span.SetStatus(codes.Error, "stale rendition url")
		return
	}
	if profileIdx < 0 || profileIdx >= len(in.Profiles) {
		span.SetStatus(codes.Error, "variant idx out of range")
		http.Error(w, `{"message":"variant not found"}`, http.StatusNotFound)
		return
	}

	// Copy/remux rung: when the selected producer owns its playlist (ffmpeg's
	// keyframe-aligned mux), serve THAT - its segment boundaries differ from the
	// precalculated generator's. Otherwise fall through to the precalculated
	// VOD manifest (the transcode path, unchanged).
	var out string
	served := false
	// Serve the producer's OWN playlist ONLY for a copy request. The
	// ManifestProvider assertion is structural (on the type): the software
	// TRANSCODE producer now also has a Manifest method, so without this gate
	// serveVariant would call it on the transcode path where copyPlanner is nil
	// and 500. Gating on the copy decision makes transcode fall through to the
	// precalculated manifest, unchanged.
	if rp.IsCopy {
		if producer := h.producerForPlan(rp); producer != nil {
			if mp, ok := producer.(apptranscoding.ManifestProvider); ok {
				data, mErr := mp.Manifest(r.Context(), rp.FP)
				if mErr != nil {
					span.RecordError(mErr)
					span.SetStatus(codes.Error, "copy manifest")
					logInternal(r.Context(), "hls: copy manifest", mErr)
					http.Error(w, `{"message":"manifest error"}`, http.StatusInternalServerError)
					return
				}
				// The copy manifest emits BARE segment URIs. Re-bake the same
				// auth + decision suffix the generator's playlists carry:
				// without it the segment requests arrive with no token (401 for
				// native players that don't hold the evey_hls cookie) and no
				// codec/hdr caps - makeDecision then re-derives a TRANSCODE
				// fingerprint and spawns a parallel ffmpeg re-encode whose
				// bytes contradict the master's copy CODECS.
				out, served = rebakeManifestURIs(string(data), apptranscoding.QuerySuffixFor(in)), true
			}
		}
	}
	if !served {
		v, err := h.manifestGen.Variant(in, profileIdx)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "manifest variant")
			logInternal(r.Context(), "hls: variant manifest", err)
			http.Error(w, `{"message":"manifest error"}`, http.StatusInternalServerError)
			return
		}
		out = v
	}

	// Shed EARLY, while the player has not yet committed to this stream. The
	// segment route is the authoritative gate, but by then a native player is
	// already buffering and can only surface "playback failed". Probing here
	// lets every client - including mobile/TV, whose players never expose a
	// segment's status code - tell the viewer plainly what is happening.
	//
	// Advisory by construction: two viewers can both pass this probe and race
	// at spawn. That is fine; the loser is shed at the segment, as before.
	// Probe with the rung actually requested: that is what the session about to
	// spawn will encode.
	if h.shedIfHostFull(w, r, mediaID, item, streams, rp, func(fp apptranscoding.DecisionFingerprint) []int {
		if profileIdx >= 0 && profileIdx < len(fp.Profiles) {
			return []int{fp.Profiles[profileIdx].Height}
		}
		return fingerprintHeights(fp)
	}) {
		return
	}

	setCORSHeaders(w, r, "")
	w.Header().Set("X-Evey-Served-By", "ffmpeg")
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(out))

	// Cold-start optimization: kick off the rafale NOW, before the player
	// has even parsed the variant playlist and asked for seg 0. Saves the
	// client → server → client round-trip (~50-200 ms) and, more
	// importantly, gives ffmpeg a head start on the slow decode path. On
	// 4K HEVC sources the segment 0 wall-clock dominates startup latency;
	// without this hint ffmpeg only starts when the .m4s request arrives.
	// PreWarm runs in a goroutine bound to the orchestrator's shutdownCtx
	// so Close() cancels every in-flight pre-warm (clean test teardown).
	// At the announced start position when the master carried one.
	if producer := h.producerForPlan(rp); producer != nil {
		producer.PreWarm(rp.FP, profileIdx, h.prewarmSegment(mediaID, in.DurationSec, time.Now()))
	}
}

func (h *hlsVODServe) serveSegment(w http.ResponseWriter, r *http.Request, mediaID, file string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, span trace.Span) {
	m := vodSegmentNameRE.FindStringSubmatch(file)
	variantIdx := atoiIndex(m[1])
	isInit := m[2] == ""
	var segmentIdx int
	if !isInit {
		segmentIdx = atoiIndex(m[2])
	}

	// Resolve the playback plan ONCE, fingerprint included, BEFORE resolving
	// the producer: the fingerprint's shaping creates/refreshes the admission
	// pin (route + ceiling), and the producer choice must read that same pin.
	// Resolving the producer first left a hole on a cold-pin segment request
	// (a player arriving without a prior master, or right after a restart):
	// the producer was picked on the natural route while the fingerprint
	// spilled to software - correct bytes and billing, but the wrong machine
	// ran the session. Every consumer below (bound check, seal, producer,
	// segment key) reads this same resolution.
	rp, err := h.resolvePlan(r, mediaID, item, streams)
	if err != nil {
		h.writeManifestErr(r.Context(), w, span, err, mediaID)
		return
	}

	// Bound segmentIdx against the media duration so a forged stream_N_99999.m4s
	// can't spawn an `ffmpeg -ss <past-the-end>` (fast exit, but 25-60s of
	// cache.Wait + session-slot churn per request). This applies ONLY to the
	// transcode ladder, whose segments are exactly ManifestSegmentSeconds long,
	// so ceil(dur/grid) is the EXACT segment count and any index >= it is forged.
	//
	// The COPY rung must be excluded: it cuts on source keyframes, which are
	// routinely SHORTER than the grid (sub-3s GOPs are common), so it lists MORE
	// segments than ceil(dur/grid). A grid bound here false-404'd the tail of
	// every such copy stream - e.g. a 51:48 h264 source with ~2.7s GOPs lists
	// 1146 segments but ceil(dur/3s)+1 = 1038, killing the last ~5 min at 46:43.
	// The copy path range-checks a forged index cheaply downstream via the plan's
	// len(segs) (software_producer startSession) with no ffmpeg -ss spawn, so it
	// needs no pre-bound here.
	if !isInit && item.File != nil {
		if durMs := item.File.Duration.Milliseconds(); durMs > 0 && !rp.IsCopy {
			const segMs = int64(apptranscoding.ManifestSegmentSeconds * 1000)
			maxSeg := int((durMs + segMs - 1) / segMs) // ceil = exact transcode segment count
			if segmentIdx >= maxSeg {
				span.SetStatus(codes.Error, "segment index past media end")
				http.Error(w, `{"message":"segment index out of range"}`, http.StatusNotFound)
				return
			}
		}
	}
	// Cast progress: a transcode-ladder segment fetched on a cast token is the
	// receiver advancing through the film. The ladder's grid is EXACT (fixed
	// 3s segments), so end = (idx+1) x grid. The COPY rung is deliberately NOT
	// observed here: it cuts on source keyframes with no grid, and grid math
	// can overshoot the real position by minutes (see cast_observe.go).
	if !isInit && !rp.IsCopy {
		durMs := int64(0)
		if item.File != nil {
			durMs = item.File.Duration.Milliseconds()
		}
		observeCastFetch(r, h.castProgress, transcodeSegEndMs(segmentIdx, apptranscoding.ManifestSegmentSeconds, durMs))
	}

	// CMCD ingest (CTA-5004). Parsed and shipped to the QoE sink before we
	// do anything that can 5xx - we want telemetry even on failure paths.
	// Sink is non-blocking; an absent payload is the no-op common case.
	h.ingestCMCD(r, mediaID, variantIdx, segmentIdx, isInit, false, false)

	fp := rp.FP
	// The seal check runs BEFORE any producer/cache work: a stale URL must not
	// spawn a rafale for a ladder the player is no longer holding, and above
	// all must not serve bytes from a rendition its init segment never named.
	if h.refuseStaleFP(w, r, rp.FPHash, mediaID, file) {
		span.SetStatus(codes.Error, "stale rendition url")
		return
	}

	// Resolve the producer for this request (software vs hardware) ONCE so the
	// EnsureSegmentRange calls, the client-hint, and the wait budget all target
	// the same machine - and the same one the ladder was shaped for. Falls back
	// to the orchestrator when no selector is wired (degenerate setups).
	producer := h.producerForPlan(rp)
	if producer == nil {
		span.SetStatus(codes.Error, "no stream producer wired")
		http.Error(w, `{"message":"transcode unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	// Phase 3 feedback loop: forward the CMCD signal to the producer so it can
	// wake up a paused ffmpeg on starvation and stretch the prefetch window when
	// the client has a fat buffer. NEVER changes which variant is served - that's
	// the player's call.
	h.notifyProducerHint(producer, r, mediaID, variantIdx, segmentIdx)

	// Bound variantIdx against the actual rung count: a forged stream_99_000.m4s
	// would otherwise reach EnsureSegmentRange. The software producer range-checks
	// it, but the HW orchestrator spawns a rafale that never writes stream_99_* →
	// the client eats a full WaitBudget (~25s) + one wasted session before 503.
	if variantIdx < 0 || variantIdx >= len(fp.Profiles) {
		span.SetStatus(codes.Error, "variant idx out of range")
		http.Error(w, `{"message":"variant index out of range"}`, http.StatusNotFound)
		return
	}

	// Ensure ffmpeg is producing the right rafale. Async - we then block on
	// SegmentCache.Wait below for the specific file. Init segments piggyback
	// on the first rafale (they are produced alongside segment 0 by fMP4 hls
	// muxer). ErrCapacityExceeded is hardware-only (the orchestrator's global
	// rafale cap); ErrVideoCapacityExhausted can come from EITHER producer (the
	// shared VideoSessionLimiter, gating only sessions that actually encode
	// video - never a video-copy/remux session).
	ensureSeg := segmentIdx
	if isInit {
		// Init segments are emitted alongside the first rafale for this rung.
		// Pass variantIdx so in single-variant mode the correct per-rung session
		// is created (or reused). In multi-variant mode variantIdx is forwarded
		// but the session covers all rungs anyway.
		// Every rafale writes the same init, so ensure it at the announced start:
		// ensuring segment 0 opened, or kept alive, a second GPU encoder at 0
		// next to the one being drained (prod 2026-09-09).
		durSec := 0.0
		if item.File != nil {
			durSec = item.File.Duration.Seconds()
		}
		ensureSeg = h.prewarmSegment(mediaID, durSec, time.Now())
	}
	if err := producer.EnsureSegmentRange(r.Context(), fp, variantIdx, ensureSeg); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "ensure segment range")
		if errors.Is(err, apptranscoding.ErrRungBusy) {
			// A cross-rung ABR probe while the viewer's rung is actively
			// encoding: answered FAST so the player abandons the up-switch
			// (Shaka blacklists the variant briefly) instead of hanging the
			// wait budget and churning the active encoder (prod 2026-07-29).
			slog.InfoContext(r.Context(), "hls-vod: refusing cross-rung probe, active rung protected",
				slog.String("media_id", mediaID),
				slog.Int("variant", variantIdx),
				slog.Int("seg", segmentIdx))
			writeTooManyStreams(w, 5, "another rung of this title is actively encoding")
			return
		}
		if errors.Is(err, apptranscoding.ErrCapacityExceeded) {
			// Host at its global transcode cap (hardware path) - shed load
			// gracefully. hls.js retries a 503 on a fragment fetch, so playback
			// resumes once a slot frees instead of melting the CPU.
			slog.WarnContext(r.Context(), "hls-vod: at capacity, shedding segment",
				slog.String("media_id", mediaID),
				slog.Int("seg", segmentIdx))
			writeTooManyStreams(w, 2, "server at transcode session capacity")
			return
		}
		if errors.Is(err, apptranscoding.ErrVideoCapacityExhausted) {
			// Before blaming other viewers, be certain this request truly needs a
			// video encode. A source that only lacks its keyframe index would be
			// served by the zero-cost copy rung (never a capacity slot) the moment
			// the index lands: nudge the on-demand walk and say so, instead of
			// telling this viewer the host is full because of everyone else.
			if copyBlockedOnlyByMissingIndex(rp.Decision, streams) {
				if h.keyframeWarmer != nil {
					go h.keyframeWarmer(context.WithoutCancel(r.Context()), mediaID, sourcePathOf(item))
				}
				slog.Info("hls-vod: shedding a copy-eligible source whose index is still building",
					slog.String("media_id", mediaID), slog.Int("seg", segmentIdx))
				writeShed(w, ErrCodeFileNotReady, 10, "source keyframe index still being built; the copy rung needs no encode slot")
				return
			}
			// Host at its calibrated/admin-capped video-encode session limit -
			// shed load gracefully rather than starting a session that would
			// starve the ones already running on a weak NAS. A longer
			// Retry-After than the raw capacity cap above: a video encode slot
			// frees far less often than a rafale slot.
			slog.WarnContext(r.Context(), "hls-vod: at video transcoding capacity, shedding segment",
				slog.String("media_id", mediaID),
				slog.Int("seg", segmentIdx))
			writeTooManyStreams(w, 5, "server at video transcoding capacity")
			return
		}
		slog.WarnContext(r.Context(), "hls-vod: ensure failed",
			slog.String("media_id", mediaID),
			slog.Int("variant", variantIdx),
			slog.Int("seg", segmentIdx),
			slog.Bool("init", isInit),
			slog.Any("err", err))
		http.Error(w, `{"message":"transcode unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	key := apptranscoding.SegmentKey{
		DecisionHash: rp.FPHash,
		VariantIdx:   variantIdx,
		SegmentIdx:   segmentIdx,
		IsInit:       isInit,
	}

	// The wait budget is producer-specific: the hardware orchestrator bursts
	// ahead and lands segment 0 fast (~25 s worst case), while the single paced
	// software encoder is slower and the client is expected to wait (~60 s).
	waitBudget := producer.WaitBudget()
	waitCtx, cancel := context.WithTimeout(r.Context(), waitBudget)
	defer cancel()
	if err := h.cache.Wait(waitCtx, key, fp.UseFMP4, waitBudget); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "segment wait timeout")
		slog.WarnContext(r.Context(), "hls-vod: segment timeout",
			slog.String("media_id", mediaID),
			slog.Int("variant", variantIdx),
			slog.Int("seg", segmentIdx),
			slog.Bool("init", isInit),
			slog.Any("err", err))
		http.Error(w, `{"message":"segment not yet ready, retry"}`, http.StatusServiceUnavailable)
		return
	}

	// Mark the hash as freshly accessed so the LRU sweeper keeps it warm. Reads
	// never bump file mtime, and the copy rung's ReadRate=0 mux freezes mtime
	// once it finishes - without this an actively-watched title would be evicted
	// out from under the player (re-mux storm). Cheap: one map write per segment.
	h.cache.Touch(key.DecisionHash)

	path := h.cache.Path(key, fp.UseFMP4)
	fi, statErr := os.Stat(path)
	if statErr != nil {
		span.RecordError(statErr)
		span.SetStatus(codes.Error, "segment missing")
		http.Error(w, `{"message":"segment missing"}`, http.StatusNotFound)
		return
	}
	// P3 ceiling-hit signal: a cost-modelled rung whose real segment ships at
	// or above its advertised bitrate is evidence the model under-priced this
	// title. Record it (in memory, O(1), off any probe) for the background
	// worker; never act on it here. An init segment or the copy rung carry no
	// model price, so they never signal.
	//
	// The measured segKbps is derived from the file SIZE, which on the default
	// path muxes audio next to the video and carries container overhead - while
	// VideoBitrate is the VIDEO ceiling alone. A video comfortably inside its
	// ceiling plus 192/384 kb/s of AAC would otherwise read as a ceiling hit on
	// the small rungs. Compare against the video ceiling PLUS the rung's muxed
	// audio, lifted by a small container margin so the mux overhead itself does
	// not trip it. A true hit (video pinned at maxrate) still clears this: the
	// segment then ships at VideoBitrate + audio + overhead.
	if !isInit && h.probeSignals != nil && variantIdx >= 0 && variantIdx < len(fp.Profiles) {
		if p := fp.Profiles[variantIdx]; p.CostModelled && !p.Copy && p.VideoBitrate > 0 {
			segKbps := float64(fi.Size()) * 8 / float64(apptranscoding.ManifestSegmentSeconds) / 1000
			ceiling := float64(p.VideoBitrate+p.AudioBitrate) * ceilingHitContainerMargin
			if segKbps >= ceiling {
				h.probeSignals.RecordCeilingHit(mediaID)
			}
		}
	}

	setCORSHeaders(w, r, "")
	// Precise serving path per rung: the fingerprint knows whether this
	// variant's bytes are a video copy (remux) or an encode.
	servedBy := "transcode"
	if variantIdx >= 0 && variantIdx < len(fp.Profiles) && fp.Profiles[variantIdx].Copy {
		servedBy = "ffmpeg-copy"
	}
	w.Header().Set("X-Evey-Served-By", servedBy)
	switch filepath.Ext(path) {
	case ".ts":
		w.Header().Set("Content-Type", "video/mp2t")
	case ".m4s", ".mp4":
		w.Header().Set("Content-Type", "video/mp4")
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, path)
}

// producerForRequest resolves the StreamProducer (software vs hardware) for the
// current request. Wrapper for the call sites without a resolved plan (prewarm
// of the NEXT episode, tests): it recomputes the decision the same way the
// manifest/fingerprint path does, then delegates to producerForPlan.
func (h *hlsVODServe) producerForRequest(ctx context.Context, r *http.Request, streams []*domainlibrary.MediaStream, item *domainlibrary.MediaItem) apptranscoding.StreamProducer {
	if h.selector == nil {
		if h.orchestrator != nil {
			return h.orchestrator
		}
		return nil
	}
	rp, err := h.probePlan(r, itemID(item), item, streams)
	if err != nil {
		slog.WarnContext(ctx, "hls-vod: producer pick: plan resolution failed", slog.String("media_id", itemID(item)), slog.Any("err", err))
		return nil
	}
	return h.producerForPlan(rp)
}

// producerForPlan picks the producer for a resolved plan - see PlanResolver.ProducerFor.
func (h *hlsVODServe) producerForPlan(rp *requestPlan) apptranscoding.StreamProducer {
	return h.planResolver().ProducerFor(rp.Request, rp.Decision, rp.IsCopy)
}

// videoCopyEligible / isCopyRequest - the copy-rung routing moved to the app
// layer (VideoCopyEligible / PlanResolver.IsCopy); wrappers for the adapter's
// remaining call sites and tests.
func videoCopyEligible(decision domaintranscoding.PlaybackDecision, streams []*domainlibrary.MediaStream) bool {
	return apptranscoding.VideoCopyEligible(decision, streams)
}

func (h *hlsVODServe) isCopyRequest(decision domaintranscoding.PlaybackDecision, streams []*domainlibrary.MediaStream, sourceBytes int64) bool {
	return h.planResolver().IsCopy(decision, streams, sourceBytes)
}

// durationMissingError is the specific 503 we return when a media file's
// duration isn't yet probed. The frontend retries on 503; a background re-probe
// is triggered by the library scheduler in the normal course.
type durationMissingError struct{ mediaID string }

func (e *durationMissingError) Error() string {
	return fmt.Sprintf("durationMissingError: media %s has no duration_ms", e.mediaID)
}

func (h *hlsVODServe) writeManifestErr(ctx context.Context, w http.ResponseWriter, span trace.Span, err error, mediaID string) {
	var dme *durationMissingError
	if errors.As(err, &dme) {
		span.SetStatus(codes.Error, "duration missing")
		slog.WarnContext(ctx, "hls-vod: duration missing",
			slog.String("media_id", mediaID))
		w.Header().Set("Retry-After", "5")
		http.Error(w, `{"message":"media duration not yet probed, retry"}`, http.StatusServiceUnavailable)
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, "buildManifestInput")
	// span.RecordError is a no-op unless OTLP tracing is configured, which is
	// not the default on a self-hosted NAS - without this line, the single
	// most-hit read path failed with zero server-side trace of why.
	slog.ErrorContext(ctx, "hls-vod: manifest build failed",
		slog.String("media_id", mediaID), slog.Any("err", err))
	http.Error(w, `{"message":"manifest error"}`, http.StatusInternalServerError)
}

// ---- small parsing helpers (kept private to this file) ----

func parseIntQuery(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func splitCSVQuery(r *http.Request, key string) []string {
	if v := r.URL.Query().Get(key); v != "" {
		return strings.Split(v, ",")
	}
	return nil
}

// servePrewarmNext handles POST /api/v1/media/{id}/prewarm-next. The {id}
// is the *current* episode. The handler resolves the next episode in the
// series and spawns a ffmpeg rafale targeting its first segment so a
// later autoplay (or next-ep button) hits a warm cache and starts in
// under a second. Safe to fire multiple times - EnsureSegmentRange is
// idempotent.
//
// Returns 200 + {next_media_item_id} on success, 204 when the current
// episode is the series finale, 4xx on bad input. Never propagates a
// transcode error to the player; warm-up failure is a UX nicety, not a
// hard playback path.
func (h *hlsVODServe) servePrewarmNext(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.transcode.PrewarmNext")
	defer span.End()
	r = r.WithContext(ctx)

	currentID := r.PathValue("id")
	if currentID == "" {
		http.Error(w, `{"message":"missing media id"}`, http.StatusBadRequest)
		return
	}

	claims := HTTPClaimsFromContext(r.Context())
	curItem, err := h.libSvc.GetMediaItem(r.Context(), currentID)
	if err != nil || curItem == nil {
		http.Error(w, `{"message":"current media not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, curItem.LibraryID, h.libraryAccess) {
		return
	}
	// Parental on the CURRENT episode too (this route is behind `auth` with the
	// real ProfileID, so it IS evaluable): a kid blocked on the current title
	// must not use prewarm-next as a pivot to discover/prefetch the next one.
	if !checkParental(r.Context(), w, claims, curItem, h.parental) {
		return
	}
	if curItem.Type != "episode" {
		// Only series episodes have a "next ep". For movies, no-op.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	nextItem, err := h.libSvc.NextEpisodeOf(r.Context(), currentID)
	if err != nil {
		span.RecordError(err)
		slog.WarnContext(r.Context(), "prewarm: next episode lookup failed", slog.String("current_id", currentID), slog.Any("err", err))
		http.Error(w, `{"message":"prewarm lookup failed"}`, http.StatusInternalServerError)
		return
	}
	if nextItem == nil || nextItem.File == nil || nextItem.File.Path == "" {
		// Series finale or next ep file missing - nothing to warm.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Re-check access on the NEXT item independently. The current item's
	// access doesn't imply the next one - a series can straddle libraries,
	// and a kid profile may have access to the current ep but not the next
	// (different rating / blocked tags). Without these two checks an
	// attacker could pivot from an accessible ep to trigger a transcode
	// rafale on any media they shouldn't see.
	if !checkLibraryAccess(r.Context(), w, claims, nextItem.LibraryID, h.libraryAccess) {
		return
	}
	if !checkParental(r.Context(), w, claims, nextItem, h.parental) {
		return
	}

	streams, err := h.libSvc.GetMediaStreams(r.Context(), string(nextItem.ID))
	if err != nil {
		span.RecordError(err)
		slog.WarnContext(r.Context(), "prewarm: streams lookup failed", slog.String("next_id", string(nextItem.ID)), slog.Any("err", err))
		http.Error(w, `{"message":"prewarm streams lookup failed"}`, http.StatusInternalServerError)
		return
	}

	// Build the same fingerprint the player would generate when it starts
	// playing this next episode: capabilities forwarded via query params,
	// per-title ladder via profilesForDecision, defaults safe.
	// Use the SAME builder the player path uses, so the warmed segments hash
	// identically to what the player will request (it earlier drifted -
	// HDRTonemap/ForceSidecarSubs hardcoded - so HDR/sidecar prewarms missed the
	// cache). One source of truth now also carries HWTonemap.
	fp, err := h.buildFingerprint(r, string(nextItem.ID), nextItem, streams)
	if err != nil {
		slog.WarnContext(r.Context(), "prewarm: fingerprint build failed", slog.String("next_id", string(nextItem.ID)), slog.Any("err", err))
		http.Error(w, `{"message":"prewarm fingerprint failed"}`, http.StatusInternalServerError)
		return
	}

	// Route the warm-up through the SAME producer the player will use for this
	// next episode (software vs hardware) so the warmed segments land in the
	// cache bucket the player reads. Resolved from the next item (container +
	// source size for the copy-rung disk guard).
	producer := h.producerForRequest(r.Context(), r, streams, nextItem)
	if producer == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"next_media_item_id":%q}`+"\n", string(nextItem.ID))
		return
	}
	if err := producer.EnsureSegmentRange(r.Context(), fp, 0, 0); err != nil {
		// Don't fail the response - the player will reach a cold cache
		// on switch, just no faster than before.
		if errors.Is(err, apptranscoding.ErrVideoCapacityExhausted) {
			// Expected graceful degradation under load, not a bug: the host is at
			// its video-encode session cap, so the best-effort prewarm backs off
			// instead of starving an in-flight viewer's encoder.
			slog.Info("prewarm: skipped, host at video transcoding capacity",
				slog.String("next_id", string(nextItem.ID)))
		} else {
			slog.WarnContext(r.Context(), "prewarm: producer EnsureSegmentRange failed",
				slog.String("next_id", string(nextItem.ID)), slog.Any("err", err))
		}
	} else {
		slog.Info("prewarm: rafale spawned for next episode",
			slog.String("current_id", currentID),
			slog.String("next_id", string(nextItem.ID)),
			slog.String("next_title", nextItem.Title))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"next_media_item_id":%q}`+"\n", string(nextItem.ID))
}
