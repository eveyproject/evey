package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/ffmpeg"
	"github.com/eveyproject/evey/internal/adapters/driven/filesystem"
	itunesadapter "github.com/eveyproject/evey/internal/adapters/driven/itunes"
	"github.com/eveyproject/evey/internal/adapters/driven/mkvgo"
	musicbrainzadapter "github.com/eveyproject/evey/internal/adapters/driven/musicbrainz"
	"github.com/eveyproject/evey/internal/adapters/driven/opensubtitles"
	"github.com/eveyproject/evey/internal/adapters/driven/pluginrunner"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	proberpkg "github.com/eveyproject/evey/internal/adapters/driven/prober"
	webpushadapter "github.com/eveyproject/evey/internal/adapters/driven/webpush"
	grpcapi "github.com/eveyproject/evey/internal/adapters/driving/grpcapi"
	"github.com/eveyproject/evey/internal/adapters/driving/pluginhost"
	appbackup "github.com/eveyproject/evey/internal/app/backup"
	appfeed "github.com/eveyproject/evey/internal/app/feed"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	apploadtest "github.com/eveyproject/evey/internal/app/loadtest"
	appmedia "github.com/eveyproject/evey/internal/app/media"
	appmusic "github.com/eveyproject/evey/internal/app/music"
	appnotification "github.com/eveyproject/evey/internal/app/notification"
	appplaylist "github.com/eveyproject/evey/internal/app/playlist"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	appsubtitle "github.com/eveyproject/evey/internal/app/subtitle"
	appsyncplay "github.com/eveyproject/evey/internal/app/syncplay"
	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/config"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
	eveylogger "github.com/eveyproject/evey/internal/pkg/logger"
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
	// Demo: no push sender at all, so the subscriptions visitors register are
	// stored and never pushed to - the host contacts no push service. Nor a
	// bench instance, whose subscriptions are a production copy's.
	if vapidPub != "" && vapidPriv != "" && !cfg.Server.Demo && !cfg.Server.BenchInstance {
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
func buildMusicServices(ctx context.Context, cfg *config.Config, infra *infraDeps) (*appmusic.Service, *appmusic.NowPlayingRegistry) {
	r := infra.repos
	bus := infra.bus
	musicScanner := filesystem.NewMusicScanner()
	musicSvc := appmusic.NewService(r.MusicTrack, bus, r.Library, musicScanner)
	musicSvc.WithPlayRepository(r.MusicPlay)
	musicSvc.CommandService().WithShutdownCtx(ctx)
	musicNowPlaying := appmusic.NewNowPlayingRegistry()
	// Same in-process fast-path as the library scan: .m4a (AAC) reads channels /
	// sample rate / esds bitrate head-only via mkvgo, while .flac/.mp3 (not
	// ISO-BMFF/EBML) and ALAC (dropped sample entry) fall through to ffprobe. The
	// dispatcher is stateless, so a fresh instance over the same ffprobe fallback.
	musicSvc.CommandService().WithProber(proberpkg.New(infra.prober, nil, nil))
	// Chaîne de sources d'artwork, dans cet ordre pour une raison : MusicBrainz
	// identifie la sortie par une requête PAR CHAMPS et ne peut donc pas
	// confondre deux albums d'un même artiste, là où la recherche plein-texte
	// d'iTunes retombe sur l'album le plus populaire quand le titre n'est pas
	// indexé (prod 07-25 : pochette de Mixed Signals sur deux albums Linkin
	// Park). iTunes reste en bout de chaîne pour les albums absents de Cover Art
	// Archive - sans risque, il vérifie désormais la correspondance lui aussi.
	// L'artwork EMBARQUÉ dans les fichiers reste prioritaire sur toute la chaîne
	// (cf FetchMissingAlbumArt).
	musicSvc.CommandService().WithAlbumArtFetcher(appmusic.NewChainAlbumArtFetcher(
		musicbrainzadapter.NewAlbumArtFetcher(infra.metadataHTTP),
		itunesadapter.NewAlbumArtFetcher(infra.metadataHTTP),
	), cfg.Transcoding.CacheDir)
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
	playlistSvc.WithMediaChecker(mediaItemCheckerAdapter{repo: r.MediaItemQuery})
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
		buildID().Version, appbackup.WithEventBus(bus),
		appbackup.WithIncludeTelemetry(cfg.EffectiveBackupIncludeTelemetry))
	backupSchedulerLog := eveylogger.NewWith(rootLog, "backup-scheduler")
	backupScheduler := appbackup.NewScheduler(r.BackupSchedule, backupSvc, backupSchedulerLog)
	return backupSvc, backupScheduler
}

// buildSyncPlayServices wires the SyncPlay hub, the cast/native device registry,
// the ephemeral stream-token store and the share-link service. The caller binds
// the stream-token store into the security revoker (a ban must cut live streams)
// once it holds the returned store.
func buildSyncPlayServices(db *sql.DB) (*appsyncplay.Hub, *appsyncplay.DeviceRegistry, *appmedia.StreamTokenStore, *appmedia.ShareService) {
	// The store is built first because the hub needs it: a guest token has no
	// user, so the room dying is the ONLY event that can revoke it.
	// Persisted (hashed) so a restart does not cut every native playback in
	// progress: those players hold one token for the whole title.
	streamTokenStore := appmedia.NewStreamTokenStore(appmedia.DefaultStreamTokenTTL).
		WithPersistence(postgres.NewStreamTokenRepo(db))
	syncPlayHub := appsyncplay.NewHub(postgres.NewSyncPlayRoomRepo(db),
		appsyncplay.WithGuestTokenRevoker(streamTokenStore))
	deviceRegistry := appsyncplay.NewDeviceRegistry()
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
) (*apptranscoding.CapOverrideService, *apptranscoding.KeyframeOnDemand, *proberpkg.KeyframeDispatcher, []func()) {
	r := infra.repos
	// Blocking stoppers for the workers this function STARTS - returned so the
	// composition root can drain them at shutdown before db.Close (the caller
	// appends the keyframe-backfill worker's, which it starts itself).
	var stoppers []func()

	// CMCD-feedback adaptive cap (Phase C). Periodically aggregates
	// playback_qoe_events to lower the calibration-set ceiling on
	// source-classes where viewers consistently starve. TTL'd so a
	// network blip doesn't lock the cap forever.
	capsOverrideRepo := postgres.NewTranscodeCapsOverrideRepo(db)
	capsOverrideSvc := apptranscoding.NewCapOverrideService(
		capsOverrideRepo, apptranscoding.CapOverrideServiceOpts{}, slog.Default(),
	)
	if !benchInstance {
		capsOverrideSvc.Start(ctx)
	}
	stoppers = append(stoppers, capsOverrideSvc.Close)

	complexityWorker := apptranscoding.NewComplexityWorker(
		complexityRepo,
		complexityClassifier,
		mediaItemReaderFunc(func(ctx context.Context, id shared.MediaItemID) (*library.MediaItem, error) {
			return libQuerySvc.GetMediaItem(ctx, string(id))
		}),
		apptranscoding.ComplexityWorkerOpts{},
		slog.Default(),
	).WithVideoDims(r.MediaStream).WithActivePlayback(hlsVODRemover.ActivePlaybackCount)
	if !benchInstance {
		complexityWorker.Start(ctx)
		stoppers = append(stoppers, complexityWorker.Stop)
	}

	// Level backfill: re-probe existing media to fill media_streams.level. The
	// scanner skips unchanged files, so it never back-populates the column the
	// H.264 video-copy (remux) rung needs. Paced conservatively; the rate is
	// env-tunable because it MUST be validated on the real (possibly weak) NAS,
	// not a dev disk. Resolution: server_config DB key wins → env fallback, so an
	// admin can flip it from /admin/server-config (key transcoding.level_backfill,
	// or the raw EVEY_LEVEL_BACKFILL) without touching the container env; a
	// restart is required. transcoding.level_backfill=off disables it entirely.
	if !benchInstance && resolveFlag(ctx, r.ServerCfg, "transcoding.level_backfill", "EVEY_LEVEL_BACKFILL") != "off" {
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
		stoppers = append(stoppers, levelBackfillWorker.Stop)
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
	// Every probed file is measured for tracks stored in blocks, so playback
	// refuses it (file_needs_remux) instead of stalling on it.
	libCmdSvc.WithInterleaveProber(mkvgo.New())

	// kfProber is returned because the PACED keyframe backfill worker (built at
	// the call site, after the load-test runner) shares this same dispatcher.
	return capsOverrideSvc, keyframeOnDemand, kfProber, stoppers
}

// buildLoadTestRunner wires the built-in capacity load test: synthetic viewers
// that hit this server's OWN stack through the loopback (real routing/auth/
// middleware). Returns the runner plus the loopback base URL its viewers hit and
// the reverse-proxy origin real viewers come through (the runner probes the
// latter and falls back to the loopback when it isn't reachable).
func buildLoadTestRunner(cfg *config.Config, libQuerySvc *applibrary.QueryService, hlsVODRemover *apptranscoding.ProducerSelector) (*apploadtest.Runner, string, string) {
	// cfg.Server.Port is the gRPC listener; the HTTP gateway the viewers must
	// hit lives on Port+1 (the same derivation main.go uses for httpSrv).
	loadTestBaseURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port+1)
	// The path real viewers come through: the reverse proxy, with its TLS and
	// buffering. The runner probes it once and falls back to the loopback above
	// when it isn't reachable from in here (no proxy deployed, or a public
	// hostname the container can't resolve).
	loadTestPublicURL := cfg.EffectiveBaseURL()
	// loadTestMinTitleDuration is the floor for a title to be worth pointing a
	// simulated viewer at. A viewer plays in real time for the whole run, so a
	// shorter title loops back on itself and is then served from the page cache,
	// which measures nothing. Episodes and films clear this; trailers and clips
	// do not. The runner narrows it further against the run's own duration (see
	// usableFraction) when it decides where a REPLAYED viewer may start.
	const loadTestMinTitleDuration = 10 * time.Minute
	// loadTestMaxConns sizes the harness's own connection pool. It must exceed
	// any audience the climb can reach, or the test throttles itself.
	const loadTestMaxConns = 2048
	// A proxy on the local network is dialed at its local address rather than
	// through the router's public one (see apploadtest.DialRedirect).
	loadTestRedirect := &apploadtest.DialRedirect{}
	loadTestDialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	loadTestRunner := apploadtest.New(
		&http.Client{
			Timeout: 30 * time.Second,
			// Go's default transport keeps only TWO idle connections per host.
			// With hundreds of simulated viewers hammering one origin, every
			// request past those two pays a fresh TCP (and TLS) handshake, and
			// the HARNESS becomes the bottleneck - the run would measure its own
			// connection churn and blame the server. A real audience is hundreds
			// of separate clients each holding their own keep-alive; these pools
			// are what reproduces that.
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				DialContext:         loadTestRedirect.Wrap(loadTestDialer.DialContext),
				MaxIdleConns:        loadTestMaxConns,
				MaxIdleConnsPerHost: loadTestMaxConns,
				MaxConnsPerHost:     0, // never queue a viewer behind another
				IdleConnTimeout:     90 * time.Second,
			},
		},
		loadTestMediaListerFunc(func(ctx context.Context, limit int, seed uint64) ([]apploadtest.MediaCandidate, error) {
			// The whole playable library, then a sample of it that keeps each
			// kind and bitrate decile in its share (apploadtest.LibrarySample):
			// the same seed lists the same titles as long as the library has not
			// changed, which is what makes a run replayable.
			all, err := playableLibrary(ctx, libQuerySvc.ListMediaItems, loadTestMinTitleDuration)
			if err != nil {
				return nil, err
			}
			out := apploadtest.LibrarySample(all, limit, seed)
			// The source codec+height decide what a viewer actually costs, so a
			// run that cannot name what it drew cannot be read. Batched in one
			// query over the sample; a failure only costs the breakdown, never
			// the run.
			ids := make([]string, 0, len(out))
			for _, c := range out {
				ids = append(ids, c.ID)
			}
			streamsByItem, err := libQuerySvc.GetStreamsForItems(ctx, ids)
			if err != nil {
				slog.WarnContext(ctx, "loadtest: could not resolve candidate source codecs, the run will not be able to name what it served",
					slog.Any("err", err))
			}
			for i := range out {
				out[i].VideoCodec, out[i].VideoHeight, out[i].VideoRange = loadTestSourceClass(streamsByItem[out[i].ID])
			}
			kinds := map[string]int{}
			for _, c := range all {
				kinds["library_"+c.Kind]++
			}
			for _, c := range out {
				kinds["sample_"+c.Kind]++
			}
			slog.InfoContext(ctx, "loadtest: library sampled", slog.Int("playable", len(all)), slog.Int("sampled", len(out)), slog.Any("kinds", kinds))
			return out, nil
		}),
		hlsVODRemover.ActivePlaybackCount,
		slog.Default(),
	)
	// A simulated viewer vanishes without a goodbye, but the encoder session
	// it started lives on (inactivity reap only) - still holding its video
	// admission slot. The runner reaps its own leftovers between tiers and at
	// the end of a run; it never runs over real traffic (Start refuses), so
	// every live session is its own to kill.
	loadTestRunner.WithDialRedirect(loadTestRedirect).WithUplinkProbe(apploadtest.UplinkProbeURL)
	loadTestRunner.WithSessionReaper(func(mediaIDs []string) {
		for _, id := range mediaIDs {
			hlsVODRemover.RemoveSession(id)
		}
	})
	return loadTestRunner, loadTestBaseURL, loadTestPublicURL
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

// loadTestSourceClass picks the first video stream's codec, height and dynamic
// range - what the load test has to decode, and therefore what its verdict is
// about.
func loadTestSourceClass(streams []*library.MediaStream) (string, int, string) {
	for _, st := range streams {
		if st == nil || st.StreamType != library.StreamTypeVideo {
			continue
		}
		h := 0
		if st.Height != nil {
			h = *st.Height
		}
		return st.Codec, h, loadTestVideoRange(st)
	}
	return "", 0, ""
}

// loadTestVideoRange reads a video stream's dynamic range off the same probe
// fields the playback decision reads (ColorTransfer, DoViProfile).
func loadTestVideoRange(st *library.MediaStream) string {
	if st.DoViProfile != nil {
		if *st.DoViProfile == 5 {
			return apploadtest.RangeDoVi5
		}
		return apploadtest.RangeDoVi
	}
	if st.ColorTransfer != nil {
		switch *st.ColorTransfer {
		case "smpte2084":
			return apploadtest.RangeHDR10
		case "arib-std-b67":
			return apploadtest.RangeHLG
		}
	}
	return apploadtest.RangeSDR
}

// loadTestLibraryPage is how many rows one query of the library walk returns.
const loadTestLibraryPage = 1000

// playableLibrary walks every film and episode of the library in id order and
// keeps those with a file a viewer can play for at least minDuration: a
// shorter title loops back on itself and is then served from the page cache.
// Each page is cut down to what the draw needs before the next is read, so a
// library of a hundred thousand episodes costs a few megabytes, not every
// row's synopsis and ratings.
func playableLibrary(ctx context.Context, list func(context.Context, library.ListMediaItemsFilter) ([]*library.MediaItem, error), minDuration time.Duration) ([]apploadtest.MediaCandidate, error) {
	var out []apploadtest.MediaCandidate
	for _, typ := range []library.MediaItemType{library.MovieItem, library.Episode} {
		t := string(typ)
		after := ""
		for {
			page, err := list(ctx, library.ListMediaItemsFilter{Type: &t, AfterID: after, KeysetByID: true, Limit: loadTestLibraryPage})
			if err != nil {
				return nil, err
			}
			for _, it := range page {
				if it == nil {
					continue
				}
				after = string(it.ID)
				if it.File != nil && it.File.Duration.Milliseconds() >= minDuration.Milliseconds() {
					out = append(out, loadTestCandidate(it))
				}
			}
			if len(page) < loadTestLibraryPage {
				break
			}
		}
	}
	return out, nil
}

// loadTestCandidate is what the draw keeps of a playable title.
func loadTestCandidate(it *library.MediaItem) apploadtest.MediaCandidate {
	// The probed bitrate, else the file's own average (size over duration):
	// what a direct-play viewer of it pulls.
	bitrate := it.File.Bitrate
	if secs := int64(it.File.Duration.Seconds()); bitrate <= 0 && secs > 0 {
		bitrate = it.File.Size * 8 / secs
	}
	return apploadtest.MediaCandidate{
		ID:          string(it.ID),
		DurationSec: int(it.File.Duration.Seconds()),
		Kind:        string(it.Type),
		BitrateBps:  bitrate,
		// The path lets the run measure how fast this box READS its media -
		// the ceiling that actually decides a direct-play server's capacity,
		// and the one the kernel cannot report when the library sits on a
		// network share or a 9p/virtiofs mount.
		Path: it.File.Path,
	}
}
