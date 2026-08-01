package ffmpeg

// compat_transcode.go - produce the phone-compatible download variant of a
// media file: video, subtitles, chapters and attachments copied verbatim,
// every audio track re-encoded to stereo AAC. This exists because plenty of
// phones ship no AC-3/E-AC-3/DTS decoder at all: the raw download plays
// silent there, and the on-device rewrite the mobile app embeds costs the
// user minutes of battery per film. The server pays the audio transcode ONCE
// per title (the handler caches the result by source fingerprint) and every
// device downloads a file that just plays.
//
// One ffmpeg invocation does the whole job - deliberately no mkvgo demux/mux
// choreography here: ffmpeg decodes every audio codec we will ever meet
// (including the DTS/TrueHD family the on-device pass must refuse), and the
// single-command shape is the one the audio-delay remuxer already field-
// proved for "copy everything, rewrite one aspect, atomic swap".

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	domainmkv "github.com/evey-project/evey/internal/domain/mkv"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// compatTimeout bounds one production. Audio-only transcode runs ~10-30x
// realtime even on the weak-NAS target, so a 4 h film clears in well under
// this; a job still running after two hours is stuck, not slow.
const compatTimeout = 2 * time.Hour

// compatHeadroomBytes keeps a safety margin beyond the expected output size
// in the space preflight, mirroring the audio-delay remuxer's contract.
const compatHeadroomBytes = 512 << 20

// CompatTranscoder produces compat-download files by shelling out to ffmpeg.
type CompatTranscoder struct {
	ffmpegPath string
}

func NewCompatTranscoder(ffmpegPath string) *CompatTranscoder {
	return &CompatTranscoder{ffmpegPath: ffmpegPath}
}

// BuildCompatArgs is the exact ffmpeg argv (exported for the unit test - the
// command line IS the behavior). Notes:
//   - "-map 0 -ignore_unknown": carry every stream, skip (rather than die on)
//     data tracks the Matroska muxer has no name for.
//   - "-c copy" then "-c:a aac": per-stream override wins, so only audio is
//     re-encoded. Stereo 192k matches the streaming ladder's audio ceiling.
//   - "-f matroska" because the temp path carries a .tmp suffix the muxer
//     cannot be inferred from (same reason as the audio-delay remuxer).
func BuildCompatArgs(src, dst string) []string {
	return []string{
		"-hide_banner", "-v", "error", "-nostdin", "-y",
		"-i", src,
		"-map", "0", "-ignore_unknown",
		"-map_metadata", "0",
		"-map_chapters", "0",
		"-c", "copy",
		"-c:a", "aac", "-ac", "2", "-b:a", "192k",
		"-f", "matroska",
		dst,
	}
}

// TranscodeCompat writes the compat variant of src at dst. The caller owns
// temp naming and the atomic publish; this only guarantees dst is complete
// when it returns nil. Runs under nice -n 19 like every other background
// ffmpeg job: a viewer's live transcode must always win the CPU.
func (t *CompatTranscoder) TranscodeCompat(ctx context.Context, src, dst string) error {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.TranscodeCompat")
	defer span.End()

	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("compat-transcode: stat source: %w", err)
	}
	// Space preflight: the output is roughly the source minus the audio-size
	// delta, so budgeting a full source-size plus margin is the safe advisory
	// bound (same contract as the audio-delay remuxer).
	var st syscall.Statfs_t
	if serr := syscall.Statfs(filepath.Dir(dst), &st); serr == nil {
		if avail := st.Bavail * uint64(st.Bsize); avail < uint64(srcInfo.Size())+compatHeadroomBytes { //nolint:gosec // sizes are positive
			return fmt.Errorf(
				"compat-transcode: %w: needs ~%d MiB free in the cache volume, %d MiB available",
				domainmkv.ErrInsufficientSpace, (srcInfo.Size()+compatHeadroomBytes)>>20, int64(avail)>>20) //nolint:gosec // display only
		}
	}

	ctx, cancel := context.WithTimeout(ctx, compatTimeout)
	defer cancel()
	argv := append([]string{"-n", "19", t.ffmpegPath}, BuildCompatArgs(src, dst)...)
	cmd := exec.CommandContext(ctx, "nice", argv...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("compat-transcode: ffmpeg: %w: %s", err, firstLine(stderr.Bytes()))
	}
	return nil
}
