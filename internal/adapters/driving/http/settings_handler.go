package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync"

	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	"github.com/gravity-zero/evey/internal/config"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
)

const (
	cfgKeyServerName          = "server.name"
	cfgKeyTranscodeMaxConc    = "transcoding.max_concurrent"
	cfgKeyTranscodeHWAccel    = "transcoding.hw_accel_enabled"
	cfgKeyRateLimitLogin      = "ratelimit.login_max"
	cfgKeyRateLimitGlobal     = "ratelimit.global_max"
)

type settingValue struct {
	Value   any    `json:"value"`
	Default any    `json:"default"`
	Source  string `json:"source"`
}

func registerServerSettingsRoutes(
	mux *http.ServeMux,
	cfg *config.Config,
	store remoteAccessPersister,
	tokenGen *auth.JWTGenerator,
	transcodingSvc *apptranscoding.Service,
	hlsMgr *apptranscoding.HLSManager,
	loginRL *ratelimit.Limiter,
	globalRL *ratelimit.Limiter,
	writeRL *ratelimit.Limiter,
) {
	var mu sync.RWMutex

	defaults := map[string]any{
		cfgKeyServerName:       "Evey",
		cfgKeyTranscodeMaxConc: 2,
		cfgKeyTranscodeHWAccel: false,
		cfgKeyRateLimitLogin:   10,
		cfgKeyRateLimitGlobal:  300,
	}

	overridden := make(map[string]bool)

	requireAdminLocal := func(r *http.Request) bool {
		claims, ok := extractClaims(r, tokenGen)
		return ok && claims.IsAdmin
	}

	source := func(key string) string {
		if overridden[key] {
			return "override"
		}
		return "default"
	}

	mux.HandleFunc("GET /api/v1/admin/server-settings", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		mu.RLock()
		resp := map[string]settingValue{
			"serverName":               {Value: cfg.EffectiveServerName(), Default: defaults[cfgKeyServerName], Source: source(cfgKeyServerName)},
			"maxConcurrentTranscodes":  {Value: cfg.EffectiveMaxConcurrent(), Default: defaults[cfgKeyTranscodeMaxConc], Source: source(cfgKeyTranscodeMaxConc)},
			"hwAccelEnabled":           {Value: cfg.EffectiveHWAccelEnabled(), Default: defaults[cfgKeyTranscodeHWAccel], Source: source(cfgKeyTranscodeHWAccel)},
			"loginRateLimit":           {Value: cfg.Server.LoginRateLimitMax, Default: defaults[cfgKeyRateLimitLogin], Source: source(cfgKeyRateLimitLogin)},
			"globalRateLimit":          {Value: cfg.Server.GlobalRateLimitMax, Default: defaults[cfgKeyRateLimitGlobal], Source: source(cfgKeyRateLimitGlobal)},
		}
		mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	})

	mux.HandleFunc("PUT /api/v1/admin/server-settings", ratelimit.MiddlewareFunc(writeRL, func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		var body struct {
			ServerName              *string `json:"serverName"`
			MaxConcurrentTranscodes *int    `json:"maxConcurrentTranscodes"`
			HWAccelEnabled          *bool   `json:"hwAccelEnabled"`
			LoginRateLimit          *int    `json:"loginRateLimit"`
			GlobalRateLimit         *int    `json:"globalRateLimit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}

		mu.Lock()
		if body.ServerName != nil {
			cfg.Server.Name = *body.ServerName
			overridden[cfgKeyServerName] = true
			if store != nil {
				if err := store.Set(r.Context(), cfgKeyServerName, *body.ServerName); err != nil {
					slog.Warn("persist server setting", slog.String("key", cfgKeyServerName), slog.Any("err", err))
				}
			}
		}
		if body.MaxConcurrentTranscodes != nil {
			n := *body.MaxConcurrentTranscodes
			cfg.Transcoding.MaxConcurrent = n
			overridden[cfgKeyTranscodeMaxConc] = true
			if store != nil {
				if err := store.Set(r.Context(), cfgKeyTranscodeMaxConc, strconv.Itoa(n)); err != nil {
					slog.Warn("persist server setting", slog.String("key", cfgKeyTranscodeMaxConc), slog.Any("err", err))
				}
			}
			if transcodingSvc != nil {
				transcodingSvc.SetMaxConcurrent(n)
			}
			if hlsMgr != nil {
				hlsMgr.SetMaxConcurrent(n)
			}
		}
		if body.HWAccelEnabled != nil {
			cfg.Transcoding.HWAccelEnabled = *body.HWAccelEnabled
			overridden[cfgKeyTranscodeHWAccel] = true
			if store != nil {
				if err := store.Set(r.Context(), cfgKeyTranscodeHWAccel, strconv.FormatBool(*body.HWAccelEnabled)); err != nil {
					slog.Warn("persist server setting", slog.String("key", cfgKeyTranscodeHWAccel), slog.Any("err", err))
				}
			}
		}
		if body.LoginRateLimit != nil {
			n := *body.LoginRateLimit
			cfg.Server.LoginRateLimitMax = n
			overridden[cfgKeyRateLimitLogin] = true
			if store != nil {
				if err := store.Set(r.Context(), cfgKeyRateLimitLogin, strconv.Itoa(n)); err != nil {
					slog.Warn("persist server setting", slog.String("key", cfgKeyRateLimitLogin), slog.Any("err", err))
				}
			}
			if loginRL != nil {
				loginRL.SetLimit(n)
			}
		}
		if body.GlobalRateLimit != nil {
			n := *body.GlobalRateLimit
			cfg.Server.GlobalRateLimitMax = n
			overridden[cfgKeyRateLimitGlobal] = true
			if store != nil {
				if err := store.Set(r.Context(), cfgKeyRateLimitGlobal, strconv.Itoa(n)); err != nil {
					slog.Warn("persist server setting", slog.String("key", cfgKeyRateLimitGlobal), slog.Any("err", err))
				}
			}
			if globalRL != nil {
				globalRL.SetLimit(n)
			}
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		mu.RLock()
		resp := map[string]settingValue{
			"serverName":               {Value: cfg.EffectiveServerName(), Default: defaults[cfgKeyServerName], Source: source(cfgKeyServerName)},
			"maxConcurrentTranscodes":  {Value: cfg.EffectiveMaxConcurrent(), Default: defaults[cfgKeyTranscodeMaxConc], Source: source(cfgKeyTranscodeMaxConc)},
			"hwAccelEnabled":           {Value: cfg.EffectiveHWAccelEnabled(), Default: defaults[cfgKeyTranscodeHWAccel], Source: source(cfgKeyTranscodeHWAccel)},
			"loginRateLimit":           {Value: cfg.Server.LoginRateLimitMax, Default: defaults[cfgKeyRateLimitLogin], Source: source(cfgKeyRateLimitLogin)},
			"globalRateLimit":          {Value: cfg.Server.GlobalRateLimitMax, Default: defaults[cfgKeyRateLimitGlobal], Source: source(cfgKeyRateLimitGlobal)},
		}
		mu.RUnlock()
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}))
}
