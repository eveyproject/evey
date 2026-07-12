package mkvgo

// Audio start-delay repair via mkvgo RetimeTracks: the per-track successor to
// the ffmpeg uniform-shift remux. It shifts each audio track's block timecodes
// back onto the video by that track's OWN delay, so a file whose tracks
// disagree on the delay - the dominant real-world case the uniform ffmpeg
// remux refused - is repaired instead of skipped. The sequential rewrite
// engine is forced: it rebuilds a healthy (video-only) seek index in the same
// pass and always keeps a backup, matching the reindex lifecycle.
//
// Per-track delays come from mkvgo's native ops.AudioStartDelays (head + first
// clusters, no ffprobe): the track number and its delay come from the same
// parse, so there is no fragile stream-order mapping.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	extmatroska "github.com/gravity-zero/mkvgo/matroska"

	domainmkv "github.com/evey-project/evey/internal/domain/mkv"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// retimeDelayFloorNs mirrors maintenance.AudioDelayFlagSec (0.1s): a track
// below it is aligned (codec priming territory) and is left untouched.
const retimeDelayFloorNs = 100_000_000

// retimeBackupSuffix is Evey's backup convention (HasBackup/RestoreBackup/
// DeleteBackup all key on it). mkvgo's KeepBackup writes "<path>.bak", so the
// adapter renames it to this after a successful retime to join the existing
// rollback/purge/scan-badge lifecycle unchanged.
const retimeBackupSuffix = ".mkvgobak"

// mkvgoReplaceBackupSuffix is what mkvgo's KeepBackup actually produces.
const mkvgoReplaceBackupSuffix = ".bak"

// AudioBackupFallback repairs the audio delay of files RetimeTracks refuses
// (unknown-size/streamed segments and other layouts its in-place+replace
// engines cannot handle) or a non-Matroska container. Satisfied by the ffmpeg
// remux adapter; optional.
type AudioBackupFallback interface {
	RemuxAudioDelay(ctx context.Context, path string) (backupPath string, skipped bool, err error)
}

// delaysFunc measures each audio track's start delay (Matroska track number ->
// nanoseconds, positive = late). Injectable for tests; production passes
// matroska.AudioStartDelays.
type delaysFunc func(ctx context.Context, path string) (map[uint64]int64, error)

// retimeFunc applies the per-track shift. Injectable for tests; production
// passes matroska.RetimeTracksReplace.
type retimeFunc func(ctx context.Context, path string, shift map[uint64]int64, opts ...extmatroska.Options) error

// AudioRetimer implements the maintenance.AudioDelayRemuxer port with mkvgo's
// per-track RetimeTracks, falling back to an ffmpeg remux for files it refuses.
type AudioRetimer struct {
	delays   delaysFunc
	retime   retimeFunc
	fallback AudioBackupFallback
}

// NewAudioRetimer builds the retimer. fallback (optional) handles files mkvgo
// refuses (unknown-size layouts) and non-Matroska containers.
func NewAudioRetimer(fallback AudioBackupFallback) *AudioRetimer {
	return &AudioRetimer{
		delays: func(ctx context.Context, path string) (map[uint64]int64, error) {
			return extmatroska.AudioStartDelays(ctx, path)
		},
		retime:   extmatroska.RetimeTracksReplace,
		fallback: fallback,
	}
}

// RemuxAudioDelay implements the maintenance.AudioDelayRemuxer port. It builds
// a per-track shift map (each late track shifted back by its own delay) and
// drives RetimeTracksReplace; on a refusal it defers to the ffmpeg fallback.
// The returned backup path is Evey's ".mkvgobak" convention.
func (r *AudioRetimer) RemuxAudioDelay(ctx context.Context, path string) (string, bool, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.RemuxAudioDelay")
	defer span.End()

	// RetimeTracks and AudioStartDelays are Matroska-only (EBML block
	// timecodes). A non-MKV container (MP4/MOV) defers to the ffmpeg fallback
	// - mkvgo has no MP4 audio-delay repair yet.
	if !IsEBMLContainer(path) {
		if r.fallback != nil {
			return r.fallback.RemuxAudioDelay(ctx, path)
		}
		return "", false, fmt.Errorf("audio-retime: %q is not a Matroska container and no fallback is configured", path)
	}

	delays, err := r.delays(ctx, path)
	if err != nil {
		// Reading the tracks/delays failed (corrupt header, mislabeled
		// container): the ffmpeg fallback works off ffprobe alone.
		if r.fallback != nil {
			return r.fallback.RemuxAudioDelay(ctx, path)
		}
		return "", false, fmt.Errorf("audio-retime: measure delays: %w", err)
	}

	shift := map[uint64]int64{}
	for track, ns := range delays {
		if ns <= retimeDelayFloorNs {
			continue // aligned, or audio-before-video (untested) - leave it
		}
		shift[track] = -ns // cancel the delay: shift the track EARLIER by its own
	}
	if len(shift) == 0 {
		return "", true, nil // no track above the defect floor - nothing to do
	}

	err = r.retime(ctx, path, shift, extmatroska.Options{KeepBackup: true})
	switch {
	case err == nil:
		// mkvgo kept "<path>.bak"; adopt Evey's ".mkvgobak" convention so the
		// existing rollback/purge/badge lifecycle sees it.
		bak := path + retimeBackupSuffix
		if rerr := os.Rename(path+mkvgoReplaceBackupSuffix, bak); rerr != nil {
			if !os.IsNotExist(rerr) {
				return "", false, fmt.Errorf("audio-retime: adopt backup name: %w", rerr)
			}
			bak = "" // no backup produced (should not happen with KeepBackup)
		}
		return bak, false, nil
	case errors.Is(err, fs.ErrPermission):
		return "", false, fmt.Errorf("audio-retime: %w: %w", domainmkv.ErrMediaNotWritable, err)
	case r.fallback != nil:
		// Layout mkvgo refuses (unknown-size/streamed segment, ...): the ffmpeg
		// remux can still handle a uniform delay. Its own guards apply.
		return r.fallback.RemuxAudioDelay(ctx, path)
	default:
		return "", false, fmt.Errorf("audio-retime: %w", err)
	}
}
