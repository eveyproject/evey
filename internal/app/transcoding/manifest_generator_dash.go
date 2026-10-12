package transcoding

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// ErrDASHUnavailable is what MPD returns for a presentation whose timeline
// cannot be declared exactly - a transcode on the legacy clock grid (unknown
// or variable frame rate), a copy rung without its keyframe plan - or whose
// audio is not a separate rendition. HLS
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
// rendition is one AdaptationSet each (selected, not switched). URIs carry the same query suffix as the
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
	audioSeparate := len(in.AudioRenditions) > 0 && !in.ExternalAudio
	if !audioSeparate {
		// Audio muxed into the video fragments is not a DASH presentation the
		// players take: dash.js drops a Representation declaring two codecs
		// ("No streams to play", measured on dash.js 5.2), DASH-IF IOP excludes
		// multiplexed Representations, and audio the client decodes itself
		// (wasm) has no place in an MPD. HLS serves both.
		return "", ErrDASHUnavailable
	}
	durMs := in.DurationMs()
	suffix := xmlAttr(querySuffix(in))

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&b, `<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" profiles="urn:mpeg:dash:profile:isoff-main:2011" mediaPresentationDuration="%s" minBufferTime="PT%.3fS">`+"\n",
		isoDuration(in.DurationSec), in.Grid.SegmentSeconds())
	b.WriteString("  <Period>\n")

	// --- video: the switch set ---
	frameRate := ""
	if fr := in.Grid.FrameRateString(); fr != "" {
		frameRate = fmt.Sprintf(` frameRate="%s"`, fr)
	}
	fmt.Fprintf(&b, `    <AdaptationSet mimeType="video/mp4" contentType="video" segmentAlignment="true" startWithSAP="1"%s>`+"\n", frameRate)
	tenBit := in.TranscodeVideoRange != ""
	for i, p := range in.Profiles {
		codecs := videoCodecAttrFor(strings.ToLower(in.ChosenVideoCodec), p, tenBit, in.Grid.FPS())
		if p.Copy && in.CopyVideoCodec != "" {
			codecs = in.CopyVideoCodec
		}
		bps := p.VideoBitrate * 1000
		// The fragments' ladder name: a fitted plan's one rung keeps its own.
		rung := in.FirstRungIndex + i
		fmt.Fprintf(&b, `      <Representation id="v%d" bandwidth="%d" width="%d" height="%d" codecs="%s">`+"\n", rung, bps, p.Width, p.Height, xmlAttr(codecs))
		if copyRung {
			fmt.Fprintf(&b, `        <SegmentTemplate initialization="stream_%d_init.mp4%s" media="stream_%d_$Number%%03d$.m4s%s" startNumber="0" timescale="1000">`+"\n", rung, suffix, rung, suffix)
			writeCopyTimeline(&b, in.CopySegments)
		} else {
			fmt.Fprintf(&b, `        <SegmentTemplate initialization="stream_%d_init.mp4%s" media="stream_%d_$Number%%03d$.m4s%s" startNumber="0" timescale="%d">`+"\n", rung, suffix, rung, suffix, in.Grid.Timescale)
			writeGridTimeline(&b, in.Grid, durMs)
		}
		b.WriteString("        </SegmentTemplate>\n")
		b.WriteString("      </Representation>\n")
	}
	b.WriteString("    </AdaptationSet>\n")

	// --- audio: one set per rendition ---
	writeAudioSets(&b, in, suffix)

	b.WriteString("  </Period>\n")
	b.WriteString("</MPD>\n")
	return b.String(), nil
}

// packagerAudioSetRE is one audio AdaptationSet of the packager's manifest.
var packagerAudioSetRE = regexp.MustCompile(`(?s)[ \t]*<AdaptationSet[^>]*contentType="audio"[^>]*>.*?</AdaptationSet>\n?`)

// HybridMPD is the DASH manifest of a hybrid presentation: the packager's
// video (and subtitles), whose timeline only the packager knows, beside
// Evey's separate audio renditions - the manifest counterpart of the hybrid
// HLS master, over the same fragments with the same query suffix. The
// packager's own audio sets describe the source's tracks, not the plan's:
// they are dropped. packagerMPD is the packager's manifest as served (its
// URIs already carry their suffix).
func (g *ManifestGenerator) HybridMPD(in ManifestInput, packagerMPD string) (string, error) {
	if err := in.Validate(); err != nil {
		return "", err
	}
	if len(in.AudioRenditions) == 0 || in.ExternalAudio {
		return "", ErrDASHUnavailable
	}
	cut := strings.LastIndex(packagerMPD, "</Period>")
	if cut < 0 || strings.Count(packagerMPD, "<Period") != 1 || !strings.Contains(packagerMPD, `contentType="video"`) {
		return "", fmt.Errorf("hybrid mpd: the packager's manifest is not one period with a video set")
	}
	head := strings.TrimRight(packagerAudioSetRE.ReplaceAllString(packagerMPD[:cut], ""), " \t")
	var b strings.Builder
	b.WriteString(head)
	writeAudioSets(&b, in, xmlAttr(querySuffix(in)))
	b.WriteString("  ")
	b.WriteString(packagerMPD[cut:])
	return b.String(), nil
}

// writeAudioSets writes one AdaptationSet per separate audio rendition, the
// requested track as the main one: shared by the plan-declared manifest and
// the hybrid one (the packager's video beside Evey's renditions).
func writeAudioSets(b *strings.Builder, in ManifestInput, suffix string) {
	audioKbps := int64(0)
	if len(in.Profiles) > 0 {
		audioKbps = in.Profiles[0].AudioBitrate
	}
	defaultAudio := defaultAudioRendition(in.AudioRenditions, in.AudioTrack)
	for k, r := range in.AudioRenditions {
		lang := ""
		if r.LanguageCode != "" {
			lang = fmt.Sprintf(` lang="%s"`, xmlAttr(r.LanguageCode))
		}
		fmt.Fprintf(b, `    <AdaptationSet mimeType="audio/mp4" contentType="audio"%s>`+"\n", lang)
		if r.Name != "" {
			fmt.Fprintf(b, "      <Label>%s</Label>\n", xmlAttr(r.Name))
		}
		// The requested track is the main one, as DEFAULT=YES in HLS: without
		// it ?audioTrack= changed only the URLs and Media3 kept picking by the
		// device locale (measured on the TV emulator, 2026-09-25).
		role := "alternate"
		if k == defaultAudio {
			role = "main"
		}
		fmt.Fprintf(b, `      <Role schemeIdUri="urn:mpeg:dash:role:2011" value="%s"/>`+"\n", role)
		// Rate, channels and bits are the rendition's own (the source
		// track's on a copy: a 44.1 kHz AAC copied stays 44.1 kHz).
		kbps := r.BitrateKbps
		if kbps <= 0 {
			kbps = audioKbps
		}
		rate := ""
		if r.SampleRate > 0 {
			rate = fmt.Sprintf(` audioSamplingRate="%d"`, r.SampleRate)
		}
		fmt.Fprintf(b, `      <Representation id="a%d" bandwidth="%d" codecs="%s"%s>`+"\n", k, kbps*1000, xmlAttr(r.codecAttr()), rate)
		if r.Channels > 0 {
			fmt.Fprintf(b, `        <AudioChannelConfiguration schemeIdUri="urn:mpeg:dash:23003:3:audio_channel_configuration:2011" value="%d"/>`+"\n", r.Channels)
		}
		// The nominal grid duration in the rendition's sample rate (its
		// track timescale); the muxer cuts on the first AAC frame past each
		// boundary, so the real spans differ by up to one frame (see the
		// doc above).
		audioTimescale := r.SampleRate
		if audioTimescale <= 0 {
			audioTimescale = 48000
		}
		fmt.Fprintf(b, `        <SegmentTemplate initialization="audio_%d_init.mp4%s" media="audio_%d_$Number%%03d$.m4s%s" startNumber="0" timescale="%d" duration="%d"/>`+"\n",
			r.SourceTrackIdx, suffix, r.SourceTrackIdx, suffix, audioTimescale, int64(math.Round(in.Grid.SegmentSeconds()*float64(audioTimescale))))
		b.WriteString("      </Representation>\n")
		b.WriteString("    </AdaptationSet>\n")
	}
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
