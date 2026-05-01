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
	ListOverriddenMediaItemIDs(ctx context.Context, mediaItemIDs []string) (map[string]bool, error)
	Delete(ctx context.Context, id string) error
	DeleteByMediaItemID(ctx context.Context, mediaItemID string) error
}

// ArtworkStats holds aggregate counts about local-path coverage.
type ArtworkStats struct {
	TotalRecords      int64
	WithLocalPath     int64
	MissingLocalPath  int64
	TotalSizeBytes    int64
}

type ArtworkRepository interface {
	Create(ctx context.Context, a *ArtworkRecord) error
	CreateBatch(ctx context.Context, records []*ArtworkRecord) error
	GetByMediaItem(ctx context.Context, mediaItemID string) ([]*ArtworkRecord, error)
	GetByMediaItems(ctx context.Context, mediaItemIDs []string) (map[string][]*ArtworkRecord, error)
	GetByPerson(ctx context.Context, personID string) ([]*ArtworkRecord, error)
	// ListMissingLocalPath returns records that have a SourceURL but no LocalPath,
	// ordered by id. Used by the bulk-download admin job.
	ListMissingLocalPath(ctx context.Context) ([]*ArtworkRecord, error)
	// LocalPathStats returns aggregate coverage statistics.
	LocalPathStats(ctx context.Context) (ArtworkStats, error)
	// UpdateLocalPath persists the local file path after a successful download.
	UpdateLocalPath(ctx context.Context, id string, localPath string) error
	UpdateWebPPath(ctx context.Context, id string, webpPath string) error
	Delete(ctx context.Context, id string) error
	DeleteByMediaItemID(ctx context.Context, mediaItemID string) error
}

type ImageConverter interface {
	ConvertToWebP(ctx context.Context, srcPath, dstPath string, quality int) error
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
