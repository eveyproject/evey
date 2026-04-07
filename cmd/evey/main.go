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

	httpapi "github.com/gravity-zero/evey/internal/adapters/driving/http"
	"github.com/gravity-zero/evey/internal/config"
	eveylogger "github.com/gravity-zero/evey/internal/pkg/logger"
	"github.com/gravity-zero/evey/internal/pkg/quickconnecthub"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
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
	if v, err2 := deps.cfgStore.Get(ctx, "server.name"); err2 == nil && v != "" {
		cfg.Server.Name = v
	}
	if v, err2 := deps.cfgStore.Get(ctx, "transcoding.max_concurrent"); err2 == nil && v != "" {
		if n, err3 := strconv.Atoi(v); err3 == nil {
			cfg.Transcoding.MaxConcurrent = n
		}
	}
	if v, err2 := deps.cfgStore.Get(ctx, "transcoding.hw_accel_enabled"); err2 == nil && v != "" {
		if b, err3 := strconv.ParseBool(v); err3 == nil {
			cfg.Transcoding.HWAccelEnabled = b
		}
	}
	if v, err2 := deps.cfgStore.Get(ctx, "ratelimit.login_max"); err2 == nil && v != "" {
		if n, err3 := strconv.Atoi(v); err3 == nil {
			cfg.Server.LoginRateLimitMax = n
		}
	}
	if v, err2 := deps.cfgStore.Get(ctx, "ratelimit.global_max"); err2 == nil && v != "" {
		if n, err3 := strconv.Atoi(v); err3 == nil {
			cfg.Server.GlobalRateLimitMax = n
		}
	}

	startMetricsCollector(ctx, db, deps.media.hlsManager, deps.repos.mediaItemRepo)

	qcHub := quickconnecthub.New()
	grpcSrv := buildGRPCServer(deps, cfg, rootLog, logCfg.Dir, ctx, qcHub)
	defer deps.media.hlsManager.CleanupAll()

	grpcAddr := fmt.Sprintf(":%d", cfg.Server.Port)
	httpPort := cfg.Server.Port + 1

	gwMux := registerGateway(grpcAddr)
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

	// Plugin host gRPC server (unix socket)
	if err := deps.engagement.pluginHostServer.Start(); err != nil {
		rootLog.Warn("plugin host server start failed", slog.Any("err", err))
	} else {
		rootLog.Info("plugin host gRPC server started")
	}

	// Auto-start previously running plugins
	go deps.engagement.pluginRuntimeSvc.AutoStartPlugins(ctx)

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
	grpcSrv.Stop()
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
