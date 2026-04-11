package httpapi

import (
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "golang.org/x/image/webp"
)

func trickplayManifestHandler(tokenGen TokenVerifier, libSvc TrickplayGetter, baseURL string, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		claims := HTTPClaimsFromContext(r.Context())

		if st, ok := StreamTokenFromContext(r.Context()); ok {
			if st.MediaItemID != mediaID {
				http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
				return
			}
		}

		item, err := mediaGetter.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			return
		}

		manifest, err := libSvc.GetTrickplayManifest(r.Context(), mediaID)
		if err != nil {
			w.Header().Set("Content-Type", "text/vtt")
			w.Header().Set("Cache-Control", "no-cache")
			fmt.Fprint(w, "WEBVTT\n")
			return
		}
		tilesPerImage := manifest.TileWidth * manifest.TileHeight

		// Compute real frame count from video duration (not ImageCount * tiles which overestimates)
		totalFrames := manifest.ImageCount * tilesPerImage
		if item.DurationMs > 0 && manifest.Interval > 0 {
			realFrames := int(item.DurationMs/1000) / manifest.Interval
			if realFrames < totalFrames {
				totalFrames = realFrames
			}
		}

		// Detect sprite format: prefer .webp, fall back to .jpg (old caches)
		spriteExt := ".webp"
		if manifest.ImagePath != "" {
			if _, err := os.Stat(filepath.Join(manifest.ImagePath, "trickplay_001.webp")); err != nil {
				spriteExt = ".jpg"
			}
		}

		// Compute actual tile height from first sprite image (ffmpeg scale=-1 preserves aspect ratio)
		tileH := manifest.Width * 9 / 16 // fallback 16:9
		if manifest.ImagePath != "" && manifest.TileWidth > 0 && manifest.TileHeight > 0 {
			firstSprite := filepath.Join(manifest.ImagePath, "trickplay_001"+spriteExt)
			if f, openErr := os.Open(firstSprite); openErr == nil {
				defer f.Close()
				if cfg, _, decErr := image.DecodeConfig(f); decErr == nil && cfg.Width > 0 && cfg.Height > 0 {
					tileH = cfg.Height / manifest.TileHeight
				}
			}
		}

		w.Header().Set("Content-Type", "text/vtt")
		setCORSHeaders(w, r, baseURL)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		fmt.Fprint(w, "WEBVTT\n\n")
		for i := range totalFrames {
			startSec := i * manifest.Interval
			endSec := (i + 1) * manifest.Interval
			imageIdx := i / tilesPerImage
			tileIdx := i % tilesPerImage
			col := tileIdx % manifest.TileWidth
			row := tileIdx / manifest.TileWidth
			xywh := fmt.Sprintf("%d,%d,%d,%d",
				col*manifest.Width, row*tileH,
				manifest.Width, tileH,
			)
			imageFile := fmt.Sprintf("trickplay_%03d%s", imageIdx+1, spriteExt)
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

func trickplayImageHandler(tokenGen TokenVerifier, libSvc TrickplayGetter, baseURL string, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		imageName := r.PathValue("image")
		claims := HTTPClaimsFromContext(r.Context())

		if st, ok := StreamTokenFromContext(r.Context()); ok {
			if st.MediaItemID != mediaID {
				http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
				return
			}
		}

		item, err := mediaGetter.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			return
		}

		manifest, err := libSvc.GetTrickplayManifest(r.Context(), mediaID)
		if err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		imagePath, err := safeJoin(manifest.ImagePath, filepath.Base(imageName))
		if err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if _, err := os.Stat(imagePath); err != nil {
			// Backward compat: try .jpg if .webp not found (or vice versa)
			alt := imagePath
			switch {
			case strings.HasSuffix(imagePath, ".webp"):
				alt = strings.TrimSuffix(imagePath, ".webp") + ".jpg"
			case strings.HasSuffix(imagePath, ".jpg"):
				alt = strings.TrimSuffix(imagePath, ".jpg") + ".webp"
			}
			if _, err := os.Stat(alt); err != nil {
				http.Error(w, `{"message":"image not found"}`, http.StatusNotFound)
				return
			}
			imagePath = alt
		}
		setCORSHeaders(w, r, baseURL)
		serveFileWithCache(w, r, imagePath)
	}
}

func trickplayGenerateHandler(tokenGen TokenVerifier, libSvc TrickplayGenerator, serverCtx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("recovered panic in trickplay generation", "panic", r)
				}
			}()
			ctx, cancel := context.WithTimeout(serverCtx, 5*time.Minute)
			defer cancel()
			if err := libSvc.GenerateTrickplay(ctx, mediaID); err != nil {
				slog.Error("trickplay generation failed", slog.String("media_id", mediaID), slog.Any("err", err))
			} else {
				slog.Info("trickplay generated", slog.String("media_id", mediaID))
			}
		}()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"message":"trickplay generation started"}`))
	}
}

func trickplayGenerateAllHandler(tokenGen TokenVerifier, libSvc AllTrickplayGenerator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		count, err := libSvc.GenerateAllMissingTrickplays(r.Context())
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(w, `{"message":"trickplay generation started","count":%d}`, count)
	}
}
