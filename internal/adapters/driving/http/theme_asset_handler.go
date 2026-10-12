package httpapi

import (
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	domainplugin "github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/pkg/safefs"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// themeCSPScript is the Content-Security-Policy applied to theme JS assets.
// Themes run in a sandboxed context: no external connections, data/blob images only.
const themeCSPScript = "default-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'none'; img-src data: blob:; font-src data:"

type themeAssetHandler struct {
	pluginDir string
	repo      domainplugin.PluginRepository
}

func registerThemeAssetRoutes(mux *http.ServeMux, tokenGen domainuser.TokenGenerator, repo domainplugin.PluginRepository, pluginDir string) {
	authMw := httpAuthMiddleware(tokenGen)
	h := &themeAssetHandler{pluginDir: pluginDir, repo: repo}

	// theme.css is public - CSS may be applied before authentication (e.g. login page styling)
	mux.HandleFunc("GET /api/v1/themes/{id}/theme.css", h.handleThemeCSS)
	// theme.js requires auth - JS execution is user-facing, not needed before login
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
	if !p.IsTheme() {
		span.SetStatus(otelcodes.Error, "not a theme")
		http.Error(w, `{"message":"theme not found"}`, http.StatusNotFound)
		return
	}

	cssPath := filepath.Join(h.pluginDir, id, "theme.css")
	// Require the path separator after pluginDir: without it, an id like
	// "../plugins-evil/foo" would resolve to /…/plugins-evil/foo which still
	// matches HasPrefix("/…/plugins") and would escape the install root.
	// (The repo lookup above is the primary defence, this is belt-and-braces.)
	cleanedRoot := filepath.Clean(h.pluginDir) + string(filepath.Separator)
	if !strings.HasPrefix(filepath.Clean(cssPath), cleanedRoot) {
		span.SetStatus(otelcodes.Error, "invalid path")
		http.Error(w, `{"message":"invalid path"}`, http.StatusBadRequest)
		return
	}

	// The theme owns its install dir (symlinks included) under the server uid,
	// and this route is public: no link is followed, regular files only.
	data, _, err := safefs.ReadFile(h.pluginDir, id+"/theme.css", 0)
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
	if !p.IsTheme() {
		span.SetStatus(otelcodes.Error, "not a theme")
		http.Error(w, `{"message":"theme not found"}`, http.StatusNotFound)
		return
	}

	jsPath := filepath.Join(h.pluginDir, id, "theme.js")
	// Separator-anchored prefix check (see handleThemeCSS above for the
	// reasoning - prevents an id like "../plugins-evil/…" from satisfying
	// HasPrefix while pointing outside the install root).
	cleanedRoot := filepath.Clean(h.pluginDir) + string(filepath.Separator)
	if !strings.HasPrefix(filepath.Clean(jsPath), cleanedRoot) {
		span.SetStatus(otelcodes.Error, "invalid path")
		http.Error(w, `{"message":"invalid path"}`, http.StatusBadRequest)
		return
	}

	data, _, err := safefs.ReadFile(h.pluginDir, id+"/theme.js", 0)
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
