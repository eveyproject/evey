package metadata

import (
	"errors"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

const (
	ArtworkTypePoster   = "poster"
	ArtworkTypeBackdrop = "backdrop"
	ArtworkTypeLogo     = "logo"
	ArtworkTypeStill    = "still"
	ArtworkTypeThumb    = "thumb"
)

type CollectionRef struct {
	TmdbID      int
	Name        string
	PosterURL   string
	BackdropURL string
}

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
	Trailers        []TrailerResult
	Popularity      float64 // TMDB popularity score, used as tiebreaker
	Collection      *CollectionRef
	// TV series structure info (for match validation)
	SeasonEpisodeCounts map[int]int // season_number → episode_count
}

type TrailerResult struct {
	Site             string // "YouTube", "Vimeo", "Dailymotion", etc.
	Key              string // platform video ID
	Language         string // ISO 639-1 — trailer audio language
	Official         bool
	Type             string // "Trailer", "Teaser", "Clip", etc.
	UseSubtitles     bool   // true when trailer is not in server language
	SubtitleLanguage string // ISO 639-1 — language for CC (= server lang when UseSubtitles=true)
}

type PersonResult struct {
	Name       string
	Role       string
	Character  string
	ThumbURL   string
	ProviderID string // TMDB person ID (for fetching bio/translations)
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
	MediaItemID shared.MediaItemID
	FieldName   string
	CustomValue any
	EditedBy    shared.UserID
	CreatedAt   time.Time
}

type ArtworkRecord struct {
	ID          string
	MediaItemID shared.MediaItemID
	PersonID    *string
	Type        string
	Provider    string
	SourceURL   *string
	LocalPath   *string
	WebPPath    *string
	Width       *int
	Height      *int
	Language    *string
	SortOrder   int
}

func (o *MetadataOverride) Validate() error {
	if o.MediaItemID == "" {
		return errors.New("metadata override requires a media item ID")
	}
	if o.FieldName == "" {
		return errors.New("metadata override requires a field name")
	}
	return nil
}

func (a *ArtworkRecord) Validate() error {
	if a.MediaItemID == "" {
		return errors.New("artwork record requires a media item ID")
	}
	if a.Type == "" {
		return errors.New("artwork record requires a type")
	}
	if a.Provider == "" {
		return errors.New("artwork record requires a provider")
	}
	return nil
}

type MetadataProviderConfig struct {
	Name     string
	Enabled  bool
	Priority int
}
