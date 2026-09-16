package httpapi

import (
	"encoding/json"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"log/slog"
	"net/http"
	"strconv"

	otelcodes "go.opentelemetry.io/otel/codes"

	appconfig "github.com/evey-project/evey/internal/app/config"
	"github.com/evey-project/evey/internal/config"
	"github.com/evey-project/evey/internal/pkg/network"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"github.com/evey-project/evey/internal/pkg/urlvalidator"
)

const remoteAccessConfigKey = "remote_access"

// RemoteAccessConfigKey is the server config key for remote access settings.
const RemoteAccessConfigKey = remoteAccessConfigKey

// registerRemoteAccessRoutes reads and writes the remote-access settings under
// the CONFIG's own lock (cfg.WithReadLock / cfg.ApplyServerSettings) rather than
// a mutex private to this file. These fields are shared with the gRPC handler
// and with the request-time gate: a private mutex here synchronized nothing that
// mattered - it did not even exclude the gRPC writer.
func registerRemoteAccessRoutes(mux *http.ServeMux, cfg *config.Config, tokenGen domainuser.TokenGenerator, store remoteAccessPersister, writeRL *ratelimit.Limiter) {
	admin := httpAdminMiddleware(tokenGen)

	encodeRA := func(w http.ResponseWriter) {
		var payload map[string]any
		cfg.WithReadLock(func() {
			// published/lockoutRisk are the panel's warning signal: "this server is
			// reachable from the internet AND refusing everyone who arrives that
			// way". That state used to be invisible everywhere (2026-07-29), so it
			// is now part of the payload rather than something an operator has to
			// deduce from two unrelated fields.
			// RemoteAccessStatus is a pure function over the struct - it takes no
			// lock of its own, so calling it here cannot deadlock.
			published, lockoutRisk := config.RemoteAccessStatus(&cfg.RemoteAccess, cfg.Server.BaseURL)
			payload = map[string]any{
				"enabled":              cfg.RemoteAccess.Enabled,
				"externalUrl":          cfg.RemoteAccess.ExternalURL,
				"serverBaseUrl":        cfg.Server.BaseURL,
				"lanNetworks":          cfg.RemoteAccess.LANNetworks,
				"maxRemoteBitrateKbps": cfg.RemoteAccess.MaxRemoteBitrateKbps,
				"published":            published,
				"lockoutRisk":          lockoutRisk,
			}
		})
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(payload); err != nil {
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
		// Validate BEFORE taking the write lock: the validator is pure, and a
		// rejected request must not have entered the critical section at all.
		// The SAME validator the gRPC handler uses. This path had its own
		// inline check (any http/https, any host), which accepted http://,
		// localhost and 169.254.169.254 - and this is the path real traffic
		// takes: Go 1.22+ ServeMux precedence puts this hand-written route
		// ahead of the grpc-gateway one bound to the same method+path, so the
		// hardened validator was dead code on the only route that runs.
		if body.ExternalURL != nil && *body.ExternalURL != "" {
			if err := urlvalidator.ValidateExternalURL(*body.ExternalURL); err != nil {
				span.RecordError(err)
				span.SetStatus(otelcodes.Error, "invalid externalUrl")
				messageError(w, "invalid externalUrl: "+err.Error(), http.StatusBadRequest)
				return
			}
		}

		var snapshot config.RemoteAccessConfig
		cfg.ApplyServerSettings(func() {
			if body.Enabled != nil {
				cfg.RemoteAccess.Enabled = *body.Enabled
				// The operator just stated a choice. Recording it stops
				// config.ResolveRemoteAccess from inferring over it on the next boot -
				// a deliberate LAN-only instance must stay LAN-only even though it
				// has a public URL configured.
				cfg.RemoteAccess.EnabledExplicit = true
			}
			if body.ExternalURL != nil {
				cfg.RemoteAccess.ExternalURL = *body.ExternalURL
			}
			if body.LANNetworks != nil {
				cfg.RemoteAccess.LANNetworks = body.LANNetworks
			}
			if body.MaxRemoteBitrateKbps != nil {
				cfg.RemoteAccess.MaxRemoteBitrateKbps = *body.MaxRemoteBitrateKbps
			}
			snapshot = cfg.RemoteAccess
		})
		// Persisted OUTSIDE the lock: the store is a DB round-trip, and the
		// request-time gate must never wait on it.
		// The live config keeps the value either way (the gate already reads
		// it); a refused persist is reported as such, never as 200 - the panel
		// would otherwise show "saved" for a setting the next boot forgets.
		if store != nil {
			data, err := json.Marshal(snapshot)
			if err == nil {
				err = store.Set(r.Context(), remoteAccessConfigKey, string(data))
			}
			if err != nil {
				span.RecordError(err)
				span.SetStatus(otelcodes.Error, "persist remote-access config")
				slog.ErrorContext(ctx, "remote-access: persist config failed", slog.Any("err", err))
				http.Error(w, `{"message":"settings applied but not persisted"}`, http.StatusInternalServerError)
				return
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
			logInternal(r.Context(), "network-info: list interfaces", err)
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"interfaces": ifaces}); err != nil {
			slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
		}
	}))
}

// cfgSvc persists the settings this route mutates. It is not optional in
// production: this hand-written route and the gRPC PlaybackSettings handler are
// bound to the SAME method+path, and Go 1.22+ ServeMux precedence gives the
// exact-path route (this one) every request - the gateway is only mounted as a
// catch-all fallback. So the gRPC handler's persistence was unreachable, and an
// operator changing the skip steps saw them apply, then silently revert on the
// next restart when initFromDB read the untouched row back. nil = no
// persistence (hand-built test servers), same as before.
func registerPlaybackSettingsRoutes(mux *http.ServeMux, cfg *config.Config, tokenGen domainuser.TokenGenerator, writeRL *ratelimit.Limiter, cfgSvc *appconfig.Service) {
	authMw := httpAuthMiddleware(tokenGen)
	admin := httpAdminMiddleware(tokenGen)

	mux.HandleFunc("GET /api/v1/playback-settings", authMw(func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.access.GetPlaybackSettings")
		defer span.End()
		// Same discipline as remote-access above: the gRPC PlaybackSettings
		// handler writes these very fields, so the lock has to be the config's.
		fwd, bwd := cfg.EffectivePlaybackSkipSeconds()
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
		var fwd, bwd int
		cfg.ApplyServerSettings(func() {
			if body.SkipForwardSeconds != nil && *body.SkipForwardSeconds > 0 {
				cfg.Playback.SkipForwardSeconds = *body.SkipForwardSeconds
			}
			if body.SkipBackwardSeconds != nil && *body.SkipBackwardSeconds > 0 {
				cfg.Playback.SkipBackwardSeconds = *body.SkipBackwardSeconds
			}
			fwd = cfg.Playback.SkipForwardSeconds
			bwd = cfg.Playback.SkipBackwardSeconds
		})
		// Persisted OUTSIDE the lock, with the SAME keys the gRPC handler and
		// the boot-time read use (appconfig.Key*) - a private copy of the key
		// string in either package would silently stop surviving restarts.
		if cfgSvc != nil {
			var persistFailed []string
			if err := cfgSvc.Set(ctx, appconfig.KeyPlaybackSkipForward, strconv.Itoa(fwd)); err != nil {
				slog.ErrorContext(ctx, "persist playback skip_forward", slog.Any("err", err))
				persistFailed = append(persistFailed, appconfig.KeyPlaybackSkipForward)
			}
			if err := cfgSvc.Set(ctx, appconfig.KeyPlaybackSkipBackward, strconv.Itoa(bwd)); err != nil {
				slog.ErrorContext(ctx, "persist playback skip_backward", slog.Any("err", err))
				persistFailed = append(persistFailed, appconfig.KeyPlaybackSkipBackward)
			}
			// Applied in memory, not saved: say so instead of answering 200.
			if len(persistFailed) > 0 {
				span.SetStatus(otelcodes.Error, "persist playback settings")
				payload, err := json.Marshal(map[string]any{
					"message":         "settings applied but not persisted",
					"unpersistedKeys": persistFailed,
				})
				if err != nil {
					http.Error(w, `{"message":"settings applied but not persisted"}`, http.StatusInternalServerError)
					return
				}
				http.Error(w, string(payload), http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"skipForwardSeconds":  fwd,
			"skipBackwardSeconds": bwd,
		}); err != nil {
			slog.WarnContext(ctx, "json encode failed", slog.Any("err", err))
		}
	})))
}
