package httpapi

import (
	"context"
	"encoding/json"
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

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	_ "golang.org/x/image/webp"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

func trickplayManifestHandler(tokenGen TokenVerifier, libSvc TrickplayGetter, baseURL string, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.trickplay.Manifest")
		defer span.End()
		r = r.WithContext(ctx)
		mediaID := r.PathValue("id")
		span.SetAttributes(attribute.String("media_item_id", mediaID))
		claims := HTTPClaimsFromContext(r.Context())

		if st, ok := StreamTokenFromContext(r.Context()); ok {
			if st.MediaItemID != shared.MediaItemID(mediaID) {
				span.SetStatus(otelcodes.Error, "stream token media mismatch")
				http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
				return
			}
		}

		item, err := mediaGetter.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "media item not found")
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		// Trickplay is public: skip library access check when no auth claims.
		if claims != nil && !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			span.SetStatus(otelcodes.Error, "library access denied")
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

		// Read the per-frame timestamp sidecar — required, no fallback.
		// (interval-based timing was wrong when keyframes are sparse.)
		frameMs, sidecarErr := readSidecarTimestamps(manifest.ImagePath)
		if sidecarErr != nil || len(frameMs) == 0 {
			slog.Warn("trickplay: missing or empty sidecar",
				slog.String("media_id", mediaID),
				slog.String("image_path", manifest.ImagePath),
				slog.Any("err", sidecarErr),
			)
			w.Header().Set("Content-Type", "text/vtt")
			w.Header().Set("Cache-Control", "no-cache")
			fmt.Fprint(w, "WEBVTT\n")
			return
		}

		var durationMs int64
		if item.File != nil {
			durationMs = item.File.Duration.Milliseconds()
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
		// Cue bounds use midpoints between successive frames so a hover at
		// time T resolves to the frame whose actual timestamp is closest to T,
		// not just the previous one. With 7-9s keyframe spacing the difference
		// matters: hover at 22.5s should show the 22.689s frame, not the
		// 14.848s one ~8s earlier.
		for i, frameTs := range frameMs {
			var cueStart, cueEnd int64
			if i == 0 {
				cueStart = 0
			} else {
				cueStart = (frameMs[i-1] + frameTs) / 2
			}
			if i+1 < len(frameMs) {
				cueEnd = (frameTs + frameMs[i+1]) / 2
			} else if durationMs > 0 {
				cueEnd = durationMs
			} else {
				cueEnd = frameTs + 8000
			}
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
				formatVTTTimeMs(cueStart), formatVTTTimeMs(cueEnd),
				imageURL, xywh,
			)
		}
	}
}

// readSidecarTimestamps reads trickplay.json next to the sprites and returns
// the actual per-frame timestamps in ms.
func readSidecarTimestamps(imagePath string) ([]int64, error) {
	if imagePath == "" {
		return nil, fmt.Errorf("empty image path")
	}
	path := filepath.Join(imagePath, "trickplay.json")
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sc struct {
		FrameTimestamps []int64 `json:"frame_ms"`
	}
	if err := json.Unmarshal(buf, &sc); err != nil {
		return nil, err
	}
	return sc.FrameTimestamps, nil
}

func formatVTTTimeMs(totalMs int64) string {
	h := totalMs / 3_600_000
	m := (totalMs % 3_600_000) / 60_000
	s := (totalMs % 60_000) / 1000
	ms := totalMs % 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

func trickplayImageHandler(tokenGen TokenVerifier, libSvc TrickplayGetter, baseURL string, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.trickplay.Image")
		defer span.End()
		r = r.WithContext(ctx)
		mediaID := r.PathValue("id")
		imageName := r.PathValue("image")
		span.SetAttributes(
			attribute.String("media_item_id", mediaID),
			attribute.String("image", imageName),
		)
		claims := HTTPClaimsFromContext(r.Context())

		if st, ok := StreamTokenFromContext(r.Context()); ok {
			if st.MediaItemID != shared.MediaItemID(mediaID) {
				span.SetStatus(otelcodes.Error, "stream token media mismatch")
				http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
				return
			}
		}

		item, err := mediaGetter.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "media item not found")
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		// Trickplay image is public: skip library access check when no auth claims.
		if claims != nil && !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			span.SetStatus(otelcodes.Error, "library access denied")
			return
		}

		manifest, err := libSvc.GetTrickplayManifest(r.Context(), mediaID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "manifest not found")
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		imagePath, err := safeJoin(manifest.ImagePath, filepath.Base(imageName))
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "unsafe image path")
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
		_, span := tracing.StartSpan(r.Context(), "http.trickplay.Generate")
		defer span.End()
		mediaID := r.PathValue("id")
		span.SetAttributes(attribute.String("media_item_id", mediaID))
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
		ctx, span := tracing.StartSpan(r.Context(), "http.trickplay.GenerateAll")
		defer span.End()
		r = r.WithContext(ctx)
		count, err := libSvc.GenerateAllMissingTrickplays(r.Context())
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "generate all failed")
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(w, `{"message":"trickplay generation started","count":%d}`, count)
	}
}
