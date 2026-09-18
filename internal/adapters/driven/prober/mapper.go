// Package prober provides a StreamProber that fast-paths containers mkvgo can
// read head-only - .mkv/.webm via the Matroska reader and .mp4/.m4v via
// mp4.OpenMeta - through the in-process mkvgo path, and falls back to the
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
// channels, samplerate, colour/HDR, duration). It returns ok=false - NOT an
// error - whenever it cannot guarantee that equivalence, so the dispatcher can
// cleanly fall back to ffprobe. Conservative by design: when in doubt, bail.
// mapContainer's ebml argument is true for Matroska/WebM sources and false for
// MP4: only Matroska defaults a track's language to "eng" when no Language
// element is present (the spec default, which ffmpeg applies too), so without it
// a tag-less audio track would not match an English-preferring selection the way
// the ffprobe authority does.
func mapContainer(c *extmkv.Container, ebml bool) (*library.ProbeResult, bool) {
	if c == nil || len(c.Tracks) == 0 {
		return nil, false
	}

	streams := make([]*library.MediaStream, 0, len(c.Tracks))
	for i := range c.Tracks {
		t := &c.Tracks[i]

		st := mapStreamType(t.Type)
		if st == "" {
			return nil, false // unknown/attachment/button track - let ffprobe classify it
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
			StreamIndex:      i,
			StreamType:       st,
			Codec:            codec,
			Profile:          t.Profile, // from the SPS as of mkvgo v0.6.0; "" otherwise (== ffprobe when absent)
			IsDefault:        t.IsDefault,
			IsForced:         t.IsForced,
			Commentary:       t.Commentary,
			HearingImpaired:  t.HearingImpaired,
			VisualImpaired:   t.VisualImpaired,
			TextDescriptions: t.TextDescriptions,
			Original:         t.Original,
		}
		// Language from the carried element (ResolvedLanguage prefers BCP-47); for
		// Matroska, fall back to the spec default "eng" when no element is present
		// so the result matches ffprobe, which applies the same default.
		if t.LanguagePresent {
			ms.Language = t.ResolvedLanguage()
		} else if ebml {
			ms.Language = "eng"
		}
		if t.Name != "" {
			ms.Title = t.Name
		}
		if cln := t.CodecLongName(); cln != "" {
			ms.CodecLongName = cln
		}
		// Per-stream average bitrate. MP4 from btrt/esds (== ffprobe's bit_rate).
		// Matroska from the per-track BPS tag: the fast path reads it head-only via
		// mkvgo's WithBitrate (SeekHead → Tags), and the ffprobe fallback reads the
		// same TAG:BPS (ffprobe's own stream bit_rate stays N/A for MKV) - so both
		// prober paths agree, and both carry the per-track rate ffprobe surfaces only
		// as a tag. Unblocks the music scan.
		if t.Bitrate != nil && *t.Bitrate > 0 {
			br := int64(*t.Bitrate)
			ms.Bitrate = &br
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
			// Codec level: the copy rung's RFC-6381 string needs it and
			// copy_eligibility bails when nil, so without it a fast-pathed video
			// loses the copy rung until the level-backfill worker fills it.
			if t.Level != nil && *t.Level > 0 {
				lvl := int(*t.Level)
				ms.Level = &lvl
			}
			// SAR only when anamorphic (mkvgo reports "1:1" for square pixels).
			if sar := t.SampleAspectRatio(); sar != "" && sar != "1:1" {
				ms.SampleAspectRatio = &sar
			}
			// mkvgo's field_order is a verdict (progressive/interlaced), not an
			// order; the domain reads both vocabularies. Should mkvgo ever split
			// a scan type out of it, map that one here and keep this word raw.
			ms.FieldOrder = library.NormalizeFieldOrder(t.FieldOrder)
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
			// Dolby Vision configuration (dvcC/dvvC), read head-only by mkvgo and
			// validated byte-for-byte against ffprobe's DOVI record (0 mismatch on
			// 100 real HDR titles). The playback decision keys on these
			// (copy_eligibility): profile 8 with an HDR10-compatible base copies,
			// profile 5 transcodes.
			if t.DolbyVision != nil {
				p := int(t.DolbyVision.Profile)
				ms.DoViProfile = &p
				c := int(t.DolbyVision.BLSignalCompatID)
				ms.DoViBLCompat = &c
			}
			// HDR-capable codec with no colour metadata: distinguish "confirmed
			// unspecified" from "could not read". mkvgo sets ColourDetermined when it
			// actually parsed a colour source (container Colour, MP4 colr, or the SPS
			// VUI / AV1 color_config / vpcC) - even when the result resolves to
			// unspecified (true SDR, validated on 230/230 real HEVC/AV1 SDR titles).
			// Only when the colour was never read (ColourDetermined=false - e.g. a bare
			// hvcC with no SPS and in-band recovery off) might real HDR be hiding in the
			// bitstream, so we bail to ffprobe there; a confirmed-unspecified track
			// fast-paths as the SDR it is.
			if ms.ColorSpace == nil && isHDRCapableCodec(codec) && !t.ColourDetermined {
				return nil, false
			}
			// Only Dolby Vision profile 5 (IPT base, no HDR10 layer) stays on ffprobe:
			// served as plain HDR10 it renders green/purple, and no P5 title exists in
			// the corpus to validate mkvgo head-only against, so ffprobe keeps authority.
			// Every other case -- plain HDR10, P8.1, and P7 (HDR10 base + RPU) -- is read
			// byte-identical to ffprobe (DoVi profile/compat + colour + track count, 0
			// mismatch on the real library incl. P7/c6 single-track titles), so the
			// fast-path output equals the ffprobe output and the decision is unchanged.
			if isHDRCapableCodec(codec) && isHDRSuspect(ms.ColorSpace, ms.ColorTransfer) {
				if ms.DoViProfile != nil && *ms.DoViProfile == 5 {
					return nil, false
				}
			}
		case library.StreamTypeAudio:
			if t.Channels != nil && *t.Channels > 0 {
				ch := int(*t.Channels)
				ms.Channels = &ch
			}
			// EffectiveSampleRate = decoder OUTPUT rate (HE-AAC SBR doubles the core).
			if esr := t.EffectiveSampleRate(); esr > 0 {
				sr := int(math.Round(esr))
				ms.SampleRate = &sr
			}
			if cl := t.ChannelLayout(); cl != "" {
				ms.ChannelLayout = &cl
			}
			if t.BitDepth != nil && *t.BitDepth > 0 {
				bd := int(*t.BitDepth)
				ms.BitDepth = &bd
			}
		}
		// CodecLongName, ChannelLayout and Bitrate are populated above (mkvgo 0.9.0+),
		// keeping the fast path field-equivalent to ffprobe across the whole stream.

		streams = append(streams, ms)
	}

	dur, err := shared.NewDurationMs(c.DurationMs)
	if err != nil {
		dur = shared.Duration{} // negative/corrupt duration -> unknown, scan recovers
	}
	// The dispatcher only routes EBML (Matroska/WebM) and ISO-BMFF here; the
	// fast-path can't tell WebM from MKV, so report the family name "matroska"
	// (checkContainer normalises it to "mkv", and WebM is a Matroska subset the
	// same players handle). ffprobe classifies everything else.
	container := "mp4"
	if ebml {
		container = "matroska"
	}
	return &library.ProbeResult{Streams: streams, Duration: dur, Container: container}, true
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
