package photo

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/photo"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

// readExifData extracts TakenAt and CameraModel from JPEG EXIF (APP1) bytes.
// Returns nil for takenAt if not found or not parseable. Silently ignores errors.
func readExifData(data []byte) (takenAt *time.Time, cameraModel string) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, ""
	}
	i := 2
	for i < len(data)-3 {
		if data[i] != 0xFF {
			break
		}
		marker := data[i+1]
		if marker == 0xD8 || marker == 0xD9 || (marker >= 0xD0 && marker <= 0xD7) {
			i += 2
			continue
		}
		if i+3 >= len(data) {
			break
		}
		segLen := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		end := i + 2 + segLen
		if end > len(data) {
			break
		}
		if marker == 0xE1 {
			seg := data[i+4 : end]
			takenAt, cameraModel = parseExifSegment(seg)
			return takenAt, cameraModel
		}
		i = end
	}
	return nil, ""
}

// parseExifSegment parses a raw APP1 EXIF segment (after the 0xFFE1 marker and length).
// Expected layout: "Exif\0\0" + TIFF header + IFD0.
func parseExifSegment(seg []byte) (takenAt *time.Time, cameraModel string) {
	if len(seg) < 8 || string(seg[:6]) != "Exif\x00\x00" {
		return nil, ""
	}
	tiff := seg[6:]
	if len(tiff) < 8 {
		return nil, ""
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return nil, ""
	}
	ifdOffset := int(order.Uint32(tiff[4:8]))
	if ifdOffset+2 > len(tiff) {
		return nil, ""
	}
	numEntries := int(order.Uint16(tiff[ifdOffset : ifdOffset+2]))
	pos := ifdOffset + 2
	const entrySize = 12
	for range numEntries {
		if pos+entrySize > len(tiff) {
			break
		}
		entry := tiff[pos : pos+entrySize]
		tag := order.Uint16(entry[0:2])
		dataType := order.Uint16(entry[2:4])
		count := int(order.Uint32(entry[4:8]))
		// Tag 0x0110 = Model, 0x9003 = DateTimeOriginal, 0x0132 = DateTime
		switch tag {
		case 0x0110: // Model
			cameraModel = readExifString(tiff, entry[8:12], dataType, count, order)
		case 0x9003, 0x0132: // DateTimeOriginal or DateTime
			if takenAt == nil {
				s := readExifString(tiff, entry[8:12], dataType, count, order)
				if t, err := time.Parse("2006:01:02 15:04:05", s); err == nil {
					takenAt = &t
				}
			}
		}
		pos += entrySize
	}
	return takenAt, cameraModel
}

// readExifString reads an ASCII string EXIF value. For short values (<=4 bytes) they're
// stored inline in the value offset field; otherwise they're at the offset into tiff.
func readExifString(tiff, valueField []byte, dataType uint16, count int, order binary.ByteOrder) string {
	if dataType != 2 || count <= 0 {
		return ""
	}
	var raw []byte
	if count <= 4 {
		raw = valueField[:count]
	} else {
		offset := int(order.Uint32(valueField))
		end := offset + count
		if offset < 0 || end > len(tiff) {
			return ""
		}
		raw = tiff[offset:end]
	}
	s := strings.TrimRight(string(raw), "\x00")
	return strings.TrimSpace(s)
}

type Service struct {
	repo    photo.Repository
	bus     shared.EventBus
	libRepo library.LibraryQueryRepo
	scanner photo.PhotoScanner
}

func NewService(repo photo.Repository, bus shared.EventBus, libRepo library.LibraryQueryRepo, scanner photo.PhotoScanner) *Service {
	return &Service{repo: repo, bus: bus, libRepo: libRepo, scanner: scanner}
}

func (s *Service) ScanDirectory(ctx context.Context, libraryID, dir string) (int, error) {
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
		size    int64
		mime    string
		dir     string
		hash    string
		modTime time.Time
	}
	var toProcess []photoJob

	walkErr := s.scanner.WalkImageFiles(ctx, dir, func(f photo.ScannedImageFile) error {
		hash, hashErr := s.scanner.ComputeHash(f.Path)
		if hashErr != nil {
			slog.Warn("photo hash error", slog.String("path", f.Path), slog.Any("err", hashErr))
		}

		if ex := byPath[f.Path]; ex != nil {
			if ex.FileSize == f.Size && ex.ModTime.Equal(f.ModTime) && ex.FileHash == hash {
				return nil
			}
		}

		if byPath[f.Path] == nil && hash != "" {
			if moved := byHash[hash]; moved != nil {
				if updateErr := s.repo.UpdateFilePath(ctx, moved.ID, f.Path, f.ModTime); updateErr != nil {
					slog.Warn("photo update path", slog.String("id", moved.ID), slog.Any("err", updateErr))
				}
				return nil
			}
		}

		toProcess = append(toProcess, photoJob{path: f.Path, size: f.Size, mime: f.MimeType, dir: dir, hash: hash, modTime: f.ModTime})
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

			name := filepath.Base(j.path)
			title := strings.TrimSuffix(name, filepath.Ext(name))
			album := filepath.Base(filepath.Dir(j.path))
			if album == "." || album == j.dir {
				album = ""
			}

			width, height := s.scanner.ImageDimensions(j.path)

			now := time.Now()
			p := photo.Photo{
				ID:          id.New("photo-"),
				LibraryID:   libraryID,
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
			if pubErr := s.bus.Publish(ctx, photo.PhotoAdded{LibraryID: libraryID, PhotoID: p.ID, Path: j.path, Title: title, Album: album, OccurredAt: time.Now()}); pubErr != nil {
				slog.Warn("photo scan: PhotoAdded publish failed", slog.String("path", j.path), slog.Any("err", pubErr))
			}
			total.Add(1)
		}(j)
	}
	wg.Wait()
	return int(total.Load()), walkErr
}


func (s *Service) DeletePhoto(ctx context.Context, photoID string) error {
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

func (s *Service) ListPhotos(ctx context.Context, libraryID string, limit, offset int) ([]photo.Photo, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	return s.repo.ListByLibrary(ctx, libraryID, limit, offset)
}

func (s *Service) GetAlbums(ctx context.Context, libraryID string) ([]photo.Album, error) {
	return s.repo.GetAlbums(ctx, libraryID)
}

func (s *Service) GetPhoto(ctx context.Context, id string) (*photo.Photo, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) Search(ctx context.Context, libraryID, query string, limit int) ([]photo.Photo, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.Search(ctx, libraryID, query, limit)
}
