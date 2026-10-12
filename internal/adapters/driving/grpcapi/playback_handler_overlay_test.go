package grpcapi

import (
	"context"
	"testing"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	appmetadata "github.com/eveyproject/evey/internal/app/metadata"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/metadata"
	metafakes "github.com/eveyproject/evey/internal/domain/metadata/fakes"
	"github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/domain/shared"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// overlayPlaybackHandler wires a playback handler whose library service knows
// a "fr" translation for two items and whose metadata service holds an admin
// title override for two items (one item carries both, the override must win).
func overlayPlaybackHandler(t *testing.T, completed bool) *playbackHandler {
	t.Helper()
	h, progressRepo, mediaQueryRepo, _ := newTestPlaybackHandlerFull()
	for _, id := range []string{"mi-tr", "mi-ov", "mi-both"} {
		mediaQueryRepo.Seed(&library.MediaItem{ID: shared.MediaItemID(id), LibraryID: "lib-1", Type: "movie", Title: "Some Movie"})
		require.NoError(t, progressRepo.Upsert(context.Background(), &playback.WatchProgress{
			ID: "wp-" + id, ProfileID: "profile-2", MediaItemID: shared.MediaItemID(id),
			Position: mustPlaybackDurationMs(t, 10000), Completed: completed, LastPlayedAt: time.Now(),
		}))
	}
	trans := &stubMediaTranslations{byID: map[string]*library.MediaItemTranslation{
		"mi-tr":   {MediaItemID: "mi-tr", Locale: "fr", Title: "Titre traduit"},
		"mi-both": {MediaItemID: "mi-both", Locale: "fr", Title: "Titre traduit auto"},
	}}
	h.librarySvc = applibrary.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(), mediaQueryRepo,
		libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex(),
		applibrary.WithTranslations(trans, nil),
	)
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	for id, title := range map[string]string{"mi-ov": "Titre corrige", "mi-both": "Titre admin"} {
		require.NoError(t, overrideRepo.Set(context.Background(), &metadata.MetadataOverride{
			ID: "mo-" + id, MediaItemID: shared.MediaItemID(id), FieldName: "title", CustomValue: title,
		}))
	}
	h.metadataSvc = appmetadata.NewQueryService(overrideRepo, metafakes.NewFakeArtworkRepository(), nil)
	return h
}

func wantOverlaidTitles() map[string]string {
	return map[string]string{
		"mi-tr":   "Titre traduit",
		"mi-ov":   "Titre corrige",
		"mi-both": "Titre admin",
	}
}

func TestPlaybackHandler_ResumeItems_AppliesLocaleAndOverrides(t *testing.T) {
	h := overlayPlaybackHandler(t, false)
	ctx := eveyi18n.ContextWithLocale(pbUserCtx("user-2", "profile-2"), "fr")

	resp, err := h.GetResumeItems(ctx, &eveyv1.GetResumeItemsRequest{Limit: 10})
	require.NoError(t, err)
	require.Len(t, resp.Items, 3)
	want := wantOverlaidTitles()
	for _, it := range resp.Items {
		assert.Equal(t, want[it.Id], it.Title, it.Id)
	}
	for _, p := range resp.Progress {
		assert.Equal(t, want[p.MediaItemId], p.Title, "progress row "+p.MediaItemId)
	}
}

func TestPlaybackHandler_NextUp_AppliesLocaleAndOverrides(t *testing.T) {
	h := overlayPlaybackHandler(t, true)
	ctx := eveyi18n.ContextWithLocale(pbUserCtx("user-2", "profile-2"), "fr")

	resp, err := h.GetNextUp(ctx, &eveyv1.GetNextUpRequest{Limit: 10})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Items, "fixture must yield next-up cards")
	want := wantOverlaidTitles()
	for _, it := range resp.Items {
		assert.Equal(t, want[it.Id], it.Title, it.Id)
	}
}

func TestPlaybackHandler_WatchHistory_AppliesLocaleAndOverrides(t *testing.T) {
	h := overlayPlaybackHandler(t, false)
	ctx := eveyi18n.ContextWithLocale(pbUserCtx("user-2", "profile-2"), "fr")

	resp, err := h.GetWatchHistory(ctx, &eveyv1.GetWatchHistoryRequest{Limit: 10})
	require.NoError(t, err)
	require.Len(t, resp.Items, 3)
	want := wantOverlaidTitles()
	for _, p := range resp.Items {
		assert.Equal(t, want[p.MediaItemId], p.Title, p.MediaItemId)
	}
}
