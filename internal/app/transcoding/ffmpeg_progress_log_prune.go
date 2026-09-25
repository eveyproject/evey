package transcoding

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// progressLogRetention is how long an encoder's -progress file is kept. The
// files are small (8 MB for 259 of them, prod 2026-09) but nothing removed
// them: two weeks keeps every incident an operator would still be reading.
const progressLogRetention = 14 * 24 * time.Hour

// progressLogPruneEvery spaces the sweeps: the prune runs from the session
// spawn path and must cost it nothing on the sessions in between.
const progressLogPruneEvery = time.Hour

var progressLogLastPrune atomic.Int64 // unix seconds of the last sweep

// maybePruneProgressLogs sweeps dir at most once per progressLogPruneEvery,
// off the caller's path. A failed sweep is a log line, never a failed spawn.
func maybePruneProgressLogs(dir string, now time.Time) {
	if dir == "" {
		return
	}
	last := progressLogLastPrune.Load()
	if now.Unix()-last < int64(progressLogPruneEvery/time.Second) {
		return
	}
	if !progressLogLastPrune.CompareAndSwap(last, now.Unix()) {
		return // another session took this sweep
	}
	go func() {
		if n, err := pruneProgressLogs(dir, now); err != nil {
			slog.Warn("ffmpeg progress logs: prune failed", slog.String("dir", dir), slog.Any("err", err))
		} else if n > 0 {
			slog.Info("ffmpeg progress logs: pruned", slog.String("dir", dir), slog.Int("removed", n))
		}
	}()
}

// pruneProgressLogs removes the ffmpeg-*.log files in dir older than
// progressLogRetention and returns how many went. Other files are left alone.
func pruneProgressLogs(dir string, now time.Time) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	cutoff := now.Add(-progressLogRetention)
	removed := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "ffmpeg-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err == nil {
			removed++
		}
	}
	return removed, nil
}
