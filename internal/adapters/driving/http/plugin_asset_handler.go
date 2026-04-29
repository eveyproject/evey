package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	appplugin "github.com/evey-project/evey/internal/app/plugin"
	domainplugin "github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type pluginManager interface {
	InstallPlugin(ctx context.Context, source string) (*domainplugin.Plugin, error)
	PreviewPlugin(ctx context.Context, zipPath string) (*appplugin.PluginPreview, error)
}

type cssSanitizer func(data []byte) (sanitized []byte, warnings []string)

type pluginAssetHandler struct {
	pluginDir string
	repo      domainplugin.PluginRepository
}

func registerPluginAssetRoutes(mux *http.ServeMux, tokenGen *auth.JWTGenerator, repo domainplugin.PluginRepository, pluginDir string, sanitize cssSanitizer) {
	authMw := httpAuthMiddleware(tokenGen)
	h := &pluginAssetHandler{pluginDir: pluginDir, repo: repo}
	// Legacy redirect: /widget.js → /front.js (301 permanent)
	mux.HandleFunc("GET /api/v1/plugins/{id}/widget.js", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		http.Redirect(w, r, "/api/v1/plugins/"+id+"/front.js", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /api/v1/plugins/{id}/front.js", authMw(h.handleFrontJS))
	// Legacy redirect: plugins/{id}/theme.css → themes/{id}/theme.css (301 permanent)
	mux.HandleFunc("GET /api/v1/plugins/{id}/theme.css", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		http.Redirect(w, r, "/api/v1/themes/"+id+"/theme.css", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /api/v1/plugins/themes", authMw(h.handleListThemes))
}

// frontCSP builds the Content-Security-Policy for plugin front modules.
// connect-src is derived from the plugin's ExternalFetch allow-list once that
// field exists on the model; for now there is no such field, so it is 'none'.
func frontCSP(_ []string) string {
	return "default-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'none'; img-src data: blob:; font-src data:"
}

func (h *pluginAssetHandler) handleFrontJS(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.plugin.FrontJS")
	defer span.End()
	r = r.WithContext(ctx)
	pluginID := r.PathValue("id")
	span.SetAttributes(
		attribute.String("plugin_id", pluginID),
		attribute.String("asset", "front.js"),
	)
	p, err := h.repo.GetByID(r.Context(), pluginID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "plugin not found")
		http.Error(w, `{"message":"plugin not found"}`, http.StatusNotFound)
		return
	}
	if !p.Enabled {
		span.SetStatus(otelcodes.Error, "plugin disabled")
		http.Error(w, `{"message":"plugin not found"}`, http.StatusNotFound)
		return
	}
	// Only plugins with frontend extension points expose a front module.
	if len(p.UIWidgets) == 0 {
		span.SetStatus(otelcodes.Error, "no frontend")
		http.Error(w, `{"message":"plugin not found"}`, http.StatusNotFound)
		return
	}

	frontPath := filepath.Join(h.pluginDir, pluginID, "front.js")
	if !strings.HasPrefix(filepath.Clean(frontPath), filepath.Clean(h.pluginDir)) {
		span.SetStatus(otelcodes.Error, "invalid path")
		http.Error(w, `{"message":"invalid path"}`, http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(frontPath)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "front.js not found")
		http.Error(w, `{"message":"plugin not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", frontCSP(nil))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if _, writeErr := w.Write(data); writeErr != nil {
		return
	}
}

func (h *pluginAssetHandler) handleListThemes(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.plugin.ListThemes")
	defer span.End()
	r = r.WithContext(ctx)
	plugins, err := h.repo.List(r.Context())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "list plugins failed")
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
		if slices.Contains(p.Capabilities, "ui.theme") {
			result = append(result, themeEntry{ID: p.ID, Name: p.Name})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if encErr := json.NewEncoder(w).Encode(map[string]any{"themes": result}); encErr != nil {
		slog.Warn("encode themes", slog.Any("err", encErr))
	}
}

func registerPluginUploadRoute(mux *http.ServeMux, tokenGen *auth.JWTGenerator, svc pluginManager, writeRL *ratelimit.Limiter) {
	admin := httpAdminMiddleware(tokenGen)
	mux.HandleFunc("POST /api/v1/admin/plugins/upload", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.plugin.Upload")
		defer span.End()
		r = r.WithContext(ctx)
		r.Body = http.MaxBytesReader(w, r.Body, 100<<20) // 100MB max
		file, _, formErr := r.FormFile("plugin")
		if formErr != nil {
			span.RecordError(formErr)
			span.SetStatus(otelcodes.Error, "plugin file required")
			http.Error(w, `{"message":"plugin file required"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()

		tmpFile, tmpErr := os.CreateTemp("", "evey-plugin-*.zip")
		if tmpErr != nil {
			span.RecordError(tmpErr)
			span.SetStatus(otelcodes.Error, "tmp file creation failed")
			slog.Error("create temp file for plugin upload", slog.Any("err", tmpErr))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		tmpName := tmpFile.Name()
		defer func() {
			if rmErr := os.Remove(tmpName); rmErr != nil {
				slog.Debug("remove temp plugin file", slog.Any("err", rmErr))
			}
		}()
		defer tmpFile.Close()

		if _, cpErr := io.Copy(tmpFile, file); cpErr != nil {
			span.RecordError(cpErr)
			span.SetStatus(otelcodes.Error, "write plugin upload failed")
			slog.Error("write plugin upload", slog.Any("err", cpErr))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		tmpFile.Close()

		// Optional cosign signature: save alongside the zip so the verifier
		// can find it at <zipPath>.sig without the caller needing to know the
		// temp file path.
		if sigFile, _, sigErr := r.FormFile("sig"); sigErr == nil {
			sigPath := tmpName + ".sig"
			defer os.Remove(sigPath) //nolint:errcheck
			if sf, createErr := os.Create(sigPath); createErr == nil {
				_, _ = io.Copy(sf, sigFile)
				sf.Close()
			}
			sigFile.Close()
		}

		p, installErr := svc.InstallPlugin(r.Context(), tmpFile.Name())
		if installErr != nil {
			span.RecordError(installErr)
			span.SetStatus(otelcodes.Error, "install plugin failed")
			slog.Error("install uploaded plugin", slog.Any("err", installErr))
			if strings.Contains(installErr.Error(), "plugin blocked:") {
				issues := extractBlockedIssues(installErr.Error())
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				if encErr := json.NewEncoder(w).Encode(map[string]any{
					"message": "Plugin blocked by security analysis",
					"blocked": true,
					"issues":  issues,
				}); encErr != nil {
					slog.Warn("encode blocked response", slog.Any("err", encErr))
				}
			} else {
				http.Error(w, `{"message":"plugin installation failed"}`, http.StatusInternalServerError)
			}
			return
		}

		span.SetAttributes(attribute.String("plugin_id", p.ID))
		slog.Info("plugin installed via upload", slog.String("id", p.ID), slog.String("name", p.Name))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		perms := p.Permissions
		if perms == nil {
			perms = []string{}
		}
		if encErr := json.NewEncoder(w).Encode(map[string]any{
			"id":          p.ID,
			"name":        p.Name,
			"permissions": perms,
		}); encErr != nil {
			slog.Warn("encode plugin response", slog.Any("err", encErr))
		}
	})))
}

func registerPluginPreviewRoute(mux *http.ServeMux, tokenGen *auth.JWTGenerator, svc pluginManager, writeRL *ratelimit.Limiter) {
	admin := httpAdminMiddleware(tokenGen)
	mux.HandleFunc("POST /api/v1/admin/plugins/preview", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.plugin.Preview")
		defer span.End()
		r = r.WithContext(ctx)
		r.Body = http.MaxBytesReader(w, r.Body, 100<<20) // 100MB max
		file, _, formErr := r.FormFile("plugin")
		if formErr != nil {
			span.RecordError(formErr)
			span.SetStatus(otelcodes.Error, "plugin file required")
			http.Error(w, `{"message":"plugin file required"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()

		tmpFile, tmpErr := os.CreateTemp("", "evey-plugin-preview-*.zip")
		if tmpErr != nil {
			span.RecordError(tmpErr)
			span.SetStatus(otelcodes.Error, "tmp file creation failed")
			slog.Error("create temp file for plugin preview", slog.Any("err", tmpErr))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		tmpName := tmpFile.Name()
		defer func() {
			if rmErr := os.Remove(tmpName); rmErr != nil {
				slog.Debug("remove temp plugin preview file", slog.Any("err", rmErr))
			}
		}()
		defer tmpFile.Close()

		if _, cpErr := io.Copy(tmpFile, file); cpErr != nil {
			span.RecordError(cpErr)
			span.SetStatus(otelcodes.Error, "write plugin preview upload failed")
			slog.Error("write plugin preview upload", slog.Any("err", cpErr))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		tmpFile.Close()

		preview, previewErr := svc.PreviewPlugin(r.Context(), tmpName)
		if previewErr != nil {
			span.RecordError(previewErr)
			span.SetStatus(otelcodes.Error, "preview plugin failed")
			slog.Warn("preview plugin", slog.Any("err", previewErr))
			http.Error(w, `{"message":"plugin preview failed"}`, http.StatusBadRequest)
			return
		}

		type analysisResp struct {
			Safe              bool     `json:"safe"`
			Issues            []string `json:"issues"`
			URLs              []string `json:"urls,omitempty"`
			TotalFilesScanned int      `json:"total_files_scanned"`
		}

		perms := preview.Permissions
		if perms == nil {
			perms = []string{}
		}
		caps := preview.Capabilities
		if caps == nil {
			caps = []string{}
		}

		resp := struct {
			Name         string        `json:"name"`
			Version      string        `json:"version"`
			Description  string        `json:"description"`
			Author       string        `json:"author"`
			Language     string        `json:"language"`
			Entrypoint   string        `json:"entrypoint"`
			Permissions  []string      `json:"permissions"`
			Capabilities []string      `json:"capabilities"`
			SourceHash   string        `json:"source_hash"`
			Analysis     *analysisResp `json:"analysis,omitempty"`
		}{
			Name:         preview.Name,
			Version:      preview.Version,
			Description:  preview.Description,
			Author:       preview.Author,
			Language:     preview.Language,
			Entrypoint:   preview.Entrypoint,
			Permissions:  perms,
			Capabilities: caps,
			SourceHash:   preview.SourceHash,
		}

		if preview.Analysis != nil {
			issues := preview.Analysis.Issues
			if issues == nil {
				issues = []string{}
			}
			resp.Analysis = &analysisResp{
				Safe:              preview.Analysis.Safe,
				Issues:            issues,
				TotalFilesScanned: preview.Analysis.FileCount,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
			slog.Warn("encode plugin preview", slog.Any("err", encErr))
		}
	})))
}

// extractBlockedIssues parses individual issues from the analyzer error message.
func extractBlockedIssues(errMsg string) []string {
	idx := strings.Index(errMsg, "[")
	if idx < 0 {
		return []string{errMsg}
	}
	raw := errMsg[idx:]
	var issues []string
	for part := range strings.SplitSeq(raw, " [") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if part[0] != '[' {
			part = "[" + part
		}
		part = strings.TrimRight(part, "]")
		if part != "" {
			issues = append(issues, part)
		}
	}
	if len(issues) == 0 {
		return []string{errMsg}
	}
	return issues
}
