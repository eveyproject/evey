package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	appmedia "github.com/gravity-zero/evey/internal/app/media"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func streamTokenHandler(tokenGen *auth.JWTGenerator, store *appmedia.StreamTokenStore, libAccess LibraryAccessChecker, mediaGetter mediaLibraryGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		claims, err := tokenGen.ValidateAccessToken(authHeader[7:])
		if err != nil {
			http.Error(w, `{"message":"invalid token"}`, http.StatusUnauthorized)
			return
		}

		mediaID := r.PathValue("id")
		item, err := mediaGetter.GetMediaItem(r.Context(), mediaID)
		if err != nil || item == nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libAccess) {
			return
		}

		token := store.Create(claims.UserID, mediaID)

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{
			"token":   token,
			"mediaId": mediaID,
		}); err != nil {
			slog.Warn("stream token encode failed", slog.Any("err", err))
		}
	}
}
