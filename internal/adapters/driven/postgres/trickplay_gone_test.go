package postgres_test

// The manifest write must classify a deleted-item foreign-key violation as
// library.ErrMediaItemGone, so the trickplay worker can treat it as a skip
// instead of a scary "generation failed" (prod, 2026-09-02).

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

func TestTrickplayRepo_CreateForDeletedItem_IsClassifiedGone(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewTrickplayRepository(db)
	// No media_items row with this id exists, so the FK on media_item_id fails.
	err := repo.Create(context.Background(), &library.TrickplayManifest{
		ID:          "tp-gone",
		MediaItemID: shared.MediaItemID("does-not-exist"),
		Interval:    15,
		Width:       320,
		TileWidth:   10,
		TileHeight:  10,
		ImageCount:  1,
		ImagePath:   "/cache/x",
		AlgoVersion: 1,
		CreatedAt:   time.Now(),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, library.ErrMediaItemGone,
		"a deleted-item FK violation must be a classifiable skip, not a raw pq error")
}
