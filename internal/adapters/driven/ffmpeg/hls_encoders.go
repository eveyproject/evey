package ffmpeg

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// hls_encoders.go groups the codec/encoder resolution helpers shared by the
// software and hardware argument builders.

// resolveVideoEncoder maps a domain codec name ("h264"/"hevc"/"h265"/"av1") to
// the ffmpeg encoder name. Empty → libx264 (previous hardcoded default).
func resolveVideoEncoder(codec string) string {
	switch strings.ToLower(codec) {
	case "", "h264", "libx264":
		return "libx264"
	case "hevc", "h265", "libx265":
		return "libx265"
	case "av1", "libaom-av1", "libsvtav1":
		// SVT-AV1, not libaom-av1: libaom is absent from the shipped
		// jellyfin-ffmpeg build (only --enable-libsvtav1) AND is far too slow
		// for streaming. SVT takes a numeric -preset (see svtAV1PresetArgs); it
		// rejects the named x264/x265 presets outright.
		return "libsvtav1"
	}
	return "libx264"
}

// svtAV1PresetArgs returns the speed/quality preset for a software video
// encoder. libx264/libx265 use the named ladder ("veryfast"); libsvtav1 takes a
// numeric 0-13 preset (higher = faster) and rejects the named values, failing
// to open. AV1 software encoding is heavy, so on the weak-NAS target we lean
// fast - 10 is a balanced default and the speed/quality knob if it needs tuning.
func svtAV1PresetArgs(encoder string) []string {
	if encoder == "libsvtav1" {
		return []string{"-preset", svtAV1Preset}
	}
	return []string{"-preset", "veryfast"}
}

// softwareProfileLevelArgs returns the -profile:v / -level:v args for a software
// encoder. "high"/"4.1" are H.264 (libx264) values: libx265 rejects them outright
// ("x265 [error]: unknown profile <high>" → the encoder fails to open and the
// whole session dies) and libaom-av1 uses different profile/level strings, so
// non-libx264 encoders get nothing and derive the profile from the pixel format.
// variantIdx < 0 emits the single-output form (-profile:v); else the per-rung
// -profile:v:N form used by the multi-variant builder. Reachable via a client
// PreferredVideoCodec=hevc on a software path (no HW HEVC) - found by audit.
// maxLevel (client decoder level_idc, 0 = none) lowers the signalled level
// when the rung's natural one exceeds it.
const h264Profile = "high"

func softwareProfileLevelArgs(encoder string, variantIdx int, height int, maxLevel int) []string {
	if encoder != "libx264" {
		return nil
	}
	idc := transcoding.H264LevelIdcFor(height)
	if maxLevel > 0 && maxLevel < idc {
		idc = maxLevel
	}
	level := transcoding.H264LevelString(idc)
	if variantIdx < 0 {
		return []string{"-profile:v", h264Profile, "-level:v", level}
	}
	return []string{
		fmt.Sprintf("-profile:v:%d", variantIdx), h264Profile,
		fmt.Sprintf("-level:v:%d", variantIdx), level,
	}
}

// x264SpeedTuning is motion-search tuning on top of the veryfast preset, which
// still pays for subpixel refinement a watch-once transcode never gets back.
// Measured +24% (6.18x -> 7.65x) on an encode-bound 1080p H.264 source at equal
// resolution and CRF; +3% on decode-bound 4K HEVC, where the decoder is the wall.
//
// Tradeoff: subme=0 costs some quality per bit. We encode at CRF 20 where the
// reference for these options (Jellyfin) uses 23, so rate control has more bits
// to spend. One constant, so the tradeoff has one place to be revisited.
const x264SpeedTuning = "subme=0:me_range=16:rc_lookahead=10:me=hex:open_gop=0"

// softwareX264TuningArgs returns -x264opts for a libx264 encode. libx264 only:
// libx265/libsvtav1 take different parameter strings and fail to open on this.
// variantIdx < 0 emits the single-output form, else the per-rung one.
func softwareX264TuningArgs(encoder string, variantIdx int) []string {
	if encoder != "libx264" {
		return nil
	}
	if variantIdx < 0 {
		return []string{"-x264opts", x264SpeedTuning}
	}
	return []string{fmt.Sprintf("-x264opts:%d", variantIdx), x264SpeedTuning}
}

// normalizeVideoCodec folds the codec through the single canonical alias
// table (transcoding.CanonicalVideoCodec), then restricts the result to the
// codecs this package can build a hardware encoder name for ("h264"/"hevc"/
// "av1"). Empty codec defaults to h264 (matches resolveVideoEncoder's
// libx264 default). Returns "" for anything unrecognised or not
// HW-encodable (e.g. vp9) so callers fall back to software.
func normalizeVideoCodec(codec string) string {
	if codec == "" {
		return "h264"
	}
	switch canon := transcoding.CanonicalVideoCodec(codec); canon {
	case "h264", "hevc", "av1":
		return canon
	}
	return ""
}

// videoEncoderFor maps a domain codec + hardware backend to the ffmpeg
// encoder name. accel None/empty/invalid → software (resolveVideoEncoder).
// When a backend is requested but the codec has no hardware encoder for it
// (unrecognised codec), it falls back to software so the command stays valid
// - defensive only: the wiring layer already gates HWAccel on per-codec
// capability probing (HWDetector.SupportsCodec), so a set HWAccel is trusted.
func videoEncoderFor(codec string, accel transcoding.HWAccelType) string {
	if accel == "" || accel == transcoding.None || !accel.IsValid() {
		return resolveVideoEncoder(codec)
	}
	base := normalizeVideoCodec(codec)
	if base == "" {
		return resolveVideoEncoder(codec)
	}
	switch accel {
	case transcoding.NVENC:
		return base + "_nvenc"
	case transcoding.VAAPI:
		return base + "_vaapi"
	case transcoding.QSV:
		return base + "_qsv"
	case transcoding.RKMPP:
		if base == "av1" {
			return resolveVideoEncoder(codec) // MPP has no AV1 encoder
		}
		return base + "_rkmpp"
	}
	return resolveVideoEncoder(codec)
}

// NativeAACEncoder is ffmpeg's built-in AAC encoder: always present, and the
// floor this server falls back to.
const NativeAACEncoder = "aac"

// PreferredAACEncoder is Fraunhofer's, when this ffmpeg carries it. Measurably
// better in the top octaves at the SAME bitrate, which is where a 5.1 fold's
// coding noise lands and where a listener hears it as grit. Measured on prod
// bytes (Arrival, AC-3 5.1 448k folded to stereo, 2026-09-07), noise below
// signal at 192 kbps CBR:
//
//	band     native aac   libfdk_aac
//	1-4 kHz    24.3 dB      27.6 dB
//	4-8 kHz    17.8 dB      20.2 dB
//	8-16 kHz   14.2 dB      16.0 dB
//
// Same bitrate, same fold, +2 to +3 dB across the band the complaint was about.
// It is also what Jellyfin uses on this very box, which is the A/B the user
// keeps making.
const PreferredAACEncoder = "libfdk_aac"

// AACEncoder reports which AAC encoder THIS ffmpeg should use. Probed once (an
// `ffmpeg -encoders` fork) and memoised: the answer is a property of the
// binary, so re-asking per session would be pure cost.
//
// The result MUST reach the cache fingerprint, not just the command line: two
// encoders at the same bitrate produce different bytes, and a cache that cannot
// tell them apart would serve a session half in one and half in the other after
// an image swap. That is why callers thread it through
// HLSProcessOpts.AudioEncoder rather than letting this function be consulted
// independently at each site.
func AACEncoder() string {
	aacEncoderOnce.Do(func() {
		// Bounded on purpose: this runs on the first playback of the process and
		// an ffmpeg that will not answer must not hold a viewer. A timeout keeps
		// the native encoder, which is always correct, just less clean.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if checkEncoderAvailable(ctx, "ffmpeg", PreferredAACEncoder) {
			aacEncoder = PreferredAACEncoder
		}
		slog.Info("audio: AAC encoder selected", slog.String("encoder", aacEncoder))
	})
	return aacEncoder
}

var (
	aacEncoderOnce sync.Once
	aacEncoder     = NativeAACEncoder
)

// resolveAudioEncoder maps a domain audio codec to the ffmpeg encoder name.
// Empty → aac (previous hardcoded default). override, when set, names the
// concrete AAC implementation the SESSION was fingerprinted with - see
// HLSProcessOpts.AudioEncoder. It only ever applies to the AAC family; asking
// for a libfdk build of ac3 or opus is meaningless and is ignored.
func resolveAudioEncoder(codec string, override ...string) string {
	switch strings.ToLower(codec) {
	case "", "aac":
		if len(override) > 0 && override[0] != "" {
			return override[0]
		}
		return NativeAACEncoder
	case "copy":
		// Passthrough: the source audio is muxed into fMP4 verbatim, no re-encode.
		// Used by the browser wasm-audio path to serve AC-3/E-AC-3 the client
		// decodes itself (ac3go wasm) - zero audio CPU on the server.
		return "copy"
	case "ac3":
		return "ac3"
	case "eac3":
		return "eac3"
	case "opus", "libopus":
		return "libopus"
	}
	return NativeAACEncoder
}

// scaledAudioBitrateKbps scales a rung's stereo-quoted audio bitrate to the
// channel target. The rule lives in the domain so the stream-info panel quotes
// the same number we hand the encoder.
func scaledAudioBitrateKbps(base int64, maxChannels int) int64 {
	return transcoding.ServedAudioBitrateKbps(base, maxChannels)
}

// resolveAudioChannels returns the -ac value. The 0 -> 2 rule lives in the
// domain so the downmix policy (which must know whether this encode folds)
// cannot disagree with what the encoder is actually told to produce.
func resolveAudioChannels(maxChannels int) string {
	return fmt.Sprintf("%d", transcoding.ResolvedAudioChannels(maxChannels))
}
