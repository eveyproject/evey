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
	"slices"
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
	"github.com/evey-project/evey/internal/pkg/cmcd"
	"github.com/evey-project/evey/internal/pkg/hdr"
	eveymetrics "github.com/evey-project/evey/internal/pkg/metrics"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// hlsVODServe wires the new pipeline: a static, content-addressable HLS VOD
// manifest (no live-edge snap, no drift), an on-disk segment cache shared
// across sessions with the same DecisionHash, and an orchestrator that spawns
// ffmpeg rafales on cache miss. Replaces hlsMasterServe / hlsSegmentHandler
// once the runtime validation step lands.
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
	if h.isCopyRequest(h.makeDecision(r.Context(), streams, containerOf(item), r), streams, sourceBytesOf(item)) {
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

	setCORSHeaders(w, r, "")
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
			producer.PreWarm(fp, 0)
		}
	}
}

// rebakeManifestURIs appends the request's auth + decision query suffix to
// every URI in an HLS playlist: bare segment lines, plus the URI="..." of any
// tag that carries one (#EXT-X-MAP init, #EXT-X-KEY AES key, #EXT-X-MEDIA
// audio/subtitle renditions, #EXT-X-STREAM-INF children in a master). No-op on
// empty suffix or on URIs that already carry a query.
func rebakeManifestURIs(manifest, suffix string) string {
	if suffix == "" {
		return manifest
	}
	lines := strings.Split(manifest, "\n")
	for i, ln := range lines {
		switch {
		case strings.HasPrefix(ln, "#") && strings.Contains(ln, "URI=\""):
			lines[i] = addSuffixToURIAttr(ln, suffix)
		case ln != "" && !strings.HasPrefix(ln, "#") && !strings.Contains(ln, "?"):
			lines[i] = ln + suffix
		}
	}
	return strings.Join(lines, "\n")
}

// addSuffixToURIAttr appends suffix inside the first URI="..." attribute of an
// HLS tag line, unless the value is empty or already has a query.
func addSuffixToURIAttr(line, suffix string) string {
	const marker = "URI=\""
	i := strings.Index(line, marker)
	if i < 0 {
		return line
	}
	start := i + len(marker)
	rel := strings.IndexByte(line[start:], '"')
	if rel < 0 {
		return line
	}
	val := line[start : start+rel]
	if val == "" || strings.Contains(val, "?") {
		return line
	}
	return line[:start] + val + suffix + line[start+rel:]
}

// dashURIAttrRE matches the segment/init template attributes of a DASH manifest
// whose value carries no query yet.
var dashURIAttrRE = regexp.MustCompile(`(media|initialization)="([^"?]*)"`)

// rebakeDASHManifestURIs appends suffix to the media/initialization template
// URIs of a DASH manifest so a cookie-less client (Chromecast) authenticates
// its segment/init requests via the query token.
func rebakeDASHManifestURIs(manifest, suffix string) string {
	if suffix == "" {
		return manifest
	}
	return dashURIAttrRE.ReplaceAllString(manifest, `${1}="${2}`+suffix+`"`)
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
	// cache.Wait + session-slot churn per request). Copy mode already rejects via
	// the plan's len(segs); this guards the transcode path. The transcode grid
	// (ManifestSegmentSeconds) is the SMALLEST segment, so ceil(dur/grid)+1 is a
	// safe upper bound for copy too (its segments are >= 6s → fewer of them).
	if !isInit && item.File != nil {
		if durMs := item.File.Duration.Milliseconds(); durMs > 0 {
			const segMs = int64(apptranscoding.ManifestSegmentSeconds * 1000)
			maxSeg := int((durMs+segMs-1)/segMs) + 1
			if segmentIdx > maxSeg {
				span.SetStatus(codes.Error, "segment index past media end")
				http.Error(w, `{"message":"segment index out of range"}`, http.StatusNotFound)
				return
			}
		}
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

	// CMCD ingest (CTA-5004). Parsed and shipped to the QoE sink before we
	// do anything that can 5xx — we want telemetry even on failure paths.
	// Sink is non-blocking; an absent payload is the no-op common case.
	h.ingestCMCD(r, mediaID, variantIdx, segmentIdx, isInit, false, false)
	// Phase 3 feedback loop: forward the CMCD signal to the producer so it can
	// wake up a paused ffmpeg on starvation and stretch the prefetch window when
	// the client has a fat buffer. NEVER changes which variant is served — that's
	// the player's call.
	h.notifyProducerHint(producer, r, mediaID, segmentIdx)

	fp, err := h.buildFingerprint(r, mediaID, item, streams)
	if err != nil {
		h.writeManifestErr(w, span, err, mediaID)
		return
	}

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
	// muxer). The software producer never returns ErrCapacityExceeded; only the
	// hardware orchestrator does (its global concurrency cap), so the 503 branch
	// is hardware-only by construction.
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
			w.Header().Set("Retry-After", "2")
			http.Error(w, `{"message":"server at transcode capacity, retry"}`, http.StatusServiceUnavailable)
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
	switch filepath.Ext(path) {
	case ".ts":
		w.Header().Set("Content-Type", "video/mp2t")
	case ".m4s", ".mp4":
		w.Header().Set("Content-Type", "video/mp4")
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, path)
}

// resolveAudioTrack returns the audio-index to use for the given request and
// stream list. When the client supplies an explicit ?audioTrack= query param,
// that value is honoured as-is (the user or player has already chosen). When
// absent, the server resolves the best match using the profile's language
// preferences (AudioLanguages → Locale → server language → IsDefault → 0).
func (h *hlsVODServe) resolveAudioTrack(r *http.Request, streams []*domainlibrary.MediaStream) int {
	if r.URL.Query().Has("audioTrack") {
		t := parseIntQuery(r, "audioTrack", 0)
		// Honour the explicit choice, but bound it against the real audio-stream
		// count: an out-of-range ?audioTrack=99 would otherwise become `-map 0:a:99`
		// and fail the whole ffmpeg session per request. Out-of-range falls through
		// to the language-based default (a valid index).
		nAudio := 0
		for _, s := range streams {
			if s.StreamType == domainlibrary.Audio {
				nAudio++
			}
		}
		if t >= 0 && t < nAudio {
			return t
		}
	}
	claims := HTTPClaimsFromContext(r.Context())
	prefs := loadProfilePrefs(r.Context(), h.profilePrefs, claims.ProfileID)
	srvLang := "en"
	if h.serverLang != nil {
		srvLang = h.serverLang()
	}
	langs := buildPrefLangs(prefs.AudioLanguages, prefs.Locale, srvLang)
	return resolveDefaultAudioTrack(streams, langs)
}

// buildManifestInput resolves the media + decision + token into the
// ManifestInput the generator consumes. Threads the same query string-parsed
// fields into both the manifest's URLs and the DecisionFingerprint, so segment
// requests can be hashed back to the same cache bucket.
func (h *hlsVODServe) buildManifestInput(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) (apptranscoding.ManifestInput, error) {
	if item == nil || item.File == nil {
		return apptranscoding.ManifestInput{}, errors.New("buildManifestInput: nil item")
	}

	if item.File.Duration.Milliseconds() <= 0 {
		return apptranscoding.ManifestInput{}, &durationMissingError{mediaID: mediaID}
	}

	decision := h.makeDecision(r.Context(), streams, item.File.ContainerFormat, r)
	profiles := h.profilesForDecision(r.Context(), item, streams, decision)

	audioTrack := h.resolveAudioTrack(r, streams)
	subBurnIn := parseIntQuery(r, "subtitleTrack", -1)

	token := RawTokenFromContext(r.Context())

	// `?resume=N` is intentionally OUTSIDE the decisionHash inputs so two
	// users on the same DecisionFingerprint don't shard the segment cache
	// just because they resume at different positions. ManifestInput.Validate
	// rejects negative/garbage; sec or ms accepted (caller convention: sec).
	resumeSec := 0.0
	if r != nil {
		if raw := r.URL.Query().Get("resume"); raw != "" {
			if n, err := strconv.ParseFloat(raw, 64); err == nil && n > 0 {
				resumeSec = n
			}
		}
	}

	// Audio renditions are populated only when the feature flag is on.
	// Empty slice preserves the muxed audio+video manifest bit-for-bit —
	// every existing client keeps working without change. Shared
	// buildAudioRenditions helper guarantees the master + the
	// audio playlist handlers agree on the same SourceTrackIdx
	// numbering (audio-index, matches `ffmpeg -map 0:a:N`).
	var audioRenditions []apptranscoding.AudioRendition
	if h.audioRenditionEnabled {
		audioRenditions = buildAudioRenditions(streams)
	}

	// Advertise the codec the segments will ACTUALLY carry, after the capability
	// gate (AV1 → H.264 when AV1 isn't encodable here). Same helper as
	// buildFingerprint, so the master CODECS can't disagree with the bytes.
	chosenVideoCodec, _, _, _, _ := h.effectiveVideoCodec(decision, streams)

	return apptranscoding.ManifestInput{
		MediaID:             mediaID,
		DurationSec:         float64(item.File.Duration.Milliseconds()) / 1000.0,
		Profiles:            profiles,
		StreamToken:         token,
		AudioTrack:          audioTrack,
		SubtitleBurnInTrack: subBurnIn,
		VideoCodecs:         splitCSVQuery(r, "videoCodecs"),
		AudioCodecs:         splitCSVQuery(r, "audioCodecs"),
		Containers:          splitCSVQuery(r, "containers"),
		UseFMP4:             h.useFMP4,
		ChosenVideoCodec:    chosenVideoCodec,
		// The codec the segments will ACTUALLY carry: the source codec on
		// audio passthrough (ac-3/ec-3 CODECS for Safari-class clients), the
		// decision's encoder otherwise. Must mirror buildFingerprint or the
		// master would advertise codecs the segments don't carry.
		ChosenAudioCodec: servedAudioCodec(
			audioCopyEligible(decision,
				h.audioNormalizeGetter != nil && h.audioNormalizeGetter(r.Context()),
				len(profiles), sourceAudioCodec(streams, audioTrack)),
			sourceAudioCodec(streams, audioTrack), decision),
		ResumeOffsetSec: resumeSec,
		AudioRenditions: audioRenditions,
		CopyVideoCodec:  sourceCopyVideoCodec(streams),
		CopyVideoRange:  sourceCopyVideoRange(streams),
		// hdr=1 is a decision input (copy rung vs tonemap) exactly like the
		// codec lists: it MUST be re-baked into the variant/segment URLs or the
		// segment-time makeDecision silently downgrades an HDR copy rung to a
		// tonemapped h264 transcode that contradicts the master's CODECS.
		HDRCapable: r != nil && r.URL.Query().Get("hdr") == "1",
	}, nil
}

// sourceCopyVideoRange maps the source's HDR type to the HLS VIDEO-RANGE
// attribute the copy rung must advertise ("PQ"/"HLG"), or "" for SDR. Uses
// the same hdr.DetectHDR as the decision, so the attribute can't contradict
// the copy-vs-tonemap routing. Dolby Vision never reaches here as raw PQ
// unless its base layer is compatible (DetectHDR reports the base type).
func sourceCopyVideoRange(streams []*domainlibrary.MediaStream) string {
	for _, s := range streams {
		if s == nil || s.StreamType != "video" {
			continue
		}
		info := hdr.DetectHDR([]hdr.VideoStream{s})
		if !info.IsHDR {
			return ""
		}
		switch info.Type {
		case "hlg":
			return "HLG"
		case "hdr10", "hdr10+":
			return "PQ"
		}
		return "" // dolby_vision etc. — never served as copy, no attribute
	}
	return ""
}

// sourceCopyVideoCodec returns the RFC-6381 codec string (avc1.PPCCLL /
// hvc1.P.C.Lxxx.B0) to advertise for a copy rung of the source video stream,
// or "" when the source is not copy-eligible (the eligibility gates — level,
// HDR ground truth, DoVi policy — live in CopyVideoCodecString). The manifest
// only uses it for a rung whose HLSProfile.Copy is true.
func sourceCopyVideoCodec(streams []*domainlibrary.MediaStream) string {
	for _, s := range streams {
		if s == nil || s.StreamType != "video" {
			continue
		}
		// No copy rung without the keyframe index: the copy plan (segment cuts +
		// StartSec) is built from s.Keyframes, so an item that has not been
		// keyframe-backfilled yet must fall back to transcode rather than route to
		// copy and break on an empty plan. Critical now that the rung is on by
		// default — the worker backfills keyframes_ms only after level.
		if len(s.Keyframes) == 0 {
			return ""
		}
		return apptranscoding.CopyVideoCodecString(apptranscoding.CopySource{
			Codec:         s.Codec,
			Profile:       s.Profile,
			Level:         s.Level,
			TenBit:        streamIsTenBit(s),
			ColorTransfer: s.ColorTransfer,
			DoViProfile:   s.DoViProfile,
			DoViBLCompat:  s.DoViBLCompat,
		})
	}
	return ""
}

// audioCopyCodecs are the source codecs the muxed pipeline may pass through
// (`-c:a copy`). The DECISION already guarantees the client decodes the
// selected track (evaluateAudio checks client.AudioCodecs, which the web
// player fills from canPlayType — ac-3/ec-3 only appear on Safari/TV-class
// clients); this set additionally bounds passthrough to codecs whose fMP4
// muxing and RFC 6381 CODECS attribute (mp4a.40.2 / ac-3 / ec-3) are wired.
var audioCopyCodecs = map[string]bool{"aac": true, "ac3": true, "eac3": true}

// audioCopyEligible reports whether this request's audio is served as a
// passthrough. Gates on top of the decision's copy verdict:
//   - loudnorm requires a re-encode;
//   - single-rung ladders only (the multi-variant builder always re-encodes,
//     and one hash must yield one byte stream);
//   - the source codec must be in audioCopyCodecs.
//
// MUST stay the single source of truth for both the fingerprint (bytes) and
// the manifest (CODECS attribute): if they disagree, the master advertises a
// codec the segments don't carry and Safari refuses to play.
func audioCopyEligible(decision domaintranscoding.PlaybackDecision, normalize bool, profileCount int, srcCodec string) bool {
	return decision.AudioDecision == "copy" &&
		!normalize &&
		profileCount == 1 &&
		audioCopyCodecs[srcCodec]
}

// servedAudioCodec is the audio codec the segments will actually carry:
// the SOURCE codec on passthrough, the decision's encoder otherwise
// ("" → AAC, the historical default).
func servedAudioCodec(copyEligible bool, srcCodec string, decision domaintranscoding.PlaybackDecision) string {
	if copyEligible {
		return srcCodec
	}
	return decision.AudioCodec
}

// sourceAudioCodec returns the codec of the audio-relative track audioIdx
// (the Nth audio stream, ffmpeg's 0:a:N), lowercased, or "" when absent.
func sourceAudioCodec(streams []*domainlibrary.MediaStream, audioIdx int) string {
	n := 0
	for _, s := range streams {
		if s == nil || s.StreamType != "audio" {
			continue
		}
		if n == audioIdx {
			return strings.ToLower(s.Codec)
		}
		n++
	}
	return ""
}

// buildFingerprint reproduces the DecisionFingerprint from a segment-time
// request. Must match exactly what was produced at manifest time, or the
// cache lookup misses on every segment. Hence shared helpers.
func (h *hlsVODServe) buildFingerprint(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) (apptranscoding.DecisionFingerprint, error) {
	if item == nil || item.File == nil {
		return apptranscoding.DecisionFingerprint{}, errors.New("buildFingerprint: nil item")
	}
	decision := h.makeDecision(r.Context(), streams, item.File.ContainerFormat, r)
	profiles := h.profilesForDecision(r.Context(), item, streams, decision)

	audioTrack := h.resolveAudioTrack(r, streams)
	subBurnIn := parseIntQuery(r, "subtitleTrack", -1)

	// When the audio-rendition feature is on, the audio is served as a SEPARATE
	// HLS rendition (audio_T.m3u8 / audio_T_*.m4s) and the video segments MUST be
	// video-only. Otherwise stream_0 keeps muxing the audio track while the master
	// also advertises the separate audio group with DEFAULT=YES — hls.js then
	// appends audio twice (muxed-in-stream_0 + audio_0), which throws
	// bufferAppendError on any seek. Signalling "no audio" with track -1 reuses the
	// tested video-only ladder (hasAudio) and forks the cache (atrack=-1 in Hash())
	// so muxed and video-only segments for the same media never alias.
	videoAudioTrack := audioTrack
	if h.audioRenditionEnabled {
		videoAudioTrack = -1
	}

	fpProfiles := make([]apptranscoding.ProfileFingerprint, len(profiles))
	for i, p := range profiles {
		// Single, reflection-guarded projection (TestProfileFingerprintOf_CopiesEveryField).
		fpProfiles[i] = apptranscoding.ProfileFingerprintOf(p)
	}

	hwTonemap := h.hwTonemapWanted(decision)
	// Capability gate: AV1 falls back to H.264 when the host can't encode it (no
	// HW AV1, no calibrated AV1 software ceiling). videoCodec is what the segments
	// actually carry — the cache key MUST use it, not the raw decision codec.
	videoCodec, hwAccel, hwDevice, hwFeatures, hwUsable := h.effectiveVideoCodec(decision, streams)
	srcAudio := sourceAudioCodec(streams, audioTrack)
	audioCopy := audioCopyEligible(decision,
		h.audioNormalizeGetter != nil && h.audioNormalizeGetter(r.Context()),
		len(fpProfiles), srcAudio)
	return apptranscoding.DecisionFingerprint{
		MediaID:    mediaID,
		SourcePath: item.File.Path,
		VideoCodec: videoCodec,
		// The SOURCE codec (excluded from Hash — derivable from MediaID): the
		// copy builder tags the fMP4 sample entry from it (-tag:v hvc1).
		SourceVideoCodec: sourceVideoCodec(streams),
		// Legacy-codec corrections (B3): idet content verdict + anamorphic SAR,
		// both hashed conditionally so progressive/square titles keep their
		// cache. No-ops on modern progressive sources.
		DeinterlaceAll: sourceDeinterlaceAll(streams),
		SourceSAR:      sourceSAR(streams),
		// The codec the segments actually carry (source codec on passthrough).
		// The builder only reads it on the transcode branch; here it keeps the
		// hash truthful per served bytes.
		AudioCodec:          servedAudioCodec(audioCopy, srcAudio, decision),
		AudioTrack:          videoAudioTrack,
		SubtitleBurnInTrack: subBurnIn,
		// HDR->SDR tonemap is needed whenever the decision flagged it (HDR source +
		// non-HDR client, or Dolby Vision). Baked into the fingerprint so it (a)
		// enters the cache hash — tonemapped vs not are different bytes — and (b)
		// reaches the orchestrator/builder, which has no source streams to detect it.
		HDRTonemap:       slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed"),
		AudioNormalize:   h.audioNormalizeGetter != nil && h.audioNormalizeGetter(r.Context()),
		MaxAudioChannels: decision.MaxAudioChannels,
		// Audio passthrough eligibility — see audioCopyEligible (shared with
		// buildManifestInput so the master CODECS always matches the bytes).
		AudioCopy:        audioCopy,
		ForceSidecarSubs: decision.SubtitleDecision == "external",
		UseFMP4:          h.useFMP4,
		HWAccel:          hwAccel,
		HWDevice:         hwDevice,
		HWFeatures:       hwFeatures,
		// GPU tone-mapping for this request (only when the HW path is actually
		// taken). Excluded from Hash() like HWAccel — a HW- vs software-tonemapped
		// segment is interchangeable in the addressable cache.
		HWTonemap: hwTonemap && hwUsable,
		Profiles:  fpProfiles,
	}, nil
}

func (h *hlsVODServe) makeDecision(ctx context.Context, streams []*domainlibrary.MediaStream, container string, r *http.Request) domaintranscoding.PlaybackDecision {
	claims := HTTPClaimsFromContext(r.Context())
	prefs := loadProfilePrefs(ctx, h.profilePrefs, claims.ProfileID)
	// Inject the selected subtitle track so the decision only burns (and so only
	// denies the copy rung) when the user actually plays this bitmap subtitle —
	// not whenever the file merely carries one. Mirrors the subtitleTrack param
	// the fingerprint (buildFingerprint) and the burn-in filter already consume.
	prefs.SubtitleBurnInTrack = parseIntQuery(r, "subtitleTrack", -1)
	// Inject the audio track the request will actually serve, so the audio
	// copy-vs-transcode decision is made on THAT track (the default may be DTS
	// while the user plays an AAC track, or vice-versa). Mirrors the audioTrack
	// resolution the fingerprint and the ffmpeg -map already use.
	prefs.AudioTrack = h.resolveAudioTrack(r, streams)
	hdrCapable := r.URL.Query().Get("hdr") == "1"
	client := parseClientCapabilities(r, hdrCapable)
	d := apptranscoding.MakePlaybackDecisionWithPrefs(ctx, streams, container, client, 0, prefs)
	if d == nil {
		return domaintranscoding.PlaybackDecision{}
	}
	// A transcoded rung is SDR BY CONTRACT — the master only advertises
	// VIDEO-RANGE on the COPY rung. An HDR-capable client (hdr=1) legitimately
	// suppresses the decision-level tonemap for the raw-serve paths, but when
	// the copy rung is UNAVAILABLE (keyframes/HDR truth not yet backfilled,
	// non-wired codec/profile, copy producer off) this request still
	// re-encodes — and a PQ source x264-encoded without tonemapping renders
	// washed-out (proven on prod bytes: SATAVG 3.1 untonemapped vs the same
	// segment tonemapped). Re-add the tonemap for any HDR source that will
	// NOT be served as a raw copy. The disk guard can still flip copy→ladder
	// at serve time without re-adding it — accepted bounded edge (hashing
	// FreeBytes would be worse).
	servedAsRawCopy := d.VideoDecision == "copy" && h.copyProducer != nil && sourceCopyVideoCodec(streams) != ""
	if !servedAsRawCopy &&
		!slices.Contains(d.TranscodeReasons, "hdr_tonemapping_needed") &&
		sourceIsHDR(streams) {
		d.TranscodeReasons = append(d.TranscodeReasons, "hdr_tonemapping_needed")
		d.VideoDecision = "transcode"
		if d.VideoCodec == "" {
			d.VideoCodec = "h264"
		}
	}
	return *d
}

// sourceDeinterlaceAll reports whether the first video stream's idet content
// verdict says interlaced (the backfill's idet_interlaced = true). Drives the
// builder's deint=all fallback for sources whose container flags lie. nil
// verdict (not yet analyzed, or modern codec) ⇒ false ⇒ the unchanged
// flag-gated deinterlace.
func sourceDeinterlaceAll(streams []*domainlibrary.MediaStream) bool {
	for _, s := range streams {
		if s == nil || s.StreamType != "video" {
			continue
		}
		return s.IdetInterlaced != nil && *s.IdetInterlaced
	}
	return false
}

// sourceSAR returns the first video stream's sample aspect ratio ("64:45" for
// anamorphic DVD), or "" for square pixels / unprobed. Drives the builder's
// anamorphic scale correction.
func sourceSAR(streams []*domainlibrary.MediaStream) string {
	for _, s := range streams {
		if s == nil || s.StreamType != "video" {
			continue
		}
		if s.SampleAspectRatio != nil {
			return *s.SampleAspectRatio
		}
		return ""
	}
	return ""
}

// sourceIsHDR reports whether the first video stream is HDR, via the same
// hdr.DetectHDR the decision and the tonemap filter use.
func sourceIsHDR(streams []*domainlibrary.MediaStream) bool {
	for _, s := range streams {
		if s == nil || s.StreamType != "video" {
			continue
		}
		return hdr.DetectHDR([]hdr.VideoStream{s}).IsHDR
	}
	return false
}

// resolveHW decides the hardware encode backend for a chosen output codec.
// Returns ("", "", false) — the software path — unless the flag is on, the
// host calibrated a HW backend, and that backend can encode this codec. The
// boolean is both the ladder's hwAccelUsable (taller HW ceiling) and the
// signal that tags the fingerprint so the orchestrator runs the GPU pipeline.
// HDR tonemap / bitmap burn-in are not gated here: the orchestrated VOD path
// does neither today, and the ffmpeg builder independently falls back to
// software for those, so the two stay consistent.
// effectiveVideoCodec applies the capability gate to the decision's chosen
// output codec and returns the codec the segments will ACTUALLY carry together
// with the HW resolution for that codec. AV1 software (libsvtav1) is several
// times heavier than H.264 and the weak-NAS target can't sustain it, so when AV1
// is requested but the host has no HW AV1 encoder AND no calibrated AV1 software
// ceiling (SoftwareAV1MaxHeight == 0), fall back to H.264 rather than starve.
// Shared by buildFingerprint (the cache key) and the manifest builder (the
// advertised CODECS) so the master playlist can never advertise a codec the
// segments don't carry. Re-resolves HW for the fallback codec (H.264 may be
// HW-encodable where AV1 was not).
func (h *hlsVODServe) effectiveVideoCodec(decision domaintranscoding.PlaybackDecision, streams []*domainlibrary.MediaStream) (codec, accel, device string, features []string, usable bool) {
	hwTonemap := h.hwTonemapWanted(decision)
	srcCodec := sourceVideoCodec(streams)
	src10 := sourceVideoTenBit(streams)
	codec = decision.VideoCodec
	accel, device, features, usable = h.resolveHW(codec, srcCodec, src10, hwTonemap)
	// AV1 output must clear TWO gates, else fall back to H.264:
	//   1. the admin opt-in (transcoding.av1_enabled) — off by default; and
	//   2. a usable HW AV1 encoder. AV1 is hardware-only for streaming because
	//      libsvtav1 (software AV1) is several times heavier than H.264 and the
	//      weak-NAS target can't sustain it in realtime — it starves and the
	//      player rebuffers forever. The software-AV1 calibration stays for
	//      non-streaming/experimental use but never feeds a live HLS session.
	if strings.EqualFold(codec, "av1") && ((h.av1Enabled != nil && !h.av1Enabled()) || !usable) {
		codec = "h264"
		accel, device, features, usable = h.resolveHW(codec, srcCodec, src10, hwTonemap)
	}
	return codec, accel, device, features, usable
}

func (h *hlsVODServe) resolveHW(outputCodec, sourceCodec string, sourceTenBit, hwTonemap bool) (accel, device string, features []string, usable bool) {
	if h.hwAccelEnabled == nil || !h.hwAccelEnabled() || h.capability == nil {
		return "", "", nil, false
	}
	cap := h.capability()
	if !cap.HWSupports(outputCodec) {
		return "", "", nil, false
	}
	// The orchestrated pipeline HW-decodes the source (-hwaccel). If this GPU
	// can't decode the source codec — a pre-Skylake Intel iGPU decodes H.264 but
	// not HEVC — that decode fails with ENOSYS and the player stalls on rebuffer.
	// Fall back to full software for such a source rather than risk the stall.
	// H.264 sources (the common case) keep the full HW path. (A future decoupled
	// software-decode + hwupload path could keep HW *encode* even here.)
	if sourceCodec != "" && !cap.HWCanDecode(sourceCodec) {
		return "", "", nil, false
	}
	// A 10-bit source (HEVC Main10, ubiquitous at 2160p HDR) needs the HW VPP to
	// fold p010 down to the 8-bit nv12 the h264 encoder takes (hwScaleFilter's
	// format=nv12). Some drivers — Intel i965 VEBOX — abort (SIGABRT)
	// the moment a 10-bit surface reaches scale_vaapi. Unless the calibration
	// proved the 10-bit VPP works, software-transcode the source instead of
	// crashing the GPU pipeline and churning through a demotion.
	// Exception: when this request HW-tone-maps (hwTonemap), the GPU tone-mapper
	// (tonemap_vaapi / vpp_qsv) folds the 10-bit p010 down to 8-bit nv12 itself,
	// so the scale_vaapi-on-p010 hazard the gate guards against never arises —
	// the 10-bit source can take the HW path even without HW10BitTranscode.
	if sourceTenBit && !cap.HW10BitTranscode && !hwTonemap {
		return "", "", nil, false
	}
	return cap.HWBackend, cap.HWDevice, cap.HWFeatures, true
}

// hwTonemapWanted reports whether this request should tone-map HDR->SDR on the
// GPU: the decision needs tone-mapping, the toggle is on, the host probed
// HWTonemap, and the backend has a wired HW tone-map filter (vaapi/qsv — NVENC's
// libplacebo path is not modelled, so it tone-maps in software). When false the
// software tonemap path runs, byte-identical to a host with no GPU tone-mapping.
func (h *hlsVODServe) hwTonemapWanted(decision domaintranscoding.PlaybackDecision) bool {
	if h.hwTonemapEnabled != nil && !h.hwTonemapEnabled() {
		return false
	}
	if h.hwAccelEnabled == nil || !h.hwAccelEnabled() || h.capability == nil {
		return false
	}
	cap := h.capability()
	if !cap.HWTonemap || (cap.HWBackend != "vaapi" && cap.HWBackend != "qsv") {
		return false
	}
	return slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed")
}

// sourceVideoCodec returns the first video stream's codec (empty if none), the
// codec the pipeline must DECODE. Used to gate HW on decode capability.
func sourceVideoCodec(streams []*domainlibrary.MediaStream) string {
	for _, s := range streams {
		if s != nil && s.StreamType == domainlibrary.StreamTypeVideo {
			return s.Codec
		}
	}
	return ""
}

// sourceVideoTenBit reports whether the source's video stream is ≥10-bit
// (HEVC Main10 / yuv420p10le). Used to gate the HW path off when the calibrated
// backend's VPP can't fold 10-bit down to 8-bit (see EncodeCapability.HW10BitTranscode).
func sourceVideoTenBit(streams []*domainlibrary.MediaStream) bool {
	for _, s := range streams {
		if s != nil && s.StreamType == domainlibrary.StreamTypeVideo {
			return streamIsTenBit(s)
		}
	}
	return false
}

// streamIsTenBit detects ≥10-bit video from the probed bit depth or, since
// ffprobe leaves bits_per_raw_sample empty for many HEVC rips, the codec
// profile name ("Main 10", "High 10").
func streamIsTenBit(s *domainlibrary.MediaStream) bool {
	if s == nil {
		return false
	}
	if s.BitDepth != nil && *s.BitDepth >= 10 {
		return true
	}
	// Lower-cased to match hdr.DetectHDR's profile predicate (e.g. "Main 10",
	// "High 10") — the two ten-bit rules must agree across layers.
	return strings.Contains(strings.ToLower(s.Profile), "10")
}

// profilesForDecision picks the ABR ladder for a given (decision, mediaItem,
// streams) triple. Reads the per-title complexity class persisted by the
// background ComplexityWorker; falls back to LadderFilmNormal when the
// item hasn't been classified yet (safe: matches pre-refactor behaviour).
// Caps the ladder to the source resolution so we never advertise a 4K rung
// for a 1080p source.
func (h *hlsVODServe) profilesForDecision(ctx context.Context, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, decision domaintranscoding.PlaybackDecision) []domaintranscoding.HLSProfile {
	class := domaintranscoding.ClassFilmNormal
	if h.complexityRepo != nil && item != nil {
		resolved := false
		if h.classLookup != nil {
			if c, ok := h.classLookup.Get(string(item.ID)); ok {
				class, resolved = c, true
			}
		}
		if !resolved {
			if entry, found, err := h.complexityRepo.Get(ctx, string(item.ID)); err == nil && found && entry.Class.IsValid() {
				class = entry.Class
			}
			if h.classLookup != nil {
				// Cache the RESOLVED class (default included) so an unclassified
				// title doesn't re-query on every segment either.
				h.classLookup.Set(string(item.ID), class, mediaLookupTTL)
			}
		}
	}
	// COMMON HEAD (shared by both producers): pick the per-title ladder. The
	// PRODUCER's pure Profiles() owns the tail — source cap + height ceiling +
	// (software) single-rendition collapse — so the manifest-time and
	// segment-time shaping is identical and routes through the SAME producer
	// (the cache-hash invariant: drift here = permanent cache miss).
	base := domaintranscoding.LadderFor(class)
	src := sourceDimsOf(streams)

	capability := domaintranscoding.SafeFallback()
	if h.capability != nil {
		capability = h.capability()
	}

	var laddered []domaintranscoding.HLSProfile
	switch {
	case h.isCopyRequest(decision, streams, sourceBytesOf(item)):
		// Copy/remux rung: a single source-resolution rung, no transcode ladder
		// (the player never ABR-switches a copy stream). Same producer the
		// segment path resolves, so manifest- and segment-time shaping agree.
		laddered = h.copyProducer.Profiles(base, src, decision, capability)
	case h.selector != nil:
		// Production path: the wired selector routes to the software or hardware
		// producer (reusing resolveHW), and the producer's pure Profiles() owns
		// the shaping tail. This is the single source of truth for ladder shape;
		// manifest-time and segment-time both reach it via the same selector.
		laddered = h.selector.For(decision, src).Profiles(base, src, decision, capability)
	default:
		// No selector wired (hand-constructed test handlers). Reproduce the
		// legacy shaping so unit tests that probe profilesForDecision directly
		// keep their contract without standing up a full producer pair.
		laddered = h.legacyProfileShaping(base, src, streams, decision, capability)
	}

	// CMCD-feedback adaptive override (Phase C). Variant-neutral: it only lowers
	// the ceiling on a source-class showing sustained starvation (MIN with the
	// benchmark cap), never raises it. Kept in the handler (not the pure
	// Profiles) because the override is mutable runtime state read from
	// playback_qoe_events. Applied AFTER the producer shaped the ladder so it
	// trims the result identically at manifest- and segment-time (the override
	// value is stable across a request pair).
	if h.capOverride != nil && src.Height > 0 {
		if ov, ok := h.capOverride(sourceVideoCodec(streams), src.Height); ok && ov.MaxHeight > 0 {
			laddered = domaintranscoding.CapLadderToHeight(laddered, ov.MaxHeight)
		}
	}

	// AV1 max-quality cap: when AV1 is the EFFECTIVE output codec, an admin can
	// hold its ladder below the source/GPU ceiling (transcoding.av1_max_height,
	// 0 = no cap). `decision` here still carries the raw preferred codec, so
	// recompute the post-gate codec. Applied here (like the CMCD override) so the
	// manifest- and segment-time ladders trim identically — the cache-hash
	// invariant holds because both reach this shared head with the same inputs.
	if h.av1MaxHeight != nil {
		if m := h.av1MaxHeight(); m > 0 {
			if ec, _, _, _, _ := h.effectiveVideoCodec(decision, streams); strings.EqualFold(ec, "av1") {
				laddered = domaintranscoding.CapLadderToHeight(laddered, m)
			}
		}
	}
	return laddered
}

// sourceDimsOf extracts the first video stream's resolution + 10-bit flag.
// Streams may be missing (degenerate file) — zero Width/Height is handled by
// CapLadderToSource as "unknown, don't cap". Width/Height are *int on the
// domain type so a missing probe doesn't conflate "unknown" with "zero pixels".
func sourceDimsOf(streams []*domainlibrary.MediaStream) apptranscoding.SourceDims {
	var dims apptranscoding.SourceDims
	for _, s := range streams {
		if s != nil && s.StreamType == "video" {
			if s.Width != nil {
				dims.Width = *s.Width
			}
			if s.Height != nil {
				dims.Height = *s.Height
			}
			dims.Codec = s.Codec
			dims.TenBit = streamIsTenBit(s)
			break
		}
	}
	return dims
}

// producerForRequest resolves the StreamProducer (software vs hardware) for the
// current request. It recomputes the decision + source dims the same way the
// manifest/fingerprint path does, so the producer chosen for a segment matches
// the one the ladder was shaped for (and thus the cache hash). Falls back to
// the orchestrator field when no selector is wired (hand-built test handlers);
// returns nil only when neither is set.
func (h *hlsVODServe) producerForRequest(ctx context.Context, r *http.Request, streams []*domainlibrary.MediaStream, item *domainlibrary.MediaItem) apptranscoding.StreamProducer {
	if h.selector != nil {
		decision := h.makeDecision(ctx, streams, containerOf(item), r)
		if h.isCopyRequest(decision, streams, sourceBytesOf(item)) {
			return h.copyProducer
		}
		return h.selector.For(decision, sourceDimsOf(streams))
	}
	if h.orchestrator != nil {
		return h.orchestrator
	}
	return nil
}

// sourceBytesOf is the on-disk size of the source file, or 0 when unknown. The
// copy-rung disk guard treats 0 as "can't size it" → fail-open (allow copy).
func sourceBytesOf(item *domainlibrary.MediaItem) int64 {
	if item == nil || item.File == nil {
		return 0
	}
	return item.File.Size
}

// containerOf is item.File.ContainerFormat, guarded against a nil file.
func containerOf(item *domainlibrary.MediaItem) string {
	if item == nil || item.File == nil {
		return ""
	}
	return item.File.ContainerFormat
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

// legacyProfileShaping reproduces the ladder tail for hand-built test handlers
// that have no wired selector. Production NEVER takes this path — it routes
// through h.selector to the real software/hardware producer (whose pure
// Profiles owns the shaping). This mirrors SoftwareProducer.Profiles exactly
// (single-rendition collapses to ONE rung at the sustainable software ceiling,
// maxH), so the test-only fallback can't drift from the production shape.
func (h *hlsVODServe) legacyProfileShaping(base []domaintranscoding.HLSProfile, src apptranscoding.SourceDims, streams []*domainlibrary.MediaStream, decision domaintranscoding.PlaybackDecision, capability domaintranscoding.EncodeCapability) []domaintranscoding.HLSProfile {
	capped := domaintranscoding.CapLadderToSource(base, src.Width, src.Height)

	hwUsable := false
	maxH := 720
	if h.capability != nil {
		_, _, _, hwUsable = h.resolveHW(decision.VideoCodec, sourceVideoCodec(streams), src.TenBit, h.hwTonemapWanted(decision))
		maxH = capability.MaxHeightFor(hwUsable)
		if !hwUsable && capability.SoftwareHDRMaxHeight > 0 &&
			capability.SoftwareHDRMaxHeight < maxH &&
			slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed") {
			maxH = capability.SoftwareHDRMaxHeight
		}
	}
	// Mirrors SoftwareProducer.Profiles: the natural ladder capped to the
	// sustainable ceiling (with its 360p floor), no single-rendition collapse,
	// and the top surviving rung inheriting the first dropped rung's bitrate
	// budget when the ceiling (not the source) trimmed the ladder.
	heightCapped := domaintranscoding.CapLadderToHeight(capped, maxH)
	return domaintranscoding.BoostCappedTopRung(capped, heightCapped)
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

// MediaSourceResolver implements apptranscoding.SourceResolver on top of
// MediaStreamsGetter. The orchestrator calls it when it needs the file path
// and streams to feed the ffmpeg adapter; the handler doesn't go through it
// (it already has the resolved media item to also build the manifest).
type MediaSourceResolver struct {
	LibSvc MediaStreamsGetter
}

func (r *MediaSourceResolver) ResolveSource(ctx context.Context, fp apptranscoding.DecisionFingerprint) (string, []apptranscoding.SourceStream, error) {
	if r == nil || r.LibSvc == nil {
		return "", nil, errors.New("MediaSourceResolver: LibSvc nil")
	}
	item, err := r.LibSvc.GetMediaItem(ctx, fp.MediaID)
	if err != nil {
		return "", nil, fmt.Errorf("ResolveSource: %w", err)
	}
	if item.File == nil || item.File.Path == "" {
		return "", nil, errors.New("ResolveSource: media has no file")
	}
	// Phase 1: streams not needed at orchestrator level — the ffmpeg adapter's
	// defaults cover the video+audio path. Phase 3 will plumb real streams for
	// burn-in subtitle / HDR tonemap branches.
	return item.File.Path, nil, nil
}

// copyStreamReaderAdapter / copyItemReaderAdapter bridge the handler's
// MediaStreamsGetter to the CopyPlanner's reader ports (the planner wants a
// typed MediaItemID and different method names). Cheap pass-throughs.
type copyStreamReaderAdapter struct{ g MediaStreamsGetter }

func (a copyStreamReaderAdapter) GetByMediaItemID(ctx context.Context, id shared.MediaItemID) ([]*domainlibrary.MediaStream, error) {
	return a.g.GetMediaStreams(ctx, string(id))
}

type copyItemReaderAdapter struct{ g MediaStreamsGetter }

func (a copyItemReaderAdapter) GetByID(ctx context.Context, id shared.MediaItemID) (*domainlibrary.MediaItem, error) {
	return a.g.GetMediaItem(ctx, string(id))
}

// newHLSVODServe binds the handler to the orchestrator + cache that wire.go
// has already built, and constructs the software producer + the selector that
// routes each request between them. Returns nil if a prerequisite is missing so
// the caller skips route registration cleanly.
// resolveServerFlag returns the effective value of an operational (non-secret)
// flag: a server_config DB row wins, else the process env var, else "". Mirrors
// cmd/evey.resolveFlag so transcode toggles are flippable from
// /admin/server-config (a restart is still required — flags are read at boot).
// Both the dotted DB key (canonical) and the raw env-var name are accepted as
// DB keys. cfgRepo may be nil (pure-env fallback).
func resolveServerFlag(cfgRepo shared.ServerConfigRepository, dbKey, envVar string) string {
	if cfgRepo != nil {
		ctx := context.Background()
		for _, k := range [...]string{dbKey, envVar} {
			if entry, err := cfgRepo.Get(ctx, k); err == nil && entry != nil && entry.Value != "" {
				return entry.Value
			}
		}
	}
	return os.Getenv(envVar)
}

func newHLSVODServe(s *HTTPServer, libSvc MediaStreamsGetter, libAccess LibraryAccessChecker, parental ParentalChecker) *hlsVODServe {
	_ = libAccess // wired through commonAuth via s.LibraryAccessRepo
	if s == nil || s.HLSVODOrch == nil || s.HLSVODCache == nil {
		return nil
	}
	cache := s.HLSVODCache
	orch := s.HLSVODOrch
	// Lazy-expiry hot-path caches (interval 0 → no sweeper goroutine; the LRU
	// cap bounds memory, expiry is checked on Get).
	mediaLookup, _ := pkgcache.NewWithMaxSize[cachedMediaLookup](0, mediaLookupMax)
	classLookup, _ := pkgcache.NewWithMaxSize[domaintranscoding.ContentClass](0, mediaLookupMax)
	h := &hlsVODServe{
		mediaLookup:          mediaLookup,
		classLookup:          classLookup,
		tokens:               s.TokenGen,
		libSvc:               libSvc,
		libraryAccess:        libAccess,
		parental:             parental,
		profilePrefs:         s.ProfilePrefs,
		audioNormalizeGetter: s.AudioNormalizeGetter,
		userMaxStreams:       s.UserMaxStreams,
		manifestGen:          apptranscoding.NewManifestGenerator(),
		cache:                cache,
		orchestrator:         orch,
		qoeSink:              s.QoESink,
		complexityRepo:       s.ComplexityRepo,
		capability: func() domaintranscoding.EncodeCapability {
			if s.CalibrationSvc == nil {
				return domaintranscoding.SafeFallback()
			}
			return s.CalibrationSvc.Current()
		},
		capOverride: func(sourceCodec string, sourceHeight int) (domaintranscoding.CapOverride, bool) {
			if s.CapOverrideSvc == nil {
				return domaintranscoding.CapOverride{}, false
			}
			return s.CapOverrideSvc.OverrideForHeight(sourceCodec, sourceHeight)
		},
		hwAccelEnabled: func() bool {
			return s.Cfg != nil && s.Cfg.EffectiveHWAccelEnabled()
		},
		av1Enabled: func() bool {
			return s.Cfg != nil && s.Cfg.EffectiveAV1Enabled()
		},
		av1MaxHeight: func() int {
			if s.Cfg == nil {
				return 0
			}
			return s.Cfg.EffectiveAV1MaxHeight()
		},
		// On by default; an operator who sees HW tone-mapping shift colours on
		// their GPU (a known VAAPI/driver hazard) sets EVEY_TRANSCODING_HW_TONEMAP=0
		// to fall back to the software tonemap.
		hwTonemapEnabled: func() bool {
			return os.Getenv("EVEY_TRANSCODING_HW_TONEMAP") != "0"
		},
		// fMP4 is now the project default — matches the ffmpeg adapter
		// (needsFMP4Segments returns true unconditionally) and Apple's
		// post-WWDC-2024 stance on CMAF. Per-rendition behaviour stays
		// uniform across all codecs (H.264 / HEVC / AV1 / VP9).
		useFMP4: true,
		// 25 s covers a worst-case ffmpeg cold start at -ss far into the
		// media (the slow probe + initial decode + 1st keyframe + first
		// segment encode chain has been measured at 11-14 s on the dev
		// box). hls.js retries 503 automatically so a tighter cap would
		// just produce visible buffering.
		segmentWaitTimeout: 25 * time.Second,
		// Audio rendition switching is gated behind a config flag for the
		// rollout. Off (default) preserves the muxed pipeline; on flips
		// buildManifestInput to populate AudioRenditions from the source
		// audio streams and the master starts emitting EXT-X-MEDIA.
		audioRenditionEnabled: s.Cfg != nil && s.Cfg.Transcoding.AudioRenditionEnabled,
		serverLang: func() string {
			if s.Cfg == nil {
				return "en"
			}
			return s.Cfg.Metadata.Language
		},
	}

	// Build the software producer: ONE long-lived, input-paced (-readrate 1.5),
	// single-rendition ffmpeg per DecisionHash — the Jellyfin model for a weak
	// NAS with hw_accel off (no multi-session burst-ahead, no global capacity
	// cap, no per-segment deadline kill). It shares the addressable SegmentCache
	// with the orchestrator (segments are encoder-agnostic), and resolves the
	// source path through the same MediaSourceResolver the orchestrator uses.
	// Prefer the shared instance wired in wire.go: the gRPC StopStream / admin-
	// kill remover references the SAME producer, so a stopped software stream is
	// torn down promptly instead of lingering until idle GC. Hand-built test
	// servers leave HLSVODSoftware nil → fall back to a private instance.
	software := s.HLSVODSoftware
	if software == nil {
		software = apptranscoding.NewSoftwareProducer(
			cache,
			s.HLSTranscoder,
			&MediaSourceResolver{LibSvc: libSvc},
			apptranscoding.SoftwareProducerOptions{}, // defaults: ReadRate 1.5, 30 min GC, 60 s wait budget
			slog.Default(),
		)
	}

	// Copy/remux rung = a dedicated SoftwareProducer in copy mode: the Jellyfin
	// remux model — one paced `-c:v copy` + AAC session, seek = restart at the
	// plan's keyframe. A distinct instance from the transcode `software` above
	// (copyPlanner is per-instance) sharing the same cache; the Copy flag in the
	// fingerprint keeps copy and transcode segments from aliasing.
	//
	// DEFAULT ON (opt-out: server_config transcoding.remux_copy=off or
	// EVEY_REMUX_COPY=off, applied on restart). The road here, for the record:
	// (1) a seek session's -copyts elst poisoned the shared init (fixed:
	// make_zero, one canonical init); (2) the libx264-on-seek init downgrade
	// (fixed: CopyOnSeek); (3) ffmpeg's hls muxer splits on a session-relative
	// n×hls_time grid → numbering drifted minutes from the playlist (fixed:
	// every-keyframe plan + -hls_time 0 + 8x pacing); (4) -ss lands on the
	// preceding cue/DTS-heuristic point, not the requested keyframe (fixed:
	// seek-landing probe + SeekTargetSec); (5) accurate-seek discarded the
	// audio between landing and target while labeling it at the landing →
	// audio early by a GOP (fixed: -noaccurate_seek). Validated end-to-end
	// 2026-06-10: prod segments byte-aligned with the playlist in every seek
	// region (video at the playlist position to the ms, audio in the same
	// segment within ~60 ms) + a clean user listen test (start, deep seeks,
	// seek-back). A request still only routes here when it is copy-eligible
	// AND keyframe-backfilled (isCopyRequest → sourceCopyVideoCodec).
	if resolveServerFlag(s.ServerCfgRepo, "transcoding.remux_copy", "EVEY_REMUX_COPY") != "off" {
		softwareCopy := apptranscoding.NewSoftwareProducer(
			cache,
			s.HLSTranscoder,
			&MediaSourceResolver{LibSvc: libSvc},
			apptranscoding.SoftwareProducerOptions{},
			slog.Default(),
		)
		softwareCopy.SetCopyMode(apptranscoding.NewCopyPlanner(
			copyStreamReaderAdapter{libSvc}, copyItemReaderAdapter{libSvc},
		), s.CopySeekProber)
		h.copyProducer = softwareCopy
		slog.Default().Info("hls vod: copy/remux rung enabled (default; disable with server_config transcoding.remux_copy=off or EVEY_REMUX_COPY=off)")
	}

	// On-demand copy rung (mp4.PlanHLS): supersedes the ffmpeg copy producer for
	// video+audio copy-eligible sources that package verbatim — no ffmpeg, no
	// fMP4 rebase, no session state, DASH included. DEFAULT ON, same opt-out
	// model as the copy rung above (server_config transcoding.hls_ondemand=off
	// or EVEY_HLS_ONDEMAND=off, applied on restart). When off, or on a source it
	// can't package, requests fall through to the copy producer / transcode
	// ladder exactly as before.
	if s.OnDemandPackager != nil &&
		resolveServerFlag(s.ServerCfgRepo, "transcoding.hls_ondemand", "EVEY_HLS_ONDEMAND") != "off" {
		h.onDemand = s.OnDemandPackager
		slog.Default().Info("hls vod: on-demand copy rung enabled (mp4.PlanHLS, default; disable with server_config transcoding.hls_ondemand=off or EVEY_HLS_ONDEMAND=off)")

		// AES-128 encryption of the on-demand segments (HLS-only, no DASH).
		// Off by default; needs the derivation secret. Only meaningful with the
		// rung enabled, hence nested.
		if len(s.HLSAESSecret) > 0 &&
			resolveServerFlag(s.ServerCfgRepo, "transcoding.hls_ondemand_encrypt", "EVEY_HLS_ONDEMAND_ENCRYPT") == "on" {
			h.onDemandEncrypt = true
			h.hlsAESSecret = s.HLSAESSecret
			slog.Default().Info("hls vod: on-demand AES-128 encryption enabled (HLS only; no DASH manifest)")
		}
	}

	// The selector routes each request: software unless resolveHW confirms the
	// hardware path is usable end-to-end for this content. hwUsableForContent
	// reuses the handler's resolveHW + hwTonemapWanted so the routing matches
	// exactly what tags the fingerprint (HW vs SW segments share the cache).
	h.selector = apptranscoding.NewProducerSelector(
		software,
		orch,
		func(decision domaintranscoding.PlaybackDecision, src apptranscoding.SourceDims) bool {
			_, _, _, usable := h.resolveHW(decision.VideoCodec, src.Codec, src.TenBit, h.hwTonemapWanted(decision))
			return usable
		},
	)

	// Shutdown teardown for the producers built HERE: the software producer and
	// the optional copy producer own long-lived ffmpeg sessions that would
	// otherwise outlive the process as orphans (their shutdownCtx is detached
	// from the server ctx by design — a request must never inherit a dying
	// ctx). The hardware orchestrator is excluded: main already defers its
	// CleanupAll (alias of Close, not idempotent).
	copyProd := h.copyProducer // may be nil (flag off)
	s.transcodeCloser = func() {
		software.Close()
		if copyProd != nil {
			copyProd.Close()
		}
	}

	return h
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
		slog.Warn("prewarm: producer EnsureSegmentRange failed",
			slog.String("next_id", string(nextItem.ID)), slog.Any("err", err))
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

// ingestCMCD parses CMCD (CTA-5004) from the request and forwards the event
// to the QoE sink. No-op when CMCD is absent or the sink isn't wired (dev /
// tests). Resolves user_id from the request context — anonymous plays
// (shared link, no token) end up with an empty user_id, which is fine.
func (h *hlsVODServe) ingestCMCD(r *http.Request, mediaID string, variantIdx, segmentIdx int, isInit, isManifest, isVariantPL bool) {
	if h.qoeSink == nil {
		return
	}
	payload, ok := cmcd.FromRequest(r)
	if !ok {
		return
	}
	eveymetrics.QoEIngestEvents.Inc()

	var userID string
	if claims := HTTPClaimsFromContext(r.Context()); claims != nil {
		userID = claims.UserID
	}

	// Debug-level: CMCD ingest fires once per beacon (per segment, per stream),
	// so Info would flood prod logs. The Phase 1 rollout is over — the QoE SQL
	// table and the /admin/qoe dashboard are confirmed working — so this drops
	// from Info to Debug (kept for on-demand troubleshooting, not steady-state).
	slog.Debug("hls-vod: CMCD ingest",
		slog.String("media_id", mediaID),
		slog.String("user_id", userID),
		slog.Int("variant", variantIdx),
		slog.Int("seg", segmentIdx),
		slog.Bool("init", isInit),
		slog.Bool("manifest", isManifest),
		slog.Bool("variant_pl", isVariantPL),
		slog.String("sid", payload.SessionID),
		slog.Int("buffer_ms", payload.BufferLengthMs),
		slog.Int("encoded_kbps", payload.EncodedKbps),
		slog.Int("measured_kbps", payload.MeasuredKbps),
		slog.Bool("starved", payload.BufferStarved),
		slog.Bool("startup", payload.Startup))

	h.qoeSink.Record(appqoe.Event{
		UserID:      userID,
		MediaItemID: mediaID,
		VariantIdx:  variantIdx,
		SegmentIdx:  segmentIdx,
		IsInit:      isInit,
		IsManifest:  isManifest,
		IsVariantPL: isVariantPL,
		DeviceClass: appqoe.DeviceClass(r.UserAgent()),
		RemoteIP:    appqoe.RemoteIP(r.RemoteAddr),
		Payload:     payload,
	})
}

// notifyProducerHint forwards CMCD telemetry (buffer length, starvation flag,
// current segment cursor) to the resolved producer's NotifyClientHint so the
// throttle / prefetch / pacing policy can react in real time. Read-only on the
// HTTP path — never blocks, never errors. No-op when CMCD is absent or no
// producer resolved.
//
// NEVER touches which variant is served — the player owns rendition
// selection. The feedback loop only adjusts encoder pacing.
func (h *hlsVODServe) notifyProducerHint(producer apptranscoding.StreamProducer, r *http.Request, mediaID string, segmentIdx int) {
	if producer == nil {
		return
	}
	payload, ok := cmcd.FromRequest(r)
	if !ok {
		return
	}
	// Bare-minimum signals: buffer length (ms) and starvation flag drive
	// the policy. Object type "v"/"av" matters most (video pipeline),
	// audio-only fetches can still bump clientHighWater forward harmlessly.
	producer.NotifyClientHint(mediaID, payload.BufferLengthMs, payload.BufferStarved, segmentIdx)
}
