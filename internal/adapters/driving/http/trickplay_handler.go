package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func trickplayManifestHandler(tokenGen TokenVerifier, libSvc TrickplayGetter, baseURL string, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		token := r.Header.Get("Authorization")
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		} else if t := r.URL.Query().Get("token"); t != "" {
			token = t
		}
		if token == "" {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
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
		totalFrames := manifest.ImageCount * tilesPerImage
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

func trickplayImageHandler(tokenGen TokenVerifier, libSvc TrickplayGetter, baseURL string, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		imageName := r.PathValue("image")
		token := r.Header.Get("Authorization")
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		} else if t := r.URL.Query().Get("token"); t != "" {
			token = t
		} else {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
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
			http.Error(w, `{"message":"image not found"}`, http.StatusNotFound)
			return
		}
		setCORSHeaders(w, r, baseURL)
		serveFileWithCache(w, r, imagePath)
	}
}

func trickplayGenerateHandler(tokenGen TokenVerifier, libSvc TrickplayGenerator, serverCtx context.Context) http.HandlerFunc {
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
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil || !claims.IsAdmin {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
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
