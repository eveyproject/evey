package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	appmedia "github.com/evey-project/evey/internal/app/media"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

func streamTokenHandler(tokenGen *auth.JWTGenerator, store *appmedia.StreamTokenStore, libAccess LibraryAccessChecker, mediaGetter mediaLibraryGetter, parental ParentalChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.stream_token.Issue")
		defer span.End()
		r = r.WithContext(ctx)
		claims := HTTPClaimsFromContext(r.Context())
		mediaID := r.PathValue("id")
		span.SetAttributes(attribute.String("media_item_id", mediaID))
		item, err := mediaGetter.GetMediaItem(r.Context(), mediaID)
		if err != nil || item == nil {
			if err != nil {
				span.RecordError(err)
			}
			span.SetStatus(otelcodes.Error, "media item not found")
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libAccess) {
			span.SetStatus(otelcodes.Error, "library access denied")
			return
		}
		if !checkParental(r.Context(), w, claims, item, parental) {
			span.SetStatus(otelcodes.Error, "parental restricted")
			return
		}

		// Cast playback gets a long-lived token: the Chromecast fetches
		// segments itself and can't re-validate the short sliding token
		// the way the local player's xhrSetup does, so the 5-minute
		// default expires during its buffer-quiet stretches.
		var token string
		if r.URL.Query().Get("purpose") == "cast" {
			token = store.CreateWithTTL(claims.UserID, mediaID, appmedia.CastStreamTokenTTL)
		} else {
			token = store.Create(claims.UserID, mediaID)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{
			"token":   token,
			"mediaId": mediaID,
		}); err != nil {
			slog.Warn("stream token encode failed", slog.Any("err", err))
		}
	}
}
