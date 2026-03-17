package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gravity-zero/evey/internal/domain/plugin"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

type pluginAssetHandler struct {
	tokenGen  *auth.JWTGenerator
	pluginDir string
	repo      plugin.PluginRepository
}

func registerPluginAssetRoutes(mux *http.ServeMux, tokenGen *auth.JWTGenerator, repo plugin.PluginRepository, pluginDir string) {
	h := &pluginAssetHandler{tokenGen: tokenGen, pluginDir: pluginDir, repo: repo}
	mux.HandleFunc("GET /api/v1/plugins/{id}/widget.js", h.handleWidgetJS)
	mux.HandleFunc("GET /api/v1/plugins/{id}/theme.css", h.handleThemeCSS)
}

func (h *pluginAssetHandler) handleWidgetJS(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	if token == "" {
		http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if _, err := h.tokenGen.ValidateAccessToken(token); err != nil {
		http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	pluginID := r.PathValue("id")
	p, err := h.repo.GetByID(r.Context(), pluginID)
	if err != nil {
		http.Error(w, `{"message":"plugin not found"}`, http.StatusNotFound)
		return
	}
	if !p.Enabled {
		http.Error(w, `{"message":"plugin disabled"}`, http.StatusNotFound)
		return
	}

	bundlePath := filepath.Join(h.pluginDir, pluginID, "widget.bundle.js")
	if !strings.HasPrefix(filepath.Clean(bundlePath), filepath.Clean(h.pluginDir)) {
		http.Error(w, `{"message":"invalid path"}`, http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(bundlePath)
	if err != nil {
		http.Error(w, `{"message":"widget not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if _, err := w.Write(data); err != nil {
		return
	}
}

func (h *pluginAssetHandler) handleThemeCSS(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	if token == "" {
		http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if _, err := h.tokenGen.ValidateAccessToken(token); err != nil {
		http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	pluginID := r.PathValue("id")
	p, err := h.repo.GetByID(r.Context(), pluginID)
	if err != nil {
		http.Error(w, `{"message":"plugin not found"}`, http.StatusNotFound)
		return
	}
	if !p.Enabled {
		http.Error(w, `{"message":"plugin disabled"}`, http.StatusNotFound)
		return
	}

	hasThemeCap := false
	for _, cap := range p.Capabilities {
		if cap == "ui.theme" {
			hasThemeCap = true
			break
		}
	}
	if !hasThemeCap {
		http.Error(w, `{"message":"not a theme plugin"}`, http.StatusNotFound)
		return
	}

	cssPath := filepath.Join(h.pluginDir, pluginID, "theme.css")
	if !strings.HasPrefix(filepath.Clean(cssPath), filepath.Clean(h.pluginDir)) {
		http.Error(w, `{"message":"invalid path"}`, http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(cssPath)
	if err != nil {
		http.Error(w, `{"message":"theme not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/css")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if _, err := w.Write(data); err != nil {
		return
	}
}
