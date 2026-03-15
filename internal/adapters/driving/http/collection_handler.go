package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	appcollection "github.com/gravity-zero/evey/internal/app/collection"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

type collectionHandler struct {
	svc      *appcollection.Service
	tokenGen *auth.JWTGenerator
}

func registerCollectionRoutes(mux *http.ServeMux, svc *appcollection.Service, tokenGen *auth.JWTGenerator) {
	h := &collectionHandler{svc: svc, tokenGen: tokenGen}
	mux.HandleFunc("GET /api/v1/collections", h.list)
	mux.HandleFunc("/api/v1/collections/", h.routeByID)
}

func (h *collectionHandler) requireAuth(r *http.Request) bool {
	hdr := r.Header.Get("Authorization")
	if !strings.HasPrefix(hdr, "Bearer ") {
		return false
	}
	_, err := h.tokenGen.ValidateAccessToken(strings.TrimPrefix(hdr, "Bearer "))
	return err == nil
}

func (h *collectionHandler) requireAdmin(r *http.Request) bool {
	hdr := r.Header.Get("Authorization")
	if !strings.HasPrefix(hdr, "Bearer ") {
		return false
	}
	claims, err := h.tokenGen.ValidateAccessToken(strings.TrimPrefix(hdr, "Bearer "))
	return err == nil && claims.IsAdmin
}

func (h *collectionHandler) list(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(r) {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	cols, err := h.svc.List(r.Context())
	if err != nil {
		http.Error(w, `{"message":"internal"}`, http.StatusInternalServerError)
		return
	}
	type colResp struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Overview    string `json:"overview,omitempty"`
		TmdbID      *int   `json:"tmdbId,omitempty"`
		PosterURL   string `json:"posterUrl,omitempty"`
		BackdropURL string `json:"backdropUrl,omitempty"`
		CreatedAt   string `json:"createdAt"`
		UpdatedAt   string `json:"updatedAt"`
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
	if err := json.NewEncoder(w).Encode(map[string]any{"collections": out}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *collectionHandler) routeByID(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(r) {
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
		h.getByID(w, r, id)
	case http.MethodDelete:
		h.delete(w, r, id)
	default:
		http.Error(w, `{"message":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (h *collectionHandler) getByID(w http.ResponseWriter, r *http.Request, id string) {
	col, err := h.svc.GetByID(r.Context(), id)
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
	items, total, err := h.svc.GetItems(r.Context(), id, limit, offset)
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
	type ruleResp struct {
		Field    string `json:"field"`
		Operator string `json:"operator"`
		Value    string `json:"value"`
	}
	rules := make([]ruleResp, 0, len(col.Rules))
	for _, r := range col.Rules {
		rules = append(rules, ruleResp{Field: r.Field, Operator: r.Operator, Value: r.Value})
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"collection": map[string]any{
			"id": col.ID, "name": col.Name, "overview": col.Overview,
			"tmdbId": col.TmdbID, "posterUrl": col.PosterURL, "backdropUrl": col.BackdropURL,
			"rules": rules, "sortBy": col.SortBy,
			"createdAt": col.CreatedAt.Format(time.RFC3339),
			"updatedAt": col.UpdatedAt.Format(time.RFC3339),
		},
		"items": mediaItems,
		"total": total,
	}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *collectionHandler) delete(w http.ResponseWriter, r *http.Request, id string) {
	if !h.requireAdmin(r) {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
