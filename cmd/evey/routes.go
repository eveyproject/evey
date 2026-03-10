package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	appfavorite "github.com/gravity-zero/evey/internal/app/favorite"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	domainmeta "github.com/gravity-zero/evey/internal/domain/metadata"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

func registerAdminRoutes(mux *http.ServeMux, metaSvc *appmetadata.Service, tokenGen *auth.JWTGenerator) {
	requireAdmin := func(r *http.Request) bool {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			return false
		}
		claims, err := tokenGen.ValidateAccessToken(strings.TrimPrefix(h, "Bearer "))
		return err == nil && claims.IsAdmin
	}

	mux.HandleFunc("/api/v1/admin/media/", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/media/"), "/")
		if len(parts) < 2 {
			http.NotFound(w, r)
			return
		}
		mediaItemID := parts[0]
		action := parts[1]

		switch {
		case action == "refresh-metadata" && r.Method == http.MethodPost:
			var body struct {
				TmdbID    string `json:"tmdb_id"`
				ImdbID    string `json:"imdb_id"`
				MediaType string `json:"media_type"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			var refreshErr error
			if body.ImdbID != "" {
				refreshErr = metaSvc.RefreshItemMetadataByIMDB(r.Context(), mediaItemID, body.ImdbID, body.MediaType)
			} else {
				refreshErr = metaSvc.RefreshItemMetadata(r.Context(), mediaItemID, body.TmdbID, body.MediaType)
			}
			if refreshErr != nil {
				slog.Error("refresh metadata", slog.Any("err", refreshErr))
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		case action == "search-metadata" && r.Method == http.MethodGet:
			query := r.URL.Query().Get("query")
			mediaType := r.URL.Query().Get("type")
			if query == "" || mediaType == "" {
				http.Error(w, "query and type required", http.StatusBadRequest)
				return
			}
			results, err := metaSvc.SearchMetadata(r.Context(), query, mediaType)
			if err != nil {
				slog.Error("search metadata", slog.Any("err", err))
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			type searchResult struct {
				TmdbID    string  `json:"tmdb_id"`
				Title     string  `json:"title"`
				Year      *int    `json:"year,omitempty"`
				Overview  string  `json:"overview"`
				PosterURL string  `json:"poster_url,omitempty"`
			}
			out := make([]searchResult, 0, len(results))
			for _, r := range results {
				sr := searchResult{
					Title:    r.Title,
					Year:     r.Year,
					Overview: r.Overview,
				}
				if tmdbID, ok := r.ProviderIDs["tmdb"]; ok {
					sr.TmdbID = tmdbID
				}
				for _, art := range r.Artwork {
					if art.Type == "poster" {
						sr.PosterURL = art.URL
						break
					}
				}
				out = append(out, sr)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(out)

		default:
			http.NotFound(w, r)
		}
	})
}

func registerGateway(grpcAddr string) http.Handler {
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("grpc-gateway dial failed", slog.Any("err", err))
		return nil
	}
	ctx := context.Background()
	mux := runtime.NewServeMux()
	for _, fn := range []func(context.Context, *runtime.ServeMux, *grpc.ClientConn) error{
		eveyv1.RegisterLibraryServiceHandler,
		eveyv1.RegisterUserServiceHandler,
		eveyv1.RegisterPlaybackServiceHandler,
		eveyv1.RegisterTranscodingServiceHandler,
		eveyv1.RegisterMKVServiceHandler,
		eveyv1.RegisterMetadataServiceHandler,
		eveyv1.RegisterAdminServiceHandler,
		eveyv1.RegisterActivityServiceHandler,
		eveyv1.RegisterNotificationServiceHandler,
		eveyv1.RegisterFeedServiceHandler,
		eveyv1.RegisterWebhookServiceHandler,
	} {
		if err := fn(ctx, mux, conn); err != nil {
			slog.Error("gateway register failed", slog.Any("err", err))
		}
	}
	return mux
}

func registerFavoriteRoutes(mux *http.ServeMux, favSvc *appfavorite.Service, tokenGen *auth.JWTGenerator) {
	extractClaims := func(r *http.Request) (*domainuser.Claims, bool) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			return nil, false
		}
		claims, err := tokenGen.ValidateAccessToken(h[7:])
		if err != nil {
			return nil, false
		}
		return claims, true
	}

	mux.HandleFunc("POST /api/v1/favorites/{media_id}", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		mediaID := r.PathValue("media_id")
		isFav, err := favSvc.Toggle(r.Context(), claims.ProfileID, mediaID)
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"isFavorite": isFav})
	})

	mux.HandleFunc("GET /api/v1/favorites", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		limit := 50
		offset := 0
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 {
				limit = n
			}
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			if n, err := strconv.Atoi(o); err == nil && n >= 0 {
				offset = n
			}
		}
		favs, err := favSvc.List(r.Context(), claims.ProfileID, limit, offset)
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		type favItem struct {
			MediaItemID string `json:"mediaItemId"`
			CreatedAt   string `json:"createdAt"`
		}
		items := make([]favItem, 0, len(favs))
		for _, f := range favs {
			items = append(items, favItem{
				MediaItemID: f.MediaItemID,
				CreatedAt:   f.CreatedAt.UTC().Format(time.RFC3339),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	})

	mux.HandleFunc("GET /api/v1/favorites/{media_id}", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		mediaID := r.PathValue("media_id")
		isFav, err := favSvc.IsFavorite(r.Context(), claims.ProfileID, mediaID)
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"isFavorite": isFav})
	})
}

func registerMetadataOverrideRoutes(mux *http.ServeMux, metaSvc *appmetadata.Service, tokenGen *auth.JWTGenerator) {
	requireAdmin := func(r *http.Request) bool {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			return false
		}
		claims, err := tokenGen.ValidateAccessToken(strings.TrimPrefix(h, "Bearer "))
		return err == nil && claims.IsAdmin
	}

	mux.HandleFunc("PUT /api/v1/media/{id}/metadata", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
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
			if err := metaSvc.SetOverride(r.Context(), override); err != nil {
				slog.Error("set override", slog.Any("err", err))
				http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})

	mux.HandleFunc("DELETE /api/v1/media/{id}/metadata", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if err := metaSvc.ResetOverrides(r.Context(), r.PathValue("id")); err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})

	mux.HandleFunc("GET /api/v1/media/{id}/metadata/overrides", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		overrides, err := metaSvc.ListOverrides(r.Context(), r.PathValue("id"))
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
		json.NewEncoder(w).Encode(map[string]any{"overrides": result})
	})
}

func registerTranscodeCacheRoutes(mux *http.ServeMux, transcodeSvc *apptranscoding.Service, tokenGen *auth.JWTGenerator) {
	requireAdmin := func(r *http.Request) bool {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			return false
		}
		claims, err := tokenGen.ValidateAccessToken(strings.TrimPrefix(h, "Bearer "))
		return err == nil && claims.IsAdmin
	}

	mux.HandleFunc("GET /api/v1/admin/transcode-cache", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		info, err := transcodeSvc.GetCacheInfo(r.Context())
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		type fileEntry struct {
			Name         string `json:"name"`
			Size         int64  `json:"size"`
			LastModified string `json:"lastModified"`
		}
		resp := struct {
			TotalSize int64       `json:"totalSize"`
			FileCount int         `json:"fileCount"`
			Files     []fileEntry `json:"files"`
		}{TotalSize: info.TotalSize, FileCount: info.FileCount, Files: make([]fileEntry, 0, len(info.Files))}
		for _, f := range info.Files {
			resp.Files = append(resp.Files, fileEntry{Name: f.Name, Size: f.Size, LastModified: f.LastModified.UTC().Format(time.RFC3339)})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("DELETE /api/v1/admin/transcode-cache", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		if err := transcodeSvc.ClearCache(r.Context()); err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("DELETE /api/v1/admin/transcode-cache/{filename}", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		filename := r.PathValue("filename")
		if err := transcodeSvc.DeleteCacheFile(r.Context(), filename); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
				return
			}
			if strings.Contains(err.Error(), "invalid") {
				http.Error(w, `{"message":"invalid filename"}`, http.StatusBadRequest)
				return
			}
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
