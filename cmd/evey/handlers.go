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

	appsubtitle "github.com/gravity-zero/evey/internal/app/subtitle"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	domainmeta "github.com/gravity-zero/evey/internal/domain/metadata"
	domainplayback "github.com/gravity-zero/evey/internal/domain/playback"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	domaintranscoding "github.com/gravity-zero/evey/internal/domain/transcoding"
	"github.com/gravity-zero/evey/internal/pkg/hdr"
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
		var streams []*domainlibrary.MediaStream
		if s, err := libSvc.GetMediaStreams(r.Context(), mediaID); err == nil {
			streams = s
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

		hdrInfo := hdr.DetectHDR(streams)
		clientSupportsHDR := r.URL.Query().Get("hdr") == "true"
		needsToneMap := hdrInfo.IsHDR && !clientSupportsHDR
		if needsToneMap {
			needsVideoTranscode = true
		}

		slog.Debug("transcode: decision", slog.String("media_id", mediaID), slog.Bool("needsVideoTranscode", needsVideoTranscode), slog.Bool("needsAudioTranscode", needsAudioTranscode), slog.Bool("remuxOnly", remuxOnly), slog.Bool("needsToneMap", needsToneMap))

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		args := []string{
			"-i", item.FilePath,
			"-map", "0:v:0",
			"-map", fmt.Sprintf("0:a:%d", audioTrack),
		}

		if needsToneMap && !remuxOnly {
			toneMapFilter := hdr.BuildToneMapFilter(hdrInfo, hdr.DefaultToneMapOptions())
			if videoScale > 0 {
				args = append(args, "-vf", fmt.Sprintf("scale=-2:%d,%s", videoScale, toneMapFilter))
			} else {
				args = append(args, "-vf", toneMapFilter)
			}
			args = append(args, "-c:v", "libx264", "-crf", "23", "-preset", "fast")
		} else if videoScale > 0 {
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

func activeSessionsHandler(tokenGen interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, pbSvc interface {
	GetActiveSessions(ctx context.Context) ([]*domainplayback.ActiveSessionInfo, error)
}) http.HandlerFunc {
	type sessionJSON struct {
		SessionID     string  `json:"sessionId"`
		ProfileName   string  `json:"profileName"`
		MediaTitle    string  `json:"mediaTitle"`
		MediaType     string  `json:"mediaType"`
		MediaItemID   string  `json:"mediaItemId"`
		Progress      float64 `json:"progress"`
		DurationMs    int64   `json:"durationMs"`
		PositionMs    int64   `json:"positionMs"`
		StartedAt     string  `json:"startedAt"`
		DeviceName    string  `json:"deviceName"`
		DeviceType    string  `json:"deviceType"`
		ClientIP      string  `json:"clientIp"`
		PlayMethod    string  `json:"playMethod"`
		VideoCodec    string  `json:"videoCodec"`
		AudioCodec    string  `json:"audioCodec"`
		Resolution    string  `json:"resolution"`
		BandwidthBps  int64   `json:"bandwidthBps"`
		TranscodeInfo *string `json:"transcodeInfo,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		}
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil || !claims.IsAdmin {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		sessions, err := pbSvc.GetActiveSessions(r.Context())
		if err != nil {
			slog.Error("active sessions", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		out := make([]sessionJSON, 0, len(sessions))
		for _, s := range sessions {
			out = append(out, sessionJSON{
				SessionID:     s.SessionID,
				ProfileName:   s.ProfileName,
				MediaTitle:    s.MediaTitle,
				MediaType:     s.MediaType,
				MediaItemID:   s.MediaItemID,
				Progress:      s.ProgressPercent(),
				DurationMs:    s.DurationMs,
				PositionMs:    s.PositionMs,
				StartedAt:     s.StartedAt.UTC().Format(time.RFC3339),
				DeviceName:    s.DeviceName,
				DeviceType:    s.DeviceType,
				ClientIP:      s.ClientIP,
				PlayMethod:    s.PlayMethod,
				VideoCodec:    s.VideoCodec,
				AudioCodec:    s.AudioCodec,
				Resolution:    s.Resolution,
				BandwidthBps:  s.BandwidthBps,
				TranscodeInfo: s.TranscodeInfo,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"sessions": out})
	}
}

func killSessionHandler(tokenGen interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, pbSvc interface {
	StopStream(ctx context.Context, sessionID string) error
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		}
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil || !claims.IsAdmin {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		sessionID := r.PathValue("id")
		if sessionID == "" {
			http.Error(w, `{"message":"session id required"}`, http.StatusBadRequest)
			return
		}
		if err := pbSvc.StopStream(r.Context(), sessionID); err != nil {
			slog.Error("kill session", slog.Any("err", err))
			http.Error(w, `{"message":"session not found"}`, http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func trickplayManifestHandler(tokenGen interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, libSvc interface {
	GetTrickplayManifest(ctx context.Context, mediaItemID string) (*domainlibrary.TrickplayManifest, error)
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
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
		manifest, err := libSvc.GetTrickplayManifest(r.Context(), mediaID)
		if err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		tilesPerImage := manifest.TileWidth * manifest.TileHeight
		totalFrames := manifest.ImageCount * tilesPerImage
		w.Header().Set("Content-Type", "text/vtt")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		fmt.Fprint(w, "WEBVTT\n\n")
		for i := 0; i < totalFrames; i++ {
			startSec := i * manifest.Interval
			endSec := (i + 1) * manifest.Interval
			imageIdx := i / tilesPerImage
			tileIdx := i % tilesPerImage
			col := tileIdx % manifest.TileWidth
			row := tileIdx / manifest.TileWidth
			xywh := fmt.Sprintf("%d,%d,%d,%d",
				col*manifest.Width, row*manifest.Width*9/16,
				manifest.Width, manifest.Width*9/16,
			)
			imageFile := fmt.Sprintf("trickplay_%03d.jpg", imageIdx+1)
			imageURL := fmt.Sprintf("/api/v1/media/%s/trickplay/%s", mediaID, imageFile)
			fmt.Fprintf(w, "%s --> %s\n%s#xywh=%s\n\n",
				formatVTTTime(startSec), formatVTTTime(endSec),
				imageURL, xywh,
			)
		}
	}
}

func formatVTTTime(totalSeconds int) string {
	h := totalSeconds / 3600
	m := (totalSeconds % 3600) / 60
	s := totalSeconds % 60
	return fmt.Sprintf("%02d:%02d:%02d.000", h, m, s)
}

func trickplayImageHandler(tokenGen interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, libSvc interface {
	GetTrickplayManifest(ctx context.Context, mediaItemID string) (*domainlibrary.TrickplayManifest, error)
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		imageName := r.PathValue("image")
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
		manifest, err := libSvc.GetTrickplayManifest(r.Context(), mediaID)
		if err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		imagePath := filepath.Join(manifest.ImagePath, filepath.Base(imageName))
		if _, err := os.Stat(imagePath); err != nil {
			http.Error(w, `{"message":"image not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, imagePath)
	}
}

func trickplayGenerateHandler(tokenGen interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, libSvc interface {
	GenerateTrickplay(ctx context.Context, mediaItemID string, cacheDir string) error
}, cacheDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		token := r.Header.Get("Authorization")
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		} else if t := r.URL.Query().Get("token"); t != "" {
			token = t
		}
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil || !claims.IsAdmin {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		go func() {
			ctx := context.Background()
			if err := libSvc.GenerateTrickplay(ctx, mediaID, cacheDir); err != nil {
				slog.Error("trickplay generation failed", slog.String("media_id", mediaID), slog.Any("err", err))
			} else {
				slog.Info("trickplay generated", slog.String("media_id", mediaID))
			}
		}()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"message":"trickplay generation started"}`))
	}
}

func registerSubtitleRoutes(mux *http.ServeMux, subtitleSvc *appsubtitle.Service, tokenGen interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}) {
	extractToken := func(r *http.Request) string {
		h := r.Header.Get("Authorization")
		if strings.HasPrefix(h, "Bearer ") {
			return h[7:]
		}
		if t := r.URL.Query().Get("token"); t != "" {
			return t
		}
		return ""
	}
	requireAuth := func(w http.ResponseWriter, r *http.Request) bool {
		token := extractToken(r)
		if token == "" {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return false
		}
		if _, err := tokenGen.ValidateAccessToken(token); err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return false
		}
		return true
	}

	mux.HandleFunc("GET /api/v1/media/{id}/subtitles/search", func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		mediaID := r.PathValue("id")
		language := r.URL.Query().Get("language")
		results, err := subtitleSvc.Search(r.Context(), mediaID, language)
		if err != nil {
			slog.Error("subtitle search", slog.Any("err", err))
			http.Error(w, `{"message":"search failed"}`, http.StatusInternalServerError)
			return
		}
		type resultJSON struct {
			FileID   int     `json:"fileId"`
			FileName string  `json:"fileName"`
			Language string  `json:"language"`
			Format   string  `json:"format"`
			Rating   float64 `json:"rating"`
		}
		out := make([]resultJSON, 0, len(results))
		for _, r := range results {
			out = append(out, resultJSON{
				FileID:   r.FileID,
				FileName: r.FileName,
				Language: r.Language,
				Format:   r.Format,
				Rating:   r.Rating,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"results": out})
	})

	mux.HandleFunc("POST /api/v1/media/{id}/subtitles/download", func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		mediaID := r.PathValue("id")
		var body struct {
			FileID   int    `json:"fileId"`
			Language string `json:"language"`
			Format   string `json:"format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		if body.FileID == 0 {
			http.Error(w, `{"message":"fileId required"}`, http.StatusBadRequest)
			return
		}
		sub, err := subtitleSvc.Download(r.Context(), mediaID, body.FileID, body.Language, body.Format)
		if err != nil {
			slog.Error("subtitle download", slog.Any("err", err))
			http.Error(w, `{"message":"download failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id":           sub.ID,
			"mediaItemId":  sub.MediaItemID,
			"language":     sub.Language,
			"format":       sub.Format,
			"filePath":     sub.FilePath,
			"source":       sub.Source,
			"downloadedAt": sub.DownloadedAt.UTC().Format(time.RFC3339),
		})
	})

	mux.HandleFunc("GET /api/v1/media/{id}/subtitles", func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		mediaID := r.PathValue("id")
		subs, err := subtitleSvc.List(r.Context(), mediaID)
		if err != nil {
			slog.Error("subtitle list", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		type subJSON struct {
			ID           string `json:"id"`
			MediaItemID  string `json:"mediaItemId"`
			Language     string `json:"language"`
			Format       string `json:"format"`
			FilePath     string `json:"filePath"`
			Source       string `json:"source"`
			DownloadedAt string `json:"downloadedAt"`
		}
		out := make([]subJSON, 0, len(subs))
		for _, s := range subs {
			out = append(out, subJSON{
				ID:           s.ID,
				MediaItemID:  s.MediaItemID,
				Language:     s.Language,
				Format:       s.Format,
				FilePath:     s.FilePath,
				Source:       s.Source,
				DownloadedAt: s.DownloadedAt.UTC().Format(time.RFC3339),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"subtitles": out})
	})

	mux.HandleFunc("DELETE /api/v1/media/{id}/subtitles/{subtitleId}", func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		subtitleID := r.PathValue("subtitleId")
		if err := subtitleSvc.Delete(r.Context(), subtitleID); err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func hlsMasterHandler(tokenGen interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, libSvc interface {
	GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error)
	GetMediaStreams(ctx context.Context, mediaItemID string) ([]*domainlibrary.MediaStream, error)
}, hlsManager *apptranscoding.HLSManager) http.HandlerFunc {
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

		sourceWidth, sourceHeight := 1920, 1080
		if streams, err := libSvc.GetMediaStreams(r.Context(), mediaID); err == nil {
			for _, s := range streams {
				if s.StreamType == "video" && s.Width != nil && s.Height != nil {
					sourceWidth = *s.Width
					sourceHeight = *s.Height
					break
				}
			}
		}

		session, err := hlsManager.GetOrCreate(mediaID, item.FilePath, sourceWidth, sourceHeight)
		if err != nil {
			slog.Error("hls: get or create session", slog.Any("err", err))
			http.Error(w, `{"message":"transcode busy"}`, http.StatusServiceUnavailable)
			return
		}
		if err := session.WaitReady(r.Context()); err != nil {
			slog.Error("hls: wait ready", slog.Any("err", err))
			http.Error(w, `{"message":"transcode failed"}`, http.StatusInternalServerError)
			return
		}

		setCORSHeaders(w, r, "")
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		http.ServeFile(w, r, filepath.Join(session.OutputDir, "master.m3u8"))
	}
}

func hlsSegmentHandler(tokenGen interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, hlsManager *apptranscoding.HLSManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		file := r.PathValue("file")
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

		session, ok := hlsManager.GetSession(mediaID)
		if !ok {
			http.Error(w, `{"message":"no active session"}`, http.StatusNotFound)
			return
		}
		session.WaitReady(r.Context())

		file = filepath.Base(file)
		filePath := filepath.Join(session.OutputDir, file)
		if _, err := os.Stat(filePath); err != nil {
			http.Error(w, `{"message":"segment not found"}`, http.StatusNotFound)
			return
		}

		switch {
		case strings.HasSuffix(file, ".m3u8"):
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		case strings.HasSuffix(file, ".ts"):
			w.Header().Set("Content-Type", "video/mp2t")
		}
		session.MarkAccess()
		http.ServeFile(w, r, filePath)
	}
}

func playbackInfoHandler(tokenGen interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, libSvc interface {
	GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error)
	GetMediaStreams(ctx context.Context, mediaItemID string) ([]*domainlibrary.MediaStream, error)
}) http.HandlerFunc {
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

		streams, err := libSvc.GetMediaStreams(r.Context(), mediaID)
		if err != nil {
			http.Error(w, `{"message":"internal"}`, http.StatusInternalServerError)
			return
		}

		client := domaintranscoding.DefaultWebProfile
		if v := r.URL.Query().Get("maxWidth"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				client.MaxWidth = n
			}
		}
		if v := r.URL.Query().Get("maxHeight"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				client.MaxHeight = n
			}
		}
		if v := r.URL.Query().Get("maxBitrate"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
				client.MaxBitrate = n
			}
		}
		if v := r.URL.Query().Get("videoCodecs"); v != "" {
			client.VideoCodecs = strings.Split(v, ",")
		}
		if v := r.URL.Query().Get("audioCodecs"); v != "" {
			client.AudioCodecs = strings.Split(v, ",")
		}
		if v := r.URL.Query().Get("containers"); v != "" {
			client.Containers = strings.Split(v, ",")
		}
		if r.URL.Query().Get("hdr") == "true" {
			client.SupportsHDR = true
		}

		var userMaxBitrate int
		if client.MaxBitrate > 0 {
			userMaxBitrate = int(client.MaxBitrate)
			client.MaxBitrate = 0
		}

		decision := apptranscoding.MakePlaybackDecision(streams, item.ContainerFormat, client, userMaxBitrate)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(decision)
	}
}
