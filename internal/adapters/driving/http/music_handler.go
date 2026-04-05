package httpapi

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	appmusic "github.com/gravity-zero/evey/internal/app/music"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

type musicHandler struct {
	svc        *appmusic.Service
	tokenGen   *auth.JWTGenerator
	libChecker LibraryAccessChecker
}

func registerMusicRoutes(mux *http.ServeMux, svc *appmusic.Service, tokenGen *auth.JWTGenerator, libChecker LibraryAccessChecker, _ libraryPathsGetter) {
	h := &musicHandler{svc: svc, tokenGen: tokenGen, libChecker: libChecker}
	mux.HandleFunc("GET /api/v1/music/tracks/{id}/art", h.getTrackArt)
	mux.HandleFunc("GET /api/v1/music/tracks/{id}/stream", h.streamTrack)
}

func (h *musicHandler) authenticatePlain(r *http.Request, w http.ResponseWriter) (*domainuser.Claims, bool) {
	token := extractBearerToken(r)
	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	claims, err := h.tokenGen.ValidateAccessToken(token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	return claims, true
}

func (h *musicHandler) getTrackArt(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.authenticatePlain(r, w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	track, err := h.svc.GetTrack(r.Context(), id)
	if err != nil || track == nil {
		http.NotFound(w, r)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, track.LibraryID, h.libChecker) {
		return
	}
	data, mimeType, err := h.svc.GetTrackArt(r.Context(), id)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		slog.Error("music get art", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if _, err := w.Write(data); err != nil {
		slog.Error("music art write", slog.Any("err", err))
	}
}

func (h *musicHandler) streamTrack(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.authenticatePlain(r, w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	track, err := h.svc.GetTrack(r.Context(), id)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		slog.Error("music get track", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if track == nil {
		http.NotFound(w, r)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, track.LibraryID, h.libChecker) {
		return
	}
	filePath, mimeType, err := h.svc.StreamTrack(r.Context(), id)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		slog.Error("music stream", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	f, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	if mimeType != "" {
		w.Header().Set("Content-Type", mimeType)
	}
	// Disable write deadline for streaming — global WriteTimeout would cut large files.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		slog.Warn("set write deadline", slog.Any("err", err))
	}
	http.ServeContent(w, r, filePath, time.Time{}, f)
}

