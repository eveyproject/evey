package photo

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/photo"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/id"
)

type CommandService struct {
	repo    photo.Repository
	bus     shared.EventBus
	libRepo library.LibraryQueryRepo
	scanner photo.PhotoScanner
}

func NewCommandService(repo photo.Repository, bus shared.EventBus, libRepo library.LibraryQueryRepo, scanner photo.PhotoScanner) *CommandService {
	return &CommandService{repo: repo, bus: bus, libRepo: libRepo, scanner: scanner}
}

func (s *CommandService) ScanDirectory(ctx context.Context, libraryID, dir string) (int, error) {
	if s.libRepo != nil {
		if _, err := s.libRepo.GetByID(ctx, shared.LibraryID(libraryID)); err != nil {
			return 0, fmt.Errorf("library %s not found: %w", libraryID, err)
		}
	}

	existing, err := s.repo.ListAllByLibrary(ctx, shared.LibraryID(libraryID))
	if err != nil {
		return 0, fmt.Errorf("list existing photos: %w", err)
	}
	byPath := make(map[string]*photo.Photo, len(existing))
	byHash := make(map[string]*photo.Photo, len(existing))
	for i := range existing {
		p := &existing[i]
		byPath[p.FilePath] = p
		if p.FileHash != "" {
			byHash[p.FileHash] = p
		}
	}

	type photoJob struct {
		path    string
		size    int64
		name    string
		mime    string
		dir     string
		hash    string
		modTime time.Time
	}
	var toProcess []photoJob

	walkErr := s.scanner.WalkImageFiles(ctx, dir, func(f photo.ScannedImageFile) error {
		// Fast dedup path: same file_path, size, and mod_time means unchanged.
		// Truncate to microseconds — Postgres stores timestamps at µs precision
		// while the filesystem exposes ns, so a raw .Equal() flags EVERY file
		// as changed on every scan, spinning upsert and saturating the DB pool.
		if ex := byPath[f.Path]; ex != nil {
			if ex.FileSize == f.Size &&
				ex.ModTime.Truncate(time.Microsecond).Equal(f.ModTime.Truncate(time.Microsecond)) {
				return nil
			}
		}

		hash, hashErr := s.scanner.ComputeHash(f.Path)
		if hashErr != nil {
			slog.Warn("photo hash error", slog.String("path", f.Path), slog.Any("err", hashErr))
		}

		if byPath[f.Path] == nil && hash != "" {
			if moved := byHash[hash]; moved != nil {
				if updateErr := s.repo.UpdateFilePath(ctx, moved.ID, f.Path, f.ModTime); updateErr != nil {
					slog.Warn("photo update path", slog.String("id", moved.ID), slog.Any("err", updateErr))
				}
				return nil
			}
		}

		toProcess = append(toProcess, photoJob{path: f.Path, size: f.Size, name: filepath.Base(f.Path), mime: f.MimeType, dir: dir, hash: hash, modTime: f.ModTime})
		return nil
	})

	var total atomic.Int64
	sem := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup
	for _, j := range toProcess {
		wg.Add(1)
		sem <- struct{}{}
		go func(j photoJob) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("photo: scan goroutine panic", slog.Any("recover", r), slog.String("path", j.path))
				}
			}()
			if ctx.Err() != nil {
				return
			}

			var takenAt *time.Time
			var cameraModel string
			if j.mime == "image/jpeg" {
				if raw, readErr := s.scanner.ReadFile(j.path); readErr == nil {
					takenAt, cameraModel = readExifData(raw)
				}
			}

			title := strings.TrimSuffix(j.name, filepath.Ext(j.name))
			// Album = subdirectory path relative to the library root, so two
			// physical folders with the same name (e.g. nested "Ma Sorciere…"
			// dirs) don't collapse into one logical album.
			parentDir := filepath.Dir(j.path)
			album := strings.TrimPrefix(parentDir, j.dir)
			album = strings.TrimPrefix(album, string(filepath.Separator))
			if album == "." || parentDir == j.dir {
				album = ""
			}

			width, height := s.scanner.ImageDimensions(j.path)

			now := time.Now()
			p := photo.Photo{
				ID:          id.New("photo-"),
				LibraryID:   shared.LibraryID(libraryID),
				Title:       title,
				FilePath:    j.path,
				FileSize:    j.size,
				FileHash:    j.hash,
				ModTime:     j.modTime,
				MimeType:    j.mime,
				Width:       width,
				Height:      height,
				Album:       album,
				TakenAt:     takenAt,
				CameraModel: cameraModel,
				CreatedAt:   now,
				UpdatedAt:   now,
			}

			if upsertErr := s.repo.Upsert(ctx, p); upsertErr != nil {
				slog.Warn("photo upsert failed", slog.String("path", j.path), slog.Any("err", upsertErr))
				return
			}
			if pubErr := s.bus.Publish(ctx, photo.PhotoAdded{LibraryID: shared.LibraryID(libraryID), PhotoID: p.ID, Path: j.path, Title: title, Album: album, OccurredAt: time.Now()}); pubErr != nil {
				slog.Warn("photo scan: PhotoAdded publish failed", slog.String("path", j.path), slog.Any("err", pubErr))
			}
			total.Add(1)
		}(j)
	}
	wg.Wait()
	return int(total.Load()), walkErr
}

func (s *CommandService) DeletePhoto(ctx context.Context, photoID string) error {
	p, err := s.repo.GetByID(ctx, photoID)
	if err != nil {
		return fmt.Errorf("CommandService.DeletePhoto: %w", err)
	}
	var libraryID shared.LibraryID
	if p != nil {
		libraryID = p.LibraryID
	}
	if err := s.repo.Delete(ctx, photoID); err != nil {
		return fmt.Errorf("CommandService.DeletePhoto: %w", err)
	}
	if pubErr := s.bus.Publish(ctx, photo.PhotoRemoved{PhotoID: photoID, LibraryID: libraryID, OccurredAt: time.Now()}); pubErr != nil {
		slog.Warn("photo: PhotoRemoved publish failed", slog.String("photoID", photoID), slog.Any("err", pubErr))
	}
	return nil
}
