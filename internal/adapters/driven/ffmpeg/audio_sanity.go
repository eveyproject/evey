package ffmpeg

// Does this file still have SOUND?
//
// Not "does it declare audio tracks" - a structural verification already
// answers that, and a silenced track answers it just as well. Two films came
// back from an audio repair on prod (2026-09-01) with their French track
// present in every header and completely silent in every player, and nothing
// in the pipeline had asked the only question that mattered.
//
// So this counts the tracks that carry signal, by decoding a few short windows
// spread across the file and looking at their loudness. It is deliberately
// generous: a track is audible as soon as ONE window has signal, because films
// are full of legitimately silent minutes and the guard must never accuse a
// healthy track. It is the "before" and "after" of every rewrite.

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/eveyproject/evey/internal/pkg/procfork"
)

const (
	// sanityWindowSec is how much of a track is decoded per probe. Long enough
	// to cross a quiet passage, short enough that the whole guard costs a few
	// seconds against a rewrite that costs minutes.
	sanityWindowSec = 8
	// sanityProbePoints are the fractions of the runtime sampled, in order.
	// The first hit wins, so an ordinary film ends at the first one.
	sanityProbePoints = 3
	// sanityFloorDB is the loudness above which a window counts as sound.
	// Digital silence reads -91 dB (or -inf); room tone on the quietest real
	// mixes measured here sits above -70.
	sanityFloorDB = -80.0
	// sanityMaxTracks bounds the work on a file carrying many dubs.
	sanityMaxTracks = 8
)

// AudioSanity counts the audible audio tracks of a file. Needs ffprobe (track
// count and runtime) and ffmpeg (the decode).
type AudioSanity struct {
	ffmpegPath  string
	ffprobePath string
}

// NewAudioSanity builds the prober from the two tool paths.
func NewAudioSanity(ffmpegPath, ffprobePath string) *AudioSanity {
	return &AudioSanity{ffmpegPath: ffmpegPath, ffprobePath: ffprobePath}
}

// ProbeAudibleAudioTracks implements maintenance.AudibleTracksProber: how many
// of the file's audio tracks carry sound. An error means the file could not be
// measured at all - the caller must read that as "no evidence", never as a
// loss.
func (p *AudioSanity) ProbeAudibleAudioTracks(ctx context.Context, path string) (int, error) {
	n, err := audioStreamCount(ctx, p.ffprobePath, path)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, nil
	}
	n = min(n, sanityMaxTracks)

	dur, err := mediaDurationSec(ctx, p.ffprobePath, path)
	if err != nil {
		return 0, err
	}

	audible := 0
	for track := range n {
		ok, terr := p.trackHasSound(ctx, path, track, dur)
		if terr != nil {
			return 0, fmt.Errorf("audio sanity: track %d: %w", track, terr)
		}
		if ok {
			audible++
		}
	}
	return audible, nil
}

// trackHasSound decodes up to sanityProbePoints windows of one track and stops
// at the first that carries signal.
func (p *AudioSanity) trackHasSound(ctx context.Context, path string, track int, durSec float64) (bool, error) {
	for _, at := range probeOffsets(durSec) {
		env, err := decodeEnvelope(ctx, p.ffmpegPath, path, track, at, sanityWindowSec)
		if err != nil {
			return false, err
		}
		for _, db := range env {
			if db > sanityFloorDB {
				return true, nil
			}
		}
	}
	return false, nil
}

// probeOffsets spreads the sample points over the runtime, skipping the head
// (logos and silent idents) and the tail (credits can be silent on a dub).
// A runtime we could not read falls back to a single early window - better a
// narrow look than none.
func probeOffsets(durSec float64) []float64 {
	if durSec <= 0 {
		return []float64{0}
	}
	if durSec <= 3*sanityWindowSec {
		return []float64{0}
	}
	out := make([]float64, 0, sanityProbePoints)
	for i := 1; i <= sanityProbePoints; i++ {
		at := durSec * float64(i) / float64(sanityProbePoints+1)
		out = append(out, math.Max(0, at))
	}
	return out
}

// mediaDurationSec reads the container runtime (header only).
func mediaDurationSec(ctx context.Context, ffprobePath, path string) (float64, error) {
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "audio_sanity.duration"}, ffprobePath,
		"-v", "error", "-show_entries", "format=duration",
		"-of", "default=nw=1:nk=1", path)
	if err != nil {
		return 0, fmt.Errorf("audio sanity: duration: %w", err)
	}
	v, convErr := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if convErr != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		// An unreadable runtime is not a failure to measure the SOUND: it only
		// costs the guard its spread, and probeOffsets falls back to one early
		// window. Refusing here would disable the guard on the very files
		// whose headers are odd - the ones most worth guarding.
		return 0, nil //nolint:nilerr // deliberate: unknown runtime, not an error
	}
	return v, nil
}
