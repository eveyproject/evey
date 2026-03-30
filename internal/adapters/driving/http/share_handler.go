package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	appmedia "github.com/gravity-zero/evey/internal/app/media"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func registerShareRoutes(mux *http.ServeMux, tokenGen *auth.JWTGenerator, shareSvc *appmedia.ShareService, mediaGetter mediaLibraryGetter, libAccess LibraryAccessChecker) {
	mux.HandleFunc("POST /api/v1/media/{id}/share", requireAdmin(tokenGen, shareCreateHandler(shareSvc, mediaGetter, libAccess)))
	mux.HandleFunc("GET /api/v1/admin/share-links", requireAdmin(tokenGen, shareListAllHandler(shareSvc)))
	mux.HandleFunc("DELETE /api/v1/share-links/{id}", requireAdmin(tokenGen, shareRevokeHandler(shareSvc)))
	mux.HandleFunc("GET /api/v1/share/{token}", shareValidateHandler(shareSvc, mediaGetter))
}

func shareCreateHandler(shareSvc *appmedia.ShareService, mediaGetter mediaLibraryGetter, libAccess LibraryAccessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := claimsFromContext(r.Context())
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
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

		var body struct {
			DurationHours int  `json:"durationHours"`
			MaxViews      *int `json:"maxViews"`
		}
		if decErr := json.NewDecoder(r.Body).Decode(&body); decErr != nil {
			http.Error(w, `{"message":"invalid body"}`, http.StatusBadRequest)
			return
		}
		if body.DurationHours <= 0 {
			body.DurationHours = 48
		}

		link, err := shareSvc.Create(r.Context(), mediaID, claims.UserID, time.Duration(body.DurationHours)*time.Hour, body.MaxViews)
		if err != nil {
			slog.Error("create share link", slog.Any("err", err))
			http.Error(w, `{"message":"failed to create share link"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"id":        link.ID,
			"token":     link.Token,
			"mediaId":   link.MediaItemID,
			"expiresAt": link.ExpiresAt,
			"maxViews":  link.MaxViews,
			"url":       "/share/" + link.Token,
		}); err != nil {
			slog.Error("encode share create response", slog.Any("err", err))
		}
	}
}

func shareListAllHandler(shareSvc *appmedia.ShareService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		links, err := shareSvc.ListAll(r.Context())
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		type resp struct {
			ID        string    `json:"id"`
			Token     string    `json:"token"`
			MediaID   string    `json:"mediaId"`
			CreatedBy string    `json:"createdBy"`
			ExpiresAt time.Time `json:"expiresAt"`
			MaxViews  *int      `json:"maxViews"`
			ViewCount int       `json:"viewCount"`
			URL       string    `json:"url"`
		}
		result := make([]resp, 0, len(links))
		for _, l := range links {
			result = append(result, resp{
				ID:        l.ID,
				Token:     l.Token,
				MediaID:   l.MediaItemID,
				CreatedBy: l.CreatedBy,
				ExpiresAt: l.ExpiresAt,
				MaxViews:  l.MaxViews,
				ViewCount: l.ViewCount,
				URL:       "/share/" + l.Token,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			slog.Error("encode share list response", slog.Any("err", err))
		}
	}
}

func shareRevokeHandler(shareSvc *appmedia.ShareService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := shareSvc.Revoke(r.Context(), id); err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func shareValidateHandler(shareSvc *appmedia.ShareService, mediaGetter mediaLibraryGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		link, err := shareSvc.Validate(r.Context(), token)
		if err != nil {
			if strings.Contains(err.Error(), "expired") || strings.Contains(err.Error(), "view limit") {
				http.Error(w, `{"message":"link expired or view limit reached"}`, http.StatusGone)
				return
			}
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}

		item, err := mediaGetter.GetMediaItem(r.Context(), link.MediaItemID)
		if err != nil || item == nil {
			http.Error(w, `{"message":"media not found"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"mediaId":    item.ID,
			"title":      item.Title,
			"mediaType":  item.Type,
			"durationMs": item.DurationMs,
			"streamUrl":  "/api/v1/media/" + item.ID + "/stream",
		}); err != nil {
			slog.Error("encode share validate response", slog.Any("err", err))
		}
	}
}

func claimsFromContext(ctx context.Context) (*domainuser.Claims, bool) {
	// requireAdmin injects claims via context — reuse that pattern
	// For now, extract from the admin middleware's context key
	claims, ok := ctx.Value(adminClaimsKey).(*domainuser.Claims)
	return claims, ok
}
