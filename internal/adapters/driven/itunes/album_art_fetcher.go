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
	"github.com/evey-project/evey/internal/pkg/tracing"
)

const (
	searchEndpoint = "https://itunes.apple.com/search"
	defaultTimeout = 10 * time.Second
)

type AlbumArtFetcher struct {
	httpClient *http.Client
}

func NewAlbumArtFetcher(client *http.Client) *AlbumArtFetcher {
	if client == nil {
		client = ssrf.SafeHTTPClient(defaultTimeout)
	}
	return &AlbumArtFetcher{httpClient: client}
}

type searchResponse struct {
	Results []struct {
		ArtworkURL100 string `json:"artworkUrl100"`
	} `json:"results"`
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
	q.Set("limit", "1")
	q.Set("country", "US")
	u := searchEndpoint + "?" + q.Encode()

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
	if len(sr.Results) == 0 || sr.Results[0].ArtworkURL100 == "" {
		return nil, "", os.ErrNotExist
	}

	artURL := strings.Replace(sr.Results[0].ArtworkURL100, "100x100bb", "600x600bb", 1)

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
