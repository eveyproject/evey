package metadata_test

import (
	"context"
	"sync"
	"testing"
	"time"

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

// recordingProvider logs how the enricher reaches a provider: by a title
// search (and with which year) or straight by a provider id.
type recordingProvider struct {
	*metafakes.FakeMetadataProvider
	mu       sync.Mutex
	searches []recordedSearch
	byID     []string
	searched chan struct{}
}

type recordedSearch struct {
	query string
	year  *int
}

func (p *recordingProvider) Search(ctx context.Context, query string, year *int, mediaType string) ([]domainmetadata.MetadataResult, error) {
	p.mu.Lock()
	p.searches = append(p.searches, recordedSearch{query: query, year: year})
	first := len(p.searches) == 1
	p.mu.Unlock()
	if first {
		close(p.searched)
	}
	return p.FakeMetadataProvider.Search(ctx, query, year, mediaType)
}

func (p *recordingProvider) GetByProviderID(ctx context.Context, id, mediaType string) (*domainmetadata.MetadataResult, error) {
	p.mu.Lock()
	p.byID = append(p.byID, id)
	p.mu.Unlock()
	return p.FakeMetadataProvider.GetByProviderID(ctx, id, mediaType)
}

func (p *recordingProvider) LookupByExternalID(ctx context.Context, providerName, externalID string) (*domainmetadata.MetadataResult, error) {
	p.mu.Lock()
	p.byID = append(p.byID, providerName+":"+externalID)
	p.mu.Unlock()
	return p.FakeMetadataProvider.LookupByExternalID(ctx, providerName, externalID)
}

// A reset exists to undo a wrong match, so the re-match it triggers must
// start from the title like a first scan, never from the provider ids the
// wrong match left behind: those would bring the same record straight back.
func TestResetItemMetadata_RematchesByTitleNotStoredIDs(t *testing.T) {
	wrongYear := 2020
	provider := &recordingProvider{FakeMetadataProvider: metafakes.NewFakeMetadataProvider("tmdb"), searched: make(chan struct{})}
	provider.ByProviderID["999"] = &domainmetadata.MetadataResult{
		Title: "Caminantes", Year: &wrongYear, ProviderIDs: map[string]string{"tmdb": "999", "imdb": "tt0000999"},
	}

	mediaRepo := libfakes.NewFakeMediaItemStore()
	enricher := appmetadata.NewEnricher(metafakes.NewFakeArtworkRepository(), libfakes.NewFakePersonRepository(),
		libfakes.NewFakeMediaPersonRepository(), mediaRepo, mediaRepo, libfakes.NewFakeSearchIndex(), provider, nil, nil, nil)
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(), metafakes.NewFakeArtworkRepository(), metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(), nil,
	)
	appmetadata.WithEnricher(enricher, mediaRepo, mediaRepo)(cmdSvc)

	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: shared.MediaItemID("m1"), Type: library.MovieItem,
		Title: "Caminantes", ScanTitle: "Caminandes Llama Drama", Overview: "wrong film",
		File:    &library.MediaFile{Path: "/media/Caminandes Llama Drama (2013).mkv"},
		Catalog: &library.MediaCatalog{Year: &wrongYear, ProviderIDs: map[string]string{"tmdb": "999", "imdb": "tt0000999"}},
	}))

	require.NoError(t, cmdSvc.ResetItemMetadata(ctx, "m1"))

	stored, err := mediaRepo.GetByID(ctx, "m1")
	require.NoError(t, err)
	assert.True(t, stored.Catalog == nil || len(stored.Catalog.ProviderIDs) == 0, "the reset clears the stored provider ids")

	select {
	case <-provider.searched:
	case <-time.After(5 * time.Second):
		t.Fatal("the reset never re-matched the item by a title search")
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	assert.Equal(t, "Caminandes Llama Drama", provider.searches[0].query, "the re-match searches the scan title")
	assert.Empty(t, provider.byID, "the re-match must not fetch the wrong record by its stored id")
}
