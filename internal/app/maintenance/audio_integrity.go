package maintenance

// The whole-file account of a file's sound, before and after a rewrite that
// touches it.
//
// The audibility guard (AudibleTracksProber) asks a cheap question - does each
// track still carry SOME sound - by decoding three short windows. That is the
// right price for an index rebuild, which copies the audio untouched. It is
// not enough for a rewrite that moves or drops audio bytes, and the evening of
// 2026-09-01 showed the two ways it fails:
//
//   - a track whose frames change format at a joint (448 kbps laid in front of
//     640 kbps) decodes without a single error, reads "AUDIBLE" in every
//     window, and is SILENT in every player, which configures itself on the
//     first frames and gives up at the switch;
//   - a hole of a few minutes falls between three eight-second windows, which
//     together cover under a tenth of a film.
//
// So for those rewrites the question is asked of the whole track: where is it
// silent, for how long, and are its frames all the same shape. The rewrite is
// judged by COMPARISON with the same measurement taken before it - a film is
// full of legitimately silent minutes, and the guard must never accuse a
// track for what the source already had.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/evey-project/evey/internal/domain/mkv"
)

// AudioIntegrityAudibleSlackSec is how much audible time a track may lose
// before the loss is called real: final-cluster rounding and priming can move
// the last window by a second or two; a hole is far larger.
const AudioIntegrityAudibleSlackSec = 3.0

// AudioIntegrityProber measures every audio track of a file end to end.
// Satisfied by the ffmpeg adapter; optional (nil disables the guard on the
// rewrites that would use it - they then run with the sampled guard alone).
type AudioIntegrityProber interface {
	ProbeAudioIntegrity(ctx context.Context, path string) ([]mkv.TrackIntegrity, error)
}

// ErrAudioIntegrityLost is the refusal of a rewrite that left a track worse
// than it found it: a new hole, a frame-format change, or lost audible time.
var ErrAudioIntegrityLost = errors.New("the rewrite damaged an audio track")

// WithAudioIntegrityProber wires the exhaustive guard. Optional and chainable.
func (s *SeekIndexService) WithAudioIntegrityProber(p AudioIntegrityProber) *SeekIndexService {
	s.integrity = p
	return s
}

// audioIntegrityLost compares a track set measured before a rewrite with the
// same set measured after it, and names the first loss it finds. Nothing is
// inferred from the AFTER measurement alone: a track that was silent for ten
// minutes before is allowed to be silent for ten minutes after.
func audioIntegrityLost(before, after []mkv.TrackIntegrity) error {
	post := make(map[int]mkv.TrackIntegrity, len(after))
	for _, t := range after {
		post[t.Track] = t
	}
	for _, b := range before {
		a, ok := post[b.Track]
		if !ok {
			return fmt.Errorf("%w: audio track %d is gone", ErrAudioIntegrityLost, b.Track)
		}
		if a.SilentStretches > b.SilentStretches {
			return fmt.Errorf("%w: track %d has %d silent stretches of %.0f s or more, it had %d",
				ErrAudioIntegrityLost, b.Track, a.SilentStretches, mkv.AudioIntegritySilenceSec, b.SilentStretches)
		}
		if b.FrameSizes == 1 && a.FrameSizes > 1 {
			return fmt.Errorf("%w: track %d now carries %d frame sizes where it had one - a player configures itself on the first frames and goes silent at the switch",
				ErrAudioIntegrityLost, b.Track, a.FrameSizes)
		}
		if b.AudibleSec-a.AudibleSec > AudioIntegrityAudibleSlackSec {
			return fmt.Errorf("%w: track %d lost %.1f s of audible sound (%.1f s before, %.1f s after)",
				ErrAudioIntegrityLost, b.Track, b.AudibleSec-a.AudibleSec, b.AudibleSec, a.AudibleSec)
		}
	}
	return nil
}

// audioIntegrityBaseline measures the file before a rewrite. A prober that is
// not wired, or one that fails, yields no baseline: the rewrite then runs with
// the sampled guard alone rather than being blocked by a broken ffmpeg - and
// says so in the log, because a guard that silently stood down is how damage
// goes unnoticed.
func (s *SeekIndexService) audioIntegrityBaseline(ctx context.Context, path string) ([]mkv.TrackIntegrity, bool) {
	if s.integrity == nil {
		return nil, false
	}
	tracks, err := s.integrity.ProbeAudioIntegrity(ctx, path)
	if err != nil {
		s.logger.Warn("seek-index: audio integrity unmeasurable, rewriting with the sampled guard only",
			slog.String("path", path), slog.Any("err", err))
		return nil, false
	}
	return tracks, true
}

// guardAudioIntegrity re-measures a just-rewritten file against its baseline
// and, on a loss, calls restore to put the original back before reporting the
// failure. A second measurement that could not run is not evidence, and the
// rewrite stands. restore may be nil when nothing can be restored, in which
// case the loss is reported as such.
func (s *SeekIndexService) guardAudioIntegrity(ctx context.Context, path string, before []mkv.TrackIntegrity, restore func() error) error {
	after, err := s.integrity.ProbeAudioIntegrity(ctx, path)
	if err != nil {
		s.logger.Warn("seek-index: audio integrity could not be re-measured after the rewrite",
			slog.String("path", path), slog.Any("err", err))
		return nil
	}
	lost := audioIntegrityLost(before, after)
	if lost == nil {
		return nil
	}
	s.logger.Error("seek-index: the rewrite damaged the sound, putting the original back",
		slog.String("path", path), slog.Any("err", lost))
	if restore == nil {
		return fmt.Errorf("%w and nothing can restore the original", lost)
	}
	if rerr := restore(); rerr != nil {
		return fmt.Errorf("%w; restoring the original ALSO failed: %w", lost, rerr)
	}
	return fmt.Errorf("%w - the original has been restored", lost)
}
