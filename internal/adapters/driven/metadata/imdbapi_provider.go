package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	domain "github.com/gravity-zero/evey/internal/domain/metadata"
)

const imdbAPIBaseURL = "https://api.imdbapi.dev"

type IMDBAPIProvider struct {
	baseURL    string
	httpClient *http.Client
}

func NewIMDBAPIProvider(httpClient *http.Client) *IMDBAPIProvider {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &IMDBAPIProvider{
		baseURL:    imdbAPIBaseURL,
		httpClient: httpClient,
	}
}

func (p *IMDBAPIProvider) Name() string { return "imdbapi" }

// Search queries imdbapi.dev /search/titles.
func (p *IMDBAPIProvider) Search(ctx context.Context, query string, _ *int, mediaType string) ([]domain.MetadataResult, error) {
	params := url.Values{"query": {query}, "limit": {"10"}}
	u := fmt.Sprintf("%s/search/titles?%s", p.baseURL, params.Encode())

	body, err := p.doRequest(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("imdbapi search: %w", err)
	}

	var resp imdbSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("imdbapi search unmarshal: %w", err)
	}

	results := make([]domain.MetadataResult, 0, len(resp.Results))
	for _, t := range resp.Results {
		results = append(results, mapIMDBTitle(t))
	}
	return results, nil
}

// GetByProviderID fetches a title by IMDB ID (e.g. "tt0111161").
func (p *IMDBAPIProvider) GetByProviderID(ctx context.Context, providerID string, mediaType string) (*domain.MetadataResult, error) {
	u := fmt.Sprintf("%s/titles/%s", p.baseURL, providerID)

	body, err := p.doRequest(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("imdbapi get title: %w", err)
	}

	var t imdbTitle
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, fmt.Errorf("imdbapi title unmarshal: %w", err)
	}

	result := mapIMDBTitle(t)
	return &result, nil
}

func (p *IMDBAPIProvider) doRequest(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("imdbapi status %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func mapIMDBTitle(t imdbTitle) domain.MetadataResult {
	result := domain.MetadataResult{
		Title:         t.PrimaryTitle,
		OriginalTitle: t.OriginalTitle,
		Overview:      t.Plot,
		ProviderIDs:   map[string]string{"imdb": t.ID},
	}

	if t.StartYear > 0 {
		y := t.StartYear
		result.Year = &y
		ts := time.Date(y, time.January, 1, 0, 0, 0, 0, time.UTC)
		result.PremiereDate = &ts
	}

	if t.Rating != nil && t.Rating.AggregateRating > 0 {
		r := t.Rating.AggregateRating
		result.CommunityRating = &r
	}

	result.Genres = t.Genres

	if t.PrimaryImage != nil && t.PrimaryImage.URL != "" {
		result.Artwork = append(result.Artwork, domain.ArtworkResult{
			Type:   "poster",
			URL:    t.PrimaryImage.URL,
			Width:  t.PrimaryImage.Width,
			Height: t.PrimaryImage.Height,
		})
	}

	for _, d := range t.Directors {
		result.People = append(result.People, domain.PersonResult{
			Name: d.DisplayName,
			Role: "Director",
		})
	}
	for _, s := range t.Stars {
		result.People = append(result.People, domain.PersonResult{
			Name: s.DisplayName,
			Role: "Actor",
		})
	}
	for _, w := range t.Writers {
		result.People = append(result.People, domain.PersonResult{
			Name: w.DisplayName,
			Role: "Writer",
		})
	}

	return result
}

// imdbapi.dev response types

type imdbSearchResponse struct {
	Results []imdbTitle `json:"results"`
}

type imdbTitle struct {
	ID             string      `json:"id"`
	Type           string      `json:"type"`
	PrimaryTitle   string      `json:"primaryTitle"`
	OriginalTitle  string      `json:"originalTitle"`
	Plot           string      `json:"plot"`
	StartYear      int         `json:"startYear"`
	EndYear        int         `json:"endYear"`
	RuntimeSeconds int         `json:"runtimeSeconds"`
	Genres         []string    `json:"genres"`
	Rating         *imdbRating `json:"rating"`
	PrimaryImage   *imdbImage  `json:"primaryImage"`
	Directors      []imdbName  `json:"directors"`
	Writers        []imdbName  `json:"writers"`
	Stars          []imdbName  `json:"stars"`
}

type imdbRating struct {
	AggregateRating float64 `json:"aggregateRating"`
	VoteCount       int     `json:"voteCount"`
}

type imdbImage struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Type   string `json:"type"`
}

type imdbName struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}
