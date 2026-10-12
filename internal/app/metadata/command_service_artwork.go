package metadata

// Background artwork jobs: bulk download, retry-missing, blurhash backfill,
// and the auto-download hook fired on new artwork records.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
)

// DownloadJob tracks the progress of a bulk artwork download operation.
type DownloadJob struct {
	ID        string
	Total     int
	Processed atomic.Int64
	Errors    []string
	mu        sync.Mutex
	// Done is set once the bulk download goroutine has finished. Read by
	// the polling HTTP handler concurrently with the writer in
	// runBulkDownload - must be atomic to keep `go test -race` happy.
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

// downloadJobRegistry is the in-memory registry of background artwork jobs
// (bulk download, retry-missing, blurhash backfill), polled by the admin UI via
// GetDownloadJob. Extracted from CommandService so the bookkeeping is one
// concern behind one field instead of a raw mutex+map inlined in the service.
type downloadJobRegistry struct {
	mu   sync.RWMutex
	jobs map[string]*DownloadJob
}

func newDownloadJobRegistry() *downloadJobRegistry {
	return &downloadJobRegistry{jobs: make(map[string]*DownloadJob)}
}

func (r *downloadJobRegistry) register(job *DownloadJob) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs[job.ID] = job
}

func (r *downloadJobRegistry) get(jobID string) *DownloadJob {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.jobs[jobID]
}

// artworkDestPath derives the on-disk path for an artwork record.
// Pattern: <dataDir>/artworks/<mediaItemID>/<artworkID>.<ext>
func (s *CommandService) artworkDestPath(rec *metadata.ArtworkRecord) string {
	ext := ".jpg"
	// Content-address by source URL, NOT by the record's UUID. Re-enrichment
	// mints a fresh ArtworkRecord (new UUID) every pass; keying the filename on
	// the UUID meant the SAME image was re-downloaded to a NEW path each time
	// and the old file leaked (DeleteByMediaItemID drops DB rows, not files) -
	// a 250 GB pile of 70+ identical copies per item. Hashing the source URL
	// maps one image to one stable path: re-download overwrites in place (or is
	// skipped), and a deleted record's file is re-referenced by the next record
	// for the same URL instead of orphaned.
	name := rec.ID
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
		sum := sha256.Sum256([]byte(u))
		name = hex.EncodeToString(sum[:16])
	}
	return filepath.Join(s.getDataDir(), "artworks", string(rec.MediaItemID), name+ext)
}

// ArtworkStats returns aggregate coverage statistics.
func (s *CommandService) ArtworkStats(ctx context.Context) (metadata.ArtworkStats, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.ArtworkStats")
	defer span.End()
	return s.artworkQuery.LocalPathStats(ctx)
}

// PurgeCorruptVariants deletes resized artwork variants (<base>_w<width>.webp)
// that are 0 bytes on disk - leftovers from a resize that exited non-zero or
// timed out. The serving path already regenerates these on demand, so this is a
// convenience for operators who'd rather clear them in bulk (e.g. after
// deploying the 0-byte fix) than shell into the data volume. Only zero-length
// _w<digits>.webp files are touched; full-resolution artwork is never removed.
// Returns the number of files deleted.
func (s *CommandService) PurgeCorruptVariants(ctx context.Context) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.PurgeCorruptVariants")
	defer span.End()
	if s.getDataDir() == "" {
		return 0, errors.New("dataDir not configured")
	}
	root := filepath.Join(s.getDataDir(), "artworks")
	if _, err := os.Stat(root); errors.Is(err, fs.ErrNotExist) {
		return 0, nil // nothing downloaded yet
	}
	removed := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil {
			// A transient error on one entry shouldn't abort the whole sweep -
			// returning nil tells WalkDir to keep going past it.
			slog.WarnContext(ctx, "purge variants: walk", slog.String("path", path), slog.Any("err", walkErr))
			return nil
		}
		if d.IsDir() || !isResizedVariant(d.Name()) {
			return nil
		}
		info, err := d.Info()
		// Can't stat the entry, or it isn't a 0-byte file → leave it and move on.
		if err != nil || info.Size() != 0 {
			return nil //nolint:nilerr // intentional: skip un-statable/non-empty entry, continue the sweep
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			slog.WarnContext(ctx, "purge variants: remove", slog.String("path", path), slog.Any("err", err))
			return nil
		}
		removed++
		return nil
	})
	if err != nil {
		return removed, err
	}
	span.SetAttributes(attribute.Int("removed", removed))
	return removed, nil
}

// isResizedVariant reports whether name is a "<base>_w<width>.webp" resized
// artwork variant - the only files PurgeCorruptVariants is allowed to delete.
// Full-resolution "<base>.webp" lacks the _w<digits> marker and is excluded.
func isResizedVariant(name string) bool {
	const suffix = ".webp"
	if !strings.HasSuffix(name, suffix) {
		return false
	}
	i := strings.LastIndex(name, "_w")
	if i < 0 {
		return false
	}
	digits := name[i+2 : len(name)-len(suffix)]
	if digits == "" {
		return false
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// StartBulkDownload launches a background goroutine to download all artwork records
// that have a SourceURL but no LocalPath. Returns a job ID immediately.
// Concurrent: at most concurrency downloads at once.
func (s *CommandService) StartBulkDownload(ctx context.Context, serverCtx context.Context) (string, int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.StartBulkDownload")
	defer span.End()
	if s.getDataDir() == "" {
		return "", 0, errors.New("dataDir not configured")
	}
	records, err := s.artworkQuery.ListMissingLocalPath(ctx)
	if err != nil {
		return "", 0, fmt.Errorf("list missing artwork: %w", err)
	}

	jobID := fmt.Sprintf("artdl-%d", time.Now().UnixNano())
	job := &DownloadJob{
		ID:    jobID,
		Total: len(records),
	}
	s.jobs.register(job)

	go s.runBulkDownload(serverCtx, job, records)
	return jobID, len(records), nil
}

// runBulkDownload executes the download job with capped concurrency (4 workers).
func (s *CommandService) runBulkDownload(ctx context.Context, job *DownloadJob, records []*metadata.ArtworkRecord) {
	const maxConcurrent = 4
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for _, rec := range records {
		select {
		case <-ctx.Done():
			// A bare `break` would only exit the select, then fall through to
			// wg.Add+go WITHOUT having acquired a sem slot - that goroutine's
			// deferred `<-sem` would steal a running worker's token, blocking
			// it forever and deadlocking wg.Wait(). Drain the workers that did
			// acquire a slot, mark done, and stop. (Mirrors RetryMissingArtworks.)
			wg.Wait()
			job.Done.Store(true)
			return
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.ErrorContext(ctx, "panic in artwork bulk download",
						slog.Any("recover", r), slog.String("id", rec.ID))
				}
			}()
			defer wg.Done()
			defer func() { <-sem }()
			destPath := s.artworkDestPath(rec)
			// Skip if already downloaded (re-entrant). Require size>0: the
			// downloader os.Create's the file before io.Copy, so a SIGKILL/OOM
			// mid-download leaves a 0-byte file at the content-addressed path -
			// reconciling that as "present" would mark the record done forever
			// (excluded from ListMissingLocalPath) with broken artwork.
			if fi, err := os.Stat(destPath); err == nil && fi.Size() > 0 {
				if err2 := s.artworkCmd.UpdateLocalPath(ctx, rec.ID, destPath); err2 != nil {
					slog.WarnContext(ctx, "artwork bulk: update already-present local path",
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
			if err := s.artworkCmd.UpdateLocalPath(ctx, rec.ID, destPath); err != nil {
				slog.WarnContext(ctx, "artwork bulk: save local path",
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
	return s.jobs.get(jobID)
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
	if s.getDataDir() == "" {
		return 0, 0
	}
	records, err := s.artworkQuery.ListMissingLocalPath(ctx)
	if err != nil {
		slog.WarnContext(ctx, "retry missing artworks: list", slog.Any("err", err))
		return 0, 0
	}
	return s.retryMissingRecords(ctx, nil, records)
}

// StartRetryMissing runs the missing-artwork retry as a pollable background job
// (GetDownloadJob), returning the job id immediately so the admin request
// doesn't block (and can't 504) on a large catalogue. serverCtx (not the
// request ctx) drives the work so a client disconnect doesn't cancel it.
func (s *CommandService) StartRetryMissing(ctx, serverCtx context.Context) (string, int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.StartRetryMissing")
	defer span.End()
	if s.getDataDir() == "" {
		return "", 0, errors.New("dataDir not configured")
	}
	records, err := s.artworkQuery.ListMissingLocalPath(ctx)
	if err != nil {
		return "", 0, fmt.Errorf("list missing artwork: %w", err)
	}
	jobID := fmt.Sprintf("artretry-%d", time.Now().UnixNano())
	job := &DownloadJob{ID: jobID, Total: len(records)}
	s.jobs.register(job)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(serverCtx, "panic in retry-missing job", slog.Any("recover", r), slog.String("job", jobID))
				job.Done.Store(true)
			}
		}()
		s.retryMissingRecords(serverCtx, job, records)
	}()
	return jobID, len(records), nil
}

// retryMissingRecords downloads the given missing-artwork records with capped
// concurrency. When job is non-nil it reports per-record progress and marks the
// job done; the (succeeded, failed) return feeds the hourly loop's telemetry.
func (s *CommandService) retryMissingRecords(ctx context.Context, job *DownloadJob, records []*metadata.ArtworkRecord) (succeeded, failed int) {
	if len(records) == 0 {
		if job != nil {
			job.Done.Store(true)
		}
		return 0, 0
	}
	const maxConcurrent = 4
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	var sucAtomic, failAtomic atomic.Int64
	for _, rec := range records {
		select {
		case <-ctx.Done():
			wg.Wait()
			if job != nil {
				job.Done.Store(true)
			}
			return int(sucAtomic.Load()), int(failAtomic.Load())
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.ErrorContext(ctx, "panic in retry missing artworks",
						slog.Any("recover", r), slog.String("id", rec.ID))
				}
			}()
			defer wg.Done()
			defer func() { <-sem }()
			if job != nil {
				defer job.Processed.Add(1)
			}
			destPath := s.artworkDestPath(rec)
			// Already on disk (caller previously downloaded outside the
			// pipeline, server crashed after fs write before DB update, etc.).
			// Just reconcile the DB row and count as a success. Require size>0
			// so a 0-byte file from a killed mid-download is re-fetched, not
			// reconciled as valid (which would strand the record permanently).
			if fi, err := os.Stat(destPath); err == nil && fi.Size() > 0 {
				if err2 := s.artworkCmd.UpdateLocalPath(ctx, rec.ID, destPath); err2 != nil {
					slog.WarnContext(ctx, "retry missing artworks: reconcile present file",
						slog.String("id", rec.ID), slog.Any("err", err2))
					failAtomic.Add(1)
					return
				}
				sucAtomic.Add(1)
				return
			}
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				slog.WarnContext(ctx, "retry missing artworks: mkdir",
					slog.String("id", rec.ID), slog.Any("err", err))
				failAtomic.Add(1)
				return
			}
			if err := s.DownloadArtwork(ctx, rec, destPath); err != nil {
				slog.DebugContext(ctx, "retry missing artworks: download still failing",
					slog.String("id", rec.ID), slog.Any("err", err))
				failAtomic.Add(1)
				return
			}
			if err := s.artworkCmd.UpdateLocalPath(ctx, rec.ID, destPath); err != nil {
				slog.WarnContext(ctx, "retry missing artworks: save local path",
					slog.String("id", rec.ID), slog.Any("err", err))
				failAtomic.Add(1)
				return
			}
			sucAtomic.Add(1)
		}()
	}
	wg.Wait()
	if job != nil {
		job.Done.Store(true)
	}
	succeeded = int(sucAtomic.Load())
	failed = int(failAtomic.Load())
	if succeeded > 0 || failed > 0 {
		slog.InfoContext(ctx, "retry missing artworks: pass complete",
			slog.Int("succeeded", succeeded),
			slog.Int("failed", failed),
			slog.Int("total", len(records)))
	}
	return succeeded, failed
}

// BackfillBlurhashes re-hashes every artwork row that has a LocalPath on
// disk but no blurhash. Retro-fits placeholders onto artworks ingested
// before migration 000095 (when the column was added). Idempotent - rows
// that already have a hash are filtered out by the repository query.
//
// Best-effort: per-record decode failures are logged at debug and the loop
// continues. Returns (succeeded, failed) for the admin UI.
func (s *CommandService) BackfillBlurhashes(ctx context.Context) (succeeded, failed int) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.BackfillBlurhashes")
	defer span.End()
	records, err := s.artworkQuery.ListMissingBlurhash(ctx)
	if err != nil {
		slog.WarnContext(ctx, "backfill blurhash: list", slog.Any("err", err))
		return 0, 0
	}
	return s.backfillBlurhashRecords(ctx, nil, records)
}

// StartBackfillBlurhashes runs the blurhash backfill as a pollable background
// job (GetDownloadJob), returning the job id immediately so the admin request
// doesn't block (~5 min on a 10k catalogue → would 504 behind a proxy).
func (s *CommandService) StartBackfillBlurhashes(ctx, serverCtx context.Context) (string, int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.StartBackfillBlurhashes")
	defer span.End()
	records, err := s.artworkQuery.ListMissingBlurhash(ctx)
	if err != nil {
		return "", 0, fmt.Errorf("list missing blurhash: %w", err)
	}
	jobID := fmt.Sprintf("artblur-%d", time.Now().UnixNano())
	job := &DownloadJob{ID: jobID, Total: len(records)}
	s.jobs.register(job)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(serverCtx, "panic in backfill-blurhash job", slog.Any("recover", r), slog.String("job", jobID))
				job.Done.Store(true)
			}
		}()
		s.backfillBlurhashRecords(serverCtx, job, records)
	}()
	return jobID, len(records), nil
}

// backfillBlurhashRecords re-hashes the given records with capped concurrency.
// When job is non-nil it reports per-record progress and marks the job done.
func (s *CommandService) backfillBlurhashRecords(ctx context.Context, job *DownloadJob, records []*metadata.ArtworkRecord) (succeeded, failed int) {
	if len(records) == 0 {
		if job != nil {
			job.Done.Store(true)
		}
		return 0, 0
	}
	// Lower concurrency than RetryMissingArtworks (which is network-bound):
	// decoding is CPU-bound, more parallelism mostly adds context-switching
	// overhead. 2 workers keeps the catalogue scan from saturating the box
	// while still finishing a 10k-row backfill in a few minutes.
	const maxConcurrent = 2
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	var sucAtomic, failAtomic atomic.Int64
	for _, rec := range records {
		select {
		case <-ctx.Done():
			wg.Wait()
			if job != nil {
				job.Done.Store(true)
			}
			return int(sucAtomic.Load()), int(failAtomic.Load())
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.ErrorContext(ctx, "panic in backfill blurhash",
						slog.Any("recover", r), slog.String("id", rec.ID))
				}
			}()
			defer wg.Done()
			defer func() { <-sem }()
			if job != nil {
				defer job.Processed.Add(1)
			}
			if rec.LocalPath == nil {
				return
			}
			hash, hashErr := computeBlurhash(*rec.LocalPath)
			if hashErr != nil {
				slog.DebugContext(ctx, "backfill blurhash: decode skipped",
					slog.String("id", rec.ID), slog.Any("err", hashErr))
				failAtomic.Add(1)
				return
			}
			if err := s.artworkCmd.UpdateBlurhash(ctx, rec.ID, hash); err != nil {
				slog.WarnContext(ctx, "backfill blurhash: persist",
					slog.String("id", rec.ID), slog.Any("err", err))
				failAtomic.Add(1)
				return
			}
			sucAtomic.Add(1)
		}()
	}
	wg.Wait()
	if job != nil {
		job.Done.Store(true)
	}
	succeeded = int(sucAtomic.Load())
	failed = int(failAtomic.Load())
	if succeeded > 0 || failed > 0 {
		slog.InfoContext(ctx, "backfill blurhash: pass complete",
			slog.Int("succeeded", succeeded),
			slog.Int("failed", failed),
			slog.Int("total", len(records)))
	}
	return succeeded, failed
}

// RetryMissingArtworksLoop runs RetryMissingArtworks periodically until
// ctx is cancelled. Intended to be launched as a goroutine at startup.
// Wired so a transient CDN/network outage during the initial enrichment
// burst is self-healing within the next interval - admin doesn't need
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

// ArtworkAutoDownloadHook exposes maybeAutoDownload as an ArtworkPostSaveHook so
// the enricher fires an auto-download the instant it persists a new artwork
// record (gated on the admin toggle inside maybeAutoDownload). Registered
// post-construction in wire.go via Enricher.SetArtworkPostSaveHook, because the
// CommandService is built WITH the enricher - the back-reference can only be set
// once both exist.
func (s *CommandService) ArtworkAutoDownloadHook() ArtworkPostSaveHook {
	return s.maybeAutoDownload
}

// maybeAutoDownload triggers an automatic download when auto-download is enabled.
// It is best-effort: errors are only logged.
func (s *CommandService) maybeAutoDownload(ctx context.Context, rec *metadata.ArtworkRecord) {
	if !s.autoDownloadEnabled.Load() || s.getDataDir() == "" || rec.SourceURL == nil {
		return
	}
	destPath := s.artworkDestPath(rec)
	if fi, err := os.Stat(destPath); err == nil && fi.Size() > 0 {
		// already downloaded (a 0-byte file = an interrupted download, re-fetch
		// it - matches runBulkDownload/retryMissingRecords/DownloadArtwork)
		return
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		slog.WarnContext(ctx, "auto-download artwork: mkdir", slog.String("id", rec.ID), slog.Any("err", err))
		return
	}
	if err := s.DownloadArtwork(ctx, rec, destPath); err != nil {
		slog.WarnContext(ctx, "auto-download artwork", slog.String("id", rec.ID), slog.Any("err", err))
		return
	}
	if err := s.artworkCmd.UpdateLocalPath(ctx, rec.ID, destPath); err != nil {
		slog.WarnContext(ctx, "auto-download artwork: save local path", slog.String("id", rec.ID), slog.Any("err", err))
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
	// Skip the network fetch when the content-addressed file is already on disk
	// (re-enrichment of an unchanged image): the path is keyed on the source
	// URL, so an existing non-empty file is the same bytes. Reconcile the DB
	// instead of re-downloading.
	if fi, statErr := os.Stat(destPath); statErr == nil && fi.Size() > 0 {
		record.LocalPath = &destPath
		if record.Blurhash == nil {
			if hash, hashErr := computeBlurhash(destPath); hashErr == nil {
				record.Blurhash = &hash
				if err := s.artworkCmd.UpdateBlurhash(ctx, record.ID, hash); err != nil {
					slog.WarnContext(ctx, "persist blurhash", slog.String("artwork.id", record.ID), slog.Any("err", err))
				}
			}
		}
		return nil
	}
	if err := s.downloader.Download(ctx, *record.SourceURL, destPath); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("download artwork: %w", err)
	}
	record.LocalPath = &destPath
	// Compute and persist the blurhash placeholder. Best-effort: a hash
	// failure (unsupported format, corrupt file) must not block ingestion.
	if hash, hashErr := computeBlurhash(destPath); hashErr != nil {
		slog.DebugContext(ctx, "blurhash encode skipped", slog.String("artwork.id", record.ID), slog.Any("err", hashErr))
	} else {
		record.Blurhash = &hash
		if err := s.artworkCmd.UpdateBlurhash(ctx, record.ID, hash); err != nil {
			slog.WarnContext(ctx, "persist blurhash", slog.String("artwork.id", record.ID), slog.Any("err", err))
		}
	}
	// Log-only: notification event - file already downloaded to destPath above
	evtArtwork := metadata.ArtworkDownloaded{MediaItemID: record.MediaItemID, Type: record.Type, LocalPath: destPath, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtArtwork); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtArtwork.EventName()), slog.Any("err", err))
	}
	return nil
}
