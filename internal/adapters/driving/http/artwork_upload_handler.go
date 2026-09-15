package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
)

// artwork_upload_handler.go - POST an image to use as an item's poster or
// backdrop, instead of picking one of the provider's.
//
// Admin-only, like the rest of fiche editing. Modelled on the avatar upload
// (avatar_handler.go), which already solved the same problems: the body read
// deadline is cleared because a large image over a slow link is not a stalled
// request, the size cap is enforced by MaxBytesReader rather than trusting
// Content-Length, and the type is decided by the file's magic bytes rather than
// by its extension or the client's Content-Type - both of which the client
// chooses and neither of which proves anything.

// maxArtworkUpload bounds the request body. Slightly above the service's own
// cap so an over-size image is refused with a clear 400 here rather than as a
// generic write failure deeper down.
const maxArtworkUpload = 13 << 20

// artworkTypeFromForm maps the form's short type to the domain constant. Only
// the two an admin can actually set are accepted; anything else would create a
// record no screen ever displays.
func artworkTypeFromForm(v string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "poster", "primary":
		return "ARTWORK_TYPE_PRIMARY", true
	case "backdrop":
		return "ARTWORK_TYPE_BACKDROP", true
	default:
		return "", false
	}
}

// imageExtFromContentType returns the extension to store under for a detected
// content type, and whether it is an image we accept at all.
func imageExtFromContentType(ct string) (string, bool) {
	switch {
	case strings.HasPrefix(ct, "image/jpeg"):
		return ".jpg", true
	case strings.HasPrefix(ct, "image/png"):
		return ".png", true
	case strings.HasPrefix(ct, "image/webp"):
		return ".webp", true
	default:
		// Deliberately narrow. GIF and SVG are images but not posters: SVG in
		// particular is a script-bearing format that would be served back to
		// browsers from our own origin.
		return "", false
	}
}

func registerArtworkUploadRoute(
	mux *http.ServeMux,
	metaCmd *appmetadata.CommandService,
	tokenGen TokenVerifier,
	libSvc MediaItemGetter,
	artworkRepo ArtworkRepo,
) {
	admin := httpAdminMiddleware(tokenGen)

	mux.HandleFunc("POST /api/v1/admin/media/{id}/artwork", admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.admin.ArtworkUpload")
		defer span.End()
		r = r.WithContext(ctx)

		mediaID := strings.TrimSpace(r.PathValue("id"))
		if mediaID == "" {
			http.Error(w, `{"message":"missing media id"}`, http.StatusBadRequest)
			return
		}
		// Reject an unknown item before reading a 12 MB body off the wire.
		if _, err := libSvc.GetMediaItem(ctx, mediaID); err != nil {
			span.SetStatus(otelcodes.Error, "media item not found")
			http.NotFound(w, r)
			return
		}

		// A poster over a slow uplink is a long request, not a stalled one.
		ClearBodyReadDeadline(w)
		r.Body = http.MaxBytesReader(w, r.Body, maxArtworkUpload)
		if err := r.ParseMultipartForm(maxArtworkUpload); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "body too large or malformed")
			http.Error(w, `{"message":"image too large (max 12MB) or malformed form"}`, http.StatusBadRequest)
			return
		}

		artType, ok := artworkTypeFromForm(r.FormValue("type"))
		if !ok {
			span.SetStatus(otelcodes.Error, "unsupported artwork type")
			http.Error(w, `{"message":"type must be poster or backdrop"}`, http.StatusBadRequest)
			return
		}

		file, _, err := r.FormFile("artwork")
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "missing artwork file")
			http.Error(w, `{"message":"missing artwork file"}`, http.StatusBadRequest)
			return
		}
		defer func() { _ = file.Close() }()

		// Magic bytes decide, not the filename: an admin renaming a .exe to
		// .jpg must not get it stored and served from our origin.
		head := make([]byte, 512)
		n, _ := file.Read(head) //nolint:errcheck // a short/failed read yields n=0 and is rejected as "not an image" below
		ext, isImage := imageExtFromContentType(http.DetectContentType(head[:n]))
		if !isImage {
			span.SetStatus(otelcodes.Error, "not a supported image")
			http.Error(w, `{"message":"file must be a JPEG, PNG or WebP image"}`, http.StatusBadRequest)
			return
		}
		if _, seekErr := file.Seek(0, 0); seekErr != nil {
			span.RecordError(seekErr)
			logInternal(r.Context(), "artwork upload: seek", seekErr)
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}

		rec, err := metaCmd.AddCustomArtwork(ctx, shared.MediaItemID(mediaID), artType, file, ext)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "store artwork failed")
			slog.ErrorContext(ctx, "custom artwork upload", slog.String("media_item_id", mediaID), slog.Any("err", err))
			http.Error(w, `{"message":"could not store the image"}`, http.StatusInternalServerError)
			return
		}

		// The item's artwork list is memoised; without this the admin uploads a
		// poster and keeps seeing the old one until the TTL lapses.
		artworkRecordsCache.Delete(artworkCacheKey(artworkRepo, mediaID))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": rec.ID, "type": rec.Type})
	}))
}
