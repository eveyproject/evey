package ffmpeg

// Do a file's own audio tracks tell the same story at the same instant?
//
// Two tracks of one film share their music and their effects bed. Read in the
// CONTAINER's timeline - so with each track's declared start offset already
// applied - their loudness envelopes therefore ride on top of each other,
// whatever those offsets happen to be. That is the whole idea: it lets a
// deliberate alignment (a dub placed 12 s in so it matches the picture) be told
// apart from a track that has been moved to the wrong place, WITHOUT needing a
// reference copy of the film or any knowledge of what the picture shows.
//
// It is the measurement that was missing when a "repair" zeroed a 12.000 s dub
// alignment on a real title and desynchronised it in every player (prod
// 2026-07-13, a prod title).
//
// The comparison is deliberately cheap and deliberately timid:
//   - one window in the middle of the film, not the whole run;
//   - a loudness envelope, not samples - two dubs never match sample for sample;
//   - and a confidence returned alongside the lag, because two mixes can share
//     no legible signal at all and a peak read off noise must never accuse a
//     file. The caller drops anything below its confidence floor.

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/evey-project/evey/internal/pkg/procfork"
)

const (
	// coherenceWindowSec is how much film is compared. Long enough to carry
	// several scene changes (the features the correlation actually locks on),
	// short enough to stay a few seconds of decoding.
	coherenceWindowSec = 240
	// coherenceStartSec skips the opening: logos, silent studio idents and
	// unscored credits carry no common signal, and on a badly aligned file they
	// are exactly where the two tracks differ most for innocent reasons.
	coherenceStartSec = 900
	// coherenceMaxLagSec bounds the search. Deliberate dub alignments observed
	// in the wild sit between 3 and 12 s; 30 covers them with room to spare
	// while keeping the search cheap. A displacement beyond it is reported as
	// "could not conclude", never as the best peak of the wrong window.
	coherenceMaxLagSec = 30
	// coherenceEnvelopeStep is the resolution asked of astats. ffmpeg emits one
	// value per decoded frame regardless, so this only bounds its accounting.
	coherenceEnvelopeStep = 0.2
	// coherenceSilenceDB stands in for -inf, which astats prints for a silent
	// window and which no arithmetic survives.
	coherenceSilenceDB = -91.0
	// coherenceMinSamples refuses a verdict drawn from too little signal.
	coherenceMinSamples = 400
	// coherenceMaxTracks bounds the work on a file carrying many dubs: the
	// first three are enough to prove agreement or catch a stray one.
	coherenceMaxTracks = 3
)

// AudioCoherence measures whether a file's own audio tracks agree. It needs
// both binaries: ffprobe to count the tracks, ffmpeg to decode the envelopes.
type AudioCoherence struct {
	ffmpegPath  string
	ffprobePath string
}

// NewAudioCoherence builds the prober from the two tool paths.
func NewAudioCoherence(ffmpegPath, ffprobePath string) *AudioCoherence {
	return &AudioCoherence{ffmpegPath: ffmpegPath, ffprobePath: ffprobePath}
}

// ProbeAudioTrackLagSec implements maintenance.AudioCoherenceProber. It returns
// the largest disagreement found between any two of the file's audio tracks and
// the correlation strength behind it. A single-track file returns (0, 0): there
// is nothing to compare, which the caller must not read as "proven coherent".
func (p *AudioCoherence) ProbeAudioTrackLagSec(ctx context.Context, path string) (float64, float64, error) {
	n, err := p.audioTrackCount(ctx, path)
	if err != nil {
		return 0, 0, err
	}
	if n < 2 {
		return 0, 0, nil
	}
	if n > coherenceMaxTracks {
		n = coherenceMaxTracks
	}

	envelopes := make([][]float64, 0, n)
	for i := range n {
		env, envErr := p.audioEnvelope(ctx, path, i)
		if envErr != nil {
			return 0, 0, fmt.Errorf("audio coherence: track %d: %w", i, envErr)
		}
		envelopes = append(envelopes, env)
	}

	var worstLag, worstConfidence float64
	for i := range envelopes {
		for j := i + 1; j < len(envelopes); j++ {
			lag, confidence, ok := correlateEnvelopes(envelopes[i], envelopes[j])
			if !ok {
				continue
			}
			// The pair that disagrees the most decides the file - but only
			// among pairs the caller would actually believe. A confident
			// agreement must never be overruled by a noisy disagreement.
			if math.Abs(lag) > math.Abs(worstLag) || worstConfidence == 0 {
				worstLag, worstConfidence = lag, confidence
			}
		}
	}
	return worstLag, worstConfidence, nil
}

// audioTrackCount counts the file's audio streams (header read).
func (p *AudioCoherence) audioTrackCount(ctx context.Context, path string) (int, error) {
	return audioStreamCount(ctx, p.ffprobePath, path)
}

// audioStreamCount counts a file's audio streams (header read). Shared with
// the audibility guard, which asks the same question of the same tools.
func audioStreamCount(ctx context.Context, ffprobePath, path string) (int, error) {
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "audio_coherence.stream_count"}, ffprobePath,
		"-v", "error", "-select_streams", "a",
		"-show_entries", "stream=index", "-of", "csv=p=0", path)
	if err != nil {
		return 0, fmt.Errorf("count audio tracks: %w", err)
	}
	n := 0
	for line := range strings.SplitSeq(string(out), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n, nil
}

// audioEnvelope decodes one window of one track and returns its loudness
// envelope in dB. `-ss` seeks in the CONTAINER timeline, so a track's declared
// start offset is applied for us - which is exactly what makes the comparison
// meaningful.
func (p *AudioCoherence) audioEnvelope(ctx context.Context, path string, track int) ([]float64, error) {
	return decodeEnvelope(ctx, p.ffmpegPath, path, track, coherenceStartSec, coherenceWindowSec)
}

// decodeEnvelope decodes windowSec seconds of one audio track from atSec and
// returns its loudness envelope in dB, one value per astats window. Silence
// (-inf) is flattened to coherenceSilenceDB, which arithmetic survives.
func decodeEnvelope(ctx context.Context, ffmpegPath, path string, track int, atSec float64, windowSec int) ([]float64, error) {
	filter := fmt.Sprintf(
		"aformat=channel_layouts=mono,astats=metadata=1:reset=1:length=%v,"+
			"ametadata=print:key=lavfi.astats.Overall.RMS_level:file=-",
		coherenceEnvelopeStep)
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Scan, Site: "audio_coherence.envelope"}, ffmpegPath,
		"-v", "error", "-nostdin",
		// Input-side discards: the picture and subtitles are never read, let
		// alone decoded. Measured at ~2 s per file against ~10 s without, and
		// the difference is what makes running this on every multi-track
		// file affordable.
		"-vn", "-sn", "-dn",
		"-ss", strconv.FormatFloat(atSec, 'f', 3, 64), "-t", strconv.Itoa(windowSec),
		"-i", path, "-map", fmt.Sprintf("0:a:%d", track),
		"-af", filter, "-f", "null", "-")
	if err != nil {
		return nil, fmt.Errorf("envelope: %w", err)
	}

	var env []float64
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	const key = "lavfi.astats.Overall.RMS_level="
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, key) {
			continue
		}
		v, convErr := strconv.ParseFloat(strings.TrimSpace(line[len(key):]), 64)
		if convErr != nil || math.IsInf(v, 0) || math.IsNaN(v) {
			v = coherenceSilenceDB
		}
		env = append(env, v)
	}
	return env, sc.Err()
}

// correlateEnvelopes returns the lag (seconds, b relative to a) of the best
// alignment between two envelopes and the Pearson correlation there, 0..1.
// ok is false when there is too little signal to say anything.
func correlateEnvelopes(a, b []float64) (lag, confidence float64, ok bool) {
	n := min(len(a), len(b))
	if n < coherenceMinSamples {
		return 0, 0, false
	}
	a, b = a[:n], b[:n]
	step := float64(coherenceWindowSec) / float64(n)

	ca, na := centre(a)
	cb, nb := centre(b)
	if na == 0 || nb == 0 {
		return 0, 0, false // a flat track (pure silence) correlates with nothing
	}

	span := int(coherenceMaxLagSec / step)
	if span >= n/2 {
		span = n/2 - 1
	}
	best, bestLag := -2.0, 0
	for shift := -span; shift <= span; shift++ {
		lo, hi := max(0, -shift), min(n, n-shift)
		if hi-lo < n/2 {
			continue
		}
		sum := 0.0
		// Every third sample: the envelope is smooth at this resolution and
		// the decimation turns a quadratic search into something a scan can
		// afford, without moving the peak.
		count := 0
		for i := lo; i < hi; i += 3 {
			sum += ca[i] * cb[i+shift]
			count++
		}
		if count == 0 {
			continue
		}
		r := sum * 3 / (na * nb)
		if r > best {
			best, bestLag = r, shift
		}
	}
	if best <= 0 {
		return 0, 0, false
	}
	// A peak sitting on the edge of the search is not a measurement: the true
	// alignment may lie anywhere beyond it, and the best of a wrong window is
	// still wrong. Say "could not conclude" rather than hand back a lag that
	// happens to be the largest one tried.
	if bestLag <= -span || bestLag >= span {
		return 0, 0, false
	}
	return float64(bestLag) * step, math.Min(best, 1), true
}

// centre returns the mean-removed series and its Euclidean norm.
func centre(v []float64) ([]float64, float64) {
	mean := 0.0
	for _, x := range v {
		mean += x
	}
	mean /= float64(len(v))
	out := make([]float64, len(v))
	norm := 0.0
	for i, x := range v {
		out[i] = x - mean
		norm += out[i] * out[i]
	}
	return out, math.Sqrt(norm)
}
