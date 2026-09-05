package httpapi

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The index is what turns a subtitle extraction from a full read of the film
// into a handful of seeks. It is also exactly as stale-able as the WebVTT it
// produces: applied to the wrong file it would seek to dead offsets and emit
// cues read from whatever bytes are there. These pin the keying.

func srcFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

func TestSubtitleIndexCache_KeyFollowsTheSourceFile(t *testing.T) {
	dir := t.TempDir()
	c := newSubtitleIndexCache(dir)
	src := srcFile(t, dir, "film.mkv", "abc")
	first := c.key(src)
	if first == "" {
		t.Fatal("a readable source must be keyable")
	}

	srcFile(t, dir, "film.mkv", "abcdef") // same path, new size
	if c.key(src) == first {
		t.Error("a file replaced in place must invalidate its index")
	}

	before := c.key(src)
	if err := os.Chtimes(src, time.Now().Add(time.Hour), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if c.key(src) == before {
		t.Error("mtime must take part in the key - a re-encode can land on the same byte count")
	}
}

// One index covers every text track of a file, which is the whole point of
// paying for a single walk. It must therefore NOT be keyed per track.
func TestSubtitleIndexCache_IsKeyedPerFileNotPerTrack(t *testing.T) {
	dir := t.TempDir()
	c := newSubtitleIndexCache(dir)
	src := srcFile(t, dir, "film.mkv", "abc")
	// Stable across instances: a restart must find the index it wrote before it.
	if again := newSubtitleIndexCache(dir).key(src); again != c.key(src) {
		t.Error("the key must be stable for a given file")
	}
	// And distinct from the WebVTT cache's own entries for that file, so the
	// two stores can share a directory shape without colliding.
	wv := newSubtitleCache(dir)
	if c.key(src) == wv.keyFor(src, 0) {
		t.Error("the index key must not collide with a track's WebVTT key")
	}
}

func TestSubtitleIndexCache_WithoutADataDirIsAWorkingNoOp(t *testing.T) {
	c := newSubtitleIndexCache("")
	if _, ok := c.get("/whatever.mkv"); ok {
		t.Error("must always miss")
	}
	if err := c.put("/whatever.mkv", nil); err != nil {
		t.Errorf("put must be a silent no-op, got %v", err)
	}
}

func TestSubtitleIndexCache_UnreadableSourceIsNotKeyable(t *testing.T) {
	c := newSubtitleIndexCache(t.TempDir())
	if k := c.key(filepath.Join(t.TempDir(), "absent.mkv")); k != "" {
		t.Errorf("key = %q, want empty for a missing source", k)
	}
}

// A corrupt blob is a miss, not a failure: the caller walks the file, which is
// slow and correct. Anything else would turn a damaged cache entry into a
// broken subtitle.
func TestSubtitleIndexCache_ACorruptBlobIsAMiss(t *testing.T) {
	dir := t.TempDir()
	c := newSubtitleIndexCache(dir)
	src := srcFile(t, dir, "film.mkv", "abc")
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(c.path(c.key(src)), []byte("not an index"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, ok := c.get(src); ok {
		t.Error("a blob that does not decode must not be served as an index")
	}
}

// An empty track list means "index everything" to mkvgo - a full walk for
// tracks we never serve as text. The caller must not reach it by accident.
func TestSubtitleIndexCache_EmptyTrackListDoesNotWalkTheFile(t *testing.T) {
	dir := t.TempDir()
	c := newSubtitleIndexCache(dir)
	src := srcFile(t, dir, "film.mkv", "abc")
	ix, err := c.buildAndStore(t.Context(), src, nil)
	if err != nil || ix != nil {
		t.Fatalf("buildAndStore(nil tracks) = %v, %v; want (nil, nil) without touching the file", ix, err)
	}
}
