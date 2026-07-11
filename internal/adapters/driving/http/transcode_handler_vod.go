package httpapi

import (
	"context"
	"errors"
	"fmt"
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

	appqoe "github.com/evey-project/evey/internal/app/qoe"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	pkgcache "github.com/evey-project/evey/internal/pkg/cache"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type hlsVODServe struct {
	tokens               TokenVerifier
	libSvc               MediaStreamsGetter
	libraryAccess        LibraryAccessChecker
	parental             ParentalChecker
	profilePrefs         ProfilePrefsGetter
	audioNormalizeGetter func(ctx context.Context) bool
	userMaxStreams       UserMaxStreamsGetter

	manifestGen *apptranscoding.ManifestGenerator
	cache       *apptranscoding.SegmentCache

	// selector routes each request to the software or hardware StreamProducer.
	// The software producer is the long-lived, input-paced, single-rendition
	// Jellyfin model (sane on a weak NAS with hw_accel off); the hardware
	// producer is the multi-session burst-ahead orchestrator (a GPU
	// parallelises rungs cheaply). The selector's For(decision, src) decides —
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
	// EVEY_REMUX_COPY=on): a dedicated SoftwareProducer in copy mode — the
	// Jellyfin remux, one paced `-c:v copy` + AAC session, seek = restart. nil =
	// the copy path is off and every request routes through the selector
	// (transcode) exactly as before. Interface-typed because both producers
	// satisfy it; only copy requests (isCopyRequest) are routed here.
	copyProducer apptranscoding.StreamProducer
	// keyframeWarmer nudges the bounded on-demand index walk. Optional (nil =
	// no nudge); see copyBlockedOnlyByMissingIndex for why it matters here.
	keyframeWarmer func(ctx context.Context, mediaID, path string) bool
	// videoAdmission is the limiter's read-only probe, used to shed a variant
	// playlist before a player commits to a stream. Optional (nil = no early
	// shed; the segment route still gates authoritatively).
	videoAdmission func(key string, class apptranscoding.LadderClass, topHeight int) bool
	// admissionRoute asks the admission shaper for this stream's pinned route
	// and ladder ceiling (see profilesForDecision's third cap): a hw stream on
	// a full GPU pool spills to the software route when the CPU pool has room.
	// Optional: nil keeps the natural shape and viewers over budget are shed.
	admissionRoute func(key string, natural apptranscoding.LadderClass, heights []int, fallback func() []int) apptranscoding.RouteGrant
	// admissionPinnedClass reads the class a live pin was granted under, so
	// the fingerprint's hw fields and the spawn's producer follow the pinned
	// route rather than the natural one. Optional.
	admissionPinnedClass func(key string) (apptranscoding.LadderClass, bool)
	// admissionBind reports the fingerprint hash built under a pin so the
	// shaper can freeze the ceiling while that session holds a slot. Optional.
	admissionBind func(key, fingerprintHash string)

	// onDemand serves the copy rung straight from mkvgo's mp4.PlanHLS —
	// zero-storage, stateless, byte-identical HLS + DASH — when wired (behind
	// transcoding.hls_ondemand). It supersedes copyProducer for video+audio
	// copy-eligible requests whose source packages verbatim (see planHLSEligible
	// / transcode_ondemand.go); everything else falls through to the ffmpeg
	// pipeline unchanged. nil = feature off, no behaviour change.
	onDemand apptranscoding.OnDemandPackager

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

	// qoeSink ingests CMCD telemetry from segment requests. Nil when QoE is
	// disabled (tests, dev without postgres). Record is non-blocking.
	qoeSink appqoe.EventSink

	// complexityRepo reads the per-title classification used to pick the
	// ABR ladder. Nil-safe — profilesForDecision falls back to
	// LadderFilmNormal when the repo isn't wired (dev w/o postgres) or
	// the row hasn't been classified yet.
	complexityRepo domaintranscoding.ComplexityRepo

	// capability reports the host's calibrated encode ceiling. Nil-safe:
	// profilesForDecision falls back to the safe 720p floor when the
	// service isn't wired (tests). Returns a fresh snapshot per call so
	// recalibration via the admin endpoint takes effect immediately —
	// no need to restart the server.
	capability func() domaintranscoding.EncodeCapability

	// capOverride is the CMCD-feedback adaptive override service.
	// Returns (override, true) when an active override applies to the
	// source-class bucket — handler uses MIN(benchmark, override) as
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

	// useFMP4 controls the segment container. fMP4 is the project default —
	// segments are ~30% smaller and the codecs string in the master playlist
	// matches what hls.js expects on modern Chromium/Firefox.
	useFMP4 bool

	// audioRenditionEnabled mirrors Cfg.Transcoding.AudioRenditionEnabled.
	// When false (default), the master playlist stays muxed audio+video
	// bit-for-bit and the audio rendition handlers respond 404. When true,
	// the master advertises EXT-X-MEDIA TYPE=AUDIO renditions per source
	// audio track and the dedicated audio pipeline serves them.
	audioRenditionEnabled bool

	// audio handles the audio rendition routes (audio_T.m3u8 / init /
	// segment). Routed through the same /hls/{file} entry point so the
	// relative URLs the master playlist emits resolve correctly. Nil-safe
	// — dispatched paths 404 when unset (tests, no AudioOrch wired).
	audio *audioServe

	// segmentWaitTimeout caps how long the segment handler will block waiting
	// for ffmpeg to land a segment. Larger than expected encode time but
	// smaller than the user's patience for a 504. Default 15s.
	segmentWaitTimeout time.Duration

	// serverLang returns the server-level metadata language (cfg.Metadata.Language,
	// e.g. "en"). Used as the lowest-priority fallback for default audio track
	// resolution when neither the profile's audio_languages nor its locale
	// match any stream. Nil-safe — falls back to "en".
	serverLang func() string

	// mediaLookup / classLookup are SHORT-TTL caches for the per-segment hot
	// path: every segment request used to pay three DB round-trips
	// (GetMediaItem + GetMediaStreams + complexity Get) for values that are
	// stable across a playback burst. 10 s of staleness is harmless — the
	// access/parental checks still run live per request, and a mid-playback
	// metadata change converging one burst later is the same tolerance the
	// manifest/segment pair already has. Cached values are SHARED between
	// requests: treat them as immutable. Nil-safe (hand-built test handlers
	// keep the direct fetch path).
	mediaLookup *pkgcache.TTLCache[cachedMediaLookup]
	classLookup *pkgcache.TTLCache[domaintranscoding.ContentClass]
}

// cachedMediaLookup is one mediaLookup entry: the item and its streams as
// resolved together (never cache one without the other — the fingerprint
// derives from both).
type cachedMediaLookup struct {
	item    *domainlibrary.MediaItem
	streams []*domainlibrary.MediaStream
}

const (
	mediaLookupTTL = 10 * time.Second
	mediaLookupMax = 256
)

// vodSegmentNameRE parses the on-the-wire segment file name and returns the
// (variantIdx, segmentIdx, isInit, ext) tuple. Strict: any other shape 404s.
// The index is \d+ (not \d{3}): ffmpeg's hls_segment_filename only zero-pads to
// a 3-digit MINIMUM, so segment 1000+ (≥ ~50 min at 3s/seg) is 4 digits — a
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
	// each downstream function from re-fetching — one master + one variant +
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
	case file == "manifest.mpd" || strings.HasPrefix(file, onDemandPrefix):
		// On-demand copy rung (mp4.PlanHLS): the DASH manifest entry point and
		// every prefixed sub-resource (od_playlist.m3u8, od_seg00001.m4s,
		// od_audio1.m3u8, od_init.mp4, od_sub1.vtt …) the plan's manifests
		// reference. 404 when the feature is off / the source can't be packaged.
		h.serveOnDemandResource(w, r, mediaID, file, item, span)
	case vodVariantNameRE.MatchString(file):
		h.serveVariant(w, r, mediaID, file, item, streams, span)
	case vodSegmentNameRE.MatchString(file):
		h.serveSegment(w, r, mediaID, file, item, streams, span)
	case audioPlaylistNameRE.MatchString(file):
		h.dispatchAudioPlaylist(w, r, mediaID, file, span)
	case audioSegmentNameRE.MatchString(file):
		h.dispatchAudioSegment(w, r, mediaID, file, span)
	default:
		span.SetStatus(codes.Error, "unknown hls path")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
	}
}

// dispatchAudioPlaylist + dispatchAudioSegment are tiny adapters: they parse
// the file name the master playlist emitted (audio_T.m3u8 / audio_T_NNN.m4s
// / audio_T_init.mp4) and forward to the audio handler with explicit ints.
// We can't register `/api/v1/media/{id}/audio_{trackIdx}.m3u8` as a separate
// route — Go 1.22+ ServeMux refuses wildcards that don't cover a whole
// path segment — and we don't want to either, because the URLs the manifest
// emits are *relative* to /hls/master.m3u8 and thus resolve into /hls/{file}.
func (h *hlsVODServe) dispatchAudioPlaylist(w http.ResponseWriter, r *http.Request, mediaID, file string, span trace.Span) {
	if h.audio == nil {
		span.SetStatus(codes.Error, "audio rendition not wired")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	m := audioPlaylistNameRE.FindStringSubmatch(file)
	trackIdx, _ := strconv.Atoi(m[1])
	h.audio.servePlaylistFile(w, r, mediaID, trackIdx)
}

func (h *hlsVODServe) dispatchAudioSegment(w http.ResponseWriter, r *http.Request, mediaID, file string, span trace.Span) {
	if h.audio == nil {
		span.SetStatus(codes.Error, "audio rendition not wired")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	m := audioSegmentNameRE.FindStringSubmatch(file)
	trackIdx, _ := strconv.Atoi(m[1])
	if m[2] == "" {
		// _init.mp4
		h.audio.serveInitFile(w, r, mediaID, trackIdx)
		return
	}
	segIdx, _ := strconv.Atoi(m[2])
	h.audio.serveSegmentFile(w, r, mediaID, trackIdx, segIdx)
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
	// ACCESS checks below always run live — only the media lookup is cached.
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
	if !cached {
		var err error
		streams, err = h.libSvc.GetMediaStreams(r.Context(), mediaID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "streams resolve failed")
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
	// Catches the startup CMCD payload (?CMCD=su,sid=...,ot=m) — the first
	// thing every player sends. Useful for "time to first frame" admin views.
	h.ingestCMCD(r, mediaID, 0, 0, false, true, false)

	// On-demand copy rung: when the request is video+audio copy-eligible and the
	// source packages verbatim, serve the whole presentation from mp4.PlanHLS
	// (stateless, byte-identical, DASH included) instead of the ffmpeg pipeline.
	// Falls through (feature off / not eligible / plan build failed) unchanged.
	if h.tryServePlanMaster(w, r, mediaID, item, streams, span) {
		return
	}

	// Tell a viewer the host is full BEFORE their player parses a ladder it can
	// never fetch - the only signal a native mobile/TV player ever sees. Probed
	// against the CHEAPEST rung: a master is refused only when no quality at
	// all fits, never merely because the tallest one does not.
	if h.shedIfHostFull(w, r, mediaID, item, streams, fingerprintHeights) {
		return
	}

	in, err := h.buildManifestInput(r, mediaID, item, streams)
	if err != nil {
		h.writeManifestErr(w, span, err, mediaID)
		return
	}

	out, err := h.manifestGen.Master(in)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "manifest master")
		http.Error(w, `{"message":"manifest error"}`, http.StatusInternalServerError)
		return
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

	slog.Info("hls-vod: master served",
		slog.String("media_id", mediaID),
		slog.Int("profiles", len(in.Profiles)),
		slog.Int("segments_per_variant", in.SegmentCount()),
	)
}

func (h *hlsVODServe) serveVariant(w http.ResponseWriter, r *http.Request, mediaID, file string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, span trace.Span) {
	m := vodVariantNameRE.FindStringSubmatch(file)
	profileIdx, _ := strconv.Atoi(m[1])

	// Catches CMCD on variant playlist fetches (typical when the player ABR
	// switches renditions). Helps the dashboard count switch frequency.
	h.ingestCMCD(r, mediaID, profileIdx, 0, false, false, true)

	in, err := h.buildManifestInput(r, mediaID, item, streams)
	if err != nil {
		h.writeManifestErr(w, span, err, mediaID)
		return
	}
	if profileIdx < 0 || profileIdx >= len(in.Profiles) {
		span.SetStatus(codes.Error, "variant idx out of range")
		http.Error(w, `{"message":"variant not found"}`, http.StatusNotFound)
		return
	}

	// Copy/remux rung: when the selected producer owns its playlist (ffmpeg's
	// keyframe-aligned mux), serve THAT — its segment boundaries differ from the
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
	if h.isCopyRequest(h.makeDecision(r.Context(), streams, containerOf(item), sourcePathOf(item), r), streams, sourceBytesOf(item)) {
		if producer := h.producerForRequest(r.Context(), r, streams, item); producer != nil {
			if mp, ok := producer.(apptranscoding.ManifestProvider); ok {
				fp, fpErr := h.buildFingerprint(r, mediaID, item, streams)
				if fpErr != nil {
					h.writeManifestErr(w, span, fpErr, mediaID)
					return
				}
				data, mErr := mp.Manifest(r.Context(), fp)
				if mErr != nil {
					span.RecordError(mErr)
					span.SetStatus(codes.Error, "copy manifest")
					http.Error(w, `{"message":"manifest error"}`, http.StatusInternalServerError)
					return
				}
				// The copy manifest emits BARE segment URIs. Re-bake the same
				// auth + decision suffix the generator's playlists carry:
				// without it the segment requests arrive with no token (401 for
				// native players that don't hold the evey_hls cookie) and no
				// codec/hdr caps — makeDecision then re-derives a TRANSCODE
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
	if h.shedIfHostFull(w, r, mediaID, item, streams, func(fp apptranscoding.DecisionFingerprint) []int {
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
	if fp, fpErr := h.buildFingerprint(r, mediaID, item, streams); fpErr == nil {
		if producer := h.producerForRequest(r.Context(), r, streams, item); producer != nil {
			producer.PreWarm(fp, profileIdx, 0)
		}
	}
}

func (h *hlsVODServe) serveSegment(w http.ResponseWriter, r *http.Request, mediaID, file string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, span trace.Span) {
	m := vodSegmentNameRE.FindStringSubmatch(file)
	variantIdx, _ := strconv.Atoi(m[1])
	isInit := m[2] == ""
	var segmentIdx int
	if !isInit {
		segmentIdx, _ = strconv.Atoi(m[2])
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
	// every such copy stream — e.g. a 51:48 h264 source with ~2.7s GOPs lists
	// 1146 segments but ceil(dur/3s)+1 = 1038, killing the last ~5 min at 46:43.
	// The copy path range-checks a forged index cheaply downstream via the plan's
	// len(segs) (software_producer startSession) with no ffmpeg -ss spawn, so it
	// needs no pre-bound here.
	if !isInit && item.File != nil {
		if durMs := item.File.Duration.Milliseconds(); durMs > 0 &&
			!h.isCopyRequest(h.makeDecision(r.Context(), streams, containerOf(item), sourcePathOf(item), r), streams, sourceBytesOf(item)) {
			const segMs = int64(apptranscoding.ManifestSegmentSeconds * 1000)
			maxSeg := int((durMs + segMs - 1) / segMs) // ceil = exact transcode segment count
			if segmentIdx >= maxSeg {
				span.SetStatus(codes.Error, "segment index past media end")
				http.Error(w, `{"message":"segment index out of range"}`, http.StatusNotFound)
				return
			}
		}
	}

	// CMCD ingest (CTA-5004). Parsed and shipped to the QoE sink before we
	// do anything that can 5xx — we want telemetry even on failure paths.
	// Sink is non-blocking; an absent payload is the no-op common case.
	h.ingestCMCD(r, mediaID, variantIdx, segmentIdx, isInit, false, false)

	// Build the fingerprint BEFORE resolving the producer: the fingerprint's
	// shaping creates/refreshes the admission pin (route + ceiling), and the
	// producer choice must read that same pin. Resolving the producer first
	// left a hole on a cold-pin segment request (a player arriving without a
	// prior master, or right after a restart): the producer was picked on the
	// natural route while the fingerprint spilled to software - correct bytes
	// and billing, but the wrong machine ran the session.
	fp, err := h.buildFingerprint(r, mediaID, item, streams)
	if err != nil {
		h.writeManifestErr(w, span, err, mediaID)
		return
	}

	// Resolve the producer for this request (software vs hardware) ONCE so the
	// EnsureSegmentRange calls, the client-hint, and the wait budget all target
	// the same machine — and the same one the ladder was shaped for. Falls back
	// to the orchestrator when no selector is wired (degenerate setups).
	producer := h.producerForRequest(r.Context(), r, streams, item)
	if producer == nil {
		span.SetStatus(codes.Error, "no stream producer wired")
		http.Error(w, `{"message":"transcode unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	// Phase 3 feedback loop: forward the CMCD signal to the producer so it can
	// wake up a paused ffmpeg on starvation and stretch the prefetch window when
	// the client has a fat buffer. NEVER changes which variant is served — that's
	// the player's call.
	h.notifyProducerHint(producer, r, mediaID, segmentIdx)

	// Bound variantIdx against the actual rung count: a forged stream_99_000.m4s
	// would otherwise reach EnsureSegmentRange. The software producer range-checks
	// it, but the HW orchestrator spawns a rafale that never writes stream_99_* →
	// the client eats a full WaitBudget (~25s) + one wasted session before 503.
	if variantIdx < 0 || variantIdx >= len(fp.Profiles) {
		span.SetStatus(codes.Error, "variant idx out of range")
		http.Error(w, `{"message":"variant index out of range"}`, http.StatusNotFound)
		return
	}

	// Ensure ffmpeg is producing the right rafale. Async — we then block on
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
		ensureSeg = 0
	}
	if err := producer.EnsureSegmentRange(r.Context(), fp, variantIdx, ensureSeg); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "ensure segment range")
		if errors.Is(err, apptranscoding.ErrCapacityExceeded) {
			// Host at its global transcode cap (hardware path) — shed load
			// gracefully. hls.js retries a 503 on a fragment fetch, so playback
			// resumes once a slot frees instead of melting the CPU.
			slog.Warn("hls-vod: at capacity, shedding segment",
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
			// Recomputed only on this rare shed path (the decision is a pure
			// function of the same inputs the segment route already resolved).
			if copyBlockedOnlyByMissingIndex(h.makeDecision(r.Context(), streams, containerOf(item), sourcePathOf(item), r), streams) {
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
			slog.Warn("hls-vod: at video transcoding capacity, shedding segment",
				slog.String("media_id", mediaID),
				slog.Int("seg", segmentIdx))
			writeTooManyStreams(w, 5, "server at video transcoding capacity")
			return
		}
		slog.Warn("hls-vod: ensure failed",
			slog.String("media_id", mediaID),
			slog.Int("variant", variantIdx),
			slog.Int("seg", segmentIdx),
			slog.Bool("init", isInit),
			slog.Any("err", err))
		http.Error(w, `{"message":"transcode unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	key := apptranscoding.SegmentKey{
		DecisionHash: fp.Hash(),
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
		slog.Warn("hls-vod: segment timeout",
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
	// once it finishes — without this an actively-watched title would be evicted
	// out from under the player (re-mux storm). Cheap: one map write per segment.
	h.cache.Touch(key.DecisionHash)

	path := h.cache.Path(key, fp.UseFMP4)
	if _, statErr := os.Stat(path); statErr != nil {
		span.RecordError(statErr)
		span.SetStatus(codes.Error, "segment stat")
		http.Error(w, `{"message":"segment missing"}`, http.StatusNotFound)
		return
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
// current request. It recomputes the decision + source dims the same way the
// manifest/fingerprint path does, so the producer chosen for a segment matches
// the one the ladder was shaped for (and thus the cache hash). Falls back to
// the orchestrator field when no selector is wired (hand-built test handlers);
// returns nil only when neither is set.
func (h *hlsVODServe) producerForRequest(ctx context.Context, r *http.Request, streams []*domainlibrary.MediaStream, item *domainlibrary.MediaItem) apptranscoding.StreamProducer {
	if h.selector != nil {
		decision := h.makeDecision(ctx, streams, containerOf(item), sourcePathOf(item), r)
		if h.isCopyRequest(decision, streams, sourceBytesOf(item)) {
			return h.copyProducer
		}
		// Admission spill: a stream pinned to the software route spawns on the
		// software producer regardless of the hw gate - the producer must be
		// the one the ladder was shaped for (see profilesForDecision).
		if h.admissionPinnedClass != nil {
			if cls, ok := h.admissionPinnedClass(h.admissionPinKey(item, streams, decision)); ok && cls != apptranscoding.LadderClassHW {
				if sw := h.selector.Software(); sw != nil {
					return sw
				}
			}
		}
		return h.selector.For(decision, sourceDimsOf(streams))
	}
	if h.orchestrator != nil {
		return h.orchestrator
	}
	return nil
}

// isCopyRequest reports whether this request should be served as a video-COPY
// (remux) rung rather than transcoded. Three conditions, all required: the copy
// path is wired (EVEY_REMUX_COPY=on → copyProducer non-nil), the decision did
// NOT require a video transcode (VideoDecision=="copy"), and the source is
// copy-eligible — sourceCopyVideoCodec returns "" unless it is H.264 8-bit SDR
// with a known level (so the master can advertise an accurate avc1.PPCCLL).
// When false, the request routes through the selector exactly as before.
func (h *hlsVODServe) isCopyRequest(decision domaintranscoding.PlaybackDecision, streams []*domainlibrary.MediaStream, sourceBytes int64) bool {
	if h.copyProducer == nil ||
		decision.VideoDecision != "copy" ||
		sourceCopyVideoCodec(streams) == "" {
		return false
	}
	// Disk guard: the copy rung's segments duplicate ~the source size on disk over
	// the session (video copied 1:1 + a smaller AAC track), and the unpaced copy
	// produces them well ahead of the viewer. If the cache filesystem can't fit it
	// with headroom, fall back to the segment-bounded transcode ladder rather than
	// risk ENOSPC on a small box. Fail-OPEN when the size is unknown or
	// statfs fails — keeps today's behaviour and the mux still fails gracefully
	// downstream. (master- and segment-time both reach this with the same
	// item.File.Size; a free-space flip between them is a rare, bounded 503-retry,
	// not a cache aliasing — the copy hash never collides with a transcode hash.)
	if sourceBytes > 0 {
		if free, err := h.cache.FreeBytes(); err == nil && free < sourceBytes+sourceBytes/10 {
			return false
		}
	}
	return true
}

// durationMissingError is the specific 503 we return when a media file's
// duration isn't yet probed. The frontend retries on 503; a background re-probe
// is triggered by the library scheduler in the normal course.
type durationMissingError struct{ mediaID string }

func (e *durationMissingError) Error() string {
	return fmt.Sprintf("durationMissingError: media %s has no duration_ms", e.mediaID)
}

func (h *hlsVODServe) writeManifestErr(w http.ResponseWriter, span trace.Span, err error, mediaID string) {
	var dme *durationMissingError
	if errors.As(err, &dme) {
		span.SetStatus(codes.Error, "duration missing")
		slog.Warn("hls-vod: duration missing",
			slog.String("media_id", mediaID))
		w.Header().Set("Retry-After", "5")
		http.Error(w, `{"message":"media duration not yet probed, retry"}`, http.StatusServiceUnavailable)
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, "buildManifestInput")
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
// under a second. Safe to fire multiple times — EnsureSegmentRange is
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
		slog.Warn("prewarm: next episode lookup failed", slog.String("current_id", currentID), slog.Any("err", err))
		http.Error(w, `{"message":"prewarm lookup failed"}`, http.StatusInternalServerError)
		return
	}
	if nextItem == nil || nextItem.File == nil || nextItem.File.Path == "" {
		// Series finale or next ep file missing — nothing to warm.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Re-check access on the NEXT item independently. The current item's
	// access doesn't imply the next one — a series can straddle libraries,
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
		slog.Warn("prewarm: streams lookup failed", slog.String("next_id", string(nextItem.ID)), slog.Any("err", err))
		http.Error(w, `{"message":"prewarm streams lookup failed"}`, http.StatusInternalServerError)
		return
	}

	// Build the same fingerprint the player would generate when it starts
	// playing this next episode: capabilities forwarded via query params,
	// per-title ladder via profilesForDecision, defaults safe.
	// Use the SAME builder the player path uses, so the warmed segments hash
	// identically to what the player will request (it earlier drifted —
	// HDRTonemap/ForceSidecarSubs hardcoded — so HDR/sidecar prewarms missed the
	// cache). One source of truth now also carries HWTonemap.
	fp, err := h.buildFingerprint(r, string(nextItem.ID), nextItem, streams)
	if err != nil {
		slog.Warn("prewarm: fingerprint build failed", slog.String("next_id", string(nextItem.ID)), slog.Any("err", err))
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
		// Don't fail the response — the player will reach a cold cache
		// on switch, just no faster than before.
		if errors.Is(err, apptranscoding.ErrVideoCapacityExhausted) {
			// Expected graceful degradation under load, not a bug: the host is at
			// its video-encode session cap, so the best-effort prewarm backs off
			// instead of starving an in-flight viewer's encoder.
			slog.Info("prewarm: skipped, host at video transcoding capacity",
				slog.String("next_id", string(nextItem.ID)))
		} else {
			slog.Warn("prewarm: producer EnsureSegmentRange failed",
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
