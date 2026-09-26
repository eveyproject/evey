// Package itunes provides an adapter to the public iTunes Search API
// for fetching album artwork (no API key required).
package itunes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/pkg/ssrf"
	"github.com/evey-project/evey/internal/pkg/titlematch"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

const (
	searchEndpoint = "https://itunes.apple.com/search"
	defaultTimeout = 10 * time.Second
)

type AlbumArtFetcher struct {
	httpClient *http.Client
	endpoint   string
}

func NewAlbumArtFetcher(client *http.Client) *AlbumArtFetcher {
	if client == nil {
		client = ssrf.SafeHTTPClient(defaultTimeout)
	}
	return &AlbumArtFetcher{httpClient: client, endpoint: searchEndpoint}
}

// SetSearchEndpointForTest redirige la recherche vers un serveur de test.
// Réservé aux tests : la production doit taper l'API iTunes publique.
func (f *AlbumArtFetcher) SetSearchEndpointForTest(url string) { f.endpoint = url }

type searchResult struct {
	ArtistName     string `json:"artistName"`
	CollectionName string `json:"collectionName"`
	ArtworkURL100  string `json:"artworkUrl100"`
}

type searchResponse struct {
	Results []searchResult `json:"results"`
}

// Fetch queries iTunes for (artist, album) and returns the JPEG bytes of the
// 600x600 artwork. Returns (nil, "", os.ErrNotExist) if no result.
func (f *AlbumArtFetcher) Fetch(ctx context.Context, artist, album string) ([]byte, string, error) {
	ctx, span := tracing.StartSpan(ctx, "itunes.albumart.Fetch")
	defer span.End()

	if artist == "" || album == "" {
		return nil, "", os.ErrNotExist
	}

	q := url.Values{}
	q.Set("term", artist+" "+album)
	q.Set("entity", "album")
	// Plusieurs résultats, PAS un seul : la recherche plein-texte d'iTunes
	// classe par popularité, donc le premier hit est très souvent l'album
	// vedette de l'artiste et non celui demandé. On récupère un lot et on
	// choisit celui qui correspond vraiment (matchingArtwork).
	q.Set("limit", "25")
	q.Set("country", "US")
	u := f.endpoint + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", fmt.Errorf("itunes: build request: %w", err)
	}
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("itunes: search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("itunes: search status %d", resp.StatusCode)
	}

	var sr searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, "", fmt.Errorf("itunes: decode: %w", err)
	}
	art100, ok := matchingArtwork(sr.Results, artist, album)
	if !ok {
		// Aucun résultat ne correspond à l'album demandé. On abandonne au lieu
		// de servir la pochette d'un AUTRE album : l'appelant retombe alors sur
		// le placeholder, ce qui est honnête, là où une fausse pochette trompe.
		return nil, "", os.ErrNotExist
	}

	artURL := strings.Replace(art100, "100x100bb", "600x600bb", 1)

	artReq, err := http.NewRequestWithContext(ctx, http.MethodGet, artURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("itunes: build art request: %w", err)
	}
	artResp, err := f.httpClient.Do(artReq)
	if err != nil {
		return nil, "", fmt.Errorf("itunes: fetch art: %w", err)
	}
	defer artResp.Body.Close()
	if artResp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("itunes: art status %d", artResp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(artResp.Body, 10*1024*1024))
	if err != nil {
		return nil, "", fmt.Errorf("itunes: read art: %w", err)
	}
	return data, "image/jpeg", nil
}

// matchingArtwork retient le premier résultat dont l'ARTISTE et l'ALBUM
// correspondent vraiment à la demande, et renvoie ok=false si aucun ne
// correspond. Sans ce filtre, iTunes servait l'album vedette de l'artiste dès
// que le titre demandé n'était pas indexé : "Neon Harbor From Nowhere" renvoie
// "Mixed Signals (Deluxe Edition)" en premier résultat.
func matchingArtwork(results []searchResult, artist, album string) (string, bool) {
	for _, r := range results {
		if r.ArtworkURL100 == "" {
			continue
		}
		if !titlematch.Equivalent(r.ArtistName, artist) {
			continue
		}
		if !titlematch.Equivalent(r.CollectionName, album) {
			continue
		}
		return r.ArtworkURL100, true
	}
	return "", false
}
