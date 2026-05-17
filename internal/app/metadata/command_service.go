package metadata

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
)

// PeopleBioEnricher enriches bios for people missing biography data.
type PeopleBioEnricher interface {
	EnrichAllPeopleBios(ctx context.Context, log *slog.Logger) (int, error)
}

type CommandServiceOption func(*CommandService)

func WithEnricher(enricher ItemEnricher, mediaItems library.MediaItemRepository) CommandServiceOption {
	return func(s *CommandService) {
		s.enricher = enricher
		s.mediaItems = mediaItems
	}
}

func WithMediaPersons(repo library.MediaPersonRepository) CommandServiceOption {
	return func(s *CommandService) { s.mediaPersons = repo }
}

// DownloadJob tracks the progress of a bulk artwork download operation.
type DownloadJob struct {
	ID        string
	Total     int
	Processed atomic.Int64
	Errors    []string
	mu        sync.Mutex
	// Done is set once the bulk download goroutine has finished. Read by
	// the polling HTTP handler concurrently with the writer in
	// runBulkDownload — must be atomic to keep `go test -race` happy.
	Done atomic.Bool
}

func (j *DownloadJob) addError(msg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Errors = append(j.Errors, msg)
}

// ListErrors returns a copy of error messages accumulated during the job.
func (j *DownloadJob) ListErrors() []string {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]string, len(j.Errors))
	copy(out, j.Errors)
	return out
}

type CommandService struct {
	overrides    metadata.MetadataOverrideRepository
	artwork      metadata.ArtworkRepository
	downloader   metadata.ArtworkDownloader
	events       shared.EventBus
	providers    []metadata.MetadataProvider
	mediaItems   library.MediaItemRepository
	mediaPersons library.MediaPersonRepository
	enricher     ItemEnricher
	workerWg     sync.WaitGroup

	// auto-download settings
	autoDownloadEnabled bool
	dataDir             string

	// in-memory job registry for bulk downloads
	jobsMu sync.RWMutex
	jobs   map[string]*DownloadJob
}

func WithAutoDownload(dataDir string, enabled bool) CommandServiceOption {
	return func(s *CommandService) {
		s.dataDir = dataDir
		s.autoDownloadEnabled = enabled
	}
}

func NewCommandService(
	overrides metadata.MetadataOverrideRepository,
	artwork metadata.ArtworkRepository,
	downloader metadata.ArtworkDownloader,
	events shared.EventBus,
	providers []metadata.MetadataProvider,
	opts ...CommandServiceOption,
) *CommandService {
	s := &CommandService{
		overrides:  overrides,
		artwork:    artwork,
		downloader: downloader,
		events:     events,
		providers:  providers,
		jobs:       make(map[string]*DownloadJob),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SetAutoDownload updates the auto-download setting at runtime (e.g. after server-config change).
func (s *CommandService) SetAutoDownload(dataDir string, enabled bool) {
	s.dataDir = dataDir
	s.autoDownloadEnabled = enabled
}

// artworkDestPath derives the on-disk path for an artwork record.
// Pattern: <dataDir>/artworks/<mediaItemID>/<artworkID>.<ext>
func (s *CommandService) artworkDestPath(rec *metadata.ArtworkRecord) string {
	ext := ".jpg"
	if rec.SourceURL != nil {
		u := *rec.SourceURL
		if idx := strings.LastIndex(u, "."); idx >= 0 && idx > strings.LastIndex(u, "/") {
			candidate := u[idx:]
			// strip query string
			if q := strings.Index(candidate, "?"); q >= 0 {
				candidate = candidate[:q]
			}
			if len(candidate) <= 5 {
				ext = candidate
			}
		}
	}
	return filepath.Join(s.dataDir, "artworks", string(rec.MediaItemID), rec.ID+ext)
}

// ArtworkStats returns aggregate coverage statistics.
func (s *CommandService) ArtworkStats(ctx context.Context) (metadata.ArtworkStats, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.ArtworkStats")
	defer span.End()
	return s.artwork.LocalPathStats(ctx)
}

// StartBulkDownload launches a background goroutine to download all artwork records
// that have a SourceURL but no LocalPath. Returns a job ID immediately.
// Concurrent: at most concurrency downloads at once.
func (s *CommandService) StartBulkDownload(ctx context.Context, serverCtx context.Context) (string, int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.StartBulkDownload")
	defer span.End()
	if s.dataDir == "" {
		return "", 0, errors.New("dataDir not configured")
	}
	records, err := s.artwork.ListMissingLocalPath(ctx)
	if err != nil {
		return "", 0, fmt.Errorf("list missing artwork: %w", err)
	}

	jobID := fmt.Sprintf("artdl-%d", time.Now().UnixNano())
	job := &DownloadJob{
		ID:    jobID,
		Total: len(records),
	}
	s.jobsMu.Lock()
	s.jobs[jobID] = job
	s.jobsMu.Unlock()

	go s.runBulkDownload(serverCtx, job, records)
	return jobID, len(records), nil
}

// runBulkDownload executes the download job with capped concurrency (4 workers).
func (s *CommandService) runBulkDownload(ctx context.Context, job *DownloadJob, records []*metadata.ArtworkRecord) {
	const maxConcurrent = 4
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for _, rec := range records {
		rec := rec
		select {
		case <-ctx.Done():
			break
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			destPath := s.artworkDestPath(rec)
			// Skip if already downloaded (re-entrant)
			if _, err := os.Stat(destPath); err == nil {
				if err2 := s.artwork.UpdateLocalPath(ctx, rec.ID, destPath); err2 != nil {
					slog.Warn("artwork bulk: update already-present local path",
						slog.String("id", rec.ID), slog.Any("err", err2))
				}
				job.Processed.Add(1)
				return
			}
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				job.addError(fmt.Sprintf("%s: mkdir: %v", rec.ID, err))
				job.Processed.Add(1)
				return
			}
			if err := s.DownloadArtwork(ctx, rec, destPath); err != nil {
				job.addError(fmt.Sprintf("%s: %v", rec.ID, err))
				job.Processed.Add(1)
				return
			}
			if err := s.artwork.UpdateLocalPath(ctx, rec.ID, destPath); err != nil {
				slog.Warn("artwork bulk: save local path",
					slog.String("id", rec.ID), slog.Any("err", err))
			}
			job.Processed.Add(1)
		}()
	}
	wg.Wait()
	job.Done.Store(true)
}

// GetDownloadJob retrieves a job by ID; returns nil if not found.
func (s *CommandService) GetDownloadJob(jobID string) *DownloadJob {
	s.jobsMu.RLock()
	defer s.jobsMu.RUnlock()
	return s.jobs[jobID]
}

// RetryMissingArtworks downloads every artwork record that has a SourceURL
// but no LocalPath. Idempotent: items already on disk are reconciled with
// their DB row without re-downloading. Best-effort: per-record failures
// (transient HTTP 5xx, dead CDN, etc.) are logged and the loop continues
// so a single broken URL doesn't block the rest of the catalogue.
//
// Returns (succeeded, failed) for telemetry.
func (s *CommandService) RetryMissingArtworks(ctx context.Context) (succeeded, failed int) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.RetryMissingArtworks")
	defer span.End()
	if s.dataDir == "" {
		return 0, 0
	}
	records, err := s.artwork.ListMissingLocalPath(ctx)
	if err != nil {
		slog.Warn("retry missing artworks: list", slog.Any("err", err))
		return 0, 0
	}
	if len(records) == 0 {
		return 0, 0
	}
	const maxConcurrent = 4
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	var sucAtomic, failAtomic atomic.Int64
	for _, rec := range records {
		rec := rec
		select {
		case <-ctx.Done():
			wg.Wait()
			return int(sucAtomic.Load()), int(failAtomic.Load())
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			destPath := s.artworkDestPath(rec)
			// Already on disk (caller previously downloaded outside the
			// pipeline, server crashed after fs write before DB update, etc.).
			// Just reconcile the DB row and count as a success.
			if _, err := os.Stat(destPath); err == nil {
				if err2 := s.artwork.UpdateLocalPath(ctx, rec.ID, destPath); err2 != nil {
					slog.Warn("retry missing artworks: reconcile present file",
						slog.String("id", rec.ID), slog.Any("err", err2))
					failAtomic.Add(1)
					return
				}
				sucAtomic.Add(1)
				return
			}
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				slog.Warn("retry missing artworks: mkdir",
					slog.String("id", rec.ID), slog.Any("err", err))
				failAtomic.Add(1)
				return
			}
			if err := s.DownloadArtwork(ctx, rec, destPath); err != nil {
				slog.Debug("retry missing artworks: download still failing",
					slog.String("id", rec.ID), slog.Any("err", err))
				failAtomic.Add(1)
				return
			}
			if err := s.artwork.UpdateLocalPath(ctx, rec.ID, destPath); err != nil {
				slog.Warn("retry missing artworks: save local path",
					slog.String("id", rec.ID), slog.Any("err", err))
				failAtomic.Add(1)
				return
			}
			sucAtomic.Add(1)
		}()
	}
	wg.Wait()
	succeeded = int(sucAtomic.Load())
	failed = int(failAtomic.Load())
	if succeeded > 0 || failed > 0 {
		slog.Info("retry missing artworks: pass complete",
			slog.Int("succeeded", succeeded),
			slog.Int("failed", failed),
			slog.Int("total", len(records)))
	}
	return succeeded, failed
}

// RetryMissingArtworksLoop runs RetryMissingArtworks periodically until
// ctx is cancelled. Intended to be launched as a goroutine at startup.
// Wired so a transient CDN/network outage during the initial enrichment
// burst is self-healing within the next interval — admin doesn't need
// to remember to hit "Download missing" by hand.
func (s *CommandService) RetryMissingArtworksLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.RetryMissingArtworks(ctx)
		}
	}
}

// maybeAutoDownload triggers an automatic download when auto-download is enabled.
// It is best-effort: errors are only logged.
func (s *CommandService) maybeAutoDownload(ctx context.Context, rec *metadata.ArtworkRecord) {
	if !s.autoDownloadEnabled || s.dataDir == "" || rec.SourceURL == nil {
		return
	}
	destPath := s.artworkDestPath(rec)
	if _, err := os.Stat(destPath); err == nil {
		// already downloaded
		return
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		slog.Warn("auto-download artwork: mkdir", slog.String("id", rec.ID), slog.Any("err", err))
		return
	}
	if err := s.DownloadArtwork(ctx, rec, destPath); err != nil {
		slog.Warn("auto-download artwork", slog.String("id", rec.ID), slog.Any("err", err))
		return
	}
	if err := s.artwork.UpdateLocalPath(ctx, rec.ID, destPath); err != nil {
		slog.Warn("auto-download artwork: save local path", slog.String("id", rec.ID), slog.Any("err", err))
	}
}

func (s *CommandService) RefreshMetadata(ctx context.Context, mediaItemID, providerID, providerName, mediaType string) (*metadata.MetadataResult, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.RefreshMetadata")
	defer span.End()
	span.SetAttributes(
		attribute.String("media_item.id", mediaItemID),
		attribute.String("provider.id", providerID),
		attribute.String("provider.name", providerName),
		attribute.String("media.type", mediaType),
	)
	for _, p := range s.providers {
		if p.Name() != providerName {
			continue
		}
		result, err := p.GetByProviderID(ctx, providerID, mediaType)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, fmt.Errorf("get metadata from %s: %w", providerName, err)
		}
		// Log-only: notification event — actual metadata fetch already succeeded above
		evt := metadata.MetadataRefreshed{MediaItemID: shared.MediaItemID(mediaItemID), Provider: providerName, OccurredAt: time.Now()}
		if err := s.events.Publish(ctx, evt); err != nil {
			slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
		}
		return result, nil
	}
	err := fmt.Errorf("provider %s not found", providerName)
	span.RecordError(err)
	span.SetStatus(otelcodes.Error, err.Error())
	return nil, err
}

func (s *CommandService) RefreshAllTranslations(ctx context.Context, onProgress func(done, total int)) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.RefreshAllTranslations")
	defer span.End()
	if s.enricher == nil || s.mediaItems == nil {
		err := errors.New("enricher not configured")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}
	items, err := s.mediaItems.List(ctx, library.ListMediaItemsFilter{})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("list all items: %w", err)
	}
	total := len(items)
	span.SetAttributes(attribute.Int("items.count", total))
	for i, itemPtr := range items {
		item := itemPtr
		if item.Catalog == nil {
			continue
		}
		tmdbID, ok := item.Catalog.ProviderIDs["tmdb"]
		if !ok || tmdbID == "" {
			continue
		}
		mediaType := "movie"
		if item.Type == "series" || item.Type == "season" || item.Type == "episode" {
			mediaType = "tv"
		}
		s.enricher.EnrichTranslationsForItem(ctx, item, tmdbID, mediaType)
		if onProgress != nil {
			onProgress(i+1, total)
		}
	}
	return nil
}

func (s *CommandService) GetMediaItem(ctx context.Context, id string) (*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.GetMediaItem")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", id))
	if s.mediaItems == nil {
		err := errors.New("media items repo not configured")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(id))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
	}
	return item, err
}

func (s *CommandService) RefreshItemMetadata(ctx context.Context, mediaItemID, tmdbID, mediaType string) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.RefreshItemMetadata")
	defer span.End()
	span.SetAttributes(
		attribute.String("media_item.id", mediaItemID),
		attribute.String("tmdb.id", tmdbID),
		attribute.String("media.type", mediaType),
	)
	if s.enricher == nil || s.mediaItems == nil {
		err := errors.New("enricher not configured")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}
	item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("get media item: %w", err)
	}
	result, _ := s.fetchFromProviders(ctx, tmdbID, mediaType, "")
	if result == nil {
		err := fmt.Errorf("no provider returned metadata for tmdb id %s", tmdbID)
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}
	if err := s.enricher.EnrichItem(ctx, item, result); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("enrich item: %w", err)
	}
	evtTmdb := metadata.MetadataRefreshed{MediaItemID: shared.MediaItemID(mediaItemID), Provider: "tmdb", OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtTmdb); err != nil {
		slog.Warn("publish event", slog.String("event", evtTmdb.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) RefreshItemMetadataByIMDB(ctx context.Context, mediaItemID, imdbID, mediaType string) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.RefreshItemMetadataByIMDB")
	defer span.End()
	span.SetAttributes(
		attribute.String("media_item.id", mediaItemID),
		attribute.String("imdb.id", imdbID),
		attribute.String("media.type", mediaType),
	)
	if s.enricher == nil || s.mediaItems == nil {
		err := errors.New("enricher not configured")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}
	item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("get media item: %w", err)
	}
	result, usedProvider := s.fetchFromProviders(ctx, imdbID, mediaType, "imdbapi")
	if result == nil {
		err := fmt.Errorf("no provider returned metadata for imdb id %s", imdbID)
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}

	if result.ProviderIDs == nil {
		result.ProviderIDs = make(map[string]string)
	}
	result.ProviderIDs["imdb"] = imdbID

	// Cross-ref IMDB → TMDB so locale translations hit the right record.
	// Without this, the old item.Catalog.ProviderIDs["tmdb"] sticks and
	// FR/ES/… get translated titles+overviews from the PREVIOUS movie.
	// Drop the stale TMDB ID if no mapping exists so translations skip
	// rather than poisoning the item.
	if _, hasTmdb := result.ProviderIDs["tmdb"]; !hasTmdb {
		if newTmdbID, ok := s.lookupTMDBByIMDB(ctx, imdbID); ok {
			result.ProviderIDs["tmdb"] = newTmdbID
		} else if item.Catalog != nil {
			delete(item.Catalog.ProviderIDs, "tmdb")
		}
	}

	if err := s.enricher.EnrichItem(ctx, item, result); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("enrich item: %w", err)
	}
	evtImdb := metadata.MetadataRefreshed{MediaItemID: shared.MediaItemID(mediaItemID), Provider: usedProvider, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtImdb); err != nil {
		slog.Warn("publish event", slog.String("event", evtImdb.EventName()), slog.Any("err", err))
	}
	return nil
}

// lookupTMDBByIMDB asks any provider that implements ExternalIDLookup to
// map an IMDB ID to its TMDB counterpart. Only TMDB reliably answers this
// today; others return no-op. Used on IMDB-based refresh so translations
// follow the new item rather than the stale TMDB ID.
func (s *CommandService) lookupTMDBByIMDB(ctx context.Context, imdbID string) (string, bool) {
	for _, p := range s.providers {
		lookup, ok := p.(metadata.ExternalIDLookup)
		if !ok {
			continue
		}
		res, err := lookup.LookupByExternalID(ctx, "imdb", imdbID)
		if err != nil || res == nil {
			continue
		}
		if id := res.ProviderIDs["tmdb"]; id != "" {
			return id, true
		}
	}
	return "", false
}

// fetchFromProviders tries providers in order. If preferProvider is set,
// that provider is tried first, then the rest as fallback.
func (s *CommandService) fetchFromProviders(ctx context.Context, providerID, mediaType, preferProvider string) (*metadata.MetadataResult, string) {
	if preferProvider != "" {
		for _, p := range s.providers {
			if p.Name() == preferProvider {
				r, err := p.GetByProviderID(ctx, providerID, mediaType)
				if err == nil {
					return r, p.Name()
				}
				break
			}
		}
	}
	for _, p := range s.providers {
		if p.Name() == preferProvider {
			continue
		}
		r, err := p.GetByProviderID(ctx, providerID, mediaType)
		if err == nil {
			return r, p.Name()
		}
	}
	return nil, ""
}

func (s *CommandService) SetOverride(ctx context.Context, override *metadata.MetadataOverride) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.SetOverride")
	defer span.End()
	span.SetAttributes(
		attribute.String("media_item.id", string(override.MediaItemID)),
		attribute.String("override.field", override.FieldName),
	)
	override.CreatedAt = time.Now()
	if err := s.overrides.Set(ctx, override); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("CommandService.SetOverride: %w", err)
	}
	// Log-only: audit event — override already persisted above
	evtOverride := metadata.MetadataOverrideSet{MediaItemID: override.MediaItemID, FieldName: override.FieldName, EditedBy: override.EditedBy, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtOverride); err != nil {
		slog.Warn("publish event", slog.String("event", evtOverride.EventName()), slog.Any("err", err))
	}
	// When an admin edits the title on an orphan item (no TMDB/IMDB IDs yet),
	// the intent is "this is the correct title — please match it now". The
	// enricher reads media_items.title for its search query, so propagate
	// the override into the raw title and kick a background auto-enrich.
	// Without this hook, fixing a parser-polluted title (e.g. stripping
	// "720" or "arte") via the UI only updated the display layer; match-
	// orphans kept failing because the search used the still-dirty title.
	s.propagateTitleOverrideForOrphan(ctx, override)
	return nil
}

// propagateTitleOverrideForOrphan is the orphan-recovery path for title
// edits. Best-effort: every step that fails just logs and bails — the
// override write itself succeeded above, so we never want to return an
// error from this side-effect.
func (s *CommandService) propagateTitleOverrideForOrphan(ctx context.Context, override *metadata.MetadataOverride) {
	if s.mediaItems == nil || s.enricher == nil {
		return
	}
	// Only the title (or originalTitle as a fallback) is search-relevant.
	// Other fields (overview, tagline, year) don't change TMDB match outcomes.
	if override.FieldName != "title" && override.FieldName != "originalTitle" {
		return
	}
	// CustomValue is any so the same override row can carry numerics,
	// dates, etc. for other fields — but title/originalTitle must be a
	// non-empty string for the propagation path to make sense.
	val, ok := override.CustomValue.(string)
	if !ok || val == "" {
		return
	}
	item, err := s.mediaItems.GetByID(ctx, override.MediaItemID)
	if err != nil || item == nil {
		return
	}
	// Only propagate for orphans — a matched item's title edit is
	// presentation-only and must not retrigger TMDB lookups.
	if item.Catalog != nil && len(item.Catalog.ProviderIDs) > 0 {
		return
	}
	if override.FieldName == "title" {
		if item.Title == val {
			return // already aligned, nothing to do
		}
		item.Title = val
	} else {
		if item.OriginalTitle == val {
			return
		}
		item.OriginalTitle = val
	}
	item.UpdatedAt = time.Now()
	if err := s.mediaItems.Update(ctx, item); err != nil {
		slog.Warn("propagate title override to media_items failed",
			slog.String("media_item.id", string(item.ID)),
			slog.Any("err", err))
		return
	}
	se, ok := s.enricher.(SingleItemEnricher)
	if !ok {
		return
	}
	// Detached context with a generous timeout so a TMDB rate-limit retry
	// chain doesn't burn the goroutine forever.
	go func(it *library.MediaItem) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in title-override auto-enrich",
					slog.Any("recover", r),
					slog.String("media_item.id", string(it.ID)))
			}
		}()
		enrichCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		se.AutoEnrichItem(enrichCtx, it, slog.Default())
	}(item)
}

func (s *CommandService) RemoveOverride(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.RemoveOverride")
	defer span.End()
	span.SetAttributes(attribute.String("override.id", id))
	if err := s.overrides.Delete(ctx, id); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("CommandService.RemoveOverride: %w", err)
	}
	return nil
}

// GetOverride exposes a single override lookup so adapters can resolve
// the media item it targets (used by the gRPC handler to gate Remove
// calls behind a per-library access check without re-implementing the
// repository hop).
func (s *CommandService) GetOverride(ctx context.Context, id string) (*metadata.MetadataOverride, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.GetOverride")
	defer span.End()
	return s.overrides.Get(ctx, id)
}

func (s *CommandService) ResetOverrides(ctx context.Context, mediaItemID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.ResetOverrides")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	if err := s.overrides.DeleteByMediaItemID(ctx, mediaItemID); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("CommandService.ResetOverrides: %w", err)
	}
	// Log-only: audit event — overrides already deleted above
	evtRemoved := metadata.MetadataOverrideRemoved{MediaItemID: shared.MediaItemID(mediaItemID), FieldName: "*", OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtRemoved); err != nil {
		slog.Warn("publish event", slog.String("event", evtRemoved.EventName()), slog.Any("err", err))
	}
	return nil
}

// deriveTitle extracts a human-readable title from the file path.
// For series: uses the top-level directory name.
// For movies/episodes: uses the filename without extension.
func deriveTitle(filePath, itemType string) string {
	if filePath == "" {
		return ""
	}
	if itemType == "series" || itemType == "season" {
		// Use the directory name
		return filepath.Base(filePath)
	}
	// Use filename without extension
	base := filepath.Base(filePath)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// deriveTitleFromChildren gets the series title from the top-level media directory
// by looking at an episode's file path (e.g. /media2/It welcome to derry/S01/... → "It welcome to derry").
func (s *CommandService) deriveTitleFromChildren(ctx context.Context, seriesID string) string {
	seasonType := "season"
	seasons, err := s.mediaItems.List(ctx, library.ListMediaItemsFilter{ParentID: &seriesID, Type: &seasonType, Limit: 1})
	if err != nil || len(seasons) == 0 {
		return ""
	}
	episodeType := "episode"
	seasonID := string(seasons[0].ID)
	episodes, err := s.mediaItems.List(ctx, library.ListMediaItemsFilter{ParentID: &seasonID, Type: &episodeType, Limit: 1})
	if err != nil || len(episodes) == 0 {
		return ""
	}
	// Extract the series directory name: /media2/It welcome to derry/S01/file.mkv → "It welcome to derry"
	fp := ""
	if episodes[0].File != nil {
		fp = episodes[0].File.Path
	}
	if fp == "" {
		return ""
	}
	dir := filepath.Dir(fp)     // /media2/It welcome to derry/S01
	dir = filepath.Dir(dir)     // /media2/It welcome to derry
	return filepath.Base(dir)   // It welcome to derry
}

// ResetItemMetadata clears enriched metadata on an item and all its children
// (seasons + episodes for series, episodes for seasons) then triggers re-enrichment.
func (s *CommandService) ResetItemMetadata(ctx context.Context, mediaItemID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.ResetItemMetadata")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	if s.mediaItems == nil {
		err := errors.New("media item repository not configured")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}
	item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("get media item: %w", err)
	}

	// Reset the item itself
	s.resetSingleItem(ctx, item)

	// Cascade to children (series → seasons + episodes, season → episodes)
	if item.Type == library.Series || item.Type == library.Season {
		s.resetChildren(ctx, string(item.ID))
	}

	// Trigger re-enrichment with background context (request ctx may be done)
	if se, ok := s.enricher.(SingleItemEnricher); ok {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic in metadata re-enrichment", slog.Any("recover", r))
				}
			}()
			enrichCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			se.AutoEnrichItem(enrichCtx, item, slog.Default())
		}()
	}

	return nil
}

// resetSingleItem clears enriched metadata fields, artwork, overrides, and cast for one item.
func (s *CommandService) resetSingleItem(ctx context.Context, item *library.MediaItem) {
	// Derive title from file path or directory name
	filePath := ""
	if item.File != nil {
		filePath = item.File.Path
	}
	if filePath != "" {
		item.Title = deriveTitle(filePath, string(item.Type))
	} else if item.Type == library.Series {
		if dirName := s.deriveTitleFromChildren(ctx, string(item.ID)); dirName != "" {
			item.Title = dirName
		}
	}

	item.Overview = ""
	item.Tagline = ""
	item.OriginalTitle = ""
	item.Catalog = nil
	item.Classification = nil
	item.Trailers = nil
	item.UpdatedAt = time.Now()

	if err := s.mediaItems.Update(ctx, item); err != nil {
		slog.Warn("reset: update item", slog.String("id", string(item.ID)), slog.Any("err", err))
	}
	if err := s.artwork.DeleteByMediaItemID(ctx, string(item.ID)); err != nil {
		slog.Warn("reset: delete artwork", slog.String("id", string(item.ID)), slog.Any("err", err))
	}
	if err := s.overrides.DeleteByMediaItemID(ctx, string(item.ID)); err != nil {
		slog.Warn("reset: delete overrides", slog.String("id", string(item.ID)), slog.Any("err", err))
	}
	if s.mediaPersons != nil {
		if err := s.mediaPersons.DeleteByMediaItemID(ctx, shared.MediaItemID(item.ID)); err != nil {
			slog.Warn("reset: delete media persons", slog.String("id", string(item.ID)), slog.Any("err", err))
		}
	}
}

// resetChildren resets all direct children and their children recursively.
func (s *CommandService) resetChildren(ctx context.Context, parentID string) {
	children, err := s.mediaItems.List(ctx, library.ListMediaItemsFilter{ParentID: &parentID})
	if err != nil {
		slog.Warn("reset: list children", slog.String("parentID", parentID), slog.Any("err", err))
		return
	}
	for _, child := range children {
		s.resetSingleItem(ctx, child)
		// Recurse (series → seasons → episodes)
		if child.Type == library.Season {
			s.resetChildren(ctx, string(child.ID))
		}
	}
}

func (s *CommandService) DownloadArtwork(ctx context.Context, record *metadata.ArtworkRecord, destPath string) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.DownloadArtwork")
	defer span.End()
	span.SetAttributes(
		attribute.String("media_item.id", string(record.MediaItemID)),
		attribute.String("artwork.type", record.Type),
	)
	if record.SourceURL == nil {
		err := errors.New("no source URL")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}
	if err := s.downloader.Download(ctx, *record.SourceURL, destPath); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("download artwork: %w", err)
	}
	record.LocalPath = &destPath
	// Log-only: notification event — file already downloaded to destPath above
	evtArtwork := metadata.ArtworkDownloaded{MediaItemID: record.MediaItemID, Type: record.Type, LocalPath: destPath, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtArtwork); err != nil {
		slog.Warn("publish event", slog.String("event", evtArtwork.EventName()), slog.Any("err", err))
	}
	return nil
}

// BulkItemEnricher is an optional extension of ItemEnricher for batch auto-enrichment.
type BulkItemEnricher interface {
	AutoEnrichAll(ctx context.Context, items []*library.MediaItem, log *slog.Logger)
}

// ForceBulkItemEnricher is the admin-triggered refresh variant — it re-runs
// the TMDB search even on items that already have an overview. Admin field
// overrides are still respected. Wired by the RefreshAllMetadata RPC.
type ForceBulkItemEnricher interface {
	ForceAutoEnrichAll(ctx context.Context, items []*library.MediaItem, log *slog.Logger)
}

// SingleItemEnricher is an optional extension of ItemEnricher for per-item auto-enrichment.
type SingleItemEnricher interface {
	AutoEnrichItem(ctx context.Context, item *library.MediaItem, log *slog.Logger)
}

// ForceSingleItemEnricher bypasses the "already enriched" short-circuit for a
// single item — used by ResearchMetadata to correct a wrong TMDB match in
// place. Admin field overrides are still respected by the underlying enricher.
type ForceSingleItemEnricher interface {
	ForceAutoEnrichItem(ctx context.Context, item *library.MediaItem, log *slog.Logger)
}

// ResearchMetadataResult is what the gRPC handler reports back: did we land on
// a different title (i.e. the re-search produced a new match) and what is it.
type ResearchMetadataResult struct {
	Item     *library.MediaItem
	Matched  bool
	NewTitle string
}

// ResearchMetadata clears the wrong-match traces from an item (overrides,
// provider IDs, year, enriched title/overview/tagline/originalTitle) and
// re-runs auto-enrichment from the stable ScanTitle. Used by the admin
// "Re-match" button to fix a bad TMDB match in one click without forcing
// the user to manually walk through the override repo.
//
// Steps:
//  1. Load item.
//  2. Delete every override targeting the item.
//  3. Strip ProviderIDs (tmdb/imdb/thetvdb) and Year from the catalog.
//  4. Reset Title to ScanTitle (when set), clear OriginalTitle/Overview/Tagline.
//  5. Persist.
//  6. Force-enrich (bypasses overview short-circuit so the search runs even
//     though the item has been touched many times before).
//  7. Re-load and report whether we ended up with a different title.
func (s *CommandService) ResearchMetadata(ctx context.Context, mediaItemID string) (*ResearchMetadataResult, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.ResearchMetadata")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	if s.mediaItems == nil {
		err := errors.New("media item repository not configured")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("get media item: %w", err)
	}

	previousTitle := item.Title

	// 1. Drop every override on the item — they were tied to the bad match.
	if err := s.overrides.DeleteByMediaItemID(ctx, mediaItemID); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("delete overrides: %w", err)
	}

	// 2. Strip wrong provider IDs + year so the next enrichment runs a fresh
	// search instead of rehydrating from the stale TMDB record.
	if item.Catalog != nil {
		if item.Catalog.ProviderIDs != nil {
			delete(item.Catalog.ProviderIDs, "tmdb")
			delete(item.Catalog.ProviderIDs, "imdb")
			delete(item.Catalog.ProviderIDs, "thetvdb")
		}
		item.Catalog.Year = nil
	}

	// 3. Reset enriched fields so re-enrichment writes a clean record.
	// ScanTitle is the filesystem-derived title that survived prior matches —
	// it's the source of truth for the search query.
	if item.ScanTitle != "" {
		item.Title = item.ScanTitle
	}
	item.OriginalTitle = ""
	item.Overview = ""
	item.Tagline = ""
	item.UpdatedAt = time.Now()

	if err := s.mediaItems.Update(ctx, item); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("update media item: %w", err)
	}

	// Audit-style event so admin tooling can surface the action.
	evtRemoved := metadata.MetadataOverrideRemoved{MediaItemID: shared.MediaItemID(mediaItemID), FieldName: "*", OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtRemoved); err != nil {
		slog.Warn("publish event", slog.String("event", evtRemoved.EventName()), slog.Any("err", err))
	}

	// 4. Re-fetch (state pre-enrich) so we hand the enricher the persisted form.
	item, err = s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("reload media item: %w", err)
	}

	// 5. Force-enrich. ForceAutoEnrichItem bypasses the "already has overview"
	// short-circuit; AutoEnrichItem would refuse to act because we just left
	// an overview-less item but its enricher chain still walks the same path
	// — keep the explicit force variant for clarity.
	if fse, ok := s.enricher.(ForceSingleItemEnricher); ok {
		fse.ForceAutoEnrichItem(ctx, item, slog.Default())
	} else if se, ok := s.enricher.(SingleItemEnricher); ok {
		se.AutoEnrichItem(ctx, item, slog.Default())
	}

	// 6. Reload to pick up enrichment writes, decide whether the title moved.
	final, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("reload after enrich: %w", err)
	}
	matched := final.Title != "" && final.Title != previousTitle && final.Overview != ""
	return &ResearchMetadataResult{Item: final, Matched: matched, NewTitle: final.Title}, nil
}

// MatchOrphansInLibrary auto-searches and enriches *only* the items in the
// library whose Catalog.ProviderIDs is effectively empty — i.e. items the
// initial scan failed to match against any provider (typically TMDB rate
// limit or network blip during bulk import). Idempotent: items that get
// matched leave the orphan set, so re-running the action just picks up
// what remains.
//
// Unlike AutoRefreshAllItems, this one returns immediately with the total
// count of orphans detected; the actual enrichment happens in a detached
// goroutine that paginates through the orphan set (the postgres adapter
// caps each List at DefaultListLimit=500). The underlying enricher pool
// (max 3 concurrent + provider-side rate limit) drains each batch before
// the next page is fetched. Two safety stops: zero orphans remaining, or
// no progress between iterations (e.g. provider unreachable).
func (s *CommandService) MatchOrphansInLibrary(ctx context.Context, libraryID string) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.MatchOrphansInLibrary")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", libraryID))
	be, ok := s.enricher.(BulkItemEnricher)
	if !ok {
		err := errors.New("enricher does not support bulk refresh")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return 0, err
	}
	missing := true
	filter := library.ListMediaItemsFilter{MissingProviderIDs: &missing}
	if libraryID != "" {
		libID := shared.LibraryID(libraryID)
		filter.LibraryID = &libID
	}
	total, err := s.mediaItems.Count(ctx, filter)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return 0, fmt.Errorf("count orphans: %w", err)
	}
	span.SetAttributes(attribute.Int("orphans.total", total))
	if total == 0 {
		return 0, nil
	}
	// Drain in background — paginated, with stall detection so a downstream
	// provider outage doesn't burn CPU on the same 500 items forever.
	go s.drainOrphans(libraryID, filter, total, be)
	return total, nil
}

// drainOrphans is the background enrichment loop for MatchOrphansInLibrary.
// Detached from any request context (HTTP/gRPC) so caller disconnect or
// the openresty 90s gateway timeout do not cancel a minutes-to-hours job.
// Stops on empty remainder (drain complete), Count stall (no progress),
// List failure, or the safety iteration cap.
func (s *CommandService) drainOrphans(libraryID string, filter library.ListMediaItemsFilter, total int, be BulkItemEnricher) {
	const (
		pageSize      = 500 // matches postgres DefaultListLimit
		maxIterations = 500 // safety cap: ≥ 250k items per invocation
	)
	log := slog.Default()
	lastCount := total
	for i := 0; i < maxIterations; i++ {
		f := filter
		f.Limit = pageSize
		items, err := s.mediaItems.List(context.Background(), f)
		if err != nil {
			log.Warn("match orphans: list page failed",
				slog.String("library_id", libraryID),
				slog.Any("err", err))
			return
		}
		if len(items) == 0 {
			log.Info("match orphans: drain complete",
				slog.String("library_id", libraryID),
				slog.Int("total_processed", total),
				slog.Int("iterations", i))
			return
		}
		be.AutoEnrichAll(context.Background(), items, log)
		current, err := s.mediaItems.Count(context.Background(), f)
		if err != nil {
			log.Warn("match orphans: recount failed",
				slog.String("library_id", libraryID),
				slog.Any("err", err))
			return
		}
		if current >= lastCount {
			log.Warn("match orphans: no progress between iterations, stopping",
				slog.String("library_id", libraryID),
				slog.Int("remaining", current),
				slog.Int("iterations", i+1))
			return
		}
		lastCount = current
	}
	log.Warn("match orphans: hit safety iteration cap",
		slog.String("library_id", libraryID),
		slog.Int("remaining", lastCount))
}

// AutoRefreshAllItems auto-searches and enriches all media items using the
// enricher. This is the admin-triggered "Refresh all metadata" entry point —
// it bypasses the "already has overview" short-circuit so past bad TMDB
// matches get a chance to be corrected. Items with admin field overrides
// are still skipped (manual intent wins).
// Falls back to non-force bulk if the enricher only implements the basic
// BulkItemEnricher interface.
func (s *CommandService) AutoRefreshAllItems(ctx context.Context, libraryID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.AutoRefreshAllItems")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", libraryID))
	fbe, forceOK := s.enricher.(ForceBulkItemEnricher)
	be, bulkOK := s.enricher.(BulkItemEnricher)
	if !forceOK && !bulkOK {
		err := errors.New("enricher does not support bulk refresh")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}
	const pageSize = 500
	for offset := 0; ; offset += pageSize {
		filter := library.ListMediaItemsFilter{Limit: pageSize, Offset: offset}
		if libraryID != "" {
			libID := shared.LibraryID(libraryID)
			filter.LibraryID = &libID
		}
		items, err := s.mediaItems.List(ctx, filter)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return fmt.Errorf("list items: %w", err)
		}
		if len(items) == 0 {
			break
		}
		if forceOK {
			fbe.ForceAutoEnrichAll(ctx, items, slog.Default())
		} else {
			be.AutoEnrichAll(ctx, items, slog.Default())
		}
		if len(items) < pageSize {
			break
		}
	}
	return nil
}

func (s *CommandService) StartEnrichmentWorker(ctx context.Context) {
	be, bulkOK := s.enricher.(BulkItemEnricher)
	se, singleOK := s.enricher.(SingleItemEnricher)
	if !bulkOK && !singleOK {
		return
	}
	if bulkOK && s.mediaItems != nil {
		s.workerWg.Go(func() {
			const pageSize = 500
			for offset := 0; ; offset += pageSize {
				items, err := s.mediaItems.List(ctx, library.ListMediaItemsFilter{Limit: pageSize, Offset: offset})
				if err != nil || len(items) == 0 {
					break
				}
				be.AutoEnrichAll(ctx, items, slog.Default())
				if len(items) < pageSize {
					break
				}
			}
		})
	}
	if bulkOK && s.mediaItems != nil {
		s.workerWg.Go(func() {
			s.dailyReEnrich(ctx, be)
		})
	}
	if singleOK && s.mediaItems != nil {
		queue := make(chan string, 100)
		for range 4 {
			s.workerWg.Go(func() {
				for {
					select {
					case <-ctx.Done():
						return
					case mediaID, ok := <-queue:
						if !ok {
							return
						}
						item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaID))
						if err == nil {
							se.AutoEnrichItem(ctx, item, slog.Default())
						}
					}
				}
			})
		}
		s.events.Subscribe("media.item.added", func(_ context.Context, ev shared.Event) {
			if e, ok := ev.(library.MediaItemAdded); ok {
				select {
				case queue <- string(e.MediaItemID):
				default:
					slog.Warn("enrich queue full, dropping", slog.String("media_id", string(e.MediaItemID)))
				}
			}
		})
	}
}

// WaitWorkers blocks until all enrichment goroutines finish (after ctx cancel).
func (s *CommandService) WaitWorkers() {
	s.workerWg.Wait()
}

func (s *CommandService) dailyReEnrich(ctx context.Context, be BulkItemEnricher) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.ReEnrichWithoutOverrides(ctx, be)
		}
	}
}

// ReEnrichWithoutOverrides re-enriches all items that have no user overrides.
func (s *CommandService) ReEnrichWithoutOverrides(ctx context.Context, be BulkItemEnricher) (refreshed, skipped int) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.ReEnrichWithoutOverrides")
	defer span.End()
	defer func() {
		span.SetAttributes(
			attribute.Int("items.refreshed", refreshed),
			attribute.Int("items.skipped", skipped),
		)
	}()
	const pageSize = 500
	for offset := 0; ; offset += pageSize {
		items, err := s.mediaItems.List(ctx, library.ListMediaItemsFilter{Limit: pageSize, Offset: offset})
		if err != nil {
			slog.Warn("re-enrich: list items", slog.Any("err", err))
			return refreshed, skipped
		}
		if len(items) == 0 {
			break
		}

		ids := make([]string, len(items))
		for i, item := range items {
			ids[i] = string(item.ID)
		}
		overridden, oErr := s.overrides.ListOverriddenMediaItemIDs(ctx, ids)
		if oErr != nil {
			slog.Warn("re-enrich: batch check overrides", slog.Any("err", oErr))
			return refreshed, skipped
		}

		var toEnrich []*library.MediaItem
		for _, item := range items {
			if overridden[string(item.ID)] {
				skipped++
				continue
			}
			toEnrich = append(toEnrich, item)
		}
		if len(toEnrich) > 0 {
			slog.Info("re-enrich: page",
				slog.Int("items", len(toEnrich)),
				slog.Int("skipped_overrides", skipped),
			)
			be.AutoEnrichAll(ctx, toEnrich, slog.Default())
			refreshed += len(toEnrich)
		}

		if len(items) < pageSize {
			break
		}
	}
	slog.Info("re-enrich: done", slog.Int("refreshed", refreshed), slog.Int("skipped", skipped))
	return refreshed, skipped
}
