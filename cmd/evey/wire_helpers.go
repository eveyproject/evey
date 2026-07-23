package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/evey-project/evey/internal/adapters/driven/filesystem"
	webpushadapter "github.com/evey-project/evey/internal/adapters/driven/webpush"
	httpapi "github.com/evey-project/evey/internal/adapters/driving/http"
	applibrary "github.com/evey-project/evey/internal/app/library"
	"github.com/evey-project/evey/internal/config"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	eveylogger "github.com/evey-project/evey/internal/pkg/logger"
)

// wire_helpers.go holds wiring helpers: config/API-key/VAPID resolution (DB
// store first, env fallback), the snapshot frame extractor selection and the
// library watcher bootstrap.

func startWatcher(ctx context.Context, cfg *config.Config, bus shared.EventBus, libraryRepo library.LibraryQueryRepo, libCmdSvc applibrary.LibraryScanner, rootLog *slog.Logger) *applibrary.WatcherService {
	if !cfg.Library.WatcherEnabled {
		return nil
	}
	fsWatcher, err := filesystem.NewFSWatcher()
	if err != nil {
		rootLog.Warn("filesystem watcher init failed, realtime monitoring disabled", slog.Any("err", err))
		return nil
	}
	watcherSvc := applibrary.NewWatcherService(fsWatcher, libraryRepo, libCmdSvc, eveylogger.NewWith(rootLog, "watcher"),
		applibrary.WatcherWithEventBus(bus),
		applibrary.WatcherWithNetworkFSDetector(filesystem.IsNetworkMount),
	)
	if err := watcherSvc.Start(ctx); err != nil {
		rootLog.Warn("watcher start failed", slog.Any("err", err))
		return nil
	}
	rootLog.Info("filesystem watcher started")
	// Active liveness probe: a watch can be established and then silently die
	// (max_user_watches hit as the library grows, an unmount/remount the kernel
	// invalidates and nothing re-arms, queue overflow). The probe writes a
	// hidden file into a watched directory, waits for its own event, deletes it
	// - and on failure marks the library so the periodic scan keeps covering it.
	watcherSvc.StartProbe(ctx, 0)
	go func() {
		<-ctx.Done()
		watcherSvc.Stop()
	}()
	return watcherSvc
}

func resolveAPIKey(ctx context.Context, cfgRepo shared.ServerConfigRepository, dbKey, envVar string) string {
	if entry, err := cfgRepo.Get(ctx, dbKey); err == nil && entry.Value != "" {
		return entry.Value
	}
	if v := os.Getenv(envVar); v != "" {
		if setErr := cfgRepo.Set(ctx, dbKey, v); setErr != nil {
			slog.Warn("seed api key to db", slog.String("key", dbKey), slog.Any("err", setErr))
		}
		return v
	}
	return ""
}

// resolveFlag returns the effective value of an operational (non-secret) flag:
// a server_config DB row wins, else the process env var, else "". This lets an
// admin flip the toggle from /admin/server-config instead of editing the
// container env - a restart is still required, since these flags are read once
// at boot. Both the dotted DB key (canonical) and the raw env-var name are
// accepted as DB keys, so a row keyed either way takes effect. Unlike
// resolveAPIKey it does NOT seed the env value back into the DB (nothing to hide
// and no surprise rows). cfgRepo may be nil (pure-env fallback).
// snapshotFrameExtractor picks the timeline-snapshot extractor: the mkvgo
// keyframe fast path over the exact ffmpeg -ss decoder (default), or the exact
// decoder alone when the operator opted out (transcoding.snapshot_fastpath=off
// / EVEY_SNAPSHOT_FASTPATH=off). The fast path already fails soft per request;
// the flag exists to rule it out entirely when debugging a wrong-frame report.
func snapshotFrameExtractor(ctx context.Context, cfgRepo shared.ServerConfigRepository, infra *infraDeps) httpapi.FrameExtractor {
	if resolveFlag(ctx, cfgRepo, "transcoding.snapshot_fastpath", "EVEY_SNAPSHOT_FASTPATH") == "off" {
		slog.Info("snapshot: mkvgo keyframe fast path disabled (transcoding.snapshot_fastpath=off) - exact ffmpeg -ss extractor only")
		return infra.trickplayGen
	}
	return httpapi.NewFastFrameExtractor(infra.mkvAdapter, infra.trickplayGen, infra.trickplayGen)
}

func resolveFlag(ctx context.Context, cfgRepo shared.ServerConfigRepository, dbKey, envVar string) string {
	if cfgRepo != nil {
		for _, k := range [...]string{dbKey, envVar} {
			if entry, err := cfgRepo.Get(ctx, k); err == nil && entry != nil && entry.Value != "" {
				return entry.Value
			}
		}
	}
	return os.Getenv(envVar)
}

func resolveVAPIDKeys(ctx context.Context, cfgRepo shared.ServerConfigRepository, cfg *config.Config) (string, string) {
	pub := cfg.VAPID.PublicKey
	priv := cfg.VAPID.PrivateKey

	if pub == "" || priv == "" {
		if e, err := cfgRepo.Get(ctx, "vapid.public_key"); err == nil && e.Value != "" {
			pub = e.Value
		}
		if e, err := cfgRepo.Get(ctx, "vapid.private_key"); err == nil && e.Value != "" {
			priv = e.Value
		}
	}

	if pub == "" || priv == "" {
		genPriv, genPub, err := webpushadapter.GenerateVAPIDKeys()
		if err != nil {
			slog.Warn("VAPID key generation failed", slog.Any("err", err))
			return "", ""
		}
		pub, priv = genPub, genPriv
		if err := cfgRepo.Set(ctx, "vapid.public_key", pub); err != nil {
			slog.Warn("store VAPID public key", slog.Any("err", err))
		}
		if err := cfgRepo.Set(ctx, "vapid.private_key", priv); err != nil {
			slog.Warn("store VAPID private key", slog.Any("err", err))
		}
		slog.Info("VAPID keys auto-generated and stored in DB")
	}
	return pub, priv
}
