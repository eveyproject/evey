package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/metadata"
	metafakes "github.com/evey-project/evey/internal/domain/metadata/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestMetadataHandler() (*metadataHandler, *metafakes.FakeMetadataProvider, *metafakes.FakeArtworkRepository, *metafakes.FakeMetadataOverrideRepository) {
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	artworkRepo := metafakes.NewFakeArtworkRepository()
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	events := sharedfakes.NewFakeEventBus()
	providers := []metadata.MetadataProvider{provider}
	cmd := appmetadata.NewCommandService(overrideRepo, artworkRepo, metafakes.NewFakeArtworkDownloader(), events, providers)
	query := appmetadata.NewQueryService(overrideRepo, artworkRepo, providers)
	return &metadataHandler{cmd: cmd, query: query, userAccess: newPermissiveTestUserAccess()}, provider, artworkRepo, overrideRepo
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
		{Title: "Some Movie Begins", Year: &year, CommunityRating: &rating, Genres: []string{"Action"}},
		{Title: "The Some Movie", Year: &year},
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
	if resp.Results[0].Title != "Some Movie Begins" {
		t.Errorf("expected title 'Some Movie Begins', got %q", resp.Results[0].Title)
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

func TestMetadataResearchMetadata_MissingID(t *testing.T) {
	h, _, _, _ := newTestMetadataHandler()
	_, err := h.ResearchMetadata(context.Background(), &eveyv1.ResearchMetadataRequest{MediaItemId: ""})
	if err == nil {
		t.Fatal("expected error for empty media_item_id")
	}
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", got)
	}
}

func TestMetadataResearchMetadata_DropsOverridesAndResets(t *testing.T) {
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	artworkRepo := metafakes.NewFakeArtworkRepository()
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	events := sharedfakes.NewFakeEventBus()
	mediaRepo := libfakes.NewFakeMediaItemStore()
	providers := []metadata.MetadataProvider{provider}
	cmd := appmetadata.NewCommandService(overrideRepo, artworkRepo, metafakes.NewFakeArtworkDownloader(), events, providers)
	appmetadata.WithEnricher(&noopEnricher{}, mediaRepo, mediaRepo)(cmd)
	query := appmetadata.NewQueryService(overrideRepo, artworkRepo, providers)
	h := &metadataHandler{cmd: cmd, query: query, userAccess: newPermissiveTestUserAccess()}

	ctx := context.Background()
	if err := mediaRepo.Create(ctx, &library.MediaItem{
		ID:        "mi-1",
		Type:      library.MovieItem,
		Title:     "Wrong Match",
		ScanTitle: "Real",
		Catalog:   &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "wrong"}},
	}); err != nil {
		t.Fatalf("create item: %v", err)
	}
	if err := overrideRepo.Set(ctx, &metadata.MetadataOverride{ID: "ov-1", MediaItemID: "mi-1", FieldName: "title", CustomValue: "x"}); err != nil {
		t.Fatalf("set override: %v", err)
	}

	resp, err := h.ResearchMetadata(ctx, &eveyv1.ResearchMetadataRequest{MediaItemId: "mi-1"})
	if err != nil {
		t.Fatalf("ResearchMetadata: %v", err)
	}
	if resp.MediaItemId != "mi-1" {
		t.Errorf("expected media_item_id mi-1, got %q", resp.MediaItemId)
	}
	if resp.NewTitle != "Real" {
		t.Errorf("expected NewTitle 'Real' (reset to ScanTitle), got %q", resp.NewTitle)
	}
	if resp.Matched {
		t.Errorf("noop enricher does not write a new match → Matched should be false")
	}

	ovs, _ := overrideRepo.GetByMediaItem(ctx, "mi-1")
	if len(ovs) != 0 {
		t.Errorf("expected overrides cleared, got %d", len(ovs))
	}
}

// noopEnricher satisfies ItemEnricher without doing any work - used to
// verify ResearchMetadata's reset half independently of search behaviour.
type noopEnricher struct{}

func (noopEnricher) EnrichItem(_ context.Context, _ *library.MediaItem, _ *metadata.MetadataResult) error {
	return nil
}
func (noopEnricher) EnrichTranslationsForItem(_ context.Context, _ *library.MediaItem, _, _ string) {
}

// newTestMetadataHandlerWithAccess builds a metadataHandler wired with the
// item->library access check (userAccess + libQuery), mirroring the production
// wiring in server.go:171. Seeds one media item "mi-1" belonging to lib-1.
func newTestMetadataHandlerWithAccess() (*metadataHandler, *userfakes.FakeLibraryAccessRepository) {
	h, _, _, _ := newTestMetadataHandler()

	mediaQuery := libfakes.NewFakeMediaItemQueryRepo()
	mediaQuery.Seed(&library.MediaItem{
		ID:        "mi-1",
		LibraryID: "lib-1",
		Title:     "Movie",
		Type:      library.MovieItem,
	})
	h.libQuery = applibrary.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		mediaQuery,
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
	)

	libAccessRepo := userfakes.NewFakeLibraryAccessRepository()
	h.userAccess = appuser.NewAccessService(
		userfakes.NewFakeUserRepository(),
		userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(),
		sharedfakes.NewFakeEventBus(),
		appuser.WithLibraryAccess(libAccessRepo),
	)
	return h, libAccessRepo
}

// IDOR: a non-admin without access to the media item's library must be denied
// with NotFound (anti-enumeration: same code as "item doesn't exist").
func TestMetadataCheckMediaItemAccess_IDOR_DeniesUnauthorizedLibrary(t *testing.T) {
	h, libAccessRepo := newTestMetadataHandlerWithAccess()
	libAccessRepo.Grant(context.Background(), "user-1", "lib-2") // not lib-1

	err := h.checkMediaItemAccess(nonAdminCtx("user-1"), "mi-1")
	if err == nil {
		t.Fatal("expected NotFound for an unauthorized library")
	}
	if got := status.Code(err); got != codes.NotFound {
		t.Errorf("expected NotFound, got %v", got)
	}
}

// A non-admin WITH access to the item's library is allowed through.
func TestMetadataCheckMediaItemAccess_IDOR_AllowsAuthorizedLibrary(t *testing.T) {
	h, libAccessRepo := newTestMetadataHandlerWithAccess()
	libAccessRepo.Grant(context.Background(), "user-1", "lib-1")

	if err := h.checkMediaItemAccess(nonAdminCtx("user-1"), "mi-1"); err != nil {
		t.Fatalf("expected nil error for authorized library, got %v", err)
	}
}

// Admins bypass the access check entirely (no library grants needed).
func TestMetadataCheckMediaItemAccess_AdminBypassesAccessCheck(t *testing.T) {
	h, _ := newTestMetadataHandlerWithAccess()

	if err := h.checkMediaItemAccess(adminCtx(), "mi-1"); err != nil {
		t.Fatalf("expected nil error for admin, got %v", err)
	}
}

// A media item id that doesn't resolve is denied for non-admins with the same
// NotFound code as "no access", keeping item existence opaque to unauthorized
// callers.
func TestMetadataCheckMediaItemAccess_IDOR_DeniesUnknownItem(t *testing.T) {
	h, libAccessRepo := newTestMetadataHandlerWithAccess()
	libAccessRepo.Grant(context.Background(), "user-1", "lib-1")

	err := h.checkMediaItemAccess(nonAdminCtx("user-1"), "nonexistent")
	if err == nil {
		t.Fatal("expected NotFound for an unknown media item id")
	}
	if got := status.Code(err); got != codes.NotFound {
		t.Errorf("expected NotFound, got %v", got)
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
