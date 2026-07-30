package httpapi

import (
	"encoding/json"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"log/slog"
	"net/http"
	"sync"

	otelcodes "go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/config"
	"github.com/evey-project/evey/internal/pkg/urlvalidator"
	"github.com/evey-project/evey/internal/pkg/network"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

const remoteAccessConfigKey = "remote_access"

// RemoteAccessConfigKey is the server config key for remote access settings.
const RemoteAccessConfigKey = remoteAccessConfigKey

func registerRemoteAccessRoutes(mux *http.ServeMux, cfg *config.Config, tokenGen domainuser.TokenGenerator, store remoteAccessPersister, writeRL *ratelimit.Limiter) {
	var mu sync.RWMutex
	admin := httpAdminMiddleware(tokenGen)

	encodeRA := func(w http.ResponseWriter) {
		mu.RLock()
		defer mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		// published/lockoutRisk are the panel's warning signal: "this server is
		// reachable from the internet AND refusing everyone who arrives that
		// way". That state used to be invisible everywhere (2026-07-29), so it
		// is now part of the payload rather than something an operator has to
		// deduce from two unrelated fields.
		published, lockoutRisk := config.RemoteAccessStatus(&cfg.RemoteAccess, cfg.Server.BaseURL)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"enabled":              cfg.RemoteAccess.Enabled,
			"externalUrl":          cfg.RemoteAccess.ExternalURL,
			"serverBaseUrl":        cfg.Server.BaseURL,
			"lanNetworks":          cfg.RemoteAccess.LANNetworks,
			"maxRemoteBitrateKbps": cfg.RemoteAccess.MaxRemoteBitrateKbps,
			"published":            published,
			"lockoutRisk":          lockoutRisk,
		}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}

	mux.HandleFunc("GET /api/v1/admin/remote-access", admin(func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.access.GetRemoteAccess")
		defer span.End()
		encodeRA(w)
	}))

	mux.HandleFunc("PUT /api/v1/admin/remote-access", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.access.UpdateRemoteAccess")
		defer span.End()
		r = r.WithContext(ctx)
		var body struct {
			Enabled              *bool    `json:"enabled"`
			ExternalURL          *string  `json:"externalUrl"`
			LANNetworks          []string `json:"lanNetworks"`
			MaxRemoteBitrateKbps *int     `json:"maxRemoteBitrateKbps"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB cap on JSON body
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "bad request")
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		mu.Lock()
		if body.Enabled != nil {
			cfg.RemoteAccess.Enabled = *body.Enabled
			// The operator just stated a choice. Recording it stops
			// config.ResolveRemoteAccess from inferring over it on the next boot -
			// a deliberate LAN-only instance must stay LAN-only even though it
			// has a public URL configured.
			cfg.RemoteAccess.EnabledExplicit = true
		}
		if body.ExternalURL != nil {
			if *body.ExternalURL == "" {
				cfg.RemoteAccess.ExternalURL = ""
			} else if err := urlvalidator.ValidateExternalURL(*body.ExternalURL); err != nil {
				// The SAME validator the gRPC handler uses. This path had its own
				// inline check (any http/https, any host), which accepted
				// http://, localhost and 169.254.169.254 - and this is the path
				// real traffic takes: Go 1.22+ ServeMux precedence puts this
				// hand-written route ahead of the grpc-gateway one bound to the
				// same method+path, so the hardened validator was dead code on
				// the only route that runs.
				span.RecordError(err)
				span.SetStatus(otelcodes.Error, "invalid externalUrl")
				mu.Unlock()
				http.Error(w, `{"message":"invalid externalUrl: `+err.Error()+`"}`, http.StatusBadRequest)
				return
			} else {
				cfg.RemoteAccess.ExternalURL = *body.ExternalURL
			}
		}
		if body.LANNetworks != nil {
			cfg.RemoteAccess.LANNetworks = body.LANNetworks
		}
		if body.MaxRemoteBitrateKbps != nil {
			cfg.RemoteAccess.MaxRemoteBitrateKbps = *body.MaxRemoteBitrateKbps
		}
		snapshot := cfg.RemoteAccess
		mu.Unlock()

		if store != nil {
			if data, err := json.Marshal(snapshot); err == nil {
				if err2 := store.Set(r.Context(), remoteAccessConfigKey, string(data)); err2 != nil {
					slog.WarnContext(ctx, "remote-access: persist config failed", slog.Any("err", err2))
				}
			}
		}
		encodeRA(w)
	})))

	mux.HandleFunc("GET /api/v1/admin/network-info", admin(func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.access.NetworkInfo")
		defer span.End()
		ifaces, err := network.NetworkInterfaces()
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "list interfaces failed")
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"interfaces": ifaces}); err != nil {
			slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
		}
	}))
}

func registerPlaybackSettingsRoutes(mux *http.ServeMux, cfg *config.Config, tokenGen domainuser.TokenGenerator, writeRL *ratelimit.Limiter) {
	authMw := httpAuthMiddleware(tokenGen)
	admin := httpAdminMiddleware(tokenGen)
	var mu sync.RWMutex

	mux.HandleFunc("GET /api/v1/playback-settings", authMw(func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.access.GetPlaybackSettings")
		defer span.End()
		mu.RLock()
		fwd := cfg.Playback.SkipForwardSeconds
		bwd := cfg.Playback.SkipBackwardSeconds
		mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"skipForwardSeconds":  fwd,
			"skipBackwardSeconds": bwd,
		}); err != nil {
			slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
		}
	}))

	mux.HandleFunc("PUT /api/v1/admin/playback-settings", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.access.UpdatePlaybackSettings")
		defer span.End()
		r = r.WithContext(ctx)
		var body struct {
			SkipForwardSeconds  *int `json:"skipForwardSeconds"`
			SkipBackwardSeconds *int `json:"skipBackwardSeconds"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB cap on JSON body
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "bad request")
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		mu.Lock()
		if body.SkipForwardSeconds != nil && *body.SkipForwardSeconds > 0 {
			cfg.Playback.SkipForwardSeconds = *body.SkipForwardSeconds
		}
		if body.SkipBackwardSeconds != nil && *body.SkipBackwardSeconds > 0 {
			cfg.Playback.SkipBackwardSeconds = *body.SkipBackwardSeconds
		}
		fwd := cfg.Playback.SkipForwardSeconds
		bwd := cfg.Playback.SkipBackwardSeconds
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"skipForwardSeconds":  fwd,
			"skipBackwardSeconds": bwd,
		}); err != nil {
			slog.WarnContext(ctx, "json encode failed", slog.Any("err", err))
		}
	})))
}
