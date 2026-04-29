package httpapi

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	domainplugin "github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// themeCSPScript is the Content-Security-Policy applied to theme JS assets.
// Themes run in a sandboxed context: no external connections, data/blob images only.
const themeCSPScript = "default-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'none'; img-src data: blob:; font-src data:"

type themeAssetHandler struct {
	pluginDir string
	repo      domainplugin.PluginRepository
}

func registerThemeAssetRoutes(mux *http.ServeMux, tokenGen *auth.JWTGenerator, repo domainplugin.PluginRepository, pluginDir string) {
	authMw := httpAuthMiddleware(tokenGen)
	h := &themeAssetHandler{pluginDir: pluginDir, repo: repo}

	// theme.css is public — CSS may be applied before authentication (e.g. login page styling)
	mux.HandleFunc("GET /api/v1/themes/{id}/theme.css", h.handleThemeCSS)
	// theme.js requires auth — JS execution is user-facing, not needed before login
	mux.HandleFunc("GET /api/v1/themes/{id}/theme.js", authMw(h.handleThemeJS))
}

func (h *themeAssetHandler) handleThemeCSS(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.theme.ThemeCSS")
	defer span.End()
	r = r.WithContext(ctx)

	id := r.PathValue("id")
	span.SetAttributes(
		attribute.String("plugin_id", id),
		attribute.String("asset", "theme.css"),
	)

	p, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "theme not found")
		http.Error(w, `{"message":"theme not found"}`, http.StatusNotFound)
		return
	}
	if !p.Enabled {
		span.SetStatus(otelcodes.Error, "theme disabled")
		http.Error(w, `{"message":"theme not found"}`, http.StatusNotFound)
		return
	}
	if !slices.Contains(p.Capabilities, "ui.theme") {
		span.SetStatus(otelcodes.Error, "not a theme")
		http.Error(w, `{"message":"theme not found"}`, http.StatusNotFound)
		return
	}

	cssPath := filepath.Join(h.pluginDir, id, "theme.css")
	if !strings.HasPrefix(filepath.Clean(cssPath), filepath.Clean(h.pluginDir)) {
		span.SetStatus(otelcodes.Error, "invalid path")
		http.Error(w, `{"message":"invalid path"}`, http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(cssPath)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "theme.css not found")
		http.Error(w, `{"message":"theme not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if _, writeErr := w.Write(data); writeErr != nil {
		slog.Debug("write theme.css response", slog.Any("err", writeErr))
	}
}

func (h *themeAssetHandler) handleThemeJS(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.theme.ThemeJS")
	defer span.End()
	r = r.WithContext(ctx)

	id := r.PathValue("id")
	span.SetAttributes(
		attribute.String("plugin_id", id),
		attribute.String("asset", "theme.js"),
	)

	p, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "theme not found")
		http.Error(w, `{"message":"theme not found"}`, http.StatusNotFound)
		return
	}
	if !p.Enabled {
		span.SetStatus(otelcodes.Error, "theme disabled")
		http.Error(w, `{"message":"theme not found"}`, http.StatusNotFound)
		return
	}
	if !slices.Contains(p.Capabilities, "ui.theme") {
		span.SetStatus(otelcodes.Error, "not a theme")
		http.Error(w, `{"message":"theme not found"}`, http.StatusNotFound)
		return
	}

	jsPath := filepath.Join(h.pluginDir, id, "theme.js")
	if !strings.HasPrefix(filepath.Clean(jsPath), filepath.Clean(h.pluginDir)) {
		span.SetStatus(otelcodes.Error, "invalid path")
		http.Error(w, `{"message":"invalid path"}`, http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(jsPath)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "theme.js not found")
		http.Error(w, `{"message":"theme not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", themeCSPScript)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if _, writeErr := w.Write(data); writeErr != nil {
		slog.Debug("write theme.js response", slog.Any("err", writeErr))
	}
}
