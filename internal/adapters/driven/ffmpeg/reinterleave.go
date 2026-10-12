package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	appmaintenance "github.com/eveyproject/evey/internal/app/maintenance"
	"github.com/eveyproject/evey/internal/pkg/procfork"
)

// reinterleaveBackupSuffix matches the mkvgo reindex backup, so the existing
// rollback / purge / "has backup" plumbing covers this repair too.
const reinterleaveBackupSuffix = ".mkvgobak"

// reinterleaveHeadroomBytes is the free space kept after the temp copy.
const reinterleaveHeadroomBytes = 256 << 20

// ErrReinterleaveUnverified: the rewrite did not carry every packet over.
var ErrReinterleaveUnverified = errors.New("reinterleave: the output does not carry the same packets as the source")

// Reinterleaver re-muxes a file whose tracks sit in blocks of one track at a
// time (maintenance.Reinterleaver). Three demuxers of the same file - video,
// audio, subtitles - and -max_interleave_delta 0: each demuxer advances in
// TIME rather than file position, so the muxer waits for no block and the
// resident memory stays under 50 MB whatever the block size (a single-input
// remux buffers a whole block, gigabytes on 4K). Stream copy throughout.
type Reinterleaver struct {
	ffmpegPath  string
	ffprobePath string
}

func NewReinterleaver(ffmpegPath, ffprobePath string) *Reinterleaver {
	return &Reinterleaver{ffmpegPath: ffmpegPath, ffprobePath: ffprobePath}
}

// Reinterleave writes the re-muxed file next to path, proves it carries the
// same packets per stream as the source, then swaps: the original becomes
// path.mkvgobak, the temp takes its place. Any failure removes the temp and
// leaves the original untouched.
func (r *Reinterleaver) Reinterleave(ctx context.Context, path string) (appmaintenance.ReinterleaveResult, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return appmaintenance.ReinterleaveResult{}, fmt.Errorf("reinterleave: %w", err)
	}
	if serr := reinterleaveSpaceOK(filepath.Dir(path), fi.Size()); serr != nil {
		return appmaintenance.ReinterleaveResult{}, serr
	}
	tmp := path + ".reinterleave.tmp"
	defer os.Remove(tmp) //nolint:errcheck // temp copy, best-effort cleanup; the swap consumes it on success

	out, err := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Scan, Site: "reinterleave.remux"},
		r.ffmpegPath, buildReinterleaveArgs(path, tmp)...)
	if err != nil {
		return appmaintenance.ReinterleaveResult{}, fmt.Errorf("reinterleave: ffmpeg: %w: %s", err, firstLine(out))
	}
	before, err := packetCounts(ctx, r.ffprobePath, path)
	if err != nil {
		return appmaintenance.ReinterleaveResult{}, err
	}
	after, err := packetCounts(ctx, r.ffprobePath, tmp)
	if err != nil {
		return appmaintenance.ReinterleaveResult{}, err
	}
	if !samePacketCounts(before, after) {
		return appmaintenance.ReinterleaveResult{}, fmt.Errorf("%w (source %v, output %v)", ErrReinterleaveUnverified, before, after)
	}
	bak := path + reinterleaveBackupSuffix
	if _, err := os.Stat(bak); err == nil {
		return appmaintenance.ReinterleaveResult{}, fmt.Errorf("reinterleave: %s already exists, not overwriting", bak)
	}
	if err := os.Rename(path, bak); err != nil {
		return appmaintenance.ReinterleaveResult{}, fmt.Errorf("reinterleave: keep original: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		if back := os.Rename(bak, path); back != nil {
			return appmaintenance.ReinterleaveResult{}, fmt.Errorf("reinterleave: swap failed (%w) AND the original could not be put back: %w", err, back)
		}
		return appmaintenance.ReinterleaveResult{}, fmt.Errorf("reinterleave: swap: %w", err)
	}
	return appmaintenance.ReinterleaveResult{BackupPath: bak}, nil
}

// buildReinterleaveArgs is the stream-copy remux: every stream type mapped
// from its own demuxer (`?` tolerates a file without that type), chapters,
// metadata and attachments carried from the first.
func buildReinterleaveArgs(src, dst string) []string {
	return []string{
		"-hide_banner", "-v", "error", "-y",
		"-i", src, "-i", src, "-i", src,
		"-map", "0:v?", "-map", "1:a?", "-map", "2:s?", "-map", "0:t?",
		"-c", "copy",
		"-max_interleave_delta", "0",
		"-map_chapters", "0", "-map_metadata", "0",
		"-f", "matroska", dst,
	}
}

// packetCounts returns, per stream index, how many packets ffprobe reads.
// Reads the whole file (no decode) - the proof that nothing was dropped.
func packetCounts(ctx context.Context, ffprobePath, path string) (map[int]int, error) {
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "reinterleave.packets"}, ffprobePath,
		"-v", "error", "-show_entries", "packet=stream_index", "-of", "csv=p=0", path)
	if err != nil {
		return nil, fmt.Errorf("reinterleave: count packets of %s: %w", filepath.Base(path), err)
	}
	counts := map[int]int{}
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx, err := strconv.Atoi(strings.SplitN(line, ",", 2)[0])
		if err != nil {
			continue
		}
		counts[idx]++
	}
	return counts, nil
}

// samePacketCounts compares the two files as multisets of per-stream packet
// counts: the remux reorders streams (video, audio, subtitles) but must keep
// every stream's packets.
func samePacketCounts(a, b map[int]int) bool {
	return fmt.Sprint(sortedCounts(a)) == fmt.Sprint(sortedCounts(b))
}

func sortedCounts(m map[int]int) []int {
	out := make([]int, 0, len(m))
	for _, n := range m {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// reinterleaveSpaceOK refuses a rewrite whose temp copy would fill the volume.
func reinterleaveSpaceOK(dir string, need int64) error {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return nil //nolint:nilerr // unknown filesystem: let the write itself decide
	}
	free := int64(st.Bavail) * st.Bsize //nolint:gosec // block counts fit int64
	if free < need+reinterleaveHeadroomBytes {
		return fmt.Errorf("reinterleave: %s has %d MB free, the temp copy needs %d MB", dir, free>>20, (need+reinterleaveHeadroomBytes)>>20)
	}
	return nil
}
