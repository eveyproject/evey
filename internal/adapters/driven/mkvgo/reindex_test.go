package mkvgo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/evey-project/evey/internal/domain/mkv"
)

// A non-EBML file (wrong extension / not Matroska) must be reported as having
// no seek index and skipped by ReindexInPlace without ever being touched —
// the sweep walks mixed trees (mp4/avi/srt) and must leave non-MKV alone.
func TestReindexInPlace_NonEBML_SkipsUntouched(t *testing.T) {
	a := New()
	dir := t.TempDir()
	path := filepath.Join(dir, "movie.mp4")
	content := []byte("not an mkv, just bytes pretending to be a container")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := a.ReindexInPlace(context.Background(), path, mkv.ReindexOptions{KeepBackup: true})
	if err != nil {
		t.Fatalf("ReindexInPlace on non-EBML: %v", err)
	}
	if !res.Skipped {
		t.Errorf("non-EBML file must be Skipped, got %+v", res)
	}
	// The file must be byte-for-byte untouched and no backup created.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("non-EBML file was modified")
	}
	if _, err := os.Stat(path + ".mkvgobak"); !os.IsNotExist(err) {
		t.Errorf("no backup must be created for a skipped file")
	}
	if _, err := os.Stat(path + ".reindex.tmp"); !os.IsNotExist(err) {
		t.Errorf("temp file must not linger")
	}
}

func TestHasSeekIndex_NonEBML_False(t *testing.T) {
	a := New()
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(path, []byte("mp4ish"), 0o644); err != nil {
		t.Fatal(err)
	}
	has, err := a.HasSeekIndex(context.Background(), path)
	if err != nil {
		t.Fatalf("HasSeekIndex on non-EBML: %v", err)
	}
	if has {
		t.Errorf("non-EBML file must report no seek index")
	}
}
