package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	apprating "github.com/gravity-zero/evey/internal/app/rating"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func registerRatingRoutes(mux *http.ServeMux, ratingRepo domainlibrary.UserRatingRepository, tokenGen *auth.JWTGenerator, mediaGetter mediaLibraryGetter, libChecker LibraryAccessChecker) {
	h := &ratingHandler{
		ratingRepo:  ratingRepo,
		tokenGen:    tokenGen,
		mediaGetter: mediaGetter,
		libChecker:  libChecker,
	}
	mux.HandleFunc("PUT /api/v1/media/{id}/rate", h.putRate)
	mux.HandleFunc("GET /api/v1/media/{id}/my-rating", h.getMyRating)
}

type ratingHandler struct {
	ratingRepo  domainlibrary.UserRatingRepository
	tokenGen    *auth.JWTGenerator
	mediaGetter mediaLibraryGetter
	libChecker  LibraryAccessChecker
}

func (h *ratingHandler) checkMediaAccess(ctx context.Context, w http.ResponseWriter, claims *domainuser.Claims, mediaID string) bool {
	item, err := h.mediaGetter.GetMediaItem(ctx, mediaID)
	if err != nil || item == nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return false
	}
	return checkLibraryAccess(ctx, w, claims, item.LibraryID, h.libChecker)
}

func (h *ratingHandler) putRate(w http.ResponseWriter, r *http.Request) {
	claims, ok := extractClaims(r, h.tokenGen)
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
	if !h.checkMediaAccess(r.Context(), w, claims, mediaID) {
		return
	}
	rating := &domainlibrary.UserRating{
		ProfileID:   claims.ProfileID,
		MediaItemID: mediaID,
		Value:       body.Value,
	}
	if err := h.ratingRepo.Upsert(r.Context(), rating); err != nil {
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"value": body.Value}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *ratingHandler) getMyRating(w http.ResponseWriter, r *http.Request) {
	claims, ok := extractClaims(r, h.tokenGen)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	mediaID := r.PathValue("id")
	if !h.checkMediaAccess(r.Context(), w, claims, mediaID) {
		return
	}
	rating, err := h.ratingRepo.Get(r.Context(), claims.ProfileID, mediaID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"value": 0}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"value": rating.Value}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func registerStarRatingRoutes(mux *http.ServeMux, svc *apprating.Service, tokenGen *auth.JWTGenerator, mediaGetter mediaLibraryGetter, libChecker LibraryAccessChecker) {
	h := &starRatingHandler{
		svc:         svc,
		tokenGen:    tokenGen,
		mediaGetter: mediaGetter,
		libChecker:  libChecker,
	}
	mux.HandleFunc("POST /api/v1/media/{id}/rating", h.postRating)
	mux.HandleFunc("GET /api/v1/media/{id}/rating", h.getRating)
	mux.HandleFunc("DELETE /api/v1/media/{id}/rating", h.deleteRating)
	mux.HandleFunc("GET /api/v1/media/{id}/rating/aggregate", h.getAggregate)
}

type starRatingHandler struct {
	svc         *apprating.Service
	tokenGen    *auth.JWTGenerator
	mediaGetter mediaLibraryGetter
	libChecker  LibraryAccessChecker
}

func (h *starRatingHandler) checkMediaAccess(ctx context.Context, w http.ResponseWriter, claims *domainuser.Claims, mediaID string) bool {
	item, err := h.mediaGetter.GetMediaItem(ctx, mediaID)
	if err != nil || item == nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return false
	}
	return checkLibraryAccess(ctx, w, claims, item.LibraryID, h.libChecker)
}

func (h *starRatingHandler) postRating(w http.ResponseWriter, r *http.Request) {
	claims, ok := extractClaims(r, h.tokenGen)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	mediaID := r.PathValue("id")
	if !h.checkMediaAccess(r.Context(), w, claims, mediaID) {
		return
	}
	var body struct {
		Value int `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return
	}
	if err := h.svc.Rate(r.Context(), mediaID, claims.ProfileID, body.Value); err != nil {
		slog.Error("star rating", slog.Any("err", err))
		http.Error(w, `{"message":"rating must be between 1 and 10"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"value": body.Value}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *starRatingHandler) getRating(w http.ResponseWriter, r *http.Request) {
	claims, ok := extractClaims(r, h.tokenGen)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	mediaID := r.PathValue("id")
	if !h.checkMediaAccess(r.Context(), w, claims, mediaID) {
		return
	}
	rt, err := h.svc.GetRating(r.Context(), mediaID, claims.ProfileID)
	if err != nil {
		slog.Error("get star rating", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if rt == nil {
		if err := json.NewEncoder(w).Encode(map[string]any{"value": 0}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
		return
	}
	if err := json.NewEncoder(w).Encode(map[string]any{"value": rt.Value.Int()}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *starRatingHandler) deleteRating(w http.ResponseWriter, r *http.Request) {
	claims, ok := extractClaims(r, h.tokenGen)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	mediaID := r.PathValue("id")
	if !h.checkMediaAccess(r.Context(), w, claims, mediaID) {
		return
	}
	if err := h.svc.DeleteRating(r.Context(), mediaID, claims.ProfileID); err != nil {
		slog.Error("delete star rating", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *starRatingHandler) getAggregate(w http.ResponseWriter, r *http.Request) {
	claims, ok := extractClaims(r, h.tokenGen)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	mediaID := r.PathValue("id")
	if !h.checkMediaAccess(r.Context(), w, claims, mediaID) {
		return
	}
	aggs, err := h.svc.GetAggregates(r.Context(), []string{mediaID})
	if err != nil {
		slog.Error("get rating aggregate", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	agg := aggs[mediaID]
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"average": agg.Average, "count": agg.Count}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}
