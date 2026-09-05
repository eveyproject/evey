package httpapi

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Producing a subtitle costs a full read of the film - 71 seconds measured on a
// production server for 7 KB of text, and 71 seconds again on the very next
// identical request. These pin the two properties that make the second request
// free without ever serving the wrong film's subtitles.

func writeSource(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return p
}

func TestSubtitleCache_StoresAndReturns(t *testing.T) {
	dir := t.TempDir()
	c := newSubtitleCache(dir)
	src := writeSource(t, dir, "film.mkv", "abc")

	key := c.keyFor(src, 0)
	if key == "" {
		t.Fatal("a readable source must be cacheable")
	}
	if _, ok := c.get(key); ok {
		t.Fatal("nothing stored yet")
	}
	if err := c.put(key, []byte("WEBVTT\n\n")); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, ok := c.get(key)
	if !ok || string(got) != "WEBVTT\n\n" {
		t.Fatalf("get = %q %v", got, ok)
	}
}

// A file replaced in place - a re-download, a repaired remux - keeps its path.
// Serving the previous file's subtitles for it would be worse than a slow
// extraction, so size and mtime are part of the key.
func TestSubtitleCache_KeyFollowsTheSourceFile(t *testing.T) {
	dir := t.TempDir()
	c := newSubtitleCache(dir)
	src := writeSource(t, dir, "film.mkv", "abc")
	first := c.keyFor(src, 0)

	// Same path, different content: different size.
	writeSource(t, dir, "film.mkv", "abcdef")
	if second := c.keyFor(src, 0); second == first {
		t.Error("a file replaced in place must invalidate its cached subtitles")
	}

	// Same path, same size, different mtime: a re-encode can land on the same
	// byte count.
	writeSource(t, dir, "film.mkv", "abcdef")
	if err := os.Chtimes(src, time.Now().Add(time.Hour), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if third := c.keyFor(src, 0); third == c.key(src, 0, 6, 0) {
		t.Error("mtime must take part in the key")
	}
}

func TestSubtitleCache_KeyIsPerTrack(t *testing.T) {
	dir := t.TempDir()
	c := newSubtitleCache(dir)
	src := writeSource(t, dir, "film.mkv", "abc")
	if c.keyFor(src, 0) == c.keyFor(src, 1) {
		t.Error("two tracks of one film must not share an entry")
	}
}

// A deployment with no data dir must behave exactly as before rather than fail:
// every lookup misses, every store is dropped.
func TestSubtitleCache_WithoutADataDirIsAWorkingNoOp(t *testing.T) {
	c := newSubtitleCache("")
	if k := c.keyFor("/whatever.mkv", 0); k != "" {
		t.Errorf("key = %q, want empty", k)
	}
	if _, ok := c.get("k"); ok {
		t.Error("must always miss")
	}
	if err := c.put("k", []byte("x")); err != nil {
		t.Errorf("put must be a silent no-op, got %v", err)
	}
}

// An unreadable source cannot be keyed on facts we do not have: extract without
// caching rather than invent an entry.
func TestSubtitleCache_UnreadableSourceIsNotCacheable(t *testing.T) {
	c := newSubtitleCache(t.TempDir())
	if k := c.keyFor(filepath.Join(t.TempDir(), "absent.mkv"), 0); k != "" {
		t.Errorf("key = %q, want empty for a missing source", k)
	}
}

func TestSubtitleCache_RefusesAnOversizedEntry(t *testing.T) {
	dir := t.TempDir()
	c := newSubtitleCache(dir)
	src := writeSource(t, dir, "film.mkv", "abc")
	if err := c.put(c.keyFor(src, 0), make([]byte, subtitleCacheMaxBytes+1)); err == nil {
		t.Error("an oversized body is not a subtitle any player renders")
	}
}

func TestSubtitleCache_PurgeEmptiesIt(t *testing.T) {
	dir := t.TempDir()
	c := newSubtitleCache(dir)
	src := writeSource(t, dir, "film.mkv", "abc")
	key := c.keyFor(src, 0)
	if err := c.put(key, []byte("WEBVTT\n")); err != nil {
		t.Fatalf("put: %v", err)
	}
	n, err := c.purge()
	if err != nil || n != 1 {
		t.Fatalf("purge = %d, %v", n, err)
	}
	if _, ok := c.get(key); ok {
		t.Error("purge must leave nothing behind")
	}
	// And purging an empty (or absent) cache is not an error.
	if _, err := c.purge(); err != nil {
		t.Errorf("second purge: %v", err)
	}
}
