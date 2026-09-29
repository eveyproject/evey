package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
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
	cfgKeyServerName        = "server.name"
	cfgKeyTranscodeMaxConc  = "transcoding.max_concurrent"
	cfgKeyTranscodeHWAccel  = "transcoding.hw_accel_enabled"
	cfgKeyTranscodeHWMode   = "transcoding.hw_mode"
	cfgKeyRateLimitLogin    = "ratelimit.login_max"
	cfgKeyRateLimitGlobal   = "ratelimit.global_max"
	cfgKeyLogLevel          = "log.level"
	cfgKeyWatcherEnabled    = "library.watcher_enabled"
	cfgKeyScanInterval      = "library.periodic_scan_interval"
	cfgKeySkipPreview       = "intro_providers.skip_preview_enabled"
	cfgKeyIntroAutoSkip     = "intro_providers.auto_skip_enabled"
	cfgKeyTranscodeSWAV1    = "transcoding.software_av1_max_height"
	cfgKeyTranscodeAV1En    = "transcoding.av1_enabled"
	cfgKeyTranscodeAV1Max   = "transcoding.av1_max_height"
	cfgKeyTranscodeLowPower = "transcoding.hw_low_power"
	cfgKeyTranscodeMaxKbps  = "transcoding.max_video_bitrate_kbps"
	// Remote (public-IP) clients' ceiling; default 20 Mb/s.
	cfgKeyTranscodeMaxRemoteKbps = "transcoding.max_video_bitrate_remote_kbps"
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
	// The last five transcoding knobs that were env-only. They are the ones an
	// operator on weak hardware actually reaches for - the 720p-HDR ceiling in
	// particular was diagnosed and fixed in production through
	// EVEY_TRANSCODING_SOFTWARE_HDR_MAX_HEIGHT, which means editing a compose
	// file and recreating the container to change a playback ceiling.
	//
	// All five are read ONCE at boot (wire.go hands them to the orchestrator and
	// to the software ladder), so unlike the retention windows they cannot take
	// effect live. That is why settingDef carries requiresRestart: persisting a
	// value the running process ignores, with nothing saying so, is worse than
	// leaving it in the environment.
	cfgKeyTranscodeSingleVariant = "transcoding.single_variant"
	cfgKeyTranscodeMaxSessions   = "transcoding.max_sessions"
	cfgKeyTranscodeHWPerVariant  = "transcoding.hw_per_variant"
	cfgKeyTranscodeSWMaxHeight   = "transcoding.software_max_height"
	cfgKeyTranscodeSWHDRMaxHt    = "transcoding.software_hdr_max_height"
)

// validAV1MaxHeights are the AV1 output height caps the panel offers. 0 = no cap
// (up to the source/GPU ceiling). AV1 is hardware-only so the ladder can reach
// 2160 (unlike the software ceiling).
var validAV1MaxHeights = map[int]bool{0: true, 720: true, 1080: true, 1440: true, 2160: true}

// validSoftwareAV1Heights are the AV1 software ceiling values the panel offers.
// 0 = "Auto" (calibrated / hardware-only). Any other value force-enables AV1
// software at that height. Bounded to the software bench ladder.
var validSoftwareAV1Heights = map[int]bool{0: true, 480: true, 720: true, 1080: true, 1440: true}

// validSoftwareHeights are the software transcode height ceilings the panel
// offers, for both the SDR and the HDR knob. 0 = no cap. An allow-list rather
// than "any non-negative int" for the same reason the AV1 ladders above use
// one: these values reach the encoder ladder, and a typo like 10800 would build
// a ladder no host can sustain while looking like a saved setting.
var validSoftwareHeights = map[int]bool{0: true, 480: true, 720: true, 1080: true, 1440: true, 2160: true}

// maxTranscodeSessions bounds the concurrent-HLS-session cap. 0 = no limit,
// which is the documented meaning here; the ceiling only exists to reject a
// value that is an obvious mistake (an accidental extra digit) rather than a
// policy.
const maxTranscodeSessions = 1000

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
	// RequiresRestart tells the panel that saving this key persists the value
	// but does NOT change the running server. Several settings already had that
	// contract (the cache budget, the watcher knobs) and it was documented only
	// in a Go comment, so the panel showed a saved value the process was
	// ignoring - the operator's next move is to wonder why the setting "did
	// nothing". Omitted from the JSON when false to keep the payload quiet.
	RequiresRestart bool `json:"requiresRestart,omitempty"`
	// Locked tells the panel the server will refuse a write of this key - the
	// demo instance locks every one of them. The panel greys what the server
	// says is locked and nothing else, so no client carries its own copy of
	// the demo policy.
	Locked bool `json:"locked,omitempty"`
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
	// requiresRestart marks a setting whose value is snapshotted at boot, so
	// saving it persists the choice without changing the running process. Only
	// meaningful on the GET side (it is reported to the panel); the PUT-side
	// `updates` entries do not carry it.
	requiresRestart bool
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
			getter:          func() any { return cfg.Library.WatcherEnabled },
			reset:           func() { cfg.Library.WatcherEnabled = true },
			requiresRestart: true},
		{jsonKey: "periodicScanInterval", cfgKey: cfgKeyScanInterval, defaultVal: "2m",
			getter:          func() any { return cfg.Library.PeriodicScanInterval.String() },
			reset:           func() { cfg.Library.PeriodicScanInterval = 2 * time.Minute },
			requiresRestart: true},
		// Skip-preview is read live by the segments handler, so toggling it
		// takes effect immediately (no restart).
		{jsonKey: "skipPreviewEnabled", cfgKey: cfgKeySkipPreview, defaultVal: false,
			getter: func() any { return cfg.Metadata.SkipPreviewEnabled },
			reset:  func() { cfg.Metadata.SkipPreviewEnabled = false }},
		// Intro auto-skip (all players) is read live by the segments handler
		// too. Off by default.
		{jsonKey: "introAutoSkipEnabled", cfgKey: cfgKeyIntroAutoSkip, defaultVal: false,
			getter: func() any { return cfg.Metadata.IntroAutoSkipEnabled },
			reset:  func() { cfg.Metadata.IntroAutoSkipEnabled = false }},
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
		// Intel low-power encoder: opt-in, it trades quality per bit for GPU headroom.
		{jsonKey: "hwLowPowerEnabled", cfgKey: cfgKeyTranscodeLowPower, defaultVal: false,
			getter: func() any { return cfg.Transcoding.HWLowPowerEnabled },
			reset:  func() { cfg.Transcoding.HWLowPowerEnabled = false }},
		// Served video bitrate ceiling; 0 follows the source bitrate.
		{jsonKey: "maxVideoBitrateKbps", cfgKey: cfgKeyTranscodeMaxKbps, defaultVal: 0,
			getter: func() any { return cfg.Transcoding.MaxVideoBitrateKbps },
			reset:  func() { cfg.Transcoding.MaxVideoBitrateKbps = 0 }},
		// Default 0 = follow LAN / no remote-specific cap; 20000/40000 are opt-in.
		{jsonKey: "maxVideoBitrateRemoteKbps", cfgKey: cfgKeyTranscodeMaxRemoteKbps, defaultVal: 0,
			getter: func() any { return cfg.Transcoding.MaxVideoBitrateRemoteKbps },
			reset:  func() { cfg.Transcoding.MaxVideoBitrateRemoteKbps = 0 }},
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
		{jsonKey: "transcodeCacheMaxGB", cfgKey: cfgKeyTranscodeCacheGB, defaultVal: 10,
			getter:          func() any { return cfg.Transcoding.CacheMaxGB },
			reset:           func() { cfg.Transcoding.CacheMaxGB = 10 },
			requiresRestart: true},
		// The five formerly env-only transcoding knobs. Every one is snapshotted
		// by wire.go at boot (the orchestrator's opts, the HLS session cap, the
		// software ladder), hence requiresRestart on all of them.
		//
		// 0 means "no override" for the three numeric ones, NOT "zero height" /
		// "no sessions": that is how the env vars already behaved, and inverting
		// it here would silently cap a library at 0p or refuse every session.
		{jsonKey: "transcodeMaxSessions", cfgKey: cfgKeyTranscodeMaxSessions, defaultVal: 0,
			getter:          func() any { return cfg.Transcoding.MaxSessions },
			reset:           func() { cfg.Transcoding.MaxSessions = 0 },
			requiresRestart: true},
		{jsonKey: "transcodeSingleVariant", cfgKey: cfgKeyTranscodeSingleVariant, defaultVal: true,
			getter:          func() any { return cfg.Transcoding.SingleVariant },
			reset:           func() { cfg.Transcoding.SingleVariant = true },
			requiresRestart: true},
		{jsonKey: "transcodeHWPerVariant", cfgKey: cfgKeyTranscodeHWPerVariant, defaultVal: false,
			getter:          func() any { return cfg.Transcoding.HWPerVariant },
			reset:           func() { cfg.Transcoding.HWPerVariant = false },
			requiresRestart: true},
		{jsonKey: "softwareMaxHeight", cfgKey: cfgKeyTranscodeSWMaxHeight, defaultVal: 0,
			getter:          func() any { return cfg.Transcoding.SoftwareMaxHeightOverride },
			reset:           func() { cfg.Transcoding.SoftwareMaxHeightOverride = 0 },
			requiresRestart: true},
		{jsonKey: "softwareHDRMaxHeight", cfgKey: cfgKeyTranscodeSWHDRMaxHt, defaultVal: 0,
			getter:          func() any { return cfg.Transcoding.SoftwareHDRMaxHeightOverride },
			reset:           func() { cfg.Transcoding.SoftwareHDRMaxHeightOverride = 0 },
			requiresRestart: true},
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
				resp[s.jsonKey] = settingValue{
					Value:           s.getter(),
					Default:         s.defaultVal,
					Source:          source(s.cfgKey),
					RequiresRestart: s.requiresRestart,
					Locked:          cfg.Server.Demo,
				}
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
			slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
		}
	}))

	mux.HandleFunc("PUT /api/v1/admin/server-settings", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.settings.UpdateServerSettings")
		defer span.End()
		r = r.WithContext(ctx)
		var body struct {
			ServerName              *string `json:"serverName"`
			MaxConcurrentTranscodes *int    `json:"maxConcurrentTranscodes"`
			HWMode                  *string `json:"hwMode"`
			LoginRateLimit          *int    `json:"loginRateLimit"`
			GlobalRateLimit         *int    `json:"globalRateLimit"`
			LogLevel                *string `json:"logLevel"`
			LibraryWatcherEnabled   *bool   `json:"libraryWatcherEnabled"`
			PeriodicScanInterval    *string `json:"periodicScanInterval"`
			SkipPreviewEnabled      *bool   `json:"skipPreviewEnabled"`
			IntroAutoSkip           *bool   `json:"introAutoSkipEnabled"`
			SoftwareAV1MaxHeight    *int    `json:"softwareAV1MaxHeight"`
			AV1Enabled              *bool   `json:"av1Enabled"`
			AV1MaxHeight            *int    `json:"av1MaxHeight"`
			HWLowPowerEnabled       *bool   `json:"hwLowPowerEnabled"`
			MaxVideoBitrateKbps     *int    `json:"maxVideoBitrateKbps"`
			MaxVideoBitrateRemote   *int    `json:"maxVideoBitrateRemoteKbps"`
			AuditRetentionDays      *int    `json:"auditRetentionDays"`
			MetricsRetentionDays    *int    `json:"metricsRetentionDays"`
			QoERetentionDays        *int    `json:"qoeRetentionDays"`
			PlaySessionRetention    *int    `json:"playSessionRetentionDays"`
			TranscodeCacheMaxGB     *int    `json:"transcodeCacheMaxGB"`
			TranscodeMaxSessions    *int    `json:"transcodeMaxSessions"`
			TranscodeSingleVariant  *bool   `json:"transcodeSingleVariant"`
			TranscodeHWPerVariant   *bool   `json:"transcodeHWPerVariant"`
			SoftwareMaxHeight       *int    `json:"softwareMaxHeight"`
			SoftwareHDRMaxHeight    *int    `json:"softwareHDRMaxHeight"`
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
					// "" is the loop's "nothing to apply" signal, so an empty
					// name cannot be told apart from an absent field: assigning
					// it and THEN returning "" blanked the live name and skipped
					// the persist, so the panel showed an empty server until a
					// restart silently put the old one back. Validate before
					// mutating, like every other string setting here does. A
					// nameless server is not a legal value anyway - the panel's
					// Reset (an explicit null) is the way back to "Evey".
					name := strings.TrimSpace(*body.ServerName)
					if name == "" {
						return ""
					}
					cfg.Server.Name = name
					return name
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
							if rec := recover(); rec != nil {
								// No ctx: the re-bench is detached and runs ~20 s
								// past the PUT, which is why it builds its own
								// timeout context below. r.Context() is cancelled
								// long before a panic could land here.
								slog.Error("settings: HW-mode re-bench goroutine panic recovered",
									slog.Any("panic", rec))
							}
						}()
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
						defer cancel()
						if err := hwMode.SetHWMode(ctx, mode); err != nil {
							slog.WarnContext(ctx, "settings: live HW-mode re-bench failed (persisted intent still applies on next bench)",
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
			{cfgKey: cfgKeyIntroAutoSkip,
				apply: func() string {
					if body.IntroAutoSkip == nil {
						return ""
					}
					cfg.Metadata.IntroAutoSkipEnabled = *body.IntroAutoSkip
					return strconv.FormatBool(*body.IntroAutoSkip)
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
			{cfgKey: cfgKeyTranscodeMaxKbps,
				apply: func() string {
					if body.MaxVideoBitrateKbps == nil || *body.MaxVideoBitrateKbps < 0 || *body.MaxVideoBitrateKbps > 200000 {
						return ""
					}
					cfg.Transcoding.MaxVideoBitrateKbps = *body.MaxVideoBitrateKbps
					return strconv.Itoa(*body.MaxVideoBitrateKbps)
				}},
			{cfgKey: cfgKeyTranscodeMaxRemoteKbps,
				apply: func() string {
					if body.MaxVideoBitrateRemote == nil || *body.MaxVideoBitrateRemote < 0 || *body.MaxVideoBitrateRemote > 200000 {
						return ""
					}
					cfg.Transcoding.MaxVideoBitrateRemoteKbps = *body.MaxVideoBitrateRemote
					return strconv.Itoa(*body.MaxVideoBitrateRemote)
				}},
			{cfgKey: cfgKeyTranscodeLowPower,
				apply: func() string {
					if body.HWLowPowerEnabled == nil {
						return ""
					}
					cfg.Transcoding.HWLowPowerEnabled = *body.HWLowPowerEnabled
					return strconv.FormatBool(*body.HWLowPowerEnabled)
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
			// The five formerly env-only knobs. Negatives are refused rather than
			// clamped: 0 already means "no override" here, so a negative is an
			// operator mistake and silently turning it into 0 would look like the
			// save worked. Clearing an override goes through the explicit-null
			// Reset path, not through a sentinel value.
			{cfgKey: cfgKeyTranscodeMaxSessions,
				apply: func() string {
					if body.TranscodeMaxSessions == nil {
						return ""
					}
					n := *body.TranscodeMaxSessions
					if n < 0 || n > maxTranscodeSessions {
						return "" // out of range -> ignore, keep the current value
					}
					cfg.Transcoding.MaxSessions = n
					return strconv.Itoa(n)
				}},
			{cfgKey: cfgKeyTranscodeSingleVariant,
				apply: func() string {
					if body.TranscodeSingleVariant == nil {
						return ""
					}
					cfg.Transcoding.SingleVariant = *body.TranscodeSingleVariant
					return strconv.FormatBool(*body.TranscodeSingleVariant)
				}},
			{cfgKey: cfgKeyTranscodeHWPerVariant,
				apply: func() string {
					if body.TranscodeHWPerVariant == nil {
						return ""
					}
					cfg.Transcoding.HWPerVariant = *body.TranscodeHWPerVariant
					return strconv.FormatBool(*body.TranscodeHWPerVariant)
				}},
			{cfgKey: cfgKeyTranscodeSWMaxHeight,
				apply: func() string {
					if body.SoftwareMaxHeight == nil {
						return ""
					}
					h := *body.SoftwareMaxHeight
					if !validSoftwareHeights[h] {
						return "" // off the allowed ladder -> ignore, keep the current value
					}
					cfg.Transcoding.SoftwareMaxHeightOverride = h
					return strconv.Itoa(h)
				}},
			{cfgKey: cfgKeyTranscodeSWHDRMaxHt,
				apply: func() string {
					if body.SoftwareHDRMaxHeight == nil {
						return ""
					}
					h := *body.SoftwareHDRMaxHeight
					if !validSoftwareHeights[h] {
						return "" // off the allowed ladder -> ignore, keep the current value
					}
					cfg.Transcoding.SoftwareHDRMaxHeightOverride = h
					return strconv.Itoa(h)
				}},
		}

		// mu (outer) guards `overridden`; cfg's write lock (inner, via
		// ApplyServerSettings) guards the cfg field writes in u.apply(). The DB
		// persist + sideEffect run OUTSIDE cfg's lock so a slow store.Set doesn't
		// block live Effective* readers - only the plain field assignment is
		// serialized against them.
		// Keys whose live value changed but whose DB row could not be written.
		// The setting applies now and vanishes at the next restart, so the call
		// must NOT report success - see the 500 below.
		var persistFailed []string
		func() {
			mu.Lock()
			// Deferred so a panicking apply/side effect cannot leave every later
			// settings call waiting on the lock forever.
			defer mu.Unlock()
			for _, u := range updates {
				var serialized string
				cfg.ApplyServerSettings(func() { serialized = u.apply() })
				if serialized == "" {
					continue
				}
				overridden[u.cfgKey] = true
				if store != nil {
					if err := store.Set(r.Context(), u.cfgKey, serialized); err != nil {
						slog.ErrorContext(r.Context(), "persist server setting", slog.String("key", u.cfgKey), slog.Any("err", err))
						persistFailed = append(persistFailed, u.cfgKey)
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
			//
			// Delete the DB row FIRST, and only touch the live config if it
			// succeeded. The reverse order diverges live from persisted on a Delete
			// failure: cfg would read default now but loadOverride re-applies the
			// surviving row at the next restart, silently undoing the Reset. This
			// way a failed Delete leaves BOTH sides on the override - honest (the
			// panel still shows override) and self-consistent across a restart.
			for _, def := range settings {
				raw, present := rawFields[def.jsonKey]
				if !present || string(raw) != "null" || def.reset == nil {
					continue
				}
				if store != nil {
					if err := store.Delete(r.Context(), def.cfgKey); err != nil {
						slog.WarnContext(r.Context(), "delete server setting override, keeping current value", slog.String("key", def.cfgKey), slog.Any("err", err))
						continue
					}
				}
				cfg.ApplyServerSettings(def.reset)
				overridden[def.cfgKey] = false
				if def.resetSideEffect != nil {
					def.resetSideEffect()
				}
			}
		}()

		// A failed persist used to be logged and then answered 200 with
		// `source: "override"` - the panel showed the setting as saved while
		// the DB held nothing, and the value disappeared at the next restart
		// with no trace. Same class as the Delete branch above, which was
		// hardened first. The live config keeps the new value on purpose (apply
		// and its side effect already ran, and there is no previous value to
		// roll back to), so the honest answer is "applied, not saved".
		if len(persistFailed) > 0 {
			span.SetStatus(otelcodes.Error, "persist server setting")
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

		w.Header().Set("Content-Type", "application/json")
		mu.RLock()
		resp := buildResponse()
		mu.RUnlock()
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
		}
	})))
}
