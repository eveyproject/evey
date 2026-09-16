package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gravity-zero/mkvgo/matroska"
)

// The index store keeps a per-FILE blob keyed like the WebVTT cache (path,
// size, mtime), publishes it atomically, and treats anything it cannot read
// back as a miss - the caller then walks the film, slow but correct. With no
// data dir it is a working no-op.
func TestSubtitleIndexCache_RoundTripAndMisses(t *testing.T) {
	dir := t.TempDir()
	film := filepath.Join(dir, "film.mkv")
	if err := os.WriteFile(film, []byte("not really matroska"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := newSubtitleIndexCache(dir)

	if _, ok := c.get(film); ok {
		t.Fatal("empty cache must miss")
	}
	if err := c.put(film, &matroska.SubtitleIndex{}); err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, ok := c.get(film); !ok {
		t.Fatal("a stored index must be found again")
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, "subtitle-index")); len(entries) != 1 {
		t.Errorf("one published blob expected, no leftover temp file: %d entries", len(entries))
	}

	// A garbage blob under the right key is a miss, not a crash.
	if err := os.WriteFile(c.path(c.key(film)), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.get(film); ok {
		t.Error("an undecodable blob must read as a miss")
	}

	// A file that changed (size) gets a new key: the old blob never applies to it.
	if err := c.put(film, &matroska.SubtitleIndex{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(film, []byte("not really matroska, but longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.get(film); ok {
		t.Error("a rewritten source must not reuse the previous index")
	}

	// Missing source, nil index, no dir: all quiet no-ops.
	if _, ok := c.get(filepath.Join(dir, "missing.mkv")); ok {
		t.Error("a missing source has no key and no index")
	}
	if err := c.put(filepath.Join(dir, "missing.mkv"), &matroska.SubtitleIndex{}); err != nil {
		t.Errorf("put for a missing source is a no-op, got %v", err)
	}
	if err := c.put(film, nil); err != nil {
		t.Errorf("put(nil) = %v", err)
	}
	none := newSubtitleIndexCache("")
	if err := none.put(film, &matroska.SubtitleIndex{}); err != nil {
		t.Errorf("no data dir: put = %v", err)
	}
	if _, ok := none.get(film); ok {
		t.Error("no data dir: always a miss")
	}

	// Nothing to index is not a failure; a source mkvgo cannot walk is.
	if ix, err := c.buildAndStore(context.Background(), film, nil); ix != nil || err != nil {
		t.Errorf("no tracks: (%v, %v)", ix, err)
	}
	if _, err := c.buildAndStore(context.Background(), film, []uint64{3}); err == nil {
		t.Error("walking a non-Matroska source must fail")
	}
}
