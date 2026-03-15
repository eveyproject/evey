package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
)

type mediaItemLister interface {
	ListMediaItems(ctx context.Context, filter domainlibrary.ListMediaItemsFilter) ([]*domainlibrary.MediaItem, error)
	CountMediaItems(ctx context.Context, filter domainlibrary.ListMediaItemsFilter) (int, error)
}

type filterHandler struct {
	lister     mediaItemLister
	tokenGen   TokenVerifier
	libChecker LibraryAccessChecker
}

func registerFilterRoutes(mux *http.ServeMux, lister mediaItemLister, tokenGen TokenVerifier, libChecker LibraryAccessChecker) {
	h := &filterHandler{lister: lister, tokenGen: tokenGen, libChecker: libChecker}
	mux.HandleFunc("GET /api/v1/media/filter", h.handleFilter)
}

func (h *filterHandler) handleFilter(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	if token == "" {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	claims, err := h.tokenGen.ValidateAccessToken(token)
	if err != nil {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	filter := h.parseFilter(r, w, claims)
	if filter == nil {
		return
	}

	total, err := h.lister.CountMediaItems(r.Context(), *filter)
	if err != nil {
		slog.Error("filter count media items", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	items, err := h.lister.ListMediaItems(r.Context(), *filter)
	if err != nil {
		slog.Error("filter list media items", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}

	h.writeResponse(w, items, total)
}

func (h *filterHandler) parseFilter(r *http.Request, w http.ResponseWriter, claims *domainuser.Claims) *domainlibrary.ListMediaItemsFilter {
	q := r.URL.Query()
	filter := domainlibrary.ListMediaItemsFilter{
		SortField: q.Get("sort_field"),
		SortOrder: q.Get("sort_order"),
	}

	if libID := q.Get("library_id"); libID != "" {
		if !checkLibraryAccess(r.Context(), w, claims, libID, h.libChecker) {
			return nil
		}
		filter.LibraryID = &libID
	}
	if t := q.Get("type"); t != "" {
		filter.Type = &t
	}
	if g := q.Get("genre"); g != "" {
		filter.Genres = []string{g}
	}
	if v := q.Get("year_from"); v != "" {
		if n, parseErr := strconv.Atoi(v); parseErr == nil && n > 0 {
			filter.YearFrom = n
		}
	}
	if v := q.Get("year_to"); v != "" {
		if n, parseErr := strconv.Atoi(v); parseErr == nil && n > 0 {
			filter.YearTo = n
		}
	}
	if v := q.Get("min_rating"); v != "" {
		if f, parseErr := strconv.ParseFloat(v, 64); parseErr == nil && f > 0 {
			filter.MinRating = f
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, parseErr := strconv.Atoi(v); parseErr == nil && n > 0 {
			filter.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, parseErr := strconv.Atoi(v); parseErr == nil && n >= 0 {
			filter.Offset = n
		}
	}

	return &filter
}

type filterItemResp struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Type            string   `json:"type"`
	Year            *int     `json:"year,omitempty"`
	CommunityRating *float64 `json:"communityRating,omitempty"`
	LibraryID       string   `json:"libraryId"`
}

func (h *filterHandler) writeResponse(w http.ResponseWriter, items []*domainlibrary.MediaItem, total int) {
	out := make([]filterItemResp, 0, len(items))
	for _, item := range items {
		out = append(out, filterItemResp{
			ID:              item.ID,
			Title:           item.Title,
			Type:            item.Type,
			Year:            item.Year,
			CommunityRating: item.CommunityRating,
			LibraryID:       item.LibraryID,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"items": out,
		"total": total,
	}); err != nil {
		slog.Warn("json encode filter response", slog.Any("err", err))
	}
}
