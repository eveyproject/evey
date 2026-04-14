package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	"github.com/evey-project/evey/internal/domain/metadata"
	metafakes "github.com/evey-project/evey/internal/domain/metadata/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
)

func newTestMetadataHandler() (*metadataHandler, *metafakes.FakeMetadataProvider, *metafakes.FakeArtworkRepository, *metafakes.FakeMetadataOverrideRepository) {
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	artworkRepo := metafakes.NewFakeArtworkRepository()
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	events := sharedfakes.NewFakeEventBus()
	providers := []metadata.MetadataProvider{provider}
	cmd := appmetadata.NewCommandService(overrideRepo, artworkRepo, metafakes.NewFakeArtworkDownloader(), events, providers)
	query := appmetadata.NewQueryService(overrideRepo, artworkRepo, providers)
	return &metadataHandler{cmd: cmd, query: query}, provider, artworkRepo, overrideRepo
}

func TestMetadataGetArtwork(t *testing.T) {
	h, _, artRepo, _ := newTestMetadataHandler()
	url := "http://example.com/poster.jpg"
	w, ht := 300, 450
	artRepo.Create(context.Background(), &metadata.ArtworkRecord{
		ID:          "art-1",
		MediaItemID: "mi-1",
		Type:        "poster",
		Provider:    "tmdb",
		SourceURL:   &url,
		Width:       &w,
		Height:      &ht,
	})

	resp, err := h.GetArtwork(context.Background(), &eveyv1.GetArtworkRequest{MediaItemId: "mi-1"})
	if err != nil {
		t.Fatalf("GetArtwork: %v", err)
	}
	if len(resp.Artwork) != 1 {
		t.Fatalf("expected 1 artwork, got %d", len(resp.Artwork))
	}
	if resp.Artwork[0].Url != url {
		t.Errorf("expected url %q, got %q", url, resp.Artwork[0].Url)
	}
}

func TestMetadataGetArtworkEmpty(t *testing.T) {
	h, _, _, _ := newTestMetadataHandler()
	resp, err := h.GetArtwork(context.Background(), &eveyv1.GetArtworkRequest{MediaItemId: "nonexistent"})
	if err != nil {
		t.Fatalf("GetArtwork: %v", err)
	}
	if len(resp.Artwork) != 0 {
		t.Errorf("expected 0 artwork, got %d", len(resp.Artwork))
	}
}

func TestMetadataSetMetadataOverride(t *testing.T) {
	h, _, _, _ := newTestMetadataHandler()
	resp, err := h.SetMetadataOverride(context.Background(), &eveyv1.SetMetadataOverrideRequest{
		MediaItemId: "mi-1",
		FieldName:   "title",
		CustomValue: "Custom Title",
	})
	if err != nil {
		t.Fatalf("SetMetadataOverride: %v", err)
	}
	if resp.MediaItemId != "mi-1" {
		t.Errorf("expected media item id mi-1, got %q", resp.MediaItemId)
	}
	if resp.FieldName != "title" {
		t.Errorf("expected field name title, got %q", resp.FieldName)
	}
	if resp.Id == "" {
		t.Error("expected non-empty override id")
	}
}

func TestMetadataSearchMetadata(t *testing.T) {
	h, provider, _, _ := newTestMetadataHandler()
	year := 2020
	rating := 8.5
	provider.SearchResults["batman"] = []metadata.MetadataResult{
		{Title: "Batman Begins", Year: &year, CommunityRating: &rating, Genres: []string{"Action"}},
		{Title: "The Batman", Year: &year},
	}

	resp, err := h.SearchMetadata(context.Background(), &eveyv1.SearchMetadataRequest{
		Query: "batman",
		Type:  eveyv1.MediaItemType_MEDIA_ITEM_TYPE_MOVIE,
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchMetadata: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
	if resp.Results[0].Title != "Batman Begins" {
		t.Errorf("expected title 'Batman Begins', got %q", resp.Results[0].Title)
	}
	if resp.Results[0].Rating != 8.5 {
		t.Errorf("expected rating 8.5, got %f", resp.Results[0].Rating)
	}
}

func TestMetadataSearchMetadata_EmptyResults(t *testing.T) {
	h, _, _, _ := newTestMetadataHandler()
	resp, err := h.SearchMetadata(context.Background(), &eveyv1.SearchMetadataRequest{
		Query: "nonexistent",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchMetadata: %v", err)
	}
	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(resp.Results))
	}
}

func TestMetadataSearchMetadata_WithLimit(t *testing.T) {
	h, provider, _, _ := newTestMetadataHandler()
	provider.SearchResults["test"] = []metadata.MetadataResult{
		{Title: "Result 1"}, {Title: "Result 2"}, {Title: "Result 3"},
	}

	resp, err := h.SearchMetadata(context.Background(), &eveyv1.SearchMetadataRequest{
		Query: "test",
		Limit: 2,
	})
	if err != nil {
		t.Fatalf("SearchMetadata: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results (limited), got %d", len(resp.Results))
	}
}

func TestMetadataRemoveMetadataOverride(t *testing.T) {
	h, _, _, overrideRepo := newTestMetadataHandler()
	overrideRepo.Set(context.Background(), &metadata.MetadataOverride{
		ID:          "mo-1",
		MediaItemID: "mi-1",
		FieldName:   "title",
		CustomValue: "Override",
	})

	resp, err := h.RemoveMetadataOverride(context.Background(), &eveyv1.RemoveMetadataOverrideRequest{Id: "mo-1"})
	if err != nil {
		t.Fatalf("RemoveMetadataOverride: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	list, _ := h.ListMetadataOverrides(context.Background(), &eveyv1.ListMetadataOverridesRequest{MediaItemId: "mi-1"})
	if len(list.Overrides) != 0 {
		t.Errorf("expected 0 overrides after removal, got %d", len(list.Overrides))
	}
}

func TestMetadataGetMetadataProviders(t *testing.T) {
	h, _, _, _ := newTestMetadataHandler()
	resp, err := h.GetMetadataProviders(context.Background(), &eveyv1.GetMetadataProvidersRequest{})
	if err != nil {
		t.Fatalf("GetMetadataProviders: %v", err)
	}
	if len(resp.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(resp.Providers))
	}
	if resp.Providers[0].Name != "tmdb" {
		t.Errorf("expected provider name 'tmdb', got %q", resp.Providers[0].Name)
	}
	if !resp.Providers[0].Enabled {
		t.Error("expected provider to be enabled")
	}
	if resp.Providers[0].Priority != 1 {
		t.Errorf("expected priority 1, got %d", resp.Providers[0].Priority)
	}
}

func TestMetadataListMetadataOverrides(t *testing.T) {
	h, _, _, overrideRepo := newTestMetadataHandler()
	overrideRepo.Set(context.Background(), &metadata.MetadataOverride{
		ID:          "mo-1",
		MediaItemID: "mi-1",
		FieldName:   "title",
		CustomValue: "Override",
	})
	overrideRepo.Set(context.Background(), &metadata.MetadataOverride{
		ID:          "mo-2",
		MediaItemID: "mi-1",
		FieldName:   "overview",
		CustomValue: "New overview",
	})

	resp, err := h.ListMetadataOverrides(context.Background(), &eveyv1.ListMetadataOverridesRequest{MediaItemId: "mi-1"})
	if err != nil {
		t.Fatalf("ListMetadataOverrides: %v", err)
	}
	if len(resp.Overrides) != 2 {
		t.Errorf("expected 2 overrides, got %d", len(resp.Overrides))
	}
}
