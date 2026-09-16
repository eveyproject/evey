package prober

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The production reader routes on the extension and then on what the bytes
// say: a .mkv that is really ISO-BMFF (a mislabeled download) is read by the
// mp4 reader instead of dying on the EBML magic, while a file that is neither
// fails with the EBML error the dispatcher turns into an ffprobe fallback.
func TestRealMetaReader_RoutesByExtensionThenByBytes(t *testing.T) {
	r := realMetaReader{}
	if !r.IsEBML("/x/a.MKV") || !r.IsEBML("/x/a.webm") || r.IsEBML("/x/a.mp4") {
		t.Error("IsEBML must follow the Matroska/WebM extensions, case-insensitively")
	}
	if !r.IsMP4("/x/a.M4V") || !r.IsMP4("/x/a.mov") || r.IsMP4("/x/a.mkv") {
		t.Error("IsMP4 must follow the ISO-BMFF extensions, case-insensitively")
	}

	dir := t.TempDir()
	ftypOnly := []byte{0, 0, 0, 0x14, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 0, 'i', 's', 'o', 'm'}
	write := func(name string, b []byte) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	ctx := context.Background()

	_, err := r.ReadMeta(ctx, write("mislabeled.mkv", ftypOnly))
	if err == nil || !strings.Contains(err.Error(), "mp4:") {
		t.Errorf("an ISO-BMFF payload under .mkv must reach the mp4 reader, got %v", err)
	}
	_, err = r.ReadMeta(ctx, write("tiny.mp4", ftypOnly))
	if err == nil || !strings.Contains(err.Error(), "mp4:") {
		t.Errorf("a .mp4 goes straight to the mp4 reader, got %v", err)
	}
	_, err = r.ReadMeta(ctx, write("garbage.mkv", []byte("not a container at all")))
	if err == nil || !strings.Contains(err.Error(), "EBML") {
		t.Errorf("neither container: the EBML error is what the dispatcher falls back on, got %v", err)
	}
	if _, err := r.ReadMeta(ctx, filepath.Join(dir, "missing.mp4")); err == nil {
		t.Error("a missing file is an error, not an empty container")
	}
}
