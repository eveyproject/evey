package metadata_test

import (
	"context"
	"testing"

	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	"github.com/gravity-zero/evey/internal/domain/metadata"
	metafakes "github.com/gravity-zero/evey/internal/domain/metadata/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
)

func TestSearchMetadata(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	downloader := metafakes.NewFakeArtworkDownloader()
	bus := sharedfakes.NewFakeEventBus()

	provider := metafakes.NewFakeMetadataProvider("tmdb")
	provider.SearchResults["matrix"] = []metadata.MetadataResult{
		{Title: "The Matrix", Overview: "A computer hacker..."},
	}

	svc := appmetadata.NewService(
		overrideRepo, artworkRepo, downloader, bus,
		[]metadata.MetadataProvider{provider},
	)

	results, err := svc.SearchMetadata(context.Background(), "matrix", "movie")
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "The Matrix", results[0].Title)
}
