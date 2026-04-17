package metadata

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestTMDB_FetchTranslations_MovieSkipsBaseLanguage(t *testing.T) {
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
	if _, ok := out["en"]; ok {
		t.Error("en (base language) should be skipped")
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
