package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	otelcodes "go.opentelemetry.io/otel/codes"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/config"
	"github.com/evey-project/evey/internal/pkg/logger"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

const (
	cfgKeyServerName       = "server.name"
	cfgKeyTranscodeMaxConc = "transcoding.max_concurrent"
	cfgKeyTranscodeHWAccel = "transcoding.hw_accel_enabled"
	cfgKeyRateLimitLogin   = "ratelimit.login_max"
	cfgKeyRateLimitGlobal  = "ratelimit.global_max"
	cfgKeyLogLevel         = "log.level"
	cfgKeyWatcherEnabled   = "library.watcher_enabled"
	cfgKeyScanInterval     = "library.periodic_scan_interval"
	cfgKeySkipPreview      = "intro_providers.skip_preview_enabled"
)

var validLogLevels = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}

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
	tokenGen TokenVerifier,
	transcodingSvc *apptranscoding.Service,
	hlsMgr *apptranscoding.HLSManager,
	loginRL *ratelimit.Limiter,
	globalRL *ratelimit.Limiter,
	writeRL *ratelimit.Limiter,
) {
	var mu sync.RWMutex
	overridden := make(map[string]bool)
	admin := httpAdminMiddleware(tokenGen)

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
		{jsonKey: "logLevel", cfgKey: cfgKeyLogLevel, defaultVal: "info",
			getter: func() any { return cfg.Log.Level }},
		// Library watcher + periodic scan are consumed at boot, so a change here
		// takes effect on the next restart (no live setter). Surfaced so the
		// values can be managed from the panel instead of the .env.
		{jsonKey: "libraryWatcherEnabled", cfgKey: cfgKeyWatcherEnabled, defaultVal: true,
			getter: func() any { return cfg.Library.WatcherEnabled }},
		{jsonKey: "periodicScanInterval", cfgKey: cfgKeyScanInterval, defaultVal: "2m",
			getter: func() any { return cfg.Library.PeriodicScanInterval.String() }},
		// Skip-preview is read live by the segments handler, so toggling it
		// takes effect immediately (no restart).
		{jsonKey: "skipPreviewEnabled", cfgKey: cfgKeySkipPreview, defaultVal: false,
			getter: func() any { return cfg.Metadata.SkipPreviewEnabled }},
	}

	buildResponse := func() map[string]settingValue {
		resp := make(map[string]settingValue, len(settings))
		for _, s := range settings {
			resp[s.jsonKey] = settingValue{Value: s.getter(), Default: s.defaultVal, Source: source(s.cfgKey)}
		}
		return resp
	}

	mux.HandleFunc("GET /api/v1/admin/server-settings", admin(func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.settings.GetServerSettings")
		defer span.End()
		mu.RLock()
		resp := buildResponse()
		mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}))

	mux.HandleFunc("PUT /api/v1/admin/server-settings", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.settings.UpdateServerSettings")
		defer span.End()
		r = r.WithContext(ctx)
		var body struct {
			ServerName              *string `json:"serverName"`
			MaxConcurrentTranscodes *int    `json:"maxConcurrentTranscodes"`
			HWAccelEnabled          *bool   `json:"hwAccelEnabled"`
			LoginRateLimit          *int    `json:"loginRateLimit"`
			GlobalRateLimit         *int    `json:"globalRateLimit"`
			LogLevel                *string `json:"logLevel"`
			LibraryWatcherEnabled   *bool   `json:"libraryWatcherEnabled"`
			PeriodicScanInterval    *string `json:"periodicScanInterval"`
			SkipPreviewEnabled      *bool   `json:"skipPreviewEnabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "bad request")
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
			{cfgKey: cfgKeyLogLevel,
				apply: func() string {
					if body.LogLevel == nil || !validLogLevels[*body.LogLevel] {
						return "" // unknown level → ignore (keep current)
					}
					cfg.Log.Level = *body.LogLevel
					return *body.LogLevel
				},
				sideEffect: func() {
					if body.LogLevel != nil {
						logger.SetLevel(*body.LogLevel)
					}
				}},
			{cfgKey: cfgKeyWatcherEnabled,
				apply: func() string {
					if body.LibraryWatcherEnabled == nil {
						return ""
					}
					cfg.Library.WatcherEnabled = *body.LibraryWatcherEnabled
					return strconv.FormatBool(*body.LibraryWatcherEnabled)
				}},
			{cfgKey: cfgKeyScanInterval,
				apply: func() string {
					if body.PeriodicScanInterval == nil {
						return ""
					}
					d, err := time.ParseDuration(*body.PeriodicScanInterval)
					if err != nil || d <= 0 {
						return "" // invalid duration → ignore
					}
					cfg.Library.PeriodicScanInterval = d
					return d.String()
				}},
			{cfgKey: cfgKeySkipPreview,
				apply: func() string {
					if body.SkipPreviewEnabled == nil {
						return ""
					}
					cfg.Metadata.SkipPreviewEnabled = *body.SkipPreviewEnabled
					return strconv.FormatBool(*body.SkipPreviewEnabled)
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
	})))
}
