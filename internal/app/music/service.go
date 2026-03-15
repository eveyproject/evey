package music

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dhowden/tag"
	"github.com/google/uuid"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/music"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/pkg/filehash"
)

var audioExts = map[string]string{
	".mp3":  "audio/mpeg",
	".flac": "audio/flac",
	".m4a":  "audio/mp4",
	".ogg":  "audio/ogg",
	".opus": "audio/ogg",
	".wav":  "audio/wav",
	".aac":  "audio/aac",
}

type Service struct {
	tracks  music.TrackRepository
	bus     shared.EventBus
	libRepo library.LibraryQueryRepo
}

func NewService(tracks music.TrackRepository, bus shared.EventBus, libRepo library.LibraryQueryRepo) *Service {
	return &Service{tracks: tracks, bus: bus, libRepo: libRepo}
}

func (s *Service) ScanDirectory(ctx context.Context, libraryID, dir string) (int, error) {
	if s.libRepo != nil {
		if _, err := s.libRepo.GetByID(ctx, libraryID); err != nil {
			return 0, fmt.Errorf("library %s not found: %w", libraryID, err)
		}
	}

	existing, err := s.tracks.ListAllByLibrary(ctx, libraryID)
	if err != nil {
		return 0, fmt.Errorf("list existing tracks: %w", err)
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

	type trackJob struct {
		path     string
		size     int64
		modTime  time.Time
		mimeType string
		hash     string
	}
	var toProcess []trackJob

	walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkE error) error {
		if walkE != nil {
			slog.Warn("music scan: walk error", slog.String("path", path), slog.Any("err", walkE))
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		mimeType, ok := audioExts[ext]
		if !ok {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			slog.Warn("music scan: stat failed", slog.String("path", path), slog.Any("err", statErr))
			return nil
		}

		hash, hashErr := filehash.Compute(path)
		if hashErr != nil {
			slog.Warn("music scan: hash failed", slog.String("path", path), slog.Any("err", hashErr))
		}

		if ex := byPath[path]; ex != nil {
			if ex.FileSize == info.Size() && ex.ModTime.Equal(info.ModTime()) && ex.FileHash == hash {
				return nil
			}
		}

		if byPath[path] == nil && hash != "" {
			if moved := byHash[hash]; moved != nil {
				if updateErr := s.tracks.UpdateFilePath(ctx, moved.ID, path, info.ModTime()); updateErr != nil {
					slog.Warn("music scan: update path", slog.String("id", moved.ID), slog.Any("err", updateErr))
				}
				return nil
			}
		}

		toProcess = append(toProcess, trackJob{path: path, size: info.Size(), modTime: info.ModTime(), mimeType: mimeType, hash: hash})
		return nil
	})

	var total atomic.Int64
	sem := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup
	for _, j := range toProcess {
		wg.Add(1)
		sem <- struct{}{}
		go func(j trackJob) {
			defer wg.Done()
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}

			t, parseErr := parseTrackWithTimeout(ctx, j.path, libraryID, j.mimeType, j.size)
			if parseErr != nil {
				slog.Warn("music scan: parse failed", slog.String("path", j.path), slog.Any("err", parseErr))
				t = &music.Track{
					ID:        uuid.New().String(),
					LibraryID: libraryID,
					Title:     filepath.Base(j.path),
					FilePath:  j.path,
					FileSize:  j.size,
					MimeType:  j.mimeType,
				}
			}
			t.FileHash = j.hash
			t.ModTime = j.modTime

			if upsertErr := s.tracks.Upsert(ctx, *t); upsertErr != nil {
				slog.Warn("music scan: upsert failed", slog.String("path", j.path), slog.Any("err", upsertErr))
				return
			}
			if pubErr := s.bus.Publish(ctx, music.TrackScanned{LibraryID: libraryID, TrackID: t.ID, Title: t.Title, Path: j.path, OccurredAt: time.Now()}); pubErr != nil {
				slog.Warn("music scan: event publish failed", slog.String("path", j.path), slog.Any("err", pubErr))
			}
			total.Add(1)
		}(j)
	}
	wg.Wait()
	return int(total.Load()), walkErr
}

func parseTrackWithTimeout(ctx context.Context, path, libraryID, mimeType string, fileSize int64) (*music.Track, error) {
	tCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	type result struct {
		track *music.Track
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		t, err := parseTrack(path, libraryID, mimeType, fileSize)
		ch <- result{t, err}
	}()
	select {
	case r := <-ch:
		return r.track, r.err
	case <-tCtx.Done():
		slog.WarnContext(ctx, "music tag parsing timeout", slog.String("file", path))
		return nil, tCtx.Err()
	}
}

func parseTrack(path, libraryID, mimeType string, fileSize int64) (*music.Track, error) {
	f, err := os.Open(path)
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

	t := &music.Track{
		ID:          uuid.New().String(),
		LibraryID:   libraryID,
		Title:       m.Title(),
		Artist:      artist,
		Album:       m.Album(),
		AlbumArtist: albumArtist,
		Year:        m.Year(),
		TrackNumber: trackNum,
		DiskNumber:  diskNum,
		Duration:    0, // tag lib doesn't expose duration directly
		FilePath:    path,
		FileSize:    fileSize,
		MimeType:    mimeType,
		Genre:       m.Genre(),
	}
	if m.Picture() != nil {
		t.CoverArtURL = fmt.Sprintf("/api/v1/music/tracks/%s/art", t.ID)
	}
	return t, nil
}

func (s *Service) GetTrackArt(ctx context.Context, trackID string) (data []byte, mimeType string, err error) {
	t, err := s.tracks.GetByID(ctx, trackID)
	if err != nil {
		return nil, "", err
	}
	if t == nil {
		return nil, "", os.ErrNotExist
	}
	f, err := os.Open(t.FilePath)
	if err != nil {
		return nil, "", os.ErrNotExist
	}
	defer f.Close()
	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil, "", os.ErrNotExist
	}
	pic := m.Picture()
	if pic == nil {
		return nil, "", os.ErrNotExist
	}
	mt := pic.MIMEType
	if mt == "" {
		mt = "image/jpeg"
	}
	return pic.Data, mt, nil
}

func (s *Service) ListTracks(ctx context.Context, libraryID string, limit, offset int, search string) ([]music.Track, int, error) {
	return s.tracks.ListByLibrary(ctx, libraryID, limit, offset, search)
}

func (s *Service) GetAlbums(ctx context.Context, libraryID string) ([]music.Album, error) {
	return s.tracks.GetAlbums(ctx, libraryID)
}

func (s *Service) GetArtists(ctx context.Context, libraryID string) ([]music.Artist, error) {
	return s.tracks.GetArtists(ctx, libraryID)
}

func (s *Service) Search(ctx context.Context, query string, limit int) ([]music.Track, error) {
	return s.tracks.Search(ctx, query, limit)
}

func (s *Service) GetTrack(ctx context.Context, trackID string) (*music.Track, error) {
	return s.tracks.GetByID(ctx, trackID)
}

func (s *Service) DeleteTrack(ctx context.Context, trackID string) error {
	t, err := s.tracks.GetByID(ctx, trackID)
	if err != nil {
		return err
	}
	libraryID := ""
	if t != nil {
		libraryID = t.LibraryID
	}
	if err := s.tracks.Delete(ctx, trackID); err != nil {
		return err
	}
	if pubErr := s.bus.Publish(ctx, music.TrackDeleted{TrackID: trackID, LibraryID: libraryID, OccurredAt: time.Now()}); pubErr != nil {
		slog.Warn("music: TrackDeleted publish failed", slog.String("trackID", trackID), slog.Any("err", pubErr))
	}
	return nil
}

func (s *Service) StreamTrack(ctx context.Context, trackID string) (filePath string, mimeType string, err error) {
	t, err := s.tracks.GetByID(ctx, trackID)
	if err != nil {
		return "", "", err
	}
	if t == nil {
		return "", "", os.ErrNotExist
	}
	return t.FilePath, t.MimeType, nil
}
