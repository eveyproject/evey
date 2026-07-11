package music

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dhowden/tag"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/music"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/scanindex"
	"github.com/evey-project/evey/internal/pkg/sysinfo"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"github.com/google/uuid"
)

// AlbumArtFetcher fetches album artwork from an external source.
type AlbumArtFetcher interface {
	Fetch(ctx context.Context, artist, album string) (data []byte, mimeType string, err error)
}

type CommandService struct {
	tracks          music.TrackRepository
	bus             shared.EventBus
	libRepo         library.LibraryQueryRepo
	scanner         music.MusicScanner
	prober          library.StreamProber
	albumArtFetcher AlbumArtFetcher
	albumArtDir     string
	searchIndex     music.MusicSearchIndex
}

func NewCommandService(tracks music.TrackRepository, bus shared.EventBus, libRepo library.LibraryQueryRepo, scanner music.MusicScanner) *CommandService {
	return &CommandService{tracks: tracks, bus: bus, libRepo: libRepo, scanner: scanner}
}

func (s *CommandService) WithProber(p library.StreamProber) *CommandService {
	s.prober = p
	return s
}

func (s *CommandService) WithAlbumArtFetcher(f AlbumArtFetcher, dataDir string) *CommandService {
	s.albumArtFetcher = f
	s.albumArtDir = filepath.Join(dataDir, "music_album_art")
	return s
}

// WithSearchIndex wires the full-text music search index. Optional: when nil,
// scans/deletes don't touch any index and search falls back to SQL ILIKE.
func (s *CommandService) WithSearchIndex(idx music.MusicSearchIndex) *CommandService {
	s.searchIndex = idx
	return s
}

// ReindexAllTracks (re)indexes every track of every music library into the
// search index, in batches per library. Used at startup to backfill the index.
// Returns the number of tracks indexed.
func (s *CommandService) ReindexAllTracks(ctx context.Context) (int, error) {
	if s.searchIndex == nil {
		return 0, nil
	}
	return s.reindexTracksInto(ctx, s.searchIndex)
}

// reindexTracksInto rebuilds the given search index from the DB, one batch per
// music library. The target is a PARAMETER so the reconcile path can write to the
// primary adapter directly instead of the resilient wrapper (which swallows
// errors). A per-library batch failure is logged and skipped - a transient miss
// is reconciled on the next pass rather than aborting the whole rebuild.
func (s *CommandService) reindexTracksInto(ctx context.Context, search music.MusicSearchIndex) (int, error) {
	if search == nil || s.libRepo == nil {
		return 0, nil
	}
	libs, err := s.libRepo.List(ctx, library.ListLibrariesFilter{Limit: library.MaxListLibrariesLimit})
	if err != nil {
		return 0, fmt.Errorf("music: list libraries for reindex: %w", err)
	}
	indexed := 0
	for _, lib := range libs {
		if lib.Type != library.Music {
			continue
		}
		if ctx.Err() != nil {
			return indexed, ctx.Err()
		}
		tracks, lerr := s.tracks.ListAllByLibrary(ctx, lib.ID)
		if lerr != nil {
			slog.Warn("music reindex: list tracks", slog.String("library", string(lib.ID)), slog.Any("err", lerr))
			continue
		}
		if ierr := search.IndexTracksBatch(ctx, tracks); ierr != nil {
			slog.Warn("music reindex: batch index", slog.String("library", string(lib.ID)), slog.Any("err", ierr))
			continue
		}
		indexed += len(tracks)
	}
	return indexed, nil
}

// musicDocCounter reports how many documents the music search engine holds.
// Satisfied by the Meilisearch music adapter.
type musicDocCounter interface {
	DocumentCount(ctx context.Context) (int64, error)
}

// ReconcileTracks heals music-search drift. The resilient wrapper swallows index
// writes while the engine is down, so a track added during an outage stays
// unsearchable until something rebuilds the index. This compares the engine's
// document count to the DB track count and rebuilds on divergence - call it at
// boot AND periodically so a mid-session outage self-heals without a restart.
// Same count-based limitation as the media reconcile: drift that nets to zero
// (equal adds and deletes in one outage) is invisible until the next real count
// change or a manual reindex - acceptable for the frequency.
//
// Returns (0, nil) and skips when the engine is UNREACHABLE (DocumentCount
// errors) so a periodic caller doesn't scan the DB against a down engine. The
// rebuild targets `primary` (the raw adapter, not the swallowing wrapper) so a
// failure surfaces instead of falsely clearing the drift.
func (s *CommandService) ReconcileTracks(ctx context.Context, primary music.MusicSearchIndex, docCounter musicDocCounter) (int, error) {
	if primary == nil || docCounter == nil || s.libRepo == nil {
		return 0, nil
	}
	docCount, err := docCounter.DocumentCount(ctx)
	if err != nil {
		//nolint:nilerr // an unreachable engine is a SKIP, not a failure to propagate - retry next tick
		return 0, nil
	}
	dbCount, err := s.countAllTracks(ctx)
	if err != nil {
		return 0, err
	}
	// Rebuild ONLY when the index has FEWER docs than the DB (missing tracks a
	// re-index can add back). When it has MORE (a delete dropped during an outage
	// left a stale doc), a re-index upserts and can never shrink the count, so
	// firing every tick would rebuild forever - a CPU loop. Skip it; clear a stale
	// doc with a manual reindex. Equal counts = in sync.
	if docCount >= int64(dbCount) {
		if docCount > int64(dbCount) {
			slog.Warn("music search reconcile: index has more docs than the DB (stale doc from a dropped delete); a re-index only upserts and can't shrink it - wipe the search index and reindex to clear",
				slog.Int64("index_docs", docCount), slog.Int("db_tracks", dbCount))
		}
		return 0, nil
	}
	slog.Info("music search reconcile: index behind the DB, rebuilding",
		slog.Int64("index_docs", docCount), slog.Int("db_tracks", dbCount))
	return s.reindexTracksInto(ctx, primary)
}

// countAllTracks returns the total track count across all music libraries, using
// the pagination total from a 1-row ListByLibrary so it stays a COUNT, not a full
// load.
func (s *CommandService) countAllTracks(ctx context.Context) (int, error) {
	libs, err := s.libRepo.List(ctx, library.ListLibrariesFilter{Limit: library.MaxListLibrariesLimit})
	if err != nil {
		return 0, fmt.Errorf("music: list libraries for count: %w", err)
	}
	total := 0
	for _, lib := range libs {
		if lib.Type != library.Music {
			continue
		}
		_, n, terr := s.tracks.ListByLibrary(ctx, lib.ID, 1, 0, "")
		if terr != nil {
			return 0, fmt.Errorf("music: count tracks in %s: %w", lib.ID, terr)
		}
		total += n
	}
	return total, nil
}

// AlbumArtDir returns the configured album art directory (empty if not set).
func (s *CommandService) AlbumArtDir() string { return s.albumArtDir }

func albumArtHash(artist, album string) string {
	sum := sha256.Sum256([]byte(artist + "|" + album))
	return hex.EncodeToString(sum[:])
}

// FetchMissingAlbumArt fetches artwork for every album in libraryID whose
// cached image is absent from disk (never fetched, or wiped with the cache),
// via the configured AlbumArtFetcher, persists it, and updates track rows.
// Albums whose image already exists on disk are skipped. Returns the number of
// albums successfully (re)fetched.
func (s *CommandService) FetchMissingAlbumArt(ctx context.Context, libraryID string) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.music.FetchMissingAlbumArt")
	defer span.End()
	if s.albumArtFetcher == nil || s.albumArtDir == "" {
		return 0, nil
	}
	if err := os.MkdirAll(s.albumArtDir, 0o755); err != nil {
		return 0, fmt.Errorf("music: mkdir album art dir: %w", err)
	}
	albums, err := s.tracks.ListAllAlbums(ctx, shared.LibraryID(libraryID))
	if err != nil {
		return 0, fmt.Errorf("music: list albums: %w", err)
	}
	fetched := 0
	for _, ak := range albums {
		if ctx.Err() != nil {
			return fetched, ctx.Err()
		}
		if ak.Artist == "" || ak.Album == "" || ak.AlbumFolder == "" {
			continue
		}
		// Gate on the on-disk file, not the DB cover_art_url: an album whose
		// URL is set but whose cached image was wiped (cache cleared) must be
		// re-fetched, otherwise the poster stays broken forever. When the file
		// is already present we skip the (external, rate-limited) fetch.
		hash := albumArtHash(ak.AlbumFolder, "")
		dst := filepath.Join(s.albumArtDir, hash+".jpg")
		if _, statErr := os.Stat(dst); statErr == nil {
			continue
		}
		data, _, ferr := s.albumArtFetcher.Fetch(ctx, ak.Artist, ak.Album)
		if ferr != nil {
			if !os.IsNotExist(ferr) {
				slog.Warn("music: fetch album art", slog.String("artist", ak.Artist), slog.String("album", ak.Album), slog.Any("err", ferr))
			}
			continue
		}
		// hash/dst computed above (folder is the album identity, so two folders
		// that share (artist, album) tags still get distinct files on disk).
		if werr := os.WriteFile(dst, data, 0o644); werr != nil {
			slog.Warn("music: write album art", slog.String("path", dst), slog.Any("err", werr))
			continue
		}
		url := "/api/v1/music/album-art/" + hash
		if uerr := s.tracks.SetAlbumCoverArtURL(ctx, shared.LibraryID(libraryID), ak.AlbumFolder, url); uerr != nil {
			slog.Warn("music: set cover url", slog.String("album_folder", ak.AlbumFolder), slog.Any("err", uerr))
			continue
		}
		fetched++
	}
	return fetched, nil
}

type trackJob struct {
	path     string
	size     int64
	modTime  time.Time
	mimeType string
	hash     string
}

func (s *CommandService) ScanDirectory(ctx context.Context, libraryID, dir string) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.music.ScanDirectory")
	defer span.End()
	if s.libRepo != nil {
		if _, err := s.libRepo.GetByID(ctx, shared.LibraryID(libraryID)); err != nil {
			return 0, fmt.Errorf("library %s not found: %w", libraryID, err)
		}
	}
	idx, err := s.loadTrackIndex(ctx, libraryID)
	if err != nil {
		return 0, err
	}
	jobs, walkErr := s.buildTrackJobs(ctx, dir, idx)
	total := s.processTrackJobs(ctx, libraryID, jobs)
	s.kickAlbumArtFetch(libraryID)
	return total, walkErr
}

// loadTrackIndex preloads every track for the library into a scanindex
// keyed by file path and (when present) content hash. The hash lookup
// drives the "moved file, same content" fast path.
func (s *CommandService) loadTrackIndex(ctx context.Context, libraryID string) (*scanindex.Index, error) {
	existing, err := s.tracks.ListAllByLibrary(ctx, shared.LibraryID(libraryID))
	if err != nil {
		return nil, fmt.Errorf("list existing tracks: %w", err)
	}
	entries := make([]scanindex.Summary, len(existing))
	for i := range existing {
		t := &existing[i]
		entries[i] = scanindex.Summary{
			ID:      t.ID,
			Path:    t.FilePath,
			Size:    t.FileSize,
			ModTime: t.ModTime,
			Hash:    t.FileHash,
		}
	}
	return scanindex.New(entries), nil
}

// buildTrackJobs walks the library directory and returns the slice of
// tracks needing a full parse. Unchanged (path+size+mtime) are skipped;
// moved (known hash) are updated in-place and skipped.
func (s *CommandService) buildTrackJobs(ctx context.Context, dir string, idx *scanindex.Index) ([]trackJob, error) {
	var jobs []trackJob
	walkErr := s.scanner.WalkAudioFiles(ctx, dir, func(f music.ScannedAudioFile) error {
		if _, unchanged := idx.Unchanged(f.Path, f.Size, f.ModTime); unchanged {
			return nil
		}
		hash, hashErr := s.scanner.ComputeHash(f.Path)
		if hashErr != nil {
			slog.Warn("music scan: hash failed", slog.String("path", f.Path), slog.Any("err", hashErr))
		}
		if movedID, isMove := idx.Moved(f.Path, hash); isMove {
			if updateErr := s.tracks.UpdateFilePath(ctx, movedID, f.Path, f.ModTime); updateErr != nil {
				slog.Warn("music scan: update path", slog.String("id", movedID), slog.Any("err", updateErr))
			}
			return nil
		}
		jobs = append(jobs, trackJob{path: f.Path, size: f.Size, modTime: f.ModTime, mimeType: f.MimeType, hash: hash})
		return nil
	})
	return jobs, walkErr
}

// processTrackJobs fans track jobs out across sysinfo.CPUBudget() workers
// (the cgroup CPU quota when one is set, else runtime.NumCPU()). Each
// worker parses tags (with fallback), upserts, and publishes Track/
// Album events. Panic recovery per-goroutine keeps one bad file from
// killing the rest.
func (s *CommandService) processTrackJobs(ctx context.Context, libraryID string, jobs []trackJob) int {
	var total atomic.Int64
	sem := make(chan struct{}, sysinfo.CPUBudget())
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j trackJob) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("music: scan goroutine panic", slog.Any("recover", r), slog.String("path", j.path))
				}
			}()
			if ctx.Err() != nil {
				return
			}
			if s.processOneTrack(ctx, libraryID, j) {
				total.Add(1)
			}
		}(j)
	}
	wg.Wait()
	return int(total.Load())
}

// processOneTrack parses, upserts, and publishes events for a single
// track job. Falls back to a minimal Track when full tag parsing fails
// so the file still gets indexed.
func (s *CommandService) processOneTrack(ctx context.Context, libraryID string, j trackJob) bool {
	t, parseErr := s.parseTrackWithTimeout(ctx, j.path, libraryID, j.mimeType, j.size)
	if parseErr != nil {
		slog.Warn("music scan: parse failed", slog.String("path", j.path), slog.Any("err", parseErr))
		t, parseErr = music.NewTrack(uuid.New().String(), shared.LibraryID(libraryID), filepath.Base(j.path), j.path,
			music.WithTrackFile(j.size, j.mimeType),
		)
		if parseErr != nil {
			slog.Warn("music scan: fallback track build failed", slog.String("path", j.path), slog.Any("err", parseErr))
			return false
		}
	}
	t.FileHash = j.hash
	t.ModTime = j.modTime
	// Defensive: parseTrack already sets this, but the fallback path
	// above constructs a Track without calling parseTrack.
	if t.AlbumFolder == "" {
		t.AlbumFolder = filepath.Dir(j.path)
	}

	persistedID, upsertErr := s.tracks.Upsert(ctx, *t)
	if upsertErr != nil {
		slog.Warn("music scan: upsert failed", slog.String("path", j.path), slog.Any("err", upsertErr))
		return false
	}
	// On a re-scan the ON CONFLICT path keeps the existing row's id, so the
	// locally-minted t.ID was never persisted. Publish the persisted id so
	// subscribers reference a row that actually exists.
	t.ID = persistedID
	// Best-effort: keep the search index in sync with the row we just wrote.
	// A failure here must never fail the scan (the SQL fallback still serves
	// search), so log and continue.
	if s.searchIndex != nil {
		if idxErr := s.searchIndex.IndexTrack(ctx, *t); idxErr != nil {
			slog.Warn("music scan: search index failed", slog.String("path", j.path), slog.Any("err", idxErr))
		}
	}
	if pubErr := s.bus.Publish(ctx, music.TrackScanned{LibraryID: shared.LibraryID(libraryID), TrackID: t.ID, Title: t.Title, Path: j.path, OccurredAt: time.Now()}); pubErr != nil {
		slog.Warn("music scan: event publish failed", slog.String("path", j.path), slog.Any("err", pubErr))
	}
	if pubErr := s.bus.Publish(ctx, music.TrackAdded{LibraryID: shared.LibraryID(libraryID), TrackID: t.ID, Title: t.Title, Artist: t.Artist, OccurredAt: time.Now()}); pubErr != nil {
		slog.Warn("music scan: TrackAdded publish failed", slog.String("path", j.path), slog.Any("err", pubErr))
	}
	if t.Album != "" {
		if pubErr := s.bus.Publish(ctx, music.AlbumCreated{LibraryID: shared.LibraryID(libraryID), AlbumName: t.Album, Artist: t.Artist, OccurredAt: time.Now()}); pubErr != nil {
			slog.Warn("music scan: AlbumCreated publish failed", slog.String("album", t.Album), slog.Any("err", pubErr))
		}
	}
	return true
}

// kickAlbumArtFetch starts a background goroutine to fetch any missing
// album art for the library. No-op when the fetcher or storage dir
// aren't wired.
func (s *CommandService) kickAlbumArtFetch(libraryID string) {
	if s.albumArtFetcher == nil || s.albumArtDir == "" {
		return
	}
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("music: fetch missing album art panic", slog.Any("recover", rec))
			}
		}()
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		n, err := s.FetchMissingAlbumArt(bgCtx, libraryID)
		if err != nil {
			slog.Warn("music: fetch missing album art failed", slog.Any("err", err))
			return
		}
		if n > 0 {
			slog.Info("music: fetched missing album arts", slog.Int("count", n), slog.String("library", libraryID))
		}
	}()
}

// FetchMissingAlbumArtAllLibraries iterates music libraries and fetches missing art.
func (s *CommandService) FetchMissingAlbumArtAllLibraries(ctx context.Context) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.music.FetchMissingAlbumArtAllLibraries")
	defer span.End()
	if s.libRepo == nil {
		return 0, nil
	}
	libs, err := s.libRepo.List(ctx, library.ListLibrariesFilter{})
	if err != nil {
		return 0, fmt.Errorf("list libraries: %w", err)
	}
	total := 0
	for _, lib := range libs {
		if lib.Type != library.Music {
			continue
		}
		n, err := s.FetchMissingAlbumArt(ctx, string(lib.ID))
		if err != nil {
			slog.Warn("music: fetch missing album art for library", slog.String("lib", string(lib.ID)), slog.Any("err", err))
			continue
		}
		total += n
	}
	return total, nil
}

func (s *CommandService) DeleteTrack(ctx context.Context, trackID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.music.DeleteTrack")
	defer span.End()
	t, err := s.tracks.GetByID(ctx, trackID)
	if err != nil && !errors.Is(err, shared.ErrNotFound) {
		return fmt.Errorf("CommandService.DeleteTrack: %w", err)
	}
	var libraryID shared.LibraryID
	if t != nil {
		libraryID = t.LibraryID
	}
	if err := s.tracks.Delete(ctx, trackID); err != nil {
		return fmt.Errorf("CommandService.DeleteTrack: %w", err)
	}
	if s.searchIndex != nil {
		if idxErr := s.searchIndex.RemoveTrack(ctx, trackID); idxErr != nil {
			slog.Warn("music: search index remove failed", slog.String("trackID", trackID), slog.Any("err", idxErr))
		}
	}
	if pubErr := s.bus.Publish(ctx, music.TrackDeleted{TrackID: trackID, LibraryID: libraryID, OccurredAt: time.Now()}); pubErr != nil {
		slog.Warn("music: TrackDeleted publish failed", slog.String("trackID", trackID), slog.Any("err", pubErr))
	}
	if pubErr := s.bus.Publish(ctx, music.TrackRemoved{TrackID: trackID, LibraryID: libraryID, OccurredAt: time.Now()}); pubErr != nil {
		slog.Warn("music: TrackRemoved publish failed", slog.String("trackID", trackID), slog.Any("err", pubErr))
	}
	return nil
}

func (s *CommandService) parseTrackWithTimeout(ctx context.Context, path, libraryID, mimeType string, fileSize int64) (*music.Track, error) {
	tCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	type result struct {
		track *music.Track
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in music parseTrack", "error", r, "stack", string(debug.Stack()))
			}
		}()
		t, err := s.parseTrack(tCtx, path, libraryID, mimeType, fileSize)
		select {
		case ch <- result{t, err}:
		case <-tCtx.Done():
		}
	}()
	select {
	case r := <-ch:
		return r.track, r.err
	case <-tCtx.Done():
		slog.WarnContext(ctx, "music tag parsing timeout", slog.String("file", path))
		return nil, tCtx.Err()
	}
}

func (s *CommandService) parseTrack(ctx context.Context, path, libraryID, mimeType string, fileSize int64) (*music.Track, error) {
	f, err := s.scanner.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil, err
	}

	trackNum, _ := m.Track()
	diskNum, _ := m.Disc()

	artist := m.Artist()
	albumArtist := m.AlbumArtist()
	if albumArtist == "" {
		albumArtist = artist
	}

	title := m.Title()
	if title == "" {
		title = filepath.Base(path)
	}
	trackID := uuid.New().String()
	opts := []music.TrackOption{
		music.WithTrackArtist(artist, albumArtist),
		music.WithTrackAlbum(m.Album()),
		music.WithTrackYear(m.Year()),
		music.WithTrackNumbers(trackNum, diskNum),
		music.WithTrackFile(fileSize, mimeType),
		music.WithTrackGenre(m.Genre()),
	}
	if m.Picture() != nil {
		opts = append(opts, music.WithTrackCoverArt(fmt.Sprintf("/api/v1/music/tracks/%s/art", trackID)))
	}
	t, err := music.NewTrack(trackID, shared.LibraryID(libraryID), title, path, opts...)
	if err != nil {
		return nil, fmt.Errorf("build track: %w", err)
	}
	// Album identity is the on-disk parent directory - see Track.AlbumFolder.
	t.AlbumFolder = filepath.Dir(path)
	if s.prober != nil {
		// Derive from the caller's ctx (carrying the parse-timeout budget and
		// scan cancellation + tracing) rather than context.Background, so an
		// aborted scan stops probing instead of running to the full timeout.
		probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if pr, perr := s.prober.ProbeStreams(probeCtx, path); perr == nil && pr != nil {
			t.Duration = pr.Duration.Seconds()
			for _, st := range pr.Streams {
				if st == nil || st.StreamType != "audio" {
					continue
				}
				if st.Bitrate != nil {
					t.Bitrate = int(*st.Bitrate)
				}
				if st.SampleRate != nil {
					t.SampleRate = *st.SampleRate
				}
				if st.Channels != nil {
					t.Channels = *st.Channels
				}
				break
			}
		}
	}
	return t, nil
}
