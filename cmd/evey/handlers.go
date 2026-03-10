package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	domainmeta "github.com/gravity-zero/evey/internal/domain/metadata"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
)

func mediaHandler(tokens interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, libSvc interface {
	GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error)
}, artworkRepo interface {
	GetByMediaItem(ctx context.Context, mediaItemID string) ([]*domainmeta.ArtworkRecord, error)
}, baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		artType := r.PathValue("artType")
		var action string
		if artType != "" {
			action = "artwork"
		} else {
			action = "stream"
		}

		switch action {
		case "stream":
			authHeader := r.Header.Get("Authorization")
			if len(authHeader) < 8 {
				if t := r.URL.Query().Get("token"); t != "" {
					authHeader = "Bearer " + t
				}
			}
			if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
				http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
				return
			}
			if _, err := tokens.ValidateAccessToken(authHeader[7:]); err != nil {
				http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
				return
			}
			item, err := libSvc.GetMediaItem(r.Context(), mediaID)
			if err != nil {
				http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
				return
			}
			setCORSHeaders(w, r, baseURL)
			http.ServeFile(w, r, item.FilePath)

		case "artwork":
			records, err := artworkRepo.GetByMediaItem(r.Context(), mediaID)
			if err != nil || len(records) == 0 {
				http.NotFound(w, r)
				return
			}
			var url string
			for _, rec := range records {
				if artType == "" || rec.Type == artType {
					if rec.SourceURL != nil {
						url = *rec.SourceURL
						break
					}
				}
			}
			if url == "" {
				http.NotFound(w, r)
				return
			}
			http.Redirect(w, r, url, http.StatusFound)

		default:
			http.NotFound(w, r)
		}
	}
}

func selectArtworkHandler(tokens interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, artworkRepo interface {
	GetByMediaItem(ctx context.Context, mediaItemID string) ([]*domainmeta.ArtworkRecord, error)
	UpdateSortOrder(ctx context.Context, id string, sortOrder int) error
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		if _, err := tokens.ValidateAccessToken(authHeader[7:]); err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		mediaID := r.PathValue("id")
		artworkID := r.PathValue("artworkId")
		records, err := artworkRepo.GetByMediaItem(r.Context(), mediaID)
		if err != nil || len(records) == 0 {
			http.NotFound(w, r)
			return
		}
		var selected *domainmeta.ArtworkRecord
		for _, rec := range records {
			if rec.ID == artworkID {
				selected = rec
				break
			}
		}
		if selected == nil {
			http.NotFound(w, r)
			return
		}
		for _, rec := range records {
			if rec.Type == selected.Type && rec.ID != selected.ID && rec.SortOrder < selected.SortOrder {
				_ = artworkRepo.UpdateSortOrder(r.Context(), rec.ID, selected.SortOrder)
				break
			}
		}
		_ = artworkRepo.UpdateSortOrder(r.Context(), selected.ID, -1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}
}

func downloadHandler(tokens interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, libSvc interface {
	GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error)
}, baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		token := r.Header.Get("Authorization")
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		} else if t := r.URL.Query().Get("token"); t != "" {
			token = t
		} else {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		if _, err := tokens.ValidateAccessToken(token); err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil || item.FilePath == "" {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		fi, err := os.Stat(item.FilePath)
		if err != nil {
			http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
			return
		}
		filename := sanitizeFilename(filepath.Base(item.FilePath))
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
		setCORSHeaders(w, r, baseURL)
		http.ServeFile(w, r, item.FilePath)
	}
}

func browseDirsHandler(tokens interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}) http.HandlerFunc {
	type entry struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		claims, err := tokens.ValidateAccessToken(authHeader[7:])
		if err != nil || !claims.IsAdmin {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}

		dir := r.URL.Query().Get("path")
		if dir != "" {
			dir = filepath.Clean(dir)
			if !filepath.IsAbs(dir) || !strings.HasPrefix(dir, "/media") {
				http.Error(w, `{"message":"invalid path"}`, http.StatusBadRequest)
				return
			}
		}

		skip := map[string]bool{"cdrom": true, "floppy": true, "usb": true, "sr0": true, "proc": true, "sys": true, "dev": true}
		result := []entry{}

		if dir == "" {
			roots, _ := filepath.Glob("/media*")
			for _, root := range roots {
				info, err := os.Stat(root)
				if err != nil || !info.IsDir() {
					continue
				}
				name := filepath.Base(root)
				result = append(result, entry{Name: name, Path: root})
				entries, err := os.ReadDir(root)
				if err != nil {
					continue
				}
				for _, e := range entries {
					if !e.IsDir() || e.Name()[0] == '.' || skip[e.Name()] {
						continue
					}
					result = append(result, entry{Name: name + "/" + e.Name(), Path: root + "/" + e.Name()})
				}
			}
		} else {
			base := dir
			if len(base) > 1 && base[len(base)-1] == '/' {
				base = base[:len(base)-1]
			}
			dirEntries, err := os.ReadDir(dir)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte("[]"))
				return
			}
			for _, e := range dirEntries {
				if !e.IsDir() || e.Name()[0] == '.' || skip[e.Name()] {
					continue
				}
				result = append(result, entry{Name: e.Name(), Path: base + "/" + e.Name()})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		jsonEncodeResponse(w, result)
	}
}

func transcodeHandler(tokenGen interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, libSvc interface {
	GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error)
	GetMediaStreams(ctx context.Context, mediaItemID string) ([]*domainlibrary.MediaStream, error)
}, baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		token := r.Header.Get("Authorization")
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		} else if t := r.URL.Query().Get("token"); t != "" {
			token = t
		} else {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		if _, err := tokenGen.ValidateAccessToken(token); err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil || item.FilePath == "" {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		audioTrack := 0
		if at := r.URL.Query().Get("audioTrack"); at != "" {
			if n, err2 := strconv.Atoi(at); err2 == nil && n >= 0 {
				audioTrack = n
			}
		}
		videoScale := 0
		if vs := r.URL.Query().Get("videoScale"); vs != "" {
			if n, err2 := strconv.Atoi(vs); err2 == nil && n > 0 {
				videoScale = n
			}
		}
		remuxOnly := r.URL.Query().Get("remux") == "1"
		needsVideoTranscode := false
		needsAudioTranscode := true
		if streams, err := libSvc.GetMediaStreams(r.Context(), mediaID); err == nil {
			for _, s := range streams {
				slog.Debug("transcode: stream", slog.String("media_id", mediaID), slog.String("type", s.StreamType), slog.String("codec", s.Codec), slog.Bool("default", s.IsDefault))
				if s.StreamType == "video" && s.Codec != "h264" && !remuxOnly {
					needsVideoTranscode = true
				}
				if s.StreamType == "audio" && s.IsDefault {
					switch s.Codec {
					case "aac", "mp3", "opus":
						needsAudioTranscode = false
					}
				}
			}
		} else {
			slog.Error("transcode: GetMediaStreams failed", slog.String("media_id", mediaID), slog.Any("err", err))
		}
		slog.Debug("transcode: decision", slog.String("media_id", mediaID), slog.Bool("needsVideoTranscode", needsVideoTranscode), slog.Bool("needsAudioTranscode", needsAudioTranscode), slog.Bool("remuxOnly", remuxOnly))

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		args := []string{
			"-i", item.FilePath,
			"-map", "0:v:0",
			"-map", fmt.Sprintf("0:a:%d", audioTrack),
		}
		if videoScale > 0 {
			args = append(args, "-vf", fmt.Sprintf("scale=-2:%d", videoScale), "-c:v", "libx264", "-crf", "23", "-preset", "fast")
		} else if needsVideoTranscode {
			args = append(args, "-c:v", "libx264", "-crf", "23", "-preset", "veryfast", "-tune", "zerolatency", "-threads", "0")
		} else {
			args = append(args, "-c:v", "copy")
		}
		if needsAudioTranscode {
			args = append(args, "-c:a", "aac", "-ac", "2", "-b:a", "192k")
		} else {
			args = append(args, "-c:a", "copy")
		}
		args = append(args, "-movflags", "frag_keyframe+empty_moov+default_base_moof",
			"-f", "mp4", "-")
		slog.Debug("transcode: ffmpeg", slog.String("media_id", mediaID), slog.String("args", strings.Join(args, " ")))
		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			slog.Error("ffmpeg stdout pipe", slog.Any("err", err))
			http.Error(w, "transcode failed", http.StatusInternalServerError)
			return
		}
		if err := cmd.Start(); err != nil {
			slog.Error("ffmpeg start", slog.Any("err", err))
			http.Error(w, "transcode failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Accept-Ranges", "none")
		setCORSHeaders(w, r, baseURL)
		go func() { <-r.Context().Done(); cancel() }()
		io.Copy(w, stdout)
		if err := cmd.Wait(); err != nil && ctx.Err() == nil {
			slog.Warn("ffmpeg transcode", slog.String("media_id", mediaID), slog.Any("err", err))
		}
	}
}

func subtitleHandler(tokenGen interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, libSvc interface {
	GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error)
	GetMediaStreams(ctx context.Context, mediaItemID string) ([]*domainlibrary.MediaStream, error)
}, baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		trackStr := r.PathValue("track")
		token := r.Header.Get("Authorization")
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		} else if t := r.URL.Query().Get("token"); t != "" {
			token = t
		} else {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		if _, err := tokenGen.ValidateAccessToken(token); err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		trackIndex, _ := strconv.Atoi(trackStr)
		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil || item.FilePath == "" {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}

		if streams, serr := libSvc.GetMediaStreams(r.Context(), mediaID); serr == nil {
			subIdx := 0
			for _, s := range streams {
				if s.StreamType != "subtitle" {
					continue
				}
				if subIdx == trackIndex && s.IsExternal && s.ExternalPath != nil {
					serveExternalSubtitle(w, r, *s.ExternalPath, baseURL)
					return
				}
				subIdx++
			}
		}

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		cmd := exec.CommandContext(ctx, "ffmpeg",
			"-i", item.FilePath,
			"-map", fmt.Sprintf("0:s:%d", trackIndex),
			"-f", "webvtt",
			"-",
		)
		w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		setCORSHeaders(w, r, baseURL)
		cmd.Stdout = w
		if err := cmd.Start(); err != nil {
			slog.Error("ffmpeg subtitle start", slog.Any("err", err))
			return
		}
		go func() { <-r.Context().Done(); cancel() }()
		if err := cmd.Wait(); err != nil && ctx.Err() == nil {
			slog.Warn("ffmpeg subtitle", slog.String("media_id", mediaID), slog.Any("err", err))
		}
	}
}

func chaptersHandler(tokenGen interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, libSvc interface {
	GetChapters(ctx context.Context, mediaItemID string) ([]*domainlibrary.Chapter, error)
}) http.HandlerFunc {
	type chapterJSON struct {
		Title       string `json:"title"`
		StartTimeMs int64  `json:"startTimeMs"`
		EndTimeMs   int64  `json:"endTimeMs"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		token := r.Header.Get("Authorization")
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		} else if t := r.URL.Query().Get("token"); t != "" {
			token = t
		} else {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		if _, err := tokenGen.ValidateAccessToken(token); err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		chapters, err := libSvc.GetChapters(r.Context(), mediaID)
		if err != nil {
			http.Error(w, `{"message":"internal"}`, http.StatusInternalServerError)
			return
		}
		out := make([]chapterJSON, 0, len(chapters))
		for _, ch := range chapters {
			out = append(out, chapterJSON{
				Title:       ch.Title,
				StartTimeMs: ch.StartTimeMs,
				EndTimeMs:   ch.EndTimeMs,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(map[string]any{"chapters": out})
	}
}

func serveExternalSubtitle(w http.ResponseWriter, r *http.Request, path, baseURL string) {
	ext := strings.ToLower(filepath.Ext(path))
	setCORSHeaders(w, r, baseURL)
	w.Header().Set("Cache-Control", "no-cache")
	if ext == ".vtt" {
		w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
		http.ServeFile(w, r, path)
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-i", path, "-f", "webvtt", "-")
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	cmd.Stdout = w
	if err := cmd.Start(); err != nil {
		slog.Error("ffmpeg external subtitle", slog.Any("err", err))
		return
	}
	go func() { <-r.Context().Done(); cancel() }()
	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		slog.Warn("ffmpeg external subtitle convert", slog.String("path", path), slog.Any("err", err))
	}
}

func libraryStatsHandler(tokenGen interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, libSvc interface {
	ListLibraries(ctx context.Context) ([]*domainlibrary.Library, error)
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		} else if t := r.URL.Query().Get("token"); t != "" {
			token = t
		}
		if _, err := tokenGen.ValidateAccessToken(token); err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		libs, err := libSvc.ListLibraries(r.Context())
		if err != nil {
			http.Error(w, `{"message":"internal"}`, http.StatusInternalServerError)
			return
		}
		type libStat struct {
			ID            string  `json:"id"`
			LastScannedAt *string `json:"lastScannedAt"`
		}
		stats := make([]libStat, 0, len(libs))
		for _, lib := range libs {
			var ts *string
			if lib.LastScannedAt != nil {
				s := lib.LastScannedAt.UTC().Format(time.RFC3339)
				ts = &s
			}
			stats = append(stats, libStat{ID: lib.ID, LastScannedAt: ts})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"stats": stats})
	}
}
