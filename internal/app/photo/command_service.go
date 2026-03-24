package photo

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

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/photo"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/pkg/filehash"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

type CommandService struct {
	repo    photo.Repository
	bus     shared.EventBus
	libRepo library.LibraryQueryRepo
}

func NewCommandService(repo photo.Repository, bus shared.EventBus, libRepo library.LibraryQueryRepo) *CommandService {
	return &CommandService{repo: repo, bus: bus, libRepo: libRepo}
}

func (s *CommandService) ScanDirectory(ctx context.Context, libraryID, dir string) (int, error) {
	if s.libRepo != nil {
		if _, err := s.libRepo.GetByID(ctx, libraryID); err != nil {
			return 0, fmt.Errorf("library %s not found: %w", libraryID, err)
		}
	}

	existing, err := s.repo.ListAllByLibrary(ctx, libraryID)
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
		info    os.FileInfo
		mime    string
		dir     string
		hash    string
		modTime time.Time
	}
	var toProcess []photoJob

	walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkE error) error {
		if walkE != nil {
			slog.Warn("photo walk error", slog.String("path", path), slog.Any("err", walkE))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ext := strings.ToLower(filepath.Ext(path))
		mime, ok := imageExts[ext]
		if !ok {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			slog.Warn("photo stat error", slog.String("path", path), slog.Any("err", infoErr))
			return nil
		}

		hash, hashErr := filehash.Compute(path)
		if hashErr != nil {
			slog.Warn("photo hash error", slog.String("path", path), slog.Any("err", hashErr))
		}

		if ex := byPath[path]; ex != nil {
			if ex.FileSize == info.Size() && ex.ModTime.Equal(info.ModTime()) && ex.FileHash == hash {
				return nil
			}
		}

		if byPath[path] == nil && hash != "" {
			if moved := byHash[hash]; moved != nil {
				if updateErr := s.repo.UpdateFilePath(ctx, moved.ID, path, info.ModTime()); updateErr != nil {
					slog.Warn("photo update path", slog.String("id", moved.ID), slog.Any("err", updateErr))
				}
				return nil
			}
		}

		toProcess = append(toProcess, photoJob{path: path, info: info, mime: mime, dir: dir, hash: hash, modTime: info.ModTime()})
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
				if raw, readErr := os.ReadFile(j.path); readErr == nil {
					takenAt, cameraModel = readExifData(raw)
				}
			}

			title := strings.TrimSuffix(j.info.Name(), filepath.Ext(j.info.Name()))
			album := filepath.Base(filepath.Dir(j.path))
			if album == "." || album == j.dir {
				album = ""
			}

			width, height := imageDimensions(j.path)

			now := time.Now()
			p := photo.Photo{
				ID:          id.New("photo-"),
				LibraryID:   libraryID,
				Title:       title,
				FilePath:    j.path,
				FileSize:    j.info.Size(),
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
			if pubErr := s.bus.Publish(ctx, photo.PhotoAdded{LibraryID: libraryID, PhotoID: p.ID, Path: j.path, Title: title, Album: album, OccurredAt: time.Now()}); pubErr != nil {
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
		return err
	}
	libraryID := ""
	if p != nil {
		libraryID = p.LibraryID
	}
	if err := s.repo.Delete(ctx, photoID); err != nil {
		return err
	}
	if pubErr := s.bus.Publish(ctx, photo.PhotoRemoved{PhotoID: photoID, LibraryID: libraryID, OccurredAt: time.Now()}); pubErr != nil {
		slog.Warn("photo: PhotoRemoved publish failed", slog.String("photoID", photoID), slog.Any("err", pubErr))
	}
	return nil
}
