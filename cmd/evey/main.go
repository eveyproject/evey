package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/pluginrunner"
	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	httpapi "github.com/evey-project/evey/internal/adapters/driving/http"
	"github.com/evey-project/evey/internal/config"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	eveylogger "github.com/evey-project/evey/internal/pkg/logger"
	"github.com/evey-project/evey/internal/pkg/quickconnecthub"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "admin" {
		if err := runAdmin(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "admin: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		slog.Error("fatal", slog.Any("err", err))
		os.Exit(1)
	}
}

func run() error {
	hardenProcess() // mark non-dumpable so same-uid plugins can't read /proc/<srv>/{mem,environ}
	startTime := time.Now()
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logCfg := eveylogger.Config{
		Level:         cfg.Log.Level,
		Dir:           cfg.Log.Dir,
		MaxSizeMB:     cfg.Log.MaxSizeMB,
		RetentionDays: cfg.Log.RetentionDays,
		DevMode:       cfg.Log.DevMode,
	}
	rootLog := eveylogger.New(logCfg)
	slog.SetDefault(rootLog)

	if err := cfg.WarnValidate(); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}

	printBootBanner(os.Stderr, cfg, version)

	applyMemoryLimit(rootLog)

	tracingShutdown, err := tracing.Init(context.Background(), tracing.Config{
		Endpoint:    cfg.Tracing.OTLPEndpoint,
		ServiceName: cfg.Tracing.ServiceName,
		SampleRatio: cfg.Tracing.SampleRatio,
		Disabled:    cfg.Tracing.OTLPEndpoint == "",
	})
	if err != nil {
		return fmt.Errorf("tracing init: %w", err)
	}
	if cfg.Tracing.OTLPEndpoint == "" {
		rootLog.Info("tracing disabled (set EVEY_OTLP_ENDPOINT to enable)")
	} else {
		rootLog.Info("tracing enabled",
			slog.String("endpoint", cfg.Tracing.OTLPEndpoint),
			slog.String("service", cfg.Tracing.ServiceName),
			slog.Float64("sample_ratio", cfg.Tracing.SampleRatio),
		)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tracingShutdown(shutdownCtx); err != nil {
			slog.Warn("tracing shutdown", slog.Any("err", err))
		}
	}()

	db, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	rootLog.Info("connected to postgres")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	eveylogger.StartCleanup(ctx, cfg.Log.Dir, cfg.Log.RetentionDays)

	// Hydrate cfg from the persisted admin settings BEFORE building the services.
	// Services take config by VALUE in their constructors (e.g. NewService(...,
	// cfg.Transcoding.MaxConcurrent, ...)), so anything built before this ran was
	// seeded with the ENV default and never saw the operator's setting again: the
	// panel showed the saved value (it re-reads cfg) while the server kept
	// behaving on the default, silently, from the next restart on. Same shape as
	// the HW-gate bug. The store needs only the DB handle, so it can be built here.
	cfgStore := httpapi.NewServerConfigStore(postgres.NewServerConfigRepository(db))

	// load RemoteAccess persisted settings from DB
	if raw, err2 := cfgStore.Get(ctx, httpapi.RemoteAccessConfigKey); err2 == nil && raw != "" {
		if err := json.Unmarshal([]byte(raw), &cfg.RemoteAccess); err != nil {
			slog.Warn("unmarshal remote access config", slog.Any("err", err))
		}
	}

	// load server settings overrides from DB
	loadOverride(ctx, cfgStore, "server.name", parseStr, &cfg.Server.Name)
	loadOverride(ctx, cfgStore, "transcoding.max_concurrent", parseInt, &cfg.Transcoding.MaxConcurrent)
	loadOverride(ctx, cfgStore, "transcoding.hw_accel_enabled", parseBool, &cfg.Transcoding.HWAccelEnabled)
	loadOverride(ctx, cfgStore, "transcoding.hw_mode", parseStr, &cfg.Transcoding.HWMode)
	loadOverride(ctx, cfgStore, "transcoding.software_av1_max_height", parseInt, &cfg.Transcoding.SoftwareAV1MaxHeightOverride)
	loadOverride(ctx, cfgStore, "transcoding.av1_enabled", parseBool, &cfg.Transcoding.AV1Enabled)
	loadOverride(ctx, cfgStore, "transcoding.av1_max_height", parseInt, &cfg.Transcoding.AV1MaxHeight)
	loadOverride(ctx, cfgStore, "ratelimit.login_max", parseInt, &cfg.Server.LoginRateLimitMax)
	loadOverride(ctx, cfgStore, "ratelimit.global_max", parseInt, &cfg.Server.GlobalRateLimitMax)
	loadOverride(ctx, cfgStore, "log.level", parseStr, &cfg.Log.Level)
	loadOverride(ctx, cfgStore, "intro_providers.skip_preview_enabled", parseBool, &cfg.Metadata.SkipPreviewEnabled)
	// The admin toggle persists the MODE only (settings_handler flips the gate in
	// memory and returns the mode string), so derive the gate from the mode here.
	// Loading the two independently lets them disagree: the panel shows hardware
	// on (the mode is persisted) while every restart resets the gate to its env
	// default and quietly sends all transcodes back to software.
	cfg.Transcoding.HWAccelEnabled = domaintranscoding.HWGateEnabled(
		cfg.Transcoding.HWMode, cfg.Transcoding.HWAccelEnabled, cfg.Transcoding.HWBackend)
	eveylogger.SetLevel(cfg.Log.Level) // apply any DB override to the live logger

	deps, err := buildServices(ctx, cfg, db, rootLog)
	if err != nil {
		return err
	}

	startMetricsCollector(ctx, db, deps.media.hlsVODOrch, deps.repos.mediaItemRepo)

	qcHub := quickconnecthub.New()
	grpcSrv := buildGRPCServer(deps, cfg, rootLog, logCfg.Dir, ctx, qcHub)
	defer deps.media.hlsVODOrch.CleanupAll()
	defer func() {
		// Give the QoE sink a brief window to flush pending CMCD events; we
		// don't block shutdown on it though - telemetry that didn't make it
		// is acceptable, a slow shutdown is not.
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := deps.media.qoeSink.Close(closeCtx); err != nil {
			slog.Warn("qoe sink close", slog.Any("err", err))
		}
	}()

	grpcAddr := fmt.Sprintf(":%d", cfg.Server.Port)
	httpPort := cfg.Server.Port + 1

	gwMux, gwConn := registerGateway(grpcAddr)
	var httpSrv *http.Server
	if gwMux != nil {
		handler, err := buildHTTPHandler(gwMux, deps, cfg, startTime, logCfg.Dir, ctx, qcHub)
		if err != nil {
			return err
		}
		httpSrv = &http.Server{
			Addr:              fmt.Sprintf(":%d", httpPort),
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
	}

	deps.engagement.notifCmdSvc.Start(ctx)
	deps.engagement.notifCmdSvc.StartCleanup(ctx)
	deps.user.authSvc.StartCleanup(ctx)
	deps.media.playbackSvc.CommandService().StartCleanup(ctx)
	deps.media.playbackSvc.CommandService().StartProgressFlusher(ctx)
	// Sweep syncplay rooms whose host created them but never opened the ws (or
	// orphans the leave path missed); rooms with a live member are untouched.
	go deps.media.syncPlayHub.StartJanitor(ctx, 10*time.Minute, time.Hour)
	// Daily retention sweep for the audit_log + server_metric tables.
	// audit_log is opt-in (EVEY_AUDIT_LOG_RETENTION_DAYS, default 0 = keep
	// forever) so an existing deployment never silently loses history;
	// server_metric rides its long-standing EVEY_METRICS_RETENTION_DAYS.
	deps.engagement.activityCmdSvc.StartRetentionPurge(ctx, cfg.Log.AuditRetentionDays, cfg.Metrics.RetentionDays)
	// Minute-grained server-metric samples (CPU/RAM, streams, measured
	// streaming bandwidth) - feeds the dashboard rate + the activity stats'
	// consumed-bandwidth integral.
	go deps.engagement.metricsSampler.Run(ctx)
	// env-var-based fallback scheduler (used when DB row has enabled=false and EVEY_BACKUP_INTERVAL_HOURS>0)
	deps.backupSvc.StartScheduler(ctx, cfg.Backup.IntervalHours, cfg.Backup.RetentionDays)
	// DB-driven cron scheduler (admin-configurable via /api/v1/admin/backup-schedule)
	go deps.backupScheduler.Run(ctx)
	go deps.library.scheduler.Run(ctx)

	// Plugin subsystem is gated by EVEY_PLUGINS_ENABLED. When false the
	// host socket and auto-start are skipped - a plugin enabled in DB
	// from a previous run will stay dormant until the flag is flipped.
	if cfg.Server.PluginsEnabled {
		// Preflight the sandbox once so a host that can't run bwrap (Docker's
		// default seccomp blocks unshare/mount) yields an actionable message
		// instead of every plugin crashing later with a cryptic socket timeout.
		if perr := pluginrunner.ProbeSandbox(cfg.Plugins.SandboxMode); perr != nil {
			rootLog.Warn("plugin sandbox preflight FAILED - plugins will crash at startup until this is fixed. "+
				"Apply the bundled seccomp profile to the evey container (security_opt: seccomp=.../seccomp-evey.json), "+
				"or set EVEY_PLUGINS_SANDBOX_MODE=basic to run without bubblewrap (weaker isolation).",
				slog.String("sandbox_mode", cfg.Plugins.SandboxMode), slog.Any("err", perr))
		} else {
			rootLog.Info("plugin sandbox preflight ok", slog.String("sandbox_mode", cfg.Plugins.SandboxMode))
		}
		if err := deps.engagement.pluginHostServer.Start(); err != nil {
			rootLog.Warn("plugin host server start failed", slog.Any("err", err))
		} else {
			rootLog.Info("plugin host gRPC server started")
		}
		go deps.engagement.pluginRuntimeSvc.AutoStartPlugins(ctx)
	} else {
		rootLog.Info("plugin subsystem disabled (set EVEY_PLUGINS_ENABLED=true to activate)")
	}

	errCh := make(chan error, 2)
	go func() {
		if err := grpcSrv.Start(grpcAddr); err != nil {
			errCh <- err
		}
	}()
	if httpSrv != nil {
		go func() {
			rootLog.Info("HTTP gateway listening", slog.Int("port", httpPort))
			if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}()
	}

	select {
	case <-ctx.Done():
		rootLog.Info("shutting down...")
	case err := <-errCh:
		return err
	}

	// Drain the front door FIRST: stop accepting new HTTP and let in-flight
	// requests finish while gRPC - which they proxy to via gwConn - is still up.
	// The previous order stopped gRPC before the HTTP gateway, so any request the
	// still-open gateway accepted in that window got an Unavailable 5xx instead of
	// a clean drain.
	if httpSrv != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			slog.Warn("http shutdown", slog.Any("err", err))
		}
	}
	deps.engagement.pluginHostServer.Stop()
	deps.engagement.notifCmdSvc.Stop()
	deps.media.playbackSvc.CommandService().StopProgressFlusher()
	deps.bus.Stop()
	deps.media.streamTokenStore.Stop()
	deps.media.mediaItemCache.Stop() // stop the cache eviction loop like the other workers
	if gwConn != nil {
		if err := gwConn.Close(); err != nil {
			slog.Warn("gateway conn close", slog.Any("err", err))
		}
	}
	grpcSrv.Stop()
	rootLog.Info("shutdown complete")
	return nil
}

// serverConfigGetter is the minimal contract loadOverride needs from cfgStore.
type serverConfigGetter interface {
	Get(ctx context.Context, key string) (string, error)
}

// loadOverride reads key from store, parses it, and writes to *target.
// Missing / empty / parse-error keys leave target untouched.
func loadOverride[T any](ctx context.Context, store serverConfigGetter, key string,
	parser func(string) (T, bool), target *T) {
	v, err := store.Get(ctx, key)
	if err != nil || v == "" {
		return
	}
	if parsed, ok := parser(v); ok {
		*target = parsed
	}
}

func parseStr(s string) (string, bool) { return s, true }

func parseInt(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	return n, err == nil
}

func parseBool(s string) (bool, bool) {
	b, err := strconv.ParseBool(s)
	return b, err == nil
}

func parseDuration(s string) (time.Duration, bool) {
	d, err := time.ParseDuration(s)
	return d, err == nil && d > 0
}
