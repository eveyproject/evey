package httpapi

import (
	"encoding/json"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"log/slog"
	"net/http"

	domainplugin "github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
)

// publicFrontModule is the safe, public representation of a FrontModule.
type publicFrontModule struct {
	Slot string `json:"slot"`
	Src  string `json:"src"`
}

// publicPlugin is the minimal, non-sensitive plugin shape returned to all
// authenticated users. Fields like analysisReport, sourceHash, state,
// errorMessage, entryPoint, language, installedAt, permissions, and
// capabilities are intentionally absent.
type publicPlugin struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Version       string              `json:"version"`
	Kind          string              `json:"kind"`
	WebRoot       string              `json:"webRoot"`
	Frontend      []publicFrontModule `json:"frontend"`
	ExternalFetch []string            `json:"externalFetch"`
}

func toPublicPlugin(p *domainplugin.Plugin) publicPlugin {
	mods := make([]publicFrontModule, 0, len(p.Frontend))
	for _, f := range p.Frontend {
		mods = append(mods, publicFrontModule{Slot: f.Slot, Src: f.Src})
	}
	ef := p.ExternalFetch
	if ef == nil {
		ef = []string{}
	}
	return publicPlugin{
		ID:            p.ID,
		Name:          p.Name,
		Version:       p.Version,
		Kind:          string(p.Kind),
		WebRoot:       p.WebRoot,
		Frontend:      mods,
		ExternalFetch: ef,
	}
}

// registerPluginPublicRoutes mounts the authenticated-but-not-admin plugin
// endpoints:
//
//	GET /api/v1/plugins            → list enabled+running plugins (public shape)
//	GET /api/v1/plugins/{id}/config → read saved config for an enabled+running plugin
func registerPluginPublicRoutes(mux *http.ServeMux, tokenGen domainuser.TokenGenerator, repo domainplugin.PluginRepository) {
	authMw := httpAuthMiddleware(tokenGen)

	mux.HandleFunc("GET /api/v1/plugins", authMw(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.plugin.ListPublic")
		defer span.End()
		r = r.WithContext(ctx)

		all, err := repo.List(ctx)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "list plugins failed")
			logInternal(r.Context(), "plugins: list", err)
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}

		result := make([]publicPlugin, 0)
		for _, p := range all {
			if !p.Enabled || p.Status != domainplugin.Running {
				continue
			}
			result = append(result, toPublicPlugin(p))
		}

		w.Header().Set("Content-Type", "application/json")
		if encErr := json.NewEncoder(w).Encode(map[string]any{"plugins": result}); encErr != nil {
			slog.WarnContext(ctx, "encode public plugins", slog.Any("err", encErr))
		}
	}))

	mux.HandleFunc("GET /api/v1/plugins/{id}/config", authMw(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.plugin.GetPublicConfig")
		defer span.End()
		r = r.WithContext(ctx)

		pluginID := r.PathValue("id")
		p, err := repo.GetByID(ctx, pluginID)
		if err != nil {
			span.SetStatus(otelcodes.Error, "plugin not found")
			http.Error(w, `{"message":"plugin not found"}`, http.StatusNotFound)
			return
		}
		if !p.Enabled || p.Status != domainplugin.Running {
			// Plugin not in a visible state - return not-found so callers
			// can't enumerate disabled/errored plugins.
			span.SetStatus(otelcodes.Error, "plugin not available")
			http.Error(w, `{"message":"plugin not found"}`, http.StatusNotFound)
			return
		}

		// Only expose config keys the manifest declares non-secret. This is the
		// public, non-admin endpoint and its result is forwarded into the
		// sandboxed iframe (client-side), so secrets (type: password / secret:
		// true) must never appear here - admins read the full config via the
		// admin endpoint instead.
		cfg := p.PublicConfig()

		w.Header().Set("Content-Type", "application/json")
		if encErr := json.NewEncoder(w).Encode(map[string]any{"values": cfg}); encErr != nil {
			slog.WarnContext(ctx, "encode plugin config", slog.Any("err", encErr))
		}
	}))
}
