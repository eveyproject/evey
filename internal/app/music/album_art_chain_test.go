package music_test

import (
	"context"
	"errors"
	"os"
	"testing"

	appmusic "github.com/eveyproject/evey/internal/app/music"
)

type stubSource struct {
	data  []byte
	mime  string
	err   error
	calls int
}

func (s *stubSource) Fetch(_ context.Context, _, _ string) ([]byte, string, error) {
	s.calls++
	return s.data, s.mime, s.err
}

func TestChainStopsAtTheFirstSourceThatKnowsTheAlbum(t *testing.T) {
	first := &stubSource{data: []byte("MB"), mime: "image/jpeg"}
	second := &stubSource{data: []byte("ITUNES"), mime: "image/jpeg"}

	data, mime, err := appmusic.NewChainAlbumArtFetcher(first, second).
		Fetch(context.Background(), "Neon Harbor", "From Nowhere")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(data) != "MB" || mime != "image/jpeg" {
		t.Errorf("la première source doit gagner, obtenu %q/%q", data, mime)
	}
	if second.calls != 0 {
		t.Errorf("la source suivante ne doit pas être interrogée (%d appels)", second.calls)
	}
}

// Une source qui ne connaît pas l'album (os.ErrNotExist) ne doit pas arrêter la
// chaîne : c'est tout l'intérêt de garder iTunes derrière MusicBrainz pour les
// albums absents de Cover Art Archive.
func TestChainFallsThroughOnNotExist(t *testing.T) {
	first := &stubSource{err: os.ErrNotExist}
	second := &stubSource{data: []byte("ITUNES"), mime: "image/jpeg"}

	data, _, err := appmusic.NewChainAlbumArtFetcher(first, second).
		Fetch(context.Background(), "A", "B")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(data) != "ITUNES" {
		t.Errorf("la seconde source doit prendre le relais, obtenu %q", data)
	}
}

// Une panne réseau ne doit pas non plus priver les sources suivantes.
func TestChainFallsThroughOnTransientFailure(t *testing.T) {
	first := &stubSource{err: errors.New("musicbrainz status 503")}
	second := &stubSource{data: []byte("ITUNES"), mime: "image/jpeg"}

	data, _, err := appmusic.NewChainAlbumArtFetcher(first, second).
		Fetch(context.Background(), "A", "B")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(data) != "ITUNES" {
		t.Errorf("une source en panne doit être contournée, obtenu %q", data)
	}
}

// Le contrat compte : l'appelant distingue « album inconnu » (ErrNotExist, il
// retire alors une image périmée) d'une panne. Épuiser la chaîne = absence.
func TestChainReturnsNotExistWhenNoSourceKnowsTheAlbum(t *testing.T) {
	first := &stubSource{err: os.ErrNotExist}
	second := &stubSource{err: errors.New("boom")}

	_, _, err := appmusic.NewChainAlbumArtFetcher(first, second).
		Fetch(context.Background(), "A", "B")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("attendu os.ErrNotExist, obtenu %v", err)
	}
}

// Un succès à corps vide n'a pas à produire un fichier de 0 octet en cache.
func TestChainTreatsAnEmptyBodyAsAnAbsence(t *testing.T) {
	first := &stubSource{data: nil, mime: "image/jpeg"}
	second := &stubSource{data: []byte("ITUNES"), mime: "image/jpeg"}

	data, _, err := appmusic.NewChainAlbumArtFetcher(first, second).
		Fetch(context.Background(), "A", "B")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(data) != "ITUNES" {
		t.Errorf("un corps vide doit être ignoré, obtenu %q", data)
	}
}

// Une annulation du scan arrête la chaîne au lieu de traverser toutes les
// sources pour échouer autant de fois.
func TestChainStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	first := &stubSource{data: []byte("MB"), mime: "image/jpeg"}
	second := &stubSource{data: []byte("ITUNES"), mime: "image/jpeg"}

	_, _, err := appmusic.NewChainAlbumArtFetcher(first, second).Fetch(ctx, "A", "B")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("attendu context.Canceled, obtenu %v", err)
	}
	if first.calls != 0 || second.calls != 0 {
		t.Errorf("aucune source ne doit être interrogée après annulation (%d, %d)", first.calls, second.calls)
	}
}

func TestChainIgnoresNilSourcesAndHandlesAnEmptyChain(t *testing.T) {
	only := &stubSource{data: []byte("MB"), mime: "image/jpeg"}
	if _, _, err := appmusic.NewChainAlbumArtFetcher(nil, only, nil).Fetch(context.Background(), "A", "B"); err != nil {
		t.Fatalf("les sources nil doivent être ignorées : %v", err)
	}
	if _, _, err := appmusic.NewChainAlbumArtFetcher().Fetch(context.Background(), "A", "B"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("chaîne vide : attendu os.ErrNotExist, obtenu %v", err)
	}
}
