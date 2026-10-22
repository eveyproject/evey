package library

// trickplay_settle.go - sprites are made from a complete file only.
//
// Until 2026-09-26 a file was analysed the moment it appeared, and some of the
// 28 truncated trickplays found on prod on 2026-10-21 stop where the copy was
// (Normal and The Whisper Man, the same files, are complete on a server that
// made them a month later); regenerated on 2026-10-22, 24 of them came out
// whole. The scan now waits for a file to sit unmodified for DefaultFileSettle,
// but a generation has other triggers (the sweep, the admin button), and a
// copy that keeps the source's timestamp (Windows Explorer, cp -p, rsync -t)
// looks settled while it grows. So a generation checks the file on disk itself
// before it starts and after it ends.
//
// Sprites that stop well before the announced end are kept and logged: the
// file's picture can end there. Of the four that stayed short, one is a
// truncated download and one, Tron, announces 156.5 min while its picture ends
// near 125 (credits at 120 min, no frame at 130); refusing them left such a
// file with no preview at all.

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/pkg/scanindex"
)

const (
	// trickplaySizeProbe is how long the file's size must hold before a
	// generation starts: a copy keeping the source's timestamp is caught
	// growing.
	trickplaySizeProbe = 5 * time.Second
	// trickplayRetryDelay is when a generation postponed for an incomplete
	// file is tried again.
	trickplayRetryDelay = DefaultFileSettle + 30*time.Second
)

// diskFile is a file as stat'd.
type diskFile struct {
	size int64
	mod  time.Time
}

func statDisk(path string) (diskFile, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return diskFile{}, false
	}
	return diskFile{size: fi.Size(), mod: fi.ModTime()}, true
}

// fileIncomplete says why the file on disk is not yet the complete one the
// library knows, or "" when it is. A file that cannot be stat'd is left to
// the generator, whose error already says so. It returns the file as stat'd,
// for the check after the generation.
func fileIncomplete(ctx context.Context, f *library.MediaFile, now time.Time, probe time.Duration) (string, diskFile) {
	d, ok := statDisk(f.Path)
	if !ok {
		return "", diskFile{}
	}
	switch {
	case d.size != f.Size:
		return fmt.Sprintf("size on disk %d, the library knows %d", d.size, f.Size), d
	case !scanindex.SameMtime(d.mod, f.ModTime):
		return "modified on disk since the scan", d
	case now.Sub(d.mod) < DefaultFileSettle:
		return "modified less than a minute ago", d
	}
	t := time.NewTimer(probe)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return "", d
	case <-t.C:
	}
	if again, ok := statDisk(f.Path); ok && again.size != d.size {
		return fmt.Sprintf("still growing (%d then %d bytes)", d.size, again.size), d
	}
	return "", d
}

// fileChangedSince reports whether the file on disk is no longer the one a
// generation started from.
func fileChangedSince(path string, before diskFile) bool {
	now, ok := statDisk(path)
	if !ok {
		return false
	}
	return now.size != before.size || !scanindex.SameMtime(now.mod, before.mod)
}

// trickplayTailGapMs is how far before the end of the media the last thumbnail
// may fall: two minutes, or a tenth of the media when it is longer. The
// thumbnails are a few seconds apart, so a complete file never comes close.
func trickplayTailGapMs(durationMs int64) int64 {
	return max(2*time.Minute.Milliseconds(), durationMs/10)
}

// errTrickplayShort is a generation whose thumbnails stop well before the
// announced end of the media: a truncated file, a container announcing more
// than its video holds, or a stream the full scan cannot read past (Raised by
// Wolves S02, no intra picture after its first minutes).
type errTrickplayShort struct {
	lastMs, durationMs int64
}

func (e errTrickplayShort) Error() string {
	return fmt.Sprintf("trickplay: the thumbnails stop at %s of %s",
		time.Duration(e.lastMs)*time.Millisecond, time.Duration(e.durationMs)*time.Millisecond)
}

// checkTrickplayCoverage reports thumbnails that stop more than
// trickplayTailGapMs before durationMs. An unknown duration is not checked.
func checkTrickplayCoverage(frameMs []int64, durationMs int64) error {
	if durationMs <= 0 {
		return nil
	}
	var last int64
	if n := len(frameMs); n > 0 {
		last = frameMs[n-1]
	}
	if durationMs-last > trickplayTailGapMs(durationMs) {
		return errTrickplayShort{lastMs: last, durationMs: durationMs}
	}
	return nil
}

// retryLater queues the item again once a copy in progress has had time to
// end. The queued generation checks the file again.
func (s *TrickplayService) retryLater(mediaItemID string) {
	time.AfterFunc(s.settle.retry, func() {
		if q := s.requeue.Load(); q != nil {
			(*q)(mediaItemID)
		}
	})
}

// trickplaySettle holds the two delays above, per service so a test can
// shorten them without racing the others.
type trickplaySettle struct {
	probe, retry time.Duration
}
