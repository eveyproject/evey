package metadata

import "context"

type MetadataProvider interface {
	Name() string
	Search(ctx context.Context, query string, mediaType string) ([]MetadataResult, error)
	GetByProviderID(ctx context.Context, providerID string, mediaType string) (*MetadataResult, error)
}

type MetadataOverrideRepository interface {
	Set(ctx context.Context, o *MetadataOverride) error
	Get(ctx context.Context, id string) (*MetadataOverride, error)
	GetByMediaItem(ctx context.Context, mediaItemID string) ([]*MetadataOverride, error)
	Delete(ctx context.Context, id string) error
	DeleteByMediaItemID(ctx context.Context, mediaItemID string) error
}

type ArtworkRepository interface {
	Create(ctx context.Context, a *ArtworkRecord) error
	GetByMediaItem(ctx context.Context, mediaItemID string) ([]*ArtworkRecord, error)
	GetByPerson(ctx context.Context, personID string) ([]*ArtworkRecord, error)
	Delete(ctx context.Context, id string) error
	DeleteByMediaItemID(ctx context.Context, mediaItemID string) error
}

type ArtworkDownloader interface {
	Download(ctx context.Context, url, destPath string) error
}

type EpisodeDetailProvider interface {
	GetEpisodeDetails(ctx context.Context, seriesID string, seasonNum, episodeNum int) (*MetadataResult, error)
}

type ExternalIDLookup interface {
	LookupByExternalID(ctx context.Context, providerName, externalID string) (*MetadataResult, error)
}
