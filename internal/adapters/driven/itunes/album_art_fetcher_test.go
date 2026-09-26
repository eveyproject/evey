package itunes_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/itunes"
)

// Régression prod 2026-07-25 : le fetcher demandait limit=1 et prenait
// Results[0] SANS vérifier que l'album renvoyé était celui demandé. Vérifié en
// direct contre l'API iTunes réelle :
//
//	term="Neon Harbor From Nowhere"       -> "Mixed Signals (Deluxe Edition)"
//	term="Neon Harbor Orchard Essentials" -> "Mixed Signals"
//
// La recherche plein-texte d'iTunes retombe sur l'album le plus POPULAIRE de
// l'artiste quand le titre demandé ne matche pas. Les deux albums Neon Harbor de
// la bibliothèque du user affichaient donc tous les deux la pochette de Hybrid
// Theory. Une pochette absente est honnête ; une pochette fausse est un mensonge.

type fakeITunes struct {
	results []map[string]any
	artHits int
}

func (f *fakeITunes) serve() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": f.results})
	})
	mux.HandleFunc("/art/", func(w http.ResponseWriter, _ *http.Request) {
		f.artHits++
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0, 'j', 'p', 'g'})
	})
	return httptest.NewServer(mux)
}

func newFetcherAgainst(t *testing.T, srv *httptest.Server) *itunes.AlbumArtFetcher {
	t.Helper()
	f := itunes.NewAlbumArtFetcher(srv.Client())
	f.SetSearchEndpointForTest(srv.URL + "/search")
	return f
}

func album(artist, collection, art string) map[string]any {
	return map[string]any{"artistName": artist, "collectionName": collection, "artworkUrl100": art}
}

func TestFetchRejectsAResultForAnotherAlbum(t *testing.T) {
	fake := &fakeITunes{results: []map[string]any{
		// Ce que renvoie réellement iTunes pour "Neon Harbor From Nowhere".
		album("NEON HARBOR", "Mixed Signals (Deluxe Edition)", "https://x/100x100bb.jpg"),
		album("NEON HARBOR", "Meteora", "https://x/100x100bb.jpg"),
	}}
	srv := fake.serve()
	defer srv.Close()

	_, _, err := newFetcherAgainst(t, srv).Fetch(context.Background(), "Neon Harbor", "From Nowhere")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("aucun résultat ne correspond à l'album demandé : attendu os.ErrNotExist, obtenu %v", err)
	}
	if fake.artHits != 0 {
		t.Errorf("l'image ne doit même pas être téléchargée quand rien ne correspond (%d appels)", fake.artHits)
	}
}

func TestFetchPicksTheMatchingAlbumEvenWhenItIsNotFirst(t *testing.T) {
	fake := &fakeITunes{results: []map[string]any{
		album("NEON HARBOR", "Mixed Signals (Deluxe Edition)", "https://x/wrong/100x100bb.jpg"),
		album("NEON HARBOR", "From Nowhere", "https://x/right/100x100bb.jpg"),
	}}
	srv := fake.serve()
	defer srv.Close()

	// L'URL de l'artwork doit être celle du bon album, en 600x600.
	var gotURL string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/search") {
			return srv.Client().Transport.RoundTrip(r)
		}
		gotURL = r.URL.String()
		return &http.Response{StatusCode: 200, Body: http.NoBody, Header: http.Header{}}, nil
	})}
	f := itunes.NewAlbumArtFetcher(client)
	f.SetSearchEndpointForTest(srv.URL + "/search")

	if _, _, err := f.Fetch(context.Background(), "Neon Harbor", "From Nowhere"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(gotURL, "/right/") {
		t.Errorf("l'artwork du bon album doit être choisi, obtenu %q", gotURL)
	}
	if !strings.Contains(gotURL, "600x600bb") {
		t.Errorf("l'artwork doit être demandé en 600x600, obtenu %q", gotURL)
	}
}

// Un titre d'édition ne doit pas faire rater un match légitime : la
// bibliothèque du user contient "Song Of The Valley (Remastered 2025)", et
// iTunes renvoie parfois le titre nu (ou l'inverse).
func TestFetchToleratesEditionSuffixes(t *testing.T) {
	for _, tc := range []struct{ name, asked, returned string }{
		{"suffixe côté iTunes", "Song Of The Valley", "Song Of The Valley (Remastered 2025)"},
		{"suffixe côté bibliothèque", "Song Of The Valley (Remastered 2025)", "Song Of The Valley"},
		{"casse et ponctuation", "song of the valley!", "Song of the Valley"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeITunes{}
			srv := fake.serve()
			defer srv.Close()
			// L'URL d'artwork doit pointer sur le serveur de test, sinon le
			// Fetch part sur le réseau réel après un match réussi.
			fake.results = []map[string]any{album("Skyrider", tc.returned, srv.URL+"/art/100x100bb.jpg")}
			if _, _, err := newFetcherAgainst(t, srv).Fetch(context.Background(), "Skyrider", tc.asked); err != nil {
				t.Fatalf("un match légitime doit passer : %v", err)
			}
			if fake.artHits != 1 {
				t.Errorf("l'artwork du bon album doit être téléchargé une fois, obtenu %d", fake.artHits)
			}
		})
	}
}

// Même titre, autre artiste : iTunes a beaucoup d'albums "From Nowhere".
func TestFetchRejectsTheRightTitleFromTheWrongArtist(t *testing.T) {
	fake := &fakeITunes{results: []map[string]any{album("Indubious", "From Nowhere", "https://x/100x100bb.jpg")}}
	srv := fake.serve()
	defer srv.Close()

	if _, _, err := newFetcherAgainst(t, srv).Fetch(context.Background(), "Neon Harbor", "From Nowhere"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("attendu os.ErrNotExist pour un homonyme d'un autre artiste, obtenu %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
