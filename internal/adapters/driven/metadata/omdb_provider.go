package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	domain "github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

const omdbBaseURL = "https://www.omdbapi.com"

type OMDbProvider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewOMDbProvider(apiKey string, httpClient *http.Client) *OMDbProvider {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &OMDbProvider{
		apiKey:     apiKey,
		baseURL:    omdbBaseURL,
		httpClient: httpClient,
	}
}

func (p *OMDbProvider) Name() string { return "omdb" }

func (p *OMDbProvider) Search(ctx context.Context, query string, year *int, mediaType string) ([]domain.MetadataResult, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.omdb.Search")
	defer span.End()

	if p.apiKey == "" {
		return nil, nil
	}

	omdbType := mediaType
	if mediaType == "tv" {
		omdbType = "series"
	}

	params := url.Values{
		"apikey": {p.apiKey},
		"s":      {query},
		"type":   {omdbType},
	}
	if year != nil {
		params.Set("y", strconv.Itoa(*year))
	}
	u := fmt.Sprintf("%s/?%s", p.baseURL, params.Encode())

	body, err := p.doRequest(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("omdb search: %w", err)
	}

	var resp omdbSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("omdb search unmarshal: %w", err)
	}

	if resp.Response == "False" {
		return nil, nil
	}

	results := make([]domain.MetadataResult, 0, len(resp.Search))
	for _, item := range resp.Search {
		r := domain.MetadataResult{
			Title:       item.Title,
			ProviderIDs: map[string]string{"imdb": item.ImdbID},
		}
		if y, err := strconv.Atoi(item.Year); err == nil {
			r.Year = &y
		}
		if item.Poster != "" && item.Poster != "N/A" {
			r.Artwork = []domain.ArtworkResult{{
				Type:     "poster",
				URL:      item.Poster,
				Provider: "omdb",
			}}
		}
		results = append(results, r)
	}
	return results, nil
}

func (p *OMDbProvider) GetByProviderID(ctx context.Context, providerID string, mediaType string) (*domain.MetadataResult, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.omdb.GetByProviderID")
	defer span.End()

	if p.apiKey == "" {
		return nil, nil
	}

	params := url.Values{
		"apikey": {p.apiKey},
		"i":      {providerID},
		"plot":   {"full"},
	}
	u := fmt.Sprintf("%s/?%s", p.baseURL, params.Encode())

	body, err := p.doRequest(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("omdb detail: %w", err)
	}

	var detail omdbDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		return nil, fmt.Errorf("omdb detail unmarshal: %w", err)
	}

	if detail.Response == "False" {
		return nil, fmt.Errorf("omdb: %s", detail.Error)
	}

	return mapOMDbDetail(detail), nil
}

func (p *OMDbProvider) doRequest(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		// *url.Error quotes the request URL, apikey included: drop it before
		// the error reaches a wrap or a log line.
		if ue, ok := errors.AsType[*url.Error](err); ok {
			ue.URL = redactAPIKey(ue.URL)
		}
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10MB max
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("omdb status %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func mapOMDbDetail(d omdbDetail) *domain.MetadataResult {
	result := &domain.MetadataResult{
		Title:       d.Title,
		Overview:    naToEmpty(d.Plot),
		ProviderIDs: map[string]string{"imdb": d.ImdbID},
		Ratings:     map[string]float64{},
	}

	if d.Year != "" {
		// OMDb sometimes returns "2010–2013" for series
		yearStr, _, _ := strings.Cut(d.Year, "\u2013") // OMDb writes a series run with an en dash
		yearStr = strings.Split(yearStr, "-")[0]
		if y, err := strconv.Atoi(strings.TrimSpace(yearStr)); err == nil {
			result.Year = &y
		}
	}

	result.OfficialRating = naToEmpty(d.Rated)

	if d.Genre != "" && d.Genre != "N/A" {
		for g := range strings.SplitSeq(d.Genre, ", ") {
			result.Genres = append(result.Genres, strings.TrimSpace(g))
		}
	}

	for _, r := range d.Ratings {
		switch r.Source {
		case "Internet Movie Database":
			if v, err := parseIMDBRating(r.Value); err == nil {
				result.Ratings["imdb"] = v
			}
		case "Rotten Tomatoes":
			if v, err := parsePercentRating(r.Value); err == nil {
				result.Ratings["rt"] = v
			}
		case "Metacritic":
			if v, err := parseFractionRating(r.Value); err == nil {
				result.Ratings["metacritic"] = v
			}
		}
	}

	if imdbR, ok := result.Ratings["imdb"]; ok && result.CommunityRating == nil {
		result.CommunityRating = &imdbR
	}

	// Parse people from comma-separated strings
	for _, name := range splitNames(d.Director) {
		result.People = append(result.People, domain.PersonResult{Name: name, Role: "Director"})
	}
	for _, name := range splitNames(d.Writer) {
		clean, _, _ := strings.Cut(name, " (") // remove "(screenplay)" etc
		result.People = append(result.People, domain.PersonResult{Name: strings.TrimSpace(clean), Role: "Writer"})
	}
	for _, name := range splitNames(d.Actors) {
		result.People = append(result.People, domain.PersonResult{Name: name, Role: "Actor"})
	}

	if d.Poster != "" && d.Poster != "N/A" {
		result.Artwork = []domain.ArtworkResult{{
			Type:     "poster",
			URL:      d.Poster,
			Provider: "omdb",
		}}
	}

	return result
}

func naToEmpty(s string) string {
	if s == "N/A" {
		return ""
	}
	return s
}

func splitNames(s string) []string {
	if s == "" || s == "N/A" {
		return nil
	}
	var names []string
	for n := range strings.SplitSeq(s, ", ") {
		n = strings.TrimSpace(n)
		if n != "" {
			names = append(names, n)
		}
	}
	return names
}

// "8.8/10" → 8.8
func parseIMDBRating(s string) (float64, error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return 0, fmt.Errorf("bad imdb rating: %s", s)
	}
	return strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
}

// "87%" → 87.0
func parsePercentRating(s string) (float64, error) {
	s = strings.TrimSuffix(s, "%")
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

// "74/100" → 74.0
func parseFractionRating(s string) (float64, error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return 0, fmt.Errorf("bad fraction rating: %s", s)
	}
	return strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
}

// OMDb response types

type omdbSearchResponse struct {
	Search   []omdbSearchItem `json:"Search"`
	Response string           `json:"Response"`
}

type omdbSearchItem struct {
	Title  string `json:"Title"`
	Year   string `json:"Year"`
	ImdbID string `json:"imdbID"`
	Type   string `json:"Type"`
	Poster string `json:"Poster"`
}

type omdbDetail struct {
	Title     string       `json:"Title"`
	Year      string       `json:"Year"`
	Rated     string       `json:"Rated"`
	Released  string       `json:"Released"`
	Runtime   string       `json:"Runtime"`
	Genre     string       `json:"Genre"`
	Director  string       `json:"Director"`
	Writer    string       `json:"Writer"`
	Actors    string       `json:"Actors"`
	Plot      string       `json:"Plot"`
	Awards    string       `json:"Awards"`
	Poster    string       `json:"Poster"`
	Ratings   []omdbRating `json:"Ratings"`
	ImdbID    string       `json:"imdbID"`
	BoxOffice string       `json:"BoxOffice"`
	Response  string       `json:"Response"`
	Error     string       `json:"Error"`
}

type omdbRating struct {
	Source string `json:"Source"`
	Value  string `json:"Value"`
}

// redactAPIKey replaces the apikey query value of rawURL.
func redactAPIKey(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "omdb request"
	}
	q := u.Query()
	if q.Has("apikey") {
		q.Set("apikey", "REDACTED")
		u.RawQuery = q.Encode()
	}
	return u.String()
}
