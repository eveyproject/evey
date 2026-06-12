package metadata

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// FetchTranslations must fetch EVERY requested locale, including the provider's
// default language. The read overlay applies a translation row for every locale
// (default included), so a missing default-language row left items whose base
// columns held the wrong language stuck — the synopsis-frozen-in-English bug.
func TestTMDB_FetchTranslations_IncludesBaseLanguage(t *testing.T) {
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/movie/42") {
			http.NotFound(w, r)
			return
		}
		locale := r.URL.Query().Get("language")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tmdbDetailResponse{
			Title:    "Title-" + locale,
			Overview: "Overview-" + locale,
			Tagline:  "Tag-" + locale,
		})
	})
	defer srv.Close()

	out, err := p.FetchTranslations(context.Background(), "42", "movie", []string{"en", "fr", "de"})
	if err != nil {
		t.Fatalf("FetchTranslations: %v", err)
	}
	if out["fr"].Title != "Title-fr" || out["fr"].Overview != "Overview-fr" || out["fr"].Tagline != "Tag-fr" {
		t.Errorf("fr = %+v", out["fr"])
	}
	if out["de"].Title != "Title-de" {
		t.Errorf("de = %+v", out["de"])
	}
	// "en" is the provider's default language (newTestServer) — it must now be
	// fetched too, not skipped.
	if out["en"].Overview != "Overview-en" {
		t.Errorf("base language en must be fetched, got %+v", out["en"])
	}
}

func TestTMDB_FetchTranslations_TVUsesNameField(t *testing.T) {
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/tv/100") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tmdbDetailResponse{Name: "Show-fr"})
	})
	defer srv.Close()

	out, err := p.FetchTranslations(context.Background(), "100", "tv", []string{"fr"})
	if err != nil {
		t.Fatalf("FetchTranslations: %v", err)
	}
	if out["fr"].Title != "Show-fr" {
		t.Errorf("TV title must come from Name field, got %+v", out["fr"])
	}
}

func TestTMDB_FetchTranslations_UnsupportedMediaType(t *testing.T) {
	p := NewTMDBProvider("k", "en", nil)
	_, err := p.FetchTranslations(context.Background(), "1", "book", []string{"fr"})
	if err == nil {
		t.Fatal("expected error for unsupported media type")
	}
}

// A foreign-language work whose per-locale title is the original-language one
// must get its regional alternative title in the translation row too — else
// the read overlay clobbers the (regional) base title.
func TestTMDB_FetchTranslations_RegionalTitleForForeignWork(t *testing.T) {
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/movie/77") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tmdbDetailResponse{
			Title:            "Deutscher Titel",
			OriginalTitle:    "Deutscher Titel",
			OriginalLanguage: "de",
			Overview:         "ov",
			AlternativeTitles: &tmdbAltTitles{Titles: []tmdbAltTitle{
				{Region: "FR", Title: "Titre français"},
			}},
		})
	})
	defer srv.Close()

	out, err := p.FetchTranslations(context.Background(), "77", "movie", []string{"fr"})
	if err != nil {
		t.Fatalf("FetchTranslations: %v", err)
	}
	if out["fr"].Title != "Titre français" {
		t.Errorf("fr translation should use regional title, got %q", out["fr"].Title)
	}
}
