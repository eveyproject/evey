package httpapi

// transcode_vod_wire.go constructs hlsVODServe and its small reader
// adapters used to wire the copy/remux path to the library service.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	pkgcache "github.com/evey-project/evey/internal/pkg/cache"
)

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
	// Phase 1: streams not needed at orchestrator level - the ffmpeg adapter's
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
// flagEnabled reads an operator flag's raw value against a default.
//
// The raw string used to be compared to the literals "on"/"off", while the admin
// panel persists every other boolean with strconv.FormatBool ("true"/"false") -
// so the two never met: `hls_ondemand = "false"` did NOT disable it, and
// `hls_ondemand_concat = "true"` did NOT enable it. An operator could believe a
// path was off while it ran. Accept the spellings people actually write; anything
// unparseable keeps the code's default rather than silently flipping a path.
func flagEnabled(raw string, defaultOn bool) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "on", "true", "1", "yes", "enabled":
		return true
	case "off", "false", "0", "no", "disabled":
		return false
	default:
		return defaultOn
	}
}

// serverFlagEnabled resolves an operator flag (DB override, then env) against a
// default. Prefer this over comparing resolveServerFlag's raw string.
func serverFlagEnabled(cfgRepo shared.ServerConfigRepository, dbKey, envVar string, defaultOn bool) bool {
	return flagEnabled(resolveServerFlag(cfgRepo, dbKey, envVar), defaultOn)
}

// resolveServerFlag returns the effective RAW value of an operational (non-secret)
// flag: a server_config DB row wins, else the process env var, else "". Both the
// dotted DB key (canonical) and the raw env-var name are accepted as DB keys.
// cfgRepo may be nil (pure-env fallback). For booleans, go through
// serverFlagEnabled - comparing this string by hand is how "true" ended up not
// meaning "on".
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
	prefsLookup, _ := pkgcache.NewWithMaxSize[apptranscoding.TranscodingPreferences](0, mediaLookupMax)
	h := &hlsVODServe{
		keyframeWarmer:        s.KeyframeWarmer,
		videoAdmission:        videoAdmissionOf(s.VideoLimiter),
		admissionRoute:        admissionRouteOf(s.VideoAdmissionShaper),
		admissionPinnedClass:  pinnedClassOf(s.VideoAdmissionShaper),
		admissionBind:         admissionBindOf(s.VideoAdmissionShaper),
		admissionFrozenLadder: frozenLadderOf(s.VideoAdmissionShaper),
		admissionFreezeLadder: freezeLadderOf(s.VideoAdmissionShaper),
		// Per-stream hash bookkeeping: pure in-memory, no dependency, so it is
		// always on. reclaimHash is attached below, once both producers exist.
		streamHashes:          apptranscoding.NewStreamHashRegistry(0, 0),
		mediaLookup:           mediaLookup,
		classLookup:           classLookup,
		prefsLookup:           prefsLookup,
		libSvc:                libSvc,
		libraryAccess:         libAccess,
		parental:              parental,
		streamTokens:          s.StreamTokenStore,
		compositionDelayProbe: s.VideoCompositionDelayProbe,
		castProgress:          s.CastProgress,
		profilePrefs:          s.ProfilePrefs,
		audioNormalizeGetter:  s.AudioNormalizeGetter,
		audioEncoderIDGetter:  s.AudioEncoderIDGetter,
		userMaxStreams:        s.UserMaxStreams,
		// Started with the server so it stops with it; nil when there is no
		// data dir, in which case Enqueue is a no-op.
		subtitlePrewarm: startSubtitlePrewarmer(s.ServerCtx, newSubtitleCache(s.DataDir), newSubtitleIndexCache(s.DataDir)),
		manifestGen:     apptranscoding.NewManifestGenerator(),
		cache:           cache,
		orchestrator:    orch,
		qoeSink:         s.QoESink,
		complexityRepo:  s.ComplexityRepo,
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
		// fMP4 is now the project default - matches the ffmpeg adapter
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
		castWasmAudioEnabled:  s.Cfg != nil && s.Cfg.Transcoding.CastWasmAudioEnabled,
		copyFallbackRung:      s.Cfg != nil && s.Cfg.Transcoding.CopyFallbackRungEnabled,
		serverLang: func() string {
			if s.Cfg == nil {
				return "en"
			}
			return s.Cfg.Metadata.Language
		},
	}

	// Build the software producer: ONE long-lived, input-paced (-readrate 1.5),
	// single-rendition ffmpeg per DecisionHash - the sane model for a weak
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

	// Copy/remux rung = a dedicated SoftwareProducer in copy mode: the paced
	// remux model - one `-c:v copy` + AAC session, seek = restart at the
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
	if serverFlagEnabled(s.ServerCfgRepo, "transcoding.remux_copy", "EVEY_REMUX_COPY", true) {
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
	// video+audio copy-eligible sources that package verbatim - no ffmpeg, no
	// fMP4 rebase, no session state, DASH included. DEFAULT ON, same opt-out
	// model as the copy rung above (server_config transcoding.hls_ondemand=off
	// or EVEY_HLS_ONDEMAND=off, applied on restart). When off, or on a source it
	// can't package, requests fall through to the copy producer / transcode
	// ladder exactly as before.
	if s.OnDemandPackager != nil &&
		serverFlagEnabled(s.ServerCfgRepo, "transcoding.hls_ondemand", "EVEY_HLS_ONDEMAND", true) {
		h.onDemand = s.OnDemandPackager
		// Paired with the packager on purpose: the tracker only has meaning
		// while this rung can serve, and leaving it wired with the rung off
		// would report playbacks that no longer take this path.
		h.playbackActivity = s.PlaybackActivity
		slog.Default().Info("hls vod: on-demand copy rung enabled (mp4.PlanHLS, default; disable with server_config transcoding.hls_ondemand=off or EVEY_HLS_ONDEMAND=off)")

		// AES-128 encryption of the on-demand segments (HLS-only, no DASH).
		// Off by default; needs the derivation secret. Only meaningful with the
		// rung enabled, hence nested.
		if len(s.HLSAESSecret) > 0 &&
			serverFlagEnabled(s.ServerCfgRepo, "transcoding.hls_ondemand_encrypt", "EVEY_HLS_ONDEMAND_ENCRYPT", false) {
			h.onDemandEncrypt = true
			h.hlsAESSecret = s.HLSAESSecret
			slog.Default().Info("hls vod: on-demand AES-128 encryption enabled (HLS only; no DASH manifest)")

			// Optional AES-128 key rotation (forward secrecy). Only meaningful
			// with encryption on, hence nested. 0/absent = single key per media.
			if n, err := strconv.Atoi(resolveServerFlag(s.ServerCfgRepo, "transcoding.hls_key_rotation_segments", "EVEY_HLS_KEY_ROTATION_SEGMENTS")); err == nil && n > 0 {
				h.hlsKeyRotateSegments = n
				slog.Default().Info("hls vod: on-demand AES-128 key rotation enabled", slog.Int("every_segments", n))
			}
		}

		// Track trim: one audio rendition per master instead of every track.
		// Off by default; only meaningful with the on-demand rung itself on.
		if serverFlagEnabled(s.ServerCfgRepo, "transcoding.hls_ondemand_track_trim", "EVEY_HLS_ONDEMAND_TRACK_TRIM", false) {
			h.onDemandTrackTrim = true
			slog.Default().Info("hls vod: on-demand track trim enabled (single resolved audio track per master; disable with server_config transcoding.hls_ondemand_track_trim=off or EVEY_HLS_ONDEMAND_TRACK_TRIM=off)")
		}

		// Gapless multi-episode session (mp4.PlanConcat), opt-in PER REQUEST
		// (?gapless=1) - and the web player already ships the toggle that sends
		// it, in 6 languages. The server gate was opt-in too, so that toggle did
		// nothing: default it ON, like the rung it rides on. Still bounded - an
		// episode that isn't fully copy-eligible just falls back to the normal
		// per-episode master (concat v1 has no mid-chain ffmpeg-audio hybrid, so
		// it reuses the copy-rung eligibility gates directly).
		if s.OnDemandConcatPackager != nil &&
			serverFlagEnabled(s.ServerCfgRepo, "transcoding.hls_ondemand_concat", "EVEY_HLS_ONDEMAND_CONCAT", true) {
			h.onDemandConcat = s.OnDemandConcatPackager
			slog.Default().Info("hls vod: gapless multi-episode concat enabled (mp4.PlanConcat, opt-in ?gapless=1; disable with server_config transcoding.hls_ondemand_concat=off or EVEY_HLS_ONDEMAND_CONCAT=off)")
		}
	}

	// The selector routes each request: software unless resolveHW confirms the
	// hardware path is usable end-to-end for this content. hwUsableForContent
	// reuses the handler's resolveHW + hwTonemapWanted so the routing matches
	// exactly what tags the fingerprint (HW vs SW segments share the cache).
	h.selector = apptranscoding.NewProducerSelector(software, orch, h.hwRouteUsable)

	// Fan the superseded-hash reclaim across BOTH encoding producers: a stream
	// can switch track while its encoder lives on either route (a hw stream
	// spilled to software, or the reverse after a demotion), and only the one
	// actually holding the hash has anything to kill. The copy producer is
	// deliberately excluded - a remux takes no video-encode slot, so tearing it
	// down would cost a viewer their playback and free nothing.
	h.reclaimHash = func(hash, reason string) int {
		return software.RemoveHash(hash, reason) + orch.RemoveHash(hash, reason)
	}

	// Shutdown teardown for the producers built HERE: the software producer and
	// the optional copy producer own long-lived ffmpeg sessions that would
	// otherwise outlive the process as orphans (their shutdownCtx is detached
	// from the server ctx by design - a request must never inherit a dying
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
