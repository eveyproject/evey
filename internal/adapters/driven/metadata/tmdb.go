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

	"github.com/gravity-zero/evey/internal/pkg/cache"
	domain "github.com/gravity-zero/evey/internal/domain/metadata"
)

const (
	defaultBaseURL = "https://api.themoviedb.org/3"
	imageBaseURL   = "https://image.tmdb.org/t/p/original"
	maxRetries     = 3
	retryWait      = 500 * time.Millisecond
	cacheTTL       = time.Hour
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
	apiKey      string
	baseURL     string
	language    string // ISO 639-1, e.g. "en", "fr"
	httpClient  *http.Client
	searchCache *cache.TTLCache[[]domain.MetadataResult]
	detailCache *cache.TTLCache[domain.MetadataResult]
	stopCleanup func()
}

func NewTMDBProvider(apiKey string, language string, httpClient *http.Client) *TMDBProvider {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if language == "" {
		language = "en"
	}
	sc, stopSC := cache.New[[]domain.MetadataResult](10 * time.Minute)
	dc, stopDC := cache.New[domain.MetadataResult](10 * time.Minute)
	return &TMDBProvider{
		apiKey:      apiKey,
		baseURL:     defaultBaseURL,
		language:    language,
		httpClient:  httpClient,
		searchCache: sc,
		detailCache: dc,
		stopCleanup: func() { stopSC(); stopDC() },
	}
}

func (p *TMDBProvider) Name() string { return "tmdb" }

func (p *TMDBProvider) Search(ctx context.Context, query string, year *int, mediaType string) ([]domain.MetadataResult, error) {
	endpoint, err := searchEndpoint(mediaType)
	if err != nil {
		return nil, err
	}

	cacheKey := "tmdb:search:" + mediaType + ":" + query
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
	u := fmt.Sprintf("%s/tv/%s/season/%d?language=%s&append_to_response=images",
		p.baseURL, seriesID, seasonNum, p.language)
	body, err := p.doRequestWithRetry(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("tmdb season request: %w", err)
	}
	var s struct {
		Name        string  `json:"name"`
		Overview    string  `json:"overview"`
		PosterPath  string  `json:"poster_path"`
		AirDate     string  `json:"air_date"`
		VoteAverage float64 `json:"vote_average"`
	}
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, fmt.Errorf("tmdb season unmarshal: %w", err)
	}
	result := &domain.MetadataResult{
		Title:    s.Name,
		Overview: s.Overview,
	}
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
	cacheKey := "tmdb:detail:" + mediaType + ":" + providerID
	if cached, ok := p.detailCache.Get(cacheKey); ok {
		return &cached, nil
	}

	endpoint, err := detailEndpoint(mediaType, providerID)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"append_to_response":     {"credits,keywords,images,external_ids,videos"},
		"language":               {p.language},
		"include_image_language": {p.language + ",en,null"},
		"include_video_language": {p.language + ",en,null"},
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

	result := mapDetailResult(detail, mediaType, p.language)
	p.detailCache.Set(cacheKey, result, cacheTTL)
	return &result, nil
}

// FetchTranslations fetches title/overview/tagline in each locale, skipping the provider's default language.
func (p *TMDBProvider) FetchTranslations(ctx context.Context, providerID string, mediaType string, locales []string) (map[string]domain.MetadataResult, error) {
	results := make(map[string]domain.MetadataResult)
	for _, locale := range locales {
		if locale == p.language {
			continue
		}
		endpoint, err := detailEndpoint(mediaType, providerID)
		if err != nil {
			return nil, err
		}
		params := url.Values{
			"language": {locale},
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
		} else {
			r.Title = detail.Title
		}
		results[locale] = r
	}
	return results, nil
}

// FetchPersonTranslations fetches biography in each locale for a person by TMDB ID.
func (p *TMDBProvider) FetchPersonTranslations(ctx context.Context, personID string, locales []string) (map[string]string, error) {
	results := make(map[string]string)
	for _, locale := range locales {
		if locale == p.language {
			continue
		}
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

func mapDetailResult(d tmdbDetailResponse, mediaType string, serverLang string) domain.MetadataResult {
	result := domain.MetadataResult{
		ProviderIDs: map[string]string{"tmdb": strconv.Itoa(d.ID)},
		Ratings:     map[string]float64{},
		Tagline:     d.Tagline,
		Overview:    d.Overview,
	}

	mapDetailExternalIDs(&result, d.ExternalIDs)
	mapDetailTitleAndDate(&result, d, mediaType)
	mapDetailRating(&result, d.VoteAverage)

	for _, g := range d.Genres {
		result.Genres = append(result.Genres, g.Name)
	}
	for _, s := range d.ProductionCompanies {
		result.Studios = append(result.Studios, s.Name)
	}

	result.People = mapDetailPeople(d.Credits)
	result.Artwork = mapDetailArtwork(d.Images)

	if d.Videos != nil {
		result.Trailers = selectTrailers(d.Videos.Results, serverLang)
	}

	result.Collection = mapDetailCollection(d.BelongsToCollection)

	if len(d.Seasons) > 0 {
		result.SeasonEpisodeCounts = make(map[int]int, len(d.Seasons))
		for _, s := range d.Seasons {
			result.SeasonEpisodeCounts[s.SeasonNumber] = s.EpisodeCount
		}
	}

	return result
}

func mapDetailExternalIDs(result *domain.MetadataResult, ext *tmdbExternalIDs) {
	if ext == nil {
		return
	}
	if ext.IMDBID != "" {
		result.ProviderIDs["imdb"] = ext.IMDBID
	}
	if ext.TVDBID > 0 {
		result.ProviderIDs["thetvdb"] = strconv.Itoa(ext.TVDBID)
	}
}

func mapDetailTitleAndDate(result *domain.MetadataResult, d tmdbDetailResponse, mediaType string) {
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
}

func mapDetailRating(result *domain.MetadataResult, voteAverage float64) {
	if voteAverage <= 0 {
		return
	}
	r := voteAverage
	result.CommunityRating = &r
	result.Ratings["tmdb"] = voteAverage
}

func mapDetailPeople(credits *tmdbCredits) []domain.PersonResult {
	if credits == nil {
		return nil
	}
	var people []domain.PersonResult
	for _, c := range credits.Cast {
		people = append(people, domain.PersonResult{
			Name:      c.Name,
			Role:      "Actor",
			Character: c.Character,
			ThumbURL:  profileURL(c.ProfilePath),
		})
	}
	for _, c := range credits.Crew {
		if c.Job == "Director" {
			people = append(people, domain.PersonResult{
				Name:     c.Name,
				Role:     "Director",
				ThumbURL: profileURL(c.ProfilePath),
			})
		}
	}
	return people
}

func mapDetailArtwork(images *tmdbImages) []domain.ArtworkResult {
	if images == nil {
		return nil
	}
	var artwork []domain.ArtworkResult
	artwork = appendImages(artwork, images.Posters, domain.ArtworkTypePoster, 5)
	artwork = appendImages(artwork, images.Backdrops, domain.ArtworkTypeBackdrop, 5)
	artwork = appendImages(artwork, images.Logos, domain.ArtworkTypeLogo, 3)
	return artwork
}

func appendImages(dst []domain.ArtworkResult, imgs []tmdbImage, artType string, limit int) []domain.ArtworkResult {
	for i, img := range imgs {
		if i >= limit {
			break
		}
		dst = append(dst, domain.ArtworkResult{
			Type:     artType,
			URL:      imageBaseURL + img.FilePath,
			Width:    img.Width,
			Height:   img.Height,
			Language: img.ISO639_1,
			Provider: "tmdb",
		})
	}
	return dst
}

func mapDetailCollection(col *tmdbCollection) *domain.CollectionRef {
	if col == nil {
		return nil
	}
	ref := &domain.CollectionRef{
		TmdbID: col.ID,
		Name:   col.Name,
	}
	if col.PosterPath != "" {
		ref.PosterURL = "https://image.tmdb.org/t/p/w500" + col.PosterPath
	}
	if col.BackdropPath != "" {
		ref.BackdropURL = imageBaseURL + col.BackdropPath
	}
	return ref
}

const maxTrailersPerLanguage = 3

// selectTrailers filters videos to type "Trailer", keeps max 3 per language,
// sorted official-first within each language group.
func selectTrailers(videos []tmdbVideo, _ string) []domain.TrailerResult {
	sortByOfficial := func(a, b tmdbVideo) int {
		if a.Official && !b.Official {
			return -1
		}
		if !a.Official && b.Official {
			return 1
		}
		return 0
	}

	byLang := map[string][]tmdbVideo{}
	for _, v := range videos {
		if v.Type != "Trailer" || v.Key == "" {
			continue
		}
		byLang[v.Language] = append(byLang[v.Language], v)
	}

	var result []domain.TrailerResult
	for _, vids := range byLang {
		sortVideos(vids, sortByOfficial)
		for i, v := range vids {
			if i >= maxTrailersPerLanguage {
				break
			}
			result = append(result, domain.TrailerResult{
				Site:     v.Site,
				Key:      v.Key,
				Language: v.Language,
				Official: v.Official,
				Type:     v.Type,
			})
		}
	}
	return result
}

func sortVideos(vs []tmdbVideo, less func(a, b tmdbVideo) int) {
	for i := 1; i < len(vs); i++ {
		for j := i; j > 0 && less(vs[j], vs[j-1]) < 0; j-- {
			vs[j], vs[j-1] = vs[j-1], vs[j]
		}
	}
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

// PersonDetails contains biographical info from TMDB.
type PersonDetails struct {
	TMDBID       int
	Name         string
	Biography    string
	Birthday     string
	Deathday     string
	PlaceOfBirth string
	KnownFor     string
	ThumbURL     string
}

type tmdbPersonSearchResult struct {
	Results []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"results"`
}

type tmdbPersonDetail struct {
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	Biography          string `json:"biography"`
	Birthday           string `json:"birthday"`
	Deathday           string `json:"deathday"`
	PlaceOfBirth       string `json:"place_of_birth"`
	KnownForDepartment string `json:"known_for_department"`
	ProfilePath        string `json:"profile_path"`
}

// GetPersonDetails searches TMDB for a person by name and returns biographical info.
func (p *TMDBProvider) GetPersonDetails(ctx context.Context, name string) (*PersonDetails, error) {
	searchURL := fmt.Sprintf("%s/search/person?query=%s&language=%s", p.baseURL, url.QueryEscape(name), p.language)
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

	detailURL := fmt.Sprintf("%s/person/%d?language=%s", p.baseURL, tmdbID, p.language)
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

type tmdbCollection struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	PosterPath   string `json:"poster_path"`
	BackdropPath string `json:"backdrop_path"`
}

type tmdbDetailResponse struct {
	ID                   int              `json:"id"`
	Title                string           `json:"title"`
	OriginalTitle        string           `json:"original_title"`
	Name                 string           `json:"name"`
	OriginalName         string           `json:"original_name"`
	Overview             string           `json:"overview"`
	Tagline              string           `json:"tagline"`
	ReleaseDate          string           `json:"release_date"`
	FirstAirDate         string           `json:"first_air_date"`
	VoteAverage          float64          `json:"vote_average"`
	NumberOfSeasons      int              `json:"number_of_seasons"`
	NumberOfEpisodes     int              `json:"number_of_episodes"`
	Seasons              []tmdbSeasonInfo `json:"seasons"`
	Genres               []tmdbGenre      `json:"genres"`
	ProductionCompanies  []tmdbCompany    `json:"production_companies"`
	Credits             *tmdbCredits     `json:"credits"`
	Images              *tmdbImages      `json:"images"`
	Videos              *tmdbVideos      `json:"videos"`
	ExternalIDs         *tmdbExternalIDs `json:"external_ids"`
	BelongsToCollection *tmdbCollection  `json:"belongs_to_collection"`
}

type tmdbSeasonInfo struct {
	SeasonNumber int `json:"season_number"`
	EpisodeCount int `json:"episode_count"`
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
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Character   string `json:"character"`
	ProfilePath string `json:"profile_path"`
}

type tmdbCrewMember struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Job         string `json:"job"`
	ProfilePath string `json:"profile_path"`
}

type tmdbImages struct {
	Posters   []tmdbImage `json:"posters"`
	Backdrops []tmdbImage `json:"backdrops"`
	Logos     []tmdbImage `json:"logos"`
}

type tmdbImage struct {
	FilePath string `json:"file_path"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	ISO639_1 string `json:"iso_639_1"`
}

type tmdbVideos struct {
	Results []tmdbVideo `json:"results"`
}

type tmdbVideo struct {
	Key      string `json:"key"`
	Site     string `json:"site"`
	Type     string `json:"type"`
	Language string `json:"iso_639_1"`
	Official bool   `json:"official"`
}
