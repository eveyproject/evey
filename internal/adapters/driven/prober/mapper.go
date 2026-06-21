// Package prober provides a StreamProber that fast-paths containers mkvgo can
// read head-only — .mkv/.webm via the Matroska reader and .mp4/.m4v via
// mp4.OpenMeta — through the in-process mkvgo path, and falls back to the
// ffprobe prober for everything else (and for any file whose metadata it cannot
// render field-equivalent to ffprobe). ffprobe stays the authority; the mkvgo
// path is a pure optimization (no fork, reads only the file head).
package prober

import (
	"math"
	"strings"

	"github.com/google/uuid"
	extmkv "github.com/gravity-zero/mkvgo/mkv"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// mapContainer converts a mkvgo *Container into a library.ProbeResult that is
// field-equivalent to what the ffprobe prober would produce for the
// decision-affecting fields (codec, language, default/forced, dimensions,
// channels, samplerate, colour/HDR, duration). It returns ok=false — NOT an
// error — whenever it cannot guarantee that equivalence, so the dispatcher can
// cleanly fall back to ffprobe. Conservative by design: when in doubt, bail.
func mapContainer(c *extmkv.Container) (*library.ProbeResult, bool) {
	if c == nil || len(c.Tracks) == 0 {
		return nil, false
	}

	streams := make([]*library.MediaStream, 0, len(c.Tracks))
	for i := range c.Tracks {
		t := &c.Tracks[i]

		st := mapStreamType(t.Type)
		if st == "" {
			return nil, false // unknown/attachment/button track — let ffprobe classify it
		}

		// Codec must land in ffprobe vocabulary. mkvgo stores a short name for a
		// known CodecID, else the raw CodecID (e.g. "V_REAL/..."). FFprobeCodecName
		// normalizes the known ones (srt->subrip, pgs->hdmv_pgs_subtitle, ...) and
		// passes unknowns through unchanged; a passed-through raw CodecID still
		// contains "/", which we treat as unmapped -> fall back.
		codec := extmkv.FFprobeCodecName(t.Codec)
		if codec == "" || strings.Contains(codec, "/") {
			return nil, false
		}

		ms := &library.MediaStream{
			ID: uuid.New().String(),
			// Surfaced-track ordinal, not the ffprobe global stream index (ffprobe
			// counts attachments mkvgo never surfaces). Display-only: track
			// selection re-counts per stream-type, so this divergence is benign.
			StreamIndex: i,
			StreamType:  st,
			Codec:       codec,
			Profile:     t.Profile, // from the SPS as of mkvgo v0.6.0; "" otherwise (== ffprobe when absent)
			IsDefault:   t.IsDefault,
			IsForced:    t.IsForced,
		}
		// Language only when the file actually carried one (mirror ffprobe's
		// "only if tags[language] present"); ResolvedLanguage prefers BCP-47.
		if t.LanguagePresent {
			ms.Language = t.ResolvedLanguage()
		}
		if t.Name != "" {
			ms.Title = t.Name
		}

		switch st {
		case library.StreamTypeVideo:
			// Dimensions feed the resolution-cap decision; ffprobe always has
			// them, so a video track without both is a fall-back trigger.
			if t.Width == nil || t.Height == nil || *t.Width == 0 || *t.Height == 0 {
				return nil, false
			}
			w, h := int(*t.Width), int(*t.Height)
			ms.Width, ms.Height = &w, &h
			if t.FrameRate != nil && *t.FrameRate > 0 {
				fr := *t.FrameRate
				ms.Framerate = &fr
			}
			if t.VideoBitDepth != nil && *t.VideoBitDepth > 0 {
				bd := int(*t.VideoBitDepth)
				ms.BitDepth = &bd
			}
			if cs := t.ColorSpaceName(); cs != "" {
				ms.ColorSpace = &cs
			}
			if ct := t.ColorTransferName(); ct != "" {
				ms.ColorTransfer = &ct
			}
			// Belt-and-suspenders for the HDR decision: an HDR-capable codec with
			// no colour metadata at all (mkvgo couldn't read it from the container
			// Colour element nor the SPS VUI) might actually be HDR signalled in a
			// way mkvgo missed. ffprobe would catch it from the bitstream, so fall
			// back rather than risk reporting real HDR as SDR.
			if ms.ColorSpace == nil && isHDRCapableCodec(codec) {
				return nil, false
			}
			// HDR candidates must go through ffprobe: mkvgo cannot read the Dolby
			// Vision configuration (MKV BlockAdditionMapping dvcC/dvvC), and a DoVi
			// profile-5 title recorded as plain HDR10 would be raw-served with
			// green/purple colours by the HDR-aware copy/direct-play paths. Any
			// bt2020 colour or PQ/HLG transfer ⇒ bail so ffprobe establishes the
			// dovi_profile ground truth. SDR tracks (the vast majority) keep the
			// fast path.
			if isHDRCapableCodec(codec) && isHDRSuspect(ms.ColorSpace, ms.ColorTransfer) {
				return nil, false
			}
		case library.StreamTypeAudio:
			if t.Channels != nil && *t.Channels > 0 {
				ch := int(*t.Channels)
				ms.Channels = &ch
			}
			if t.SampleRate != nil && *t.SampleRate > 0 {
				sr := int(math.Round(*t.SampleRate))
				ms.SampleRate = &sr
			}
			if t.BitDepth != nil && *t.BitDepth > 0 {
				bd := int(*t.BitDepth)
				ms.BitDepth = &bd
			}
		}
		// Per-stream Bitrate is left nil: mkvgo has none, and ffprobe itself
		// reports none for MKV video (Matroska stores no per-track bitrate), so
		// nil is field-equivalent for the load-bearing video case. ChannelLayout
		// and CodecLongName are left nil/empty (display-only, no decision impact).

		streams = append(streams, ms)
	}

	dur, err := shared.NewDurationMs(c.DurationMs)
	if err != nil {
		dur = shared.Duration{} // negative/corrupt duration -> unknown, scan recovers
	}
	return &library.ProbeResult{Streams: streams, Duration: dur}, true
}

func mapStreamType(t extmkv.TrackType) library.StreamType {
	switch t {
	case extmkv.VideoTrack:
		return library.StreamTypeVideo
	case extmkv.AudioTrack:
		return library.StreamTypeAudio
	case extmkv.SubtitleTrack:
		return library.StreamTypeSubtitle
	}
	return ""
}

// isHDRCapableCodec reports codecs that commonly carry HDR signalling in the
// bitstream (so a missing colour read is suspicious). h264 is excluded: H.264
// HDR is effectively nonexistent and h264 SDR legitimately has no colour read.
func isHDRCapableCodec(codec string) bool {
	switch codec {
	case "hevc", "av1", "vp9":
		return true
	}
	return false
}

// isHDRSuspect reports colour metadata that marks a potential HDR (and thus
// potential Dolby Vision) stream: bt2020 colour space or a PQ/HLG transfer.
// Such tracks must be probed by ffprobe, the only prober that can read the
// DOVI configuration record.
func isHDRSuspect(colorSpace, colorTransfer *string) bool {
	if colorSpace != nil && strings.Contains(strings.ToLower(*colorSpace), "bt2020") {
		return true
	}
	if colorTransfer != nil {
		switch strings.ToLower(strings.TrimSpace(*colorTransfer)) {
		case "smpte2084", "arib-std-b67":
			return true
		}
	}
	return false
}
