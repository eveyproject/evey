package grpcapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/playback"
	"github.com/evey-project/evey/internal/domain/shared"
)

type fakeArtworkBatch struct {
	records map[string][]*metadata.ArtworkRecord
	err     error
	calls   int
}

func (f *fakeArtworkBatch) BatchGetArtwork(_ context.Context, _ []string) (map[string][]*metadata.ArtworkRecord, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.records, nil
}

func TestFillPrimaryBlurhashes_FirstPosterWins_NonPosterSkipped(t *testing.T) {
	svc := &fakeArtworkBatch{records: map[string][]*metadata.ArtworkRecord{
		"mi-1": {
			{Type: "backdrop", Blurhash: new("HASH-backdrop")},
			{Type: "poster", Blurhash: nil},
			{Type: "poster", Blurhash: new("HASH-poster-1")},
			{Type: "poster", Blurhash: new("HASH-poster-2")},
		},
	}}
	items := []*eveyv1.MediaItem{{Id: "mi-1"}, {Id: "mi-2"}, nil}

	fillPrimaryBlurhashes(context.Background(), svc, items)

	require.Len(t, items[0].Artwork, 1,
		"exactly one poster record is attached; backdrops and hashless posters are skipped")
	assert.Equal(t, eveyv1.ArtworkType_ARTWORK_TYPE_PRIMARY, items[0].Artwork[0].Type)
	assert.Equal(t, "HASH-poster-1", items[0].Artwork[0].Blurhash,
		"first poster CARRYING a hash wins; a backdrop hash must never be used")
	assert.Empty(t, items[1].Artwork)
}

func TestFillPrimaryBlurhashes_BestEffortOnFailureAndNilService(t *testing.T) {
	items := []*eveyv1.MediaItem{{Id: "mi-1"}}
	fillPrimaryBlurhashes(context.Background(), &fakeArtworkBatch{err: errors.New("db down")}, items)
	assert.Empty(t, items[0].Artwork, "a fetch failure degrades to no placeholder, never an error")

	fillPrimaryBlurhashes(context.Background(), nil, items) // must not panic
	assert.Empty(t, items[0].Artwork)
}

func TestFillPrimaryBlurhashes_LeavesExistingArtworkAlone(t *testing.T) {
	svc := &fakeArtworkBatch{records: map[string][]*metadata.ArtworkRecord{
		"mi-1": {{Type: "poster", Blurhash: new("HASH-new")}},
	}}
	items := []*eveyv1.MediaItem{{Id: "mi-1", Artwork: []*eveyv1.Artwork{{Id: "art-existing"}}}}

	fillPrimaryBlurhashes(context.Background(), svc, items)

	require.Len(t, items[0].Artwork, 1, "items that already ship artwork are not re-fetched")
	assert.Equal(t, "art-existing", items[0].Artwork[0].Id)
	assert.Equal(t, 0, svc.calls)
}

// The wiring test: resume and next-up responses carry the poster blurhash so
// home-row cards paint the same placeholder as feed rows.
func TestPlaybackHandler_ResumeItems_CarryPrimaryBlurhash(t *testing.T) {
	h, progressRepo, mediaQueryRepo, _ := newTestPlaybackHandlerFull()
	mediaQueryRepo.Seed(&library.MediaItem{ID: "mi-bh", LibraryID: "lib-1", Type: "movie", Title: "Some Movie"})
	require.NoError(t, progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-bh", ProfileID: "profile-2", MediaItemID: "mi-bh",
		Position: mustPlaybackDurationMs(t, 10000), LastPlayedAt: time.Now(),
	}))
	svc := &fakeArtworkBatch{records: map[string][]*metadata.ArtworkRecord{
		"mi-bh": {{Type: "poster", MediaItemID: shared.MediaItemID("mi-bh"), Blurhash: new("HASH-resume")}},
	}}
	h.metadataSvc = svc

	resp, err := h.GetResumeItems(pbUserCtx("user-2", "profile-2"), &eveyv1.GetResumeItemsRequest{Limit: 10})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Len(t, resp.Items[0].Artwork, 1)
	assert.Equal(t, "HASH-resume", resp.Items[0].Artwork[0].Blurhash)
	assert.Equal(t, 1, svc.calls, "one batched fetch per response, never one per card")
}
