package music

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dhowden/tag"
	"github.com/google/uuid"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/music"
	"github.com/evey-project/evey/internal/domain/shared"
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

// AlbumArtDir returns the configured album art directory (empty if not set).
func (s *CommandService) AlbumArtDir() string { return s.albumArtDir }

func albumArtHash(artist, album string) string {
	sum := sha256.Sum256([]byte(artist + "|" + album))
	return hex.EncodeToString(sum[:])
}

// FetchMissingAlbumArt queries albums without cover art in libraryID and attempts
// to fetch artwork via the configured AlbumArtFetcher, persists to disk, and
// updates track rows. Returns the number of albums successfully updated.
func (s *CommandService) FetchMissingAlbumArt(ctx context.Context, libraryID string) (int, error) {
	if s.albumArtFetcher == nil || s.albumArtDir == "" {
		return 0, nil
	}
	if err := os.MkdirAll(s.albumArtDir, 0o755); err != nil {
		return 0, fmt.Errorf("music: mkdir album art dir: %w", err)
	}
	albums, err := s.tracks.ListAlbumsMissingArt(ctx, shared.LibraryID(libraryID))
	if err != nil {
		return 0, fmt.Errorf("music: list albums missing art: %w", err)
	}
	fetched := 0
	for _, ak := range albums {
		if ctx.Err() != nil {
			return fetched, ctx.Err()
		}
		if ak.Artist == "" || ak.Album == "" {
			continue
		}
		data, _, ferr := s.albumArtFetcher.Fetch(ctx, ak.Artist, ak.Album)
		if ferr != nil {
			if !os.IsNotExist(ferr) {
				slog.Warn("music: fetch album art", slog.String("artist", ak.Artist), slog.String("album", ak.Album), slog.Any("err", ferr))
			}
			continue
		}
		hash := albumArtHash(ak.Artist, ak.Album)
		dst := filepath.Join(s.albumArtDir, hash+".jpg")
		if werr := os.WriteFile(dst, data, 0o644); werr != nil {
			slog.Warn("music: write album art", slog.String("path", dst), slog.Any("err", werr))
			continue
		}
		url := "/api/v1/music/album-art/" + hash
		if uerr := s.tracks.SetAlbumCoverArtURL(ctx, shared.LibraryID(libraryID), ak.Album, ak.Artist, url); uerr != nil {
			slog.Warn("music: set cover url", slog.String("album", ak.Album), slog.Any("err", uerr))
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
	if s.libRepo != nil {
		if _, err := s.libRepo.GetByID(ctx, shared.LibraryID(libraryID)); err != nil {
			return 0, fmt.Errorf("library %s not found: %w", libraryID, err)
		}
	}
	byPath, byHash, err := s.loadExistingTracks(ctx, libraryID)
	if err != nil {
		return 0, err
	}
	jobs, walkErr := s.buildTrackJobs(ctx, dir, byPath, byHash)
	total := s.processTrackJobs(ctx, libraryID, jobs)
	s.kickAlbumArtFetch(libraryID)
	return total, walkErr
}

// loadExistingTracks preloads every track for the library keyed by file
// path and (when present) content hash. The hash map drives the
// "moved file, same content" fast path.
func (s *CommandService) loadExistingTracks(ctx context.Context, libraryID string) (map[string]*music.Track, map[string]*music.Track, error) {
	existing, err := s.tracks.ListAllByLibrary(ctx, shared.LibraryID(libraryID))
	if err != nil {
		return nil, nil, fmt.Errorf("list existing tracks: %w", err)
	}
	byPath := make(map[string]*music.Track, len(existing))
	byHash := make(map[string]*music.Track, len(existing))
	for i := range existing {
		t := &existing[i]
		byPath[t.FilePath] = t
		if t.FileHash != "" {
			byHash[t.FileHash] = t
		}
	}
	return byPath, byHash, nil
}

// buildTrackJobs walks the library directory and returns the slice of
// tracks needing a full parse. Unchanged (path+size+mtime) are skipped;
// moved (known hash) are updated in-place and skipped.
func (s *CommandService) buildTrackJobs(ctx context.Context, dir string, byPath, byHash map[string]*music.Track) ([]trackJob, error) {
	var jobs []trackJob
	walkErr := s.scanner.WalkAudioFiles(ctx, dir, func(f music.ScannedAudioFile) error {
		path := f.Path
		// Fast dedup: same path+size+modtime = unchanged. Truncate to
		// microseconds — Postgres timestamp precision is µs vs ns from the
		// filesystem; raw .Equal() falsely flags every file each scan.
		if ex := byPath[path]; ex != nil {
			if ex.FileSize == f.Size &&
				ex.ModTime.Truncate(time.Microsecond).Equal(f.ModTime.Truncate(time.Microsecond)) {
				return nil
			}
		}
		hash, hashErr := s.scanner.ComputeHash(path)
		if hashErr != nil {
			slog.Warn("music scan: hash failed", slog.String("path", path), slog.Any("err", hashErr))
		}
		if byPath[path] == nil && hash != "" {
			if moved := byHash[hash]; moved != nil {
				if updateErr := s.tracks.UpdateFilePath(ctx, moved.ID, path, f.ModTime); updateErr != nil {
					slog.Warn("music scan: update path", slog.String("id", moved.ID), slog.Any("err", updateErr))
				}
				return nil
			}
		}
		jobs = append(jobs, trackJob{path: path, size: f.Size, modTime: f.ModTime, mimeType: f.MimeType, hash: hash})
		return nil
	})
	return jobs, walkErr
}

// processTrackJobs fans track jobs out across NumCPU workers. Each
// worker parses tags (with fallback), upserts, and publishes Track/
// Album events. Panic recovery per-goroutine keeps one bad file from
// killing the rest.
func (s *CommandService) processTrackJobs(ctx context.Context, libraryID string, jobs []trackJob) int {
	var total atomic.Int64
	sem := make(chan struct{}, runtime.NumCPU())
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

	if upsertErr := s.tracks.Upsert(ctx, *t); upsertErr != nil {
		slog.Warn("music scan: upsert failed", slog.String("path", j.path), slog.Any("err", upsertErr))
		return false
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
	t, err := s.tracks.GetByID(ctx, trackID)
	if err != nil {
		return fmt.Errorf("CommandService.DeleteTrack: %w", err)
	}
	var libraryID shared.LibraryID
	if t != nil {
		libraryID = t.LibraryID
	}
	if err := s.tracks.Delete(ctx, trackID); err != nil {
		return fmt.Errorf("CommandService.DeleteTrack: %w", err)
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
				slog.Error("panic recovered in music parseTrack", "error", r, "stack", debug.Stack())
			}
		}()
		t, err := s.parseTrack(path, libraryID, mimeType, fileSize)
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

func (s *CommandService) parseTrack(path, libraryID, mimeType string, fileSize int64) (*music.Track, error) {
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
	if s.prober != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if pr, perr := s.prober.ProbeStreams(ctx, path); perr == nil && pr != nil {
			t.Duration = float64(pr.DurationMs) / 1000.0
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
