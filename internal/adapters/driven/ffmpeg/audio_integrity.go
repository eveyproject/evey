package ffmpeg

// Every audio track of a file, measured end to end.
//
// The sampled audibility guard (audio_sanity.go) decodes three eight-second
// windows and is the right price for an index rebuild. A rewrite that moves or
// drops audio bytes is judged here instead, on the whole track: where it is
// silent and for how long (ffmpeg silencedetect over a full decode), and
// whether every frame has the same size (ffprobe packet sizes, no decode).
// The second measurement is the one that catches the failure a full decode
// cannot see: a track whose frames change format at a joint decodes without a
// single error and plays silent (prod, 2026-09-01).

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/eveyproject/evey/internal/pkg/procfork"

	"github.com/eveyproject/evey/internal/domain/mkv"
)

const (
	// integritySilenceDB is the loudness under which a stretch is silence.
	// Digital silence reads -91 dB; the quietest room tone measured on real
	// mixes sits above -70.
	integritySilenceDB = -75.0
	// integrityMaxFrameSizes bounds the distinct-size set kept per track: past
	// a handful the codec is simply variable-frame and the count means
	// nothing more.
	integrityMaxFrameSizes = 16
)

// constantFrameCodecs are the codecs whose packets are all the same size on
// a sound track (constant bitrate, fixed samples per frame). Anything else
// reports FrameSizes 0 - the check does not apply.
var constantFrameCodecs = map[string]bool{"ac3": true, "eac3": true, "dts": true, "mp2": true}

// AudioIntegrity implements maintenance.AudioIntegrityProber with ffmpeg
// (silence map) and ffprobe (streams, duration, packet sizes).
type AudioIntegrity struct {
	ffmpegPath  string
	ffprobePath string
}

// NewAudioIntegrity builds the prober from the two tool paths.
func NewAudioIntegrity(ffmpegPath, ffprobePath string) *AudioIntegrity {
	return &AudioIntegrity{ffmpegPath: ffmpegPath, ffprobePath: ffprobePath}
}

// ProbeAudioIntegrity measures every audio track. An error means the file
// could not be measured at all; the caller reads that as "no evidence".
func (p *AudioIntegrity) ProbeAudioIntegrity(ctx context.Context, path string) ([]mkv.TrackIntegrity, error) {
	codecs, err := audioStreamCodecs(ctx, p.ffprobePath, path)
	if err != nil {
		return nil, err
	}
	if len(codecs) == 0 {
		return nil, nil
	}
	dur, err := mediaDurationSec(ctx, p.ffprobePath, path)
	if err != nil {
		return nil, err
	}
	sizes, err := audioPacketSizeSets(ctx, p.ffprobePath, path)
	if err != nil {
		return nil, err
	}

	out := make([]mkv.TrackIntegrity, 0, len(codecs))
	for i, codec := range codecs {
		stderr, runErr := p.silenceLog(ctx, path, i)
		if runErr != nil {
			return nil, fmt.Errorf("audio integrity: track %d: %w", i, runErr)
		}
		audible, stretches := parseSilenceLog(stderr, dur)
		t := mkv.TrackIntegrity{Track: i, Codec: codec, AudibleSec: audible, SilentStretches: stretches}
		if constantFrameCodecs[codec] {
			t.FrameSizes = len(sizes[i])
		}
		out = append(out, t)
	}
	return out, nil
}

// silenceLog decodes one whole track through silencedetect and returns
// ffmpeg's log, where the filter reports every stretch it found.
func (p *AudioIntegrity) silenceLog(ctx context.Context, path string, track int) (string, error) {
	filter := fmt.Sprintf("silencedetect=n=%vdB:d=%v", integritySilenceDB, mkv.AudioIntegritySilenceSec)
	out, err := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Scan, Site: "audio_integrity.silence"}, p.ffmpegPath,
		"-v", "info", "-nostats", "-nostdin",
		"-vn", "-sn", "-dn",
		"-i", path, "-map", fmt.Sprintf("0:a:%d", track),
		"-af", filter, "-f", "null", "-")
	if err != nil {
		return "", fmt.Errorf("silencedetect: %w: %s", err, firstLine(out))
	}
	return string(out), nil
}

// parseSilenceLog turns silencedetect's log into audible seconds and the
// number of silent stretches. A stretch still open at the end of the log (the
// track ends in silence) runs to the track's end.
func parseSilenceLog(log string, durationSec float64) (audibleSec float64, stretches int) {
	silent := 0.0
	openStart := math.NaN()
	sc := bufio.NewScanner(strings.NewReader(log))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, "silence_start: "); i >= 0 {
			if v, err := strconv.ParseFloat(strings.TrimSpace(line[i+len("silence_start: "):]), 64); err == nil {
				openStart = v
			}
			continue
		}
		if i := strings.Index(line, "silence_duration: "); i >= 0 {
			if v, err := strconv.ParseFloat(strings.TrimSpace(line[i+len("silence_duration: "):]), 64); err == nil {
				silent += v
				stretches++
				openStart = math.NaN()
			}
		}
	}
	if !math.IsNaN(openStart) && durationSec > openStart {
		silent += durationSec - openStart
		stretches++
	}
	audibleSec = durationSec - silent
	if audibleSec < 0 {
		audibleSec = 0
	}
	return audibleSec, stretches
}

// audioStreamCodecs lists the codec of every audio stream, in audio order.
func audioStreamCodecs(ctx context.Context, ffprobePath, path string) ([]string, error) {
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "audio_integrity.codecs"}, ffprobePath,
		"-v", "error", "-select_streams", "a",
		"-show_entries", "stream=codec_name", "-of", "csv=p=0", path)
	if err != nil {
		return nil, fmt.Errorf("audio integrity: streams: %w", err)
	}
	var codecs []string
	for line := range strings.SplitSeq(string(out), "\n") {
		if c := strings.TrimSpace(strings.TrimSuffix(line, ",")); c != "" {
			codecs = append(codecs, c)
		}
	}
	return codecs, nil
}

// audioPacketSizeSets reads every audio packet's size in ONE pass over the
// file (no decode) and returns, per audio stream in audio order, the set of
// distinct sizes seen - capped, because past a handful the codec is simply
// variable-frame.
func audioPacketSizeSets(ctx context.Context, ffprobePath, path string) (map[int]map[int]struct{}, error) {
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "audio_integrity.packets"}, ffprobePath,
		"-v", "error", "-select_streams", "a",
		"-show_entries", "packet=stream_index,size", "-of", "csv=p=0", path)
	if err != nil {
		return nil, fmt.Errorf("audio integrity: packets: %w", err)
	}
	return parsePacketSizes(string(out)), nil
}

// parsePacketSizes maps "stream_index,size" rows onto audio-order track
// indexes (the first stream index seen is audio track 0, and so on).
func parsePacketSizes(csv string) map[int]map[int]struct{} {
	order := map[int]int{} // stream_index -> audio track
	sets := map[int]map[int]struct{}{}
	sc := bufio.NewScanner(strings.NewReader(csv))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		a, b, ok := strings.Cut(strings.TrimSpace(sc.Text()), ",")
		if !ok {
			continue
		}
		idx, err1 := strconv.Atoi(a)
		size, err2 := strconv.Atoi(strings.TrimSuffix(b, ","))
		if err1 != nil || err2 != nil {
			continue
		}
		track, seen := order[idx]
		if !seen {
			track = len(order)
			order[idx] = track
			sets[track] = map[int]struct{}{}
		}
		if len(sets[track]) < integrityMaxFrameSizes {
			sets[track][size] = struct{}{}
		}
	}
	return sets
}
