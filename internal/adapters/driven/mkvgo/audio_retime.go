package mkvgo

// Audio start-delay repair via mkvgo RetimeTracks: the per-track successor to
// the ffmpeg uniform-shift remux. It shifts each audio track's block timecodes
// back onto the video by that track's OWN delay, so a file whose tracks
// disagree on the delay - the dominant real-world case the uniform ffmpeg
// remux refused - is repaired instead of skipped. The sequential rewrite
// engine is forced: it rebuilds a healthy (video-only) seek index in the same
// pass and always keeps a backup, matching the reindex lifecycle.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"time"

	extmatroska "github.com/gravity-zero/mkvgo/matroska"
	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/reader"

	domainmkv "github.com/evey-project/evey/internal/domain/mkv"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// retimeDelayFloorSec mirrors maintenance.AudioDelayFlagSec: a track below it
// is aligned (codec priming territory) and is left untouched.
const retimeDelayFloorSec = 0.1

// retimeProbeTimeout bounds the ffprobe call (the library can sit on a network
// mount).
const retimeProbeTimeout = 15 * time.Second

// retimeBackupSuffix is Evey's backup convention (HasBackup/RestoreBackup/
// DeleteBackup all key on it). mkvgo's KeepBackup writes "<path>.bak", so the
// adapter renames it to this after a successful retime to join the existing
// rollback/purge/scan-badge lifecycle unchanged.
const retimeBackupSuffix = ".mkvgobak"

// mkvgoReplaceBackupSuffix is what mkvgo's KeepBackup actually produces.
const mkvgoReplaceBackupSuffix = ".bak"

// AudioBackupFallback repairs the audio delay of files RetimeTracks refuses
// (unknown-size/streamed segments and other layouts its in-place+replace
// engines cannot handle). Satisfied by the ffmpeg remux adapter; optional.
type AudioBackupFallback interface {
	RemuxAudioDelay(ctx context.Context, path string) (backupPath string, skipped bool, err error)
}

// RetimeFunc is the mkvgo entry point the retimer drives (injectable for
// tests). Production passes matroska.RetimeTracksReplace.
type RetimeFunc func(ctx context.Context, path string, shift map[uint64]int64, opts ...extmatroska.Options) error

// AudioRetimer implements the maintenance.AudioDelayRemuxer port with mkvgo's
// per-track RetimeTracks, falling back to an ffmpeg remux for files it refuses.
type AudioRetimer struct {
	ffprobe  string
	retime   RetimeFunc
	fallback AudioBackupFallback
}

// NewAudioRetimer builds the retimer. ffprobe measures per-track start delays;
// fallback (optional) handles files mkvgo refuses.
func NewAudioRetimer(ffprobeBin string, fallback AudioBackupFallback) *AudioRetimer {
	return &AudioRetimer{
		ffprobe:  ffprobeBin,
		retime:   extmatroska.RetimeTracksReplace,
		fallback: fallback,
	}
}

// probeStream is the minimal ffprobe JSON the delay probe reads.
type probeStream struct {
	CodecType   string `json:"codec_type"`
	StartTime   string `json:"start_time"`
	Disposition struct {
		AttachedPic int `json:"attached_pic"`
	} `json:"disposition"`
}

// audioDelaysInOrder returns each audio stream's start delay vs the first real
// video stream, in the file's stream (= Matroska Tracks) order - the order the
// track numbers are zipped against.
func (r *AudioRetimer) audioDelaysInOrder(ctx context.Context, path string) ([]float64, error) {
	ctx, cancel := context.WithTimeout(ctx, retimeProbeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, r.ffprobe, //nolint:gosec // fixed ffprobe binary; path is a library file, no user-controlled args
		"-v", "error",
		"-show_entries", "stream=codec_type,start_time:stream_disposition=attached_pic",
		"-of", "json",
		path,
	).Output()
	if err != nil {
		return nil, fmt.Errorf("audio-retime probe: %w", err)
	}
	var parsed struct {
		Streams []probeStream `json:"streams"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("audio-retime probe parse: %w", err)
	}
	videoStart, videoOK := 0.0, false
	for _, s := range parsed.Streams {
		if s.CodecType == "video" && !videoOK && s.Disposition.AttachedPic == 0 {
			videoStart, videoOK = parseSeconds(s.StartTime)
		}
	}
	if !videoOK {
		return nil, nil // no video reference: nothing to align against
	}
	var delays []float64
	for _, s := range parsed.Streams {
		if s.CodecType != "audio" {
			continue
		}
		start, ok := parseSeconds(s.StartTime)
		if !ok {
			delays = append(delays, 0) // unknown start: treat as aligned
			continue
		}
		delays = append(delays, start-videoStart)
	}
	return delays, nil
}

// audioTrackNumbers reads the Matroska track numbers of the audio tracks, in
// Tracks order, head-only. Zipped with audioDelaysInOrder by position.
func audioTrackNumbers(ctx context.Context, path string) ([]uint64, error) {
	c, err := reader.OpenMeta(ctx, path, reader.WithCues())
	if err != nil {
		return nil, fmt.Errorf("audio-retime read tracks: %w", err)
	}
	var nums []uint64
	for _, t := range c.Tracks {
		if t.Type == extmkv.AudioTrack {
			nums = append(nums, t.ID)
		}
	}
	return nums, nil
}

// RemuxAudioDelay implements the maintenance.AudioDelayRemuxer port. It builds
// a per-track shift map (each late track shifted back by its own delay) and
// drives RetimeTracksReplace; on a refusal it defers to the ffmpeg fallback.
// The returned backup path is Evey's ".mkvgobak" convention.
func (r *AudioRetimer) RemuxAudioDelay(ctx context.Context, path string) (string, bool, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.RemuxAudioDelay")
	defer span.End()

	delays, err := r.audioDelaysInOrder(ctx, path)
	if err != nil {
		return "", false, err
	}
	nums, err := audioTrackNumbers(ctx, path)
	if err != nil {
		return "", false, err
	}
	if len(delays) != len(nums) {
		// ffprobe and the Matroska Tracks disagree on the audio count: cannot
		// map delays to track numbers safely. Defer to the fallback (which
		// works off ffprobe alone) rather than risk shifting the wrong track.
		if r.fallback != nil {
			return r.fallback.RemuxAudioDelay(ctx, path)
		}
		return "", false, fmt.Errorf("audio-retime: %d audio streams but %d audio tracks - cannot map delays", len(delays), len(nums))
	}

	shift := map[uint64]int64{}
	for i, d := range delays {
		if d <= retimeDelayFloorSec {
			continue // aligned, or audio-before-video (untested) - leave it
		}
		// Shift the track EARLIER by its own delay (negative = earlier), in ns.
		shift[nums[i]] = -int64(math.Round(d * 1e9))
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

// parseSeconds parses ffprobe's start_time ("0.900000", "N/A", "").
func parseSeconds(s string) (float64, bool) {
	if s == "" || s == "N/A" {
		return 0, false
	}
	var v float64
	if _, err := fmt.Sscanf(s, "%g", &v); err != nil {
		return 0, false
	}
	return v, true
}
