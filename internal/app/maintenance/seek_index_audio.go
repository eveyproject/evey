package maintenance

import (
	"context"
	"errors"
)

// seek_index_audio.go holds the audio start-delay repair: the validated remux
// per-file step and its library/media entry points.

// ErrRemuxUnavailable is returned by the remux entry points when no remuxer
// is wired (e.g. a build without ffmpeg).
var ErrRemuxUnavailable = errors.New("audio remux is not available in this build")

// remuxAudioFile is the shared per-file step of both remux entry points.
func (s *SeekIndexService) remuxAudioFile(bg context.Context, path string) (bool, error) {
	return s.remuxAudioStep(bg, path, nil)
}

// remuxAudioStep repairs one file's audio delay and folds the result into the
// cached scan, advancing the rolling backup window when one is configured -
// so a combined "repair all" that is NOT keeping backups drains the audio
// backups too (they used to persist regardless of the operator's choice).
func (s *SeekIndexService) remuxAudioStep(bg context.Context, path string, rb *rollingBackups) (bool, error) {
	bak, skipped, err := s.audioRemux.RemuxAudioDelay(bg, path)
	if err != nil {
		return false, err
	}
	if skipped {
		return true, nil
	}
	// The video stream and its keyframe TIMES are copied untouched; refresh
	// the fingerprint so the next scan keeps the stream records instead of
	// re-probing (same rationale as the reindex path).
	s.refreshFileState(bg, path)
	hasBak := bak != ""
	s.updateCachedFile(path, func(st *SeekStatus) { st.AudioDelaySec = 0; st.HasBackup = hasBak })
	if hasBak && rb != nil && rb.max > 0 {
		rb.window = append(rb.window, path)
		if len(rb.window) > rb.max {
			s.purgeRolledBackup(bg, rb.window[0])
			rb.window = rb.window[1:]
		}
	}
	return false, nil
}

// StartRemuxAudioLibrary kicks a background job that repairs every file the
// scan flagged with an audio start delay above the defect floor.
func (s *SeekIndexService) StartRemuxAudioLibrary(_ context.Context, libraryID string) error {
	if s.audioRemux == nil {
		return ErrRemuxUnavailable
	}
	return s.startBatch(JobRemuxAudio, libraryID, nil,
		func(it SeekStatus) bool { return it.AudioDelaySec > AudioDelayFlagSec },
		s.remuxAudioFile, nil)
}

// StartRemuxAudioMedia repairs one media file's audio start delay in the
// background. The media lookup happens synchronously so unknown IDs fail fast.
func (s *SeekIndexService) StartRemuxAudioMedia(ctx context.Context, mediaID string) error {
	if s.audioRemux == nil {
		return ErrRemuxUnavailable
	}
	return s.startSingle(ctx, JobRemuxAudio, mediaID, s.remuxAudioFile)
}
