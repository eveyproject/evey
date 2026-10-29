package playlist

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/music"
	domain "github.com/eveyproject/evey/internal/domain/playlist"
	"github.com/eveyproject/evey/internal/domain/playlist/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// One removed media item or music track purges its own rows by id; a library
// deletion, whose cascade removes media without an event each, sweeps every
// orphan.
func TestSubscribeMediaRemovals_PurgesOrphans(t *testing.T) {
	ctx := context.Background()
	items := fakes.NewFakePlaylistItemRepository()
	for i, it := range []domain.PlaylistItem{
		{PlaylistID: "p1", MediaItemID: "m1", MediaType: domain.MediaTypeVideo},
		{PlaylistID: "p1", MediaItemID: "m2", MediaType: domain.MediaTypeVideo},
		{PlaylistID: "p1", MediaItemID: "t1", MediaType: domain.MediaTypeMusic},
	} {
		it.SortOrder = i
		_ = items.Add(ctx, &it)
	}
	svc := NewCommandService(fakes.NewFakePlaylistRepository(), items)
	bus := sharedfakes.NewFakeEventBus()
	svc.SubscribeMediaRemovals(bus)

	_ = bus.Publish(ctx, library.MediaItemRemoved{MediaItemID: "m1"})
	_ = bus.Publish(ctx, music.TrackDeleted{TrackID: "t1"})
	left, _ := items.GetByPlaylistID(ctx, "p1")
	if len(left) != 1 || left[0].MediaItemID != "m2" {
		t.Fatalf("left = %+v, want only m2: the removed media item and track go by id", left)
	}
	if items.PurgeOrphansCalls() != 0 {
		t.Fatal("a single removal must not sweep the whole table")
	}

	_ = bus.Publish(ctx, library.LibraryDeleted{LibraryID: "l1"})
	if items.PurgeOrphansCalls() != 1 {
		t.Fatalf("orphan sweeps = %d, want one for the library deletion", items.PurgeOrphansCalls())
	}
}
