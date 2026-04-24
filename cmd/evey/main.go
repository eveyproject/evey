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

	httpapi "github.com/evey-project/evey/internal/adapters/driving/http"
	"github.com/evey-project/evey/internal/config"
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

	deps, err := buildServices(ctx, cfg, db, rootLog)
	if err != nil {
		return err
	}

	// load RemoteAccess persisted settings from DB
	if raw, err2 := deps.cfgStore.Get(ctx, httpapi.RemoteAccessConfigKey); err2 == nil && raw != "" {
		if err := json.Unmarshal([]byte(raw), &cfg.RemoteAccess); err != nil {
			slog.Warn("unmarshal remote access config", slog.Any("err", err))
		}
	}

	// load server settings overrides from DB
	loadOverride(ctx, deps.cfgStore, "server.name", parseStr, &cfg.Server.Name)
	loadOverride(ctx, deps.cfgStore, "transcoding.max_concurrent", parseInt, &cfg.Transcoding.MaxConcurrent)
	loadOverride(ctx, deps.cfgStore, "transcoding.hw_accel_enabled", parseBool, &cfg.Transcoding.HWAccelEnabled)
	loadOverride(ctx, deps.cfgStore, "ratelimit.login_max", parseInt, &cfg.Server.LoginRateLimitMax)
	loadOverride(ctx, deps.cfgStore, "ratelimit.global_max", parseInt, &cfg.Server.GlobalRateLimitMax)

	startMetricsCollector(ctx, db, deps.media.hlsManager, deps.repos.mediaItemRepo)

	qcHub := quickconnecthub.New()
	grpcSrv := buildGRPCServer(deps, cfg, rootLog, logCfg.Dir, ctx, qcHub)
	defer deps.media.hlsManager.CleanupAll()

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
	deps.backupSvc.StartScheduler(ctx, cfg.Backup.IntervalHours, cfg.Backup.RetentionDays)
	go deps.library.scheduler.Run(ctx)

	// Plugin subsystem is gated by EVEY_PLUGINS_ENABLED. When false the
	// host socket and auto-start are skipped — a plugin enabled in DB
	// from a previous run will stay dormant until the flag is flipped.
	if cfg.Server.PluginsEnabled {
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

	deps.engagement.pluginHostServer.Stop()
	deps.engagement.notifCmdSvc.Stop()
	deps.bus.Stop()
	deps.media.streamTokenStore.Stop()
	grpcSrv.Stop()
	if gwConn != nil {
		if err := gwConn.Close(); err != nil {
			slog.Warn("gateway conn close", slog.Any("err", err))
		}
	}
	if httpSrv != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			slog.Warn("http shutdown", slog.Any("err", err))
		}
	}
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
