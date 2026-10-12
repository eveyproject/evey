package metadata

import (
	"errors"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
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
	// CertificationsByCountry holds per-country content ratings when a provider
	// (e.g. TMDB release_dates / content_ratings) exposes them.
	CertificationsByCountry map[string]string
	Genres                  []string
	Studios                 []string
	ProviderIDs             map[string]string
	Ratings                 map[string]float64 // imdb, rt, metacritic, tmdb
	People                  []PersonResult
	Artwork                 []ArtworkResult
	Trailers                []TrailerResult
	Popularity              float64 // TMDB popularity score, used as tiebreaker
	Collection              *CollectionRef
	// TV series structure info (for match validation)
	SeasonEpisodeCounts map[int]int // season_number → episode_count
	// AlternativeTitles holds locale-variant release titles (e.g. the US
	// English title for a film that TMDB returns under its localised name).
	// Populated lazily on the ambiguous-match path; empty otherwise.
	AlternativeTitles []string
}

type TrailerResult struct {
	Site             string // "YouTube", "Vimeo", "Dailymotion", etc.
	Key              string // platform video ID
	Language         string // ISO 639-1 - trailer audio language
	Official         bool
	Type             string // "Trailer", "Teaser", "Clip", etc.
	UseSubtitles     bool   // true when trailer is not in server language
	SubtitleLanguage string // ISO 639-1 - language for CC (= server lang when UseSubtitles=true)
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
	// Blurhash is a compact (~30-char) representation of the image's
	// low-frequency content. Populated at download time and used as an
	// instant placeholder by clients while the full image is still loading.
	// Nil when generation failed or pre-dates the feature.
	Blurhash *string
}

func (o *MetadataOverride) Validate() error {
	if o.ID == "" {
		return errors.New("metadata override requires an ID")
	}
	if o.MediaItemID == "" {
		return errors.New("metadata override requires a media item ID")
	}
	if o.FieldName == "" {
		return errors.New("metadata override requires a field name")
	}
	return nil
}

func (a *ArtworkRecord) Validate() error {
	if a.ID == "" {
		return errors.New("artwork record requires an ID")
	}
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

type MetadataOverrideOption func(*MetadataOverride)

func WithOverrideValue(v any) MetadataOverrideOption {
	return func(o *MetadataOverride) { o.CustomValue = v }
}

func WithOverrideEditor(userID shared.UserID) MetadataOverrideOption {
	return func(o *MetadataOverride) { o.EditedBy = userID }
}

// NewMetadataOverride is the canonical constructor. Stamps CreatedAt
// and validates. Required identity fields (id, mediaItem, fieldName)
// are surfaced as params so construction never succeeds with them
// missing.
func NewMetadataOverride(id string, mediaItemID shared.MediaItemID, fieldName string, opts ...MetadataOverrideOption) (*MetadataOverride, error) {
	o := &MetadataOverride{
		ID:          id,
		MediaItemID: mediaItemID,
		FieldName:   fieldName,
		CreatedAt:   time.Now(),
	}
	for _, opt := range opts {
		opt(o)
	}
	if err := o.Validate(); err != nil {
		return nil, err
	}
	return o, nil
}

type ArtworkRecordOption func(*ArtworkRecord)

func WithArtworkSource(url string) ArtworkRecordOption {
	return func(a *ArtworkRecord) { a.SourceURL = &url }
}

func WithArtworkDimensions(width, height int) ArtworkRecordOption {
	return func(a *ArtworkRecord) { a.Width, a.Height = &width, &height }
}

func WithArtworkLanguage(lang string) ArtworkRecordOption {
	return func(a *ArtworkRecord) { a.Language = &lang }
}

func WithArtworkSortOrder(order int) ArtworkRecordOption {
	return func(a *ArtworkRecord) { a.SortOrder = order }
}

// NewArtworkRecord is the canonical constructor. Validates. Required
// identity fields (id, mediaItem, type, provider) are surfaced as
// params so construction never succeeds with them missing.
func NewArtworkRecord(id string, mediaItemID shared.MediaItemID, artType, provider string, opts ...ArtworkRecordOption) (*ArtworkRecord, error) {
	a := &ArtworkRecord{
		ID:          id,
		MediaItemID: mediaItemID,
		Type:        artType,
		Provider:    provider,
	}
	for _, opt := range opts {
		opt(a)
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	return a, nil
}

type MetadataProviderConfig struct {
	Name     string
	Enabled  bool
	Priority int
}
