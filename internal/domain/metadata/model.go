package metadata

import "time"

type MetadataResult struct {
	Title           string
	OriginalTitle   string
	Overview        string
	Tagline         string
	PremiereDate    *time.Time
	Year            *int
	CommunityRating *float64
	OfficialRating  string
	Genres          []string
	Studios         []string
	ProviderIDs     map[string]string
	Ratings         map[string]float64 // imdb, rt, metacritic, tmdb
	People          []PersonResult
	Artwork         []ArtworkResult
	Popularity      float64 // TMDB popularity score, used as tiebreaker
}

type PersonResult struct {
	Name      string
	Role      string
	Character string
	ThumbURL  string
}

type ArtworkResult struct {
	Type     string
	URL      string
	Width    int
	Height   int
	Language string
	Provider string
}

type MetadataOverride struct {
	ID          string
	MediaItemID string
	FieldName   string
	CustomValue any
	EditedBy    string
	CreatedAt   time.Time
}

type ArtworkRecord struct {
	ID          string
	MediaItemID string
	PersonID    *string
	Type        string
	Provider    string
	SourceURL   *string
	LocalPath   *string
	Width       *int
	Height      *int
	Language    *string
	SortOrder   int
}

type MetadataProviderConfig struct {
	Name     string
	Enabled  bool
	Priority int
}
