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

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
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

		// Codec must land in ffprobe vocabulary. The track resolves what its
		// CodecID alone cannot (a VfW FourCC, PCM by bit depth, "/" ids such as
		// A_MPEG/L3) and hands back the raw CodecID when it cannot: never a
		// guess. A raw id is unmapped -> ffprobe decides (WMV3/WVC1/HEVC in VfW).
		codec := t.FFprobeCodecName()
		if !ebml {
			// An MP4 timed-text track (tx3g) reads as "srt" and FFprobeCodecName
			// says "subrip"; ffprobe says "mov_text". The container knows.
			codec = c.TrackCodecName(t)
		}
		if codec == "" || strings.Contains(codec, "/") || isRawCodecID(codec) {
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
		ms.HandlerName = t.HandlerName
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
			// mkvgo >= v0.31 splits the verdict (ScanType: progressive /
			// interlaced) from the order (FieldOrder: tt/bb/tb/bt, known far
			// less often). Persist the finer word when the order is known, the
			// verdict otherwise; the domain reads both through ScanType().
			ms.FieldOrder = library.NormalizeFieldOrder(t.FieldOrder)
			if ms.FieldOrder == nil {
				ms.FieldOrder = library.NormalizeFieldOrder(t.ScanType)
			}
			// VP8, VP9 and AV1 have no interlaced coding at all: a track that
			// says nothing is progressive, which is what ffprobe reports for
			// them (parity gap measured on a VP8 WebM, 2026-09-29).
			if ms.FieldOrder == nil && codecNeverInterlaced(codec) {
				ms.FieldOrder = library.NormalizeFieldOrder("progressive")
			}
			// A VP9 CodecPrivate in the Matroska "Codec Feature Metadata" form
			// ({id, len 1, value} records: profile, level, bit depth, chroma -
			// what ffmpeg and mkvmerge write) is not a vpcC. A reader that took
			// it for one produced a wrong profile, an invalid level and a bogus
			// colour (Sprite Fright, 2026-09-29: "Profile 1", level 1, ycgco).
			// Until the pinned mkvgo parses it, such a track is ffprobe's.
			if codec == "vp9" && isVP9FeatureMetadata(t.CodecPrivate) {
				return nil, false
			}
			// A Matroska track without DefaultDuration has no FrameRate; mkvgo
			// 0.50 estimates one from the statistics tags or the first block
			// timestamps, each equal to ffprobe's r_frame_rate or avg_frame_rate
			// on the files it was measured on. The ffprobe fallback stores that
			// same rate, so the estimate is stored the same way.
			if t.FrameRate != nil && *t.FrameRate > 0 {
				fr := *t.FrameRate
				ms.Framerate = &fr
			} else if t.EstimatedFrameRate != nil && *t.EstimatedFrameRate > 0 {
				fr := *t.EstimatedFrameRate
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
			if codec == "aac" && ms.Profile == "" {
				// The profile ffprobe names, read where it lives: the
				// AudioSpecificConfig's object type - what a copied track's esds
				// carries, so what a manifest must declare (mp4a.40.<AOT>).
				ms.Profile = aacProfileFromASC(t.CodecPrivate)
			}
			// The channels the stream states, read from its first frame or its
			// AudioSpecificConfig, replace the container's only when they are
			// at least as many (a Matroska track without Channels, an AAC whose
			// program config element counts 6 behind a container's 2). Fewer is
			// not trusted on one frame: a broadcast AC-3 capture can open on a
			// stereo advert inside a 5.1 programme, a 7.1 E-AC-3 states its two
			// extra channels in a dependent substream mkvgo 0.51 does not count,
			// and an HE-AACv2 with implicit Parametric Stereo has a mono core
			// that decodes to stereo. Every audio decision (copy, downmix, the
			// client's channel cap) reads this count.
			channels := t.Channels
			if t.StreamChannels != nil && *t.StreamChannels > 0 &&
				(t.Channels == nil || *t.StreamChannels >= *t.Channels) {
				channels = t.StreamChannels
			}
			if channels != nil && *channels > 0 {
				ch := int(*channels)
				ms.Channels = &ch
			}
			// The rate ffprobe and ffmpeg's decoder report: the SBR-doubled
			// output only when the AudioSpecificConfig signals SBR (the same
			// test that names the profile HE-AAC). A Matroska
			// OutputSamplingFrequency over an LC config is implicit SBR, which
			// ffmpeg decodes at the core rate (test5: 22050, not 44100).
			rate := t.EffectiveSampleRate()
			if codec == "aac" && !strings.HasPrefix(ms.Profile, "HE-AAC") && t.SampleRate != nil && *t.SampleRate > 0 {
				rate = *t.SampleRate
			}
			if rate > 0 {
				sr := int(math.Round(rate))
				ms.SampleRate = &sr
			}
			counted := *t
			counted.Channels = channels
			if cl := counted.ChannelLayout(); cl != "" {
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
	// The dispatcher only routes EBML (Matroska/WebM) and ISO-BMFF here. A
	// WebM says so in its EBML DocType (mkvgo >= v0.35.0 reads it): report
	// "webm", the container a browser plays raw (served as video/webm); any
	// other EBML is "matroska", which a browser does not take raw (Firefox
	// refuses video/x-matroska). ffprobe classifies everything else - and
	// names both "matroska" (one demuxer), so the decision keeps its own
	// extension check for that path (checkContainer).
	container := "mp4"
	if ebml {
		container = "matroska"
		if c.IsWebM() {
			container = "webm"
		}
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

// aacProfileNames are ffprobe's names for the AAC audio object types.
var aacProfileNames = map[uint32]string{1: "Main", 2: "LC", 3: "SSR", 4: "LTP", 5: "HE-AAC", 23: "LD", 29: "HE-AACv2", 39: "ELD"}

// aacProfileFromASC names the audio object type an AudioSpecificConfig
// declares first (5 bits, 31 escapes to 32 + 6 more): HE-AAC and HE-AACv2
// when SBR / PS are signalled explicitly, LC for an implicit one - the frames
// alone carry it, and the codecs value of the copied track is the ASC's
// type. "" when there is no config or the type is not one of ffprobe's.
func aacProfileFromASC(asc []byte) string {
	if len(asc) == 0 {
		return ""
	}
	aot := uint32(asc[0] >> 3)
	if aot == 31 {
		if len(asc) < 2 {
			return ""
		}
		aot = 32 + (uint32(asc[0]&0x07)<<3 | uint32(asc[1]>>5))
	}
	return aacProfileNames[aot]
}

// codecNeverInterlaced reports the codecs whose bitstream has no interlaced
// mode: an unknown field order on them is "progressive", not "unknown".
func codecNeverInterlaced(codec string) bool {
	switch codec {
	case "vp8", "vp9", "av1":
		return true
	}
	return false
}

// isVP9FeatureMetadata recognises the Matroska "VP9 Codec Feature Metadata"
// CodecPrivate: a sequence of {id, length, value} records, each 3 bytes with
// length 1 and id in 1..4 (Profile, Level, Bit depth, Chroma subsampling). A
// vpcC (the other shape a VP9 CodecPrivate takes) starts with version 0x01
// followed by three flag bytes, so its second byte is 0, never 1.
func isVP9FeatureMetadata(cp []byte) bool {
	if len(cp) == 0 || len(cp)%3 != 0 || len(cp) > 12 {
		return false
	}
	for i := 0; i < len(cp); i += 3 {
		if cp[i] < 1 || cp[i] > 4 || cp[i+1] != 1 {
			return false
		}
	}
	return true
}

// isRawCodecID reports a Matroska CodecID left unresolved (V_MS/VFW/FOURCC,
// A_QUICKTIME, S_TEXT/WEBVTT...): an uppercase family letter, then "_". ffprobe
// names are lowercase, so no real codec name looks like this.
func isRawCodecID(codec string) bool {
	return len(codec) > 2 && codec[1] == '_' && strings.ContainsRune("VASBD", rune(codec[0]))
}
