package metadata_test

import (
	"context"
	"testing"

	appmetadata "github.com/eveyproject/evey/internal/app/metadata"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	domainmetadata "github.com/eveyproject/evey/internal/domain/metadata"
	metafakes "github.com/eveyproject/evey/internal/domain/metadata/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seriesFixture builds a series with `seasons` seasons of `episodesPer`
// episodes each, wired into mediaRepo, and returns the series ID.
func seriesFixture(t *testing.T, ctx context.Context, mediaRepo *libfakes.FakeMediaItemStore, seasons, episodesPer int) string {
	t.Helper()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: shared.MediaItemID("s1"), Type: library.Series, Title: "Old Series"}))
	epNum := 0
	for si := range seasons {
		seasonID := "se" + string(rune('1'+si))
		require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
			ID: shared.MediaItemID(seasonID), ParentID: "s1", Type: library.Season, Title: "Season",
		}))
		for range episodesPer {
			epNum++
			epID := "e" + string(rune('0'+epNum))
			require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
				ID: shared.MediaItemID(epID), ParentID: seasonID, Type: library.Episode,
				File: &library.MediaFile{Path: "/media/Show/S0" + string(rune('1'+si)) + "/ep.mkv"},
			}))
		}
	}
	return "s1"
}

// TestResetItemMetadata_BatchesDeletesPerLevel is the N+1 regression test:
// before batching, resetting a series with S seasons x E episodes issued one
// DELETE triplet (overrides/artwork/persons) PER ITEM - 1 (series) + S
// (seasons) + S*E (episodes) single-item calls per repo. After batching, only
// the series item itself (reset directly, not via a list) uses the
// single-item call; every list-derived level (seasons, then each season's
// episodes) is deleted in one batch call per type.
func TestResetItemMetadata_BatchesDeletesPerLevel(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	personRepo := libfakes.NewFakeMediaPersonRepository()
	cmdSvc := appmetadata.NewCommandService(
		overrideRepo, artworkRepo, metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(), nil,
	)
	mediaRepo := libfakes.NewFakeMediaItemStore()
	appmetadata.WithEnricher(&fakeTranslationsEnricher{}, mediaRepo, mediaRepo)(cmdSvc)
	appmetadata.WithMediaPersons(personRepo)(cmdSvc)

	ctx := context.Background()
	const seasons, episodesPer = 2, 2
	seriesID := seriesFixture(t, ctx, mediaRepo, seasons, episodesPer)

	require.NoError(t, cmdSvc.ResetItemMetadata(ctx, seriesID))

	// Single-item delete: only the series itself (reset directly by
	// ResetItemMetadata, not through a List). Before this fix, every season
	// and episode also went through this path - here that would be
	// 1 + seasons + seasons*episodesPer = 7 calls.
	assert.Equal(t, 1, overrideRepo.DeleteByMediaItemIDCalls, "series item should use the single-item delete")
	assert.Equal(t, 1, artworkRepo.DeleteByMediaItemIDCalls)
	assert.Equal(t, 1, personRepo.DeleteByMediaItemIDCalls)

	// Batch delete: one call for the season level (all seasons of the
	// series) + one call per season for its episode level.
	// (seasons + 1 recursions) = 3 for seasons=2.
	wantBatchCalls := seasons + 1
	assert.Equal(t, wantBatchCalls, overrideRepo.DeleteByMediaItemIDsCalls, "seasons + one batch per season's episodes")
	assert.Equal(t, wantBatchCalls, artworkRepo.DeleteByMediaItemIDsCalls)
	assert.Equal(t, wantBatchCalls, personRepo.DeleteByMediaItemIDsCalls)

	// Total DELETE-type calls per repo (single + batch), vs the pre-fix
	// per-item count of 1 + seasons + seasons*episodesPer = 7.
	totalCalls := overrideRepo.DeleteByMediaItemIDCalls + overrideRepo.DeleteByMediaItemIDsCalls
	assert.Less(t, totalCalls, 1+seasons+seasons*episodesPer, "batched call count must be lower than the old per-item count")
	assert.Equal(t, 4, totalCalls)
}

// TestResetChildren_DeletesOrderedOverridesArtworkPersons asserts the safety
// invariant survives batching: for every level (and for the top item's own
// single-item reset), overrides are dropped before artwork, which is dropped
// before media persons - so a failure mid-run always leaves the safe
// half-state (fields still populated, overrides already gone) rather than the
// unsafe one (fields cleared, overrides still applying on next enrichment).
func TestResetChildren_DeletesOrderedOverridesArtworkPersons(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	personRepo := libfakes.NewFakeMediaPersonRepository()

	var callLog []string
	overrideRepo.CallLog = &callLog
	artworkRepo.CallLog = &callLog
	personRepo.CallLog = &callLog

	cmdSvc := appmetadata.NewCommandService(
		overrideRepo, artworkRepo, metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(), nil,
	)
	mediaRepo := libfakes.NewFakeMediaItemStore()
	appmetadata.WithEnricher(&fakeTranslationsEnricher{}, mediaRepo, mediaRepo)(cmdSvc)
	appmetadata.WithMediaPersons(personRepo)(cmdSvc)

	ctx := context.Background()
	seriesID := seriesFixture(t, ctx, mediaRepo, 2, 2)

	require.NoError(t, cmdSvc.ResetItemMetadata(ctx, seriesID))

	require.NotEmpty(t, callLog)
	require.Zero(t, len(callLog)%3, "delete calls must come in complete overrides/artwork/persons triplets")
	for i := 0; i < len(callLog); i += 3 {
		assert.Equal(t, []string{"overrides", "artwork", "persons"}, callLog[i:i+3],
			"triplet at index %d must be ordered overrides -> artwork -> persons", i)
	}
}

// A batched delete failing partway through a level must still abort the
// whole reset and report the error - same contract as the single-item path.
func TestResetChildren_BatchDeleteFailureAborts(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	cmdSvc := appmetadata.NewCommandService(
		overrideRepo, artworkRepo, metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(), nil,
	)
	mediaRepo := libfakes.NewFakeMediaItemStore()
	appmetadata.WithEnricher(&fakeTranslationsEnricher{}, mediaRepo, mediaRepo)(cmdSvc)

	ctx := context.Background()
	seriesID := seriesFixture(t, ctx, mediaRepo, 1, 2)

	// Only the batch delete fails, so the series' own single-item reset
	// (which runs first, before resetChildren) still succeeds - the failure
	// lands squarely inside the season-level batch.
	artworkRepo.ErrDeleteByMediaItemIDs = errBatchDeleteFailed

	err := cmdSvc.ResetItemMetadata(ctx, seriesID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete artwork for children")

	// The season's fields must be untouched: the batch failed before any
	// per-item Update ran for this level.
	season, getErr := mediaRepo.GetByID(ctx, "se1")
	require.NoError(t, getErr)
	assert.Equal(t, "Season", season.Title)
}

var errBatchDeleteFailed = &staticErr{"artwork batch delete failed"}

type staticErr struct{ msg string }

func (e *staticErr) Error() string { return e.msg }

// GetOverride is the hop the gRPC handler uses to find WHICH media item an
// override targets before gating Remove on that item's library.
func TestGetOverride_ResolvesTheTargetItem(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	cmdSvc := appmetadata.NewCommandService(
		overrideRepo, metafakes.NewFakeArtworkRepository(), metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(), nil,
	)
	ctx := context.Background()
	ov, err := domainmetadata.NewMetadataOverride("ov-1", "mi-1", "title",
		domainmetadata.WithOverrideValue("Corrected"), domainmetadata.WithOverrideEditor("adm-1"))
	require.NoError(t, err)
	require.NoError(t, cmdSvc.SetOverride(ctx, ov))

	got, err := cmdSvc.GetOverride(ctx, ov.ID)
	require.NoError(t, err)
	assert.Equal(t, shared.MediaItemID("mi-1"), got.MediaItemID)
	assert.Equal(t, "title", got.FieldName)

	_, err = cmdSvc.GetOverride(ctx, "ghost")
	assert.Error(t, err, "an unknown override must not resolve to a nil target the caller then dereferences")
}
