package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	"github.com/gravity-zero/evey/internal/domain/metadata"
	metafakes "github.com/gravity-zero/evey/internal/domain/metadata/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
)

func newTestMetadataHandler() (*metadataHandler, *metafakes.FakeMetadataProvider, *metafakes.FakeArtworkRepository, *metafakes.FakeMetadataOverrideRepository) {
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	artworkRepo := metafakes.NewFakeArtworkRepository()
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	events := sharedfakes.NewFakeEventBus()
	svc := appmetadata.NewService(
		overrideRepo,
		artworkRepo,
		metafakes.NewFakeArtworkDownloader(),
		events,
		[]metadata.MetadataProvider{provider},
	)
	return &metadataHandler{svc: svc}, provider, artworkRepo, overrideRepo
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
