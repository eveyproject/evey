package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/ffmpeg"
	"github.com/evey-project/evey/internal/adapters/driven/filesystem"
	itunesadapter "github.com/evey-project/evey/internal/adapters/driven/itunes"
	"github.com/evey-project/evey/internal/adapters/driven/opensubtitles"
	"github.com/evey-project/evey/internal/adapters/driven/pluginrunner"
	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	proberpkg "github.com/evey-project/evey/internal/adapters/driven/prober"
	webpushadapter "github.com/evey-project/evey/internal/adapters/driven/webpush"
	grpcapi "github.com/evey-project/evey/internal/adapters/driving/grpcapi"
	"github.com/evey-project/evey/internal/adapters/driving/pluginhost"
	appbackup "github.com/evey-project/evey/internal/app/backup"
	appfeed "github.com/evey-project/evey/internal/app/feed"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmedia "github.com/evey-project/evey/internal/app/media"
	appmusic "github.com/evey-project/evey/internal/app/music"
	appnotification "github.com/evey-project/evey/internal/app/notification"
	appplaylist "github.com/evey-project/evey/internal/app/playlist"
	appplugin "github.com/evey-project/evey/internal/app/plugin"
	appsubtitle "github.com/evey-project/evey/internal/app/subtitle"
	appsyncplay "github.com/evey-project/evey/internal/app/syncplay"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/config"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	eveylogger "github.com/evey-project/evey/internal/pkg/logger"
)

// wire_services.go holds the self-contained per-domain service builders that
// buildAppServices used to inline. Each takes only what it reads (never the
// tangled transcoding locals) and returns the services it produces, so the
// composition root stays a readable sequence while these blocks are named,
// isolated units. The interleaved media/transcoding core is deliberately NOT
// extracted here - its cross-block variable threading makes a helper a net loss.

// buildNotificationServices wires the notification command/query services,
// enabling Web Push when VAPID keys resolve and locale-aware push titles when a
// translator is present. Registers the command service on the event bus.
func buildNotificationServices(ctx context.Context, cfg *config.Config, infra *infraDeps, rootLog *slog.Logger) (*appnotification.CommandService, *appnotification.QueryService) {
	r := infra.repos
	bus := infra.bus
	var notifOpts []appnotification.Option
	vapidPub, vapidPriv := resolveVAPIDKeys(ctx, r.ServerCfg, cfg)
	if vapidPub != "" && vapidPriv != "" {
		wpSender := webpushadapter.NewSender(vapidPub, vapidPriv, cfg.VAPID.Subject)
		notifOpts = append(notifOpts, appnotification.WithPush(wpSender, r.PushSubscription))
		rootLog.Info("Web Push (VAPID) notifications enabled")
	}
	notifOpts = append(notifOpts, appnotification.WithAdminLister(r.Profile))
	if infra.translator != nil {
		notifOpts = append(notifOpts, appnotification.WithTranslator(infra.translator))
	}
	notifCmdSvc := appnotification.NewCommandService(r.Notification, r.NotificationSettings, r.WatchHistoryAnalyzer, r.Profile, bus, notifOpts...)
	notifCmdSvc.Register()
	notifQuerySvc := appnotification.NewQueryService(r.Notification, r.NotificationSettings, appnotification.WithPushSubs(r.PushSubscription))
	return notifCmdSvc, notifQuerySvc
}

// buildFeedServices wires the feed command/query services (the analyzer cached
// behind a 5-minute TTL) and ensures every existing library has a feed config.
func buildFeedServices(ctx context.Context, infra *infraDeps) (*appfeed.CommandService, *appfeed.QueryService) {
	r := infra.repos
	bus := infra.bus
	feedCmdSvc := appfeed.NewCommandService(r.FeedConfigCmd, r.FeedConfigQuery, appfeed.CommandWithEventBus(bus))
	cachedAnalyzer := appfeed.NewCachedAnalyzer(r.WatchHistoryAnalyzer, 5*time.Minute, appfeed.CacheWithEventBus(bus))
	feedQuerySvc := appfeed.NewQueryService(r.FeedConfigQuery, r.FeedQuerier, cachedAnalyzer)

	if libs, libErr := r.Library.List(ctx, library.ListLibrariesFilter{}); libErr == nil {
		for _, lib := range libs {
			if err := feedCmdSvc.EnsureLibraryFeedConfig(ctx, lib.ID, lib.Name); err != nil {
				slog.Warn("ensure library feed config", slog.String("library", string(lib.ID)), slog.Any("err", err))
			}
		}
	}
	return feedCmdSvc, feedQuerySvc
}

// buildMusicServices wires the music service (in-process mkvgo prober fast-path,
// iTunes album art, optional search index) and its now-playing registry.
func buildMusicServices(cfg *config.Config, infra *infraDeps) (*appmusic.Service, *appmusic.NowPlayingRegistry) {
	r := infra.repos
	bus := infra.bus
	musicScanner := filesystem.NewMusicScanner()
	musicSvc := appmusic.NewService(r.MusicTrack, bus, r.Library, musicScanner)
	musicSvc.WithPlayRepository(r.MusicPlay)
	musicNowPlaying := appmusic.NewNowPlayingRegistry()
	// Same in-process fast-path as the library scan: .m4a (AAC) reads channels /
	// sample rate / esds bitrate head-only via mkvgo, while .flac/.mp3 (not
	// ISO-BMFF/EBML) and ALAC (dropped sample entry) fall through to ffprobe. The
	// dispatcher is stateless, so a fresh instance over the same ffprobe fallback.
	musicSvc.CommandService().WithProber(proberpkg.New(infra.prober, nil, nil))
	musicSvc.CommandService().WithAlbumArtFetcher(itunesadapter.NewAlbumArtFetcher(infra.metadataHTTP), cfg.Transcoding.CacheDir)
	if infra.musicSearch != nil {
		musicSvc.CommandService().WithSearchIndex(infra.musicSearch)
		musicSvc.QueryService().WithSearchIndex(infra.musicSearch)
	}
	return musicSvc, musicNowPlaying
}

// buildPlaylistServices wires the playlist service (with its media/library
// access checkers) and the smart-playlist generator.
func buildPlaylistServices(infra *infraDeps) (*appplaylist.Service, *appplaylist.GeneratorService) {
	r := infra.repos
	playlistSvc := appplaylist.NewService(r.Playlist, r.PlaylistItem, r.SmartPlaylist, r.MediaItemQuery)
	playlistSvc.WithMusicTrackChecker(musicTrackCheckerAdapter{repo: r.MusicTrack})
	// AddItem resolves the media item (for its library) and gates it on the
	// acting profile's library access, so a profile cannot add - and then read
	// the metadata of - media from a library it has no access to.
	playlistSvc.WithMediaChecker(mediaItemCheckerAdapter{repo: r.MediaItem})
	playlistSvc.WithLibraryAccessChecker(playlistLibraryAccessAdapter{profiles: r.Profile, access: r.LibraryAccess})
	playlistGenSvc := appplaylist.NewGeneratorService(r.MusicTrack, r.MusicPlay, r.Playlist, r.PlaylistItem)
	return playlistSvc, playlistGenSvc
}

// buildSubtitleService wires the OpenSubtitles-backed subtitle service, or nil
// when no API key is configured (the feature stays cleanly disabled).
func buildSubtitleService(ctx context.Context, infra *infraDeps, libQuerySvc *applibrary.QueryService, rootLog *slog.Logger) *appsubtitle.Service {
	r := infra.repos
	osAPIKey := resolveAPIKey(ctx, r.ServerCfg, "api.opensubtitles_key", "EVEY_OPENSUBTITLES_API_KEY")
	if osAPIKey == "" {
		return nil
	}
	osClient := opensubtitles.New(osAPIKey, os.Getenv("EVEY_OPENSUBTITLES_USERNAME"), os.Getenv("EVEY_OPENSUBTITLES_PASSWORD"), infra.metadataHTTP)
	rootLog.Info("opensubtitles integration enabled")
	return appsubtitle.NewService(r.Subtitle, osClient, opensubtitles.NewLibraryMediaGetter(libQuerySvc))
}

// buildBackupServices wires the backup service (filesystem store + pg_dump) and
// its schedule runner.
func buildBackupServices(cfg *config.Config, infra *infraDeps, rootLog *slog.Logger) (*appbackup.Service, *appbackup.Scheduler) {
	r := infra.repos
	bus := infra.bus
	backupSvc := appbackup.NewService(filesystem.NewBackupStore(cfg.Backup.Dir),
		postgres.NewDatabaseDumper(cfg.DB.Host, cfg.DB.Port, cfg.DB.User, cfg.DB.Password, cfg.DB.DBName, cfg.DB.SSLMode),
		version, appbackup.WithEventBus(bus))
	backupSchedulerLog := eveylogger.NewWith(rootLog, "backup-scheduler")
	backupScheduler := appbackup.NewScheduler(r.BackupSchedule, backupSvc, backupSchedulerLog)
	return backupSvc, backupScheduler
}

// buildSyncPlayServices wires the SyncPlay hub, the cast/native device registry,
// the ephemeral stream-token store and the share-link service. The caller binds
// the stream-token store into the security revoker (a ban must cut live streams)
// once it holds the returned store.
func buildSyncPlayServices(db *sql.DB) (*appsyncplay.Hub, *appsyncplay.DeviceRegistry, *appmedia.StreamTokenStore, *appmedia.ShareService) {
	syncPlayHub := appsyncplay.NewHub(postgres.NewSyncPlayRoomRepo(db))
	deviceRegistry := appsyncplay.NewDeviceRegistry()
	streamTokenStore := appmedia.NewStreamTokenStore(appmedia.DefaultStreamTokenTTL)
	shareLinkRepo := postgres.NewShareLinkRepo(db)
	shareSvc := appmedia.NewShareService(shareLinkRepo)
	return syncPlayHub, deviceRegistry, streamTokenStore, shareSvc
}

// startTranscodingWorkers starts the background transcoding workers (CMCD-feedback
// cap override, per-title complexity classification, media_streams.level backfill
// and keyframe indexing) and returns the two services the rest of the app keeps a
// handle on: the cap-override service and the on-demand keyframe filler (also
// wired as the scan-time keyframe warmer). Each worker yields to live playback via
// hlsVODRemover's active-playback count.
func startTranscodingWorkers(
	ctx context.Context,
	db *sql.DB,
	infra *infraDeps,
	libCmdSvc *applibrary.CommandService,
	libQuerySvc *applibrary.QueryService,
	complexityRepo domaintranscoding.ComplexityRepo,
	complexityClassifier *ffmpeg.ComplexityClassifier,
	hlsVODRemover *apptranscoding.ProducerSelector,
) (*apptranscoding.CapOverrideService, *apptranscoding.KeyframeOnDemand, *proberpkg.KeyframeDispatcher) {
	r := infra.repos

	// CMCD-feedback adaptive cap (Phase C). Periodically aggregates
	// playback_qoe_events to lower the calibration-set ceiling on
	// source-classes where viewers consistently starve. TTL'd so a
	// network blip doesn't lock the cap forever.
	capsOverrideRepo := postgres.NewTranscodeCapsOverrideRepo(db)
	capsOverrideSvc := apptranscoding.NewCapOverrideService(
		capsOverrideRepo, apptranscoding.CapOverrideServiceOpts{}, slog.Default(),
	)
	capsOverrideSvc.Start(ctx)

	complexityWorker := apptranscoding.NewComplexityWorker(
		complexityRepo,
		complexityClassifier,
		mediaItemReaderFunc(func(ctx context.Context, id shared.MediaItemID) (*library.MediaItem, error) {
			return libQuerySvc.GetMediaItem(ctx, string(id))
		}),
		apptranscoding.ComplexityWorkerOpts{},
		slog.Default(),
	).WithVideoDims(r.MediaStream)
	complexityWorker.Start(ctx)

	// Level backfill: re-probe existing media to fill media_streams.level. The
	// scanner skips unchanged files, so it never back-populates the column the
	// H.264 video-copy (remux) rung needs. Paced conservatively; the rate is
	// env-tunable because it MUST be validated on the real (possibly weak) NAS,
	// not a dev disk. Resolution: server_config DB key wins → env fallback, so an
	// admin can flip it from /admin/server-config (key transcoding.level_backfill,
	// or the raw EVEY_LEVEL_BACKFILL) without touching the container env; a
	// restart is required. transcoding.level_backfill=off disables it entirely.
	if resolveFlag(ctx, r.ServerCfg, "transcoding.level_backfill", "EVEY_LEVEL_BACKFILL") != "off" {
		levelOpts := apptranscoding.LevelBackfillOpts{}
		if v := resolveFlag(ctx, r.ServerCfg, "transcoding.level_backfill_batch", "EVEY_LEVEL_BACKFILL_BATCH"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				levelOpts.BatchSize = n
			}
		}
		if v := resolveFlag(ctx, r.ServerCfg, "transcoding.level_backfill_pause_ms", "EVEY_LEVEL_BACKFILL_PAUSE_MS"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				levelOpts.ItemPause = time.Duration(n) * time.Millisecond
			}
		}
		if v := resolveFlag(ctx, r.ServerCfg, "transcoding.level_backfill_idle_sec", "EVEY_LEVEL_BACKFILL_IDLE_SEC"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				levelOpts.IdleInterval = time.Duration(n) * time.Second
			}
		}
		levelBackfillWorker := apptranscoding.NewLevelBackfillWorker(
			r.MediaStream,
			mediaItemReaderFunc(func(ctx context.Context, id shared.MediaItemID) (*library.MediaItem, error) {
				return libQuerySvc.GetMediaItem(ctx, string(id))
			}),
			infra.prober,
			levelOpts,
			slog.Default(),
		)
		// Same live-session signal as the keyframe worker below: yield to
		// viewers instead of probing alongside them (2026-07-19 incident family).
		levelBackfillWorker.WithActivePlayback(hlsVODRemover.ActivePlaybackCount)
		levelBackfillWorker.Start(ctx)
	}

	// Keyframe backfill: index keyframe times for copy-eligible media so the
	// byte-range/per-segment copy rung can cut deterministic, keyframe-aligned
	// segments. ON by default since the mkvgo era (2026-07-08 decision): the
	// probe reads the container's own seek index in milliseconds and even a
	// cue-less MKV costs a bounded in-process scan, so the historical
	// ffprobe-era caution (opt-in + ultra-slow pacing) no longer applies -
	// out-of-the-box installs get the copy rung. Opt OUT with
	// transcoding.keyframe_backfill = off (or EVEY_KEYFRAME_BACKFILL=off);
	// pacing stays tunable via the server_config keys below. Runs after the
	// level backfill by construction (the candidate query requires a known
	// level); backs off to the polite pace while playback sessions are active
	// (WithActivePlayback); a library scan-completed event nudges it after a
	// 10-min settle so fresh additions are indexed without waiting the idle
	// cycle. Resolution: server_config DB key wins → env fallback; a restart
	// is required.
	// Read the container seek index (mkvgo: MP4 stss / Matroska Cues) head-only
	// before the multi-minute ffprobe packet scan; falls back to ffprobe when the
	// container yields no index. Always on, like the metadata prober (nil gates).
	// Shared by the paced background worker below AND the on-demand inline fill
	// at playback negotiation.
	kfProber := proberpkg.NewKeyframeDispatcher(infra.prober, nil, nil)
	// On-demand keyframe fill at GetStreamInfo: unconditional (unlike the paced
	// worker) because its budget context caps the attempt at the fast-path cost
	// - a file the fast-path can't index stays on the worker's slow path.
	keyframeOnDemand := apptranscoding.NewKeyframeOnDemand(r.MediaStream, kfProber, slog.Default()).
		WithFrameRateAnalyzer(infra.mkvAdapter, r.MediaStream).
		WithActivePlayback(hlsVODRemover.ActivePlaybackCount)
	// First-play fast path (Phase 2 consumption): a fresh file whose serving
	// plan says direct-play/remux gets its keyframe index warmed right at scan
	// time through the bounded on-demand walker, so the copy rung is live on
	// the very first request instead of after the debounced backfill.
	libCmdSvc.WithIngestKeyframeWarmer(keyframeOnDemand.TryIndex)

	// kfProber is returned because the PACED keyframe backfill worker (built at
	// the call site, after the load-test runner) shares this same dispatcher.
	return capsOverrideSvc, keyframeOnDemand, kfProber
}

// buildPluginSubsystem wires the whole plugin subsystem: installer, runtime,
// query and token services, the security dashboard, the idle (low-memory
// suspend) manager and the plugin-host gRPC server. It also registers the runner
// callbacks (IPC result, host registry, SDK stubs), the memory-profile live
// re-evaluation and the startup orphan-dir reap. Returns the seven services the
// app keeps plus the resolved memory profile (consumed downstream by the
// software HLS producer's low-memory knobs).
func buildPluginSubsystem(ctx context.Context, infra *infraDeps, rootLog *slog.Logger) (
	*appplugin.InstallerService,
	*appplugin.RuntimeService,
	*appplugin.QueryService,
	*appplugin.TokenService,
	*appplugin.SecurityDashboardService,
	*appplugin.IdleManager,
	*pluginhost.HostServer,
	string,
) {
	r := infra.repos
	bus := infra.bus

	permChecker := appplugin.NewPermissionChecker(r.Plugin)
	pluginFS := filesystem.NewPluginFileSystem()
	// Refresh the server-shipped SDK stubs at every plugin start, so installed
	// plugins pick up host-contract changes (new RPCs) after a server upgrade.
	// Safe here: AutoStartPlugins only runs after wiring completes (main.go).
	infra.pluginRunner.WithSDKStubs(pluginFS)
	cssSanitizer := pluginrunner.NewCSSSanitizer()
	runtimeChecker := pluginrunner.NewRuntimeAvailability()
	slog.Info("plugin runtimes detected", slog.Any("available", runtimeChecker.Available()))
	pluginInstallerSvc := appplugin.NewInstallerService(r.Plugin, infra.pluginRunner, infra.manifestParser, bus,
		appplugin.InstallerWithInstaller(infra.pluginInstaller),
		appplugin.InstallerWithAnalyzer(infra.pluginAnalyzer),
		appplugin.InstallerWithSignatureVerifier(infra.pluginSigVerifier),
		appplugin.InstallerWithPermissionChecker(permChecker),
		appplugin.InstallerWithFileSystem(pluginFS),
		appplugin.InstallerWithCSSSanitizer(cssSanitizer),
		appplugin.InstallerWithRuntimeChecker(runtimeChecker),
	)
	// Reap any plugin install dirs left behind by uninstalls that crashed
	// before the FS removal succeeded. Async so it never blocks startup.
	go func() {
		if n := pluginInstallerSvc.CleanupOrphans(ctx); n > 0 {
			slog.Info("plugin orphans cleaned at startup", slog.Int("removed", n))
		}
	}()
	pluginRuntimeSvc := appplugin.NewRuntimeService(r.Plugin, r.PluginRuntime, infra.pluginRunner, infra.manifestParser, bus,
		appplugin.RuntimeWithPermissionChecker(permChecker),
		appplugin.RuntimeWithShutdownCtx(ctx),
	)
	infra.pluginRunner.WithIPCCallback(pluginRuntimeSvc.HandleIPCResult)
	pluginQuerySvc := appplugin.NewQueryService(r.Plugin, r.PluginRuntime)
	pluginTokenSvc := appplugin.NewTokenService(r.Plugin, infra.tokenGen)
	pluginSecurityDashboardSvc := appplugin.NewSecurityDashboardService(r.Plugin, bus, infra.pluginRunner.EffectiveSandboxMode).
		WithSandboxStatus(func() appplugin.SandboxStatus {
			return appplugin.SandboxStatus(infra.pluginRunner.SandboxStatus())
		})

	// Low-memory profile: suspend idle plugin processes and resume them on
	// demand (widget asset fetch). The manager runs its sweep loop for the
	// whole server lifetime; when the profile is normal every tick no-ops.
	// Profile changes (admin setting) and the setting's "auto" resolution are
	// re-evaluated live on ServerConfigChanged.
	pluginIdleMgr := appplugin.NewIdleManager(r.Plugin, infra.pluginRunner, appplugin.DefaultPluginIdleTimeout)
	memProfile, memBudget := appplugin.ResolveMemoryProfile(ctx, r.ServerCfg)
	rootLog.Info("memory profile resolved",
		slog.String("profile", memProfile),
		slog.Int64("budget_mb", memBudget>>20))
	pluginIdleMgr.SetEnabled(ctx, memProfile == appplugin.MemoryProfileLow)
	go pluginIdleMgr.Run(ctx)
	bus.Subscribe(shared.ServerConfigChanged{}.EventName(), func(evtCtx context.Context, evt shared.Event) {
		if c, ok := evt.(shared.ServerConfigChanged); ok && c.Key == appplugin.KeyMemoryProfile {
			p, _ := appplugin.ResolveMemoryProfile(evtCtx, r.ServerCfg)
			pluginIdleMgr.SetEnabled(evtCtx, p == appplugin.MemoryProfileLow)
		}
	})

	permInterceptor := grpcapi.PluginPermissionInterceptor(permChecker)
	pluginHostSrv := pluginhost.NewHostServer(r.MediaItemQuery, r.Plugin, bus, infra.pluginLogger, permInterceptor).
		WithDataStore(r.PluginData).
		WithActivityFunc(pluginIdleMgr.Touch)
	infra.pluginRunner.WithHostRegistry(pluginHostSrv)
	infra.pluginLogger.StartCleanup(ctx, &pluginIDLister{repo: r.Plugin})

	return pluginInstallerSvc, pluginRuntimeSvc, pluginQuerySvc, pluginTokenSvc, pluginSecurityDashboardSvc, pluginIdleMgr, pluginHostSrv, memProfile
}
