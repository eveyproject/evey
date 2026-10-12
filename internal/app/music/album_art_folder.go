package music

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/eveyproject/evey/internal/domain/shared"
)

// folderCoverNames are the files a music folder conventionally keeps its album
// art in (what rippers, Jellyfin and Plex write), tried in this order.
var folderCoverNames = []string{
	"cover.jpg", "Cover.jpg", "folder.jpg", "Folder.jpg", "front.jpg", "Front.jpg",
	"cover.jpeg", "folder.jpeg", "front.jpeg",
}

// maxFolderCoverBytes bounds the read of a folder image: a cover is a few
// hundred KB, anything far larger is not one.
const maxFolderCoverBytes = 20 << 20

// localMarkerPath sits next to a cached image copied from the album folder and
// records the source name and size, so an unchanged cover is not read again.
func localMarkerPath(imagePath string) string { return imagePath + ".local" }

// useFolderCover copies the album folder's own cover image into the album art
// cache and points the folder's tracks at it. found is true when the folder
// has one: an image the owner put next to the music beats any external guess,
// so the caller then skips the fetcher. copied is false when the cache already
// held that exact image.
func (s *CommandService) useFolderCover(ctx context.Context, libraryID, albumFolder, hash, dst string) (found, copied bool) {
	if s.scanner == nil {
		return false, false
	}
	for _, name := range folderCoverNames {
		src := filepath.Join(albumFolder, name)
		size, ok := s.folderCoverSize(src)
		if !ok {
			continue
		}
		stamp := name + "|" + strconv.FormatInt(size, 10)
		if prev, err := os.ReadFile(localMarkerPath(dst)); err == nil && string(prev) == stamp {
			if _, err := os.Stat(dst); err == nil {
				return true, false
			}
		}
		f, err := s.scanner.OpenFile(src)
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(f, maxFolderCoverBytes))
		_ = f.Close()
		if err != nil || !isJPEG(data) {
			continue
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			slog.WarnContext(ctx, "music: write folder cover", slog.String("path", dst), slog.Any("err", err))
			return false, false
		}
		if err := os.WriteFile(localMarkerPath(dst), []byte(stamp), 0o644); err != nil {
			slog.WarnContext(ctx, "music: write folder cover marker", slog.String("path", dst), slog.Any("err", err))
		}
		url := "/api/v1/music/album-art/" + hash
		if err := s.tracks.SetAlbumCoverArtURL(ctx, shared.LibraryID(libraryID), albumFolder, url); err != nil {
			slog.WarnContext(ctx, "music: set folder cover url", slog.String("album_folder", albumFolder), slog.Any("err", err))
			return true, false
		}
		slog.InfoContext(ctx, "music: album art taken from the album folder", slog.String("file", src))
		return true, true
	}
	return false, false
}

// folderCoverSize opens a candidate, checks it starts as a JPEG and returns its
// size without reading the rest.
func (s *CommandService) folderCoverSize(path string) (int64, bool) {
	f, err := s.scanner.OpenFile(path)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	head := make([]byte, 3)
	if _, rerr := io.ReadFull(f, head); rerr != nil || !isJPEG(head) {
		return 0, false
	}
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil || size > maxFolderCoverBytes {
		return 0, false
	}
	return size, true
}

// isJPEG reads the SOI marker: the album art route serves image/jpeg.
func isJPEG(b []byte) bool { return bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}) }
