package httpapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// The store is refused under the free-space floor: the directory lives on the
// system disk and nothing else bounds it.
func TestSubtitleCache_RefusesBelowTheFreeSpaceFloor(t *testing.T) {
	dir := t.TempDir()
	c := newSubtitleCache(dir)
	src := writeSource(t, dir, "film.mkv", "abc")
	key := c.keyFor(src, 0)

	oldFloor := subtitleCacheWriteFloor.Load()
	oldFree := subtitleCacheFreeBytes
	t.Cleanup(func() {
		subtitleCacheWriteFloor.Store(oldFloor)
		subtitleCacheFreeBytes = oldFree
	})
	SetSubtitleCacheWriteFloor(1 << 20)

	// 1 MiB + 10 bytes free, a 100-byte body: the store would cross the floor.
	subtitleCacheFreeBytes = func(string) (int64, error) { return 1<<20 + 10, nil }
	if err := c.put(key, make([]byte, 100)); !errors.Is(err, errSubtitleCacheFull) {
		t.Fatalf("a store crossing the floor must be refused, got %v", err)
	}
	if _, ok := c.get(key); ok {
		t.Error("nothing may be stored under the floor")
	}
	// Plenty of room: stored.
	subtitleCacheFreeBytes = func(string) (int64, error) { return 10 << 30, nil }
	if err := c.put(key, []byte("WEBVTT\n")); err != nil {
		t.Fatalf("put above the floor: %v", err)
	}
	// A statfs failure never blocks the store: the floor is a guard.
	subtitleCacheFreeBytes = func(string) (int64, error) { return 0, errors.New("statfs boom") }
	if err := c.put(c.keyFor(src, 1), []byte("WEBVTT\n")); err != nil {
		t.Fatalf("a statfs failure must not refuse the store: %v", err)
	}
	// Floor 0 disables the check.
	SetSubtitleCacheWriteFloor(0)
	subtitleCacheFreeBytes = func(string) (int64, error) { return 0, nil }
	if err := c.put(c.keyFor(src, 2), []byte("WEBVTT\n")); err != nil {
		t.Fatalf("floor 0 must disable the check: %v", err)
	}
}

// A failed extraction is remembered under the mkvgo version, so a decoder
// upgrade replays it; a cancelled one is not a failure of the track.
func TestSubtitleCache_FailureMemo_IsVersionedAndIgnoresCancellation(t *testing.T) {
	dir := t.TempDir()
	c := newSubtitleCache(dir)
	src := writeSource(t, dir, "film.mkv", "abc")
	key := c.keyFor(src, 0)

	if _, failed := c.getFailure(key); failed {
		t.Fatal("no memo yet")
	}
	if err := c.putFailure(key, context.Canceled); err != nil {
		t.Fatal(err)
	}
	if _, failed := c.getFailure(key); failed {
		t.Fatal("a cancelled extraction must not be memorised")
	}
	if err := c.putFailure(key, errors.New("PGS: zlib without ContentEncodings")); err != nil {
		t.Fatal(err)
	}
	reason, failed := c.getFailure(key)
	if !failed || reason != "PGS: zlib without ContentEncodings" {
		t.Fatalf("memo = %q, %v", reason, failed)
	}
	if !strings.Contains(c.failureKey(key), packagerVersion()) {
		t.Errorf("the memo key must carry the mkvgo version, got %q", c.failureKey(key))
	}
	// The memo is a separate entry: the track itself is still a miss.
	if _, ok := c.get(key); ok {
		t.Error("a failure memo is not a cached subtitle")
	}
	// Purge takes the memos with it.
	if _, err := c.purge(); err != nil {
		t.Fatal(err)
	}
	if _, failed := c.getFailure(key); failed {
		t.Error("purge must remove failure memos")
	}
	// No data dir: a no-op, like every other store.
	var none subtitleCache
	if err := none.putFailure("k", errors.New("x")); err != nil {
		t.Errorf("no-dir putFailure: %v", err)
	}
	if _, failed := none.getFailure("k"); failed {
		t.Error("no-dir cache remembers nothing")
	}
}

// The admin wipe reaches this store through SubtitleCachePurger.
func TestSubtitleCachePurger_PurgesTheStoreUnderDataDir(t *testing.T) {
	dataDir := t.TempDir()
	c := newSubtitleCache(dataDir)
	src := writeSource(t, dataDir, "film.mkv", "abc")
	if err := c.put(c.keyFor(src, 0), []byte("WEBVTT\n")); err != nil {
		t.Fatal(err)
	}
	n, err := SubtitleCachePurger(dataDir)(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("purger = %d, %v", n, err)
	}
}

// A refused store pages the listener once, then stays quiet for an hour: a
// full disk refuses every store, and one page per track is hundreds of
// notifications for one fact.
func TestSubtitleCache_FloorHitWarnsOncePerHour(t *testing.T) {
	dir := t.TempDir()
	c := newSubtitleCache(dir)
	src := writeSource(t, dir, "film.mkv", "abc")
	oldFloor := subtitleCacheWriteFloor.Load()
	oldFree := subtitleCacheFreeBytes
	oldHook := subtitleCacheFloorHit
	oldWarned := subtitleCacheFloorWarnedA.Load()
	t.Cleanup(func() {
		subtitleCacheWriteFloor.Store(oldFloor)
		subtitleCacheFreeBytes = oldFree
		subtitleCacheFloorHit = oldHook
		subtitleCacheFloorWarnedA.Store(oldWarned)
	})
	SetSubtitleCacheWriteFloor(1 << 20)
	subtitleCacheFreeBytes = func(string) (int64, error) { return 1 << 20, nil }
	subtitleCacheFloorWarnedA.Store(0)
	calls := 0
	var gotDir string
	OnSubtitleCacheFloorHit(func(d string, free, floor int64) {
		calls++
		gotDir = d
		if free != 1<<20 || floor != 1<<20 {
			t.Errorf("listener told free=%d floor=%d", free, floor)
		}
	})
	for i := 0; i < 5; i++ {
		if err := c.put(c.keyFor(src, i), []byte("WEBVTT\n")); !errors.Is(err, errSubtitleCacheFull) {
			t.Fatalf("store %d must be refused, got %v", i, err)
		}
	}
	if calls != 1 {
		t.Errorf("five refusals within the hour = one page, got %d", calls)
	}
	if gotDir != c.dir {
		t.Errorf("listener told dir %q, want %q", gotDir, c.dir)
	}
	// An hour later, one more.
	subtitleCacheFloorWarnedA.Store(time.Now().Add(-2 * time.Hour).Unix())
	_ = c.put(c.keyFor(src, 9), []byte("WEBVTT\n"))
	if calls != 2 {
		t.Errorf("after an hour the next refusal pages again, got %d", calls)
	}
}
