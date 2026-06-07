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
		// hdr.DetectHDR the tonemap path uses — one source of truth. Without this
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
			Type:      string(s.StreamType),
			Codec:     s.Codec,
			Width:     s.Width,
			Height:    s.Height,
			HDRType:   hdrType,
			Channels:  s.Channels,
			Bitrate:   s.Bitrate,
			IsDefault: s.IsDefault,
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
	// burns — and so forces a video transcode — ONLY for this track when it is a
	// bitmap sub. Without it the decision would burn whenever ANY bitmap track
	// merely EXISTS, needlessly transcoding (and denying the copy/remux rung)
	// every Blu-ray rip even when the user plays with no subtitle. This is a
	// request-scoped field injected by the transport layer (like AudioLanguages /
	// Locale), not a profile pref — ParseTranscodingPreferences leaves it at the
	// -1 default.
	SubtitleBurnInTrack    int
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
) *transcoding.PlaybackDecision {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.MakePlaybackDecision")
	defer span.End()
	span.SetAttributes(attribute.String("container.format", containerFormat))
	return MakePlaybackDecisionWithPrefs(ctx, streams, containerFormat, client, maxBitrateKbps, DefaultTranscodingPreferences())
}

// MakePlaybackDecisionWithPrefs is the full entry point that also honors user prefs.
// MakePlaybackDecision delegates here with default (permissive) prefs for backward compat.
//
// The public signature still accepts []*library.MediaStream so the cascade of
// call sites (playback_handler.go and friends) stays untouched. Internally we
// convert once to []StreamSpec via toStreamSpecs and every evaluator below
// works on that lean VO — keeping the algorithm decoupled from the library
// entity it used to depend on field-by-field.
func MakePlaybackDecisionWithPrefs(
	ctx context.Context,
	streams []*library.MediaStream,
	containerFormat string,
	client transcoding.ClientProfile,
	maxBitrateKbps int,
	prefs TranscodingPreferences,
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
		case "audio":
			evaluateAudio(d, s, client, prefs)
		case "subtitle":
			evaluateSubtitle(d, s, prefs, subIdx)
			subIdx++
		}
	}

	checkBitrate(d, totalBitrate, effectiveMaxBitrate)
	checkContainer(d, containerFormat, client)
	applyCodecPreferences(d, specs, prefs)
	applyDirectPlayPrefs(d, prefs)
	resolvePlayMethod(d, client, prefs)
	applyDefaultBitrates(d)

	span.SetAttributes(
		attribute.String("decision.play_method", d.PlayMethod),
		attribute.String("decision.video", d.VideoDecision),
		attribute.String("decision.audio", d.AudioDecision),
	)
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
	if !contains(client.VideoCodecs, strings.ToLower(s.Codec)) {
		d.TranscodeReasons = append(d.TranscodeReasons, "video_codec_unsupported")
		d.VideoDecision = "transcode"
		d.VideoCodec = "h264"
	}

	if s.Width != nil && client.MaxWidth > 0 && *s.Width > client.MaxWidth {
		d.TranscodeReasons = append(d.TranscodeReasons, "resolution_exceeded")
		d.VideoDecision = "transcode"
		d.MaxWidth = client.MaxWidth
	}
	if s.Height != nil && client.MaxHeight > 0 && *s.Height > client.MaxHeight {
		if !containsReason(d.TranscodeReasons, "resolution_exceeded") {
			d.TranscodeReasons = append(d.TranscodeReasons, "resolution_exceeded")
		}
		d.VideoDecision = "transcode"
		d.MaxHeight = client.MaxHeight
	}

	if s.HDRType != nil && *s.HDRType != "" {
		// Direct-play HDR only when the client signalled HDR support, and never
		// for Dolby Vision: a single SupportsHDR bool can't tell a DoVi-capable
		// panel from an HDR10-only one, and direct-playing a DoVi stream to an
		// HDR10-only display renders wrong. DoVi always tonemaps to SDR.
		hdr := strings.ToLower(*s.HDRType)
		isDolbyVision := strings.Contains(hdr, "dolby") || strings.Contains(hdr, "dovi")
		if !client.SupportsHDR || isDolbyVision {
			d.TranscodeReasons = append(d.TranscodeReasons, "hdr_tonemapping_needed")
			d.VideoDecision = "transcode"
		}
	}

	if d.VideoDecision == "transcode" && d.VideoCodec == "" {
		d.VideoCodec = "h264"
	}
}

func evaluateAudio(d *transcoding.PlaybackDecision, s StreamSpec, client transcoding.ClientProfile, prefs TranscodingPreferences) {
	if !s.IsDefault {
		return
	}
	if !contains(client.AudioCodecs, strings.ToLower(s.Codec)) {
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
		if !containsReason(d.TranscodeReasons, "audio_channels_exceeded") {
			d.TranscodeReasons = append(d.TranscodeReasons, "audio_channels_exceeded")
		}
		d.AudioDecision = "transcode"
		if d.AudioCodec == "" {
			d.AudioCodec = transcoding.DefaultAudioCodec
		}
	}
}

// evaluateSubtitle decides how the subtitle stream s (subtitle-relative index
// subIdx) is delivered. Burn-in is the only destructive path — it overlays onto
// decoded frames, so it forces a video transcode (forceBurnVideoTranscode) and
// denies the copy/remux rung. To match Jellyfin (which gates burn on the
// SELECTED SubtitleStreamIndex, not on existence), burn fires ONLY when this is
// the selected track (subIdx == prefs.SubtitleBurnInTrack) and it is a bitmap
// sub. An unselected bitmap track is a no-op — not burnt, and not deliverable as
// a sidecar — so a Blu-ray rip that merely CARRIES a PGS track still gets its
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
		// User forces burn — but there must be a track to burn: only the selected
		// one. With none selected, a text sub is still offered as a sidecar.
		if selected {
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
		d.VideoCodec = "h264"
	}
}

func checkBitrate(d *transcoding.PlaybackDecision, totalBitrate, effectiveMaxBitrate int64) {
	totalBitrateKbps := totalBitrate / 1000
	if effectiveMaxBitrate > 0 && totalBitrateKbps > effectiveMaxBitrate {
		d.TranscodeReasons = append(d.TranscodeReasons, "bitrate_exceeded")
		d.VideoDecision = "transcode"
		d.VideoBitrate = effectiveMaxBitrate
		if d.VideoCodec == "" {
			d.VideoCodec = "h264"
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
		if srcVideoCodec != prefs.PreferredVideoCodec {
			d.VideoCodec = prefs.PreferredVideoCodec
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
		if d.VideoCodec == "" {
			d.VideoCodec = "h264"
		}
		if d.AudioCodec == "" {
			d.AudioCodec = transcoding.DefaultAudioCodec
		}
	}
	if !prefs.AllowDirectStream && onlyContainerIssue(d.TranscodeReasons) && d.VideoDecision == "copy" && d.AudioDecision == "copy" {
		d.VideoDecision = "transcode"
		d.AudioDecision = "transcode"
		d.TranscodeReasons = append(d.TranscodeReasons, "direct_stream_disabled")
		if d.VideoCodec == "" {
			d.VideoCodec = "h264"
		}
		if d.AudioCodec == "" {
			d.AudioCodec = transcoding.DefaultAudioCodec
		}
	}
}

func resolvePlayMethod(d *transcoding.PlaybackDecision, client transcoding.ClientProfile, prefs TranscodingPreferences) {
	switch {
	case len(d.TranscodeReasons) == 0:
		d.PlayMethod = "direct_play"
	case onlyContainerIssue(d.TranscodeReasons) && d.VideoDecision == "copy" && d.AudioDecision == "copy":
		d.PlayMethod = "direct_stream"
		d.Container = preferredContainer(client.Containers)
	default:
		d.PlayMethod = "transcode"
		if d.Container == "" {
			d.Container = preferredContainer(client.Containers)
		}
	}
	// Safety net: if user disallowed transcoding and we ended up needing it,
	// surface as "transcode_disallowed" so callers can return a clear error.
	// We keep PlayMethod="transcode" for consistency with existing flow; handlers
	// should check TranscodeDisallowed(decision, prefs) before starting HLS.
	_ = prefs
}

// TranscodeDisallowed returns true when the decision requires transcoding but
// the user pref forbids it.
func TranscodeDisallowed(d *transcoding.PlaybackDecision, prefs TranscodingPreferences) bool {
	return !prefs.AllowTranscoding && d.PlayMethod == "transcode"
}

func applyDefaultBitrates(d *transcoding.PlaybackDecision) {
	if d.VideoDecision == "transcode" && d.VideoBitrate == 0 {
		d.VideoBitrate = 4000
	}
	if d.AudioDecision == "transcode" && d.AudioBitrate == 0 {
		d.AudioBitrate = 192
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
