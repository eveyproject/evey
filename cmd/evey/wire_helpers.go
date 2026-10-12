package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/gravity-zero/mkvgo/matroska"

	"github.com/evey-project/evey/internal/adapters/driven/ffmpeg"
	"github.com/evey-project/evey/internal/adapters/driven/filesystem"
	mkvgoadapter "github.com/evey-project/evey/internal/adapters/driven/mkvgo"
	webpushadapter "github.com/evey-project/evey/internal/adapters/driven/webpush"
	httpapi "github.com/evey-project/evey/internal/adapters/driving/http"
	applibrary "github.com/evey-project/evey/internal/app/library"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/config"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
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
	return resolveAPIKeyWithBuiltin(ctx, cfgRepo, dbKey, envVar, "")
}

// builtinTMDBKey is the TMDB read token migration 000154 seeds into
// server_config so a fresh install fetches metadata before anyone owns a TMDB
// account. It must stay byte-identical to the migration (a test reads the
// file): it is how resolveAPIKeyWithBuiltin tells "still the default" from
// "an admin's own key".
const builtinTMDBKey = "eyJhbGciOiJIUzI1NiJ9.eyJhdWQiOiI0OTdmMDk2NTBlOTlkNGJjZmM0NDE1NjNiYWVmY2NlYiIsIm5iZiI6MTU4NjUxMTg0NS40MTI5OTk5LCJzdWIiOiI1ZTkwM2ZlNWU2NDBkNjAwMWIxZjIzODMiLCJzY29wZXMiOlsiYXBpX3JlYWQiXSwidmVyc2lvbiI6MX0.nG3dwy0jKg2LKxyjipQScOocwA3pEyj2N8qXkI1V51Q"

// resolveAPIKeyWithBuiltin is resolveAPIKey for a key the database may hold
// as a SHIPPED default: an admin's own value in server_config wins; a
// non-empty env var wins over an empty row OR over the built-in default (and
// is seeded, so it shows in the panel); the built-in default comes last. A
// migration-seeded default must never make an operator's EVEY_*_API_KEY
// silently inert on first boot, which the plain "row wins" rule would do.
func resolveAPIKeyWithBuiltin(ctx context.Context, cfgRepo shared.ServerConfigRepository, dbKey, envVar, builtin string) string {
	stored := ""
	if entry, err := cfgRepo.Get(ctx, dbKey); err == nil {
		stored = entry.Value
	}
	if stored != "" && stored != builtin {
		return stored
	}
	if v := os.Getenv(envVar); v != "" {
		if setErr := cfgRepo.Set(ctx, dbKey, v); setErr != nil {
			slog.Warn("seed api key to db", slog.String("key", dbKey), slog.Any("err", setErr))
		}
		return v
	}
	return stored
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

// titleProberFunc adapts the ffmpeg TitleProber (returns a ProbeReport) to the
// app-layer TitleProber interface (returns the probe plus its stats), so the
// driven adapter needs no dependency on the app package.
type titleProberFunc struct {
	p *ffmpeg.TitleProberAdapter
}

func (a titleProberFunc) Probe(ctx context.Context, sourcePath string, cost domaintranscoding.TitleCost) (domaintranscoding.TitleProbe, apptranscoding.ProbeStats, error) {
	rep, err := a.p.Probe(ctx, sourcePath, cost)
	if err != nil {
		return domaintranscoding.TitleProbe{}, apptranscoding.ProbeStats{}, err
	}
	return rep.Probe, apptranscoding.ProbeStats{Points: rep.Points, CacheHits: rep.CacheHits, Wall: rep.Wall}, nil
}

// trickplayKeyframes hands the trickplay generator mkvgo's keyframe-only
// stream, translating mkvgo's stats into the generator's.
type trickplayKeyframes struct{ a *mkvgoadapter.Adapter }

func (k trickplayKeyframes) WriteKeyframeStream(ctx context.Context, filePath string, everyMs int64, w io.Writer) (ffmpeg.KeyframeStreamStats, error) {
	s, err := k.a.WriteKeyframeStream(ctx, filePath, everyMs, w)
	if errors.Is(err, matroska.ErrNoKeyframeIndex) || errors.Is(err, matroska.ErrNoVideo) || errors.Is(err, matroska.ErrUnsupportedCodec) {
		err = fmt.Errorf("%w: %w", ffmpeg.ErrKeyframesUnavailable, err)
	}
	return ffmpeg.KeyframeStreamStats{
		Samples:        s.Samples,
		PtsMs:          s.PtsMs,
		Skipped:        s.Skipped,
		BytesRead:      s.BytesRead + s.MetaBytesRead,
		MedianCueGapMs: s.MedianCueGapMs,
	}, err
}

func (k trickplayKeyframes) CuesMissKeyframes(ctx context.Context, filePath string) (bool, error) {
	return k.a.CuesMissKeyframes(ctx, filePath)
}
