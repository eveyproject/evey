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

	domain "github.com/gravity-zero/evey/internal/domain/metadata"
)

const (
	defaultBaseURL   = "https://api.themoviedb.org/3"
	imageBaseURL     = "https://image.tmdb.org/t/p/original"
	maxRetries       = 3
	retryWait        = 500 * time.Millisecond
)

var movieGenres = map[int]string{
	28: "Action", 12: "Adventure", 16: "Animation", 35: "Comedy",
	80: "Crime", 99: "Documentary", 18: "Drama", 10751: "Family",
	14: "Fantasy", 36: "History", 27: "Horror", 10402: "Music",
	9648: "Mystery", 10749: "Romance", 878: "Science Fiction",
	10770: "TV Movie", 53: "Thriller", 10752: "War", 37: "Western",
}

var tvGenres = map[int]string{
	10759: "Action & Adventure", 16: "Animation", 35: "Comedy",
	80: "Crime", 99: "Documentary", 18: "Drama", 10751: "Family",
	10762: "Kids", 9648: "Mystery", 10763: "News", 10764: "Reality",
	10765: "Sci-Fi & Fantasy", 10766: "Soap", 10767: "Talk",
	10768: "War & Politics", 37: "Western",
}

type TMDBProvider struct {
	apiKey     string
	baseURL    string
	language   string // ISO 639-1, e.g. "en", "fr"
	httpClient *http.Client
}

func NewTMDBProvider(apiKey string, language string, httpClient *http.Client) *TMDBProvider {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if language == "" {
		language = "en"
	}
	return &TMDBProvider{
		apiKey:     apiKey,
		baseURL:    defaultBaseURL,
		language:   language,
		httpClient: httpClient,
	}
}

func (p *TMDBProvider) Name() string { return "tmdb" }

func (p *TMDBProvider) Search(ctx context.Context, query string, year *int, mediaType string) ([]domain.MetadataResult, error) {
	endpoint, err := searchEndpoint(mediaType)
	if err != nil {
		return nil, err
	}

	// Try with year filter first, fallback without if no results
	if year != nil {
		results, err := p.searchPages(ctx, endpoint, query, year, mediaType)
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			return results, nil
		}
	}
	return p.searchPages(ctx, endpoint, query, nil, mediaType)
}

func (p *TMDBProvider) searchPages(ctx context.Context, endpoint, query string, year *int, mediaType string) ([]domain.MetadataResult, error) {
	var allResults []domain.MetadataResult
	genreMap := genreMapForType(mediaType)

	for page := 1; ; page++ {
		params := url.Values{
			"query":    {query},
			"page":     {strconv.Itoa(page)},
			"language": {p.language},
		}
		if year != nil {
			yearStr := strconv.Itoa(*year)
			if mediaType == "tv" {
				params.Set("first_air_date_year", yearStr)
			} else {
				params.Set("primary_release_year", yearStr)
			}
		}
		u := fmt.Sprintf("%s%s?%s", p.baseURL, endpoint, params.Encode())

		body, err := p.doRequestWithRetry(ctx, u)
		if err != nil {
			return nil, fmt.Errorf("tmdb search request: %w", err)
		}

		var resp tmdbSearchResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("tmdb search unmarshal: %w", err)
		}

		for _, item := range resp.Results {
			allResults = append(allResults, mapSearchResult(item, mediaType, genreMap))
		}

		if page >= resp.TotalPages || page >= 5 {
			break
		}
	}

	return allResults, nil
}

// GetEpisodeDetails fetches title, overview and still image for a specific episode.
func (p *TMDBProvider) GetEpisodeDetails(ctx context.Context, seriesID string, seasonNum, episodeNum int) (*domain.MetadataResult, error) {
	u := fmt.Sprintf("%s/tv/%s/season/%d/episode/%d?language=%s&append_to_response=images",
		p.baseURL, seriesID, seasonNum, episodeNum, p.language)
	body, err := p.doRequestWithRetry(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("tmdb episode request: %w", err)
	}
	var ep struct {
		Name     string  `json:"name"`
		Overview string  `json:"overview"`
		StillPath string `json:"still_path"`
		VoteAverage float64 `json:"vote_average"`
		AirDate  string  `json:"air_date"`
	}
	if err := json.Unmarshal(body, &ep); err != nil {
		return nil, fmt.Errorf("tmdb episode unmarshal: %w", err)
	}
	if ep.Name == "" {
		return nil, fmt.Errorf("episode not found")
	}
	result := &domain.MetadataResult{
		Title:    ep.Name,
		Overview: ep.Overview,
	}
	if ep.VoteAverage > 0 {
		r := ep.VoteAverage
		result.CommunityRating = &r
	}
	if ep.StillPath != "" {
		result.Artwork = []domain.ArtworkResult{{
			Type: "backdrop",
			URL:  imageBaseURL + ep.StillPath,
		}}
	}
	if t := parseDate(ep.AirDate); t != nil {
		result.PremiereDate = t
		y := t.Year()
		result.Year = &y
	}
	return result, nil
}

func (p *TMDBProvider) GetByProviderID(ctx context.Context, providerID string, mediaType string) (*domain.MetadataResult, error) {
	endpoint, err := detailEndpoint(mediaType, providerID)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"append_to_response":  {"credits,keywords,images,external_ids"},
		"language":            {p.language},
		"include_image_language": {p.language + ",en,null"},
	}
	u := fmt.Sprintf("%s%s?%s", p.baseURL, endpoint, params.Encode())

	body, err := p.doRequestWithRetry(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("tmdb detail request: %w", err)
	}

	var detail tmdbDetailResponse
	if err := json.Unmarshal(body, &detail); err != nil {
		return nil, fmt.Errorf("tmdb detail unmarshal: %w", err)
	}

	result := mapDetailResult(detail, mediaType)
	return &result, nil
}

func (p *TMDBProvider) doRequestWithRetry(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryWait * time.Duration(attempt)):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
		req.Header.Set("Accept", "application/json")

		resp, err := p.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("rate limited (429)")
			wait := retryWait * time.Duration(attempt+1)
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, err := strconv.Atoi(ra); err == nil {
					wait = time.Duration(secs) * time.Second
				}
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("tmdb api status %d: %s", resp.StatusCode, string(body))
		}

		return body, nil
	}
	return nil, fmt.Errorf("tmdb request failed after retries: %w", lastErr)
}

func searchEndpoint(mediaType string) (string, error) {
	switch mediaType {
	case "movie":
		return "/search/movie", nil
	case "tv":
		return "/search/tv", nil
	default:
		return "", fmt.Errorf("unsupported media type: %s", mediaType)
	}
}

func detailEndpoint(mediaType, id string) (string, error) {
	switch mediaType {
	case "movie":
		return "/movie/" + id, nil
	case "tv":
		return "/tv/" + id, nil
	default:
		return "", fmt.Errorf("unsupported media type: %s", mediaType)
	}
}

func genreMapForType(mediaType string) map[int]string {
	if mediaType == "tv" {
		return tvGenres
	}
	return movieGenres
}

func mapSearchResult(item tmdbSearchItem, mediaType string, genreMap map[int]string) domain.MetadataResult {
	result := domain.MetadataResult{
		ProviderIDs: map[string]string{"tmdb": strconv.Itoa(item.ID)},
		Popularity:  item.Popularity,
	}

	if mediaType == "tv" {
		result.Title = item.Name
		result.OriginalTitle = item.OriginalName
		result.PremiereDate = parseDate(item.FirstAirDate)
	} else {
		result.Title = item.Title
		result.OriginalTitle = item.OriginalTitle
		result.PremiereDate = parseDate(item.ReleaseDate)
	}

	result.Overview = item.Overview

	if result.PremiereDate != nil {
		y := result.PremiereDate.Year()
		result.Year = &y
	}

	if item.VoteAverage > 0 {
		r := item.VoteAverage
		result.CommunityRating = &r
	}

	for _, gid := range item.GenreIDs {
		if name, ok := genreMap[gid]; ok {
			result.Genres = append(result.Genres, name)
		}
	}

	return result
}

func mapDetailResult(d tmdbDetailResponse, mediaType string) domain.MetadataResult {
	result := domain.MetadataResult{
		ProviderIDs: map[string]string{"tmdb": strconv.Itoa(d.ID)},
		Ratings:     map[string]float64{},
		Tagline:     d.Tagline,
		Overview:    d.Overview,
	}

	if d.ExternalIDs != nil {
		if d.ExternalIDs.IMDBID != "" {
			result.ProviderIDs["imdb"] = d.ExternalIDs.IMDBID
		}
		if d.ExternalIDs.TVDBID > 0 {
			result.ProviderIDs["thetvdb"] = strconv.Itoa(d.ExternalIDs.TVDBID)
		}
	}

	if mediaType == "tv" {
		result.Title = d.Name
		result.OriginalTitle = d.OriginalName
		result.PremiereDate = parseDate(d.FirstAirDate)
	} else {
		result.Title = d.Title
		result.OriginalTitle = d.OriginalTitle
		result.PremiereDate = parseDate(d.ReleaseDate)
	}

	if result.PremiereDate != nil {
		y := result.PremiereDate.Year()
		result.Year = &y
	}

	if d.VoteAverage > 0 {
		r := d.VoteAverage
		result.CommunityRating = &r
		result.Ratings["tmdb"] = d.VoteAverage
	}

	for _, g := range d.Genres {
		result.Genres = append(result.Genres, g.Name)
	}

	for _, s := range d.ProductionCompanies {
		result.Studios = append(result.Studios, s.Name)
	}

	if d.Credits != nil {
		for _, c := range d.Credits.Cast {
			result.People = append(result.People, domain.PersonResult{
				Name:      c.Name,
				Role:      "Actor",
				Character: c.Character,
				ThumbURL:  profileURL(c.ProfilePath),
			})
		}
		for _, c := range d.Credits.Crew {
			if c.Job == "Director" {
				result.People = append(result.People, domain.PersonResult{
					Name:     c.Name,
					Role:     "Director",
					ThumbURL: profileURL(c.ProfilePath),
				})
			}
		}
	}

	if d.Images != nil {
		for i, img := range d.Images.Posters {
			if i >= 5 {
				break
			}
			result.Artwork = append(result.Artwork, domain.ArtworkResult{
				Type:     "poster",
				URL:      imageBaseURL + img.FilePath,
				Width:    img.Width,
				Height:   img.Height,
				Language: img.ISO639_1,
				Provider: "tmdb",
			})
		}
		for i, img := range d.Images.Backdrops {
			if i >= 5 {
				break
			}
			result.Artwork = append(result.Artwork, domain.ArtworkResult{
				Type:     "backdrop",
				URL:      imageBaseURL + img.FilePath,
				Width:    img.Width,
				Height:   img.Height,
				Language: img.ISO639_1,
				Provider: "tmdb",
			})
		}
	}

	return result
}

func profileURL(path string) string {
	if path == "" {
		return ""
	}
	return imageBaseURL + path
}

func parseDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

// TMDb API response types

type tmdbSearchResponse struct {
	Page         int              `json:"page"`
	TotalPages   int              `json:"total_pages"`
	TotalResults int              `json:"total_results"`
	Results      []tmdbSearchItem `json:"results"`
}

type tmdbSearchItem struct {
	ID            int     `json:"id"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title"`
	Name          string  `json:"name"`
	OriginalName  string  `json:"original_name"`
	Overview      string  `json:"overview"`
	ReleaseDate   string  `json:"release_date"`
	FirstAirDate  string  `json:"first_air_date"`
	VoteAverage   float64 `json:"vote_average"`
	VoteCount     int     `json:"vote_count"`
	Popularity    float64 `json:"popularity"`
	GenreIDs      []int   `json:"genre_ids"`
}

type tmdbDetailResponse struct {
	ID                  int                `json:"id"`
	Title               string             `json:"title"`
	OriginalTitle       string             `json:"original_title"`
	Name                string             `json:"name"`
	OriginalName        string             `json:"original_name"`
	Overview            string             `json:"overview"`
	Tagline             string             `json:"tagline"`
	ReleaseDate         string             `json:"release_date"`
	FirstAirDate        string             `json:"first_air_date"`
	VoteAverage         float64            `json:"vote_average"`
	Genres              []tmdbGenre        `json:"genres"`
	ProductionCompanies []tmdbCompany      `json:"production_companies"`
	Credits             *tmdbCredits       `json:"credits"`
	Images              *tmdbImages        `json:"images"`
	ExternalIDs         *tmdbExternalIDs   `json:"external_ids"`
}

type tmdbExternalIDs struct {
	IMDBID  string `json:"imdb_id"`
	TVDBID  int    `json:"tvdb_id"`
}

type tmdbGenre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type tmdbCompany struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type tmdbCredits struct {
	Cast []tmdbCastMember `json:"cast"`
	Crew []tmdbCrewMember `json:"crew"`
}

type tmdbCastMember struct {
	Name        string `json:"name"`
	Character   string `json:"character"`
	ProfilePath string `json:"profile_path"`
}

type tmdbCrewMember struct {
	Name        string `json:"name"`
	Job         string `json:"job"`
	ProfilePath string `json:"profile_path"`
}

type tmdbImages struct {
	Posters   []tmdbImage `json:"posters"`
	Backdrops []tmdbImage `json:"backdrops"`
}

type tmdbImage struct {
	FilePath  string `json:"file_path"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	ISO639_1  string `json:"iso_639_1"`
}
