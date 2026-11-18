package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

func TestAudioFingerprintStore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s := NewAudioFingerprintStore(root)
	at := time.UnixMilli(1_700_000_000_123)
	require.NoError(t, s.Upsert(ctx, &library.IntroAudioFingerprintCacheEntry{
		MediaItemID: "ep-1", FileFingerprint: "file-1", WindowSec: 720,
		Fingerprint: []uint32{0x1A2B3C4D, 0, 0xFFFFFFFF}, AlgoVersion: 3, ExtractedAt: at,
	}))

	got, err := s.Get(ctx, "ep-1")
	require.NoError(t, err)
	assert.Equal(t, "ep-1", got.MediaItemID)
	assert.Equal(t, "file-1", got.FileFingerprint)
	assert.Equal(t, 720, got.WindowSec)
	assert.Equal(t, []uint32{0x1A2B3C4D, 0, 0xFFFFFFFF}, got.Fingerprint)
	assert.Equal(t, int16(3), got.AlgoVersion)
	assert.True(t, got.ExtractedAt.Equal(at), "extracted_at survives the round trip")
	assert.FileExists(t, filepath.Join(root, "audio", "ep-1.fp"))

	// Upsert replaces.
	require.NoError(t, s.Upsert(ctx, &library.IntroAudioFingerprintCacheEntry{
		MediaItemID: "ep-1", FileFingerprint: "file-2", WindowSec: 600, Fingerprint: []uint32{7}, AlgoVersion: 4,
	}))
	got, err = s.Get(ctx, "ep-1")
	require.NoError(t, err)
	assert.Equal(t, "file-2", got.FileFingerprint)
	assert.Equal(t, []uint32{7}, got.Fingerprint)
	assert.False(t, got.ExtractedAt.IsZero(), "an unset extraction time is stamped on write")
}

func TestAudioFingerprintStore_MissAndBatch(t *testing.T) {
	ctx := context.Background()
	s := NewAudioFingerprintStore(t.TempDir())

	_, err := s.Get(ctx, "nope")
	require.ErrorIs(t, err, shared.ErrNotFound)

	for _, id := range []string{"a", "b"} {
		require.NoError(t, s.Upsert(ctx, &library.IntroAudioFingerprintCacheEntry{
			MediaItemID: id, FileFingerprint: "f-" + id, WindowSec: 720, Fingerprint: []uint32{1, 2}, AlgoVersion: 1,
		}))
	}
	got, err := s.GetBatch(ctx, []string{"a", "b", "c"})
	require.NoError(t, err)
	require.Len(t, got, 2, "an id with no file is simply absent")
	assert.Equal(t, "f-b", got["b"].FileFingerprint)

	empty, err := s.GetBatch(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

// A damaged file must read as a miss so the caller recomputes, never as data.
func TestFingerprintStore_DamagedFileIsAMiss(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s := NewAudioFingerprintStore(root)
	require.NoError(t, s.Upsert(ctx, &library.IntroAudioFingerprintCacheEntry{
		MediaItemID: "ep", FileFingerprint: "f", WindowSec: 720, Fingerprint: []uint32{1, 2, 3, 4}, AlgoVersion: 1,
	}))
	p := filepath.Join(root, "audio", "ep.fp")
	good, err := os.ReadFile(p)
	require.NoError(t, err)

	cases := map[string][]byte{
		"empty":     {},
		"truncated": good[:len(good)-6],
		"bit flip":  flip(good, len(good)/2),
		"crc flip":  flip(good, len(good)-1),
		"bad magic": flip(good, 0),
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(p, content, 0o600))
			_, getErr := s.Get(ctx, "ep")
			require.ErrorIs(t, getErr, shared.ErrNotFound)
		})
	}

	// A pHash file under the audio name is not an audio entry.
	ph := NewPhashFingerprintStore(root)
	require.NoError(t, ph.Upsert(ctx, &library.IntroFingerprintCacheEntry{
		MediaItemID: "ep", FileFingerprint: "f", PhashTiles: []uint64{1}, PhashTimestamps: []int64{2}, AlgoVersion: 1,
	}))
	other, err := os.ReadFile(filepath.Join(root, "phash", "ep.fp"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, other, 0o600))
	_, err = s.Get(ctx, "ep")
	require.ErrorIs(t, err, shared.ErrNotFound)
}

func flip(b []byte, i int) []byte {
	out := append([]byte(nil), b...)
	out[i] ^= 0x40
	return out
}

func TestPhashFingerprintStore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	s := NewPhashFingerprintStore(t.TempDir())
	require.NoError(t, s.Upsert(ctx, &library.IntroFingerprintCacheEntry{
		MediaItemID: "film", FileFingerprint: "ff", AlgoVersion: 2,
		PhashTiles: []uint64{0xDEADBEEFCAFEBABE, 1}, PhashTimestamps: []int64{-5, 90_000},
	}))
	got, err := s.Get(ctx, "film")
	require.NoError(t, err)
	assert.Equal(t, []uint64{0xDEADBEEFCAFEBABE, 1}, got.PhashTiles)
	assert.Equal(t, []int64{-5, 90_000}, got.PhashTimestamps)
	assert.Equal(t, "ff", got.FileFingerprint)
	assert.Equal(t, int16(2), got.AlgoVersion)

	err = s.Upsert(ctx, &library.IntroFingerprintCacheEntry{
		MediaItemID: "film", PhashTiles: []uint64{1, 2}, PhashTimestamps: []int64{1},
	})
	require.Error(t, err, "mismatched lengths are refused")
}

// An id that is not a plain file name can never reach the file system.
func TestFingerprintStore_UnsafeIDs(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s := NewAudioFingerprintStore(root)
	for _, id := range []string{"", "../escape", "a/b", ".hidden", "x.fp"} {
		err := s.Upsert(ctx, &library.IntroAudioFingerprintCacheEntry{MediaItemID: id, Fingerprint: []uint32{1}})
		require.Error(t, err, id)
		_, err = s.Get(ctx, id)
		require.ErrorIs(t, err, shared.ErrNotFound, id)
		require.NoError(t, s.Delete(ctx, id), id)
	}
	_, err := os.Stat(filepath.Join(root, "escape.fp"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestFingerprintStore_ListDeleteAndAlgoSweep(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s := NewAudioFingerprintStore(root)

	ids, err := s.MediaItemIDs(ctx)
	require.NoError(t, err, "a store that was never written lists nothing")
	assert.Empty(t, ids)

	for id, algo := range map[string]int16{"old": 1, "cur": 2, "new": 3} {
		require.NoError(t, s.Upsert(ctx, &library.IntroAudioFingerprintCacheEntry{
			MediaItemID: id, WindowSec: 720, Fingerprint: []uint32{1}, AlgoVersion: algo,
		}))
	}
	dir := filepath.Join(root, "audio")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.fp"), []byte("junk"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cur.123.part"), []byte("torn"), 0o600))

	ids, err = s.MediaItemIDs(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"old", "cur", "new", "broken"}, ids)
	assert.NoFileExists(t, filepath.Join(dir, "cur.123.part"), "a leftover temp file is cleaned")

	n, err := s.DeleteByAlgoVersionBelow(ctx, 2)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n, "the old entry and the undecodable one go")
	ids, err = s.MediaItemIDs(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"cur", "new"}, ids)

	assert.True(t, s.Has("cur"))
	require.NoError(t, s.Delete(ctx, "cur"))
	require.NoError(t, s.Delete(ctx, "cur"), "deleting twice is fine")
	assert.False(t, s.Has("cur"))
}
