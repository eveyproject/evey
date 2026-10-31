package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/mkv"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/bgtasks"
	"github.com/eveyproject/evey/internal/pkg/cpubudget"
	"github.com/eveyproject/evey/internal/pkg/metrics"
	"github.com/eveyproject/evey/internal/pkg/scanindex"
	"github.com/eveyproject/evey/internal/pkg/sysinfo"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"golang.org/x/time/rate"
)

// ErrScanInProgress is returned by ScanLibrary when a scan is already running
// for the given library. Callers (gRPC handler, HTTP handler) should map this
// to a user-friendly 409/FailedPrecondition response.
var ErrScanInProgress = errors.New("scan already in progress")

// publish emits an event to the bus while detaching the publication from the
// caller's cancellation/deadline. Without this, an upstream timeout (e.g.
// Meilisearch index call) expires the scan ctx, the subsequent Publish sees
// ctx.Err() != nil, InMemoryEventBus drops the event before enqueueing, and
// downstream subscribers (trickplay, intro detection, metadata enricher,
// artwork downloader) never run for that media item. context.WithoutCancel
// preserves trace/values for observability while removing the cancel chain.
func (s *ScanService) publish(ctx context.Context, evt shared.Event) {
	if err := s.events.Publish(context.WithoutCancel(ctx), evt); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
}

func catalogGenres(item *library.MediaItem) []string {
	if item == nil || item.Catalog == nil {
		return nil
	}
	return item.Catalog.Genres
}

type TypedDirectoryScanner interface {
	ScanDirectory(ctx context.Context, libraryID, dir string) (int, error)
}

type ScanService struct {
	scanner library.FileScanner
	prober  library.StreamProber
	// staleReprobes paces the re-probes a StreamProbeVersion bump owes every
	// unchanged file (reprobeStaleVersion): without it the first scan after
	// an update reads the head of the whole library at once.
	staleReprobes *rate.Limiter
	// playingMedia lists the media items a play session is open on. A version
	// re-probe skips them: a refreshed frame rate moves the segment grid, and
	// the player's sealed rendition URLs would be refused 410 mid-film - an
	// error screen on the phone and TV builds that predate the reload. The
	// file is taken by a later scan. nil = not wired, nothing skipped.
	playingMedia func(ctx context.Context) (map[shared.MediaItemID]bool, error)
	subScanner   library.ExternalSubtitleScanner
	search       library.SearchIndex
	// indexQueue batches search documents during a scan (one Meilisearch task
	// per page instead of one per item); flushed by processNewItems.
	indexMu      sync.Mutex
	indexQueue   []*library.MediaItem
	indexBatch   atomic.Bool
	events       shared.EventBus
	mediaItems   library.MediaItemCommandRepo
	libQueries   library.LibraryQueryRepo
	libraries    library.LibraryCommandRepo
	mediaStreams library.MediaStreamRepository
	persons      library.PersonRepository
	mediaPersons library.MediaPersonRepository

	metadata     *MetadataExtractionService
	musicScanner TypedDirectoryScanner
	photoScanner TypedDirectoryScanner
	pruner       *OrphanPruner

	// Ingest-plan precompute (mkvgo ops.Ingest, v0.17.0) for freshly-scanned
	// files. All optional; a nil ingestPlanner disables the
	// feature outright regardless of what ingestCfg says. See ingest_plan.go.
	ingestCfg            shared.ServerConfigRepository
	ingestPlanner        library.IngestPlanner
	reindexer            mkv.Reindexer
	ingestActivePlayback func() int
	keyframeWarmer       func(ctx context.Context, mediaItemID, path string) bool
	interleave           InterleaveProber // optional; see WithInterleaveProber
	// budget bounds the per-file fan-out of processNewItems: each file holds
	// one slot of the library_scan share for as long as it is analysed. nil
	// (tests) falls back to scanWorkerCount.
	budget *cpubudget.Budget

	scanMu   sync.Mutex
	scanning map[string]bool
	// settleWindow is how long a file must go unmodified before it is
	// analysed (0: no wait); settleRetry holds the one rescan per library
	// already booked for the files still being written.
	settleWindow time.Duration
	settleRetry  map[string]*time.Timer
}

func NewScanService(
	libraries library.LibraryCommandRepo,
	libQueries library.LibraryQueryRepo,
	mediaItems library.MediaItemCommandRepo,
	mediaStreams library.MediaStreamRepository,
	persons library.PersonRepository,
	mediaPersons library.MediaPersonRepository,
	scanner library.FileScanner,
	prober library.StreamProber,
	search library.SearchIndex,
	events shared.EventBus,
) *ScanService {
	return &ScanService{
		libraries:     libraries,
		libQueries:    libQueries,
		mediaItems:    mediaItems,
		mediaStreams:  mediaStreams,
		persons:       persons,
		mediaPersons:  mediaPersons,
		scanner:       scanner,
		prober:        prober,
		search:        search,
		events:        events,
		pruner:        NewOrphanPruner(mediaItems, search, events, scanner),
		scanning:      make(map[string]bool),
		settleRetry:   make(map[string]*time.Timer),
		staleReprobes: rate.NewLimiter(staleReprobeRate, staleReprobeBurst),
	}
}

// staleReprobeRate and staleReprobeBurst pace the version re-probes: 2 files a
// second once the first 20 are done, about an hour for 7,300 files on a NAS,
// next to the scan's own CPU budget. A file over the pace is left unstamped
// and taken by a later scan.
const (
	staleReprobeRate  = rate.Limit(2)
	staleReprobeBurst = 20
)

// WithPlayingMedia wires the open play sessions (see playingMedia).
func (s *ScanService) WithPlayingMedia(list func(ctx context.Context) (map[shared.MediaItemID]bool, error)) *ScanService {
	s.playingMedia = list
	return s
}

// isPlaying reports whether a play session is open on id, read right before
// the re-probe (at the re-probe pace, so at most a few reads a second): a
// cached answer would let a session opened since go unseen. When the sessions
// cannot be read it answers true: a re-probe skipped is taken later, a player
// cut mid-film is not.
func (s *ScanService) isPlaying(ctx context.Context, id shared.MediaItemID) bool {
	if s.playingMedia == nil {
		return false
	}
	playing, err := s.playingMedia(ctx)
	if err != nil {
		slog.WarnContext(ctx, "scan: open play sessions unreadable, version re-probe waits",
			slog.String("mediaItemID", string(id)), slog.Any("err", err))
		return true
	}
	return playing[id]
}

// WithPruneGuard sets the mass-disappearance thresholds of the orphan prune.
func (s *ScanService) WithPruneGuard(g PruneGuard) *ScanService {
	s.pruner.WithGuard(g)
	return s
}

// WithStaleReprobePace overrides the version re-probe pace (tests).
func (s *ScanService) WithStaleReprobePace(r rate.Limit, burst int) *ScanService {
	s.staleReprobes = rate.NewLimiter(r, burst)
	return s
}

// WithFileSettle makes the scan leave alone a file modified less than window
// ago: it is still being copied. On 2026-09-25 an episode was ingested
// mid-transfer ("invalid VINT: leading zero byte" on its truncated tail) and
// every analysis ran on half a file.
func (s *ScanService) WithFileSettle(window time.Duration) *ScanService {
	s.settleWindow = window
	return s
}

// splitSettled keeps the files whose last write is at least settleWindow
// old. A future mtime beyond the window (a copied timestamp, clock skew) is
// not a copy in progress.
func (s *ScanService) splitSettled(files []library.ScannedFile, now time.Time) (settled []library.ScannedFile, unsettled int) {
	if s.settleWindow <= 0 {
		return files, 0
	}
	settled = files[:0:0]
	for _, f := range files {
		if age := now.Sub(f.ModTime); age < s.settleWindow && age > -s.settleWindow {
			unsettled++
			continue
		}
		settled = append(settled, f)
	}
	return settled, unsettled
}

// retryWhenSettled books one rescan of the library once the files skipped as
// still being written have had time to settle: the watcher sees no event
// after the copy's last write, so nothing else would bring them back soon.
func (s *ScanService) retryWhenSettled(ctx context.Context, libraryID string) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	if s.settleRetry[libraryID] != nil {
		return
	}
	ctx = context.WithoutCancel(ctx)
	s.settleRetry[libraryID] = time.AfterFunc(s.settleWindow+5*time.Second, func() {
		s.scanMu.Lock()
		delete(s.settleRetry, libraryID)
		s.scanMu.Unlock()
		if _, _, err := s.ScanLibrary(ctx, libraryID); err != nil && !errors.Is(err, ErrScanInProgress) {
			slog.WarnContext(ctx, "scan: rescan after files settled", slog.String("library_id", libraryID), slog.Any("err", err))
		}
	})
}

func (s *ScanService) WithMetadataExtraction(m *MetadataExtractionService) {
	s.metadata = m
}

func (s *ScanService) WithSubtitleScanner(sc library.ExternalSubtitleScanner) {
	s.subScanner = sc
}

func (s *ScanService) WithMusicScanner(sc TypedDirectoryScanner) {
	s.musicScanner = sc
}

func (s *ScanService) WithPhotoScanner(sc TypedDirectoryScanner) {
	s.photoScanner = sc
}

// WithIngestPlanning wires the mkvgo-fronted serving-plan precompute for
// freshly-scanned files, gated at runtime by the server_config key
// IngestServingPlanConfigKey (see ingest_plan.go). planner/store/reindexer
// come from the composition root: planner is the mkvgo adapter's Plan
// method, store persists the decision, reindexer drives the "plan+repair"
// mode's in-place seek-index repair (mkv.Reindexer, ModeAuto - same port
// app/maintenance/seek_index.go uses). A nil planner or store leaves the
// feature off even when the config says otherwise. Chainable.
func (s *ScanService) WithIngestPlanning(cfgRepo shared.ServerConfigRepository, planner library.IngestPlanner, reindexer mkv.Reindexer) *ScanService {
	s.ingestCfg = cfgRepo
	s.ingestPlanner = planner
	s.reindexer = reindexer
	return s
}

// WithIngestActivePlayback wires the live playback-session counter so the
// ingest-plan pacing backs off to ingestPoliteItemPause while someone is
// streaming - mirrors KeyframeBackfillWorker.WithActivePlayback. Optional;
// nil (the default) keeps the fixed ingestItemPause gap. Chainable.
func (s *ScanService) WithIngestActivePlayback(count func() int) *ScanService {
	s.ingestActivePlayback = count
	return s
}

// WithKeyframeWarmer arms the first-play fast path: right after a serving
// plan is stored for a fresh file whose strategy serves without a transcode,
// warm its keyframe index (the copy rung's prerequisite) instead of leaving
// it to the debounced backfill or the first request's inline walk. The
// warmer is expected to be cheap-bounded (KeyframeOnDemand.TryIndex).
// WithCPUBudget wires the background CPU budget the scan draws its per-file
// workers from. Registers the scan in the task registry so the dashboard
// shows what it is analysing.
func (s *ScanService) WithCPUBudget(b *cpubudget.Budget) *ScanService {
	s.budget = b
	bgtasks.Register(libraryScanTask)
	return s
}

func (s *ScanService) WithKeyframeWarmer(warm func(ctx context.Context, mediaItemID, path string) bool) *ScanService {
	s.keyframeWarmer = warm
	return s
}

// InterleaveProber measures a container's interleave defect (see
// library.MediaFile.InterleaveBackstepMs). Satisfied by the mkvgo adapter.
type InterleaveProber interface {
	ProbeInterleaveBackstepSec(ctx context.Context, path string) (float64, error)
}

// WithInterleaveProber measures every probed file for tracks stored in blocks
// (~11 MB of sampled reads) so the playback gate refuses it before a viewer
// stalls on it. Optional; without it the media-health scan is the only source.
func (s *ScanService) WithInterleaveProber(p InterleaveProber) *ScanService {
	s.interleave = p
	return s
}

// probeInterleave fills f.InterleaveBackstepMs; a probe failure keeps the
// previous value (no finding is not a clearance).
func (s *ScanService) probeInterleave(ctx context.Context, f *library.MediaFile, path string) {
	if s.interleave == nil || f == nil {
		return
	}
	back, err := s.interleave.ProbeInterleaveBackstepSec(ctx, path)
	if err != nil {
		slog.WarnContext(ctx, "scan: interleave probe failed", slog.String("path", path), slog.Any("err", err))
		return
	}
	f.InterleaveBackstepMs = int64(back * 1000)
}

type scanWork struct {
	file      library.ScannedFile
	parentID  string
	showTitle string
}

// ScanReport is the outcome of one library scan. FailedPaths lists the roots
// that could not be read (music and photo libraries keep going past one); a
// non-empty list means the scan is partial.
type ScanReport struct {
	Added, Updated int
	FailedPaths    []string
}

func (s *ScanService) ScanLibrary(ctx context.Context, libraryID string) (added, updated int, err error) {
	r, err := s.ScanLibraryReport(ctx, libraryID)
	return r.Added, r.Updated, err
}

// ScanLibraryReport scans a library and says which roots it could not read.
func (s *ScanService) ScanLibraryReport(ctx context.Context, libraryID string) (report ScanReport, err error) {
	var added, updated int
	defer func() { report.Added, report.Updated = added, updated }()
	ctx, span := tracing.StartSpan(ctx, "app.scan.ScanLibrary")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", libraryID))
	defer func() {
		span.SetAttributes(
			attribute.Int("items.added", added),
			attribute.Int("items.updated", updated),
		)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
		}
	}()

	s.scanMu.Lock()
	if s.scanning[libraryID] {
		s.scanMu.Unlock()
		return report, fmt.Errorf("%w for library %s", ErrScanInProgress, libraryID)
	}
	s.scanning[libraryID] = true
	defer func() {
		s.scanMu.Lock()
		delete(s.scanning, libraryID)
		s.scanMu.Unlock()
	}()
	s.scanMu.Unlock()

	lib, err := s.libQueries.GetByID(ctx, shared.LibraryID(libraryID))
	if err != nil {
		return report, fmt.Errorf("get library: %w", err)
	}
	span.SetAttributes(attribute.String("library.type", string(lib.Type)))

	s.publishScanStarted(ctx, libraryID)

	switch string(lib.Type) {
	case "music":
		if s.musicScanner == nil {
			return report, fmt.Errorf("music scanner not configured")
		}
		for _, p := range lib.Paths {
			n, e := s.musicScanner.ScanDirectory(ctx, libraryID, p)
			if e != nil {
				slog.WarnContext(ctx, "music scan path failed", slog.String("path", p), slog.Any("err", e))
				report.FailedPaths = append(report.FailedPaths, p)
				continue
			}
			added += n
		}
		s.finalizeScan(ctx, lib, libraryID, added, 0, 0, report.FailedPaths)
		return report, nil
	case "photo":
		if s.photoScanner == nil {
			return report, fmt.Errorf("photo scanner not configured")
		}
		for _, p := range lib.Paths {
			n, e := s.photoScanner.ScanDirectory(ctx, libraryID, p)
			if e != nil {
				slog.WarnContext(ctx, "photo scan path failed", slog.String("path", p), slog.Any("err", e))
				report.FailedPaths = append(report.FailedPaths, p)
				continue
			}
			added += n
		}
		s.finalizeScan(ctx, lib, libraryID, added, 0, 0, report.FailedPaths)
		return report, nil
	}

	idx, err := s.loadScanIndex(ctx, libraryID)
	if err != nil {
		return report, fmt.Errorf("ScanService.ScanLibrary: %w", err)
	}

	seenPaths, emptyRoots, failedPaths, toProcess, tvAdded, tvUpdated, err := s.scanDirectories(ctx, lib, idx)
	if err != nil {
		return report, fmt.Errorf("ScanService.ScanLibrary: %w", err)
	}
	report.FailedPaths = append(report.FailedPaths, failedPaths...)
	added += tvAdded
	updated += tvUpdated

	metrics.ScanQueueDepth.WithLabelValues(libraryID).Set(float64(len(toProcess)))
	a, u := s.processNewItems(ctx, libraryID, toProcess)
	metrics.ScanQueueDepth.WithLabelValues(libraryID).Set(0)
	added += a
	updated += u

	removed, pruneErr := s.pruner.Prune(ctx, libraryID, lib.Paths, seenPaths, emptyRoots)
	if pruneErr != nil {
		slog.WarnContext(ctx, "orphan cleanup failed", slog.String("libraryID", libraryID), slog.Any("err", pruneErr))
	}

	// Heal parser-pollution on existing orphans before finalising. The
	// regular scan path (detectChanges) short-circuits any file whose
	// size+mtime+fingerprint match the cached index, so a parser fix only
	// reaches newly-discovered files. The orphans the heal pass targets
	// have been on disk untouched since they were originally scanned -
	// without this they would stay frozen on titles written by the
	// pre-fix parser ("Die hard 2 2016P", "Léon remastered extended", …)
	// and match-orphans would loop forever on the same stall.
	healedTitles := s.healOrphanTitles(ctx, libraryID)
	updated += healedTitles

	s.finalizeScan(ctx, lib, libraryID, added, updated, removed, report.FailedPaths)
	return report, nil
}

// healOrphanTitles re-parses media_items.title for every orphan in the
// library whose stored Title still matches its ScanTitle (= untouched by
// TMDB enrichment or by a manual edit propagated via the override hook).
// When the current parser produces a different output for the same file
// path, the row is updated in place. Returns the count of items rewritten.
//
// Iterates ListByLibraryID and filters orphans client-side (CommandRepo
// has no orphan-aware list method; cost is one extra full library load
// per scan, which is acceptable at scan cadence). Idempotent - drift-only
// updates, no-op when the parser output matches the stored title.
func (s *ScanService) healOrphanTitles(ctx context.Context, libraryID string) int {
	ctx, span := tracing.StartSpan(ctx, "app.scan.healOrphanTitles")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", libraryID))
	if s.scanner == nil || s.mediaItems == nil {
		return 0
	}
	items, err := s.mediaItems.ListByLibraryID(ctx, shared.LibraryID(libraryID))
	if err != nil {
		slog.WarnContext(ctx, "heal orphan titles: list library failed",
			slog.String("library_id", libraryID), slog.Any("err", err))
		return 0
	}
	healed := 0
	for _, it := range items {
		if it.File == nil || it.File.Path == "" {
			continue
		}
		// Orphan filter: no provider IDs yet.
		if it.Catalog != nil && len(it.Catalog.ProviderIDs) > 0 {
			continue
		}
		fresh := s.scanner.ParseTitle(it.File.Path)
		if fresh == "" {
			continue
		}
		// Three exit cases ordered by likelihood:
		// 1. ScanTitle already aligned with the current parser → no-op.
		// 2. Title diverges from ScanTitle and from the parser output →
		//    the user (override-propagation hook) or TMDB enrichment
		//    rewrote Title; their intent wins, leave both alone.
		// 3. Otherwise we either have a fresh drift to heal (Title ==
		//    ScanTitle but parser improved) or a half-healed row from
		//    before scan_title became mutable in Update (Title already
		//    equals the parser output, ScanTitle still holds the dirty
		//    original - happens once per row recovering from that bug).
		if fresh == it.ScanTitle {
			continue
		}
		if it.Title != it.ScanTitle && it.Title != fresh {
			continue
		}
		it.Title = fresh
		it.ScanTitle = fresh
		it.UpdatedAt = time.Now()
		if err := s.mediaItems.Update(ctx, it); err != nil {
			slog.WarnContext(ctx, "heal orphan title: update failed",
				slog.String("media_item.id", string(it.ID)), slog.Any("err", err))
			continue
		}
		healed++
	}
	if healed > 0 {
		slog.InfoContext(ctx, "heal orphan titles: rewrote stale parser output",
			slog.String("library_id", libraryID), slog.Int("healed", healed))
	}
	return healed
}

func (s *ScanService) publishScanStarted(ctx context.Context, libraryID string) {
	s.publish(ctx, library.LibraryScanStarted{
		LibraryID:  shared.LibraryID(libraryID),
		OccurredAt: time.Now(),
	})
}

func (s *ScanService) loadScanIndex(ctx context.Context, libraryID string) (*scanindex.Index, error) {
	ctx, span := tracing.StartSpan(ctx, "app.scan.loadScanIndex")
	defer span.End()
	summaries, err := s.mediaItems.ListScanSummaries(ctx, shared.LibraryID(libraryID))
	if err != nil {
		return nil, fmt.Errorf("list existing items: %w", err)
	}
	entries := make([]scanindex.Summary, len(summaries))
	for i, row := range summaries {
		entries[i] = scanindex.Summary{
			ID:      row.ID,
			Path:    row.FilePath,
			Size:    row.FileSize,
			ModTime: row.ModTime,
			Hash:    row.FileFingerprint,
			// A streamless file still inside its bounded re-probe budget must be
			// reprocessed even when unchanged, so updateMediaFile re-probes it.
			// A row probed by an older prober revision is owed a head-only
			// re-probe so fields the prober has since learned get stored (no
			// prober wired = nothing to refresh).
			NeedsReprobe: (row.StreamProbeAttempts > 0 && row.StreamProbeAttempts < maxStreamProbeAttempts) ||
				(s.prober != nil && row.StreamProbeAttempts == 0 && row.StreamProbeVersion < library.StreamProbeVersion),
		}
	}
	return scanindex.New(entries), nil
}

func (s *ScanService) scanDirectories(ctx context.Context, lib *library.Library, idx *scanindex.Index) (seenPaths map[string]struct{}, emptyRoots, failedPaths []string, toProcess []scanWork, added, updated int, err error) {
	ctx, span := tracing.StartSpan(ctx, "app.scan.scanDirectories")
	defer span.End()
	seenPaths = make(map[string]struct{})
	unsettled := 0
	defer func() {
		if unsettled > 0 && err == nil {
			slog.InfoContext(ctx, "scan: files still being written, analysed once they settle",
				slog.String("library_id", string(lib.ID)), slog.Int("files", unsettled))
			s.retryWhenSettled(ctx, string(lib.ID))
		}
	}()

	for _, path := range lib.Paths {
		all, scanErr := s.scanner.Scan(ctx, path)
		if scanErr != nil {
			// An unreadable root (permission denied, a dropped NFS/SMB mount) must
			// not abort the whole scan, nor let the pruner drop the items under it.
			// Treat it like a degraded mount, exactly as the music and photo scans
			// already do: report it as a failed path and protect it from pruning
			// (emptyRoots feeds OrphanPruner's degraded-mount guard). The scan
			// carries on with the other roots and reports what it could not read,
			// instead of surfacing an opaque internal error to the operator.
			slog.WarnContext(ctx, "scan path failed", slog.String("path", path), slog.Any("err", scanErr))
			failedPaths = append(failedPaths, path)
			emptyRoots = append(emptyRoots, path)
			continue
		}
		// A file still being copied counts as present (the pruner must not
		// drop the item it may be replacing) but is not analysed yet.
		for _, f := range all {
			seenPaths[f.Path] = struct{}{}
		}
		files, n := s.splitSettled(all, time.Now())
		unsettled += n
		// A root that returned no files (and no error) is suspect: it may be a
		// dropped/empty NFS-SMB mount rather than a genuinely empty folder. Record
		// it so the pruner won't delete the items that live under it (see the
		// degraded-mount guard in OrphanPruner.Prune).
		if len(all) == 0 {
			emptyRoots = append(emptyRoots, path)
		}

		if lib.Type == "tvshow" {
			a, u, e := s.scanTVShowFiles(ctx, string(lib.ID), files)
			if e != nil {
				return nil, nil, nil, nil, 0, 0, e
			}
			added += a
			updated += u
			continue
		}

		work, u := s.detectChanges(ctx, files, idx)
		toProcess = append(toProcess, work...)
		updated += u
	}
	return seenPaths, emptyRoots, failedPaths, toProcess, added, updated, nil
}

func (s *ScanService) detectChanges(ctx context.Context, files []library.ScannedFile, idx *scanindex.Index) (toProcess []scanWork, moved int) {
	for _, f := range files {
		if _, unchanged := idx.Unchanged(f.Path, f.Size, f.ModTime); unchanged {
			// Extra fingerprint gate when both sides have one: catches the
			// rare inode/mtime coincidence where the OS reused both for
			// different content.
			exHash, _ := idx.ExistingHash(f.Path)
			hashMatches := exHash == "" || f.Fingerprint == "" || exHash == f.Fingerprint
			// A file indexed without streams is owed a bounded re-probe: don't
			// skip it on the fast path, let updateMediaFile re-probe it.
			if hashMatches && !idx.NeedsReprobe(f.Path) {
				continue
			}
		}

		if movedID, isMove := idx.Moved(f.Path, f.Fingerprint); isMove {
			fullItem, getErr := s.mediaItems.GetByID(ctx, shared.MediaItemID(movedID))
			if getErr == nil && fullItem.File != nil && s.scanner.FileExists(fullItem.File.Path) {
				// The matched item's file is still on disk: f shares its size+mtime
				// fingerprint but is a DISTINCT file (a collision, e.g. cp -p), not a
				// move. Process it as a new item instead of repointing the existing
				// one onto f's path (which would erase the original from the library).
				toProcess = append(toProcess, scanWork{file: f})
				continue
			}
			if getErr != nil {
				slog.WarnContext(ctx, "get moved item", slog.String("id", movedID), slog.Any("err", getErr))
			} else {
				if fullItem.File == nil {
					fullItem.File = &library.MediaFile{}
				}
				fullItem.File.Path = f.Path
				fullItem.File.ModTime = f.ModTime
				fullItem.UpdatedAt = time.Now()
				if updateErr := s.mediaItems.Update(ctx, fullItem); updateErr != nil {
					slog.WarnContext(ctx, "update moved item path", slog.String("id", movedID), slog.Any("err", updateErr))
				}
			}
			moved++
			continue
		}

		toProcess = append(toProcess, scanWork{file: f})
	}
	return toProcess, moved
}

func (s *ScanService) processNewItems(ctx context.Context, libraryID string, toProcess []scanWork) (added, updated int) {
	ctx, span := tracing.StartSpan(ctx, "app.scan.processNewItems")
	defer span.End()
	// 5min accommodates ffprobe + extractChapters + meili index on 4K 40GB+
	// files; 30s was sufficient for 1080p but timed out on UHD content,
	// starving the final Update of its budget.
	const workerTimeout = 5 * time.Minute

	s.indexBatch.Store(true)
	defer func() {
		s.indexBatch.Store(false)
		s.flushIndex(ctx)
	}()
	var addedAtomic, updatedAtomic atomic.Int64
	budget, workers := s.scanWorkers()
	if budget != nil {
		bgtasks.Queue(libraryScanTask, len(toProcess))
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, w := range toProcess {
		wg.Add(1)
		sem <- struct{}{}
		go func(w scanWork) {
			defer wg.Done()
			defer func() { <-sem }()
			fileCtx := ctx
			if budget != nil {
				// One slot per file for the whole analysis (probe, chapters,
				// ingest plan): the share is enforced here, live, whatever the
				// pool size was when the pass started.
				itemCtx, done := bgtasks.BeginCtx(ctx, libraryScanTask, w.file.Path)
				defer done()
				heldCtx, release, err := budget.Acquire(itemCtx, libraryScanTask, 1)
				if err != nil {
					return
				}
				defer release()
				fileCtx = heldCtx
			}
			defer func() {
				if r := recover(); r != nil {
					// Without the path this says a worker died, not WHICH file
					// killed it - and a panic here is almost always one
					// specific malformed container, so the path is the whole
					// diagnosis.
					slog.ErrorContext(ctx, "panic recovered in scan worker",
						slog.String("library_id", libraryID),
						slog.String("path", w.file.Path),
						slog.Any("error", r),
						slog.String("stack", string(debug.Stack())))
				}
			}()
			workerCtx, cancel := context.WithTimeout(fileCtx, workerTimeout)
			defer cancel()
			a, u, e := s.upsertMediaFile(workerCtx, libraryID, w.parentID, w.file, w.showTitle)
			if e != nil {
				slog.WarnContext(workerCtx, "upsert media file", slog.String("path", w.file.Path), slog.Any("err", e))
				return
			}
			addedAtomic.Add(int64(a))
			updatedAtomic.Add(int64(u))
		}(w)
	}
	wg.Wait()
	return int(addedAtomic.Load()), int(updatedAtomic.Load())
}

const indexFlushEvery = 200

// indexItem queues the document during a scan and indexes it straight away
// outside one (single-file ingests).
func (s *ScanService) indexItem(ctx context.Context, item *library.MediaItem) {
	if s.search == nil || item == nil {
		return
	}
	if !s.indexBatch.Load() {
		if err := s.search.IndexMediaItem(ctx, item); err != nil {
			slog.WarnContext(ctx, "index media item failed", slog.String("mediaItemID", string(item.ID)), slog.Any("err", err))
		}
		return
	}
	s.indexMu.Lock()
	s.indexQueue = append(s.indexQueue, item)
	full := len(s.indexQueue) >= indexFlushEvery
	s.indexMu.Unlock()
	if full {
		s.flushIndex(ctx)
	}
}

func (s *ScanService) flushIndex(ctx context.Context) {
	s.indexMu.Lock()
	batch := s.indexQueue
	s.indexQueue = nil
	s.indexMu.Unlock()
	if len(batch) == 0 || s.search == nil {
		return
	}
	if err := s.search.IndexMediaItemsBatch(ctx, batch); err != nil {
		slog.WarnContext(ctx, "index media item failed", slog.Int("count", len(batch)), slog.Any("err", err))
	}
}

func (s *ScanService) finalizeScan(ctx context.Context, lib *library.Library, libraryID string, added, updated, removed int, failedPaths []string) {
	ctx, span := tracing.StartSpan(ctx, "app.scan.finalizeScan")
	defer span.End()

	now := time.Now()
	lib.LastScannedAt = &now
	if err := s.libraries.Update(ctx, lib); err != nil {
		slog.WarnContext(ctx, "update library last scanned failed", slog.String("libraryID", libraryID), slog.Any("err", err))
	}

	s.publish(ctx, library.LibraryScanCompleted{
		LibraryID:    shared.LibraryID(libraryID),
		ItemsAdded:   added,
		ItemsUpdated: updated,
		ItemsRemoved: removed,
		FailedPaths:  failedPaths,
		OccurredAt:   now,
	})
}

func (s *ScanService) upsertMediaFile(ctx context.Context, libraryID, parentID string, f library.ScannedFile, seriesTitle ...string) (added, updated int, err error) {
	return s.upsertMediaFileWithIndex(ctx, libraryID, parentID, f, nil, seriesTitle...)
}

// upsertMediaFileWithIndex is upsertMediaFile with the caller's preloaded
// path index, letting a scan that has already read the whole library skip a
// per-file SELECT. Passing nil is exactly the old behaviour.
func (s *ScanService) upsertMediaFileWithIndex(ctx context.Context, libraryID, parentID string, f library.ScannedFile, byPath map[string]*library.MediaItem, seriesTitle ...string) (added, updated int, err error) {
	showTitle := ""
	if len(seriesTitle) > 0 {
		showTitle = seriesTitle[0]
	}
	existing, err := s.findExistingMediaFile(ctx, libraryID, f, byPath)
	if err != nil {
		return 0, 0, err
	}
	if existing == nil {
		created, err := s.createMediaFile(ctx, libraryID, parentID, f, showTitle)
		if err != nil {
			return 0, 0, err
		}
		if !created {
			return 0, 0, nil // skipped (failed invariants) - not an error, not counted
		}
		return 1, 0, nil
	}
	s.updateMediaFile(ctx, existing, parentID, f, showTitle)
	return 0, 1, nil
}

// findExistingMediaFile locates the media item that already owns the scanned
// file. Path is the authoritative identity, checked first: a re-scan of an
// existing file must resolve to the item at that path, never to a different file
// that merely shares its size+mtime fingerprint. Only when the path is new do we
// fall back to the fingerprint to detect a move/rename - and because the
// fingerprint is size+mtime (not content-derived), distinct files can collide,
// so a fingerprint match in a DIFFERENT library is rejected to prevent
// cross-library repointing. Returns (nil, nil) when no match exists (caller
// creates a fresh item).
//
// NOTE: a same-library size+mtime collision (e.g. `cp -p` of a distinct file)
// can still mis-resolve via the fingerprint fallback; a content-derived
// fingerprint is the full fix (deferred - changing the format forces a rescan).
func (s *ScanService) findExistingMediaFile(ctx context.Context, libraryID string, f library.ScannedFile, byPath map[string]*library.MediaItem) (*library.MediaItem, error) {
	// byPath is the caller's already-loaded snapshot of this library. The TV
	// path preloads the whole library anyway (to resolve show/season
	// containers), so hitting the DB again per episode was one SELECT per
	// episode per scan pass - ~5000 queries every couple of minutes on a large
	// series library, all of them re-fetching rows already in memory.
	//
	// A hit here is authoritative. A MISS is not: the file may exist under a
	// different path (a move/rename), which is exactly what the fingerprint
	// fallback below resolves - so a miss still falls through to the DB.
	if byPath != nil {
		if hit, ok := byPath[f.Path]; ok {
			return hit, nil
		}
	}
	existing, getErr := s.mediaItems.GetByFilePath(ctx, f.Path)
	if getErr == nil {
		return existing, nil
	}
	if !errors.Is(getErr, shared.ErrNotFound) {
		return nil, fmt.Errorf("check media item %s: %w", f.Path, getErr)
	}
	// New path: try the fingerprint to catch a move/rename.
	if f.Fingerprint == "" {
		return nil, nil
	}
	existing, getErr = s.mediaItems.GetByFileFingerprint(ctx, f.Fingerprint)
	if errors.Is(getErr, shared.ErrNotFound) {
		return nil, nil
	}
	if getErr != nil {
		return nil, fmt.Errorf("check media item %s: %w", f.Path, getErr)
	}
	if existing != nil && string(existing.LibraryID) != libraryID {
		// Cross-library fingerprint collision: treat as a new file rather than
		// repoint another library's item to this path.
		return nil, nil
	}
	// Same-library fingerprint match is only a MOVE if the old file is gone. If
	// it's still on disk, two distinct files share a size+mtime fingerprint (a
	// collision, e.g. `cp -p`) - don't repoint the existing item onto this path,
	// create a fresh one instead.
	if existing != nil && existing.File != nil && s.scanner.FileExists(existing.File.Path) {
		return nil, nil
	}
	return existing, nil
}

// maxStreamProbeAttempts bounds how many times the scanner re-probes a file
// that was indexed without streams before giving up. The create attempt counts
// as 1, so this allows one retry on a later scan for a transient failure, then
// stops so a durably-unprobeable file isn't re-probed forever. An admin
// reindex/repair resets the counter (a fresh probe success sets it to 0).
const maxStreamProbeAttempts = 2

// createMediaFile materializes a new MediaItem + child enrichment
// (probe streams, chapters, meili index) and publishes MediaItemAdded.
// Called only when findExistingMediaFile returned nil.
// createMediaFile builds and persists a new MediaItem. Returns created=false
// (with a nil error) when the file failed the domain invariants and was skipped,
// so the caller doesn't count a skipped file as added.
func (s *ScanService) createMediaFile(ctx context.Context, libraryID, parentID string, f library.ScannedFile, showTitle string) (created bool, err error) {
	// Enforce the hierarchy invariant up front: an episode or season without a
	// parent ID would persist as an orphan that only surfaces later as a broken
	// row (mirrors library.ErrMissingParent in the NewMediaItem constructor).
	if (f.Type == library.Episode || f.Type == library.Season) && parentID == "" {
		return false, fmt.Errorf("build media item %s: %w", f.Path, library.ErrMissingParent)
	}
	// Build through the canonical constructor so the domain invariants (valid
	// type, non-empty title, parent for episode/season) are enforced at the
	// boundary instead of trusting the scanner to have set them. A real scan
	// always satisfies them; a degenerate file (parser yielded no title, or a
	// future scan path that forgot the type) is logged and skipped rather than
	// persisted as a broken row.
	opts := []library.MediaItemOption{
		library.WithMediaItemID(shared.MediaItemID(uuid.New().String())),
		library.WithMediaItemParent(parentID),
		library.WithMediaItemFile(&library.MediaFile{
			Path:        f.Path,
			Size:        f.Size,
			Fingerprint: f.Fingerprint,
			ModTime:     f.ModTime,
		}),
	}
	if f.Episode != nil {
		opts = append(opts, library.WithMediaItemIndexNumber(*f.Episode))
	}
	if f.Year != nil {
		opts = append(opts, library.WithMediaItemYear(*f.Year))
	}
	item, err := library.NewMediaItem(shared.LibraryID(libraryID), f.Type, f.Title, opts...)
	if err != nil {
		slog.WarnContext(ctx, "scan: skipping media item that fails domain invariants",
			slog.String("path", f.Path), slog.Any("err", err))
		return false, nil
	}
	if err := s.mediaItems.Create(ctx, item); err != nil {
		return false, fmt.Errorf("create media item: %w", err)
	}
	dur, container, probedOK := s.ProbeAndStoreStreams(ctx, string(item.ID), f.Path)
	if !probedOK {
		// The probe stored no streams (timeout / NAS I/O hiccup / streamless
		// file). Record the first failed attempt so a later scan re-probes
		// (bounded) instead of leaving the item streamless forever.
		item.File.StreamProbeAttempts = 1
	} else {
		item.File.StreamProbeVersion = library.StreamProbeVersion
	}
	if !dur.IsZero() {
		item.File.Duration = dur
	}
	// Persisted once the duration is known: the column was never written by any
	// prober, so every consumer had to re-derive it (or fork ffprobe for it).
	item.File.Bitrate = item.File.BitrateBps()
	if container != "" {
		item.File.ContainerFormat = container
	}
	s.probeInterleave(ctx, item.File, f.Path)
	// Persist whenever the probe taught us anything - container included. Gating
	// only on duration silently dropped ContainerFormat for a file whose probe
	// succeeded but reported no duration: a successful probe leaves
	// StreamProbeAttempts at 0, so NeedsReprobe stays false and the
	// unchanged-file fast path never revisits it. The column then stays empty
	// forever, and an empty container reads as "incompatible" in the playback
	// decision, forcing a remux that the source never needed.
	if !dur.IsZero() || container != "" || !probedOK || item.File.StreamProbeVersion != 0 {
		if err := s.mediaItems.Update(ctx, item); err != nil {
			slog.WarnContext(ctx, "update media item after probe failed", slog.String("mediaItemID", string(item.ID)), slog.Any("err", err))
		}
	}
	// Ingest-plan precompute runs for brand-new files ONLY (this is
	// createMediaFile, never reached by an unchanged re-scan - see
	// detectChanges' short-circuit and updateMediaFile for the changed-content
	// path, which intentionally does not call this). No-op unless
	// WithIngestPlanning wired a planner+store and the server_config mode is
	// not "off".
	s.applyIngestPlan(ctx, item.ID, f.Path)
	if s.metadata != nil {
		s.metadata.extractAndStoreChapters(ctx, string(item.ID), f.Path)
	}
	s.indexItem(ctx, item)
	s.publish(ctx, library.MediaItemAdded{
		MediaItemID: item.ID,
		LibraryID:   shared.LibraryID(libraryID),
		Title:       item.Title,
		SeriesTitle: showTitle,
		Type:        item.Type,
		Genres:      catalogGenres(item),
		OccurredAt:  time.Now(),
	})
	return true, nil
}

// updateMediaFile reconciles an existing MediaItem against the freshly
// scanned file. Handles two paths: the fast "file moved / reparented
// but content unchanged" loop, and the full "content replaced" rescan
// that re-probes streams + chapters and re-publishes MediaItemAdded so
// translation/artwork jobs re-enrich the item.
func (s *ScanService) updateMediaFile(ctx context.Context, existing *library.MediaItem, parentID string, f library.ScannedFile, showTitle string) {
	if existing.File == nil {
		existing.File = &library.MediaFile{}
	}
	unchanged := existing.File.Size == f.Size && (f.Fingerprint == "" || existing.File.Fingerprint == f.Fingerprint)
	if unchanged {
		// Backfill ModTime when it was never persisted (older rows created
		// before the scanner started setting File.ModTime). Without this,
		// SameMtime would keep returning false each scan for those rows
		// and we'd pointlessly re-enter updateMediaFile every tick.
		mtimeStale := !scanindex.SameMtime(existing.File.ModTime, f.ModTime)
		if existing.File.Path != f.Path || existing.ParentID != parentID || mtimeStale {
			existing.File.Path = f.Path
			existing.ParentID = parentID
			existing.File.ModTime = f.ModTime
			existing.UpdatedAt = time.Now()
			if err := s.mediaItems.Update(ctx, existing); err != nil {
				slog.WarnContext(ctx, "update media item failed", slog.String("mediaItemID", string(existing.ID)), slog.Any("err", err))
			}
		}
		// A prior probe left this file without streams (StreamProbeAttempts in
		// (0, max)). The change-detection fast path would otherwise never
		// re-probe an unchanged file, so it would stay streamless until a manual
		// reindex. Re-probe a bounded number of times so a transient failure
		// self-heals, without re-probing a durably-unprobeable file every scan.
		if existing.File.StreamProbeAttempts > 0 && existing.File.StreamProbeAttempts < maxStreamProbeAttempts {
			s.reprobeStreamlessFile(ctx, existing, f.Path)
		} else if existing.File.StreamProbeAttempts == 0 && existing.File.StreamProbeVersion < library.StreamProbeVersion &&
			s.staleReprobes.Allow() && !s.isPlaying(ctx, existing.ID) {
			s.reprobeStaleVersion(ctx, existing, f.Path)
		}
		return
	}
	previousDurationMs := existing.File.Duration.Milliseconds()
	existing.ParentID = parentID
	existing.File.Path = f.Path
	existing.File.Size = f.Size
	existing.File.Fingerprint = f.Fingerprint
	existing.File.ModTime = f.ModTime
	existing.UpdatedAt = time.Now()
	// Persist the new fingerprint + mtime BEFORE the heavy probe/chapters
	// pipeline. Otherwise on a 4K 46GB file the probe + extractChapters can
	// eat the whole workerCtx budget (30s) and the Update that follows
	// fails with 'begin tx: context deadline exceeded' - the stale
	// fingerprint then survives, the next scan re-flags the file as
	// changed, and the loop pins CPU forever. Updating first means a
	// timeout only loses the duration_ms refresh (recoverable next scan)
	// instead of the change-detection cornerstone.
	if err := s.mediaItems.Update(ctx, existing); err != nil {
		slog.WarnContext(ctx, "update media item failed", slog.String("mediaItemID", string(existing.ID)), slog.Any("err", err))
	}
	dur, container, probedOK := s.ProbeAndStoreStreams(ctx, string(existing.ID), f.Path)
	// New content → a fresh bounded re-probe budget: 0 on success, 1 on failure.
	if probedOK {
		existing.File.StreamProbeAttempts = 0
		existing.File.StreamProbeVersion = library.StreamProbeVersion
	} else {
		existing.File.StreamProbeAttempts = 1
	}
	if !dur.IsZero() {
		existing.File.Duration = dur
	}
	existing.File.Bitrate = existing.File.BitrateBps()
	if container != "" {
		existing.File.ContainerFormat = container
	}
	s.probeInterleave(ctx, existing.File, f.Path)
	existing.UpdatedAt = time.Now()
	if err := s.mediaItems.Update(ctx, existing); err != nil {
		slog.WarnContext(ctx, "update media item after probe failed", slog.String("mediaItemID", string(existing.ID)), slog.Any("err", err))
	}
	if s.metadata != nil {
		s.metadata.extractAndStoreChapters(ctx, string(existing.ID), f.Path)
	}
	s.indexItem(ctx, existing)
	newDurationMs := int64(0)
	if probedOK {
		newDurationMs = existing.File.Duration.Milliseconds()
	}
	s.publish(ctx, library.MediaItemUpdated{
		MediaItemID:        existing.ID,
		LibraryID:          existing.LibraryID,
		Reason:             library.UpdateReasonFileReplaced,
		PreviousDurationMs: previousDurationMs,
		DurationMs:         newDurationMs,
		OccurredAt:         time.Now(),
	})
	if existing.Overview == "" {
		s.publish(ctx, library.MediaItemAdded{
			MediaItemID: existing.ID,
			LibraryID:   existing.LibraryID,
			Title:       existing.Title,
			SeriesTitle: showTitle,
			Type:        existing.Type,
			Genres:      catalogGenres(existing),
			OccurredAt:  time.Now(),
		})
	}
}

// ProbeAndStoreStreams probes filePath and replaces the item's media_streams.
// It returns the container duration and probedOK: true only when the probe
// yielded at least one container stream AND the store succeeded. probedOK=false
// (probe error, a genuinely streamless file, or a failed ReplaceBatch) is the
// signal the scanner uses to drive the bounded re-probe counter so a transient
// failure self-heals on a later scan.
// reprobeStreamlessFile retries the stream probe for an unchanged file that was
// indexed without streams (StreamProbeAttempts in (0, max)). On success it
// resets the counter to 0 and refreshes the duration; on failure it increments
// so the retries stay bounded. Persists the updated counter either way.
func (s *ScanService) reprobeStreamlessFile(ctx context.Context, item *library.MediaItem, path string) {
	dur, container, probedOK := s.ProbeAndStoreStreams(ctx, string(item.ID), path)
	if probedOK {
		item.File.StreamProbeAttempts = 0
		item.File.StreamProbeVersion = library.StreamProbeVersion
		if !dur.IsZero() {
			item.File.Duration = dur
		}
		item.File.Bitrate = item.File.BitrateBps()
		if container != "" {
			item.File.ContainerFormat = container
		}
		s.probeInterleave(ctx, item.File, path)
		slog.InfoContext(ctx, "scan: streamless file re-probe succeeded",
			slog.String("mediaItemID", string(item.ID)), slog.String("path", path))
	} else {
		item.File.StreamProbeAttempts++
		slog.WarnContext(ctx, "scan: streamless file re-probe still failing",
			slog.String("mediaItemID", string(item.ID)), slog.String("path", path),
			slog.Int("attempts", item.File.StreamProbeAttempts))
	}
	item.UpdatedAt = time.Now()
	if err := s.mediaItems.Update(ctx, item); err != nil {
		slog.WarnContext(ctx, "update media item after re-probe failed", slog.String("mediaItemID", string(item.ID)), slog.Any("err", err))
	}
}

// reprobeStaleVersion refreshes the metadata of an unchanged file last probed
// by an older prober revision and stamps the current StreamProbeVersion. The
// streams are refreshed in place (RefreshProbedFields), never replaced: a
// replace resets what the backfills derived on the rows (level, idet,
// frame-rate mode, keyframes) and sends their workers back over every file -
// a whole-library decode pass for a metadata refresh. So it publishes nothing
// either: no trickplay, intro, chapter or cache work follows. A probe that
// finds other streams than the stored ones writes none (logged); a failed
// probe keeps the old streams. Either way the version is stamped: retrying an
// unchanged file every scan only reruns the probe, and a change to the file
// probes it again in full.
func (s *ScanService) reprobeStaleVersion(ctx context.Context, item *library.MediaItem, path string) {
	item.File.StreamProbeVersion = library.StreamProbeVersion
	defer func() {
		if err := s.mediaItems.Update(ctx, item); err != nil {
			slog.WarnContext(ctx, "update media item after version re-probe failed", slog.String("mediaItemID", string(item.ID)), slog.Any("err", err))
		}
	}()
	if s.prober == nil {
		return
	}
	result, err := s.prober.ProbeStreams(ctx, path)
	if err != nil || len(result.Streams) == 0 {
		slog.WarnContext(ctx, "scan: version re-probe failed, streams kept",
			slog.String("mediaItemID", string(item.ID)), slog.String("path", path), slog.Any("err", err))
		return
	}
	refreshed, err := s.mediaStreams.RefreshProbedFields(ctx, item.ID, result.Streams)
	switch {
	case errors.Is(err, library.ErrStreamSetChanged):
		slog.WarnContext(ctx, "scan: version re-probe found other streams than the stored ones, none written",
			slog.String("mediaItemID", string(item.ID)), slog.String("path", path))
	case err != nil:
		slog.WarnContext(ctx, "scan: version re-probe could not refresh the streams",
			slog.String("mediaItemID", string(item.ID)), slog.Any("err", err))
	case refreshed.Changed > 0:
		slog.InfoContext(ctx, "scan: version re-probe refreshed stream metadata",
			slog.String("mediaItemID", string(item.ID)), slog.Int("streams", refreshed.Changed))
	}
	// What the refresh left against the probe, field by field: the rest of
	// the file was refreshed all the same.
	if len(refreshed.Kept) > 0 {
		slog.InfoContext(ctx, "scan: version re-probe kept stored values the probe disagrees with",
			slog.String("mediaItemID", string(item.ID)), slog.String("path", path), slog.Any("kept", refreshed.Kept))
	}
	if !result.Duration.IsZero() {
		item.File.Duration = result.Duration
	}
	item.File.Bitrate = item.File.BitrateBps()
	if result.Container != "" {
		item.File.ContainerFormat = result.Container
	}
	item.UpdatedAt = time.Now()
}

func (s *ScanService) ProbeAndStoreStreams(ctx context.Context, mediaItemID, filePath string) (dur shared.Duration, container string, probedOK bool) {
	ctx, span := tracing.StartSpan(ctx, "app.scan.ProbeAndStoreStreams")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	if s.prober == nil {
		return shared.Duration{}, "", false
	}
	result, err := s.prober.ProbeStreams(ctx, filePath)
	if err != nil {
		// Don't swallow: a silent probe failure indexes the item with no
		// streams and zero duration, leaving operators no trace of why.
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "probe failed")
		slog.WarnContext(ctx, "ProbeAndStoreStreams: probe failed",
			slog.String("mediaItemID", mediaItemID),
			slog.String("filePath", filePath),
			slog.Any("err", err))
		return shared.Duration{}, "", false
	}
	// Success is judged on the CONTAINER streams, before external sidecar subs
	// are appended: a file whose container yields nothing but happens to have a
	// neighbouring .srt must still count as a failed probe (and be re-probed).
	containerStreams := len(result.Streams)
	for _, st := range result.Streams {
		st.MediaItemID = shared.MediaItemID(mediaItemID)
	}

	var extSubs []*library.MediaStream
	if s.metadata != nil {
		extSubs = s.metadata.discoverExternalSubtitles(result.Streams, mediaItemID, filePath)
	} else if s.subScanner != nil {
		m := &MetadataExtractionService{subScanner: s.subScanner}
		extSubs = m.discoverExternalSubtitles(result.Streams, mediaItemID, filePath)
	}
	result.Streams = append(result.Streams, extSubs...)
	allStreams := result.Streams

	// A probe that succeeds but yields ZERO container streams is a FAILED probe
	// (ffprobe can exit 0 with an empty stream list on a truncated / mid-copy /
	// odd file; the success comment above says so). ReplaceBatch deletes every
	// existing row before inserting, so writing the externals-only slice here would
	// WIPE the known video/audio metadata of an item that probed fine before.
	// Preserve the persisted container streams and merge the freshly-discovered
	// external subs instead: a new item still records its sidecar sub (nothing to
	// preserve), while an existing one keeps its streams until a real reprobe
	// (containerStreams > 0) replaces them.
	if containerStreams == 0 {
		existing, gerr := s.mediaStreams.GetByMediaItemID(ctx, shared.MediaItemID(mediaItemID))
		if gerr != nil {
			slog.WarnContext(ctx, "preserve streams on empty probe: reload failed",
				slog.String("mediaItemID", mediaItemID), slog.Any("err", gerr))
			return result.Duration, result.Container, false
		}
		preserved := make([]*library.MediaStream, 0, len(existing))
		for _, st := range existing {
			if !st.IsExternal {
				preserved = append(preserved, st)
			}
		}
		preserved = append(preserved, extSubs...)
		allStreams = preserved
	}

	if err := s.mediaStreams.ReplaceBatch(ctx, shared.MediaItemID(mediaItemID), allStreams); err != nil {
		slog.WarnContext(ctx, "replace media streams failed", slog.String("mediaItemID", mediaItemID), slog.Any("err", err))
		return result.Duration, result.Container, false
	}
	return result.Duration, result.Container, containerStreams > 0
}

// libraryScanTask is the scan's name in the task registry and the CPU budget.
const libraryScanTask = "library_scan"

// scanWorkers is the per-file fan-out of one pass and the budget it draws
// on. EVEY_LIBRARY_SCAN_WORKERS is the operator sizing the pool by hand and
// wins outright (no budget); otherwise the pool is the task's CPU share and
// every file holds a slot of it. Without a budget (tests), scanWorkerCount.
func (s *ScanService) scanWorkers() (*cpubudget.Budget, int) {
	if n, ok := scanWorkersFromEnv(); ok {
		return nil, n
	}
	if s.budget == nil {
		return nil, scanWorkerCount()
	}
	return s.budget, s.budget.Slots(libraryScanTask)
}

func scanWorkersFromEnv() (int, bool) {
	if v := os.Getenv("EVEY_LIBRARY_SCAN_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
}

// scanWorkerCount is the pool size with no budget wired: the env override,
// else sysinfo.CPUBudget() (the cgroup CPU quota when one is set, else
// runtime.NumCPU()) - the pre-budget behaviour, kept for the tests and the
// tools that build a scan service without the composition root.
func scanWorkerCount() int {
	if n, ok := scanWorkersFromEnv(); ok {
		return n
	}
	return sysinfo.CPUBudget()
}

// playbackScanBackoff is the periodic scan's cadence while playback is active,
// against its normal interval (2 min by default).
//
// The scan walks every library root - filepath.WalkDir, one getdents per
// directory - on the SAME disk the viewer is streaming from. On a weak NAS a
// cold pass over a large library is tens of seconds of seeks in direct
// contention with playback.
//
// Backing off rather than skipping outright: this scan is the fallback for
// libraries whose inotify watcher missed something, so suspending it for a
// whole evening of viewing would delay discovery until everyone stopped
// watching. 15 minutes keeps the fallback alive at a fraction of the I/O.
const playbackScanBackoff = 15 * time.Minute

// watchedScanInterval is how often a library whose watch is PROVEN alive still
// gets a full walk. inotify handles the real-time path; this is only a backstop
// for what inotify cannot see even when healthy - most importantly a network
// share written by another host, where the local watch is alive and blind.
//
// An hour rather than the old two minutes: that is ~97% of the passes removed,
// while keeping a bounded worst case for the blind spots. Libraries whose watch
// is NOT proven alive keep the operator's configured interval.
const watchedScanInterval = time.Hour

// shouldScanNow decides whether this tick pays for a full library walk.
// Exported behaviour is the same as before when no playback gate is wired.
//
// The first tick always scans (lastScan zero) so a server that boots with a
// stale library is not left waiting on the viewer.
func (s *ScanService) shouldScanNow(now, lastScan time.Time, interval time.Duration) bool {
	if s.ingestActivePlayback == nil || s.ingestActivePlayback() == 0 {
		return true
	}
	if lastScan.IsZero() {
		return true
	}
	// Never scan MORE often than the operator asked for.
	backoff := max(playbackScanBackoff, interval)
	return now.Sub(lastScan) >= backoff
}
