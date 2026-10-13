package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/config"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
)

// AudioSegmentEnsurer is the minimal interface the audio handlers need from
// AudioSegmentOrchestrator. Using an interface allows tests to inject a fake
// without spinning up a real ffmpeg process.
type AudioSegmentEnsurer interface {
	EnsureAudioSegment(ctx context.Context, fp apptranscoding.AudioFingerprint, segIdx int) error
	EnsureAudioInit(ctx context.Context, fp apptranscoding.AudioFingerprint) error
}

// audioStartAware is the optional half of the ensurer that takes the viewer's
// position: the real orchestrator has it, test fakes need not.
type audioStartAware interface {
	EnsureAudioInitAt(ctx context.Context, fp apptranscoding.AudioFingerprint, startSeg int) error
	PreWarmAudio(fp apptranscoding.AudioFingerprint, segIdx int)
}

// audioServe groups the shared dependencies for the three audio-rendition
// HTTP handlers (playlist / init / segment). Constructed once and shared.
type audioServe struct {
	cfg           *config.Config
	libSvc        MediaStreamsGetter
	libraryAccess LibraryAccessChecker
	parental      ParentalChecker
	manifestGen   *apptranscoding.ManifestGenerator
	cache         *apptranscoding.SegmentCache
	audioOrch     AudioSegmentEnsurer
	// startHint is the segment the master request said playback starts at
	// (transcode_start_hint.go), 0 when unknown.
	startHint func(mediaID string) int

	// audioNormalizeGetter mirrors the video pipeline knob.
	audioNormalizeGetter func(ctx context.Context) bool
	// audioEncoderIDGetter names the concrete ffmpeg AAC implementation this
	// host encodes with; nil → native, the historical bytes.
	audioEncoderIDGetter func() string

	// segmentWaitTimeout caps how long serveSegmentFile blocks waiting for
	// an audio segment to materialise on disk.
	segmentWaitTimeout time.Duration
}

// audioRenditionEnabled reports whether the audio-rendition feature flag is on.
// When off, the serve paths (playlist/init/segment) return 404 immediately -
// the master manifest never advertises EXT-X-MEDIA entries in that case, so
// well-behaved players won't hit them. Fail-closed for misbehaving or probing
// clients.
func (h *audioServe) audioRenditionEnabled() bool {
	return h.cfg != nil && h.cfg.Transcoding.AudioRenditionEnabled
}

// wasmAudio reports whether THIS request is really on the wasm-audio path -
// the same answer the master handler and the fingerprint read, so the three
// can never disagree (see wasmAudioEffective).
func (h *audioServe) wasmAudio(r *http.Request) bool {
	return wasmAudioEffective(r, h.cfg != nil && h.cfg.Transcoding.CastWasmAudioEnabled)
}

// servePlaylistFile is the auth-and-render path for the audio playlist,
// called from the central hlsVODServe dispatcher (regex-routed via
// /hls/{file}) - the on-the-wire URL the manifest emits resolves relative to
// /api/v1/media/{id}/hls/master.m3u8, so the dispatcher is the actual
// production code path.
func (h *audioServe) servePlaylistFile(w http.ResponseWriter, r *http.Request, mediaID string, trackIdx int, videoCompositionDelay float64, copyCodec string) {
	if !h.audioRenditionEnabled() && !h.wasmAudio(r) {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}

	claims := HTTPClaimsFromContext(r.Context())
	item, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil || item == nil || item.File == nil || item.File.Path == "" {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		return
	}
	if !checkParental(r.Context(), w, claims, item, h.parental) {
		return
	}
	if !checkPlayable(w, item) {
		return
	}

	streams, err := h.libSvc.GetMediaStreams(r.Context(), mediaID)
	if err != nil {
		logInternal(r.Context(), "audio playlist: get media streams", err)
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}

	if item.File.Duration.Milliseconds() <= 0 {
		w.Header().Set("Retry-After", "5")
		http.Error(w, `{"message":"media duration not yet probed, retry"}`, http.StatusServiceUnavailable)
		return
	}

	audioRenditions := buildAudioRenditions(streams)
	if len(audioRenditions) == 0 {
		http.Error(w, `{"message":"no audio renditions"}`, http.StatusNotFound)
		return
	}

	token := RawTokenFromContext(r.Context())
	in := apptranscoding.ManifestInput{
		MediaID:     mediaID,
		DurationSec: float64(item.File.Duration.Milliseconds()) / 1000.0,
		// Profiles must be non-empty for ManifestInput.Validate().
		// AudioPlaylist doesn't use the profiles list but Validate() requires
		// at least one entry. We inject a placeholder that matches what a
		// real call would produce; the manifest output only reads segments.
		Profiles:            placeholderProfiles(),
		StreamToken:         token,
		AudioTrack:          parseIntQuery(r, "audioTrack", 0),
		SubtitleBurnInTrack: parseIntQuery(r, "subtitleTrack", -1),
		UseFMP4:             true,
		AudioRenditions:     audioRenditions,
		// wasm-audio passthrough: bake wasmAudio=1 into the segment/init URIs so
		// their fingerprint matches this playlist's copy init (see field doc).
		WasmAudio: h.wasmAudio(r),
		// The plan's pins too: the init and segments re-decide the video to
		// shift onto its timeline, and without vd= a transcoded rung's audio was
		// decided copy there and served unshifted (measured 2026-09-24: the
		// playlist asked 1 frame, the segments came out at 0).
		PinnedVideoDecision: r.URL.Query().Get("vd"),
		PinnedAudioDecision: r.URL.Query().Get("ad"),
		PinnedVideoCodec:    r.URL.Query().Get("vc"),
	}

	// The client's declared capabilities, the same way the master bakes them
	// (bakeClientInputs): this playlist's init and segments must re-decide for
	// the client the master was decided for - dropped here, the opt-in was
	// lost and the segments came out AAC under a master advertising Opus.
	bakeClientInputs(r, &in)
	out, err := h.manifestGen.AudioPlaylist(in, trackIdx)
	if err != nil {
		logInternal(r.Context(), "audio playlist: manifest", err)
		http.Error(w, `{"message":"manifest error"}`, http.StatusInternalServerError)
		return
	}

	setCORSHeaders(w, r, "")
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(out))

	slog.Info("audio rendition: playlist served",
		slog.String("media_id", mediaID),
		slog.Int("track_idx", trackIdx))

	// The player's next request is the segment at its position: start encoding it now.
	if pw, ok := h.audioOrch.(audioStartAware); ok {
		if seg := h.hintFor(mediaID); seg > 0 {
			pw.PreWarmAudio(h.buildAudioFingerprint(r, mediaID, item, trackIdx, videoCompositionDelay, copyCodec), seg)
		}
	}
}

func (h *audioServe) hintFor(mediaID string) int {
	if h.startHint == nil {
		return 0
	}
	return h.startHint(mediaID)
}

func (h *audioServe) serveInitFile(w http.ResponseWriter, r *http.Request, mediaID string, trackIdx int, videoCompositionDelay float64, copyCodec string) {
	if !h.audioRenditionEnabled() && !h.wasmAudio(r) {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}

	claims := HTTPClaimsFromContext(r.Context())
	item, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil || item == nil || item.File == nil || item.File.Path == "" {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		return
	}
	if !checkParental(r.Context(), w, claims, item, h.parental) {
		return
	}
	if !checkPlayable(w, item) {
		return
	}

	fp := h.buildAudioFingerprint(r, mediaID, item, trackIdx, videoCompositionDelay, copyCodec)

	// The init describes the codec, not a position: it is served off whatever
	// session is live. Asking for segment 0 here (as this once did) makes every
	// deep resume mute - the session a resume spawns starts at the viewer's
	// position and never writes segment 0.
	waitCtx, cancel := context.WithTimeout(r.Context(), h.segmentWaitTimeout)
	defer cancel()
	ensureInit := h.audioOrch.EnsureAudioInit
	if at, ok := h.audioOrch.(audioStartAware); ok {
		ensureInit = func(ctx context.Context, fp apptranscoding.AudioFingerprint) error {
			return at.EnsureAudioInitAt(ctx, fp, h.hintFor(mediaID))
		}
	}
	if err := ensureInit(waitCtx, fp); err != nil {
		slog.WarnContext(r.Context(), "audio rendition: ensure init failed",
			slog.String("media_id", mediaID),
			slog.Int("track_idx", trackIdx),
			slog.Any("err", err))
		http.Error(w, `{"message":"audio transcode unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	key := apptranscoding.SegmentKey{
		DecisionHash: fp.Hash(),
		VariantIdx:   trackIdx,
		IsInit:       true,
		Kind:         apptranscoding.KindAudio,
	}
	path := h.cache.Path(key, true)
	if _, statErr := os.Stat(path); statErr != nil {
		http.Error(w, `{"message":"audio init segment not yet ready, retry"}`, http.StatusServiceUnavailable)
		return
	}

	setCORSHeaders(w, r, "")
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, path)
}

func (h *audioServe) serveSegmentFile(w http.ResponseWriter, r *http.Request, mediaID string, trackIdx, segIdx int, videoCompositionDelay float64, copyCodec string) {
	if !h.audioRenditionEnabled() && !h.wasmAudio(r) {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}

	claims := HTTPClaimsFromContext(r.Context())
	item, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil || item == nil || item.File == nil || item.File.Path == "" {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		return
	}
	if !checkParental(r.Context(), w, claims, item, h.parental) {
		return
	}
	if !checkPlayable(w, item) {
		return
	}

	fp := h.buildAudioFingerprint(r, mediaID, item, trackIdx, videoCompositionDelay, copyCodec)

	waitCtx, cancel := context.WithTimeout(r.Context(), h.segmentWaitTimeout)
	defer cancel()
	if err := h.audioOrch.EnsureAudioSegment(waitCtx, fp, segIdx); err != nil {
		// Past the proven end of the source audio stream (and no silence tail
		// possible), the segment will NEVER exist: 410 tells the player to stop
		// retrying. A 503 here made hls.js hammer the endpoint, each retry
		// respawning a doomed ffmpeg (92 spawns for one segment, prod 07-26).
		if errors.Is(err, apptranscoding.ErrAudioPastEndOfStream) {
			slog.InfoContext(r.Context(), "audio rendition: segment past end of source stream",
				slog.String("media_id", mediaID),
				slog.Int("track_idx", trackIdx),
				slog.Int("seg_idx", segIdx))
			http.Error(w, `{"message":"audio stream ends before this segment"}`, http.StatusGone)
			return
		}
		slog.WarnContext(r.Context(), "audio rendition: ensure segment failed",
			slog.String("media_id", mediaID),
			slog.Int("track_idx", trackIdx),
			slog.Int("seg_idx", segIdx),
			slog.Any("err", err))
		http.Error(w, `{"message":"audio transcode unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	key := apptranscoding.SegmentKey{
		DecisionHash: fp.Hash(),
		VariantIdx:   trackIdx,
		SegmentIdx:   segIdx,
		Kind:         apptranscoding.KindAudio,
	}
	path := h.cache.Path(key, true)
	if _, statErr := os.Stat(path); statErr != nil {
		http.Error(w, `{"message":"audio segment not yet ready, retry"}`, http.StatusServiceUnavailable)
		return
	}

	setCORSHeaders(w, r, "")
	w.Header().Set("Content-Type", "video/iso.segment")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, path)
}

// buildAudioFingerprint constructs the AudioFingerprint for an HTTP request.
// AudioCodec is "aac" (safe on every client) and AudioBitrate carries the
// STEREO BASE - BuildFFmpegAudioOnlyArgs scales it to the channel count with the
// same rule as the muxed ladder (~64 kbps/channel beyond stereo, 5.1 -> 384k).
//
// MaxAudioChannels comes from the requested SOURCE track: ffmpeg reads 0 as
// `-ac 2`, so leaving it unset would downmix a 5.1 AC-3 track to stereo. The
// muxed path this rung replaces preserves the layout, and the hybrid rung is
// meant to serve the source's audio, not a quietly degraded one. Both fields are
// hashed, so a channel change forks the cache instead of aliasing.
//
// A CLIENT-declared ceiling (?maxAudioChannels) wins over the source layout,
// exactly as it does on the muxed path. Reading only the source is what made a
// cast receiver's stereo fallback a placebo: it renegotiated the master down to
// 2, the rendition kept serving the source's 5.1, and the reload failed the
// same way as the load it was rescuing (2026-08-08, Chromecast HD).
func (h *audioServe) buildAudioFingerprint(r *http.Request, mediaID string, item *domainlibrary.MediaItem, trackIdx int, videoCompositionDelay float64, copyCodec string) apptranscoding.AudioFingerprint {
	// wasm-audio passthrough: a Chrome/Firefox-class client that runs the
	// ac3go wasm decoder asks (?wasmAudio=1) for the source
	// AC-3/E-AC-3 served VERBATIM instead of re-encoded to AAC. AudioCodec="copy"
	// makes BuildFFmpegAudioOnlyArgs stream-copy it (no -b:a/-ac/-ar/-af, no
	// loudnorm) - zero audio CPU on the server. Distinct AudioCodec forks the
	// segment cache, so the AAC and passthrough renditions never alias. Gated on
	// the source actually being AC-3/E-AC-3; anything else keeps the AAC path.
	// Transport, not encode input (never hashed): lets the orchestrator bound
	// the silence tail it synthesizes when the track ends before this duration.
	durationSec := float64(item.File.Duration.Milliseconds()) / 1000.0
	// The rendition is cut on the video's grid, whichever pipeline serves the
	// video: audio segment N must cover video segment N.
	grid := h.sourceGrid(r.Context(), mediaID)

	// Native passthrough (copyCodec, decided by the VOD handler for this
	// client and track) and the wasm passthrough share one fingerprint shape:
	// the same verbatim samples, whoever decodes them.
	if copyCodec != "" {
		return apptranscoding.AudioFingerprint{
			Grid:                  grid,
			MediaID:               mediaID,
			SourcePath:            item.File.Path,
			SourceFingerprint:     item.File.Fingerprint,
			SourceTrackIdx:        trackIdx,
			AudioCodec:            "copy",
			SourceAudioCodec:      strings.ToLower(copyCodec),
			VideoCompositionDelay: videoCompositionDelay,
			SourceDurationSec:     durationSec,
		}
	}
	if h.wasmAudio(r) {
		if c := strings.ToLower(h.sourceAudioCodecForTrack(r.Context(), mediaID, trackIdx)); c == "ac3" || c == "eac3" {
			return apptranscoding.AudioFingerprint{
				Grid:              grid,
				MediaID:           mediaID,
				SourcePath:        item.File.Path,
				SourceFingerprint: item.File.Fingerprint,
				SourceTrackIdx:    trackIdx,
				AudioCodec:        "copy",
				SourceAudioCodec:  c,
				// The shift is the video MUXER's, not the codec's: a passthrough
				// rendition rides the same timeline as an AAC one.
				VideoCompositionDelay: videoCompositionDelay,
				// A stream copy takes no bitrate/channel/normalize input - they are
				// left zero so the arg builder emits a pure passthrough and the hash
				// stays stable.
				SourceDurationSec: durationSec,
			}
		}
	}
	sourceChannels := h.sourceAudioChannels(r.Context(), mediaID, trackIdx)
	return apptranscoding.AudioFingerprint{
		Grid:              grid,
		MediaID:           mediaID,
		SourcePath:        item.File.Path,
		SourceFingerprint: item.File.Fingerprint,
		SourceTrackIdx:    trackIdx,
		AudioCodec:        "aac",
		// Same encoder the muxed ladder uses, and hashed with the bytes: a
		// rendition re-encoded by a different AAC implementation is different
		// bytes and must not land on the same cache entry.
		AudioEncoder:        h.audioEncoderID(),
		AudioBitrate:        domaintranscoding.AudioRenditionBaseKbps, // scaled to the channel count at arg-build
		AudioNormalize:      h.audioNormalizeGetter != nil && h.audioNormalizeGetter(r.Context()),
		MaxAudioChannels:    renditionChannels(sourceChannels, maxAudioChannelsQuery(r)),
		SourceAudioChannels: sourceChannels,
		SourceDurationSec:   durationSec,
		// Non-zero only when the video is packaged by the ffmpeg copy fallback
		// instead of mkvgo - see transcode_audio_sync.go.
		VideoCompositionDelay: videoCompositionDelay,
	}
}

// sourceAudioCodecForTrack returns the codec of the trackIdx-th AUDIO stream
// (the `ffmpeg -map 0:a:N` numbering buildAudioRenditions advertises), lowercased
// and empty when the streams can't be read. Mirrors sourceAudioChannels so the
// passthrough gate reads the same track numbering the rest of the audio path uses.
func (h *audioServe) sourceAudioCodecForTrack(ctx context.Context, mediaID string, trackIdx int) string {
	if h.libSvc == nil {
		return ""
	}
	streams, err := h.libSvc.GetMediaStreams(ctx, mediaID)
	if err != nil {
		return ""
	}
	audioIdx := 0
	for _, s := range streams {
		if s == nil || s.StreamType != domainlibrary.Audio {
			continue
		}
		if audioIdx == trackIdx {
			return s.Codec
		}
		audioIdx++
	}
	return ""
}

// renditionChannels folds the client's declared ceiling into the source layout.
// A ceiling of 0 (nothing declared - every web player) keeps today's behaviour:
// the source layout, untouched. A source of 0 (unknown layout) is never guessed
// upward, so it stays ffmpeg's stereo default.
func renditionChannels(sourceChannels, clientMax int) int {
	if clientMax <= 0 {
		return sourceChannels
	}
	if sourceChannels <= 0 {
		return sourceChannels
	}
	return min(sourceChannels, clientMax)
}

// maxRenditionChannels is the highest channel count a browser will decode as AAC
// (5.1). A 7.1 source is folded down to it rather than shipped un-decodable.
const maxRenditionChannels = 6

// sourceAudioChannels returns the channel count of the trackIdx-th AUDIO stream
// (the `ffmpeg -map 0:a:N` numbering buildAudioRenditions advertises). Returns 0
// - ffmpeg's stereo default, the previous behaviour - when the streams can't be
// read or the source never reported a channel count, so an unknown layout is
// never guessed.
// sourceGrid is the segmentation grid of the media's video (SourceGrid),
// the legacy clock when the streams cannot be read.
func (h *audioServe) sourceGrid(ctx context.Context, mediaID string) domaintranscoding.SegmentGrid {
	if h.libSvc == nil {
		return domaintranscoding.SegmentGrid{}
	}
	streams, err := h.libSvc.GetMediaStreams(ctx, mediaID)
	if err != nil {
		return domaintranscoding.SegmentGrid{}
	}
	return apptranscoding.SourceGrid(streams)
}

func (h *audioServe) sourceAudioChannels(ctx context.Context, mediaID string, trackIdx int) int {
	if h.libSvc == nil {
		return 0
	}
	streams, err := h.libSvc.GetMediaStreams(ctx, mediaID)
	if err != nil {
		slog.WarnContext(ctx, "audio rendition: stream lookup failed, defaulting to stereo",
			slog.String("media_id", mediaID), slog.Any("err", err))
		return 0
	}
	return renditionSourceChannels(audioStreamAt(streams, trackIdx))
}

// renditionSourceChannels is the source layout a re-encoded rendition starts
// from: 0 when unknown (never guessed), capped at 5.1 - no browser decodes
// AAC 7.1 in MSE (Chrome tops out at 5.1). Emitting `-ac 8` for a Dolby Atmos
// 7.1 source made the SourceBuffer refuse the stream and the title never
// started - the opposite failure to the stereo downmix this derivation replaced.
func renditionSourceChannels(s *domainlibrary.MediaStream) int {
	if s == nil || s.Channels == nil || *s.Channels <= 0 {
		return 0
	}
	return min(*s.Channels, maxRenditionChannels)
}

// audioStreamAt is the trackIdx-th AUDIO stream (`ffmpeg -map 0:a:N`), nil
// when absent.
func audioStreamAt(streams []*domainlibrary.MediaStream, trackIdx int) *domainlibrary.MediaStream {
	audioIdx := 0
	for _, s := range streams {
		if s == nil || s.StreamType != domainlibrary.Audio {
			continue
		}
		if audioIdx == trackIdx {
			return s
		}
		audioIdx++
	}
	return nil
}

// renditionShape is the sample rate, channel count and bitrate a rendition's
// fragments carry, from the inputs buildAudioFingerprint builds it from: the
// source track's own on a copy (copyCodec != ""), the AAC encode's otherwise.
// 0 = unknown.
func renditionShape(s *domainlibrary.MediaStream, copyCodec string, clientMaxChannels int) (sampleRate, channels int, kbps int64) {
	if copyCodec != "" {
		if s == nil {
			return 0, 0, 0
		}
		if s.SampleRate != nil && *s.SampleRate > 0 {
			sampleRate = *s.SampleRate
		}
		if s.Channels != nil && *s.Channels > 0 {
			channels = *s.Channels
		}
		if s.Bitrate != nil && *s.Bitrate > 0 {
			kbps = (*s.Bitrate + 999) / 1000
		}
		return sampleRate, channels, kbps
	}
	channels = domaintranscoding.ResolvedAudioChannels(renditionChannels(renditionSourceChannels(s), clientMaxChannels))
	return 48000, channels, domaintranscoding.ServedAudioBitrateKbps(domaintranscoding.AudioRenditionBaseKbps, channels)
}

// buildAudioRenditions converts the raw media streams into the
// AudioRendition slice the ManifestGenerator expects. Only audio streams are
// included; the SourceTrackIdx is the index AMONG AUDIO streams (not the
// overall ffmpeg stream index) - this matches `ffmpeg -map 0:a:N` semantics
// so the audio orchestrator hands ffmpeg the right specifier. Name falls
// back to the language code (upper-cased) when the source track has no
// title; empty Language is kept empty so the master playlist omits the
// LANGUAGE attribute entirely (some strict players reject LANGUAGE="").
//
// The source-flagged default track (IsDefault) is bubbled to index 0 so
// EXT-X-MEDIA emits DEFAULT=YES/AUTOSELECT=YES on it. Falls back to source
// order when no track is flagged default.
func buildAudioRenditions(streams []*domainlibrary.MediaStream) []apptranscoding.AudioRendition {
	var out []apptranscoding.AudioRendition
	audioIdx := 0
	defaultIdx := -1
	for _, s := range streams {
		if s == nil || s.StreamType != domainlibrary.Audio {
			continue
		}
		name := s.Title
		if name == "" {
			name = strings.ToUpper(s.Language)
		}
		out = append(out, apptranscoding.AudioRendition{
			SourceTrackIdx: audioIdx,
			Name:           name,
			LanguageCode:   s.Language,
		})
		if defaultIdx < 0 && s.IsDefault {
			defaultIdx = len(out) - 1
		}
		audioIdx++
	}
	if defaultIdx > 0 {
		out[0], out[defaultIdx] = out[defaultIdx], out[0]
	}
	return out
}

// placeholderProfiles returns a minimal single-profile slice that satisfies
// ManifestInput.Validate() (which requires len(Profiles) > 0). AudioPlaylist
// does not use the profile data in its output.
func placeholderProfiles() []domaintranscoding.HLSProfile {
	return []domaintranscoding.HLSProfile{
		{Name: "placeholder", Width: 1280, Height: 720, VideoBitrate: 2000, AudioBitrate: 128},
	}
}

// audioEncoderID is the nil-safe read of the encoder probe: unwired keeps the
// native encoder, byte-identical and hash-identical to before the feature.
func (h *audioServe) audioEncoderID() string {
	if h.audioEncoderIDGetter == nil {
		return ""
	}
	return h.audioEncoderIDGetter()
}
