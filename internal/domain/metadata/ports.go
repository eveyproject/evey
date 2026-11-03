package metadata

import (
	"context"
	"errors"
	"time"
)

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
	// Dead counts the records whose source is given up on: the provider said
	// gone, or the fetch failed ArtworkMaxFetchAttempts times.
	Dead int64
}

// How a missing artwork is retried. A fetch that fails is tried again after
// ArtworkFetchBackoff(attempts), until ArtworkMaxFetchAttempts; a source the
// provider says is gone (FetchStatusError 404 or 410) is not tried again at
// all. Before this every missing record was fetched every hour for good.
const (
	ArtworkMaxFetchAttempts = 8
	ArtworkFetchBackoffBase = time.Hour
	ArtworkFetchBackoffMax  = 7 * 24 * time.Hour
)

// ArtworkFetchBackoff is how long to wait after the given number of failed
// attempts: doubled each time from the base, capped.
func ArtworkFetchBackoff(attempts int) time.Duration {
	d := ArtworkFetchBackoffBase
	for i := 1; i < attempts && d < ArtworkFetchBackoffMax; i++ {
		d *= 2
	}
	return min(d, ArtworkFetchBackoffMax)
}

// FetchStatusError is an artwork fetch the server answered with a status
// other than OK, kept typed so a caller can tell a gone source from an outage.
type FetchStatusError struct {
	Code int
}

func (e *FetchStatusError) Error() string {
	return "download failed with status " + itoa(e.Code)
}

// IsPermanentFetchError says whether err is a source the provider will not
// serve again (404, 410): retrying it only burns the hourly pass.
func IsPermanentFetchError(err error) bool {
	var se *FetchStatusError
	if !errors.As(err, &se) {
		return false
	}
	return se.Code == 404 || se.Code == 410
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
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
	// RecordFetchFailure counts a failed fetch of the record's source and
	// schedules the next try (ArtworkFetchBackoff), or gives the source up
	// when permanent. ListMissingLocalPath leaves out what is not due.
	RecordFetchFailure(ctx context.Context, id string, permanent bool) error
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
	// ListMissingLocalPath returns records that have a SourceURL but no LocalPath
	// and are due for a fetch (not given up on, not waiting out a backoff),
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
