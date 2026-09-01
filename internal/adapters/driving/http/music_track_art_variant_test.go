package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A track's cover comes out of the audio file's tag as bytes: the first ?w=
// request materialises them once under the cache dir, and every request after
// that is a plain resize of that file (coalesced, cached) - never the full
// cover again.
func TestTrackArtVariant_MaterialisesOnceThenResizes(t *testing.T) {
	dir := t.TempDir()
	conv := &fakeImageConverter{}
	h := &musicHandler{imgConverter: conv, trackArtDir: dir}
	data := []byte("jpeg-bytes-from-the-tag")

	v := h.trackArtVariant(context.Background(), "tr-0a1b", "image/jpeg", data, 160)
	if v == "" || !strings.HasSuffix(v, "_w160.webp") {
		t.Fatalf("variant = %q, want a _w160.webp path", v)
	}
	got, err := os.ReadFile(v)
	if err != nil || string(got) != "RIFF-webp-resized" {
		t.Fatalf("variant content = %q, %v", got, err)
	}
	src, err := os.ReadFile(filepath.Join(dir, "tr-0a1b.jpg"))
	if err != nil || string(src) != string(data) {
		t.Fatalf("source not materialised as-is: %q, %v", src, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "tr-0a1b.jpg.tmp")); !os.IsNotExist(err) {
		t.Fatal("temp file left behind")
	}

	again := h.trackArtVariant(context.Background(), "tr-0a1b", "image/jpeg", data, 160)
	if again != v {
		t.Fatalf("second call = %q, want the cached %q", again, v)
	}
	if conv.resizeCalled != 1 {
		t.Fatalf("resize called %d times, want 1 (the variant is cached)", conv.resizeCalled)
	}
	// Another width is another variant of the same source.
	if w2 := h.trackArtVariant(context.Background(), "tr-0a1b", "image/jpeg", data, 320); !strings.HasSuffix(w2, "_w320.webp") || conv.resizeCalled != 2 {
		t.Fatalf("second width: %q, resizes=%d", w2, conv.resizeCalled)
	}
}

// The id is a route parameter and becomes a file name: only the slug shape
// Evey mints is admitted, so it can never name a path outside the cache dir.
func TestTrackArtVariant_RefusesUnsafeIDs(t *testing.T) {
	dir := t.TempDir()
	h := &musicHandler{imgConverter: &fakeImageConverter{}, trackArtDir: dir}
	for _, id := range []string{"../evil", "a/b", "", "tr-1.jpg", strings.Repeat("x", 129), "tr 1"} {
		if v := h.trackArtVariant(context.Background(), id, "image/jpeg", []byte("x"), 100); v != "" {
			t.Fatalf("id %q produced a variant %q", id, v)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("cache dir touched by refused ids: %v", entries)
	}
}

func TestTrackArtVariant_OffWithoutDirOrConverter(t *testing.T) {
	if v := (&musicHandler{imgConverter: &fakeImageConverter{}}).trackArtVariant(context.Background(), "tr-1", "image/png", []byte("x"), 100); v != "" {
		t.Fatalf("no dir: got %q", v)
	}
	if v := (&musicHandler{trackArtDir: t.TempDir()}).trackArtVariant(context.Background(), "tr-1", "image/png", []byte("x"), 100); v != "" {
		t.Fatalf("no converter: got %q", v)
	}
	if v := (&musicHandler{imgConverter: &fakeImageConverter{}, trackArtDir: t.TempDir()}).trackArtVariant(context.Background(), "tr-1", "image/png", []byte("x"), 0); v != "" {
		t.Fatalf("width 0: got %q", v)
	}
}

// A resize that fails must not poison the cache: the original stays, the
// caller serves it, and the next request may try again.
func TestTrackArtVariant_ResizeFailureFallsBack(t *testing.T) {
	dir := t.TempDir()
	conv := &fakeImageConverter{failErr: os.ErrPermission}
	h := &musicHandler{imgConverter: conv, trackArtDir: dir}
	if v := h.trackArtVariant(context.Background(), "tr-9", "image/png", []byte("png"), 120); v != "" {
		t.Fatalf("got %q, want empty on resize failure", v)
	}
	if _, err := os.Stat(filepath.Join(dir, "tr-9.png")); err != nil {
		t.Fatalf("source should be materialised even when the resize fails: %v", err)
	}
}

func TestTrackArtExt(t *testing.T) {
	for in, want := range map[string]string{"image/jpeg": ".jpg", "IMAGE/JPG": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif", "application/octet-stream": ".img", "": ".img"} {
		if got := trackArtExt(in); got != want {
			t.Errorf("trackArtExt(%q) = %q, want %q", in, got, want)
		}
	}
}
