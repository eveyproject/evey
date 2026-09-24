package transcoding

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// ErrDASHUnavailable is what MPD returns for a presentation whose timeline
// cannot be declared exactly: a transcode on the legacy clock grid (unknown
// or variable frame rate), or a copy rung without its keyframe plan. HLS
// still serves it; DASH is not offered.
var ErrDASHUnavailable = errors.New("dash: no exact timeline for this presentation")

// MPD renders the static DASH manifest of the ffmpeg-served presentation the
// HLS master describes, from the plan alone - no fragment is read, none
// needs to exist yet, exactly like the HLS playlists. What makes that
// honest is the grid: every transcoded rung is cut on the same whole number
// of frames, so the timeline is exact in the video track's ticks and
// identical across the rungs of the switch set (the invariant a DASH player
// assumes when it switches quality mid-stream). A copy rung's timeline is its
// keyframe plan, exact to the millisecond.
//
// Video is one AdaptationSet with a Representation per rung; a separate audio
// rendition is one AdaptationSet each (selected, not switched); muxed audio
// rides its rung's codecs attribute. URIs carry the same query suffix as the
// HLS playlists (token, decision inputs, fingerprint seal), so the segment
// handler serves a DASH fetch exactly as an HLS one - the same fragments.
//
// The audio renditions are cut by the muxer on the first AAC frame past each
// boundary, so their spans are not a whole number of anything: their
// SegmentTemplate declares the grid's nominal duration in the rendition's
// sample rate, which addresses segment N within one AAC frame (21 ms) of its
// real start; players place audio by the fragments' own timestamps.
// mkvgo's DASHFromCMAF over the produced fragments is the oracle this
// manifest is checked against (see the mkvgo adapter's CMAF parity check).
func (g *ManifestGenerator) MPD(in ManifestInput) (string, error) {
	if err := in.Validate(); err != nil {
		return "", err
	}
	copyRung := len(in.Profiles) == 1 && in.Profiles[0].Copy
	if copyRung {
		if len(in.CopySegments) == 0 {
			return "", ErrDASHUnavailable
		}
	} else if !in.Grid.Exact() {
		return "", ErrDASHUnavailable
	}
	durMs := in.DurationMs()
	suffix := xmlAttr(querySuffix(in))
	audioSeparate := len(in.AudioRenditions) > 0 && !in.ExternalAudio

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&b, `<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" profiles="urn:mpeg:dash:profile:isoff-main:2011" mediaPresentationDuration="%s" minBufferTime="PT%.3fS">`+"\n",
		isoDuration(in.DurationSec), in.Grid.SegmentSeconds())
	b.WriteString("  <Period>\n")

	// --- video: the switch set ---
	audioAttrs := audioCodecAttr(strings.ToLower(in.ChosenAudioCodec))
	if audioSeparate {
		var attrs []string
		for _, r := range in.AudioRenditions {
			if a := audioCodecAttr(strings.ToLower(r.Codec)); !slices.Contains(attrs, a) {
				attrs = append(attrs, a)
			}
		}
		audioAttrs = strings.Join(attrs, ",")
	}
	frameRate := ""
	if fr := in.Grid.FrameRateString(); fr != "" {
		frameRate = fmt.Sprintf(` frameRate="%s"`, fr)
	}
	fmt.Fprintf(&b, `    <AdaptationSet mimeType="video/mp4" contentType="video" segmentAlignment="true" startWithSAP="1"%s>`+"\n", frameRate)
	tenBit := in.TranscodeVideoRange != ""
	for i, p := range in.Profiles {
		codecs := videoCodecAttrFor(strings.ToLower(in.ChosenVideoCodec), p.Height, tenBit)
		if p.Copy && in.CopyVideoCodec != "" {
			codecs = in.CopyVideoCodec
		}
		bps := p.VideoBitrate * 1000
		if !audioSeparate && !in.ExternalAudio {
			// Muxed audio rides the rung: its codec and its bits.
			codecs += "," + audioAttrs
			bps += p.AudioBitrate * 1000
		}
		fmt.Fprintf(&b, `      <Representation id="v%d" bandwidth="%d" width="%d" height="%d" codecs="%s">`+"\n", i, bps, p.Width, p.Height, xmlAttr(codecs))
		if copyRung {
			fmt.Fprintf(&b, `        <SegmentTemplate initialization="stream_%d_init.mp4%s" media="stream_%d_$Number%%03d$.m4s%s" startNumber="0" timescale="1000">`+"\n", i, suffix, i, suffix)
			writeCopyTimeline(&b, in.CopySegments)
		} else {
			fmt.Fprintf(&b, `        <SegmentTemplate initialization="stream_%d_init.mp4%s" media="stream_%d_$Number%%03d$.m4s%s" startNumber="0" timescale="%d">`+"\n", i, suffix, i, suffix, in.Grid.Timescale)
			writeGridTimeline(&b, in.Grid, durMs)
		}
		b.WriteString("        </SegmentTemplate>\n")
		b.WriteString("      </Representation>\n")
	}
	b.WriteString("    </AdaptationSet>\n")

	// --- audio: one set per rendition ---
	if audioSeparate {
		audioKbps := int64(0)
		if len(in.Profiles) > 0 {
			audioKbps = in.Profiles[0].AudioBitrate
		}
		for k, r := range in.AudioRenditions {
			lang := ""
			if r.LanguageCode != "" {
				lang = fmt.Sprintf(` lang="%s"`, xmlAttr(r.LanguageCode))
			}
			fmt.Fprintf(&b, `    <AdaptationSet mimeType="audio/mp4" contentType="audio"%s>`+"\n", lang)
			codec := strings.ToLower(r.Codec)
			if codec == "" {
				codec = "aac"
			}
			rate := ""
			if codec == "aac" {
				// A re-encoded rendition is 48 kHz AAC (hls_audio_only); a copied
				// track keeps the source's rate, which the plan does not carry.
				rate = ` audioSamplingRate="48000"`
			}
			fmt.Fprintf(&b, `      <Representation id="a%d" bandwidth="%d" codecs="%s"%s>`+"\n", k, audioKbps*1000, xmlAttr(audioCodecAttr(codec)), rate)
			// The nominal grid duration in the rendition's timescale; the
			// muxer cuts on the first AAC frame past each boundary, so the
			// real spans differ by up to one frame (see the doc above).
			const audioTimescale = 48000
			fmt.Fprintf(&b, `        <SegmentTemplate initialization="audio_%d_init.mp4%s" media="audio_%d_$Number%%03d$.m4s%s" startNumber="0" timescale="%d" duration="%d"/>`+"\n",
				r.SourceTrackIdx, suffix, r.SourceTrackIdx, suffix, audioTimescale, int64(math.Round(in.Grid.SegmentSeconds()*audioTimescale)))
			b.WriteString("      </Representation>\n")
			b.WriteString("    </AdaptationSet>\n")
		}
	}

	b.WriteString("  </Period>\n")
	b.WriteString("</MPD>\n")
	return b.String(), nil
}

// writeGridTimeline is the SegmentTimeline of a frame-exact grid: every
// segment SegmentTicks long but the last, which holds the leftover frames.
func writeGridTimeline(b *strings.Builder, g transcoding.SegmentGrid, durMs int64) {
	count := g.SegmentCount(durMs)
	if count <= 0 {
		return
	}
	b.WriteString("          <SegmentTimeline>\n")
	full := count - 1
	if full > 0 {
		if full == 1 {
			fmt.Fprintf(b, `            <S t="0" d="%d"/>`+"\n", g.SegmentTicks())
		} else {
			fmt.Fprintf(b, `            <S t="0" d="%d" r="%d"/>`+"\n", g.SegmentTicks(), full-1)
		}
	}
	last := g.SegmentDurationTicks(count-1, durMs)
	if full == 0 {
		fmt.Fprintf(b, `            <S t="0" d="%d"/>`+"\n", last)
	} else {
		fmt.Fprintf(b, `            <S d="%d"/>`+"\n", last)
	}
	b.WriteString("          </SegmentTimeline>\n")
}

// writeCopyTimeline is the SegmentTimeline of a copy rung: the keyframe
// plan's spans in milliseconds (timescale 1000), runs of equal spans folded
// into a repeat count.
func writeCopyTimeline(b *strings.Builder, segs []CopySegment) {
	b.WriteString("          <SegmentTimeline>\n")
	i := 0
	for i < len(segs) {
		d := segs[i].DurationMs()
		j := i + 1
		for j < len(segs) && segs[j].DurationMs() == d {
			j++
		}
		t := ""
		if i == 0 {
			t = fmt.Sprintf(` t="%d"`, segs[0].StartMs)
		}
		if r := j - i - 1; r > 0 {
			fmt.Fprintf(b, `            <S%s d="%d" r="%d"/>`+"\n", t, d, r)
		} else {
			fmt.Fprintf(b, `            <S%s d="%d"/>`+"\n", t, d)
		}
		i = j
	}
	b.WriteString("          </SegmentTimeline>\n")
}

// isoDuration formats seconds as an ISO 8601 duration ("PT7200.042S").
func isoDuration(sec float64) string {
	return fmt.Sprintf("PT%.3fS", math.Max(sec, 0))
}

// xmlAttr escapes a value for an XML attribute: the query suffix carries '&'.
func xmlAttr(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
