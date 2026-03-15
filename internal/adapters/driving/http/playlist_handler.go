package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	appplaylist "github.com/gravity-zero/evey/internal/app/playlist"
	domainplaylist "github.com/gravity-zero/evey/internal/domain/playlist"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

type playlistHandler struct {
	svc      *appplaylist.Service
	tokenGen *auth.JWTGenerator
}

func registerPlaylistRoutes(mux *http.ServeMux, svc *appplaylist.Service, tokenGen *auth.JWTGenerator) {
	h := &playlistHandler{svc: svc, tokenGen: tokenGen}
	mux.HandleFunc("GET /api/v1/playlists", h.list)
	mux.HandleFunc("POST /api/v1/playlists", h.create)
	mux.HandleFunc("/api/v1/playlists/", h.routeByID)
}

func (h *playlistHandler) extractClaims(r *http.Request) (*domainuser.Claims, bool) {
	hdr := r.Header.Get("Authorization")
	if !strings.HasPrefix(hdr, "Bearer ") {
		return nil, false
	}
	claims, err := h.tokenGen.ValidateAccessToken(hdr[7:])
	if err != nil {
		return nil, false
	}
	return claims, true
}

func (h *playlistHandler) list(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.extractClaims(r)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	pls, err := h.svc.List(r.Context(), claims.UserID)
	if err != nil {
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	type plResp struct {
		ID          string                `json:"id"`
		Name        string                `json:"name"`
		Description string                `json:"description,omitempty"`
		Type        string                `json:"type"`
		Rules       []domainplaylist.Rule `json:"rules"`
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
	if err := json.NewEncoder(w).Encode(map[string]any{"playlists": out}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *playlistHandler) create(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.extractClaims(r)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		Name        string                `json:"name"`
		Description string                `json:"description"`
		Type        string                `json:"type"`
		Rules       []domainplaylist.Rule `json:"rules"`
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
	if err := h.svc.Create(r.Context(), pl); err != nil {
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]any{"id": pl.ID}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *playlistHandler) routeByID(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.extractClaims(r)
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
		h.getByID(w, r, plID, claims)
	case action == "" && r.Method == http.MethodPut:
		h.update(w, r, plID, claims)
	case action == "" && r.Method == http.MethodDelete:
		h.delete(w, r, plID, claims)
	case action == "items" && r.Method == http.MethodPost:
		h.addItem(w, r, plID, claims)
	case strings.HasPrefix(action, "items/") && r.Method == http.MethodDelete:
		mediaItemID := strings.TrimPrefix(action, "items/")
		h.removeItem(w, r, plID, mediaItemID, claims)
	case action == "reorder" && r.Method == http.MethodPut:
		h.reorder(w, r, plID, claims)
	default:
		http.Error(w, `{"message":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (h *playlistHandler) getByID(w http.ResponseWriter, r *http.Request, plID string, claims *domainuser.Claims) {
	pl, err := h.svc.GetByID(r.Context(), plID, claims.UserID)
	if err != nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	mediaItems, plItems, err := h.svc.GetItems(r.Context(), plID, claims.UserID)
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
	if err := json.NewEncoder(w).Encode(map[string]any{
		"playlist": map[string]any{
			"id": pl.ID, "name": pl.Name, "description": pl.Description,
			"type": string(pl.Type), "rules": rules,
			"sortBy": pl.SortBy, "sortOrder": pl.SortOrder,
			"createdAt": pl.CreatedAt.Format(time.RFC3339),
			"updatedAt": pl.UpdatedAt.Format(time.RFC3339),
		},
		"items": items,
		"media": media,
	}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *playlistHandler) update(w http.ResponseWriter, r *http.Request, plID string, claims *domainuser.Claims) {
	var body struct {
		Name        string                `json:"name"`
		Description string                `json:"description"`
		Rules       []domainplaylist.Rule `json:"rules"`
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
	if err := h.svc.Update(r.Context(), pl, claims.UserID); err != nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *playlistHandler) delete(w http.ResponseWriter, r *http.Request, plID string, claims *domainuser.Claims) {
	if err := h.svc.Delete(r.Context(), plID, claims.UserID); err != nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *playlistHandler) addItem(w http.ResponseWriter, r *http.Request, plID string, claims *domainuser.Claims) {
	var body struct {
		MediaItemID string `json:"mediaItemId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return
	}
	if err := h.svc.AddItem(r.Context(), plID, body.MediaItemID, claims.UserID); err != nil {
		switch {
		case errors.Is(err, appplaylist.ErrNotOwner):
			http.Error(w, `{"message":"not playlist owner"}`, http.StatusForbidden)
		case errors.Is(err, appplaylist.ErrSmartItems):
			http.Error(w, `{"message":"cannot manually add items to smart playlist"}`, http.StatusBadRequest)
		case errors.Is(err, appplaylist.ErrMaxItems):
			http.Error(w, `{"message":"playlist item limit reached"}`, http.StatusBadRequest)
		case errors.Is(err, appplaylist.ErrMediaItemNotFound):
			http.Error(w, `{"message":"media item not found"}`, http.StatusBadRequest)
		default:
			slog.Error("add playlist item failed", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *playlistHandler) removeItem(w http.ResponseWriter, r *http.Request, plID, mediaItemID string, claims *domainuser.Claims) {
	if err := h.svc.RemoveItem(r.Context(), plID, mediaItemID, claims.UserID); err != nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *playlistHandler) reorder(w http.ResponseWriter, r *http.Request, plID string, claims *domainuser.Claims) {
	var body struct {
		MediaItemIDs []string `json:"mediaItemIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return
	}
	if err := h.svc.ReorderItems(r.Context(), plID, claims.UserID, body.MediaItemIDs); err != nil {
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
