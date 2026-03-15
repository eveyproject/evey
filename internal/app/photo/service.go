package photo

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
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

var imageExts = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".heic": "image/heic",
	".webp": "image/webp",
	".gif":  "image/gif",
	".tiff": "image/tiff",
	".tif":  "image/tiff",
	".bmp":  "image/bmp",
}

const scanWorkers = 8

type Service struct {
	repo    photo.Repository
	bus     shared.EventBus
	libRepo library.LibraryQueryRepo
}

func NewService(repo photo.Repository, bus shared.EventBus, libRepo library.LibraryQueryRepo) *Service {
	return &Service{repo: repo, bus: bus, libRepo: libRepo}
}

type scanJob struct {
	path  string
	info  os.FileInfo
	mime  string
	dir   string
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
			stripExifIfJPEG(j.path)

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
			if pubErr := s.bus.Publish(ctx, photo.PhotoImported{LibraryID: libraryID, PhotoID: p.ID, Path: j.path, OccurredAt: time.Now()}); pubErr != nil {
				slog.Warn("photo scan: event publish failed", slog.String("path", j.path), slog.Any("err", pubErr))
			}
			total.Add(1)
		}(j)
	}
	wg.Wait()
	return int(total.Load()), walkErr
}

// stripExifIfJPEG removes EXIF (APP1) segments from JPEG files in-place.
// Silently skips non-JPEG or files that can't be processed.
func stripExifIfJPEG(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".jpg" && ext != ".jpeg" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 4 {
		return
	}
	// Check JPEG magic bytes
	if data[0] != 0xFF || data[1] != 0xD8 {
		return
	}
	stripped := stripJPEGApp1(data)
	if stripped == nil {
		return
	}
	if err := os.WriteFile(path, stripped, 0); err != nil {
		slog.Warn("exif strip write failed", slog.String("path", path), slog.Any("err", err))
	}
}

// stripJPEGApp1 returns JPEG bytes with all APP1 (0xFFE1) segments removed.
// Returns nil if no APP1 segments were found (no change needed).
func stripJPEGApp1(data []byte) []byte {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil
	}
	var out bytes.Buffer
	out.Write(data[:2]) // SOI marker
	i := 2
	found := false
	for i < len(data)-1 {
		if data[i] != 0xFF {
			// Not a marker; append rest as-is (image data)
			out.Write(data[i:])
			break
		}
		marker := data[i+1]
		// Markers without length: SOI (0xD8), EOI (0xD9), standalone RST (0xD0-0xD7)
		if marker == 0xD8 || marker == 0xD9 || (marker >= 0xD0 && marker <= 0xD7) {
			out.Write(data[i : i+2])
			i += 2
			continue
		}
		if i+3 >= len(data) {
			out.Write(data[i:])
			break
		}
		segLen := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		end := i + 2 + segLen
		if end > len(data) {
			out.Write(data[i:])
			break
		}
		if marker == 0xE1 { // APP1 — skip
			found = true
		} else {
			out.Write(data[i:end])
		}
		i = end
	}
	if !found {
		return nil
	}
	return out.Bytes()
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
	if pubErr := s.bus.Publish(ctx, photo.PhotoDeleted{PhotoID: photoID, LibraryID: libraryID, OccurredAt: time.Now()}); pubErr != nil {
		slog.Warn("photo: PhotoDeleted publish failed", slog.String("photoID", photoID), slog.Any("err", pubErr))
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

func imageDimensions(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}
