package metadata

import (
	"time"
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

const (
	defaultBaseURL = "https://api.themoviedb.org/3"
	imageBaseURL   = "https://image.tmdb.org/t/p/original"
	maxRetries     = 3
	retryWait      = 500 * time.Millisecond
	cacheTTL       = time.Hour
)

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
