package transcoding

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/hdr"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
)

// StreamSpec is the lean value object the playback-decision pipeline operates
// on. It carries only the stream metadata the algorithm actually inspects
// (codec, dimensions, channels, bitrate, HDR, default flag) and decouples
// decision.go from the much larger library.MediaStream entity. The conversion
// from library.MediaStream happens once at the entry point of
// MakePlaybackDecisionWithPrefs via toStreamSpecs; every internal evaluator
// then consumes StreamSpec.
type StreamSpec struct {
	Type      string // "video" | "audio" | "subtitle"
	Codec     string
	Width     *int
	Height    *int
	HDRType   *string
	Channels  *int
	Bitrate   *int64
	IsDefault bool
	// Commentary marks a director/cast commentary audio track. The auto-selection
	// (selectedAudioSpec) prefers a non-commentary default over a commentary default
	// when both exist; an explicit AudioTrack selection always wins regardless.
	Commentary bool
}

// toStreamSpecs adapts the persistence-flavored library.MediaStream slice
// into the lean StreamSpec slice the decision pipeline consumes. Keeping this
// adapter at the package boundary lets the public API stay backward-compatible
// (call sites still pass []*library.MediaStream) while the internal logic only
// depends on the value object.
func toStreamSpecs(streams []*library.MediaStream) []StreamSpec {
	out := make([]StreamSpec, 0, len(streams))
	for _, s := range streams {
		if s == nil {
			continue
		}
		// MediaStream.HDRType is never populated by the prober (it stores
		// color_space only), so derive it from the colour metadata via the same
		// hdr.DetectHDR the tonemap path uses - one source of truth. Without this
		// the decision's HDR branch never fires and an HDR title is never seen as
		// HDR (so an HDR-capable client never gets the HDR-aware direct-play/
		// tonemap path).
		hdrType := s.HDRType
		if s.StreamType == library.StreamTypeVideo {
			if info := hdr.DetectHDR([]hdr.VideoStream{s}); info.IsHDR {
				t := info.Type
				hdrType = &t
			}
		}
		out = append(out, StreamSpec{
			Type:       string(s.StreamType),
			Codec:      s.Codec,
			Width:      s.Width,
			Height:     s.Height,
			HDRType:    hdrType,
			Channels:   s.Channels,
			Bitrate:    s.Bitrate,
			IsDefault:  s.IsDefault,
			Commentary: s.Commentary,
		})
	}
	return out
}

// TranscodingPreferences are client-side playback prefs persisted in
// profile.preferences (JSONB) and enforced by the server when computing
// PlaybackDecision.
//
// Empty / zero values mean "auto" (use existing defaults). MaxBitrateKbps is
// the baseline cap; MaxBitrateCellularKbps overrides it when NetworkType ==
// "cellular" and is > 0 (lets mobile clients save data on cellular).
//
// AudioLanguages and Locale drive server-side default audio track selection:
// when the client does not specify an explicit audioTrack query param, the
// handler picks the first audio track whose language matches the preference
// list (AudioLanguages → Locale → server language → IsDefault flag → 0).
// These fields come from profile.audio_languages / profile.locale (not the
// JSONB preferences map) and are injected by the HTTP layer before the
// fingerprint is computed.
type TranscodingPreferences struct {
	PreferredVideoCodec string // "", "auto", or e.g. "h264", "hevc", "av1"
	PreferredAudioCodec string // "", "auto", or e.g. "aac", "ac3", "eac3", "opus"
	MaxAudioChannels    int    // 0 = unlimited, 2 = force stereo, 6 = 5.1, etc.
	SubtitleBurnIn      string // "", "auto", "always", "never"
	// SubtitleBurnInTrack is the subtitle-relative index of the track the client
	// SELECTED for display (the Nth subtitle stream, matching ffmpeg's 0:s:N and
	// the `subtitleTrack` query param), or -1 when none is selected. The decision
	// burns - and so forces a video transcode - ONLY for this track when it is a
	// bitmap sub. Without it the decision would burn whenever ANY bitmap track
	// merely EXISTS, needlessly transcoding (and denying the copy/remux rung)
	// every Blu-ray rip even when the user plays with no subtitle. This is a
	// request-scoped field injected by the transport layer (like AudioLanguages /
	// Locale), not a profile pref - ParseTranscodingPreferences leaves it at the
	// -1 default.
	SubtitleBurnInTrack int
	// AudioTrack is the audio-relative index of the track the client actually
	// PLAYS (the Nth audio stream, matching ffmpeg's 0:a:N and the `audioTrack`
	// query param), or -1 to fall back to the container's default-flag track.
	// The audio decision (copy vs transcode) must be made on THIS track: the
	// default may be DTS while the user selects an AAC track (or vice-versa),
	// and judging the wrong track either re-encodes copyable audio or serves a
	// codec the client cannot decode. Request-scoped, injected by the transport
	// layer like SubtitleBurnInTrack; ParseTranscodingPreferences leaves it -1.
	AudioTrack int
	// WasmAudio: the client decodes AC-3/E-AC-3 itself. Request-scoped like
	// AudioTrack; it cannot ride the supported-codec list (MSE refuses those
	// codecs muxed), so it needs its own flag.
	WasmAudio              bool
	AllowDirectPlay        bool
	AllowDirectStream      bool
	AllowTranscoding       bool
	MaxBitrateKbps         int
	MaxBitrateCellularKbps int
	NetworkType            string // "wifi" (default) or "cellular"
	// AudioLanguages is the ordered list of preferred audio track languages
	// for this profile (e.g. ["fr","en"]). Used only for default-track
	// resolution; does not affect codec or bitrate decisions.
	AudioLanguages []string
	// Locale is the UI locale of the profile (e.g. "fr", "en"). Used as a
	// lower-priority fallback when AudioLanguages is empty or yields no match.
	Locale string
}

// DefaultTranscodingPreferences returns prefs with all flags enabled (client defaults).
// SubtitleBurnInTrack defaults to -1 ("no subtitle selected"): the zero value 0
// would otherwise mean "the first subtitle track is selected" and re-introduce
// burn-on-existence for every call site that doesn't inject an explicit selection.
func DefaultTranscodingPreferences() TranscodingPreferences {
	return TranscodingPreferences{
		SubtitleBurnInTrack: -1,
		AudioTrack:          -1,
		AllowDirectPlay:     true,
		AllowDirectStream:   true,
		AllowTranscoding:    true,
	}
}

// EffectiveMaxBitrateKbps returns the bitrate cap to apply given the current
// network. Cellular cap is preferred only when explicitly set and smaller than
// the general cap (or when no general cap exists).
func (p TranscodingPreferences) EffectiveMaxBitrateKbps() int {
	if p.NetworkType == "cellular" && p.MaxBitrateCellularKbps > 0 {
		if p.MaxBitrateKbps <= 0 || p.MaxBitrateCellularKbps < p.MaxBitrateKbps {
			return p.MaxBitrateCellularKbps
		}
	}
	return p.MaxBitrateKbps
}

// ParseTranscodingPreferences extracts transcoding prefs from a profile.Preferences map.
// Missing keys default to permissive values (allow_* = true, codec = auto).
func ParseTranscodingPreferences(prefs map[string]string) TranscodingPreferences {
	tp := DefaultTranscodingPreferences()
	if prefs == nil {
		return tp
	}
	if v, ok := prefs["preferred_video_codec"]; ok {
		tp.PreferredVideoCodec = strings.ToLower(strings.TrimSpace(v))
	}
	if v, ok := prefs["preferred_audio_codec"]; ok {
		tp.PreferredAudioCodec = strings.ToLower(strings.TrimSpace(v))
	}
	if v, ok := prefs["max_audio_channels"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			tp.MaxAudioChannels = n
		}
	}
	if v, ok := prefs["subtitle_burn_in"]; ok {
		tp.SubtitleBurnIn = strings.ToLower(strings.TrimSpace(v))
	}
	if v, ok := prefs["allow_direct_play"]; ok {
		tp.AllowDirectPlay = parseBool(v, true)
	}
	if v, ok := prefs["allow_direct_stream"]; ok {
		tp.AllowDirectStream = parseBool(v, true)
	}
	if v, ok := prefs["allow_transcoding"]; ok {
		tp.AllowTranscoding = parseBool(v, true)
	}
	if v, ok := prefs["max_bitrate_kbps"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			tp.MaxBitrateKbps = n
		}
	}
	if v, ok := prefs["max_bitrate_cellular_kbps"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			tp.MaxBitrateCellularKbps = n
		}
	}
	return tp
}

func parseBool(s string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	}
	return def
}

func MakePlaybackDecision(
	ctx context.Context,
	streams []*library.MediaStream,
	containerFormat string,
	client transcoding.ClientProfile,
	maxBitrateKbps int,
	sourcePath ...string,
) *transcoding.PlaybackDecision {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.MakePlaybackDecision")
	defer span.End()
	span.SetAttributes(attribute.String("container.format", containerFormat))
	return MakePlaybackDecisionWithPrefs(ctx, streams, containerFormat, client, maxBitrateKbps, DefaultTranscodingPreferences(), sourcePath...)
}

// playabilityShadow is the optional mkvgo ops.Playability shadow comparer
// wired by SetPlayabilityShadow. nil (the default) makes the hook at the end
// of MakePlaybackDecisionWithPrefs an inert no-op - the decision engine above
// it is identical either way. See playability_shadow.go.
var playabilityShadow *PlayabilityShadow

// SetPlayabilityShadow wires (or, with nil, disables) the shadow comparison
// that runs after every MakePlaybackDecisionWithPrefs call for which a
// sourcePath was supplied. Call sites that don't pass sourcePath are
// unaffected either way - Observe no-ops on an empty path.
func SetPlayabilityShadow(s *PlayabilityShadow) {
	playabilityShadow = s
}

// MakePlaybackDecisionWithPrefs is the full entry point that also honors user prefs.
// MakePlaybackDecision delegates here with default (permissive) prefs for backward compat.
//
// The public signature still accepts []*library.MediaStream so the cascade of
// call sites (playback_handler.go and friends) stays untouched. Internally we
// convert once to []StreamSpec via toStreamSpecs and every evaluator below
// works on that lean VO - keeping the algorithm decoupled from the library
// entity it used to depend on field-by-field.
//
// sourcePath is variadic and OPTIONAL so every existing call site keeps
// compiling unmodified: passing the media file's path as the trailing
// argument opts that call site into the playability shadow comparison (see
// SetPlayabilityShadow); an absent or empty path leaves it a no-op.
func MakePlaybackDecisionWithPrefs(
	ctx context.Context,
	streams []*library.MediaStream,
	containerFormat string,
	client transcoding.ClientProfile,
	maxBitrateKbps int,
	prefs TranscodingPreferences,
	sourcePath ...string,
) *transcoding.PlaybackDecision {
	_, span := tracing.StartSpan(ctx, "app.transcoding.MakePlaybackDecisionWithPrefs")
	defer span.End()
	span.SetAttributes(
		attribute.String("container.format", containerFormat),
		attribute.Int("streams.count", len(streams)),
	)
	d := &transcoding.PlaybackDecision{
		PlayMethod:       "direct_play",
		VideoDecision:    "copy",
		AudioDecision:    "copy",
		SubtitleDecision: "",
		TranscodeReasons: []string{},
	}

	specs := toStreamSpecs(streams)

	// Apply cellular cap if the client signaled it: prefs.EffectiveMaxBitrateKbps
	// returns the cellular value when NetworkType=="cellular" and a cap is set.
	if cellCap := prefs.EffectiveMaxBitrateKbps(); cellCap > 0 && (maxBitrateKbps <= 0 || cellCap < maxBitrateKbps) {
		maxBitrateKbps = cellCap
	}
	effectiveMaxBitrate := resolveMaxBitrate(client.MaxBitrate, maxBitrateKbps)
	totalBitrate := sumBitrates(specs)

	// subIdx is the subtitle-relative index (0,1,2…) so it lines up with the
	// client's selection (prefs.SubtitleBurnInTrack) and ffmpeg's 0:s:N.
	subIdx := 0
	for i := range specs {
		s := specs[i]
		switch s.Type {
		case "video":
			evaluateVideo(d, s, client)
		case "subtitle":
			evaluateSubtitle(d, s, prefs, subIdx)
			subIdx++
		}
	}
	// Judge exactly the audio track the client will actually hear - the same
	// stream selectedAudioSpec serves (explicit selection, else the default-flag
	// track, else the first audio stream). Judging per-stream in the loop missed
	// the codec check entirely for a file with no default-flagged audio track:
	// none matched, the decision stayed "copy", and ffmpeg then served the first
	// (possibly undecodable, e.g. DTS) track raw.
	if sel := selectedAudioSpec(specs, prefs); sel != nil {
		evaluateAudio(d, *sel, client, prefs)
	}

	checkBitrate(d, totalBitrate, effectiveMaxBitrate)
	checkContainer(d, containerFormat, client)
	applyCodecPreferences(d, specs, prefs)
	applyAudioChannelTarget(d, specs, client, prefs)
	applyDirectPlayPrefs(d, prefs)
	resolvePlayMethod(d, client)
	applyDefaultBitrates(d)

	span.SetAttributes(
		attribute.String("decision.play_method", d.PlayMethod),
		attribute.String("decision.video", d.VideoDecision),
		attribute.String("decision.audio", d.AudioDecision),
	)

	// SHADOW MODE ONLY: observes mkvgo's independent ops.Playability verdict
	// against d, off this call's path (nil-safe, and Observe itself spawns a
	// detached goroutine) - never mutates d. See playability_shadow.go.
	if len(sourcePath) > 0 {
		playabilityShadow.Observe(ctx, sourcePath[0], d, client)
	}
	return d
}

func resolveMaxBitrate(clientMax int64, userCapKbps int) int64 {
	if userCapKbps > 0 && (clientMax == 0 || int64(userCapKbps) < clientMax) {
		return int64(userCapKbps)
	}
	return clientMax
}

func sumBitrates(streams []StreamSpec) int64 {
	var total int64
	for _, s := range streams {
		if s.Bitrate != nil {
			total += *s.Bitrate
		}
	}
	return total
}

func evaluateVideo(d *transcoding.PlaybackDecision, s StreamSpec, client transcoding.ClientProfile) {
	if !videoCodecSupported(client.VideoCodecs, s.Codec) {
		d.TranscodeReasons = append(d.TranscodeReasons, "video_codec_unsupported")
		d.VideoDecision = "transcode"
		d.VideoCodec = transcoding.DefaultVideoCodec
	}

	if s.Width != nil && client.MaxWidth > 0 && *s.Width > client.MaxWidth {
		d.TranscodeReasons = append(d.TranscodeReasons, "resolution_exceeded")
		d.VideoDecision = "transcode"
		d.MaxWidth = client.MaxWidth
	}
	if s.Height != nil && client.MaxHeight > 0 && *s.Height > client.MaxHeight {
		addReasonOnce(d, "resolution_exceeded")
		d.VideoDecision = "transcode"
		d.MaxHeight = client.MaxHeight
	}

	if s.HDRType != nil && *s.HDRType != "" {
		// Direct-play HDR only when the client signalled HDR support, and never
		// for Dolby Vision: a single SupportsHDR bool can't tell a DoVi-capable
		// panel from an HDR10-only one, and direct-playing a DoVi stream to an
		// HDR10-only display renders wrong. DoVi always tonemaps to SDR.
		hdrType := strings.ToLower(*s.HDRType)
		isDolbyVision := strings.Contains(hdrType, "dolby") || strings.Contains(hdrType, "dovi")
		if !client.SupportsHDR || isDolbyVision {
			d.TranscodeReasons = append(d.TranscodeReasons, "hdr_tonemapping_needed")
			d.VideoDecision = "transcode"
		}
	}

	if d.VideoDecision == "transcode" && d.VideoCodec == "" {
		d.VideoCodec = transcoding.DefaultVideoCodec
	}
}

// addReasonOnce appends a transcode reason unless it is already recorded, so a
// stream that trips the same condition twice (e.g. both width and height over
// the client cap) lists the reason a single time.
func addReasonOnce(d *transcoding.PlaybackDecision, reason string) {
	if !containsReason(d.TranscodeReasons, reason) {
		d.TranscodeReasons = append(d.TranscodeReasons, reason)
	}
}

// evaluateAudio decides copy-vs-transcode for the audio stream the client will
// actually hear. The caller passes the already-selected stream (selectedAudioSpec:
// the explicit prefs.AudioTrack, else the default-flag track, else the first
// audio stream), so this only judges its codec and channel count - judging any
// other track would be wrong in both directions (a copyable selected track
// re-encoded, an unsupported served track copied raw).
func evaluateAudio(d *transcoding.PlaybackDecision, s StreamSpec, client transcoding.ClientProfile, prefs TranscodingPreferences) {
	// Served untouched for a client-side decode: neither the codec nor the
	// channel count is a reason to re-encode anything.
	if wasmAudioApplies(s, prefs) {
		d.AudioDecision = "wasm"
		return
	}
	if !audioCodecSupported(client.AudioCodecs, s.Codec) {
		d.TranscodeReasons = append(d.TranscodeReasons, "audio_codec_unsupported")
		d.AudioDecision = "transcode"
		d.AudioCodec = transcoding.DefaultAudioCodec
	}
	if s.Channels != nil && client.MaxAudioChannels > 0 && *s.Channels > client.MaxAudioChannels {
		d.TranscodeReasons = append(d.TranscodeReasons, "audio_channels_exceeded")
		d.AudioDecision = "transcode"
		if d.AudioCodec == "" {
			d.AudioCodec = transcoding.DefaultAudioCodec
		}
		d.MaxAudioChannels = client.MaxAudioChannels
	}
	// Per-profile cap on audio channels (user pref).
	if s.Channels != nil && prefs.MaxAudioChannels > 0 && *s.Channels > prefs.MaxAudioChannels {
		addReasonOnce(d, "audio_channels_exceeded")
		d.AudioDecision = "transcode"
		if d.AudioCodec == "" {
			d.AudioCodec = transcoding.DefaultAudioCodec
		}
		// Carry the cap to the builder's -ac like the client-cap branch does;
		// without it the pref only worked by accident of the stereo default.
		if d.MaxAudioChannels == 0 || prefs.MaxAudioChannels < d.MaxAudioChannels {
			d.MaxAudioChannels = prefs.MaxAudioChannels
		}
	}
}

// Mirrors WasmAudioApplies on the spec the decision already carries. The
// profile's max_audio_channels is not consulted because the master handler does
// not consult it either - honouring it here would describe a downmix the server
// will not perform.
func wasmAudioApplies(s StreamSpec, prefs TranscodingPreferences) bool {
	if !prefs.WasmAudio || !WasmAudioCodecSupported(s.Codec) {
		return false
	}
	channels := 0
	if s.Channels != nil {
		channels = *s.Channels
	}
	return WasmAudioChannelsOK(channels)
}

// aacMaxTargetChannels caps the transcoded AAC layout at 5.1. Multichannel
// AAC decode is universal across MSE browsers up to 6 channels (the browser
// downmixes to the output device); 7.1 AAC support is patchier, so 7.1
// sources fold to 5.1 rather than risk an undecodable rendition.
const aacMaxTargetChannels = 6

// applyAudioChannelTarget sets the transcode's channel target from the
// SELECTED source track instead of letting the builder fall back to its
// historical stereo default. Runs as a finishing pass because AudioDecision
// can flip to "transcode" in several places (codec unsupported, container,
// codec preferences). Rules:
//   - copy / mono / stereo sources: untouched (d.MaxAudioChannels stays as-is;
//     0 keeps the pre-feature hash AND the pre-feature -ac 2 bytes);
//   - multichannel source: target = min(source, client cap, profile pref cap,
//     aacMaxTargetChannels), never raising a cap already set by the
//     channels-exceeded path.
//
// A DTS/TrueHD 5.1 film therefore re-encodes to AAC 5.1 instead of being
// silently downmixed to stereo - the single biggest audible quality loss the
// transcode path had.
func applyAudioChannelTarget(d *transcoding.PlaybackDecision, specs []StreamSpec, client transcoding.ClientProfile, prefs TranscodingPreferences) {
	if d.AudioDecision != "transcode" {
		return
	}
	src := selectedAudioSpec(specs, prefs)
	if src == nil || src.Channels == nil || *src.Channels <= 2 {
		return
	}
	target := min(*src.Channels, aacMaxTargetChannels)
	if client.MaxAudioChannels > 0 {
		target = min(target, client.MaxAudioChannels)
	}
	if prefs.MaxAudioChannels > 0 {
		target = min(target, prefs.MaxAudioChannels)
	}
	if d.MaxAudioChannels > 0 {
		target = min(target, d.MaxAudioChannels)
	}
	if target > 2 {
		d.MaxAudioChannels = target
	}
}

// selectedAudioSpec returns the audio stream the client will actually hear:
// the prefs.AudioTrack audio-relative index when selected (honored even for
// commentary tracks), else the default-flag track skipping commentary when a
// non-commentary audio exists, else the first audio stream.
//
// Commentary skip rule: when the implicit fallback would select a
// commentary-flagged track (either the default or the sole track), but a
// non-commentary audio track exists, the first non-commentary track is
// returned instead. If every audio track is commentary the behaviour is
// unchanged (a commentary track is better than silence).
func selectedAudioSpec(specs []StreamSpec, prefs TranscodingPreferences) *StreamSpec {
	audIdx := 0
	var def, firstNonCommentary, first *StreamSpec
	for i := range specs {
		s := &specs[i]
		if s.Type != "audio" {
			continue
		}
		if prefs.AudioTrack >= 0 && audIdx == prefs.AudioTrack {
			return s
		}
		if s.IsDefault && def == nil {
			def = s
		}
		if !s.Commentary && firstNonCommentary == nil {
			firstNonCommentary = s
		}
		if first == nil {
			first = s
		}
		audIdx++
	}
	if prefs.AudioTrack >= 0 {
		return nil // explicit selection out of range: judge nothing extra
	}
	// If the default track exists and is NOT commentary, return it unchanged.
	if def != nil && !def.Commentary {
		return def
	}
	// If the default track IS commentary but a non-commentary alternative exists,
	// prefer the non-commentary track so director/cast commentary tracks are not
	// played by default even when the container flags them as the default stream.
	if firstNonCommentary != nil {
		return firstNonCommentary
	}
	// All audio tracks are commentary (or no default): fall back to the original
	// logic (default if flagged, else first) - commentary is better than silence.
	if def != nil {
		return def
	}
	return first
}

// evaluateSubtitle decides how the subtitle stream s (subtitle-relative index
// subIdx) is delivered. Burn-in is the only destructive path - it overlays onto
// decoded frames, so it forces a video transcode (forceBurnVideoTranscode) and
// denies the copy/remux rung. Burn is gated on the SELECTED track (not on
// mere existence), so it fires ONLY when this is
// the selected track (subIdx == prefs.SubtitleBurnInTrack) and it is a bitmap
// sub. An unselected bitmap track is a no-op - not burnt, and not deliverable as
// a sidecar - so a Blu-ray rip that merely CARRIES a PGS track still gets its
// copy rung when played with no subtitle or with a text subtitle. Text subs are
// always offered as "external" (sidecar) regardless of selection: non-destructive,
// the player picks.
func evaluateSubtitle(d *transcoding.PlaybackDecision, s StreamSpec, prefs TranscodingPreferences, subIdx int) {
	selected := subIdx == prefs.SubtitleBurnInTrack
	isImage := IsImageSubtitle(s.Codec)

	switch prefs.SubtitleBurnIn {
	case "never":
		// User forbids burn entirely. Offer as sidecar; the adapter decides how to
		// deliver a bitmap sub it can't render as text. Selection-independent.
		if d.SubtitleDecision == "" {
			d.SubtitleDecision = "external"
		}
		return
	case "always":
		// User forces burn - but the ffmpeg layer only burns IMAGE subs
		// (hls_transcoder's burnIn predicate requires IsImageSubtitle); a text sub
		// cannot be overlaid, so forcing burn would needlessly transcode the video
		// AND silently drop the subtitle. Burn only a selected image sub; a text
		// sub (selected or not) is delivered as a sidecar instead.
		if selected && isImage {
			d.SubtitleDecision = "burn"
			forceBurnVideoTranscode(d)
		} else if !isImage && d.SubtitleDecision == "" {
			d.SubtitleDecision = "external"
		}
		return
	}
	// auto / default
	if selected && isImage {
		d.SubtitleDecision = "burn"
		forceBurnVideoTranscode(d)
	} else if !isImage && d.SubtitleDecision == "" {
		// Text track → sidecar-deliverable (selected or not). An unselected bitmap
		// track falls through to a no-op: not burnt, not a sidecar candidate.
		d.SubtitleDecision = "external"
	}
}

// forceBurnVideoTranscode makes a burn-in subtitle decision imply a video
// transcode: burning overlays subtitles onto the decoded frames, which is
// impossible on a `-c:v copy` (remux) stream. Without this, a copyable H.264
// title with a burn-in subtitle would route to the copy path and the subtitles
// would be SILENTLY DROPPED. Guards "burn-in ⟹ never copy" at the source.
func forceBurnVideoTranscode(d *transcoding.PlaybackDecision) {
	if d.VideoDecision == "transcode" {
		return
	}
	d.VideoDecision = "transcode"
	d.TranscodeReasons = append(d.TranscodeReasons, "subtitle_burn_in")
	if d.VideoCodec == "" {
		d.VideoCodec = transcoding.DefaultVideoCodec
	}
}

func checkBitrate(d *transcoding.PlaybackDecision, totalBitrate, effectiveMaxBitrate int64) {
	totalBitrateKbps := totalBitrate / 1000
	if effectiveMaxBitrate > 0 && totalBitrateKbps > effectiveMaxBitrate {
		d.TranscodeReasons = append(d.TranscodeReasons, "bitrate_exceeded")
		d.VideoDecision = "transcode"
		d.VideoBitrate = effectiveMaxBitrate
		if d.VideoCodec == "" {
			d.VideoCodec = transcoding.DefaultVideoCodec
		}
	}
}

func checkContainer(d *transcoding.PlaybackDecision, containerFormat string, client transcoding.ClientProfile) {
	containerLower := strings.ToLower(containerFormat)
	if containerLower == "matroska" {
		containerLower = "mkv"
	}
	if !contains(client.Containers, containerLower) {
		d.TranscodeReasons = append(d.TranscodeReasons, "container_incompatible")
	}
}

// applyCodecPreferences overrides VideoCodec / AudioCodec with user-preferred codec
// when source != preferred AND the client supports the preferred codec. Forces transcode
// if the source is already copying but the user pref differs.
func applyCodecPreferences(d *transcoding.PlaybackDecision, streams []StreamSpec, prefs TranscodingPreferences) {
	if prefs.PreferredVideoCodec != "" && prefs.PreferredVideoCodec != "auto" {
		srcVideoCodec := findStreamCodec(streams, "video")
		wantVideoCodec := normalizeVideoCodec(prefs.PreferredVideoCodec)
		if normalizeVideoCodec(srcVideoCodec) != wantVideoCodec {
			d.VideoCodec = wantVideoCodec
			if d.VideoDecision != "transcode" {
				d.VideoDecision = "transcode"
				d.TranscodeReasons = append(d.TranscodeReasons, "preferred_video_codec")
			}
		}
	}
	if prefs.PreferredAudioCodec != "" && prefs.PreferredAudioCodec != "auto" {
		srcAudioCodec := findDefaultAudioCodec(streams)
		if srcAudioCodec != prefs.PreferredAudioCodec {
			d.AudioCodec = prefs.PreferredAudioCodec
			if d.AudioDecision != "transcode" {
				d.AudioDecision = "transcode"
				d.TranscodeReasons = append(d.TranscodeReasons, "preferred_audio_codec")
			}
		}
	}
}

// applyDirectPlayPrefs forces transcode when user disabled direct_play / direct_stream.
// allow_direct_play=false: if currently no transcode reason, we force video+audio transcode.
// allow_direct_stream=false: if we would remux (direct_stream), force transcode too.
func applyDirectPlayPrefs(d *transcoding.PlaybackDecision, prefs TranscodingPreferences) {
	if !prefs.AllowDirectPlay && len(d.TranscodeReasons) == 0 {
		d.VideoDecision = "transcode"
		d.AudioDecision = "transcode"
		d.TranscodeReasons = append(d.TranscodeReasons, "direct_play_disabled")
		ensureTranscodeCodecs(d)
	}
	if !prefs.AllowDirectStream && isDirectStreamEligible(d) {
		d.VideoDecision = "transcode"
		d.AudioDecision = "transcode"
		d.TranscodeReasons = append(d.TranscodeReasons, "direct_stream_disabled")
		ensureTranscodeCodecs(d)
	}
}

// isDirectStreamEligible reports that the only thing keeping this off direct
// play is an incompatible container while both streams still copy - i.e. a pure
// remux (direct_stream), no re-encoding.
//
// "wasm" is excluded on purpose: PlayMethod routes (direct_stream hands web the
// /transcode?remux=1 pipe) and the wasm path needs the HLS master. The UI names
// the method from the per-track decisions instead (effectivePlayMethod).
func isDirectStreamEligible(d *transcoding.PlaybackDecision) bool {
	return onlyContainerIssue(d.TranscodeReasons) &&
		d.VideoDecision == "copy" && d.AudioDecision == "copy"
}

// ensureTranscodeCodecs fills the fallback codecs whenever a stream flipped to
// transcode without a target codec having been chosen yet.
func ensureTranscodeCodecs(d *transcoding.PlaybackDecision) {
	if d.VideoCodec == "" {
		d.VideoCodec = transcoding.DefaultVideoCodec
	}
	if d.AudioCodec == "" {
		d.AudioCodec = transcoding.DefaultAudioCodec
	}
}

func resolvePlayMethod(d *transcoding.PlaybackDecision, client transcoding.ClientProfile) {
	switch {
	case len(d.TranscodeReasons) == 0:
		d.PlayMethod = "direct_play"
	case isDirectStreamEligible(d):
		d.PlayMethod = "direct_stream"
		d.Container = preferredContainer(client.Containers)
	default:
		d.PlayMethod = "transcode"
		if d.Container == "" {
			d.Container = preferredContainer(client.Containers)
		}
	}
	// Safety net: a user who disallowed transcoding but ends up needing it keeps
	// PlayMethod="transcode" here; handlers gate on TranscodeDisallowed(decision,
	// prefs) before starting HLS rather than this function reporting it.
}

// TranscodeDisallowed returns true when the decision requires transcoding but
// the user pref forbids it.
func TranscodeDisallowed(d *transcoding.PlaybackDecision, prefs TranscodingPreferences) bool {
	return !prefs.AllowTranscoding && d.PlayMethod == "transcode"
}

// Fallback bitrates (kbps) when a transcode decision carries no explicit
// target: a mid-quality 1080p video rung and transparent stereo audio.
const (
	defaultVideoBitrateKbps = 4000
	defaultAudioBitrateKbps = 192
)

func applyDefaultBitrates(d *transcoding.PlaybackDecision) {
	if d.VideoDecision == "transcode" && d.VideoBitrate == 0 {
		d.VideoBitrate = defaultVideoBitrateKbps
	}
	if d.AudioDecision == "transcode" && d.AudioBitrate == 0 {
		d.AudioBitrate = defaultAudioBitrateKbps
	}
}

func findStreamCodec(streams []StreamSpec, streamType string) string {
	for _, s := range streams {
		if s.Type == streamType {
			return strings.ToLower(s.Codec)
		}
	}
	return ""
}

func findDefaultAudioCodec(streams []StreamSpec) string {
	for _, s := range streams {
		if s.Type == "audio" && s.IsDefault {
			return strings.ToLower(s.Codec)
		}
	}
	return ""
}

func contains(list []string, val string) bool {
	for _, v := range list {
		if strings.EqualFold(v, val) {
			return true
		}
	}
	return false
}

// normalizeVideoCodec folds the common aliases of a video codec to one canonical
// name (the ffprobe spelling the prober stores) via the single canonical table
// in transcoding.CanonicalVideoCodec. A client that advertises "h265" or "hvc1"
// means HEVC; without this fold it would not match a source probed as "hevc"
// and the stream would be needlessly transcoded. Unrecognised codecs fall back
// to lowercase passthrough so two-sided comparisons still degrade to a plain
// string match instead of colliding on "".
func normalizeVideoCodec(c string) string {
	if canon := transcoding.CanonicalVideoCodec(c); canon != "" {
		return canon
	}
	return strings.ToLower(c)
}

// videoCodecSupported reports whether the client can play srcCodec, comparing on
// the canonical codec name so codec aliases (h265==hevc) match.
func videoCodecSupported(clientCodecs []string, srcCodec string) bool {
	want := normalizeVideoCodec(srcCodec)
	for _, c := range clientCodecs {
		if normalizeVideoCodec(c) == want {
			return true
		}
	}
	return false
}

// normalizeAudioCodec folds the common spellings of an audio codec to one
// canonical name (the ffprobe spelling the prober stores). A client that
// advertises the hyphenated RFC/MIME forms ("ac-3", "e-ac-3", "ec-3") or "mp4a"
// means the same codec the prober probed as "ac3"/"eac3"/"aac"; without this fold
// it would not match and the audio would be needlessly transcoded.
func normalizeAudioCodec(c string) string {
	switch strings.ToLower(c) {
	case "ac3", "ac-3":
		return "ac3"
	case "eac3", "e-ac-3", "ec-3", "ec3":
		return "eac3"
	case "aac", "mp4a", "aac_latm":
		return "aac"
	case "mp3", "mp3float":
		return "mp3"
	case "dts", "dca":
		return "dts"
	case "truehd", "mlp":
		return "truehd"
	}
	return strings.ToLower(c)
}

// audioCodecSupported reports whether the client can play srcCodec, comparing on
// the canonical codec name so codec aliases (ac-3==ac3, e-ac-3==eac3) match.
func audioCodecSupported(clientCodecs []string, srcCodec string) bool {
	want := normalizeAudioCodec(srcCodec)
	for _, c := range clientCodecs {
		if normalizeAudioCodec(c) == want {
			return true
		}
	}
	return false
}

func containsReason(reasons []string, reason string) bool {
	return slices.Contains(reasons, reason)
}

func onlyContainerIssue(reasons []string) bool {
	for _, r := range reasons {
		if r != "container_incompatible" {
			return false
		}
	}
	return len(reasons) > 0
}

func preferredContainer(containers []string) string {
	if len(containers) > 0 {
		return containers[0]
	}
	return "mp4"
}
