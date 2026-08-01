package metadata

import "context"

type MetadataProvider interface {
	Name() string
	Search(ctx context.Context, query string, year *int, mediaType string) ([]MetadataResult, error)
	GetByProviderID(ctx context.Context, providerID string, mediaType string) (*MetadataResult, error)
}

// ProviderUnwrapper is implemented by decorators (retry, cache, tracing)
// so that capability discovery (EpisodeDetailProvider, TranslationProvider)
// can reach the underlying provider through layers of wrappers.
type ProviderUnwrapper interface {
	Unwrap() MetadataProvider
}

type MetadataOverrideRepository interface {
	Set(ctx context.Context, o *MetadataOverride) error
	Get(ctx context.Context, id string) (*MetadataOverride, error)
	GetByMediaItem(ctx context.Context, mediaItemID string) ([]*MetadataOverride, error)
	// GetByMediaItems returns the overrides for every given media item in one
	// query, grouped by media_item_id (batch form of GetByMediaItem).
	GetByMediaItems(ctx context.Context, mediaItemIDs []string) (map[string][]*MetadataOverride, error)
	ListOverriddenMediaItemIDs(ctx context.Context, mediaItemIDs []string) (map[string]bool, error)
	Delete(ctx context.Context, id string) error
	DeleteByMediaItemID(ctx context.Context, mediaItemID string) error
	// DeleteByMediaItemIDs is the batch form of DeleteByMediaItemID: one
	// statement for a whole level of items (e.g. all seasons of a series)
	// instead of one round-trip per item.
	DeleteByMediaItemIDs(ctx context.Context, mediaItemIDs []string) error
}

// ArtworkStats holds aggregate counts about local-path coverage.
type ArtworkStats struct {
	TotalRecords     int64
	WithLocalPath    int64
	MissingLocalPath int64
	TotalSizeBytes   int64
}

// ArtworkCommandRepo is the write half of the artwork repository (ADR-0009).
type ArtworkCommandRepo interface {
	Create(ctx context.Context, a *ArtworkRecord) error
	CreateBatch(ctx context.Context, records []*ArtworkRecord) error
	// UpdateLocalPath persists the local file path after a successful download.
	UpdateLocalPath(ctx context.Context, id string, localPath string) error
	UpdateWebPPath(ctx context.Context, id string, webpPath string) error
	// UpdateBlurhash persists the encoded blurhash placeholder after image
	// download. Best-effort: the caller logs and swallows errors so a hash
	// failure never blocks the artwork pipeline.
	UpdateBlurhash(ctx context.Context, id string, blurhash string) error
	Delete(ctx context.Context, id string) error
	DeleteByMediaItemID(ctx context.Context, mediaItemID string) error
	// DeleteByMediaItemIDs is the batch form of DeleteByMediaItemID: one
	// statement for a whole level of items (e.g. all seasons of a series)
	// instead of one round-trip per item.
	DeleteByMediaItemIDs(ctx context.Context, mediaItemIDs []string) error
}

// ArtworkQueryRepo is the read half of the artwork repository (ADR-0009).
type ArtworkQueryRepo interface {
	GetByMediaItem(ctx context.Context, mediaItemID string) ([]*ArtworkRecord, error)
	GetByMediaItems(ctx context.Context, mediaItemIDs []string) (map[string][]*ArtworkRecord, error)
	GetByPerson(ctx context.Context, personID string) ([]*ArtworkRecord, error)
	// ListMissingLocalPath returns records that have a SourceURL but no LocalPath,
	// ordered by id. Used by the bulk-download admin job.
	ListMissingLocalPath(ctx context.Context) ([]*ArtworkRecord, error)
	// ListMissingBlurhash returns records that have a LocalPath on disk but no
	// blurhash. Used by the backfill admin job to retro-fit hashes onto
	// artworks that pre-date the blurhash column (migration 000095).
	ListMissingBlurhash(ctx context.Context) ([]*ArtworkRecord, error)
	// LocalPathStats returns aggregate coverage statistics.
	LocalPathStats(ctx context.Context) (ArtworkStats, error)
}

// ArtworkRepository is the transitional union of ArtworkCommandRepo and
// ArtworkQueryRepo (ADR-0009). Consumers that only read or only write should
// depend on the narrower half directly; this stays for callers that
// genuinely need both (e.g. CommandService, Enricher) and for the postgres
// adapter's compile-time assertion. Prefer ArtworkCommandRepo for writes and
// ArtworkQueryRepo for reads over this union in new code.
type ArtworkRepository interface {
	ArtworkCommandRepo
	ArtworkQueryRepo
}

type ImageConverter interface {
	ConvertToWebP(ctx context.Context, srcPath, dstPath string, quality int) error
	// ResizeAndConvertToWebP scales src to fit width (height auto, ratio preserved)
	// and encodes WebP at quality. width=0 means no resize.
	ResizeAndConvertToWebP(ctx context.Context, srcPath, dstPath string, width, quality int) error
}

type ArtworkDownloader interface {
	Download(ctx context.Context, url, destPath string) error
}

type EpisodeDetailProvider interface {
	GetEpisodeDetails(ctx context.Context, seriesID string, seasonNum, episodeNum int) (*MetadataResult, error)
}

type SeasonDetailProvider interface {
	GetSeasonDetails(ctx context.Context, seriesID string, seasonNum int) (*MetadataResult, error)
}

type ExternalIDLookup interface {
	LookupByExternalID(ctx context.Context, providerName, externalID string) (*MetadataResult, error)
}

// TranslationProvider fetches metadata in multiple locales.
type TranslationProvider interface {
	FetchTranslations(ctx context.Context, providerID string, mediaType string, locales []string) (map[string]MetadataResult, error)
	FetchPersonTranslations(ctx context.Context, personName string, locales []string) (map[string]string, error)
}

// LanguageAware is implemented by providers whose results depend on a
// configured language (e.g. TMDB). Caches in front of such a provider must
// fold the language into their key, else a result fetched in one language is
// served under another.
type LanguageAware interface {
	Language() string
}

// AlternativeTitlesProvider fetches locale-variant release titles for a
// specific provider ID. Used on the ambiguous-match path to resolve
// filenames that differ from the localised search-language title
// (e.g. "Train to Busan" when TMDB returns "Dernier train pour Busan").
// Providers that have no equivalent endpoint return (nil, nil).
type AlternativeTitlesProvider interface {
	AlternativeTitles(ctx context.Context, providerID string, mediaType string) ([]string, error)
}
