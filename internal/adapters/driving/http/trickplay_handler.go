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
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	_ "golang.org/x/image/webp"

	appmedia "github.com/eveyproject/evey/internal/app/media"
	"github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// spriteCredential returns the ?token= suffix the manifest must put on each
// sprite URL. The sprites are loaded by plain <img> tags, which can attach
// neither an Authorization header nor a cookie the auth layer would read, so
// the URL has to carry the credential itself - the same reason <video src> and
// hls.js use stream tokens. Reusing the caller's own token when it has one
// keeps a watch-together guest inside the single media item it was granted;
// otherwise a fresh media-scoped token is minted for the authenticated caller.
//
// The long TTL mirrors the cast/native case: the manifest is fetched once per
// page load while sprites keep being pulled as the user scrubs, so a 5-minute
// token would expire mid-session with no way to re-issue it.
func spriteCredential(r *http.Request, store *appmedia.StreamTokenStore, claims *domainuser.Claims, mediaID string) string {
	if raw := RawTokenFromContext(r.Context()); raw != "" {
		return "?token=" + url.QueryEscape(raw)
	}
	if store == nil || claims == nil {
		return ""
	}
	return "?token=" + url.QueryEscape(
		store.CreateWithTTL(shared.UserID(claims.UserID), shared.ProfileID(claims.ProfileID), shared.MediaItemID(mediaID), appmedia.CastStreamTokenTTL))
}

func trickplayManifestHandler(libSvc TrickplayGetter, baseURL string, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter, parental ParentalChecker, tokenStore *appmedia.StreamTokenStore) http.HandlerFunc {
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
		// Trickplay sprites are FRAMES OF THE FILM, so this gate is the same one
		// every sibling media route applies - it used to be skipped whenever
		// claims were nil, which combined with the route being mounted without
		// auth meant any anonymous caller got a storyboard of any media id, past
		// both library access and the parental gate (checkParental also passes on
		// nil claims). The watch-together guest is served by the stream token
		// checked above, which is scoped to its one media item.
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			span.SetStatus(otelcodes.Error, "library access denied")
			return
		}
		if !checkParental(r.Context(), w, claims, item, parental) {
			span.SetStatus(otelcodes.Error, "parental restricted")
			return
		}

		// Minted AFTER the access + parental gates above, so an unauthorised
		// caller never obtains a usable sprite credential.
		spriteCred := spriteCredential(r, tokenStore, claims, mediaID)

		manifest, err := libSvc.GetTrickplayManifest(r.Context(), mediaID)
		if err != nil {
			w.Header().Set("Content-Type", "text/vtt")
			w.Header().Set("Cache-Control", "no-cache")
			fmt.Fprint(w, "WEBVTT\n")
			return
		}
		tilesPerImage := manifest.TileWidth * manifest.TileHeight
		if tilesPerImage <= 0 || manifest.TileWidth <= 0 {
			// A zero/negative tile geometry (corrupted or partial-migration row)
			// would divide-by-zero in the cue loop below - emit an empty VTT
			// instead of panicking every request for this item.
			w.Header().Set("Content-Type", "text/vtt")
			w.Header().Set("Cache-Control", "no-cache")
			fmt.Fprint(w, "WEBVTT\n")
			return
		}

		// Read the per-frame timestamp sidecar - required, no fallback.
		// (interval-based timing was wrong when keyframes are sparse.)
		frameMs, sidecarErr := readSidecarTimestamps(manifest.ImagePath)
		if sidecarErr != nil || len(frameMs) == 0 {
			slog.WarnContext(r.Context(), "trickplay: missing or empty sidecar",
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
		w.Header().Set("Cache-Control", "private, max-age=86400")
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
			imageURL := fmt.Sprintf("/api/v1/media/%s/trickplay/%s%s", mediaID, imageFile, spriteCred)
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

func trickplayImageHandler(libSvc TrickplayGetter, baseURL string, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter, parental ParentalChecker) http.HandlerFunc {
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
		// Same gate as the manifest above, and for the same reason: a sprite is
		// film content, not metadata.
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			span.SetStatus(otelcodes.Error, "library access denied")
			return
		}
		if !checkParental(r.Context(), w, claims, item, parental) {
			span.SetStatus(otelcodes.Error, "parental restricted")
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
		serveFileWithCache(w, r, imagePath, false)
	}
}

func trickplayGenerateHandler(libSvc TrickplayGenerator, serverCtx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.trickplay.Generate")
		defer span.End()
		mediaID := r.PathValue("id")
		span.SetAttributes(attribute.String("media_item_id", mediaID))
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.ErrorContext(serverCtx, "recovered panic in trickplay generation", "panic", r)
				}
			}()
			ctx, cancel := context.WithTimeout(serverCtx, 5*time.Minute)
			defer cancel()
			if err := libSvc.GenerateTrickplay(ctx, mediaID); err != nil {
				slog.ErrorContext(ctx, "trickplay generation failed", slog.String("media_id", mediaID), slog.Any("err", err))
			} else {
				slog.Info("trickplay generated", slog.String("media_id", mediaID))
			}
		}()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"message":"trickplay generation started"}`))
	}
}

func trickplayGenerateAllHandler(libSvc AllTrickplayGenerator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.trickplay.GenerateAll")
		defer span.End()
		r = r.WithContext(ctx)
		count, err := libSvc.GenerateAllMissingTrickplays(r.Context())
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "generate all failed")
			logInternal(r.Context(), "trickplay: generate all", err)
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(w, `{"message":"trickplay generation started","count":%d}`, count)
	}
}
