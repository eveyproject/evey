package playlist

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/playlist/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// A media removal, or a library deletion that cascades its media away, purges
// the playlist rows left pointing at nothing.
func TestSubscribeMediaRemovals_PurgesOrphans(t *testing.T) {
	items := fakes.NewFakePlaylistItemRepository()
	svc := NewCommandService(fakes.NewFakePlaylistRepository(), items)
	bus := sharedfakes.NewFakeEventBus()
	svc.SubscribeMediaRemovals(bus)

	_ = bus.Publish(context.Background(), library.MediaItemRemoved{MediaItemID: "m1"})
	_ = bus.Publish(context.Background(), library.LibraryDeleted{LibraryID: "l1"})

	if items.PurgeOrphansCalls() != 2 {
		t.Fatalf("orphan purges = %d, want one per removal event", items.PurgeOrphansCalls())
	}
}
