package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	domain "github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"golang.org/x/time/rate"
)

const musicbrainzBaseURL = "https://musicbrainz.org/ws/2"

// musicbrainzUserAgent identifies Evey to the API - MusicBrainz rejects
// requests without an explicit User-Agent.
const musicbrainzUserAgent = "Evey/1.0 (https://github.com/eveyproject/evey)"

// MusicBrainzProvider looks up music release groups. It only handles
// mediaType "music"; other types short-circuit to (nil, nil) so it can sit
// alongside the movie/TV providers without special-casing at the call site.
type MusicBrainzProvider struct {
	baseURL    string
	httpClient *http.Client
	limiter    *rate.Limiter
}

func NewMusicBrainzProvider() *MusicBrainzProvider {
	return &MusicBrainzProvider{
		baseURL:    musicbrainzBaseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		limiter:    rate.NewLimiter(rate.Every(time.Second), 1), // MusicBrainz asks for <= 1 req/s
	}
}

func (p *MusicBrainzProvider) Name() string { return "musicbrainz" }

func (p *MusicBrainzProvider) Search(ctx context.Context, query string, _ *int, mediaType string) ([]domain.MetadataResult, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.musicbrainz.Search")
	defer span.End()

	if mediaType != "music" {
		return nil, nil
	}

	params := url.Values{
		"query": {query},
		"fmt":   {"json"},
		"limit": {"8"},
	}
	u := fmt.Sprintf("%s/release-group?%s", p.baseURL, params.Encode())

	body, err := p.doRequest(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("musicbrainz search: %w", err)
	}

	var resp musicbrainzSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("musicbrainz search unmarshal: %w", err)
	}

	results := make([]domain.MetadataResult, 0, len(resp.ReleaseGroups))
	for _, rg := range resp.ReleaseGroups {
		results = append(results, mapMusicBrainzReleaseGroup(rg))
	}
	return results, nil
}

func (p *MusicBrainzProvider) GetByProviderID(ctx context.Context, providerID string, mediaType string) (*domain.MetadataResult, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.musicbrainz.GetByProviderID")
	defer span.End()

	if mediaType != "music" {
		return nil, nil
	}

	params := url.Values{
		"fmt": {"json"},
		"inc": {"artist-credits"},
	}
	u := fmt.Sprintf("%s/release-group/%s?%s", p.baseURL, url.PathEscape(providerID), params.Encode())

	body, err := p.doRequest(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("musicbrainz detail: %w", err)
	}

	var rg musicbrainzReleaseGroup
	if err := json.Unmarshal(body, &rg); err != nil {
		return nil, fmt.Errorf("musicbrainz detail unmarshal: %w", err)
	}

	result := mapMusicBrainzReleaseGroup(rg)
	return &result, nil
}

func (p *MusicBrainzProvider) doRequest(ctx context.Context, rawURL string) ([]byte, error) {
	if err := p.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", musicbrainzUserAgent)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10MB max
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("musicbrainz status %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func mapMusicBrainzReleaseGroup(rg musicbrainzReleaseGroup) domain.MetadataResult {
	title := rg.Title
	if len(rg.ArtistCredit) > 0 && rg.ArtistCredit[0].Name != "" {
		title = rg.ArtistCredit[0].Name + " - " + rg.Title
	}

	result := domain.MetadataResult{
		Title:       title,
		ProviderIDs: map[string]string{"musicbrainz": rg.ID},
	}

	if len(rg.FirstReleaseDate) >= 4 {
		if y, err := strconv.Atoi(rg.FirstReleaseDate[:4]); err == nil {
			result.Year = &y
		}
	}

	if rg.ID != "" {
		// UNVERIFIED by construction: the Cover Art Archive addresses artwork by
		// release-group id, so the URL can be built without a lookup - but plenty
		// of release-groups have no cover at all and answer 404/500. Confirming
		// here would mean one rate-limited round trip PER search result, on the
		// search path; the client renders a placeholder on image error instead
		// (web: features/requests/RequestPoster.tsx). Treat this as a best-effort
		// hint, never as a URL known to resolve.
		result.Artwork = []domain.ArtworkResult{{
			Type:     domain.ArtworkTypePoster,
			URL:      "https://coverartarchive.org/release-group/" + rg.ID + "/front-250",
			Provider: "musicbrainz",
		}}
	}

	return result
}

// MusicBrainz response types (release-group search + lookup share this shape).

type musicbrainzSearchResponse struct {
	ReleaseGroups []musicbrainzReleaseGroup `json:"release-groups"`
}

type musicbrainzReleaseGroup struct {
	ID               string                  `json:"id"`
	Title            string                  `json:"title"`
	FirstReleaseDate string                  `json:"first-release-date"`
	ArtistCredit     []musicbrainzArtistCred `json:"artist-credit"`
}

type musicbrainzArtistCred struct {
	Name string `json:"name"`
}
