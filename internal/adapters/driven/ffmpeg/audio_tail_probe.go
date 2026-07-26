package ffmpeg

// audio_tail_probe.go - does the audio content actually reach the container's
// declared duration? The defect this measures (prod 2026-07-26, "Scary
// Movie"): a mux whose audio track dies minutes before the video's end. The
// file is structurally healthy - index fine, sizes coherent - so the seek
// -index triage cannot see it, yet the VOD playlists (synthesized from the
// DECLARED duration) promise audio segments that can never exist.
//
// Two-stage probe, cheapest first:
//  1. HEAD: mkvmerge and ffmpeg stamp every track with a DURATION tag; when
//     every audio track carries one, one header-only ffprobe settles the gap.
//  2. TAIL: otherwise read the packets of the last minutes only
//     (-read_intervals) and take each audio stream's last timestamp, widening
//     the window in steps so a deep gap is still measured without ever
//     scanning the whole file.

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// audioTailProbeTimeout bounds each ffprobe run, same reasoning as
// audioDelayProbeTimeout: a probe hung on a dead network mount must not wedge
// the seek-index job subsystem until restart.
const audioTailProbeTimeout = 20 * time.Second

// audioTailWindowsSec are the successive "read the last N seconds" windows of
// the tail fallback. The first covers ordinary files (audio runs to the end),
// the later ones measure deep gaps (the motivating file was 249 s short). A
// stream still silent past the last window is reported AT the window - a lower
// bound, already far past any flagging threshold.
var audioTailWindowsSec = []float64{120, 900, 3600}

// audioTailStream is one audio stream's head-probe view.
type audioTailStream struct {
	Index  int
	EndSec float64 // from the DURATION tag; <0 when untagged
}

// ProbeAudioTailGapSec reports the largest gap, in seconds, between the
// container's declared duration and the point where an audio track's content
// actually ends. 0 = every audio track reaches the end (or the file carries no
// audio / no usable duration). Consumed by the admin seek-index scan through
// the maintenance.AudioTailProber port.
func (p *Prober) ProbeAudioTailGapSec(ctx context.Context, filePath string) (float64, error) {
	head, err := runAudioTailHeadProbe(ctx, p.binary, filePath)
	if err != nil {
		return 0, err
	}
	dur, audio, err := parseAudioTailHead(head)
	if err != nil {
		return 0, err
	}
	if dur <= 0 || len(audio) == 0 {
		return 0, nil
	}
	if gap, allTagged := audioTailGapFromTags(dur, audio); allTagged {
		return gap, nil
	}
	return p.probeAudioTailGapFromPackets(ctx, filePath, dur, audio)
}

// probeAudioTailGapFromPackets is the tail fallback: last packet timestamp per
// audio stream, windows widening until every untagged stream has answered (or
// the window reaches the start of the file).
func (p *Prober) probeAudioTailGapFromPackets(ctx context.Context, filePath string, dur float64, audio []audioTailStream) (float64, error) {
	pending := map[int]bool{}
	for _, a := range audio {
		if a.EndSec < 0 {
			pending[a.Index] = true
		}
	}
	last := map[int]float64{}
	lastWindow := 0.0
	for _, w := range audioTailWindowsSec {
		start := dur - w
		if start < 0 {
			start = 0
		}
		out, err := runAudioTailPacketProbe(ctx, p.binary, filePath, start)
		if err != nil {
			return 0, err
		}
		got, err := parseAudioTailPackets(out)
		if err != nil {
			return 0, err
		}
		for idx, ts := range got {
			if ts > last[idx] {
				last[idx] = ts
			}
			delete(pending, idx)
		}
		lastWindow = w
		if len(pending) == 0 || start == 0 {
			break
		}
	}

	gap := 0.0
	for _, a := range audio {
		end := a.EndSec
		if end < 0 {
			var ok bool
			end, ok = last[a.Index], last[a.Index] > 0
			if !ok {
				// No packet inside the widest window read: the track ends at
				// least that far before the declared duration. Report the
				// window as a LOWER bound (when the window reached the file's
				// start, the track simply has no content at all).
				end = dur - lastWindow
				if end < 0 {
					end = 0
				}
			}
		}
		if g := dur - end; g > gap {
			gap = g
		}
	}
	return gap, nil
}

// runAudioTailHeadProbe reads the container duration and, per stream, the
// codec type and (when the muxer wrote one) the DURATION tag. Header-only.
func runAudioTailHeadProbe(ctx context.Context, binary, filePath string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, audioTailProbeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary,
		"-v", "error",
		"-show_entries", "format=duration:stream=index,codec_type:stream_tags",
		"-of", "json",
		filePath,
	).Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe audio-tail head: %w", err)
	}
	return out, nil
}

// runAudioTailPacketProbe lists the audio packets from startSec to EOF -
// timestamps only, so even a 2-hour tail stays a small JSON.
func runAudioTailPacketProbe(ctx context.Context, binary, filePath string, startSec float64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, audioTailProbeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary,
		"-v", "error",
		"-select_streams", "a",
		"-show_entries", "packet=stream_index,pts_time,dts_time",
		"-read_intervals", fmt.Sprintf("%.3f%%", startSec),
		"-of", "json",
		filePath,
	).Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe audio-tail packets: %w", err)
	}
	return out, nil
}

// parseAudioTailHead extracts (container duration, audio streams) from the
// head probe. EndSec is -1 for a stream without a parseable DURATION tag.
func parseAudioTailHead(out []byte) (float64, []audioTailStream, error) {
	var doc struct {
		Streams []struct {
			Index     int               `json:"index"`
			CodecType string            `json:"codec_type"`
			Tags      map[string]string `json:"tags"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return 0, nil, fmt.Errorf("ffprobe audio-tail parse: %w", err)
	}
	dur, _ := strconv.ParseFloat(doc.Format.Duration, 64)
	var audio []audioTailStream
	for _, s := range doc.Streams {
		if s.CodecType != "audio" {
			continue
		}
		end := -1.0
		for k, v := range s.Tags {
			if !strings.EqualFold(k, "DURATION") {
				continue
			}
			if sec, ok := parseTrackDurationTag(v); ok {
				end = sec
			}
			break
		}
		audio = append(audio, audioTailStream{Index: s.Index, EndSec: end})
	}
	return dur, audio, nil
}

// audioTailGapFromTags resolves the gap from the head probe alone. allTagged
// reports whether every audio stream carried a usable DURATION tag - when one
// did not, the caller must fall back to reading tail packets.
func audioTailGapFromTags(dur float64, audio []audioTailStream) (gap float64, allTagged bool) {
	allTagged = true
	for _, a := range audio {
		if a.EndSec < 0 {
			allTagged = false
			continue
		}
		if g := dur - a.EndSec; g > gap {
			gap = g
		}
	}
	return gap, allTagged
}

// parseTrackDurationTag parses the Matroska DURATION tag form
// "HH:MM:SS.nnnnnnnnn" into seconds.
func parseTrackDurationTag(s string) (float64, bool) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 3 {
		return 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	sec, err3 := strconv.ParseFloat(parts[2], 64)
	if err1 != nil || err2 != nil || err3 != nil || h < 0 || m < 0 || sec < 0 {
		return 0, false
	}
	return float64(h)*3600 + float64(m)*60 + sec, true
}

// parseAudioTailPackets keeps the last timestamp seen per stream index. pts
// wins; dts fills in when the muxer left pts out.
func parseAudioTailPackets(out []byte) (map[int]float64, error) {
	var doc struct {
		Packets []struct {
			StreamIndex int    `json:"stream_index"`
			PtsTime     string `json:"pts_time"`
			DtsTime     string `json:"dts_time"`
		} `json:"packets"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, fmt.Errorf("ffprobe audio-tail packets parse: %w", err)
	}
	last := map[int]float64{}
	for _, p := range doc.Packets {
		ts := p.PtsTime
		if ts == "" || ts == "N/A" {
			ts = p.DtsTime
		}
		v, err := strconv.ParseFloat(ts, 64)
		if err != nil {
			continue
		}
		if v > last[p.StreamIndex] {
			last[p.StreamIndex] = v
		}
	}
	return last, nil
}
