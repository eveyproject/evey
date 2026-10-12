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
	"time"

	domain "github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/pkg/cache"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type TMDBProvider struct {
	apiKey        string
	baseURL       string
	languageFn    func() string
	httpClient    *http.Client
	searchCache   *cache.TTLCache[[]domain.MetadataResult]
	detailCache   *cache.TTLCache[domain.MetadataResult]
	altTitleCache *cache.TTLCache[[]string]
	stopCleanup   func()
}

func (p *TMDBProvider) language() string { return p.languageFn() }

// Language exposes the provider's current language so a fronting cache can key
// on it (implements domain.LanguageAware).
func (p *TMDBProvider) Language() string { return p.language() }

func NewTMDBProviderWithGetter(apiKey string, languageFn func() string, httpClient *http.Client) *TMDBProvider {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	sc, stopSC := cache.New[[]domain.MetadataResult](10 * time.Minute)
	dc, stopDC := cache.New[domain.MetadataResult](10 * time.Minute)
	ac, stopAC := cache.New[[]string](10 * time.Minute)
	return &TMDBProvider{
		apiKey:        apiKey,
		baseURL:       defaultBaseURL,
		languageFn:    languageFn,
		httpClient:    httpClient,
		searchCache:   sc,
		detailCache:   dc,
		altTitleCache: ac,
		stopCleanup:   func() { stopSC(); stopDC(); stopAC() },
	}
}

func NewTMDBProvider(apiKey string, language string, httpClient *http.Client) *TMDBProvider {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if language == "" {
		language = "en"
	}
	lang := language
	sc, stopSC := cache.New[[]domain.MetadataResult](10 * time.Minute)
	dc, stopDC := cache.New[domain.MetadataResult](10 * time.Minute)
	ac, stopAC := cache.New[[]string](10 * time.Minute)
	return &TMDBProvider{
		apiKey:        apiKey,
		baseURL:       defaultBaseURL,
		languageFn:    func() string { return lang },
		httpClient:    httpClient,
		searchCache:   sc,
		detailCache:   dc,
		altTitleCache: ac,
		stopCleanup:   func() { stopSC(); stopDC(); stopAC() },
	}
}

func (p *TMDBProvider) Name() string { return "tmdb" }

func (p *TMDBProvider) Search(ctx context.Context, query string, year *int, mediaType string) ([]domain.MetadataResult, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.tmdb.Search")
	defer span.End()

	endpoint, err := searchEndpoint(mediaType)
	if err != nil {
		return nil, err
	}

	// Language is part of the key: the same query in a different language is a
	// different result, and omitting it let a result fetched in one language
	// stick under another (the cause of synopses frozen in the wrong language).
	cacheKey := "tmdb:search:" + p.language() + ":" + mediaType + ":" + query
	if year != nil {
		cacheKey += ":" + strconv.Itoa(*year)
	}
	if cached, ok := p.searchCache.Get(cacheKey); ok {
		return cached, nil
	}

	var results []domain.MetadataResult
	if year != nil {
		results, err = p.searchPages(ctx, endpoint, query, year, mediaType)
		if err != nil {
			return nil, err
		}
	}
	if len(results) == 0 {
		results, err = p.searchPages(ctx, endpoint, query, nil, mediaType)
		if err != nil {
			return nil, err
		}
	}

	p.searchCache.Set(cacheKey, results, cacheTTL)
	return results, nil
}

func (p *TMDBProvider) searchPages(ctx context.Context, endpoint, query string, year *int, mediaType string) ([]domain.MetadataResult, error) {
	var allResults []domain.MetadataResult
	genreMap := genreMapForType(mediaType)

	for page := 1; ; page++ {
		params := url.Values{
			"query":    {query},
			"page":     {strconv.Itoa(page)},
			"language": {p.language()},
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
	ctx, span := tracing.StartSpan(ctx, "metadata.tmdb.GetEpisodeDetails")
	defer span.End()

	u := fmt.Sprintf("%s/tv/%s/season/%d/episode/%d?language=%s&append_to_response=images,external_ids",
		p.baseURL, seriesID, seasonNum, episodeNum, p.language())
	body, err := p.doRequestWithRetry(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("tmdb episode request: %w", err)
	}
	var ep struct {
		ID          int     `json:"id"`
		Name        string  `json:"name"`
		Overview    string  `json:"overview"`
		StillPath   string  `json:"still_path"`
		VoteAverage float64 `json:"vote_average"`
		AirDate     string  `json:"air_date"`
		// external_ids appended via append_to_response saves a second
		// round-trip (the alternative was calling /tv/{id}/season/{n}/
		// episode/{e}/external_ids separately). Fields documented at
		// https://developers.themoviedb.org/3/tv-episodes/get-tv-episode-external-ids
		ExternalIDs *tmdbExternalIDs `json:"external_ids"`
	}
	if err := json.Unmarshal(body, &ep); err != nil {
		return nil, fmt.Errorf("tmdb episode unmarshal: %w", err)
	}
	if ep.Name == "" {
		return nil, errors.New("episode not found")
	}
	result := &domain.MetadataResult{
		Title:    ep.Name,
		Overview: ep.Overview,
	}
	// Persist the episode's own TMDB ID so the row leaves the orphan set,
	// plus IMDB/TVDB IDs if TMDB knows them - saves the admin a manual
	// "Identify on IMDB" round-trip later.
	result.ProviderIDs = map[string]string{}
	if ep.ID > 0 {
		result.ProviderIDs["tmdb"] = strconv.Itoa(ep.ID)
	}
	mapDetailExternalIDs(result, ep.ExternalIDs)
	if ep.VoteAverage > 0 {
		r := ep.VoteAverage
		result.CommunityRating = &r
	}
	if ep.StillPath != "" {
		result.Artwork = []domain.ArtworkResult{{
			Type:     "backdrop",
			URL:      imageBaseURL + ep.StillPath,
			Provider: "tmdb",
		}}
	}
	if t := parseDate(ep.AirDate); t != nil {
		result.PremiereDate = t
		y := t.Year()
		result.Year = &y
	}
	return result, nil
}

// GetSeasonDetails fetches metadata for a specific season of a TV series.
func (p *TMDBProvider) GetSeasonDetails(ctx context.Context, seriesID string, seasonNum int) (*domain.MetadataResult, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.tmdb.GetSeasonDetails")
	defer span.End()

	u := fmt.Sprintf("%s/tv/%s/season/%d?language=%s&append_to_response=images,external_ids",
		p.baseURL, seriesID, seasonNum, p.language())
	body, err := p.doRequestWithRetry(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("tmdb season request: %w", err)
	}
	var s struct {
		ID          int              `json:"id"`
		Name        string           `json:"name"`
		Overview    string           `json:"overview"`
		PosterPath  string           `json:"poster_path"`
		AirDate     string           `json:"air_date"`
		VoteAverage float64          `json:"vote_average"`
		ExternalIDs *tmdbExternalIDs `json:"external_ids"`
	}
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, fmt.Errorf("tmdb season unmarshal: %w", err)
	}
	result := &domain.MetadataResult{
		Title:    s.Name,
		Overview: s.Overview,
	}
	result.ProviderIDs = map[string]string{}
	if s.ID > 0 {
		result.ProviderIDs["tmdb"] = strconv.Itoa(s.ID)
	}
	mapDetailExternalIDs(result, s.ExternalIDs)
	if s.VoteAverage > 0 {
		r := s.VoteAverage
		result.CommunityRating = &r
	}
	if s.PosterPath != "" {
		result.Artwork = []domain.ArtworkResult{{
			Type:     "poster",
			URL:      imageBaseURL + s.PosterPath,
			Provider: "tmdb",
		}}
	}
	if t := parseDate(s.AirDate); t != nil {
		result.PremiereDate = t
		y := t.Year()
		result.Year = &y
	}
	return result, nil
}

func (p *TMDBProvider) GetByProviderID(ctx context.Context, providerID string, mediaType string) (*domain.MetadataResult, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.tmdb.GetByProviderID")
	defer span.End()

	cacheKey := "tmdb:detail:" + p.language() + ":" + mediaType + ":" + providerID
	if cached, ok := p.detailCache.Get(cacheKey); ok {
		return &cached, nil
	}

	endpoint, err := detailEndpoint(mediaType, providerID)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"append_to_response":     {"credits,keywords,images,external_ids,videos,release_dates,content_ratings,alternative_titles"},
		"language":               {p.language()},
		"include_image_language": {p.language() + ",en,null"},
		"include_video_language": {p.language() + ",en,null"},
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

	result := mapDetailResult(detail, mediaType, p.language())
	p.detailCache.Set(cacheKey, result, cacheTTL)
	return &result, nil
}

// LookupByExternalID resolves a TMDB record from an external provider ID
// (IMDB, TVDB, …) via TMDB's /find endpoint. Returns nil when the mapping
// yields no hit. Only imdb is wired for now - that's the path used by the
// admin "change IMDB ID" flow, which otherwise leaves the stale TMDB ID
// on the item and fetches translations for the wrong title.
func (p *TMDBProvider) LookupByExternalID(ctx context.Context, providerName, externalID string) (*domain.MetadataResult, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.tmdb.LookupByExternalID")
	defer span.End()

	if providerName != "imdb" || externalID == "" {
		return nil, nil
	}
	params := url.Values{
		"external_source": {"imdb_id"},
		"language":        {p.language()},
	}
	u := fmt.Sprintf("%s/find/%s?%s", p.baseURL, externalID, params.Encode())
	body, err := p.doRequestWithRetry(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("tmdb find: %w", err)
	}
	var findResp struct {
		MovieResults []struct {
			ID int `json:"id"`
		} `json:"movie_results"`
		TVResults []struct {
			ID int `json:"id"`
		} `json:"tv_results"`
	}
	if err := json.Unmarshal(body, &findResp); err != nil {
		return nil, fmt.Errorf("tmdb find unmarshal: %w", err)
	}
	var tmdbID string
	switch {
	case len(findResp.MovieResults) > 0:
		tmdbID = strconv.Itoa(findResp.MovieResults[0].ID)
	case len(findResp.TVResults) > 0:
		tmdbID = strconv.Itoa(findResp.TVResults[0].ID)
	default:
		return nil, nil
	}
	return &domain.MetadataResult{
		ProviderIDs: map[string]string{"tmdb": tmdbID, "imdb": externalID},
	}, nil
}

// FetchTranslations fetches title/overview/tagline for each requested locale.
//
// It deliberately does NOT skip the provider's default language: the read path
// overlays media_item_translations for every locale (including the default),
// so a row must exist for the default locale too. Skipping it left items whose
// base columns ended up in the wrong language (e.g. an OMDb-origin item later
// matched to TMDB) with no correct row to serve - permanently stuck in the
// wrong language with no re-enrichment able to fix it.
func (p *TMDBProvider) FetchTranslations(ctx context.Context, providerID string, mediaType string, locales []string) (map[string]domain.MetadataResult, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.tmdb.FetchTranslations")
	defer span.End()

	results := make(map[string]domain.MetadataResult)
	for _, locale := range locales {
		endpoint, err := detailEndpoint(mediaType, providerID)
		if err != nil {
			return nil, err
		}
		params := url.Values{
			"language":           {locale},
			"append_to_response": {"alternative_titles"},
		}
		u := fmt.Sprintf("%s%s?%s", p.baseURL, endpoint, params.Encode())
		body, err := p.doRequestWithRetry(ctx, u)
		if err != nil {
			continue
		}
		var detail tmdbDetailResponse
		if err := json.Unmarshal(body, &detail); err != nil {
			continue
		}
		r := domain.MetadataResult{
			Overview: detail.Overview,
			Tagline:  detail.Tagline,
		}
		if mediaType == "tv" {
			r.Title = detail.Name
			r.OriginalTitle = detail.OriginalName
		} else {
			r.Title = detail.Title
			r.OriginalTitle = detail.OriginalTitle
		}
		// Same regional fallback as the base enrichment: a foreign-language
		// work with no localized title in this locale takes its regional
		// alternative title - otherwise the per-locale translation row would
		// hold the original-language title and the read overlay would clobber
		// the (correct, regional) base title with it.
		applyRegionalTitle(&r, detail, locale)
		results[locale] = r
	}
	return results, nil
}

// FetchPersonTranslations fetches biography in each locale for a person by TMDB ID.
func (p *TMDBProvider) FetchPersonTranslations(ctx context.Context, personID string, locales []string) (map[string]string, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.tmdb.FetchPersonTranslations")
	defer span.End()

	// Like FetchTranslations, the default language is NOT skipped: the bio
	// overlay applies for every locale including the default, so a row is
	// needed there too.
	results := make(map[string]string)
	for _, locale := range locales {
		u := fmt.Sprintf("%s/person/%s?language=%s", p.baseURL, personID, locale)
		body, err := p.doRequestWithRetry(ctx, u)
		if err != nil {
			continue
		}
		var detail tmdbPersonDetail
		if err := json.Unmarshal(body, &detail); err != nil {
			continue
		}
		if detail.Biography != "" {
			results[locale] = detail.Biography
		}
	}
	return results, nil
}

// AlternativeTitles fetches alternative release titles for a movie or TV series
// from TMDB. Movie titles are under the `titles` key; TV titles use `results`.
// Results are cached to bound extra calls on the ambiguous-match path.
func (p *TMDBProvider) AlternativeTitles(ctx context.Context, providerID string, mediaType string) ([]string, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.tmdb.AlternativeTitles")
	defer span.End()

	cacheKey := "tmdb:alttitles:" + mediaType + ":" + providerID
	if cached, ok := p.altTitleCache.Get(cacheKey); ok {
		return cached, nil
	}

	var u string
	switch mediaType {
	case "movie":
		u = fmt.Sprintf("%s/movie/%s/alternative_titles", p.baseURL, providerID)
	case "tv":
		u = fmt.Sprintf("%s/tv/%s/alternative_titles", p.baseURL, providerID)
	default:
		return nil, fmt.Errorf("unsupported media type for alternative_titles: %s", mediaType)
	}

	body, err := p.doRequestWithRetry(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("tmdb alternative_titles request: %w", err)
	}

	// Movie response: {"id":…,"titles":[{"iso_3166_1":"US","title":"…"},…]}
	// TV response:    {"id":…,"results":[{"iso_3166_1":"US","title":"…"},…]}
	var resp struct {
		Titles []struct {
			Title string `json:"title"`
		} `json:"titles"`
		Results []struct {
			Title string `json:"title"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("tmdb alternative_titles unmarshal: %w", err)
	}

	seen := make(map[string]bool)
	var titles []string
	for _, t := range resp.Titles {
		if t.Title != "" && !seen[t.Title] {
			seen[t.Title] = true
			titles = append(titles, t.Title)
		}
	}
	for _, t := range resp.Results {
		if t.Title != "" && !seen[t.Title] {
			seen[t.Title] = true
			titles = append(titles, t.Title)
		}
	}

	p.altTitleCache.Set(cacheKey, titles, cacheTTL)
	return titles, nil
}

// GetPersonDetails searches TMDB for a person by name and returns biographical info.
func (p *TMDBProvider) GetPersonDetails(ctx context.Context, name string) (*PersonDetails, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.tmdb.GetPersonDetails")
	defer span.End()

	searchURL := fmt.Sprintf("%s/search/person?query=%s&language=%s", p.baseURL, url.QueryEscape(name), p.language())
	body, err := p.doRequestWithRetry(ctx, searchURL)
	if err != nil {
		return nil, fmt.Errorf("tmdb search person: %w", err)
	}
	var searchResp tmdbPersonSearchResult
	if err = json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("tmdb decode search person: %w", err)
	}
	if len(searchResp.Results) == 0 {
		return nil, nil
	}
	tmdbID := searchResp.Results[0].ID

	detailURL := fmt.Sprintf("%s/person/%d?language=%s", p.baseURL, tmdbID, p.language())
	body, err = p.doRequestWithRetry(ctx, detailURL)
	if err != nil {
		return nil, fmt.Errorf("tmdb get person detail: %w", err)
	}
	var detail tmdbPersonDetail
	if err = json.Unmarshal(body, &detail); err != nil {
		return nil, fmt.Errorf("tmdb decode person detail: %w", err)
	}
	return &PersonDetails{
		TMDBID:       detail.ID,
		Name:         detail.Name,
		Biography:    detail.Biography,
		Birthday:     detail.Birthday,
		Deathday:     detail.Deathday,
		PlaceOfBirth: detail.PlaceOfBirth,
		KnownFor:     detail.KnownForDepartment,
		ThumbURL:     profileURL(detail.ProfilePath),
	}, nil
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
		body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10MB max
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			lastErr = errors.New("rate limited (429)")
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
