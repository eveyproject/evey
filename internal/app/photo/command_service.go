package photo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/photo"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/scanindex"
	"github.com/evey-project/evey/internal/pkg/sysinfo"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type CommandService struct {
	repo photo.CommandRepo
	// scanReads is the read subset the scan reconciliation needs; both halves
	// come from the same concrete repository, the split only narrows what each
	// call site can reach (ADR-0009).
	scanReads photo.QueryRepo
	bus       shared.EventBus
	libRepo   library.LibraryQueryRepo
	scanner   photo.PhotoScanner
}

func NewCommandService(repo photo.Repository, bus shared.EventBus, libRepo library.LibraryQueryRepo, scanner photo.PhotoScanner) *CommandService {
	return &CommandService{repo: repo, scanReads: repo, bus: bus, libRepo: libRepo, scanner: scanner}
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

func (s *CommandService) ScanDirectory(ctx context.Context, libraryID, dir string) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.photo.ScanDirectory")
	defer span.End()
	if s.libRepo != nil {
		if _, err := s.libRepo.GetByID(ctx, shared.LibraryID(libraryID)); err != nil {
			return 0, fmt.Errorf("library %s not found: %w", libraryID, err)
		}
	}

	idx, err := s.loadPhotoIndex(ctx, libraryID)
	if err != nil {
		return 0, err
	}

	jobs, walkErr := s.buildPhotoJobs(ctx, dir, idx)
	total := s.processPhotoJobs(ctx, libraryID, jobs)
	return total, walkErr
}

// loadPhotoIndex preloads every photo row for the library into a
// scanindex keyed by file path and (when available) content hash. The
// hash lookup drives the "moved file, same content" fast path so a
// rename doesn't trigger a full re-upsert.
func (s *CommandService) loadPhotoIndex(ctx context.Context, libraryID string) (*scanindex.Index, error) {
	existing, err := s.scanReads.ListAllByLibrary(ctx, shared.LibraryID(libraryID))
	if err != nil {
		return nil, fmt.Errorf("list existing photos: %w", err)
	}
	entries := make([]scanindex.Summary, len(existing))
	for i := range existing {
		p := &existing[i]
		entries[i] = scanindex.Summary{
			ID:      p.ID,
			Path:    p.FilePath,
			Size:    p.FileSize,
			ModTime: p.ModTime,
			Hash:    p.FileHash,
		}
	}
	return scanindex.New(entries), nil
}

// buildPhotoJobs walks the library directory and returns the slice of
// photos that need full processing. Unchanged files (same path+size+
// mtime at µs precision) are skipped; moved files (known hash, new
// path) are updated in-place and also skipped.
func (s *CommandService) buildPhotoJobs(ctx context.Context, dir string, idx *scanindex.Index) ([]photoJob, error) {
	var jobs []photoJob
	walkErr := s.scanner.WalkImageFiles(ctx, dir, func(f photo.ScannedImageFile) error {
		if _, unchanged := idx.Unchanged(f.Path, f.Size, f.ModTime); unchanged {
			return nil
		}

		hash, hashErr := s.scanner.ComputeHash(f.Path)
		if hashErr != nil {
			slog.WarnContext(ctx, "photo hash error", slog.String("path", f.Path), slog.Any("err", hashErr))
		}

		if movedID, isMove := idx.Moved(f.Path, hash); isMove {
			if updateErr := s.repo.UpdateFilePath(ctx, movedID, f.Path, f.ModTime); updateErr != nil {
				slog.WarnContext(ctx, "photo update path", slog.String("id", movedID), slog.Any("err", updateErr))
			}
			return nil
		}

		jobs = append(jobs, photoJob{path: f.Path, size: f.Size, name: filepath.Base(f.Path), mime: f.MimeType, dir: dir, hash: hash, modTime: f.ModTime})
		return nil
	})
	return jobs, walkErr
}

// processPhotoJobs fans the photo jobs out across sysinfo.CPUBudget() workers
// (the cgroup CPU quota when one is set, else runtime.NumCPU()) with
// per-goroutine panic recovery. Each worker reads EXIF when applicable,
// computes album from the relative subdir, upserts the row, and
// publishes a PhotoAdded event.
func (s *CommandService) processPhotoJobs(ctx context.Context, libraryID string, jobs []photoJob) int {
	var total atomic.Int64
	sem := make(chan struct{}, sysinfo.CPUBudget())
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j photoJob) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					slog.ErrorContext(ctx, "photo: scan goroutine panic", slog.Any("recover", r), slog.String("path", j.path))
				}
			}()
			if ctx.Err() != nil {
				return
			}
			if s.processOnePhoto(ctx, libraryID, j) {
				total.Add(1)
			}
		}(j)
	}
	wg.Wait()
	return int(total.Load())
}

// processOnePhoto handles a single job: EXIF, dimensions, album, upsert,
// event publish. Returns true when the row was successfully created and
// the event emitted.
func (s *CommandService) processOnePhoto(ctx context.Context, libraryID string, j photoJob) bool {
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
	if parentDir == j.dir {
		album = ""
	}

	width, height := s.scanner.ImageDimensions(j.path)

	// Compute a blurhash placeholder so the gallery grid renders a coloured
	// preview before the full image streams in. Best-effort: an unsupported
	// codec (e.g. HEIC without lib support) returns an error and we just
	// skip the field - the row still ingests, the gallery falls back to
	// the surface background.
	blurhash, _ := computePhotoBlurhash(j.path) //nolint:errcheck // empty placeholder on failure is acceptable

	p, err := photo.NewPhoto(
		shared.LibraryID(libraryID), j.path, j.size, j.modTime,
		photo.WithTitle(title),
		photo.WithHash(j.hash),
		photo.WithMimeType(j.mime),
		photo.WithDimensions(width, height),
		photo.WithAlbum(album),
		photo.WithTakenAt(takenAt),
		photo.WithCameraModel(cameraModel),
		photo.WithBlurhash(blurhash),
	)
	if err != nil {
		slog.WarnContext(ctx, "photo scan: NewPhoto failed", slog.String("path", j.path), slog.Any("err", err))
		return false
	}

	if upsertErr := s.repo.Upsert(ctx, *p); upsertErr != nil {
		slog.WarnContext(ctx, "photo upsert failed", slog.String("path", j.path), slog.Any("err", upsertErr))
		return false
	}
	if pubErr := s.bus.Publish(ctx, photo.PhotoAdded{LibraryID: shared.LibraryID(libraryID), PhotoID: p.ID, Path: j.path, Title: title, Album: album, OccurredAt: time.Now()}); pubErr != nil {
		slog.WarnContext(ctx, "photo scan: PhotoAdded publish failed", slog.String("path", j.path), slog.Any("err", pubErr))
	}
	return true
}

func (s *CommandService) DeletePhoto(ctx context.Context, photoID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.photo.DeletePhoto")
	defer span.End()
	p, err := s.scanReads.GetByID(ctx, photoID)
	if err != nil && !errors.Is(err, shared.ErrNotFound) {
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
		slog.WarnContext(ctx, "photo: PhotoRemoved publish failed", slog.String("photoID", photoID), slog.Any("err", pubErr))
	}
	return nil
}
