package ffmpeg

// Audio start-delay REPAIR: the file-level counterpart of the playback-time
// cancellation in appendAudioDelayInput. A repack whose audio content starts
// N ms after the video is remuxed with the audio shifted back onto the video
// (-itsoffset on a second input, streams copied bit-for-bit), then re-probed
// to PROVE the delay is gone before an atomic swap keeps the original as
// .mkvgobak. Any failed check leaves the original byte-for-byte intact -
// the same contract as the mkvgo reindexer.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	domainmkv "github.com/evey-project/evey/internal/domain/mkv"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// remuxDelayFloorSec mirrors maintenance.AudioDelayFlagSec: below it there is
// nothing to repair (codec priming territory).
const remuxDelayFloorSec = 0.1

// remuxTrackSpreadMaxSec bounds how much the audio tracks' delays may disagree
// before the uniform shift is refused: shifting every track by the max would
// pull an aligned track EARLY. Mixed-delay files are a different (rarer)
// defect needing per-track offsets - out of this repair's scope.
const remuxTrackSpreadMaxSec = 0.1

// remuxHeadroomBytes mirrors the reindex rewrite's space-preflight margin.
const remuxHeadroomBytes = 256 << 20

// remuxDurationToleranceSec allows container rounding between the source and
// the remux; a real content loss is orders of magnitude bigger.
const remuxDurationToleranceSec = 1.0

// remuxBackupSuffix matches the reindexer's, so the existing backup lifecycle
// (scan badge, rollback, purge) covers remux backups with zero extra wiring.
const remuxBackupSuffix = ".mkvgobak"

// AudioRemuxer implements the maintenance.AudioDelayRemuxer port with the
// bundled ffmpeg/ffprobe.
type AudioRemuxer struct {
	ffmpeg  string
	ffprobe string
}

func NewAudioRemuxer(ffmpegBin, ffprobeBin string) *AudioRemuxer {
	return &AudioRemuxer{ffmpeg: ffmpegBin, ffprobe: ffprobeBin}
}

// remuxProbe is one ffprobe snapshot of the facts the repair pivots on.
type remuxProbe struct {
	maxDelay    float64 // max audio start delay vs video (sec)
	minDelay    float64 // min across audio tracks (uniformity guard)
	audioTracks int
	streams     int // all streams (order-independent count check)
	durationSec float64
}

func (r *AudioRemuxer) probe(ctx context.Context, path string) (remuxProbe, error) {
	ctx, cancel := context.WithTimeout(ctx, audioDelayProbeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, r.ffprobe,
		"-v", "error",
		"-show_entries", "stream=codec_type,start_time:stream_disposition=attached_pic:format=duration",
		"-of", "json",
		path,
	).Output()
	if err != nil {
		return remuxProbe{}, fmt.Errorf("audio-remux probe: %w", err)
	}
	var parsed struct {
		Streams []audioDelayProbeStream `json:"streams"`
		Format  struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return remuxProbe{}, fmt.Errorf("audio-remux probe parse: %w", err)
	}
	p := remuxProbe{streams: len(parsed.Streams)}
	p.durationSec, _ = parseProbeSeconds(parsed.Format.Duration)
	videoStart, videoOK := 0.0, false
	for _, s := range parsed.Streams {
		if s.CodecType == "video" && !videoOK && s.Disposition.AttachedPic == 0 {
			videoStart, videoOK = parseProbeSeconds(s.StartTime)
		}
	}
	first := true
	for _, s := range parsed.Streams {
		if s.CodecType != "audio" {
			continue
		}
		p.audioTracks++
		if !videoOK {
			continue
		}
		if start, ok := parseProbeSeconds(s.StartTime); ok {
			d := start - videoStart
			if first || d > p.maxDelay {
				p.maxDelay = d
			}
			if first || d < p.minDelay {
				p.minDelay = d
			}
			first = false
		}
	}
	return p, nil
}

// buildRemuxArgs assembles the repair command: input 0 carries everything but
// audio, input 1 is the same file with its timestamps shifted EARLIER by
// delay, contributing the audio streams. Stream copy throughout - no frame is
// re-encoded. The defective audio's first packet sits at +delay, so the shift
// lands it at ~0: no negative timestamps are produced.
func buildRemuxArgs(src, dst string, delaySec float64) []string {
	return []string{
		"-hide_banner", "-v", "error", "-y",
		"-i", src,
		"-itsoffset", fmt.Sprintf("-%.6f", delaySec),
		"-i", src,
		"-map", "0", "-map", "-0:a", "-map", "1:a",
		"-c", "copy",
		"-map_chapters", "0",
		// The temp carries a .remux.tmp suffix, so the muxer cannot be
		// inferred from it. WebM is a Matroska subset - one muxer covers both.
		"-f", "matroska",
		dst,
	}
}

// RemuxAudioDelay implements the maintenance.AudioDelayRemuxer port. See the
// package comment for the ordering; every check failure removes the temp and
// leaves the original untouched.
func (r *AudioRemuxer) RemuxAudioDelay(ctx context.Context, path string) (string, bool, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.RemuxAudioDelay")
	defer span.End()

	src, err := r.probe(ctx, path)
	if err != nil {
		return "", false, err
	}
	if src.maxDelay <= remuxDelayFloorSec {
		return "", true, nil // aligned (or fixed since the scan) - nothing to do
	}
	if src.audioTracks > 1 && src.maxDelay-src.minDelay > remuxTrackSpreadMaxSec {
		return "", false, fmt.Errorf(
			"audio-remux: the %d audio tracks disagree on the delay (%.2fs..%.2fs) - a uniform shift would misalign some of them; this file needs per-track repair",
			src.audioTracks, src.minDelay, src.maxDelay)
	}

	srcInfo, err := os.Stat(path)
	if err != nil {
		return "", false, fmt.Errorf("audio-remux: stat source: %w", err)
	}
	// Space preflight, same advisory contract as the reindex rewrite.
	var st syscall.Statfs_t
	if serr := syscall.Statfs(filepath.Dir(path), &st); serr == nil {
		if avail := st.Bavail * uint64(st.Bsize); avail < uint64(srcInfo.Size())+remuxHeadroomBytes { //nolint:gosec // sizes are positive
			return "", false, fmt.Errorf(
				"audio-remux: %w: the remux needs ~%d MiB free on this volume, %d MiB available",
				domainmkv.ErrInsufficientSpace, (srcInfo.Size()+remuxHeadroomBytes)>>20, int64(avail)>>20) //nolint:gosec // display only
		}
	}

	tmp := path + ".remux.tmp"
	_ = os.Remove(tmp) //nolint:errcheck // clear any stale temp from an aborted run
	cmd := exec.CommandContext(ctx, r.ffmpeg, buildRemuxArgs(path, tmp, src.maxDelay)...)
	if out, runErr := cmd.CombinedOutput(); runErr != nil {
		_ = os.Remove(tmp) //nolint:errcheck // best-effort cleanup
		if errors.Is(runErr, fs.ErrPermission) || errors.Is(ctx.Err(), context.Canceled) {
			return "", false, fmt.Errorf("audio-remux: %w: %w", domainmkv.ErrMediaNotWritable, runErr)
		}
		return "", false, fmt.Errorf("audio-remux: ffmpeg: %w: %s", runErr, firstLine(out))
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmp) //nolint:errcheck // best-effort cleanup
		}
	}()

	// Re-probe the temp: the repair must be PROVEN before it replaces anything.
	got, err := r.probe(ctx, tmp)
	if err != nil {
		return "", false, fmt.Errorf("audio-remux: verify: %w", err)
	}
	switch {
	case got.streams != src.streams:
		return "", false, fmt.Errorf("audio-remux: stream count changed (%d -> %d), refusing swap", src.streams, got.streams)
	case got.maxDelay > remuxDelayFloorSec:
		return "", false, fmt.Errorf("audio-remux: delay still %.2fs after the remux, refusing swap", got.maxDelay)
	case got.minDelay < -remuxDelayFloorSec:
		return "", false, fmt.Errorf("audio-remux: audio overshot %.2fs EARLY, refusing swap", -got.minDelay)
	case src.durationSec > 0 && got.durationSec > 0 &&
		(got.durationSec-src.durationSec > remuxDurationToleranceSec || src.durationSec-got.durationSec > remuxDurationToleranceSec):
		return "", false, fmt.Errorf("audio-remux: duration changed (%.2fs -> %.2fs), refusing swap", src.durationSec, got.durationSec)
	}

	bak := path + remuxBackupSuffix
	if err := os.Rename(path, bak); err != nil {
		return "", false, fmt.Errorf("audio-remux: back up original: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		if rerr := os.Rename(bak, path); rerr != nil {
			return "", false, fmt.Errorf("audio-remux: swap failed AND restore failed, original preserved at %q: swap=%w restore=%w", bak, err, rerr)
		}
		return "", false, fmt.Errorf("audio-remux: swap failed, original restored: %w", err)
	}
	committed = true
	return bak, false, nil
}

// firstLine trims ffmpeg's stderr to its first line for error messages.
func firstLine(out []byte) string {
	for i, b := range out {
		if b == '\n' {
			return string(out[:i])
		}
	}
	return string(out)
}
