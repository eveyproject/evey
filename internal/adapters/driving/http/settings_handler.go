package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	otelcodes "go.opentelemetry.io/otel/codes"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/config"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/logger"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

const (
	cfgKeyServerName       = "server.name"
	cfgKeyTranscodeMaxConc = "transcoding.max_concurrent"
	cfgKeyTranscodeHWAccel = "transcoding.hw_accel_enabled"
	cfgKeyTranscodeHWMode  = "transcoding.hw_mode"
	cfgKeyRateLimitLogin   = "ratelimit.login_max"
	cfgKeyRateLimitGlobal  = "ratelimit.global_max"
	cfgKeyLogLevel         = "log.level"
	cfgKeyWatcherEnabled   = "library.watcher_enabled"
	cfgKeyScanInterval     = "library.periodic_scan_interval"
	cfgKeySkipPreview      = "intro_providers.skip_preview_enabled"
	cfgKeyTranscodeSWAV1   = "transcoding.software_av1_max_height"
	cfgKeyTranscodeAV1En   = "transcoding.av1_enabled"
	cfgKeyTranscodeAV1Max  = "transcoding.av1_max_height"
	// Retention windows (days, 0 = keep forever) + transcode cache budget.
	// Panel-managed per the project rule that operator knobs never live in
	// env-only. The retention sweeps re-read their window every pass, so these
	// apply without a restart; the cache budget is consumed at boot
	// (StartEviction snapshot) and needs one.
	cfgKeyAuditRetention   = "log.audit_retention_days"
	cfgKeyMetricsRetention = "metrics.retention_days"
	cfgKeyQoERetention     = "metrics.qoe_retention_days"
	cfgKeySessionRetention = "playback.session_retention_days"
	cfgKeyTranscodeCacheGB = "transcoding.cache_max_gb"
)

// validAV1MaxHeights are the AV1 output height caps the panel offers. 0 = no cap
// (up to the source/GPU ceiling). AV1 is hardware-only so the ladder can reach
// 2160 (unlike the software ceiling).
var validAV1MaxHeights = map[int]bool{0: true, 720: true, 1080: true, 1440: true, 2160: true}

// validSoftwareAV1Heights are the AV1 software ceiling values the panel offers.
// 0 = "Auto" (calibrated / hardware-only). Any other value force-enables AV1
// software at that height. Bounded to the software bench ladder.
var validSoftwareAV1Heights = map[int]bool{0: true, 480: true, 720: true, 1080: true, 1440: true}

// hwModeController is the slice of CalibrationService the settings handler needs
// to surface + apply the admin HW-acceleration mode. A nil controller (tests,
// or a build without calibration) degrades to "mode reported from config, no
// live re-bench" so the endpoint still works.
type hwModeController interface {
	HWMode() domaintranscoding.HWMode
	SetHWMode(ctx context.Context, mode domaintranscoding.HWMode) error
	// SoftwareAV1MaxOverride / SetSoftwareAV1MaxOverride surface + apply the
	// admin-settable AV1 software ceiling (0 = Auto/hardware-only).
	SoftwareAV1MaxOverride() int
	SetSoftwareAV1MaxOverride(h int)
}

// hwModeControllerOf adapts a concrete *CalibrationService to the interface,
// returning a genuinely nil interface when the pointer is nil (so the handler's
// `hwMode != nil` guard works - a typed nil would slip past it and panic).
func hwModeControllerOf(c *apptranscoding.CalibrationService) hwModeController {
	if c == nil {
		return nil
	}
	return c
}

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
	// reset restores the cfg field to defaultVal. Driven by an EXPLICIT null
	// in the PUT body (the panel's Reset button); nil = not clearable this
	// way (hw mode: "unset" has no runtime meaning, the operator picks one).
	reset           func()
	resetSideEffect func()
}

func registerServerSettingsRoutes(
	mux *http.ServeMux,
	cfg *config.Config,
	store remoteAccessPersister,
	tokenGen TokenVerifier,
	transcodingSvc *apptranscoding.Service,
	hwMode hwModeController,
	loginRL *ratelimit.Limiter,
	globalRL *ratelimit.Limiter,
	writeRL *ratelimit.Limiter,
) {
	// mu guards the handler-local `overridden` map only. The cfg fields these
	// settings mutate are guarded separately by cfg's own lock (cfg.mu, via
	// ApplyServerSettings / withReadLock). Lock ordering is always mu (outer) →
	// cfg's lock (inner) to avoid deadlock: buildResponse takes mu.RLock then
	// cfg.withReadLock; the PUT takes mu.Lock then cfg.ApplyServerSettings.
	var mu sync.RWMutex
	overridden := make(map[string]bool)

	// effectiveHWMode resolves the mode the same way boot does: an explicit
	// configured/overridden value wins, else derive from the legacy knobs. Used
	// by the GET getter so the panel always shows a concrete mode (never "").
	effectiveHWMode := func() domaintranscoding.HWMode {
		if hwMode != nil {
			if m := hwMode.HWMode(); m != "" {
				return m
			}
		}
		return domaintranscoding.ResolveHWMode(cfg.Transcoding.HWMode, cfg.Transcoding.HWAccelEnabled, cfg.Transcoding.HWBackend)
	}
	admin := httpAdminMiddleware(tokenGen)

	source := func(key string) string {
		if overridden[key] {
			return "override"
		}
		return "default"
	}

	settings := []settingDef{
		// Getters read cfg fields RAW (not via the Effective* accessors): the
		// whole settings loop runs inside cfg.withReadLock in buildResponse, and
		// the Effective* accessors take cfg's read lock themselves - calling them
		// here would recursively RLock, which Go's RWMutex forbids.
		{jsonKey: "serverName", cfgKey: cfgKeyServerName, defaultVal: "Evey",
			getter: func() any { return cfg.Server.Name },
			reset:  func() { cfg.Server.Name = "Evey" }},
		{jsonKey: "maxConcurrentTranscodes", cfgKey: cfgKeyTranscodeMaxConc, defaultVal: 2,
			getter: func() any { return cfg.Transcoding.MaxConcurrent },
			reset:  func() { cfg.Transcoding.MaxConcurrent = 2 },
			resetSideEffect: func() {
				if transcodingSvc != nil {
					transcodingSvc.SetMaxConcurrent(2)
				}
			}},
		{jsonKey: "hwAccelEnabled", cfgKey: cfgKeyTranscodeHWAccel, defaultVal: false,
			getter: func() any { return cfg.Transcoding.HWAccelEnabled }},
		{jsonKey: "hwMode", cfgKey: cfgKeyTranscodeHWMode, defaultVal: string(domaintranscoding.HWModeAuto),
			getter: func() any { return string(effectiveHWMode()) }},
		{jsonKey: "loginRateLimit", cfgKey: cfgKeyRateLimitLogin, defaultVal: 10,
			getter: func() any { return cfg.Server.LoginRateLimitMax },
			reset:  func() { cfg.Server.LoginRateLimitMax = 10 },
			resetSideEffect: func() {
				if loginRL != nil {
					loginRL.SetLimit(10)
				}
			}},
		{jsonKey: "globalRateLimit", cfgKey: cfgKeyRateLimitGlobal, defaultVal: 300,
			getter: func() any { return cfg.Server.GlobalRateLimitMax },
			reset:  func() { cfg.Server.GlobalRateLimitMax = 300 },
			resetSideEffect: func() {
				if globalRL != nil {
					globalRL.SetLimit(300)
				}
			}},
		{jsonKey: "logLevel", cfgKey: cfgKeyLogLevel, defaultVal: "info",
			getter:          func() any { return cfg.Log.Level },
			reset:           func() { cfg.Log.Level = "info" },
			resetSideEffect: func() { logger.SetLevel("info") }},
		// Library watcher + periodic scan are consumed at boot, so a change here
		// takes effect on the next restart (no live setter). Surfaced so the
		// values can be managed from the panel instead of the .env.
		{jsonKey: "libraryWatcherEnabled", cfgKey: cfgKeyWatcherEnabled, defaultVal: true,
			getter: func() any { return cfg.Library.WatcherEnabled },
			reset:  func() { cfg.Library.WatcherEnabled = true }},
		{jsonKey: "periodicScanInterval", cfgKey: cfgKeyScanInterval, defaultVal: "2m",
			getter: func() any { return cfg.Library.PeriodicScanInterval.String() },
			reset:  func() { cfg.Library.PeriodicScanInterval = 2 * time.Minute }},
		// Skip-preview is read live by the segments handler, so toggling it
		// takes effect immediately (no restart).
		{jsonKey: "skipPreviewEnabled", cfgKey: cfgKeySkipPreview, defaultVal: false,
			getter: func() any { return cfg.Metadata.SkipPreviewEnabled },
			reset:  func() { cfg.Metadata.SkipPreviewEnabled = false }},
		// AV1 software ceiling: 0 = Auto (calibrated; AV1 stays hardware-only when
		// the bench found software AV1 non-viable). A non-zero value force-enables
		// AV1 software at that height. Read live from the calibration service.
		{jsonKey: "softwareAV1MaxHeight", cfgKey: cfgKeyTranscodeSWAV1, defaultVal: 0,
			getter: func() any {
				if hwMode != nil {
					return hwMode.SoftwareAV1MaxOverride()
				}
				return cfg.Transcoding.SoftwareAV1MaxHeightOverride
			},
			reset: func() { cfg.Transcoding.SoftwareAV1MaxHeightOverride = 0 },
			resetSideEffect: func() {
				if hwMode != nil {
					hwMode.SetSoftwareAV1MaxOverride(0)
				}
			}},
		// AV1 as a transcode OUTPUT codec. Hardware-only (libsvtav1 starves a weak
		// CPU), so this is an explicit opt-in surfaced only when HW accel is on.
		{jsonKey: "av1Enabled", cfgKey: cfgKeyTranscodeAV1En, defaultVal: false,
			getter: func() any { return cfg.Transcoding.AV1Enabled },
			reset:  func() { cfg.Transcoding.AV1Enabled = false }},
		{jsonKey: "av1MaxHeight", cfgKey: cfgKeyTranscodeAV1Max, defaultVal: 0,
			getter: func() any { return cfg.Transcoding.AV1MaxHeight },
			reset:  func() { cfg.Transcoding.AV1MaxHeight = 0 }},
		// Retention windows are re-read by their sweeps at every pass, so a
		// change here applies at the next sweep (<= 24h for the daily ones,
		// seconds for play_sessions) without a restart. 0 = keep forever.
		{jsonKey: "auditRetentionDays", cfgKey: cfgKeyAuditRetention, defaultVal: 0,
			getter: func() any { return cfg.Log.AuditRetentionDays },
			reset:  func() { cfg.Log.AuditRetentionDays = 0 }},
		{jsonKey: "metricsRetentionDays", cfgKey: cfgKeyMetricsRetention, defaultVal: 90,
			getter: func() any { return cfg.Metrics.RetentionDays },
			reset:  func() { cfg.Metrics.RetentionDays = 90 }},
		{jsonKey: "qoeRetentionDays", cfgKey: cfgKeyQoERetention, defaultVal: 30,
			getter: func() any { return cfg.Metrics.QoERetentionDays },
			reset:  func() { cfg.Metrics.QoERetentionDays = 30 }},
		{jsonKey: "playSessionRetentionDays", cfgKey: cfgKeySessionRetention, defaultVal: 0,
			getter: func() any { return cfg.Playback.SessionRetentionDays },
			reset:  func() { cfg.Playback.SessionRetentionDays = 0 }},
		// Consumed at boot (the eviction sweep snapshots its budget), so a
		// change takes effect on the next restart - same contract as the
		// library watcher settings above.
		{jsonKey: "transcodeCacheMaxGB", cfgKey: cfgKeyTranscodeCacheGB, defaultVal: 50,
			getter: func() any { return cfg.Transcoding.CacheMaxGB },
			reset:  func() { cfg.Transcoding.CacheMaxGB = 50 }},
	}

	// Seed `overridden` from the store: after a restart this map started empty
	// and the panel showed source=default while a DB override was live (main's
	// loadOverride HAD applied it to cfg). Boot-time and best-effort - a store
	// read failure just leaves that key displayed as default.
	if store != nil {
		seedCtx := context.Background() // registration runs synchronously at boot
		for _, def := range settings {
			if v, err := store.Get(seedCtx, def.cfgKey); err == nil && v != "" {
				overridden[def.cfgKey] = true
			}
		}
	}

	// buildResponse snapshots every setting. The caller must already hold
	// mu.RLock (guards `overridden`, read via source()); withReadLock adds cfg's
	// read lock (guards the raw cfg field reads in the getters) - mu outer, cfg
	// inner.
	buildResponse := func() map[string]settingValue {
		resp := make(map[string]settingValue, len(settings))
		cfg.WithReadLock(func() {
			for _, s := range settings {
				resp[s.jsonKey] = settingValue{Value: s.getter(), Default: s.defaultVal, Source: source(s.cfgKey)}
			}
		})
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
			HWMode                  *string `json:"hwMode"`
			LoginRateLimit          *int    `json:"loginRateLimit"`
			GlobalRateLimit         *int    `json:"globalRateLimit"`
			LogLevel                *string `json:"logLevel"`
			LibraryWatcherEnabled   *bool   `json:"libraryWatcherEnabled"`
			PeriodicScanInterval    *string `json:"periodicScanInterval"`
			SkipPreviewEnabled      *bool   `json:"skipPreviewEnabled"`
			SoftwareAV1MaxHeight    *int    `json:"softwareAV1MaxHeight"`
			AV1Enabled              *bool   `json:"av1Enabled"`
			AV1MaxHeight            *int    `json:"av1MaxHeight"`
			AuditRetentionDays      *int    `json:"auditRetentionDays"`
			MetricsRetentionDays    *int    `json:"metricsRetentionDays"`
			QoERetentionDays        *int    `json:"qoeRetentionDays"`
			PlaySessionRetention    *int    `json:"playSessionRetentionDays"`
			TranscodeCacheMaxGB     *int    `json:"transcodeCacheMaxGB"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB cap on JSON body
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "bad request")
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(bodyBytes, &body); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "bad request")
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		// Second decode into raw fields: the typed struct cannot tell "absent"
		// from "explicit null", and an explicit null is the panel's Reset - it
		// must clear the override, not be silently ignored (which is what
		// happened until 2026-07-24: the Reset button was a no-op).
		var rawFields map[string]json.RawMessage
		if err := json.Unmarshal(bodyBytes, &rawFields); err != nil {
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
					// The orchestrator takes no runtime cap on concurrent
					// rafales; SetMaxConcurrent above bounds the background
					// transcode jobs, which is the primary use of this setting.
				}},
			{cfgKey: cfgKeyTranscodeHWAccel,
				apply: func() string {
					if body.HWAccelEnabled == nil {
						return ""
					}
					cfg.Transcoding.HWAccelEnabled = *body.HWAccelEnabled
					return strconv.FormatBool(*body.HWAccelEnabled)
				}},
			{cfgKey: cfgKeyTranscodeHWMode,
				apply: func() string {
					if body.HWMode == nil {
						return ""
					}
					mode, ok := domaintranscoding.ParseHWMode(*body.HWMode)
					if !ok {
						return "" // unknown mode → ignore (keep current)
					}
					// Keep the configured intent + the legacy master gate
					// consistent so resolveHW's pre-check agrees: off disables the
					// gate, anything else enables it (the capability still decides
					// the actual backend).
					cfg.Transcoding.HWMode = string(mode)
					cfg.Transcoding.HWAccelEnabled = mode != domaintranscoding.HWModeOff
					return string(mode)
				},
				sideEffect: func() {
					if hwMode == nil || body.HWMode == nil {
						return
					}
					mode, ok := domaintranscoding.ParseHWMode(*body.HWMode)
					if !ok {
						return
					}
					// The re-bench can take ~20 s on a weak host; run it detached
					// so the PUT returns immediately. HWMode() already reflects the
					// new mode synchronously inside SetHWMode, and the mode string
					// is persisted in the main loop, so a browser refresh sees the
					// change even before the bench lands.
					go func() {
						// Detached from the request goroutine → NOT under the HTTP
						// panic middleware. Recover so a panic in the calibration /
						// ffmpeg-probe parsing can't take down the whole server
						// (matches the recover discipline of every other worker).
						defer func() {
							if r := recover(); r != nil {
								slog.Error("settings: HW-mode re-bench goroutine panic recovered",
									slog.Any("panic", r))
							}
						}()
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
						defer cancel()
						if err := hwMode.SetHWMode(ctx, mode); err != nil {
							slog.Warn("settings: live HW-mode re-bench failed (persisted intent still applies on next bench)",
								slog.String("mode", string(mode)), slog.Any("err", err))
						}
					}()
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
			{cfgKey: cfgKeyTranscodeSWAV1,
				apply: func() string {
					if body.SoftwareAV1MaxHeight == nil {
						return ""
					}
					h := *body.SoftwareAV1MaxHeight
					if !validSoftwareAV1Heights[h] {
						return "" // out of the allowed ladder → ignore (keep current)
					}
					cfg.Transcoding.SoftwareAV1MaxHeightOverride = h
					return strconv.Itoa(h)
				},
				sideEffect: func() {
					// Apply live so the next request sees it (no re-bench: the override
					// just overlays the calibrated ceiling in CalibrationService.Current).
					if hwMode == nil || body.SoftwareAV1MaxHeight == nil {
						return
					}
					if h := *body.SoftwareAV1MaxHeight; validSoftwareAV1Heights[h] {
						hwMode.SetSoftwareAV1MaxOverride(h)
					}
				}},
			{cfgKey: cfgKeyTranscodeAV1En,
				apply: func() string {
					if body.AV1Enabled == nil {
						return ""
					}
					cfg.Transcoding.AV1Enabled = *body.AV1Enabled
					return strconv.FormatBool(*body.AV1Enabled)
				}},
			{cfgKey: cfgKeyTranscodeAV1Max,
				apply: func() string {
					if body.AV1MaxHeight == nil || !validAV1MaxHeights[*body.AV1MaxHeight] {
						return "" // absent or off-ladder → ignore (keep current)
					}
					cfg.Transcoding.AV1MaxHeight = *body.AV1MaxHeight
					return strconv.Itoa(*body.AV1MaxHeight)
				}},
			// Retention windows: negative values are ignored (0 already means
			// "keep forever", a negative has no meaning and is likelier a form
			// bug than an intent). No sideEffect needed - the sweeps re-read
			// the window from cfg at every pass.
			{cfgKey: cfgKeyAuditRetention,
				apply: func() string {
					if body.AuditRetentionDays == nil || *body.AuditRetentionDays < 0 {
						return ""
					}
					cfg.Log.AuditRetentionDays = *body.AuditRetentionDays
					return strconv.Itoa(*body.AuditRetentionDays)
				}},
			{cfgKey: cfgKeyMetricsRetention,
				apply: func() string {
					if body.MetricsRetentionDays == nil || *body.MetricsRetentionDays < 0 {
						return ""
					}
					cfg.Metrics.RetentionDays = *body.MetricsRetentionDays
					return strconv.Itoa(*body.MetricsRetentionDays)
				}},
			{cfgKey: cfgKeyQoERetention,
				apply: func() string {
					if body.QoERetentionDays == nil || *body.QoERetentionDays < 0 {
						return ""
					}
					cfg.Metrics.QoERetentionDays = *body.QoERetentionDays
					return strconv.Itoa(*body.QoERetentionDays)
				}},
			{cfgKey: cfgKeySessionRetention,
				apply: func() string {
					if body.PlaySessionRetention == nil || *body.PlaySessionRetention < 0 {
						return ""
					}
					cfg.Playback.SessionRetentionDays = *body.PlaySessionRetention
					return strconv.Itoa(*body.PlaySessionRetention)
				}},
			// Cache budget: 0 would mean "evict everything", which is never the
			// operator's intent from this field - require at least 1 GB. Boot
			// snapshot consumer, so the new budget applies on the next restart.
			{cfgKey: cfgKeyTranscodeCacheGB,
				apply: func() string {
					if body.TranscodeCacheMaxGB == nil || *body.TranscodeCacheMaxGB < 1 {
						return ""
					}
					cfg.Transcoding.CacheMaxGB = *body.TranscodeCacheMaxGB
					return strconv.Itoa(*body.TranscodeCacheMaxGB)
				}},
		}

		// mu (outer) guards `overridden`; cfg's write lock (inner, via
		// ApplyServerSettings) guards the cfg field writes in u.apply(). The DB
		// persist + sideEffect run OUTSIDE cfg's lock so a slow store.Set doesn't
		// block live Effective* readers - only the plain field assignment is
		// serialized against them.
		mu.Lock()
		for _, u := range updates {
			var serialized string
			cfg.ApplyServerSettings(func() { serialized = u.apply() })
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
		// Explicit nulls = the panel's Reset: restore the default, drop the DB
		// row (a surviving row would re-apply the override at next boot) and
		// flip the source back. Keys without a reset closure (hw mode family)
		// ignore null, as before.
		for _, def := range settings {
			raw, present := rawFields[def.jsonKey]
			if !present || string(raw) != "null" || def.reset == nil {
				continue
			}
			cfg.ApplyServerSettings(def.reset)
			overridden[def.cfgKey] = false
			if store != nil {
				if err := store.Delete(r.Context(), def.cfgKey); err != nil {
					slog.Warn("delete server setting override", slog.String("key", def.cfgKey), slog.Any("err", err))
				}
			}
			if def.resetSideEffect != nil {
				def.resetSideEffect()
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
