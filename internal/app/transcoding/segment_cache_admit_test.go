package transcoding

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAdmitWrite(t *testing.T) {
	newCache := func(t *testing.T) *SegmentCache {
		t.Helper()
		c, err := NewSegmentCache(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return c
	}

	t.Run("floor zero fails open", func(t *testing.T) {
		c := newCache(t)
		c.freeBytesFn = func() (int64, error) { return 0, nil } // below any floor
		if err := c.AdmitWrite(); err != nil {
			t.Fatalf("floor 0 must never block: %v", err)
		}
	})

	t.Run("statfs error fails open", func(t *testing.T) {
		c := newCache(t)
		c.SetWriteFloor(1000)
		c.freeBytesFn = func() (int64, error) { return 0, errors.New("statfs boom") }
		if err := c.AdmitWrite(); err != nil {
			t.Fatalf("a statfs error must fail open: %v", err)
		}
	})

	t.Run("above floor admits without a walk", func(t *testing.T) {
		c := newCache(t)
		c.SetWriteFloor(1000)
		c.freeBytesFn = func() (int64, error) { return 2000, nil }
		if err := c.AdmitWrite(); err != nil {
			t.Fatalf("above floor must admit: %v", err)
		}
	})

	t.Run("below floor with nothing evictable sheds", func(t *testing.T) {
		c := newCache(t)
		c.SetWriteFloor(1000)
		c.freeBytesFn = func() (int64, error) { return 500, nil }
		if err := c.AdmitWrite(); !errors.Is(err, ErrCacheFull) {
			t.Fatalf("want ErrCacheFull, got %v", err)
		}
	})

	t.Run("eviction recovers room then admits", func(t *testing.T) {
		c := newCache(t)
		c.SetWriteFloor(1000)
		// An idle hash directory the on-demand pass can evict.
		dir := filepath.Join(c.baseDir, "deadbeef")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "seg0.m4s"), []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
		// Free space is a function of the cache state: once the idle hash is
		// evicted, the filesystem clears the floor. Deterministic, no goroutine.
		c.freeBytesFn = func() (int64, error) {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				return 2000, nil
			}
			return 500, nil
		}
		if err := c.AdmitWrite(); err != nil {
			t.Fatalf("eviction should have recovered room: %v", err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Error("the idle hash should have been evicted")
		}
	})

	t.Run("cooldown skips a second walk", func(t *testing.T) {
		c := newCache(t)
		c.SetWriteFloor(1000)
		c.freeBytesFn = func() (int64, error) { return 500, nil }
		c.lastOnDemandEvict = time.Now() // inside the cooldown
		if ran := c.tryOnDemandEvict(1000); ran {
			t.Error("a walk ran inside the cooldown window")
		}
	})
}
