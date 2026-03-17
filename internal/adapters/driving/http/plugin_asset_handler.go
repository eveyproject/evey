package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	domainplugin "github.com/gravity-zero/evey/internal/domain/plugin"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

type pluginInstaller interface {
	InstallPlugin(ctx context.Context, source string) (*domainplugin.Plugin, error)
}

type pluginAssetHandler struct {
	tokenGen  *auth.JWTGenerator
	pluginDir string
	repo      domainplugin.PluginRepository
}

func registerPluginAssetRoutes(mux *http.ServeMux, tokenGen *auth.JWTGenerator, repo domainplugin.PluginRepository, pluginDir string) {
	h := &pluginAssetHandler{tokenGen: tokenGen, pluginDir: pluginDir, repo: repo}
	mux.HandleFunc("GET /api/v1/plugins/{id}/widget.js", h.handleWidgetJS)
	mux.HandleFunc("GET /api/v1/plugins/{id}/theme.css", h.handleThemeCSS)
	mux.HandleFunc("GET /api/v1/plugins/themes", h.handleListThemes)
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
	// theme CSS is public — no auth required (not sensitive data)
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

func (h *pluginAssetHandler) handleListThemes(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	if token == "" {
		http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if _, err := h.tokenGen.ValidateAccessToken(token); err != nil {
		http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	plugins, err := h.repo.List(r.Context())
	if err != nil {
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}

	type themeEntry struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	var result []themeEntry
	for _, p := range plugins {
		if !p.Enabled {
			continue
		}
		for _, cap := range p.Capabilities {
			if cap == "ui.theme" {
				result = append(result, themeEntry{ID: p.ID, Name: p.Name})
				break
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if encErr := json.NewEncoder(w).Encode(map[string]any{"themes": result}); encErr != nil {
		slog.Warn("encode themes", slog.Any("err", encErr))
	}
}

func registerPluginUploadRoute(mux *http.ServeMux, tokenGen *auth.JWTGenerator, svc pluginInstaller) {
	mux.HandleFunc("POST /api/v1/admin/plugins/upload", func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil || !claims.IsAdmin {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 100<<20) // 100MB max
		file, _, formErr := r.FormFile("plugin")
		if formErr != nil {
			http.Error(w, `{"message":"plugin file required"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()

		tmpFile, tmpErr := os.CreateTemp("", "evey-plugin-*.zip")
		if tmpErr != nil {
			slog.Error("create temp file for plugin upload", slog.Any("err", tmpErr))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()

		if _, cpErr := io.Copy(tmpFile, file); cpErr != nil {
			slog.Error("write plugin upload", slog.Any("err", cpErr))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		tmpFile.Close()

		p, installErr := svc.InstallPlugin(r.Context(), tmpFile.Name())
		if installErr != nil {
			slog.Error("install uploaded plugin", slog.Any("err", installErr))
			http.Error(w, `{"message":"install failed: `+installErr.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		slog.Info("plugin installed via upload", slog.String("id", p.ID), slog.String("name", p.Name))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if encErr := json.NewEncoder(w).Encode(map[string]any{
			"id":   p.ID,
			"name": p.Name,
		}); encErr != nil {
			slog.Warn("encode plugin response", slog.Any("err", encErr))
		}
	})
}
