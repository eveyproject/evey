package musicbrainz_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driven/musicbrainz"
)

// jpegOfSize forge un JPEG réel des dimensions demandées : le fetcher lit
// l'en-tête pour écarter les scans qui ne sont pas des pochettes carrées, donc
// des octets bidon ne testeraient rien.
func jpegOfSize(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 200, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

type stubAPIs struct {
	// searchResults : release-groups renvoyés par MusicBrainz.
	searchResults []map[string]any
	// releases : éditions renvoyées pour le release-group.
	releases []map[string]any
	// art : chemin CAA -> image servie. Tout chemin absent répond 404.
	art map[string][]byte

	artRequests []string
}

func (s *stubAPIs) serve(t *testing.T) (mbURL, caaURL string) {
	t.Helper()
	mb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/release-group"):
			_ = json.NewEncoder(w).Encode(map[string]any{"release-groups": s.searchResults})
		case strings.HasPrefix(r.URL.Path, "/release"):
			_ = json.NewEncoder(w).Encode(map[string]any{"releases": s.releases})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mb.Close)

	caa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.artRequests = append(s.artRequests, r.URL.Path)
		img, ok := s.art[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(img)
	}))
	t.Cleanup(caa.Close)
	return mb.URL, caa.URL
}

func newFetcher(t *testing.T, s *stubAPIs) *musicbrainz.AlbumArtFetcher {
	t.Helper()
	mbURL, caaURL := s.serve(t)
	f := musicbrainz.NewAlbumArtFetcher(nil)
	f.SetBaseURLsForTest(mbURL, caaURL)
	return f
}

func releaseGroup(id, title, artist string) map[string]any {
	return map[string]any{
		"id": id, "title": title,
		"artist-credit": []map[string]any{{"name": artist}},
	}
}

func release(id, format string) map[string]any {
	return map[string]any{"id": id, "media": []map[string]any{{"format": format}}}
}

// Le cas qui a motivé le changement de provider : iTunes ne connaît pas
// "From Nowhere" et renvoyait Mixed Signals. MusicBrainz le trouve par champs.
func TestFetchReturnsTheArtOfTheMatchingReleaseGroup(t *testing.T) {
	square := jpegOfSize(t, 500, 500)
	s := &stubAPIs{
		searchResults: []map[string]any{releaseGroup("rg-1", "From Nowhere", "Neon Harbor")},
		releases:      []map[string]any{release("rel-cd", "CD")},
		art:           map[string][]byte{"/release/rel-cd/front-500": square},
	}

	data, mime, err := newFetcher(t, s).Fetch(context.Background(), "Neon Harbor", "From Nowhere")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !bytes.Equal(data, square) {
		t.Error("les octets de la pochette de l'édition doivent être renvoyés")
	}
	if mime != "image/jpeg" {
		t.Errorf("mime inattendu: %q", mime)
	}
}

// Un release-group au titre différent ne doit jamais être retenu, même s'il est
// le seul résultat renvoyé - c'est exactement la faute d'iTunes.
func TestFetchRejectsAReleaseGroupForAnotherAlbum(t *testing.T) {
	s := &stubAPIs{
		searchResults: []map[string]any{releaseGroup("rg-ht", "Mixed Signals", "Neon Harbor")},
		art:           map[string][]byte{"/release-group/rg-ht/front-500": jpegOfSize(t, 500, 500)},
	}

	_, _, err := newFetcher(t, s).Fetch(context.Background(), "Neon Harbor", "From Nowhere")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("attendu os.ErrNotExist, obtenu %v", err)
	}
	if len(s.artRequests) != 0 {
		t.Errorf("aucune image ne doit être demandée sans correspondance (%v)", s.artRequests)
	}
}

func TestFetchRejectsTheRightTitleFromTheWrongArtist(t *testing.T) {
	s := &stubAPIs{
		searchResults: []map[string]any{releaseGroup("rg-x", "From Nowhere", "Indubious")},
	}
	if _, _, err := newFetcher(t, s).Fetch(context.Background(), "Neon Harbor", "From Nowhere"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("attendu os.ErrNotExist pour un homonyme, obtenu %v", err)
	}
}

// Cover Art Archive sert des SCANS d'éditions physiques : l'image « front » du
// release-group de From Nowhere est une cassette (mesuré : 320x500). Dans une
// grille carrée elle serait rognée, donc on doit préférer l'édition CD ou
// numérique - et refuser le scan hors proportions même en repli.
func TestFetchPrefersASquareReleaseOverANonSquareReleaseGroupScan(t *testing.T) {
	cassette := jpegOfSize(t, 320, 500)
	square := jpegOfSize(t, 500, 500)
	s := &stubAPIs{
		searchResults: []map[string]any{releaseGroup("rg-1", "From Nowhere", "Neon Harbor")},
		releases: []map[string]any{
			release("rel-vinyl", `12" Vinyl`),
			release("rel-digital", "Digital Media"),
		},
		art: map[string][]byte{
			"/release/rel-digital/front-500": square,
			"/release-group/rg-1/front-500":  cassette,
		},
	}

	data, _, err := newFetcher(t, s).Fetch(context.Background(), "Neon Harbor", "From Nowhere")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !bytes.Equal(data, square) {
		t.Error("l'édition numérique carrée doit primer sur le scan du release-group")
	}
	for _, p := range s.artRequests {
		if strings.Contains(p, "rel-vinyl") {
			t.Errorf("le vinyle ne doit pas être interrogé (%v)", s.artRequests)
		}
	}
}

func TestFetchRejectsANonSquareScanEvenAsLastResort(t *testing.T) {
	s := &stubAPIs{
		searchResults: []map[string]any{releaseGroup("rg-1", "From Nowhere", "Neon Harbor")},
		art: map[string][]byte{
			"/release-group/rg-1/front-500":  jpegOfSize(t, 320, 500),
			"/release-group/rg-1/front-1200": jpegOfSize(t, 383, 599),
		},
	}
	if _, _, err := newFetcher(t, s).Fetch(context.Background(), "Neon Harbor", "From Nowhere"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("un scan aux proportions de cassette doit être refusé, obtenu %v", err)
	}
}

// Repli sur le release-group quand aucune édition n'a d'image : c'est le cas le
// plus fréquent sur Cover Art Archive.
func TestFetchFallsBackToTheReleaseGroupArt(t *testing.T) {
	square := jpegOfSize(t, 500, 494) // scan légèrement imprécis, toléré
	s := &stubAPIs{
		searchResults: []map[string]any{releaseGroup("rg-2", "Song Of The Valley", "Skyrider")},
		releases:      []map[string]any{release("rel-cd", "CD")},
		art:           map[string][]byte{"/release-group/rg-2/front-500": square},
	}

	data, _, err := newFetcher(t, s).Fetch(context.Background(), "Skyrider", "Song Of The Valley")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !bytes.Equal(data, square) {
		t.Error("l'image du release-group doit servir de repli")
	}
}

// Le suffixe d'édition ne doit pas faire rater la correspondance : la
// bibliothèque du user a "Song Of The Valley (Remastered 2025)".
func TestFetchToleratesEditionSuffixes(t *testing.T) {
	square := jpegOfSize(t, 500, 500)
	s := &stubAPIs{
		searchResults: []map[string]any{releaseGroup("rg-3", "Song of the Valley", "Skyrider")},
		art:           map[string][]byte{"/release-group/rg-3/front-500": square},
	}
	if _, _, err := newFetcher(t, s).Fetch(context.Background(),
		"Skyrider", "Song Of The Valley (Remastered 2025)"); err != nil {
		t.Fatalf("un match légitime doit passer : %v", err)
	}
}

// Mesuré contre l'API réelle : `releasegroup:"Song Of The Valley (Remastered
// 2025)"` ne renvoie AUCUN résultat, alors que le titre nu sort avec un score de
// 100. La requête doit donc porter le titre débarrassé de son suffixe d'édition,
// la vérification restant faite sur le titre complet.
func TestFetchQueriesTheTitleWithoutItsEditionSuffix(t *testing.T) {
	var gotQuery string
	mb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/release-group") {
			gotQuery = r.URL.Query().Get("query")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"release-groups": []map[string]any{}})
	}))
	defer mb.Close()

	f := musicbrainz.NewAlbumArtFetcher(nil)
	f.SetBaseURLsForTest(mb.URL, mb.URL)
	_, _, _ = f.Fetch(context.Background(), "Skyrider", "Song Of The Valley (Remastered 2025)")

	if strings.Contains(gotQuery, "Remastered") {
		t.Errorf("le suffixe d'édition ne doit pas être interrogé : %q", gotQuery)
	}
	if !strings.Contains(gotQuery, "Song Of The Valley") {
		t.Errorf("le titre de base doit être interrogé : %q", gotQuery)
	}
}

func TestBaseTitleStripping(t *testing.T) {
	square := jpegOfSize(t, 500, 500)
	for _, tc := range []struct{ name, libraryTitle, providerTitle string }{
		{"remaster", "Song Of The Valley (Remastered 2025)", "Song of the Valley"},
		{"deluxe", "From Nowhere (Deluxe Edition)", "From Nowhere"},
		{"crochets", "Meteora [Explicit]", "Meteora"},
		{"suffixes empilés", "Meteora (Deluxe) [Explicit]", "Meteora"},
		{"aucun suffixe", "Mixed Signals", "Mixed Signals"},
		// Un titre entièrement parenthésé n'a pas de base à extraire : il doit
		// rester interrogeable tel quel.
		{"titre entre parenthèses", "(What's the Story) Morning Glory?", "(What's the Story) Morning Glory?"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &stubAPIs{
				searchResults: []map[string]any{releaseGroup("rg", tc.providerTitle, "X")},
				art:           map[string][]byte{"/release-group/rg/front-500": square},
			}
			if _, _, err := newFetcher(t, s).Fetch(context.Background(), "X", tc.libraryTitle); err != nil {
				t.Errorf("%q devait correspondre à %q : %v", tc.libraryTitle, tc.providerTitle, err)
			}
		})
	}
}

// Aucun résultat : "Orchard Essentials" est une playlist Apple, pas une sortie.
func TestFetchReturnsNotExistWhenNothingMatches(t *testing.T) {
	s := &stubAPIs{searchResults: nil}
	if _, _, err := newFetcher(t, s).Fetch(context.Background(), "Neon Harbor", "Orchard Essentials"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("attendu os.ErrNotExist, obtenu %v", err)
	}
}

func TestFetchRequiresBothArtistAndAlbum(t *testing.T) {
	s := &stubAPIs{}
	f := newFetcher(t, s)
	for _, tc := range []struct{ artist, album string }{{"", "From Nowhere"}, {"Neon Harbor", ""}, {"", ""}} {
		if _, _, err := f.Fetch(context.Background(), tc.artist, tc.album); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("(%q,%q) : attendu os.ErrNotExist, obtenu %v", tc.artist, tc.album, err)
		}
	}
}

// Un titre contenant de la syntaxe Lucene ne doit pas casser la requête ni
// faire échouer la recherche.
func TestFetchSanitizesLuceneSyntaxInTheQuery(t *testing.T) {
	square := jpegOfSize(t, 500, 500)
	s := &stubAPIs{
		searchResults: []map[string]any{releaseGroup("rg-4", `Where Is My Mind?`, "Pixies")},
		art:           map[string][]byte{"/release-group/rg-4/front-500": square},
	}
	if _, _, err := newFetcher(t, s).Fetch(context.Background(), "Pixies", `Where Is My Mind?`); err != nil {
		t.Fatalf("un titre avec '?' doit être géré : %v", err)
	}
}
