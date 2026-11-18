package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
)

// Run on the HOST (the container skips silently, no docker socket):
// CGO_ENABLED=0 go test ./internal/adapters/driven/postgres/ -run TestIntroFingerprintLegacy

// The move out of the database walks each table by keyset: with more rows
// than a page holds, every row comes back exactly once, in id order, with what
// was stored. What moved is deleted, by id, inside the caller's transaction.
func TestIntroFingerprintLegacy_PagesAndDeletes(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()

	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, newTestLibrary("lib-leg")))
	itemRepo := postgres.NewMediaItemRepository(db)
	ids := make([]string, 5)
	for i := range ids {
		ids[i] = fmt.Sprintf("leg-ep%02d", i)
	}
	for _, id := range ids {
		require.NoError(t, itemRepo.Create(ctx, newTestMediaItem(id, "lib-leg")))
	}
	audio := postgres.NewIntroAudioFingerprintCacheRepository(db)
	visual := postgres.NewIntroFingerprintCacheRepository(db)
	for i, id := range ids {
		require.NoError(t, audio.Upsert(ctx, &library.IntroAudioFingerprintCacheEntry{
			MediaItemID: id, FileFingerprint: "file-" + id, WindowSec: 720,
			Fingerprint: []uint32{uint32(i), 0xDEADBEEF}, AlgoVersion: 1,
		}))
		require.NoError(t, visual.Upsert(ctx, &library.IntroFingerprintCacheEntry{
			MediaItemID: id, FileFingerprint: "file-" + id,
			PhashTiles: []uint64{uint64(i), 1 << 63}, PhashTimestamps: []int64{int64(i) * 1000, 2000}, AlgoVersion: 2,
		}))
	}

	t.Run("audio pages", func(t *testing.T) {
		var seen []string
		after := ""
		for range 10 {
			page, err := audio.Page(ctx, after, 2)
			require.NoError(t, err)
			if len(page) == 0 {
				break
			}
			require.LessOrEqual(t, len(page), 2)
			for _, e := range page {
				seen = append(seen, e.MediaItemID)
				assert.Equal(t, "file-"+e.MediaItemID, e.FileFingerprint)
				assert.Equal(t, 720, e.WindowSec)
				assert.Len(t, e.Fingerprint, 2)
				assert.False(t, e.ExtractedAt.IsZero(), "the extraction time travels with the row")
			}
			after = page[len(page)-1].MediaItemID
		}
		assert.Equal(t, ids, seen, "every row once, in id order")
	})

	t.Run("visual pages", func(t *testing.T) {
		var seen []string
		after := ""
		for range 10 {
			page, err := visual.Page(ctx, after, 2)
			require.NoError(t, err)
			if len(page) == 0 {
				break
			}
			for _, e := range page {
				seen = append(seen, e.MediaItemID)
				assert.Len(t, e.PhashTiles, 2)
				assert.Len(t, e.PhashTimestamps, 2)
				assert.EqualValues(t, 2, e.AlgoVersion)
				assert.False(t, e.ExtractedAt.IsZero())
			}
			after = page[len(page)-1].MediaItemID
		}
		assert.Equal(t, ids, seen, "every row once, in id order")
	})

	t.Run("a page after the last id is empty", func(t *testing.T) {
		page, err := audio.Page(ctx, ids[len(ids)-1], 2)
		require.NoError(t, err)
		assert.Empty(t, page)
	})

	t.Run("existing media item ids", func(t *testing.T) {
		got, err := postgres.ExistingMediaItemIDs(ctx, db, []string{ids[0], "gone-1", ids[3]})
		require.NoError(t, err)
		assert.Equal(t, map[string]struct{}{ids[0]: {}, ids[3]: {}}, got)

		none, err := postgres.ExistingMediaItemIDs(ctx, db, nil)
		require.NoError(t, err)
		assert.Empty(t, none)
	})

	t.Run("no id deletes nothing", func(t *testing.T) {
		require.NoError(t, audio.DeleteIDs(ctx, nil))
		require.NoError(t, visual.DeleteIDs(ctx, []string{}))
		page, err := audio.Page(ctx, "", 10)
		require.NoError(t, err)
		assert.Len(t, page, 5)
	})

	t.Run("a delete rolled back with its transaction leaves the rows", func(t *testing.T) {
		boom := errors.New("the move failed after the delete")
		err := postgres.NewTxUnit(db).Do(ctx, func(ctx context.Context) error {
			if err := audio.DeleteIDs(ctx, ids[:2]); err != nil {
				return err
			}
			if err := visual.DeleteIDs(ctx, ids[:2]); err != nil {
				return err
			}
			inTx, err := audio.Page(ctx, "", 10)
			if err != nil {
				return err
			}
			assert.Len(t, inTx, 3, "the page reads through the same transaction")
			return boom
		})
		require.ErrorIs(t, err, boom)
		a, err := audio.Page(ctx, "", 10)
		require.NoError(t, err)
		v, err := visual.Page(ctx, "", 10)
		require.NoError(t, err)
		assert.Len(t, a, 5)
		assert.Len(t, v, 5)
	})

	t.Run("deletes the ids given and only them", func(t *testing.T) {
		require.NoError(t, audio.DeleteIDs(ctx, []string{ids[1], ids[4], "never-there"}))
		require.NoError(t, visual.DeleteIDs(ctx, []string{ids[0]}))
		a, err := audio.Page(ctx, "", 10)
		require.NoError(t, err)
		v, err := visual.Page(ctx, "", 10)
		require.NoError(t, err)
		var left []string
		for _, e := range a {
			left = append(left, e.MediaItemID)
		}
		assert.Equal(t, []string{ids[0], ids[2], ids[3]}, left)
		assert.Len(t, v, 4)
	})
}
