package transcoding

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Prod 2026-09: 259 progress files and nothing removing them. Old ffmpeg-*.log
// files go, recent ones and anything else in the directory stay.
func TestPruneProgressLogs_RemovesOnlyOldProgressFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	write := func(name string, age time.Duration) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		mt := now.Add(-age)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	write("ffmpeg-aaaa-v0-s0-1.log", 20*24*time.Hour)
	write("ffmpeg-bbbb-v1-s3-2.log", 2*24*time.Hour)
	write("notes.txt", 40*24*time.Hour)

	n, err := pruneProgressLogs(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("removed %d files, want 1", n)
	}
	for name, want := range map[string]bool{"ffmpeg-aaaa-v0-s0-1.log": false, "ffmpeg-bbbb-v1-s3-2.log": true, "notes.txt": true} {
		_, statErr := os.Stat(filepath.Join(dir, name))
		if exists := statErr == nil; exists != want {
			t.Fatalf("%s exists=%v, want %v", name, exists, want)
		}
	}
}

func TestPruneProgressLogs_MissingDirIsNotAnError(t *testing.T) {
	n, err := pruneProgressLogs(filepath.Join(t.TempDir(), "absent"), time.Now())
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v, want 0 and nil", n, err)
	}
}

// The sweep is spaced: a burst of sessions triggers one prune per hour.
func TestMaybePruneProgressLogs_AtMostOncePerInterval(t *testing.T) {
	progressLogLastPrune.Store(0)
	dir := t.TempDir()
	t0 := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	maybePruneProgressLogs(dir, t0)
	first := progressLogLastPrune.Load()
	maybePruneProgressLogs(dir, t0.Add(10*time.Minute))
	if progressLogLastPrune.Load() != first {
		t.Fatal("a second call inside the interval must not sweep again")
	}
	maybePruneProgressLogs(dir, t0.Add(progressLogPruneEvery+time.Second))
	if progressLogLastPrune.Load() == first {
		t.Fatal("past the interval the sweep must run")
	}
}
