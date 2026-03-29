package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	domainmeta "github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

func registerMetadataOverrideRoutes(mux *http.ServeMux, metaCmd *appmetadata.CommandService, metaQuery *appmetadata.QueryService, tokenGen *auth.JWTGenerator) {
	requireAdminLocal := func(r *http.Request) bool {
		claims, ok := extractClaims(r, tokenGen)
		return ok && claims.IsAdmin
	}

	mux.HandleFunc("PUT /api/v1/media/{id}/metadata", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		mediaItemID := r.PathValue("id")
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		allowedFields := map[string]bool{
			"title": true, "overview": true, "genres": true,
			"year": true, "community_rating": true, "official_rating": true,
			"imdb_id": true, "tmdb_id": true,
		}
		for fieldName, value := range body {
			if !allowedFields[fieldName] || value == "" {
				continue
			}
			override := &domainmeta.MetadataOverride{
				ID:          id.New("mo-"),
				MediaItemID: mediaItemID,
				FieldName:   fieldName,
				CustomValue: value,
			}
			if err := metaCmd.SetOverride(r.Context(), override); err != nil {
				slog.Error("set override", slog.Any("err", err))
				http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
			slog.Warn("write failed", slog.Any("err", err))
		}
	})

	mux.HandleFunc("DELETE /api/v1/media/{id}/metadata", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if err := metaCmd.ResetOverrides(r.Context(), r.PathValue("id")); err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
			slog.Warn("write failed", slog.Any("err", err))
		}
	})

	mux.HandleFunc("GET /api/v1/media/{id}/metadata/overrides", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		overrides, err := metaQuery.ListOverrides(r.Context(), r.PathValue("id"))
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		type item struct {
			ID          string `json:"id"`
			FieldName   string `json:"field_name"`
			CustomValue any    `json:"custom_value"`
			CreatedAt   string `json:"created_at"`
		}
		result := make([]item, 0, len(overrides))
		for _, o := range overrides {
			result = append(result, item{
				ID: o.ID, FieldName: o.FieldName, CustomValue: o.CustomValue,
				CreatedAt: o.CreatedAt.UTC().Format(time.RFC3339),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"overrides": result}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	})
}
