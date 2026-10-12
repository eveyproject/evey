package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	domain "github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"golang.org/x/time/rate"
)

const tvmazeBaseURL = "https://api.tvmaze.com"

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

type TVMazeProvider struct {
	baseURL    string
	httpClient *http.Client
	limiter    *rate.Limiter
}

func NewTVMazeProvider(httpClient *http.Client) *TVMazeProvider {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &TVMazeProvider{
		baseURL:    tvmazeBaseURL,
		httpClient: httpClient,
		limiter:    rate.NewLimiter(rate.Every(500*time.Millisecond), 1), // 2 req/s
	}
}

func (p *TVMazeProvider) Name() string { return "tvmaze" }

func (p *TVMazeProvider) Search(ctx context.Context, query string, _ *int, mediaType string) ([]domain.MetadataResult, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.tvmaze.Search")
	defer span.End()

	if mediaType == "movie" {
		return nil, nil
	}

	params := url.Values{"q": {query}}
	u := fmt.Sprintf("%s/search/shows?%s", p.baseURL, params.Encode())

	body, err := p.doRequest(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("tvmaze search: %w", err)
	}

	var resp []tvmazeSearchResult
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("tvmaze search unmarshal: %w", err)
	}

	results := make([]domain.MetadataResult, 0, len(resp))
	for _, r := range resp {
		results = append(results, mapTVMazeShow(r.Show))
	}
	return results, nil
}

func (p *TVMazeProvider) GetByProviderID(ctx context.Context, providerID string, mediaType string) (*domain.MetadataResult, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.tvmaze.GetByProviderID")
	defer span.End()

	if mediaType == "movie" {
		return nil, nil
	}

	u := fmt.Sprintf("%s/shows/%s?embed[]=cast", p.baseURL, providerID)
	body, err := p.doRequest(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("tvmaze detail: %w", err)
	}

	var show tvmazeShowDetail
	if err := json.Unmarshal(body, &show); err != nil {
		return nil, fmt.Errorf("tvmaze detail unmarshal: %w", err)
	}

	result := mapTVMazeShow(show.tvmazeShow)

	if show.Embedded != nil {
		for _, cm := range show.Embedded.Cast {
			pr := domain.PersonResult{
				Name: cm.Person.Name,
				Role: "Actor",
			}
			if cm.Character.Name != "" {
				pr.Character = cm.Character.Name
			}
			if cm.Person.Image != nil {
				pr.ThumbURL = cm.Person.Image.Medium
			}
			result.People = append(result.People, pr)
		}
	}

	return &result, nil
}

func (p *TVMazeProvider) GetEpisodeDetails(ctx context.Context, seriesID string, seasonNum, episodeNum int) (*domain.MetadataResult, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.tvmaze.GetEpisodeDetails")
	defer span.End()

	u := fmt.Sprintf("%s/shows/%s/episodebynumber?season=%d&number=%d", p.baseURL, seriesID, seasonNum, episodeNum)
	body, err := p.doRequest(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("tvmaze episode: %w", err)
	}

	var ep tvmazeEpisode
	if err := json.Unmarshal(body, &ep); err != nil {
		return nil, fmt.Errorf("tvmaze episode unmarshal: %w", err)
	}

	result := &domain.MetadataResult{
		Title:    ep.Name,
		Overview: stripHTML(ep.Summary),
	}
	// Persist the tvmaze episode ID - without it the orphan filter keeps
	// flagging the row even though enrichment ran (provider_ids stays {}).
	if ep.ID > 0 {
		result.ProviderIDs = map[string]string{"tvmaze": strconv.Itoa(ep.ID)}
	}
	if ep.Rating.Average > 0 {
		r := ep.Rating.Average
		result.CommunityRating = &r
	}
	if ep.Airdate != "" {
		if t := parseDate(ep.Airdate); t != nil {
			result.PremiereDate = t
			y := t.Year()
			result.Year = &y
		}
	}
	if ep.Image != nil && ep.Image.Original != "" {
		result.Artwork = []domain.ArtworkResult{{
			Type:     "backdrop",
			URL:      ep.Image.Original,
			Provider: "tvmaze",
		}}
	}
	return result, nil
}

func (p *TVMazeProvider) LookupByExternalID(ctx context.Context, providerName, externalID string) (*domain.MetadataResult, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.tvmaze.LookupByExternalID")
	defer span.End()

	var param string
	switch providerName {
	case "imdb":
		param = "imdb"
	case "thetvdb":
		param = "thetvdb"
	default:
		return nil, fmt.Errorf("tvmaze: unsupported external provider %q", providerName)
	}

	u := fmt.Sprintf("%s/lookup/shows?%s=%s", p.baseURL, param, url.QueryEscape(externalID))
	body, err := p.doRequest(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("tvmaze lookup: %w", err)
	}

	var show tvmazeShow
	if err := json.Unmarshal(body, &show); err != nil {
		return nil, fmt.Errorf("tvmaze lookup unmarshal: %w", err)
	}

	result := mapTVMazeShow(show)
	return &result, nil
}

func (p *TVMazeProvider) doRequest(ctx context.Context, rawURL string) ([]byte, error) {
	if err := p.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10MB max
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("tvmaze: not found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tvmaze status %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func stripHTML(s string) string {
	return htmlTagRe.ReplaceAllString(s, "")
}

func mapTVMazeShow(s tvmazeShow) domain.MetadataResult {
	result := domain.MetadataResult{
		Title:       s.Name,
		Overview:    stripHTML(s.Summary),
		Genres:      s.Genres,
		ProviderIDs: map[string]string{"tvmaze": strconv.Itoa(s.ID)},
		Ratings:     map[string]float64{},
	}

	if s.Externals.IMDB != "" {
		result.ProviderIDs["imdb"] = s.Externals.IMDB
	}
	if s.Externals.TheTVDB > 0 {
		result.ProviderIDs["thetvdb"] = strconv.Itoa(s.Externals.TheTVDB)
	}

	if s.Rating.Average > 0 {
		r := s.Rating.Average
		result.CommunityRating = &r
	}

	if s.Premiered != "" {
		if t := parseDate(s.Premiered); t != nil {
			result.PremiereDate = t
			y := t.Year()
			result.Year = &y
		}
	}

	if s.Image != nil && s.Image.Original != "" {
		result.Artwork = []domain.ArtworkResult{{
			Type:     "poster",
			URL:      s.Image.Original,
			Provider: "tvmaze",
		}}
	}

	return result
}

// TVMaze response types

type tvmazeSearchResult struct {
	Score float64    `json:"score"`
	Show  tvmazeShow `json:"show"`
}

type tvmazeShow struct {
	ID        int            `json:"id"`
	Name      string         `json:"name"`
	Summary   string         `json:"summary"`
	Genres    []string       `json:"genres"`
	Status    string         `json:"status"`
	Premiered string         `json:"premiered"`
	Rating    tvmazeRating   `json:"rating"`
	Image     *tvmazeImage   `json:"image"`
	Externals tvmazeExternal `json:"externals"`
}

type tvmazeShowDetail struct {
	tvmazeShow
	Embedded *tvmazeEmbedded `json:"_embedded"`
}

type tvmazeEmbedded struct {
	Cast []tvmazeCastMember `json:"cast"`
}

type tvmazeCastMember struct {
	Person    tvmazePerson    `json:"person"`
	Character tvmazeCharacter `json:"character"`
}

type tvmazePerson struct {
	ID    int          `json:"id"`
	Name  string       `json:"name"`
	Image *tvmazeImage `json:"image"`
}

type tvmazeCharacter struct {
	Name string `json:"name"`
}

type tvmazeEpisode struct {
	ID      int          `json:"id"`
	Name    string       `json:"name"`
	Season  int          `json:"season"`
	Number  int          `json:"number"`
	Airdate string       `json:"airdate"`
	Summary string       `json:"summary"`
	Rating  tvmazeRating `json:"rating"`
	Image   *tvmazeImage `json:"image"`
}

type tvmazeRating struct {
	Average float64 `json:"average"`
}

type tvmazeImage struct {
	Medium   string `json:"medium"`
	Original string `json:"original"`
}

type tvmazeExternal struct {
	IMDB    string `json:"imdb"`
	TheTVDB int    `json:"thetvdb"`
	TVRage  int    `json:"tvrage"`
}
