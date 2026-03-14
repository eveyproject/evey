package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	appbackup "github.com/gravity-zero/evey/internal/app/backup"
	appcollection "github.com/gravity-zero/evey/internal/app/collection"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	domainshared "github.com/gravity-zero/evey/internal/domain/shared"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/protobuf/encoding/protojson"
	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	appfavorite "github.com/gravity-zero/evey/internal/app/favorite"
	appplaylist "github.com/gravity-zero/evey/internal/app/playlist"
	domainplaylist "github.com/gravity-zero/evey/internal/domain/playlist"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	domainmeta "github.com/gravity-zero/evey/internal/domain/metadata"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/config"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/id"
	"github.com/gravity-zero/evey/internal/pkg/network"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
)

func registerAdminRoutes(mux *http.ServeMux, metaCmd *appmetadata.CommandService, metaQuery *appmetadata.QueryService, tokenGen *auth.JWTGenerator, heavyRL *ratelimit.Limiter) {
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
			if !heavyRL.Allow(ratelimit.ClientIP(r)) {
				http.Error(w, `{"message":"too many requests"}`, http.StatusTooManyRequests)
				return
			}
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
				refreshErr = metaCmd.RefreshItemMetadataByIMDB(r.Context(), mediaItemID, body.ImdbID, body.MediaType)
			} else {
				refreshErr = metaCmd.RefreshItemMetadata(r.Context(), mediaItemID, body.TmdbID, body.MediaType)
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
			results, err := metaQuery.SearchMetadata(r.Context(), query, mediaType)
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

	mux.HandleFunc("POST /api/v1/admin/metadata/refresh-all", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		if !heavyRL.Allow(ratelimit.ClientIP(r)) {
			http.Error(w, `{"message":"too many requests"}`, http.StatusTooManyRequests)
			return
		}
		go func() {
			if err := metaCmd.AutoRefreshAllItems(context.Background()); err != nil {
				slog.Error("bulk metadata refresh", slog.Any("err", err))
			}
		}()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"message": "refresh started"})
	})
}

func registerGateway(grpcAddr string) http.Handler {
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("grpc-gateway dial failed", slog.Any("err", err))
		return nil
	}
	ctx := context.Background()
	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{EmitUnpopulated: true},
		}),
	)
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
		eveyv1.RegisterCollectionServiceHandler,
		eveyv1.RegisterPlaylistServiceHandler,
		eveyv1.RegisterFavoriteServiceHandler,
		eveyv1.RegisterTranscodeCacheServiceHandler,
		eveyv1.RegisterBackupServiceHandler,
		eveyv1.RegisterRemoteAccessServiceHandler,
		eveyv1.RegisterPlaybackSettingsServiceHandler,
		eveyv1.RegisterLibraryAccessServiceHandler,
		eveyv1.RegisterServerConfigServiceHandler,
	} {
		if err := fn(ctx, mux, conn); err != nil {
			slog.Error("gateway register failed", slog.Any("err", err))
		}
	}
	return mux
}

func registerFavoriteRoutes(mux *http.ServeMux, favSvc *appfavorite.Service, libQuery *applibrary.QueryService, tokenGen *auth.JWTGenerator) {
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
			MediaItemID     string   `json:"mediaItemId"`
			CreatedAt       string   `json:"createdAt"`
			Title           string   `json:"title,omitempty"`
			Type            string   `json:"type,omitempty"`
			Year            *int     `json:"year,omitempty"`
			CommunityRating *float64 `json:"communityRating,omitempty"`
		}
		items := make([]favItem, len(favs))
		for i, f := range favs {
			items[i] = favItem{MediaItemID: f.MediaItemID, CreatedAt: f.CreatedAt.UTC().Format(time.RFC3339)}
		}
		// batch-fetch media item details in parallel
		var wg sync.WaitGroup
		ctx := r.Context()
		for i, f := range favs {
			wg.Add(1)
			go func(idx int, mediaID string) {
				defer wg.Done()
				mi, ferr := libQuery.GetMediaItem(ctx, mediaID)
				if ferr != nil || mi == nil {
					return
				}
				items[idx].Title = mi.Title
				items[idx].Type = mi.Type
				items[idx].Year = mi.Year
				items[idx].CommunityRating = mi.CommunityRating
			}(i, f.MediaItemID)
		}
		wg.Wait()
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

func registerMetadataOverrideRoutes(mux *http.ServeMux, metaCmd *appmetadata.CommandService, metaQuery *appmetadata.QueryService, tokenGen *auth.JWTGenerator) {
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
			if err := metaCmd.SetOverride(r.Context(), override); err != nil {
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
		if err := metaCmd.ResetOverrides(r.Context(), r.PathValue("id")); err != nil {
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

func registerCollectionRoutes(mux *http.ServeMux, svc *appcollection.Service, tokenGen *auth.JWTGenerator) {
	requireAuth := func(r *http.Request) bool {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			return false
		}
		_, err := tokenGen.ValidateAccessToken(strings.TrimPrefix(h, "Bearer "))
		return err == nil
	}
	requireAdmin := func(r *http.Request) bool {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			return false
		}
		claims, err := tokenGen.ValidateAccessToken(strings.TrimPrefix(h, "Bearer "))
		return err == nil && claims.IsAdmin
	}

	mux.HandleFunc("GET /api/v1/collections", func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(r) {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		cols, err := svc.List(r.Context())
		if err != nil {
			http.Error(w, `{"message":"internal"}`, http.StatusInternalServerError)
			return
		}
		type colResp struct {
			ID          string  `json:"id"`
			Name        string  `json:"name"`
			Overview    string  `json:"overview,omitempty"`
			TmdbID      *int    `json:"tmdbId,omitempty"`
			PosterURL   string  `json:"posterUrl,omitempty"`
			BackdropURL string  `json:"backdropUrl,omitempty"`
			CreatedAt   string  `json:"createdAt"`
			UpdatedAt   string  `json:"updatedAt"`
		}
		out := make([]colResp, 0, len(cols))
		for _, c := range cols {
			out = append(out, colResp{
				ID: c.ID, Name: c.Name, Overview: c.Overview,
				TmdbID: c.TmdbID, PosterURL: c.PosterURL, BackdropURL: c.BackdropURL,
				CreatedAt: c.CreatedAt.Format(time.RFC3339),
				UpdatedAt: c.UpdatedAt.Format(time.RFC3339),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"collections": out})
	})

	mux.HandleFunc("/api/v1/collections/", func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(r) {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/collections/")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.SplitN(id, "/", 2)
		id = parts[0]

		switch r.Method {
		case http.MethodGet:
			col, err := svc.GetByID(r.Context(), id)
			if err != nil {
				http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
				return
			}
			limit := 50
			offset := 0
			if l := r.URL.Query().Get("limit"); l != "" {
				if n, e := strconv.Atoi(l); e == nil && n > 0 && n <= 500 {
					limit = n
				}
			}
			if o := r.URL.Query().Get("offset"); o != "" {
				if n, e := strconv.Atoi(o); e == nil && n >= 0 {
					offset = n
				}
			}
			items, total, err := svc.GetItems(r.Context(), id, limit, offset)
			if err != nil {
				http.Error(w, `{"message":"internal"}`, http.StatusInternalServerError)
				return
			}
			type itemResp struct {
				ID              string   `json:"id"`
				Title           string   `json:"title"`
				Type            string   `json:"type"`
				Year            *int     `json:"year,omitempty"`
				Overview        string   `json:"overview,omitempty"`
				CommunityRating *float64 `json:"communityRating,omitempty"`
				Genres          []string `json:"genres,omitempty"`
			}
			mediaItems := make([]itemResp, 0, len(items))
			for _, m := range items {
				mediaItems = append(mediaItems, itemResp{
					ID: m.ID, Title: m.Title, Type: m.Type,
					Year: m.Year, Overview: m.Overview,
					CommunityRating: m.CommunityRating, Genres: m.Genres,
				})
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"collection": map[string]any{
					"id": col.ID, "name": col.Name, "overview": col.Overview,
					"tmdbId": col.TmdbID, "posterUrl": col.PosterURL, "backdropUrl": col.BackdropURL,
					"createdAt": col.CreatedAt.Format(time.RFC3339),
					"updatedAt": col.UpdatedAt.Format(time.RFC3339),
				},
				"items": mediaItems,
				"total": total,
			})

		case http.MethodDelete:
			if !requireAdmin(r) {
				http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
				return
			}
			if err := svc.Delete(r.Context(), id); err != nil {
				http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			http.Error(w, `{"message":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})
}

func registerScheduleRoutes(mux *http.ServeMux, scheduler *applibrary.Scheduler, tokenGen *auth.JWTGenerator) {
	requireAdmin := func(r *http.Request) bool {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			return false
		}
		claims, err := tokenGen.ValidateAccessToken(strings.TrimPrefix(h, "Bearer "))
		return err == nil && claims.IsAdmin
	}

	mux.HandleFunc("GET /api/v1/libraries/{id}/schedule", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		sched, err := scheduler.GetSchedule(r.Context(), r.PathValue("id"))
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"schedule":null}`))
			return
		}
		type resp struct {
			LibraryID string  `json:"libraryId"`
			Cron      string  `json:"cron"`
			Enabled   bool    `json:"enabled"`
			LastRunAt *string `json:"lastRunAt"`
			NextRunAt *string `json:"nextRunAt"`
		}
		out := resp{
			LibraryID: sched.LibraryID,
			Cron:      sched.CronExpression,
			Enabled:   sched.Enabled,
		}
		if sched.LastRunAt != nil {
			s := sched.LastRunAt.UTC().Format(time.RFC3339)
			out.LastRunAt = &s
		}
		if sched.NextRunAt != nil {
			s := sched.NextRunAt.UTC().Format(time.RFC3339)
			out.NextRunAt = &s
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"schedule": out})
	})

	mux.HandleFunc("PUT /api/v1/libraries/{id}/schedule", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		var body struct {
			Cron    string `json:"cron"`
			Enabled *bool  `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		if body.Cron == "" {
			http.Error(w, `{"message":"cron expression required"}`, http.StatusBadRequest)
			return
		}
		enabled := true
		if body.Enabled != nil {
			enabled = *body.Enabled
		}
		sched, err := scheduler.SetSchedule(r.Context(), r.PathValue("id"), body.Cron, enabled)
		if err != nil {
			if strings.HasPrefix(err.Error(), "invalid cron") {
				http.Error(w, `{"message":"invalid cron expression"}`, http.StatusBadRequest)
			} else {
				slog.Error("set schedule failed", slog.Any("err", err))
				http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			}
			return
		}
		type resp struct {
			LibraryID string  `json:"libraryId"`
			Cron      string  `json:"cron"`
			Enabled   bool    `json:"enabled"`
			NextRunAt *string `json:"nextRunAt"`
		}
		out := resp{LibraryID: sched.LibraryID, Cron: sched.CronExpression, Enabled: sched.Enabled}
		if sched.NextRunAt != nil {
			s := sched.NextRunAt.UTC().Format(time.RFC3339)
			out.NextRunAt = &s
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"schedule": out})
	})

	mux.HandleFunc("DELETE /api/v1/libraries/{id}/schedule", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		if err := scheduler.DeleteSchedule(r.Context(), r.PathValue("id")); err != nil {
			slog.Warn("delete schedule", slog.Any("err", err))
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func registerLibraryAccessRoutes(mux *http.ServeMux, accessRepo domainuser.LibraryAccessRepository, libQuery *applibrary.QueryService, tokenGen *auth.JWTGenerator) {
	requireAdmin := func(r *http.Request) bool {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			return false
		}
		claims, err := tokenGen.ValidateAccessToken(strings.TrimPrefix(h, "Bearer "))
		return err == nil && claims.IsAdmin
	}

	mux.HandleFunc("GET /api/v1/admin/users/{userId}/library-access", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		userID := r.PathValue("userId")
		ids, err := accessRepo.GetByUserID(r.Context(), userID)
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		libs, libErr := libQuery.ListLibraries(r.Context())
		if libErr != nil {
			slog.Warn("list libraries for access", slog.Any("err", libErr))
		}
		type libInfo struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		allLibs := make([]libInfo, 0, len(libs))
		for _, l := range libs {
			allLibs = append(allLibs, libInfo{ID: l.ID, Name: l.Name})
		}
		if ids == nil {
			ids = []string{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"libraryIds": ids,
			"libraries":  allLibs,
		})
	})

	mux.HandleFunc("PUT /api/v1/admin/users/{userId}/library-access", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		userID := r.PathValue("userId")
		var body struct {
			LibraryIDs []string `json:"libraryIds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		existing, _ := accessRepo.GetByUserID(r.Context(), userID) //nolint:errcheck
		for _, id := range existing {
			if rErr := accessRepo.Revoke(r.Context(), userID, id); rErr != nil {
				slog.Warn("revoke library access", slog.Any("err", rErr))
			}
		}
		for _, id := range body.LibraryIDs {
			if err := accessRepo.Grant(r.Context(), userID, id); err != nil {
				http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("DELETE /api/v1/admin/users/{userId}/library-access", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		userID := r.PathValue("userId")
		existing, _ := accessRepo.GetByUserID(r.Context(), userID) //nolint:errcheck
		for _, id := range existing {
			if rErr := accessRepo.Revoke(r.Context(), userID, id); rErr != nil {
				slog.Warn("revoke library access", slog.Any("err", rErr))
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func registerPlaylistRoutes(mux *http.ServeMux, svc *appplaylist.Service, tokenGen *auth.JWTGenerator) {
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

	mux.HandleFunc("GET /api/v1/playlists", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		pls, err := svc.List(r.Context(), claims.UserID)
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		type plResp struct {
			ID          string                `json:"id"`
			Name        string                `json:"name"`
			Description string                `json:"description,omitempty"`
			Type        string                `json:"type"`
			Rules       []domainplaylist.Rule  `json:"rules"`
			SortBy      string                `json:"sortBy"`
			SortOrder   string                `json:"sortOrder"`
			CreatedAt   string                `json:"createdAt"`
			UpdatedAt   string                `json:"updatedAt"`
		}
		out := make([]plResp, 0, len(pls))
		for _, p := range pls {
			rules := p.Rules
			if rules == nil {
				rules = []domainplaylist.Rule{}
			}
			out = append(out, plResp{
				ID: p.ID, Name: p.Name, Description: p.Description,
				Type: string(p.Type), Rules: rules,
				SortBy: p.SortBy, SortOrder: p.SortOrder,
				CreatedAt: p.CreatedAt.Format(time.RFC3339),
				UpdatedAt: p.UpdatedAt.Format(time.RFC3339),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"playlists": out})
	})

	mux.HandleFunc("POST /api/v1/playlists", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		var body struct {
			Name        string                `json:"name"`
			Description string                `json:"description"`
			Type        string                `json:"type"`
			Rules       []domainplaylist.Rule  `json:"rules"`
			SortBy      string                `json:"sortBy"`
			SortOrder   string                `json:"sortOrder"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		if body.Name == "" {
			http.Error(w, `{"message":"name required"}`, http.StatusBadRequest)
			return
		}
		pl := &domainplaylist.Playlist{
			Name:        body.Name,
			Description: body.Description,
			UserID:      claims.UserID,
			Type:        domainplaylist.PlaylistType(body.Type),
			Rules:       body.Rules,
			SortBy:      body.SortBy,
			SortOrder:   body.SortOrder,
		}
		if pl.Type == "" {
			pl.Type = domainplaylist.Manual
		}
		if err := svc.Create(r.Context(), pl); err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": pl.ID})
	})

	mux.HandleFunc("/api/v1/playlists/", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/playlists/")
		parts := strings.SplitN(path, "/", 2)
		plID := parts[0]
		if plID == "" {
			http.NotFound(w, r)
			return
		}
		action := ""
		if len(parts) > 1 {
			action = parts[1]
		}

		switch {
		case action == "" && r.Method == http.MethodGet:
			pl, err := svc.GetByID(r.Context(), plID, claims.UserID)
			if err != nil {
				http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
				return
			}
			mediaItems, plItems, err := svc.GetItems(r.Context(), plID, claims.UserID)
			if err != nil {
				http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
				return
			}
			rules := pl.Rules
			if rules == nil {
				rules = []domainplaylist.Rule{}
			}
			type itemResp struct {
				MediaItemID string `json:"mediaItemId"`
				SortOrder   int    `json:"sortOrder"`
				AddedAt     string `json:"addedAt"`
			}
			type mediaResp struct {
				ID    string `json:"id"`
				Title string `json:"title"`
				Type  string `json:"type"`
				Year  *int   `json:"year,omitempty"`
			}
			var items []itemResp
			var media []mediaResp
			for _, it := range plItems {
				items = append(items, itemResp{
					MediaItemID: it.MediaItemID,
					SortOrder:   it.SortOrder,
					AddedAt:     it.AddedAt.Format(time.RFC3339),
				})
			}
			for _, m := range mediaItems {
				media = append(media, mediaResp{
					ID: m.ID, Title: m.Title, Type: m.Type, Year: m.Year,
				})
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"playlist": map[string]any{
					"id": pl.ID, "name": pl.Name, "description": pl.Description,
					"type": string(pl.Type), "rules": rules,
					"sortBy": pl.SortBy, "sortOrder": pl.SortOrder,
					"createdAt": pl.CreatedAt.Format(time.RFC3339),
					"updatedAt": pl.UpdatedAt.Format(time.RFC3339),
				},
				"items": items,
				"media": media,
			})

		case action == "" && r.Method == http.MethodPut:
			var body struct {
				Name        string                `json:"name"`
				Description string                `json:"description"`
				Rules       []domainplaylist.Rule  `json:"rules"`
				SortBy      string                `json:"sortBy"`
				SortOrder   string                `json:"sortOrder"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
				return
			}
			pl := &domainplaylist.Playlist{
				ID:          plID,
				Name:        body.Name,
				Description: body.Description,
				Rules:       body.Rules,
				SortBy:      body.SortBy,
				SortOrder:   body.SortOrder,
			}
			if err := svc.Update(r.Context(), pl, claims.UserID); err != nil {
				http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		case action == "" && r.Method == http.MethodDelete:
			if err := svc.Delete(r.Context(), plID, claims.UserID); err != nil {
				http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		case action == "items" && r.Method == http.MethodPost:
			var body struct {
				MediaItemID string `json:"mediaItemId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
				return
			}
			if err := svc.AddItem(r.Context(), plID, body.MediaItemID, claims.UserID); err != nil {
				switch {
				case errors.Is(err, appplaylist.ErrNotOwner):
					http.Error(w, `{"message":"not playlist owner"}`, http.StatusForbidden)
				case errors.Is(err, appplaylist.ErrSmartItems):
					http.Error(w, `{"message":"cannot manually add items to smart playlist"}`, http.StatusBadRequest)
				case errors.Is(err, appplaylist.ErrMaxItems):
					http.Error(w, `{"message":"playlist item limit reached"}`, http.StatusBadRequest)
				default:
					slog.Error("add playlist item failed", slog.Any("err", err))
					http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
				}
				return
			}
			w.WriteHeader(http.StatusNoContent)

		case strings.HasPrefix(action, "items/") && r.Method == http.MethodDelete:
			mediaItemID := strings.TrimPrefix(action, "items/")
			if err := svc.RemoveItem(r.Context(), plID, mediaItemID, claims.UserID); err != nil {
				http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		case action == "reorder" && r.Method == http.MethodPut:
			var body struct {
				MediaItemIDs []string `json:"mediaItemIds"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
				return
			}
			if err := svc.ReorderItems(r.Context(), plID, claims.UserID, body.MediaItemIDs); err != nil {
				http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			http.Error(w, `{"message":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})
}

func registerBackupRoutes(mux *http.ServeMux, svc *appbackup.Service, tokenGen *auth.JWTGenerator, heavyRL *ratelimit.Limiter) {
	requireAdmin := func(r *http.Request) bool {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			return false
		}
		claims, err := tokenGen.ValidateAccessToken(strings.TrimPrefix(h, "Bearer "))
		return err == nil && claims.IsAdmin
	}

	mux.HandleFunc("POST /api/v1/admin/backups", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		m, err := svc.CreateBackup(r.Context())
		if err != nil {
			slog.Error("create backup", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id":        m.ID,
			"createdAt": m.CreatedAt.Format(time.RFC3339),
			"version":   m.Version,
			"tables":    m.Tables,
			"size":      m.Size,
		})
	})

	mux.HandleFunc("GET /api/v1/admin/backups", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		list, err := svc.ListBackups(r.Context())
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		type item struct {
			ID        string `json:"id"`
			CreatedAt string `json:"createdAt"`
			Version   string `json:"version"`
			Size      int64  `json:"size"`
		}
		items := make([]item, 0, len(list))
		for _, m := range list {
			items = append(items, item{
				ID:        m.ID,
				CreatedAt: m.CreatedAt.Format(time.RFC3339),
				Version:   m.Version,
				Size:      m.Size,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"backups": items})
	})

	mux.HandleFunc("GET /api/v1/admin/backups/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		backupID := r.PathValue("id")
		m, rc, err := svc.GetBackup(r.Context(), backupID)
		if err != nil {
			http.Error(w, `{"message":"backup not found"}`, http.StatusNotFound)
			return
		}
		defer rc.Close()
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.gz"`, m.ID))
		if _, err := io.Copy(w, rc); err != nil {
			slog.Warn("backup download copy", slog.Any("err", err))
		}
	})

	mux.HandleFunc("POST /api/v1/admin/backups/restore", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		if !heavyRL.Allow(ratelimit.ClientIP(r)) {
			http.Error(w, `{"message":"too many requests"}`, http.StatusTooManyRequests)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2<<30) // 2GB max
		file, _, err := r.FormFile("backup")
		if err != nil {
			http.Error(w, `{"message":"backup file required"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()
		if err := svc.RestoreBackup(r.Context(), file); err != nil {
			slog.Error("restore backup", slog.Any("err", err))
			http.Error(w, `{"message":"restore failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})

	mux.HandleFunc("DELETE /api/v1/admin/backups/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		backupID := r.PathValue("id")
		if err := svc.DeleteBackup(r.Context(), backupID); err != nil {
			http.Error(w, `{"message":"backup not found"}`, http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func networkMiddleware(detector *network.Detector, remoteEnabled *bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}
			nt := detector.Detect(r)
			if nt == network.WAN && !*remoteEnabled {
				http.Error(w, `{"message":"remote access disabled"}`, http.StatusForbidden)
				return
			}
			w.Header().Set("X-Evey-Network", string(nt))
			ctx := network.WithNetworkType(r.Context(), nt)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

const remoteAccessConfigKey = "remote_access"

type remoteAccessPersister interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

func registerRemoteAccessRoutes(mux *http.ServeMux, cfg *config.Config, tokenGen *auth.JWTGenerator, store remoteAccessPersister) {
	var mu sync.RWMutex

	requireAdmin := func(r *http.Request) bool {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			return false
		}
		claims, err := tokenGen.ValidateAccessToken(strings.TrimPrefix(h, "Bearer "))
		return err == nil && claims.IsAdmin
	}

	encodeRA := func(w http.ResponseWriter) {
		mu.RLock()
		defer mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"enabled":              cfg.RemoteAccess.Enabled,
			"externalUrl":          cfg.RemoteAccess.ExternalURL,
			"lanNetworks":          cfg.RemoteAccess.LANNetworks,
			"maxRemoteBitrateKbps": cfg.RemoteAccess.MaxRemoteBitrateKbps,
		})
	}

	mux.HandleFunc("GET /api/v1/admin/remote-access", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		encodeRA(w)
	})

	mux.HandleFunc("PUT /api/v1/admin/remote-access", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		var body struct {
			Enabled              *bool    `json:"enabled"`
			ExternalURL          *string  `json:"externalUrl"`
			LANNetworks          []string `json:"lanNetworks"`
			MaxRemoteBitrateKbps *int     `json:"maxRemoteBitrateKbps"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		mu.Lock()
		if body.Enabled != nil {
			cfg.RemoteAccess.Enabled = *body.Enabled
		}
		if body.ExternalURL != nil {
			u, err := url.Parse(*body.ExternalURL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				mu.Unlock()
				http.Error(w, `{"message":"invalid externalUrl: must be http/https URL"}`, http.StatusBadRequest)
				return
			}
			cfg.RemoteAccess.ExternalURL = *body.ExternalURL
		}
		if body.LANNetworks != nil {
			cfg.RemoteAccess.LANNetworks = body.LANNetworks
		}
		if body.MaxRemoteBitrateKbps != nil {
			cfg.RemoteAccess.MaxRemoteBitrateKbps = *body.MaxRemoteBitrateKbps
		}
		snapshot := cfg.RemoteAccess
		mu.Unlock()

		if store != nil {
			if data, err := json.Marshal(snapshot); err == nil {
				if err2 := store.Set(r.Context(), remoteAccessConfigKey, string(data)); err2 != nil {
					slog.Warn("remote-access: persist config failed", slog.Any("err", err2))
				}
			}
		}
		encodeRA(w)
	})

	mux.HandleFunc("GET /api/v1/admin/network-info", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		ifaces, err := network.NetworkInterfaces()
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"interfaces": ifaces})
	})
}

func registerPlaybackSettingsRoutes(mux *http.ServeMux, cfg *config.Config, tokenGen *auth.JWTGenerator) {
	requireAuth := func(r *http.Request) bool {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			return false
		}
		_, err := tokenGen.ValidateAccessToken(strings.TrimPrefix(h, "Bearer "))
		return err == nil
	}
	requireAdmin := func(r *http.Request) bool {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			return false
		}
		claims, err := tokenGen.ValidateAccessToken(strings.TrimPrefix(h, "Bearer "))
		return err == nil && claims.IsAdmin
	}

	mux.HandleFunc("GET /api/v1/playback-settings", func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(r) {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"skipForwardSeconds":  cfg.Playback.SkipForwardSeconds,
			"skipBackwardSeconds": cfg.Playback.SkipBackwardSeconds,
		})
	})

	mux.HandleFunc("PUT /api/v1/admin/playback-settings", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		var body struct {
			SkipForwardSeconds  *int `json:"skipForwardSeconds"`
			SkipBackwardSeconds *int `json:"skipBackwardSeconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		if body.SkipForwardSeconds != nil && *body.SkipForwardSeconds > 0 {
			cfg.Playback.SkipForwardSeconds = *body.SkipForwardSeconds
		}
		if body.SkipBackwardSeconds != nil && *body.SkipBackwardSeconds > 0 {
			cfg.Playback.SkipBackwardSeconds = *body.SkipBackwardSeconds
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"skipForwardSeconds":  cfg.Playback.SkipForwardSeconds,
			"skipBackwardSeconds": cfg.Playback.SkipBackwardSeconds,
		})
	})
}

const refreshCookieName = "evey_refresh"
const refreshCookieMaxAge = 30 * 24 * 3600 // 30 days

// authCookieMiddleware wraps a handler: after the inner handler responds with an AuthResponse,
// it extracts refreshToken from the JSON body, sets an httpOnly cookie, and strips it from the body.
func authCookieMiddleware(next http.Handler, httpsOnly bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &responseRecorder{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rec, r)

		ct := rec.header.Get("Content-Type")
		if rec.code < 200 || rec.code >= 300 || !strings.Contains(ct, "application/json") {
			rec.flush(w)
			return
		}

		var body map[string]json.RawMessage
		if err := json.Unmarshal(rec.buf.Bytes(), &body); err != nil {
			rec.flush(w)
			return
		}

		rt, ok := body["refreshToken"]
		if !ok {
			rec.flush(w)
			return
		}

		var rtStr string
		if err := json.Unmarshal(rt, &rtStr); err != nil || rtStr == "" {
			rec.flush(w)
			return
		}

		delete(body, "refreshToken")
		http.SetCookie(w, &http.Cookie{
			Name:     refreshCookieName,
			Value:    rtStr,
			Path:     "/api/v1/auth/",
			HttpOnly: true,
			Secure:   httpsOnly,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   refreshCookieMaxAge,
		})

		newBody, err := json.Marshal(body)
		if err != nil {
			rec.flush(w)
			return
		}
		for k, v := range rec.header {
			w.Header()[k] = v
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rec.code)
		w.Write(newBody)
	})
}

// injectRefreshTokenFromCookie reads the evey_refresh cookie and injects its value
// into the request body if refreshToken is not already present.
func injectRefreshTokenFromCookie(r *http.Request) *http.Request {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil || cookie.Value == "" {
		return r
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return r
	}
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	var m map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &m); err != nil {
		m = make(map[string]json.RawMessage)
	}
	if _, exists := m["refreshToken"]; !exists {
		quoted, _ := json.Marshal(cookie.Value)
		m["refreshToken"] = quoted
	}
	merged, err := json.Marshal(m)
	if err != nil {
		return r
	}
	r2 := r.Clone(r.Context())
	r2.Body = io.NopCloser(bytes.NewReader(merged))
	r2.ContentLength = int64(len(merged))
	return r2
}

// clearRefreshCookie deletes the httpOnly refresh cookie.
func clearRefreshCookie(w http.ResponseWriter, httpsOnly bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/api/v1/auth/",
		HttpOnly: true,
		Secure:   httpsOnly,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// responseRecorder buffers the response from an inner handler.
type responseRecorder struct {
	http.ResponseWriter
	code   int
	buf    bytes.Buffer
	header http.Header
}

func (r *responseRecorder) Header() http.Header {
	if r.header == nil {
		r.header = make(http.Header)
	}
	return r.header
}

func (r *responseRecorder) WriteHeader(code int) { r.code = code }

func (r *responseRecorder) Write(b []byte) (int, error) { return r.buf.Write(b) }

func (r *responseRecorder) flush(w http.ResponseWriter) {
	for k, v := range r.header {
		w.Header()[k] = v
	}
	w.WriteHeader(r.code)
	w.Write(r.buf.Bytes())
}

func registerRatingRoutes(mux *http.ServeMux, ratingRepo domainlibrary.UserRatingRepository, tokenGen *auth.JWTGenerator) {
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

	// PUT /api/v1/media/{id}/rate — upsert rating; value: -1 dislike, 0 remove, 1 like
	mux.HandleFunc("PUT /api/v1/media/{id}/rate", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		var body struct {
			Value int `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		if body.Value < -1 || body.Value > 1 {
			http.Error(w, `{"message":"value must be -1, 0, or 1"}`, http.StatusBadRequest)
			return
		}
		mediaID := r.PathValue("id")
		rating := &domainlibrary.UserRating{
			ProfileID:   claims.ProfileID,
			MediaItemID: mediaID,
			Value:       body.Value,
		}
		if err := ratingRepo.Upsert(r.Context(), rating); err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"value": body.Value})
	})

	// GET /api/v1/media/{id}/my-rating — get current user's rating
	mux.HandleFunc("GET /api/v1/media/{id}/my-rating", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		mediaID := r.PathValue("id")
		rating, err := ratingRepo.Get(r.Context(), claims.ProfileID, mediaID)
		if err != nil {
			// no rating yet → return neutral
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"value": 0})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"value": rating.Value})
	})
}

func registerPopularRoute(mux *http.ServeMux, activityQuery *appactivity.QueryService, libQuery *applibrary.QueryService, tokenGen *auth.JWTGenerator) {
	mux.HandleFunc("GET /api/v1/popular", func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		if _, err := tokenGen.ValidateAccessToken(h[7:]); err != nil {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		limit := 20
		popular, err := activityQuery.GetMostPopular(r.Context(), limit)
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		type popItem struct {
			MediaItemID string `json:"mediaItemId"`
			Title       string `json:"title"`
			Type        string `json:"type,omitempty"`
			PlayCount   int    `json:"playCount"`
		}
		items := make([]popItem, 0, len(popular))
		// batch-fetch types in parallel
		results := make([]popItem, len(popular))
		for i, p := range popular {
			results[i] = popItem{MediaItemID: p.MediaItemID, Title: p.Title, PlayCount: p.PlayCount}
		}
		var wg sync.WaitGroup
		ctx := r.Context()
		for i, p := range popular {
			wg.Add(1)
			go func(idx int, mediaID string) {
				defer wg.Done()
				mi, ferr := libQuery.GetMediaItem(ctx, mediaID)
				if ferr == nil && mi != nil {
					results[idx].Type = mi.Type
				}
			}(i, p.MediaItemID)
		}
		wg.Wait()
		items = append(items, results...)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	})
}

func registerSystemStatsRoute(mux *http.ServeMux, startTime time.Time, tokenGen *auth.JWTGenerator) {
	mux.HandleFunc("GET /api/v1/admin/system/stats", func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		claims, err := tokenGen.ValidateAccessToken(strings.TrimPrefix(h, "Bearer "))
		if err != nil || !claims.IsAdmin {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}

		var ms goruntime.MemStats
		goruntime.ReadMemStats(&ms)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"goroutines":    goruntime.NumGoroutine(),
			"memAllocBytes": ms.Alloc,
			"memSysBytes":   ms.Sys,
			"numGC":         ms.NumGC,
			"uptimeSeconds": int64(time.Since(startTime).Seconds()),
			"goVersion":     goruntime.Version(),
			"os":            goruntime.GOOS,
			"arch":          goruntime.GOARCH,
			"numCPU":        goruntime.NumCPU(),
		})
	})
}

func registerServerConfigRoutes(mux *http.ServeMux, cfgRepo domainshared.ServerConfigRepository, tokenGen *auth.JWTGenerator) {
	requireAdmin := func(w http.ResponseWriter, r *http.Request) bool {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return false
		}
		claims, err := tokenGen.ValidateAccessToken(strings.TrimPrefix(h, "Bearer "))
		if err != nil || !claims.IsAdmin {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return false
		}
		return true
	}

	mux.HandleFunc("GET /api/v1/admin/server-config", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r) {
			return
		}
		entries, err := cfgRepo.List(r.Context())
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		type entry struct {
			Key       string    `json:"key"`
			Value     string    `json:"value"`
			UpdatedAt time.Time `json:"updatedAt"`
		}
		items := make([]entry, 0, len(entries))
		for _, e := range entries {
			items = append(items, entry{Key: e.Key, Value: e.Value, UpdatedAt: e.UpdatedAt})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"entries": items})
	})

	mux.HandleFunc("PUT /api/v1/admin/server-config/{key}", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r) {
			return
		}
		key := r.PathValue("key")
		var body struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"message":"invalid body"}`, http.StatusBadRequest)
			return
		}
		if err := cfgRepo.Set(r.Context(), key, body.Value); err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("DELETE /api/v1/admin/server-config/{key}", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r) {
			return
		}
		key := r.PathValue("key")
		if err := cfgRepo.Delete(r.Context(), key); err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func registerLogsRoute(mux *http.ServeMux, logDir string, tokenGen *auth.JWTGenerator) {
	mux.HandleFunc("GET /api/v1/admin/logs", func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		claims, err := tokenGen.ValidateAccessToken(strings.TrimPrefix(h, "Bearer "))
		if err != nil || !claims.IsAdmin {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}

		lines := 200
		if v := r.URL.Query().Get("lines"); v != "" {
			if n, err2 := strconv.Atoi(v); err2 == nil && n > 0 && n <= 2000 {
				lines = n
			}
		}

		logPath := logDir + "/evey.log"
		data, err := tailFile(logPath, lines)
		if err != nil {
			http.Error(w, `{"message":"log file not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"lines": data})
	})
}

// tailFile reads the last n lines from a file efficiently.
func tailFile(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	size := stat.Size()
	if size == 0 {
		return nil, nil
	}

	// Read up to 512KB from the end for efficiency
	readSize := int64(512 * 1024)
	if readSize > size {
		readSize = size
	}
	offset := size - readSize
	buf := make([]byte, readSize)
	if _, err := f.ReadAt(buf, offset); err != nil {
		return nil, err
	}

	rawLines := strings.Split(string(buf), "\n")
	// Remove trailing empty
	for len(rawLines) > 0 && rawLines[len(rawLines)-1] == "" {
		rawLines = rawLines[:len(rawLines)-1]
	}
	if len(rawLines) > n {
		rawLines = rawLines[len(rawLines)-n:]
	}
	return rawLines, nil
}
