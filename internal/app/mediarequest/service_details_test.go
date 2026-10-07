package mediarequest_test

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	appmediarequest "github.com/evey-project/evey/internal/app/mediarequest"
	"github.com/evey-project/evey/internal/domain/mediarequest"
	mrfakes "github.com/evey-project/evey/internal/domain/mediarequest/fakes"
	"github.com/evey-project/evey/internal/domain/metadata"
)

// The synopsis, release date and rating ride along with the request so the
// operator can judge it without a browser. They come from the client, so the
// service keeps them within bounds rather than trusting them.
func TestCreateRequest_KeepsOverviewReleaseDateAndRating(t *testing.T) {
	repo := mrfakes.NewFakeRepository()
	svc := newService(repo, mrfakes.NewFakeAccess(true), &fakeBus{})
	release := time.Date(1999, 3, 31, 0, 0, 0, 0, time.UTC)
	rating := 8.7

	req, err := svc.CreateRequest(context.Background(), "prf-1", "usr-1", appmediarequest.CreateInput{
		MediaType: mediarequest.MediaTypeMovie, Provider: "tmdb", ProviderID: "603", Title: "Some Movie",
		Overview: "  A hacker learns the truth.  ", ReleaseDate: &release, Rating: &rating,
	})
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if req.Overview != "A hacker learns the truth." {
		t.Errorf("overview not trimmed: %q", req.Overview)
	}
	if req.ReleaseDate == nil || !req.ReleaseDate.Equal(release) {
		t.Errorf("release date lost: %v", req.ReleaseDate)
	}
	if req.Rating == nil || *req.Rating != 8.7 {
		t.Errorf("rating lost: %v", req.Rating)
	}
}

func TestCreateRequest_BoundsClientSuppliedDetails(t *testing.T) {
	repo := mrfakes.NewFakeRepository()
	svc := newService(repo, mrfakes.NewFakeAccess(true), &fakeBus{})

	tooHigh := 11.0
	zero := 0.0
	long := strings.Repeat("é", 2500) // multi-byte on purpose: the cut must land on a rune boundary
	req, err := svc.CreateRequest(context.Background(), "prf-1", "usr-1", appmediarequest.CreateInput{
		MediaType: mediarequest.MediaTypeMovie, Title: "Long", Overview: long, Rating: &tooHigh,
	})
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if got := utf8.RuneCountInString(req.Overview); got != 2000 {
		t.Errorf("overview should be cut to 2000 runes, got %d", got)
	}
	if !utf8.ValidString(req.Overview) {
		t.Error("overview cut inside a character")
	}
	if req.Rating != nil {
		t.Errorf("an out-of-scale rating must be dropped, got %v", *req.Rating)
	}

	req, err = svc.CreateRequest(context.Background(), "prf-1", "usr-1", appmediarequest.CreateInput{
		MediaType: mediarequest.MediaTypeTV, Title: "Unrated", Rating: &zero,
	})
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if req.Rating != nil {
		t.Error("zero means unrated and must not be kept as a score")
	}
	if req.ReleaseDate != nil {
		t.Error("no release date given, none expected")
	}
}

// The search result carries the same three facts, straight from the provider.
func TestSearchTitles_CarriesReleaseDateAndRating(t *testing.T) {
	premiere := time.Date(2010, 7, 16, 0, 0, 0, 0, time.UTC)
	score := 8.4
	provider := &detailsProvider{results: []metadata.MetadataResult{{
		Title: "Dreams", Overview: "A heist inside a dream.", PremiereDate: &premiere, CommunityRating: &score,
		ProviderIDs: map[string]string{"tmdb": "27205"},
	}}}
	svc := appmediarequest.NewService(mrfakes.NewFakeRepository(), mrfakes.NewFakeAccess(true), []metadata.MetadataProvider{provider}, &fakeBus{})

	cands, err := svc.SearchTitles(context.Background(), "usr-1", mediarequest.MediaTypeMovie, "dreams", nil)
	if err != nil {
		t.Fatalf("SearchTitles: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("want 1 candidate, got %d", len(cands))
	}
	c := cands[0]
	if c.Overview != "A heist inside a dream." {
		t.Errorf("overview: %q", c.Overview)
	}
	if c.ReleaseDate == nil || !c.ReleaseDate.Equal(premiere) {
		t.Errorf("release date: %v", c.ReleaseDate)
	}
	if c.Rating == nil || *c.Rating != 8.4 {
		t.Errorf("rating: %v", c.Rating)
	}
}

type detailsProvider struct{ results []metadata.MetadataResult }

func (p *detailsProvider) Name() string { return "tmdb" }
func (p *detailsProvider) Search(context.Context, string, *int, string) ([]metadata.MetadataResult, error) {
	return p.results, nil
}
func (p *detailsProvider) GetByProviderID(context.Context, string, string) (*metadata.MetadataResult, error) {
	return nil, nil
}
