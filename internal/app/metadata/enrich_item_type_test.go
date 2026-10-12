package metadata

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/library"
)

// The single-item enrich subscriber must only queue items that live in
// media_items. Since the 2026-08-26 audio/image ingestion work the music scan
// publishes MediaItemAdded{Type: Track} per track (and photos publish too):
// without this filter each one costs a losing GetByID and, on a bulk import,
// floods the bounded queue and drops the real video items behind it.
func TestEnrichableItemType(t *testing.T) {
	enrichable := []library.MediaItemType{
		library.MovieItem, library.Series, library.Season, library.Episode,
	}
	for _, ty := range enrichable {
		if !enrichableItemType(ty) {
			t.Errorf("type %q must be enrichable (it has a media_items row)", ty)
		}
	}

	skip := []library.MediaItemType{
		library.Track, library.Album, library.PhotoItem,
	}
	for _, ty := range skip {
		if enrichableItemType(ty) {
			t.Errorf("type %q must NOT be queued for enrichment (no media_items row)", ty)
		}
	}
}
