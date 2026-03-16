package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	appsyncplay "github.com/gravity-zero/evey/internal/app/syncplay"
	domainsyncplay "github.com/gravity-zero/evey/internal/domain/syncplay"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

type syncplayHandler struct {
	hub        *appsyncplay.Hub
	tokenGen   *auth.JWTGenerator
	media      mediaLibraryGetter
	libChecker LibraryAccessChecker
}

func registerSyncPlayRoutes(mux *http.ServeMux, hub *appsyncplay.Hub, tokenGen *auth.JWTGenerator, mediaGetter mediaLibraryGetter, libChecker LibraryAccessChecker) {
	h := &syncplayHandler{
		hub:        hub,
		tokenGen:   tokenGen,
		media:      mediaGetter,
		libChecker: libChecker,
	}
	mux.HandleFunc("POST /api/v1/syncplay/rooms", h.createRoom)
	mux.HandleFunc("GET /api/v1/syncplay/rooms/{id}", h.getRoom)
	mux.HandleFunc("POST /api/v1/syncplay/rooms/{id}/join", h.joinRoom)
	mux.HandleFunc("DELETE /api/v1/syncplay/rooms/{id}/leave", h.leaveRoom)
	mux.Handle("/api/v1/syncplay/rooms/{id}/ws", hub.WSHandler(h.validateToken))
}

func (s *syncplayHandler) extractClaims(r *http.Request) (*domainuser.Claims, bool) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return nil, false
	}
	claims, err := s.tokenGen.ValidateAccessToken(h[7:])
	if err != nil {
		return nil, false
	}
	return claims, true
}

func (s *syncplayHandler) validateToken(token string) (userID, username string, err error) {
	claims, vErr := s.tokenGen.ValidateAccessToken(token)
	if vErr != nil {
		return "", "", vErr
	}
	return claims.UserID, claims.Username, nil
}

func (s *syncplayHandler) createRoom(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.extractClaims(r)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		MediaID string `json:"mediaId"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.MediaID == "" {
		http.Error(w, `{"message":"mediaId required"}`, http.StatusBadRequest)
		return
	}
	item, err := s.media.GetMediaItem(r.Context(), body.MediaID)
	if err != nil || item == nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, s.libChecker) {
		return
	}
	if body.Name == "" {
		body.Name = "Watch Together"
	}
	room, err := s.hub.CreateRoom(r.Context(), claims.UserID, body.MediaID, body.Name)
	if err != nil {
		slog.Error("syncplay create room", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id":      room.ID,
		"name":    room.Name,
		"mediaId": room.MediaID,
		"hostId":  room.HostID,
	}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (s *syncplayHandler) getRoom(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.extractClaims(r)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	room, err := s.hub.GetRoom(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, `{"message":"room not found"}`, http.StatusNotFound)
		return
	}
	isMember := room.HostID == claims.UserID
	for _, m := range room.Members {
		if m.UserID == claims.UserID {
			isMember = true
			break
		}
	}
	if !isMember && !claims.IsAdmin {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	type memberResp struct {
		UserID   string `json:"userId"`
		Username string `json:"username"`
	}
	members := make([]memberResp, 0, len(room.Members))
	for _, m := range room.Members {
		members = append(members, memberResp{UserID: m.UserID, Username: domainsyncplay.MaskUsername(m.Username)})
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id":      room.ID,
		"name":    room.Name,
		"mediaId": room.MediaID,
		"hostId":  room.HostID,
		"members": members,
		"state": map[string]any{
			"playing":    room.State.Playing,
			"positionMs": room.State.PositionMs,
		},
	}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (s *syncplayHandler) joinRoom(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.extractClaims(r)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	roomID := r.PathValue("id")
	room, err := s.hub.GetRoom(r.Context(), roomID)
	if err != nil {
		http.Error(w, `{"message":"room not found"}`, http.StatusNotFound)
		return
	}
	item, err := s.media.GetMediaItem(r.Context(), room.MediaID)
	if err != nil || item == nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, s.libChecker) {
		return
	}
	if err := s.hub.JoinRoom(r.Context(), roomID, claims.UserID, claims.Username); err != nil {
		http.Error(w, `{"message":"room not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"roomId": roomID,
		"wsUrl":  "/api/v1/syncplay/rooms/" + roomID + "/ws",
	}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (s *syncplayHandler) leaveRoom(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.extractClaims(r)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	if err := s.hub.LeaveRoom(r.Context(), r.PathValue("id"), claims.UserID); err != nil {
		slog.Warn("leave room", slog.String("room", r.PathValue("id")), slog.Any("err", err))
	}
	w.WriteHeader(http.StatusNoContent)
}
