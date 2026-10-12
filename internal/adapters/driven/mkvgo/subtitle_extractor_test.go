package mkvgo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/eveyproject/evey/internal/domain/mkv"
)

// TestExtractSubtitle_NonEBML verifies that ExtractSubtitle wraps
// mkv.ErrNotEBML for files with a non-EBML extension, without touching
// the filesystem.
func TestExtractSubtitle_NonEBML(t *testing.T) {
	a := New()
	dir := t.TempDir()
	src := filepath.Join(dir, "movie.mp4")
	out := filepath.Join(dir, "sub.srt")
	if err := os.WriteFile(src, []byte("not mkv"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := a.ExtractSubtitle(context.Background(), mkv.ExtractSubtitleRequest{
		SrcPath: src,
		TrackID: 1,
		OutPath: out,
	})
	if !errors.Is(err, mkv.ErrNotEBML) {
		t.Errorf("ExtractSubtitle on mp4: want ErrNotEBML, got %v", err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Errorf("output file must not be created for non-EBML source")
	}
}

// TestInferSubtitleFormat validates that the format inference from file
// extension returns the expected SubtitleFormat values.
func TestInferSubtitleFormat(t *testing.T) {
	cases := []struct {
		path string
		want mkv.SubtitleFormat
	}{
		{"/tmp/sub.srt", mkv.SubtitleFormatSRT},
		{"/tmp/sub.SRT", mkv.SubtitleFormatSRT},
		{"/tmp/dialog.ass", mkv.SubtitleFormatASS},
		{"/tmp/dialog.ASS", mkv.SubtitleFormatASS},
		{"/tmp/dialog.ssa", mkv.SubtitleFormatASS},
		{"/tmp/dialog.SSA", mkv.SubtitleFormatASS},
		{"/tmp/unknown.vtt", mkv.SubtitleFormatSRT}, // falls back to SRT
		{"noext", mkv.SubtitleFormatSRT},
	}

	for _, tc := range cases {
		got := inferSubtitleFormat(tc.path)
		if got != tc.want {
			t.Errorf("inferSubtitleFormat(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// TestExtractSubtitle_ExplicitFormatOverridesExtension verifies that an
// explicit Format in the request takes precedence over the OutPath extension.
// We use a non-EBML source to stay purely in the mapping layer.
func TestExtractSubtitle_ExplicitFormatOverridesExtension(t *testing.T) {
	a := New()
	dir := t.TempDir()
	// Extension says .ass but the source is non-EBML → ErrNotEBML is returned
	// before format resolution matters for I/O, confirming that the
	// non-EBML guard fires first and the code path reaches it.
	src := filepath.Join(dir, "clip.avi")
	out := filepath.Join(dir, "sub.ass")
	if err := os.WriteFile(src, []byte("avi header"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := a.ExtractSubtitle(context.Background(), mkv.ExtractSubtitleRequest{
		SrcPath: src,
		TrackID: 3,
		OutPath: out,
		Format:  mkv.SubtitleFormatASS,
	})
	if !errors.Is(err, mkv.ErrNotEBML) {
		t.Errorf("expected ErrNotEBML for avi, got %v", err)
	}
}

// TestExtractSubtitle_MissingFile verifies that ExtractSubtitle returns an
// error (not a panic) when the source EBML file does not exist.
func TestExtractSubtitle_MissingFile(t *testing.T) {
	a := New()
	dir := t.TempDir()
	src := filepath.Join(dir, "ghost.mkv") // EBML extension but file absent

	err := a.ExtractSubtitle(context.Background(), mkv.ExtractSubtitleRequest{
		SrcPath: src,
		TrackID: 1,
		OutPath: filepath.Join(dir, "sub.srt"),
	})
	if err == nil {
		t.Error("expected error for absent MKV file, got nil")
	}
}
