package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	appplugin "github.com/evey-project/evey/internal/app/plugin"
	domainplugin "github.com/evey-project/evey/internal/domain/plugin"
	pkgcache "github.com/evey-project/evey/internal/pkg/cache"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/safefs"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type pluginManager interface {
	InstallPlugin(ctx context.Context, source string) (*domainplugin.Plugin, error)
	PreviewPlugin(ctx context.Context, zipPath string) (*appplugin.PluginPreview, error)
	// PreviewSource previews an install source (URL or path) the way
	// InstallPlugin would resolve it, installing nothing.
	PreviewSource(ctx context.Context, source string) (*appplugin.PluginPreview, error)
}

type cssSanitizer func(data []byte) (sanitized []byte, warnings []string)

// PluginWakener is the slice of the idle manager the asset route drives: a
// widget being loaded in a browser is the demand signal that resumes a
// backend suspended by the low-memory profile (and refreshes its idle clock).
type PluginWakener interface {
	Touch(pluginID string)
	EnsureStarted(ctx context.Context, pluginID string)
}

type pluginAssetHandler struct {
	pluginDir    string
	repo         domainplugin.PluginRepository
	tokenGen     domainuser.TokenGenerator
	pluginTokens PluginTokenParser
	sanitizeCSS  cssSanitizer
	wakener      PluginWakener
	// etags remembers the content ETag per file so a revalidation of an
	// unchanged asset is a stat, not a full read and hash of the bundle.
	etags *pkgcache.TTLCache[assetETag]
}

// assetETag is one etags entry: the ETag is valid while size and mtime match.
type assetETag struct {
	size  int64
	mtime time.Time
	etag  string
}

const (
	assetETagTTL     = time.Hour
	assetETagMaxSize = 4096
)

// maxPluginAssetBytes is the per-file size limit for plugin asset responses.
// The 100 MB total zip limit is enforced separately at install time.
const maxPluginAssetBytes int64 = 50 << 20 // 50 MB

// corsRequiredAssetExt is the set of asset extensions that need
// Access-Control-Allow-Origin: null for the iframe (opaque origin) to
// fetch them - fonts always require CORS, fetch()-able blobs (3D models,
// JSON, raw binaries) too. Other types load via <img>/<script>/<link>
// without CORS.
var corsRequiredAssetExt = map[string]bool{
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true,
	".glb": true, ".gltf": true, ".json": true, ".bin": true,
	".wasm": true,
}

func registerPluginAssetRoutes(mux *http.ServeMux, tokenGen domainuser.TokenGenerator, pluginTokens PluginTokenParser, repo domainplugin.PluginRepository, pluginDir string, sanitize cssSanitizer, wakener PluginWakener) {
	authMw := httpAuthMiddleware(tokenGen)
	etags, _ := pkgcache.NewWithMaxSize[assetETag](10*time.Minute, assetETagMaxSize)
	h := &pluginAssetHandler{pluginDir: pluginDir, repo: repo, tokenGen: tokenGen, pluginTokens: pluginTokens, sanitizeCSS: sanitize, wakener: wakener, etags: etags}
	// Generic asset route - serves any file under <install_dir>/<web_root>/.
	// Single-bundle frontends only in v1 (no dynamic-import code splitting).
	// Authentication accepts a user access token OR the scoped plugin token
	// issued via /plugins/{id}/token (the iframe loads the script via
	// <script src="...?token=PLUGIN_TOKEN">).
	mux.HandleFunc("GET /api/v1/plugins/{id}/assets/{path...}", h.handlePluginAsset)
	mux.HandleFunc("GET /api/v1/plugins/themes", authMw(h.handleListThemes))
}

// authorizeFrontJS validates the bearer token in the request - either a
// plugin-scoped token (must match the current pluginID) or a regular access
// token. Returns true on success.
func (h *pluginAssetHandler) authorizeFrontJS(r *http.Request, pluginID string) bool {
	tok := extractBearerToken(r)
	if tok == "" {
		return false
	}
	if pc, err := h.pluginTokens.ParsePluginToken(tok); err == nil {
		return pc.PluginID == pluginID
	}
	if _, err := h.tokenGen.ValidateAccessToken(tok); err == nil {
		return true
	}
	return false
}

func (h *pluginAssetHandler) handlePluginAsset(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.plugin.Asset")
	defer span.End()
	r = r.WithContext(ctx)

	pluginID := r.PathValue("id")
	requestedPath := r.PathValue("path")

	span.SetAttributes(
		attribute.String("plugin_id", pluginID),
		attribute.String("asset_path", requestedPath),
	)

	// (1) Auth - plugin-scoped token OR access token.
	if !h.authorizeFrontJS(r, pluginID) {
		span.SetStatus(otelcodes.Error, "unauthorized")
		http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// (2) Demand signal: a browser loading this widget refreshes the plugin's
	// idle clock and resumes a backend the low-memory profile suspended.
	// Async - the assets are served from disk and never wait on the process.
	if h.wakener != nil {
		h.wakener.Touch(pluginID)
		go h.wakener.EnsureStarted(context.WithoutCancel(ctx), pluginID)
	}

	// (3) Path traversal - explicit reject before any join.
	if strings.Contains(requestedPath, "..") ||
		strings.HasPrefix(requestedPath, "/") ||
		strings.Contains(requestedPath, "\\") {
		span.SetStatus(otelcodes.Error, "path traversal rejected")
		http.Error(w, `{"message":"invalid path"}`, http.StatusBadRequest)
		return
	}

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
	if len(p.Frontend) == 0 {
		span.SetStatus(otelcodes.Error, "no frontend module declared")
		http.Error(w, `{"message":"plugin not found"}`, http.StatusNotFound)
		return
	}
	if p.WebRoot == "" {
		span.SetStatus(otelcodes.Error, "plugin has no web_root configured")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}

	// (2) Path restriction - only files under <install_dir>/<web_root>/.
	// web_root is RE-validated here, not only at install time: the value read
	// back from the DB is untrusted input for this handler (a hand-edited or
	// migrated row must not gain traversal). Rejecting ".." / absolute before
	// the join, then anchoring the containment check on a trailing separator -
	// a bare HasPrefix would accept a SIBLING directory whose name merely
	// starts with the plugin's ("/plugins/foo-evil" vs "/plugins/foo").
	if strings.Contains(p.WebRoot, "..") || filepath.IsAbs(p.WebRoot) {
		span.SetStatus(otelcodes.Error, "web_root rejected")
		http.Error(w, `{"message":"invalid path"}`, http.StatusBadRequest)
		return
	}
	pluginRoot := filepath.Clean(filepath.Join(h.pluginDir, pluginID))
	webRoot := filepath.Clean(filepath.Join(pluginRoot, p.WebRoot))
	if webRoot != pluginRoot && !strings.HasPrefix(webRoot, pluginRoot+string(filepath.Separator)) {
		span.SetStatus(otelcodes.Error, "web_root escapes plugin dir")
		http.Error(w, `{"message":"invalid path"}`, http.StatusBadRequest)
		return
	}
	candidate := filepath.Clean(filepath.Join(webRoot, requestedPath))
	if !strings.HasPrefix(candidate, webRoot+string(filepath.Separator)) && candidate != webRoot {
		span.SetStatus(otelcodes.Error, "path escapes web_root")
		http.Error(w, `{"message":"invalid path"}`, http.StatusBadRequest)
		return
	}

	// The plugin owns its install dir (symlinks included) under the server uid:
	// resolve beneath pluginDir without following any link, regular files only.
	rel, relErr := filepath.Rel(filepath.Clean(h.pluginDir), candidate)
	if relErr != nil {
		span.SetStatus(otelcodes.Error, "asset path not relative")
		http.Error(w, `{"message":"invalid path"}`, http.StatusBadRequest)
		return
	}
	rel = filepath.ToSlash(rel)

	// (7) Per-file size limit - refuse > 50 MB before reading.
	stat, statErr := safefs.StatRegular(h.pluginDir, rel)
	if statErr != nil {
		span.RecordError(statErr)
		span.SetStatus(otelcodes.Error, "asset not found")
		http.Error(w, `{"message":"asset not found"}`, http.StatusNotFound)
		return
	}
	if stat.Size() > maxPluginAssetBytes {
		h.rejectOversized(r, span, pluginID, requestedPath, stat.Size())
		http.Error(w, `{"message":"asset too large"}`, http.StatusNotFound)
		return
	}

	// A revalidation of an unchanged file is answered from the stat: same
	// size and mtime as when the ETag was computed means the same bytes.
	if known, ok := h.etags.Get(candidate); ok && known.size == stat.Size() && known.mtime.Equal(stat.ModTime()) &&
		r.Header.Get("If-None-Match") == known.etag {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("ETag", known.etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Read through the fd and take size/mtime from it: the stat above is a
	// separate open, the file may have been swapped in between.
	data, fdStat, readErr := safefs.ReadFile(h.pluginDir, rel, maxPluginAssetBytes)
	if errors.Is(readErr, safefs.ErrTooLarge) {
		h.rejectOversized(r, span, pluginID, requestedPath, fdStat.Size())
		http.Error(w, `{"message":"asset too large"}`, http.StatusNotFound)
		return
	}
	if readErr != nil {
		span.RecordError(readErr)
		span.SetStatus(otelcodes.Error, "asset read failed")
		http.Error(w, `{"message":"asset not found"}`, http.StatusNotFound)
		return
	}
	stat = fdStat

	// (4) Content-Type by extension + nosniff.
	ext := strings.ToLower(filepath.Ext(candidate))
	ct := mime.TypeByExtension(ext)
	if ct == "" {
		ct = "application/octet-stream"
	}
	// .js/.mjs sometimes resolves to text/javascript on older Go runtimes;
	// prefer application/javascript for ES modules dispatched by Vite.
	if ext == ".js" || ext == ".mjs" {
		ct = "application/javascript; charset=utf-8"
	}
	// Some runtimes ship a mime DB without font types - pin them explicitly
	// so plugin-shipped fonts always advertise as font/* (the asset test
	// suite asserts this) and downstream caches can choose to compress
	// based on type.
	switch ext {
	case ".woff2":
		ct = "font/woff2"
	case ".woff":
		ct = "font/woff"
	case ".ttf":
		ct = "font/ttf"
	case ".otf":
		ct = "font/otf"
	case ".eot":
		ct = "application/vnd.ms-fontobject"
	}

	// (8) CSS sanitizer - pass any CSS asset through the same sanitizer
	// that runs on theme.css at install time, so a plugin-shipped stylesheet
	// can't smuggle dangerous selectors past the install-time scrub.
	if ext == ".css" && h.sanitizeCSS != nil {
		sanitized, warns := h.sanitizeCSS(data)
		if len(warns) > 0 {
			slog.WarnContext(r.Context(), "plugin css sanitizer cleaned rules",
				slog.String("plugin_id", pluginID),
				slog.String("path", requestedPath),
				slog.Any("warnings", warns))
		}
		data = sanitized
	}

	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Prevent plugin assets from being embedded in a foreign <iframe>.
	// Assets are loaded via <script>/<link> inside the srcdoc iframe, not
	// framed themselves, so DENY does not interfere with normal operation.
	w.Header().Set("X-Frame-Options", "DENY")
	// If a browser navigates directly to a plugin asset URL, constrain what
	// it can do. 'none' blocks all sub-resources; sandbox adds extra isolation.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	// (6) Cache policy: plugin assets keep STABLE filenames (dist/index.js is
	// rewritten in place by every plugin update and sun-live-*.jpg at runtime),
	// so a far-future immutable would pin the OLD bundle in browsers until a
	// manual hard refresh - a plugin update was invisible client-side
	// (2026-07-22). no-cache forces revalidation on every use; the content
	// ETag turns the unchanged case into a bodyless 304 via ServeContent.
	w.Header().Set("Cache-Control", "no-cache")
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256(data))
	w.Header().Set("ETag", etag)
	h.etags.Set(candidate, assetETag{size: stat.Size(), mtime: stat.ModTime(), etag: etag}, assetETagTTL)
	// (5) CORS for fonts/binary fetch-targets only.
	if corsRequiredAssetExt[ext] {
		w.Header().Set("Access-Control-Allow-Origin", "null")
		w.Header().Set("Access-Control-Allow-Methods", "GET")
	}
	// Name left empty: Content-Type is already pinned above, and ServeContent
	// must not re-derive it (CSS goes out sanitized, not as on disk).
	http.ServeContent(w, r, "", stat.ModTime(), bytes.NewReader(data))
}

func (h *pluginAssetHandler) rejectOversized(r *http.Request, span trace.Span, pluginID, path string, size int64) {
	slog.WarnContext(r.Context(), "plugin asset rejected: size limit",
		slog.String("plugin_id", pluginID),
		slog.String("path", path),
		slog.Int64("size", size),
		slog.Int64("limit", maxPluginAssetBytes))
	span.SetStatus(otelcodes.Error, "asset exceeds size limit")
}

func (h *pluginAssetHandler) handleListThemes(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.plugin.ListThemes")
	defer span.End()
	r = r.WithContext(ctx)
	plugins, err := h.repo.List(r.Context())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "list plugins failed")
		logInternal(r.Context(), "plugin assets: list plugins", err)
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}

	type themeEntry struct {
		ID     string            `json:"id"`
		Name   string            `json:"name"`
		Tokens map[string]string `json:"tokens,omitempty"`
	}
	var result []themeEntry
	for _, p := range plugins {
		if !p.Enabled || !p.IsTheme() {
			continue
		}
		entry := themeEntry{ID: p.ID, Name: p.Name}
		// Parse :root { --X: Y; ... } from theme.css so mobile clients (which
		// can't apply CSS) get a JSON token map to feed their native theme
		// provider. Web ignores this field and keeps loading theme.css.
		if cssBytes, _, readErr := safefs.ReadFile(h.pluginDir, p.ID+"/theme.css", 0); readErr == nil {
			if vars := parseRootCSSVars(cssBytes); len(vars) > 0 {
				entry.Tokens = vars
			}
		}
		result = append(result, entry)
	}

	w.Header().Set("Content-Type", "application/json")
	if encErr := json.NewEncoder(w).Encode(map[string]any{"themes": result}); encErr != nil {
		slog.WarnContext(r.Context(), "encode themes", slog.Any("err", encErr))
	}
}

// parseRootCSSVars extracts `--name: value;` declarations from the first
// `:root { ... }` block in a CSS file. Comments are stripped first; values
// are trimmed but otherwise returned verbatim (the client decides what to
// do with rgba(), hex, etc.). Only the first :root block is read - themes
// don't ship multiple, and this keeps parsing predictable.
func parseRootCSSVars(css []byte) map[string]string {
	src := stripCSSComments(string(css))
	rootIdx := strings.Index(src, ":root")
	if rootIdx < 0 {
		return nil
	}
	open := strings.Index(src[rootIdx:], "{")
	if open < 0 {
		return nil
	}
	open += rootIdx
	close := strings.Index(src[open:], "}")
	if close < 0 {
		return nil
	}
	body := src[open+1 : open+close]
	out := make(map[string]string)
	for decl := range strings.SplitSeq(body, ";") {
		decl = strings.TrimSpace(decl)
		if !strings.HasPrefix(decl, "--") {
			continue
		}
		colon := strings.Index(decl, ":")
		if colon < 0 {
			continue
		}
		name := strings.TrimSpace(decl[:colon])
		value := strings.TrimSpace(decl[colon+1:])
		if name == "" || value == "" {
			continue
		}
		out[name] = value
	}
	return out
}

func stripCSSComments(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '/' && s[i+1] == '*' {
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				break
			}
			i += end + 4
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func registerPluginUploadRoute(mux *http.ServeMux, tokenGen domainuser.TokenGenerator, svc pluginManager, writeRL *ratelimit.Limiter) {
	admin := httpAdminMiddleware(tokenGen)
	mux.HandleFunc("POST /api/v1/admin/plugins/upload", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.plugin.Upload")
		defer span.End()
		r = r.WithContext(ctx)
		// A 100 MB plugin on a slow link can take a while - opt out of the body
		// read deadline (BodyReadTimeoutMiddleware) so it is not cut off.
		ClearBodyReadDeadline(w)
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
			internalError(ctx, w, "create temp file for plugin upload", tmpErr)
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
			internalError(ctx, w, "write plugin upload", cpErr)
			return
		}
		if closeErr := tmpFile.Close(); closeErr != nil {
			span.RecordError(closeErr)
			span.SetStatus(otelcodes.Error, "flush plugin upload failed")
			internalError(ctx, w, "close plugin upload temp file", closeErr)
			return
		}

		// Optional cosign signature: save alongside the zip so the verifier
		// can find it at <zipPath>.sig without the caller needing to know the
		// temp file path.
		if sigFile, _, sigErr := r.FormFile("sig"); sigErr == nil {
			sigPath := tmpName + ".sig"
			defer os.Remove(sigPath) //nolint:errcheck
			if sf, createErr := os.Create(sigPath); createErr == nil {
				// A partial write here produces a truncated .sig, and cosign
				// then reports "signature verification failed" - which sends the
				// admin looking for a bad signature instead of the disk problem
				// that actually happened. Fails closed either way; the point is
				// naming the real cause.
				_, copyErr := io.Copy(sf, sigFile)
				closeErr := sf.Close()
				if copyErr != nil || closeErr != nil {
					slog.WarnContext(ctx, "plugin signature upload could not be stored - verification will fail on a truncated file",
						slog.Any("copy_err", copyErr), slog.Any("close_err", closeErr))
				}
			}
			sigFile.Close()
		}

		// accept_permission_changes=true means the admin has acknowledged the
		// permission diff in the UI and authorizes the update to widen the set.
		installCtx := r.Context()
		if r.FormValue("accept_permission_changes") == "true" {
			installCtx = appplugin.WithAllowPermissionEscalation(installCtx)
		}

		p, installErr := svc.InstallPlugin(installCtx, tmpFile.Name())
		if installErr != nil {
			span.RecordError(installErr)
			span.SetStatus(otelcodes.Error, "install plugin failed")
			slog.ErrorContext(ctx, "install uploaded plugin", slog.Any("err", installErr))
			if errors.Is(installErr, appplugin.ErrPluginPermissionEscalation) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusPreconditionRequired)
				if encErr := json.NewEncoder(w).Encode(map[string]any{
					"message":            "plugin update requires permission acknowledgment",
					"permission_changes": true,
					"new_entries":        extractEscalationEntries(installErr.Error()),
				}); encErr != nil {
					slog.WarnContext(ctx, "encode escalation response", slog.Any("err", encErr))
				}
				return
			}
			if strings.Contains(installErr.Error(), "plugin blocked:") {
				// Prefer the structured PluginBlockedError when available (the
				// report.Safe=false path): it carries the deduplicated
				// `categories` enum list - machine-readable, useful for the
				// admin dashboard + the e2e security spec. Other "plugin
				// blocked:" paths (analyzer-failed, theme.js scanner,
				// native-binary, escalated warnings) don't have categories
				// and fall back to the legacy string-extracted issues.
				var blockErr *appplugin.PluginBlockedError
				var issues, categories []string
				if errors.As(installErr, &blockErr) {
					issues = blockErr.Issues
					categories = blockErr.Categories
				} else {
					issues = extractBlockedIssues(installErr.Error())
				}
				if categories == nil {
					categories = []string{}
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				if encErr := json.NewEncoder(w).Encode(map[string]any{
					"message":    "Plugin blocked by security analysis",
					"blocked":    true,
					"issues":     issues,
					"categories": categories,
				}); encErr != nil {
					slog.WarnContext(ctx, "encode blocked response", slog.Any("err", encErr))
				}
			} else if errors.Is(installErr, appplugin.ErrInvalidManifest) || errors.Is(installErr, appplugin.ErrHashMismatch) {
				// The archive itself is the problem: say why, like the blocked path does.
				messageError(w, installErr.Error(), http.StatusBadRequest)
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
			slog.WarnContext(ctx, "encode plugin response", slog.Any("err", encErr))
		}
	})))
}

func registerPluginPreviewRoute(mux *http.ServeMux, tokenGen domainuser.TokenGenerator, svc pluginManager, writeRL *ratelimit.Limiter) {
	admin := httpAdminMiddleware(tokenGen)
	mux.HandleFunc("POST /api/v1/admin/plugins/preview", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.plugin.Preview")
		defer span.End()
		r = r.WithContext(ctx)
		// A 100 MB plugin on a slow link can take a while - opt out of the body
		// read deadline (BodyReadTimeoutMiddleware) so it is not cut off.
		ClearBodyReadDeadline(w)
		r.Body = http.MaxBytesReader(w, r.Body, 100<<20) // 100MB max

		// JSON body {"source": ...} = preview an install source (URL or path)
		// instead of an uploaded file. Install-from-URL used to skip the
		// preview entirely, so warnings and outbound URLs were only visible
		// once the plugin was already running.
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			var body struct {
				Source string `json:"source"`
			}
			if decErr := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&body); decErr != nil || strings.TrimSpace(body.Source) == "" {
				span.SetStatus(otelcodes.Error, "plugin source required")
				http.Error(w, `{"message":"plugin source required"}`, http.StatusBadRequest)
				return
			}
			preview, previewErr := svc.PreviewSource(r.Context(), strings.TrimSpace(body.Source))
			if previewErr != nil {
				span.RecordError(previewErr)
				span.SetStatus(otelcodes.Error, "preview plugin source failed")
				slog.WarnContext(ctx, "preview plugin source", slog.Any("err", previewErr))
				// The source is what the admin typed: a 404 on the URL or a hash
				// mismatch is THEIR input to fix, so the reason is returned.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				if encErr := json.NewEncoder(w).Encode(map[string]string{"message": "plugin preview failed: " + previewErr.Error()}); encErr != nil {
					slog.WarnContext(ctx, "encode plugin preview error", slog.Any("err", encErr))
				}
				return
			}
			writePluginPreview(ctx, w, preview)
			return
		}

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
			internalError(ctx, w, "create temp file for plugin preview", tmpErr)
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
			internalError(ctx, w, "write plugin preview upload", cpErr)
			return
		}
		if closeErr := tmpFile.Close(); closeErr != nil {
			span.RecordError(closeErr)
			span.SetStatus(otelcodes.Error, "flush plugin preview failed")
			internalError(ctx, w, "close plugin preview temp file", closeErr)
			return
		}

		preview, previewErr := svc.PreviewPlugin(r.Context(), tmpName)
		if previewErr != nil {
			span.RecordError(previewErr)
			span.SetStatus(otelcodes.Error, "preview plugin failed")
			slog.WarnContext(ctx, "preview plugin", slog.Any("err", previewErr))
			http.Error(w, `{"message":"plugin preview failed"}`, http.StatusBadRequest)
			return
		}
		writePluginPreview(ctx, w, preview)
	})))
}

// writePluginPreview renders a preview as the JSON the install dialogs read,
// whether the archive came as an upload or from a source.
func writePluginPreview(ctx context.Context, w http.ResponseWriter, preview *appplugin.PluginPreview) {
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
		// Surface the outbound URLs the analyzer extracted so the admin can
		// SEE which endpoints a plugin will contact, right next to the
		// network warning - not just that it makes network calls. Dedup by
		// URL, annotate with the first location, cap to avoid flooding.
		var urls []string
		seen := make(map[string]bool)
		for _, u := range preview.Analysis.URLs {
			// Trim quote/backtick artifacts left by template-literal
			// extraction so the displayed URL reads cleanly.
			clean := strings.Trim(u.URL, "`'\"")
			if clean == "" || seen[clean] {
				continue
			}
			seen[clean] = true
			label := clean
			if u.File != "" {
				label = fmt.Sprintf("%s  (%s:%d)", clean, u.File, u.Line)
			}
			urls = append(urls, label)
			if len(urls) >= 100 {
				break
			}
		}
		resp.Analysis = &analysisResp{
			Safe:              preview.Analysis.Safe,
			Issues:            issues,
			URLs:              urls,
			TotalFilesScanned: preview.Analysis.FileCount,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
		slog.WarnContext(ctx, "encode plugin preview", slog.Any("err", encErr))
	}
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

// extractEscalationEntries pulls the "permission:network, capability:foo, ..."
// list out of the error message produced by checkPermissionEscalation.
// The format is `...: new entries requested: ENTRY[, ENTRY...]`. Returns the
// list as `["permission:network", "capability:foo"]` so the frontend can
// render the diff modal without re-parsing free-form prose.
func extractEscalationEntries(errMsg string) []string {
	const marker = "new entries requested: "
	idx := strings.Index(errMsg, marker)
	if idx < 0 {
		return nil
	}
	raw := errMsg[idx+len(marker):]
	parts := strings.Split(raw, ", ")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
