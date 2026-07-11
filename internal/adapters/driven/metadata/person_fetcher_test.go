package metadata

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestTMDBPersonFetcher_GetPersonDetails_Success(t *testing.T) {
	search := tmdbPersonSearchResult{}
	search.Results = append(search.Results, struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}{ID: 287, Name: "Brad Pitt"})
	detail := tmdbPersonDetail{
		ID:                 287,
		Name:               "Brad Pitt",
		Biography:          "William Bradley Pitt…",
		Birthday:           "1963-12-18",
		PlaceOfBirth:       "Shawnee, Oklahoma, USA",
		KnownForDepartment: "Acting",
		ProfilePath:        "/abcde.jpg",
	}

	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/person"):
			_ = json.NewEncoder(w).Encode(search)
		case strings.HasPrefix(r.URL.Path, "/person/287"):
			_ = json.NewEncoder(w).Encode(detail)
		default:
			http.NotFound(w, r)
		}
	})
	defer srv.Close()

	f := NewTMDBPersonFetcher(p)
	got, err := f.GetPersonDetails(context.Background(), "Brad Pitt")
	if err != nil {
		t.Fatalf("GetPersonDetails: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.Biography != "William Bradley Pitt…" {
		t.Errorf("Biography = %q", got.Biography)
	}
	if got.Birthday != "1963-12-18" {
		t.Errorf("Birthday = %q", got.Birthday)
	}
	if got.KnownFor != "Acting" {
		t.Errorf("KnownFor = %q", got.KnownFor)
	}
	if got.ThumbURL == "" {
		t.Error("ThumbURL should be set when profile_path present")
	}
}

func TestTMDBPersonFetcher_GetPersonDetails_NoMatch(t *testing.T) {
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tmdbPersonSearchResult{})
	})
	defer srv.Close()

	f := NewTMDBPersonFetcher(p)
	got, err := f.GetPersonDetails(context.Background(), "Nobody Nowhere")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil when search empty, got %+v", got)
	}
}

func TestTMDBPersonFetcher_FetchPersonTranslations_Success(t *testing.T) {
	search := tmdbPersonSearchResult{}
	search.Results = append(search.Results, struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}{ID: 500, Name: "Actor"})

	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/person"):
			_ = json.NewEncoder(w).Encode(search)
		case strings.HasPrefix(r.URL.Path, "/person/500"):
			locale := r.URL.Query().Get("language")
			_ = json.NewEncoder(w).Encode(tmdbPersonDetail{Biography: "bio-" + locale})
		}
	})
	defer srv.Close()

	f := NewTMDBPersonFetcher(p)
	out, err := f.FetchPersonTranslations(context.Background(), "Actor", []string{"en", "fr", "de"})
	if err != nil {
		t.Fatalf("FetchPersonTranslations: %v", err)
	}
	// Every requested locale is fetched, including the base language (en) -
	// the overlay applies a bio row for the default locale too.
	if out["fr"] != "bio-fr" {
		t.Errorf("fr = %q, want bio-fr", out["fr"])
	}
	if out["de"] != "bio-de" {
		t.Errorf("de = %q, want bio-de", out["de"])
	}
	if out["en"] != "bio-en" {
		t.Errorf("en = %q, want bio-en (base language no longer skipped)", out["en"])
	}
}

func TestTMDBPersonFetcher_FetchPersonTranslations_NoMatch(t *testing.T) {
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tmdbPersonSearchResult{})
	})
	defer srv.Close()

	f := NewTMDBPersonFetcher(p)
	out, err := f.FetchPersonTranslations(context.Background(), "Unknown", []string{"fr"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil map when search empty, got %v", out)
	}
}
