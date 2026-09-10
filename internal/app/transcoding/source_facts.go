package transcoding

// source_facts.go - pure helpers that read facts off the source MediaStream
// slice (codec, HDR, SAR, ten-bit...) and off the served-audio decision.
// Moved from the http adapter (transcode_vod_source_helpers.go) as part of
// the PlaybackPlanResolver migration: the plan resolution consumes them here,
// and the adapter keeps thin aliases for its remaining call sites.

import (
	"strings"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/hdr"
)

// SourceCopyVideoRange maps the source's HDR type to the HLS VIDEO-RANGE
// attribute the copy rung must advertise ("PQ"/"HLG"), or "" for SDR. Uses
// the same hdr.DetectHDR as the decision, so the attribute can't contradict
// the copy-vs-tonemap routing. Dolby Vision never reaches here as raw PQ
// unless its base layer is compatible (DetectHDR reports the base type).
func SourceCopyVideoRange(streams []*library.MediaStream) string {
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
		return "" // dolby_vision etc. - never served as copy, no attribute
	}
	return ""
}

// SourceCopyVideoCodec returns the RFC-6381 codec string (avc1.PPCCLL /
// hvc1.P.C.Lxxx.B0) to advertise for a copy rung of the source video stream,
// or "" when the source is not copy-eligible (the eligibility gates - level,
// HDR ground truth, DoVi policy - live in CopyVideoCodecString). The manifest
// only uses it for a rung whose HLSProfile.Copy is true.
func SourceCopyVideoCodec(streams []*library.MediaStream) string {
	for _, s := range streams {
		if s == nil || s.StreamType != "video" {
			continue
		}
		// No copy rung without the keyframe index: the copy plan (segment cuts +
		// StartSec) is built from s.Keyframes, so an item that has not been
		// keyframe-backfilled yet must fall back to transcode rather than route to
		// copy and break on an empty plan. Critical now that the rung is on by
		// default - the worker backfills keyframes_ms only after level.
		if len(s.Keyframes) == 0 {
			return ""
		}
		return CopyVideoCodecString(CopySource{
			Codec:         s.Codec,
			Profile:       s.Profile,
			Level:         s.Level,
			TenBit:        StreamIsTenBit(s),
			ColorTransfer: s.ColorTransfer,
			DoViProfile:   s.DoViProfile,
			DoViBLCompat:  s.DoViBLCompat,
			FrameRateMode: s.FrameRateMode,
		})
	}
	return ""
}

// VideoCopyEligible reports whether the SOURCE itself can ride a copy rung:
// the decision says copy and the video codec is one we can carry verbatim.
// This is the only part the mkvgo packager shares with the ffmpeg copy rung -
// everything else (a wired copy producer, the disk guard) is about ffmpeg
// WRITING segments, which the packager never does.
func VideoCopyEligible(decision transcoding.PlaybackDecision, streams []*library.MediaStream) bool {
	return decision.VideoDecision == "copy" && SourceCopyVideoCodec(streams) != ""
}

// audioCopyCodecs are the source codecs the muxed pipeline may pass through
// (`-c:a copy`) for EVERY client. The DECISION already guarantees the client
// decodes the selected track (evaluateAudio checks client.AudioCodecs, which
// the web player fills from canPlayType - ac-3/ec-3 only appear on Safari/
// TV-class clients); this set additionally bounds passthrough to codecs whose
// fMP4 muxing and RFC 6381 CODECS attribute (mp4a.40.2 / ac-3 / ec-3) are
// wired everywhere.
var audioCopyCodecs = map[string]bool{"aac": true, "ac3": true, "eac3": true}

// fmp4PassthroughOptIn bounds what a client may ADD to the set above by
// declaring FMP4AudioPassthrough. mp3 muxes into fMP4 as an `mp4a` sample entry
// and advertises as mp4a.40.34 - verified on prod bytes (2026-09-06): the
// packaging evey uses (`-c:a copy -f hls -hls_segment_type fmp4`) produces an
// init + segments that read back as `mp3, mp4a, 48000, 2`. media3/ExoPlayer
// plays that; AVPlayer does not take MP3 in fMP4 at all, and the web declares
// mp3 unconditionally, so this stays OPT-IN rather than a blanket widening.
//
// Why it matters: re-encoding an already-lossy source cascades. Measured on a
// 128 kbps MP3 DVD rip (2026-09-06), coding noise under the signal at the
// 192k AAC rendition: 30.6 dB on dialogue, 20.3 dB on dense music - BELOW the
// 26.9 dB the 128k rendition scored on a clean source in September, which is
// what "the background is not on the same plane as the voices" sounds like.
// A passthrough adds exactly none of it.
var fmp4PassthroughOptIn = map[string]bool{"mp3": true}

// AudioCopyCodecAllowed reports whether a (lowercased) source audio codec is in
// the always-wired passthrough set above. Used by the mkvgo CMAF packager gate,
// which carries tracks verbatim into a container this opt-in says nothing
// about - do NOT widen that path with the client's declaration.
func AudioCopyCodecAllowed(codec string) bool {
	return audioCopyCodecs[codec]
}

// AudioCopyCodecAllowedFor is AudioCopyCodecAllowed plus whatever THIS client
// declared it accepts muxed in fMP4. A codec outside fmp4PassthroughOptIn can
// never be unlocked by a client, so a stale or hostile declaration can only
// ever select something the muxer and the manifest already know how to emit.
func AudioCopyCodecAllowedFor(codec string, client transcoding.ClientProfile) bool {
	codec = normalizeAudioCodec(codec)
	if audioCopyCodecs[codec] {
		return true
	}
	if !fmp4PassthroughOptIn[codec] {
		return false
	}
	for _, c := range client.FMP4AudioPassthrough {
		if normalizeAudioCodec(c) == codec {
			return true
		}
	}
	return false
}

// AudioCopyEligible reports whether this request's audio is served as a
// passthrough. Gates on top of the decision's copy verdict:
//   - loudnorm requires a re-encode;
//   - single-rung ladders only (the multi-variant builder always re-encodes,
//     and one hash must yield one byte stream);
//   - the source codec must be carriable for THIS client (see
//     AudioCopyCodecAllowedFor).
//
// MUST stay the single source of truth for both the fingerprint (bytes) and
// the manifest (CODECS attribute): if they disagree, the master advertises a
// codec the segments don't carry and Safari refuses to play.
func AudioCopyEligible(decision transcoding.PlaybackDecision, normalize bool, profileCount int, srcCodec string, client transcoding.ClientProfile) bool {
	return decision.AudioDecision == "copy" &&
		!normalize &&
		profileCount == 1 &&
		AudioCopyCodecAllowedFor(srcCodec, client)
}

// ServedAudioCodec is the audio codec the segments will actually carry:
// the SOURCE codec on passthrough, the decision's encoder otherwise
// ("" → AAC, the historical default).
func ServedAudioCodec(copyEligible bool, srcCodec string, decision transcoding.PlaybackDecision) string {
	if copyEligible {
		return srcCodec
	}
	return decision.AudioCodec
}

// SourceAudioCodec returns the codec of the audio-relative track audioIdx
// (the Nth audio stream, ffmpeg's 0:a:N), lowercased, or "" when absent.
func SourceAudioCodec(streams []*library.MediaStream, audioIdx int) string {
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

// SourceSubtitleCodec returns the codec of the burn-in subtitle track (the Nth
// subtitle stream, ffmpeg's 0:s:N), lowercased, or "" when absent. The VOD
// pipeline re-derives burn-in eligibility inside the ffmpeg builder from this
// codec (IsImageSubtitle), so it MUST be threaded or a bitmap subtitle the
// decision forced a transcode to burn is silently dropped.
func SourceSubtitleCodec(streams []*library.MediaStream, subIdx int) string {
	if subIdx < 0 {
		return ""
	}
	n := 0
	for _, s := range streams {
		if s == nil || s.StreamType != "subtitle" {
			continue
		}
		if n == subIdx {
			return strings.ToLower(s.Codec)
		}
		n++
	}
	return ""
}

// SourceDeinterlaceAll reports whether the first video stream's idet content
// verdict says interlaced (the backfill's idet_interlaced = true). Drives the
// builder's deint=all fallback for sources whose container flags lie. nil
// verdict (not yet analyzed, or modern codec) ⇒ false ⇒ the unchanged
// flag-gated deinterlace.
func SourceDeinterlaceAll(streams []*library.MediaStream) bool {
	for _, s := range streams {
		if s == nil || s.StreamType != "video" {
			continue
		}
		return s.IdetInterlaced != nil && *s.IdetInterlaced
	}
	return false
}

// SourceSAR returns the first video stream's sample aspect ratio ("64:45" for
// anamorphic DVD), or "" for square pixels / unprobed. Drives the builder's
// anamorphic scale correction.
func SourceSAR(streams []*library.MediaStream) string {
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

// SourceIsHDR reports whether the first video stream is HDR, via the same
// hdr.DetectHDR the decision and the tonemap filter use.
func SourceIsHDR(streams []*library.MediaStream) bool {
	for _, s := range streams {
		if s == nil || s.StreamType != "video" {
			continue
		}
		return hdr.DetectHDR([]hdr.VideoStream{s}).IsHDR
	}
	return false
}

// SourceVideoCodec returns the first video stream's codec (empty if none), the
// codec the pipeline must DECODE. Used to gate HW on decode capability.
func SourceVideoCodec(streams []*library.MediaStream) string {
	for _, s := range streams {
		if s != nil && s.StreamType == library.StreamTypeVideo {
			return s.Codec
		}
	}
	return ""
}

// SourceVideoHeight returns the first video stream's height (0 when unprobed),
// the other half of what the pipeline must DECODE. Admission prices the pair.
func SourceVideoHeight(streams []*library.MediaStream) int {
	for _, s := range streams {
		if s != nil && s.StreamType == library.StreamTypeVideo {
			if s.Height != nil && *s.Height > 0 {
				return *s.Height
			}
			return 0
		}
	}
	return 0
}

// SourceVideoTenBit reports whether the source's video stream is ≥10-bit
// (HEVC Main10 / yuv420p10le). Used to gate the HW path off when the calibrated
// backend's VPP can't fold 10-bit down to 8-bit (see EncodeCapability.HW10BitTranscode).
func SourceVideoTenBit(streams []*library.MediaStream) bool {
	for _, s := range streams {
		if s != nil && s.StreamType == library.StreamTypeVideo {
			return StreamIsTenBit(s)
		}
	}
	return false
}

// StreamIsTenBit detects ≥10-bit video from the probed bit depth or, since
// ffprobe leaves bits_per_raw_sample empty for many HEVC rips, the codec
// profile name ("Main 10", "High 10").
func StreamIsTenBit(s *library.MediaStream) bool {
	if s == nil {
		return false
	}
	if s.BitDepth != nil && *s.BitDepth >= 10 {
		return true
	}
	// Lower-cased to match hdr.DetectHDR's profile predicate (e.g. "Main 10",
	// "High 10") - the two ten-bit rules must agree across layers.
	return strings.Contains(strings.ToLower(s.Profile), "10")
}

// SourceDimsOf extracts the first video stream's resolution + 10-bit flag.
// Streams may be missing (degenerate file) - zero Width/Height is handled by
// CapLadderToSource as "unknown, don't cap". Width/Height are *int on the
// domain type so a missing probe doesn't conflate "unknown" with "zero pixels".
func SourceDimsOf(streams []*library.MediaStream) SourceDims {
	var dims SourceDims
	for _, s := range streams {
		if s != nil && s.StreamType == "video" {
			if s.Width != nil {
				dims.Width = *s.Width
			}
			if s.Height != nil {
				dims.Height = *s.Height
			}
			dims.Codec = s.Codec
			dims.TenBit = StreamIsTenBit(s)
			dims.HDRRange = SourceCopyVideoRange(streams)
			break
		}
	}
	return dims
}

// SourceAudioChannels is the channel count of the audioIdx-th AUDIO stream, or
// 0 when the stream does not exist or was never probed. Audio-relative index,
// like SourceAudioCodec.
//
// 0 means UNKNOWN and callers must treat it as such: the wasm-audio gate reads
// this to refuse handing a Dolby track the client has to decode itself when it
// carries more channels than a constrained device sustains (an E-AC-3 7.1 track
// silenced a Chromecast and got the receiver killed, prod 2026-07-30), and
// guessing "probably stereo" here would re-open exactly that door.
func SourceAudioChannels(streams []*library.MediaStream, audioIdx int) int {
	n := 0
	for _, s := range streams {
		if s == nil || s.StreamType != "audio" {
			continue
		}
		if n == audioIdx {
			if s.Channels == nil || *s.Channels < 0 {
				return 0
			}
			return *s.Channels
		}
		n++
	}
	return 0
}
