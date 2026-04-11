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

type settingDef struct {
	jsonKey    string
	cfgKey     string
	defaultVal any
	getter     func() any
	// apply sets the config, returns the serialized value for persistence
	apply      func() string
	sideEffect func()
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

	settings := []settingDef{
		{jsonKey: "serverName", cfgKey: cfgKeyServerName, defaultVal: "Evey",
			getter: func() any { return cfg.EffectiveServerName() }},
		{jsonKey: "maxConcurrentTranscodes", cfgKey: cfgKeyTranscodeMaxConc, defaultVal: 2,
			getter: func() any { return cfg.EffectiveMaxConcurrent() }},
		{jsonKey: "hwAccelEnabled", cfgKey: cfgKeyTranscodeHWAccel, defaultVal: false,
			getter: func() any { return cfg.EffectiveHWAccelEnabled() }},
		{jsonKey: "loginRateLimit", cfgKey: cfgKeyRateLimitLogin, defaultVal: 10,
			getter: func() any { return cfg.Server.LoginRateLimitMax }},
		{jsonKey: "globalRateLimit", cfgKey: cfgKeyRateLimitGlobal, defaultVal: 300,
			getter: func() any { return cfg.Server.GlobalRateLimitMax }},
	}

	buildResponse := func() map[string]settingValue {
		resp := make(map[string]settingValue, len(settings))
		for _, s := range settings {
			resp[s.jsonKey] = settingValue{Value: s.getter(), Default: s.defaultVal, Source: source(s.cfgKey)}
		}
		return resp
	}

	mux.HandleFunc("GET /api/v1/admin/server-settings", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		mu.RLock()
		resp := buildResponse()
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
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
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

		updates := []settingDef{
			{cfgKey: cfgKeyServerName,
				apply: func() string {
					if body.ServerName == nil {
						return ""
					}
					cfg.Server.Name = *body.ServerName
					return *body.ServerName
				}},
			{cfgKey: cfgKeyTranscodeMaxConc,
				apply: func() string {
					if body.MaxConcurrentTranscodes == nil {
						return ""
					}
					cfg.Transcoding.MaxConcurrent = *body.MaxConcurrentTranscodes
					return strconv.Itoa(*body.MaxConcurrentTranscodes)
				},
				sideEffect: func() {
					if transcodingSvc != nil {
						transcodingSvc.SetMaxConcurrent(*body.MaxConcurrentTranscodes)
					}
					if hlsMgr != nil {
						hlsMgr.SetMaxConcurrent(*body.MaxConcurrentTranscodes)
					}
				}},
			{cfgKey: cfgKeyTranscodeHWAccel,
				apply: func() string {
					if body.HWAccelEnabled == nil {
						return ""
					}
					cfg.Transcoding.HWAccelEnabled = *body.HWAccelEnabled
					return strconv.FormatBool(*body.HWAccelEnabled)
				}},
			{cfgKey: cfgKeyRateLimitLogin,
				apply: func() string {
					if body.LoginRateLimit == nil {
						return ""
					}
					cfg.Server.LoginRateLimitMax = *body.LoginRateLimit
					return strconv.Itoa(*body.LoginRateLimit)
				},
				sideEffect: func() {
					if loginRL != nil {
						loginRL.SetLimit(*body.LoginRateLimit)
					}
				}},
			{cfgKey: cfgKeyRateLimitGlobal,
				apply: func() string {
					if body.GlobalRateLimit == nil {
						return ""
					}
					cfg.Server.GlobalRateLimitMax = *body.GlobalRateLimit
					return strconv.Itoa(*body.GlobalRateLimit)
				},
				sideEffect: func() {
					if globalRL != nil {
						globalRL.SetLimit(*body.GlobalRateLimit)
					}
				}},
		}

		mu.Lock()
		for _, u := range updates {
			serialized := u.apply()
			if serialized == "" {
				continue
			}
			overridden[u.cfgKey] = true
			if store != nil {
				if err := store.Set(r.Context(), u.cfgKey, serialized); err != nil {
					slog.Warn("persist server setting", slog.String("key", u.cfgKey), slog.Any("err", err))
				}
			}
			if u.sideEffect != nil {
				u.sideEffect()
			}
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		mu.RLock()
		resp := buildResponse()
		mu.RUnlock()
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}))
}
