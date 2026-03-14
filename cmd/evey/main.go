package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"google.golang.org/grpc"

	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	appbackup "github.com/gravity-zero/evey/internal/app/backup"
	appcollection "github.com/gravity-zero/evey/internal/app/collection"
	appfavorite "github.com/gravity-zero/evey/internal/app/favorite"
	appfeed "github.com/gravity-zero/evey/internal/app/feed"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	appmkv "github.com/gravity-zero/evey/internal/app/mkv"
	appnotification "github.com/gravity-zero/evey/internal/app/notification"
	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	appplaylist "github.com/gravity-zero/evey/internal/app/playlist"
	appsubtitle "github.com/gravity-zero/evey/internal/app/subtitle"
	appwebhook "github.com/gravity-zero/evey/internal/app/webhook"
	"github.com/gravity-zero/evey/internal/adapters/driven/ebml"
	"github.com/gravity-zero/evey/internal/adapters/driven/ffmpeg"
	"github.com/gravity-zero/evey/internal/adapters/driven/filesystem"
	meili "github.com/gravity-zero/evey/internal/adapters/driven/meilisearch"
	"github.com/gravity-zero/evey/internal/adapters/driven/opensubtitles"
	metadataadapter "github.com/gravity-zero/evey/internal/adapters/driven/metadata"
	"github.com/gravity-zero/evey/internal/adapters/driven/pluginrunner"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/gravity-zero/evey/internal/config"
	"github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/backupcrypto"
	eveymetrics "github.com/gravity-zero/evey/internal/pkg/metrics"
	"github.com/gravity-zero/evey/internal/pkg/eventbus"
	eveylogger "github.com/gravity-zero/evey/internal/pkg/logger"
	"github.com/gravity-zero/evey/internal/pkg/network"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

const version = "0.1.0"

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

	// --- Logger ---
	logCfg := eveylogger.Config{
		Level:         cfg.Log.Level,
		Dir:           cfg.Log.Dir,
		MaxSizeMB:     cfg.Log.MaxSizeMB,
		RetentionDays: cfg.Log.RetentionDays,
		DevMode:       cfg.Log.DevMode,
	}
	rootLog := eveylogger.New(logCfg)
	slog.SetDefault(rootLog)

	// --- Database ---
	db, err := sql.Open("postgres", cfg.DB.DSN())
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()
	if err2 := db.Ping(); err2 != nil {
		return fmt.Errorf("ping db: %w", err2)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	rootLog.Info("connected to postgres")

	if err2 := postgres.RunMigrations(db); err2 != nil {
		return fmt.Errorf("migrations: %w", err2)
	}

	// --- Repositories ---
	serverCfgRepo := postgres.NewServerConfigRepository(db)
	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	refreshTokenRepo := postgres.NewRefreshTokenRepository(db)
	apiKeyRepo := postgres.NewAPIKeyRepository(db)
	inviteLinkRepo := postgres.NewInviteLinkRepository(db)
	quickConnectRepo := postgres.NewQuickConnectRepository(db)
	roleRepo := postgres.NewRoleRepository(db)
	userRoleRepo := postgres.NewUserRoleRepository(db)

	libraryRepo := postgres.NewLibraryRepository(db)
	mediaItemRepo := postgres.NewMediaItemRepository(db)
	mediaStreamRepo := postgres.NewMediaStreamRepository(db)
	personRepo := postgres.NewPersonRepository(db)
	mediaPersonRepo := postgres.NewMediaPersonRepository(db)
	trickplayRepo := postgres.NewTrickplayRepository(db)

	watchProgressRepo := postgres.NewWatchProgressRepository(db)
	playSessionRepo := postgres.NewPlaySessionRepository(db)

	transcodeJobRepo := postgres.NewTranscodeJobRepository(db)
	transcodeProfileRepo := postgres.NewTranscodeProfileRepository(db)

	pluginRepo := postgres.NewPluginRepository(db)
	pluginRuntimeRepo := postgres.NewPluginRuntimeRepository(db)

	metadataOverrideRepo := postgres.NewMetadataOverrideRepository(db)
	artworkRepo := postgres.NewArtworkRepository(db)

	loginHistoryRepo := postgres.NewLoginHistoryRepository(db)
	serverMetricRepo := postgres.NewServerMetricRepository(db)
	auditLogRepo := postgres.NewAuditLogRepository(db)
	playSessionStatsRepo := postgres.NewPlaySessionStatsReader(db)
	playHistoryRepo := postgres.NewPlayHistoryRepository(db)

	notificationRepo := postgres.NewNotificationRepository(db)
	notificationSettingsRepo := postgres.NewNotificationSettingsRepository(db)
	watchHistoryAnalyzer := postgres.NewWatchHistoryAnalyzer(db)
	webhookRepo := postgres.NewWebhookRepository(db)
	webhookDeliveryRepo := postgres.NewWebhookDeliveryRepository(db)
	feedQuerier := postgres.NewFeedQuerier(db)
	feedCmdRepo := postgres.NewFeedConfigCommandRepo(db)
	feedQueryRepo := postgres.NewFeedConfigQueryRepo(db)
	chapterRepo := postgres.NewChapterRepository(db)
	scanScheduleRepo := postgres.NewScanScheduleRepository(db)
	libraryAccessRepo := postgres.NewLibraryAccessRepository(db)

	// --- Infrastructure ---
	searchAdapter, err := meili.NewSearchAdapter(cfg.Meilisearch.URL, cfg.Meilisearch.APIKey)
	if err != nil {
		slog.Warn("meilisearch unavailable, search disabled", slog.Any("err", err))
	}
	scanner := filesystem.NewScanner()
	prober := ffmpeg.NewProber("ffprobe")
	transcoder := ffmpeg.NewTranscoder("ffmpeg", cfg.Transcoding.CacheDir)
	hwDetector := ffmpeg.NewHWDetector("ffmpeg")
	trickplayGen := ffmpeg.NewTrickplayGenerator("ffmpeg", "ffprobe")
	ebmlReader := ebml.NewReader()
	ebmlWriter := ebml.NewWriter("ffmpeg")
	bus := eventbus.New()
	hasher := auth.BcryptHasher{}
	// DB value takes precedence over env var for metadata language
	if langEntry, err2 := serverCfgRepo.Get(context.Background(), "metadata.language"); err2 == nil && langEntry.Value != "" {
		cfg.Metadata.Language = langEntry.Value
	}
	tmdbProvider := metadataadapter.NewTMDBProvider(os.Getenv("EVEY_TMDB_API_KEY"), cfg.Metadata.Language, nil)
	imdbProvider := metadataadapter.NewIMDBAPIProvider(nil)
	tvmazeProvider := metadataadapter.NewTVMazeProvider(nil)
	omdbProvider := metadataadapter.NewOMDbProvider(os.Getenv("EVEY_OMDB_API_KEY"), nil)
	artworkDownloader := metadataadapter.NewArtworkHTTPDownloader(nil)
	pluginRunner := pluginrunner.NewRunner()
	manifestParser := pluginrunner.NewManifestParser()

	// --- JWT ---
	privateKey, err := loadOrGenerateRSAKey(cfg.JWT.PrivateKeyPath)
	if err != nil {
		return fmt.Errorf("jwt key: %w", err)
	}
	tokenGen := auth.NewJWTGenerator(privateKey, cfg.JWT.AccessTokenTTL)

	// --- Query adapters ---
	mediaItemQueryRepo := postgres.NewMediaItemQueryRepoAdapter(db)

	// --- Application services ---
	subScanner := filesystem.NewSubtitleScanner()
	libCmdSvc := applibrary.NewCommandService(
		libraryRepo, libraryRepo, mediaItemRepo,
		mediaStreamRepo, personRepo, mediaPersonRepo,
		scanner, prober, searchAdapter, bus,
	)
	libCmdSvc.WithChapters(chapterRepo, ebmlReader)
	libCmdSvc.WithSubtitleScanner(subScanner)
	libCmdSvc.WithTrickplay(trickplayRepo, trickplayGen, cfg.Transcoding.TrickplayCacheDir)
	libQuerySvc := applibrary.NewQueryService(
		libraryRepo, mediaItemQueryRepo,
		mediaStreamRepo, searchAdapter,
	)

	schedulerLog := eveylogger.NewWith(rootLog, "scheduler")
	libScheduler := applibrary.NewScheduler(scanScheduleRepo, libCmdSvc, schedulerLog)

	userCmdSvc := appuser.NewCommandService(
		userRepo,
		profileRepo, refreshTokenRepo, apiKeyRepo,
		inviteLinkRepo, quickConnectRepo, roleRepo, userRoleRepo,
		hasher, tokenGen, bus,
	)
	userCmdSvc.InitSetupState(context.Background())
	userQuerySvc := appuser.NewQueryService(
		userRepo, profileRepo, apiKeyRepo, inviteLinkRepo, quickConnectRepo,
	)

	playbackSvc := appplayback.NewService(
		watchProgressRepo, watchProgressRepo, playSessionRepo, userRepo, profileRepo, bus,
	)

	transcodingSvc := apptranscoding.NewService(
		transcoder, hwDetector,
		transcodeJobRepo, transcodeProfileRepo,
		bus, cfg.Transcoding.MaxConcurrent,
	)
	cacheStore := filesystem.NewCacheStore(cfg.Transcoding.CacheDir)
	transcodingSvc.WithCacheStore(cacheStore)

	mkvSvc := appmkv.NewService(ebmlReader, ebmlWriter)

	collectionRepo := postgres.NewCollectionRepository(db)
	collectionSvc := appcollection.NewService(collectionRepo, collectionRepo)

	metaProviders := []metadata.MetadataProvider{tmdbProvider, omdbProvider, tvmazeProvider, imdbProvider}
	metadataCmdSvc := appmetadata.NewCommandService(
		metadataOverrideRepo, artworkRepo, artworkDownloader, bus, metaProviders,
	)
	metadataQuerySvc := appmetadata.NewQueryService(metadataOverrideRepo, artworkRepo, metaProviders)
	enricher := metadataadapter.NewEnricher(
		artworkRepo, personRepo, mediaPersonRepo, mediaItemRepo, searchAdapter,
		tmdbProvider,
		[]metadata.MetadataProvider{omdbProvider, tvmazeProvider, imdbProvider},
		collectionSvc,
	)
	metadataCmdSvc.WithEnricher(enricher, mediaItemRepo)

	activityCmdSvc := appactivity.NewCommandService(loginHistoryRepo, serverMetricRepo, auditLogRepo, bus)
	activityQuerySvc := appactivity.NewQueryService(loginHistoryRepo, serverMetricRepo, auditLogRepo, playSessionStatsRepo, playHistoryRepo)

	pluginSvc := appplugin.NewService(
		pluginRepo, pluginRuntimeRepo, pluginRunner, manifestParser,
		bus,
	)

	// --- Filesystem watcher ---
	if cfg.Library.WatcherEnabled {
		fsWatcher, fsWatcherErr := filesystem.NewFSWatcher()
		if fsWatcherErr != nil {
			rootLog.Warn("filesystem watcher init failed, realtime monitoring disabled", slog.Any("err", fsWatcherErr))
		} else {
			watcherLog := eveylogger.NewWith(rootLog, "watcher")
			watcherSvc := applibrary.NewWatcherService(fsWatcher, libraryRepo, libCmdSvc, watcherLog)
			if err := watcherSvc.Start(context.Background()); err != nil {
				rootLog.Warn("watcher start failed", slog.Any("err", err))
			} else {
				rootLog.Info("filesystem watcher started")
				bus.Subscribe("library.created", func(_ context.Context, ev shared.Event) {
					if e, ok := ev.(domainlibrary.LibraryCreated); ok {
						lib, err := libraryRepo.GetByID(context.Background(), e.LibraryID)
						if err == nil {
							watcherSvc.AddLibrary(lib)
						}
					}
				})
				bus.Subscribe("library.deleted", func(_ context.Context, ev shared.Event) {
					if e, ok := ev.(domainlibrary.LibraryDeleted); ok {
						watcherSvc.RemoveLibraryByID(e.LibraryID)
					}
				})
				defer watcherSvc.Stop()
			}
		}
	}

	// --- Graceful shutdown context (created early so goroutines can respect it) ---
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	libCmdSvc.WithShutdownCtx(ctx)

	// --- Metadata enrichment ---
	if os.Getenv("EVEY_TMDB_API_KEY") != "" {
		go func() {
			items, err := mediaItemRepo.List(ctx, domainlibrary.ListMediaItemsFilter{Limit: 5000})
			if err == nil {
				enricher.AutoEnrichAll(ctx, items, rootLog)
			}
		}()
		enrichQueue := make(chan domainlibrary.MediaItemAdded, 100)
		for i := 0; i < 4; i++ {
			go func() {
				for e := range enrichQueue {
					item, err := mediaItemRepo.GetByID(ctx, e.MediaItemID)
					if err == nil {
						enricher.AutoEnrichItem(ctx, item, rootLog)
					}
				}
			}()
		}
		bus.Subscribe("media.item.added", func(_ context.Context, ev shared.Event) {
			if e, ok := ev.(domainlibrary.MediaItemAdded); ok {
				select {
				case enrichQueue <- e:
				default:
					slog.Warn("enrich queue full, dropping event", slog.String("media_id", e.MediaItemID))
				}
			}
		})
	}

	bus.Subscribe("media.item.added", func(_ context.Context, ev shared.Event) {
		if e, ok := ev.(domainlibrary.MediaItemAdded); ok {
			go func() {
				if _, err := libCmdSvc.GetTrickplayManifest(ctx, e.MediaItemID); err == nil {
					return // already generated
				}
				if err := libCmdSvc.GenerateTrickplay(ctx, e.MediaItemID); err != nil {
					slog.Warn("trickplay generation failed", slog.String("media_id", e.MediaItemID), slog.Any("err", err))
				}
			}()
		}
	})

	webhookLog := eveylogger.NewWith(rootLog, "webhook")
	webhookSvc := appwebhook.NewService(webhookRepo, webhookDeliveryRepo, webhookLog, bus)

	notifCmdSvc := appnotification.NewCommandService(
		notificationRepo, notificationSettingsRepo, watchHistoryAnalyzer,
		profileRepo, bus,
	)
	notifQuerySvc := appnotification.NewQueryService(notificationRepo, notificationSettingsRepo)
	// notification cleanup goroutine started after ctx is available (see below)

	feedCmdSvc := appfeed.NewCommandService(feedCmdRepo)
	feedQuerySvc := appfeed.NewQueryService(feedQueryRepo, feedQuerier, watchHistoryAnalyzer)

	favoriteRepo := postgres.NewFavoriteRepository(db)
	favoriteSvc := appfavorite.NewService(favoriteRepo)

	userRatingRepo := postgres.NewUserRatingRepository(db)

	playlistRepo := postgres.NewPlaylistRepository(db)
	playlistItemRepo := postgres.NewPlaylistItemRepository(db)
	smartQuerier := postgres.NewSmartPlaylistQuerier(db)
	playlistSvc := appplaylist.NewService(playlistRepo, playlistItemRepo, smartQuerier)

	// --- Subtitle service ---
	subtitleRepo := postgres.NewSubtitleRepository(db)
	var subtitleSvc *appsubtitle.Service
	if osAPIKey := os.Getenv("EVEY_OPENSUBTITLES_API_KEY"); osAPIKey != "" {
		osUser := os.Getenv("EVEY_OPENSUBTITLES_USERNAME")
		osPass := os.Getenv("EVEY_OPENSUBTITLES_PASSWORD")
		osClient := opensubtitles.New(osAPIKey, osUser, osPass)
		mediaGetter := &libMediaGetter{q: libQuerySvc}
		subtitleSvc = appsubtitle.NewService(subtitleRepo, osClient, mediaGetter)
		rootLog.Info("opensubtitles integration enabled")
	}

	// --- Backup service ---
	backupStore := filesystem.NewBackupStore(cfg.Backup.Dir)
	dbDumper := postgres.NewDatabaseDumper(cfg.DB.Host, cfg.DB.Port, cfg.DB.User, cfg.DB.Password, cfg.DB.DBName, cfg.DB.SSLMode)
	backupSvc := appbackup.NewService(backupStore, dbDumper, version)
	if cfg.Backup.EncryptionKey != "" {
		bkey, bkeyErr := backupcrypto.KeyFromBase64(cfg.Backup.EncryptionKey)
		if bkeyErr != nil {
			return fmt.Errorf("backup encryption key: %w", bkeyErr)
		}
		backupSvc.WithEncryptionKey(bkey)
		rootLog.Info("backup encryption enabled")
	}

	// --- Config store (remote access, playback settings) ---
	cfgStore := &serverConfigStore{repo: serverCfgRepo}
	// load RemoteAccess persisted settings from DB (overrides env-based config)
	if raw, err2 := cfgStore.Get(context.Background(), remoteAccessConfigKey); err2 == nil && raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg.RemoteAccess)
	}

	// --- Auth interceptor ---
	authInterceptor := interceptors.NewAuthInterceptor(tokenGen).WithAPIKeys(apiKeyRepo, userRepo)
	grpcLog := eveylogger.NewWith(rootLog, "grpc")

	// --- gRPC server ---
	grpcSrv := grpcapi.NewGRPCServer(
		libCmdSvc, libQuerySvc, userCmdSvc, userQuerySvc,
		playbackSvc.CommandService(), playbackSvc.QueryService(),
		transcodingSvc,
		mkvSvc, pluginSvc, metadataCmdSvc, metadataQuerySvc, activityCmdSvc, activityQuerySvc,
		notifCmdSvc, notifQuerySvc, feedCmdSvc, feedQuerySvc, webhookSvc,
		collectionSvc, playlistSvc, favoriteSvc,
		userRepo, profileRepo, libraryAccessRepo,
		backupSvc, cfg, cfgStore,
		version,
		grpc.ChainUnaryInterceptor(
			interceptors.ErrorSanitizerUnary(),
			interceptors.ClientInfoUnary(),
			interceptors.LoggingUnary(grpcLog),
			authInterceptor.Unary(),
		),
		grpc.ChainStreamInterceptor(
			interceptors.ErrorSanitizerStream(),
			interceptors.ClientInfoStream(),
			interceptors.LoggingStream(grpcLog),
			authInterceptor.Stream(),
		),
	)
	hlsManager := apptranscoding.NewHLSManager(cfg.Transcoding.CacheDir+"/hls", cfg.Transcoding.MaxConcurrent)
	defer hlsManager.CleanupAll()
	grpcSrv.SetHLSManager(hlsManager)
	grpcSrv.SetScheduler(libScheduler)
	grpcSrv.SetLogDir(logCfg.Dir)
	grpcSrv.SetServerConfigHandler(grpcapi.NewServerConfigHandler(serverCfgRepo))
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			eveymetrics.HLSSessionsGauge.Set(float64(hlsManager.ActiveSessionCount()))
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	grpcSrv.RegisterServices()

	grpcAddr := fmt.Sprintf(":%d", cfg.Server.Port)

	netDetector := network.NewDetector(cfg.RemoteAccess.LANNetworks)

	// --- gRPC-Gateway HTTP proxy ---
	httpPort := cfg.Server.Port + 1
	gwMux := registerGateway(grpcAddr)
	var httpSrv *http.Server
	if gwMux != nil {
		mux := http.NewServeMux()
		loginRL := ratelimit.New(cfg.Server.LoginRateLimitMax)
		globalRL := ratelimit.New(100)
		adminHeavyRL := ratelimit.NewWithBurst(3, 6.0)
		defer loginRL.Stop()
		defer globalRL.Stop()
		defer adminHeavyRL.Stop()

		mux.Handle("GET /metrics", eveymetrics.Handler())
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok","version":"` + version + `"}`))
		})
		mux.HandleFunc("/api/v1/admin/fs/browse", requireAdmin(tokenGen, browseDirsHandler(tokenGen)))
		mh := mediaHandler(tokenGen, libQuerySvc, artworkRepo, cfg.Server.BaseURL, libraryAccessRepo)
		mux.HandleFunc("GET /api/v1/media/{id}/stream", mh)
		mux.HandleFunc("GET /api/v1/media/{id}/artwork/{artType}", mh)
		mux.HandleFunc("POST /api/v1/media/{id}/stream", mh)
		mux.HandleFunc("POST /api/v1/media/{id}/artwork/{artworkId}/select", selectArtworkHandler(tokenGen, artworkRepo))
		mux.HandleFunc("GET /api/v1/media/{id}/download", downloadHandler(tokenGen, libQuerySvc, cfg.Server.BaseURL, libraryAccessRepo))
		transcodeThrottle := apptranscoding.NewTranscodeThrottle()
		userMaxStreams := &userMaxStreamsAdapter{repo: userRepo}
		mux.HandleFunc("GET /api/v1/media/{id}/transcode", transcodeHandler(tokenGen, libQuerySvc, cfg.Server.BaseURL, libraryAccessRepo, transcodeThrottle, userMaxStreams))
		mux.HandleFunc("GET /api/v1/media/{id}/subtitle/{track}", subtitleHandler(tokenGen, libQuerySvc, cfg.Server.BaseURL, libraryAccessRepo))
		mux.HandleFunc("GET /api/v1/media/{id}/chapters", chaptersHandler(tokenGen, libCmdSvc, cfg.Server.BaseURL, libraryAccessRepo, libQuerySvc))
		mux.HandleFunc("GET /api/v1/media/{id}/trickplay", trickplayManifestHandler(tokenGen, libCmdSvc, cfg.Server.BaseURL, libraryAccessRepo, libQuerySvc))
		mux.HandleFunc("GET /api/v1/media/{id}/trickplay/{image}", trickplayImageHandler(tokenGen, libCmdSvc, cfg.Server.BaseURL, libraryAccessRepo, libQuerySvc))
		mux.HandleFunc("POST /api/v1/admin/media/{id}/trickplay/generate", requireAdmin(tokenGen, ratelimit.MiddlewareFunc(adminHeavyRL, trickplayGenerateHandler(tokenGen, libCmdSvc))))
		mux.HandleFunc("POST /api/v1/admin/trickplay/generate-all", requireAdmin(tokenGen, ratelimit.MiddlewareFunc(adminHeavyRL, trickplayGenerateAllHandler(tokenGen, libCmdSvc))))
		mux.HandleFunc("GET /api/v1/media/{id}/playback-info", playbackInfoHandler(tokenGen, libQuerySvc, libraryAccessRepo))
		mux.HandleFunc("GET /api/v1/media/{id}/hls/master.m3u8", hlsMasterHandler(tokenGen, libQuerySvc, hlsManager, libraryAccessRepo, userMaxStreams))
		mux.HandleFunc("GET /api/v1/media/{id}/hls/{file}", hlsSegmentHandler(tokenGen, libQuerySvc, hlsManager, libraryAccessRepo))
		mux.HandleFunc("GET /api/v1/admin/library-stats", requireAdmin(tokenGen, libraryStatsHandler(tokenGen, libQuerySvc)))
		httpsOnly := cfg.Server.ForwardedProto
		mux.HandleFunc("POST /api/v1/auth/login", ratelimit.MiddlewareFunc(loginRL, func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			authCookieMiddleware(gwMux, httpsOnly).ServeHTTP(w, r)
		}))
		mux.HandleFunc("POST /api/v1/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
			r = injectRefreshTokenFromCookie(r)
			authCookieMiddleware(gwMux, httpsOnly).ServeHTTP(w, r)
		})
		mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
			clearRefreshCookie(w, httpsOnly)
			gwMux.ServeHTTP(w, r)
		})
		registerAdminRoutes(mux, metadataCmdSvc, metadataQuerySvc, tokenGen, adminHeavyRL)
		registerFavoriteRoutes(mux, favoriteSvc, libQuerySvc, tokenGen)
		registerMetadataOverrideRoutes(mux, metadataCmdSvc, metadataQuerySvc, tokenGen)
		registerTranscodeCacheRoutes(mux, transcodingSvc, tokenGen)
		registerCollectionRoutes(mux, collectionSvc, tokenGen)
		registerScheduleRoutes(mux, libScheduler, tokenGen)
		registerLibraryAccessRoutes(mux, libraryAccessRepo, libQuerySvc, tokenGen)
		registerPlaylistRoutes(mux, playlistSvc, tokenGen)

		registerBackupRoutes(mux, backupSvc, tokenGen, adminHeavyRL)

		// OPS-3: backup retention cleanup on startup
		go func() {
			if err := backupSvc.CleanupOldBackups(context.Background(), cfg.Backup.RetentionDays); err != nil {
				rootLog.Warn("backup retention cleanup", slog.Any("err", err))
			}
		}()

		// OPS-3: periodic backup schedule
		if cfg.Backup.IntervalHours > 0 {
			go func() {
				ticker := time.NewTicker(time.Duration(cfg.Backup.IntervalHours) * time.Hour)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						if _, err := backupSvc.CreateBackup(context.Background()); err != nil {
							rootLog.Warn("scheduled backup failed", slog.Any("err", err))
						} else {
							rootLog.Info("scheduled backup created")
						}
						if err := backupSvc.CleanupOldBackups(context.Background(), cfg.Backup.RetentionDays); err != nil {
							rootLog.Warn("backup retention cleanup", slog.Any("err", err))
						}
					}
				}
			}()
		}

		registerRemoteAccessRoutes(mux, cfg, tokenGen, cfgStore)
		registerPlaybackSettingsRoutes(mux, cfg, tokenGen)
		registerParentalRoutes(mux, profileRepo, hasher, tokenGen)
		registerRatingRoutes(mux, userRatingRepo, tokenGen)
		registerPopularRoute(mux, activityQuerySvc, libQuerySvc, tokenGen)
		registerSystemStatsRoute(mux, startTime, tokenGen)
		registerServerConfigRoutes(mux, serverCfgRepo, tokenGen)
		registerLogsRoute(mux, logCfg.Dir, tokenGen)
		if subtitleSvc != nil {
			registerSubtitleRoutes(mux, subtitleSvc, tokenGen, libQuerySvc, libraryAccessRepo)
		}
		mux.HandleFunc("GET /api/v1/admin/sessions/active", requireAdmin(tokenGen, activeSessionsHandler(tokenGen, playbackSvc)))
		mux.HandleFunc("DELETE /api/v1/admin/sessions/{id}", requireAdmin(tokenGen, killSessionHandler(tokenGen, playbackSvc)))
		mux.Handle("/", ratelimit.Middleware(globalRL,
			"/api/v1/playback/progress",
			"/api/v1/playback/start",
			"/api/v1/playback/stop",
		)(gwMux))
		netMw := networkMiddleware(netDetector, &cfg.RemoteAccess.Enabled)
		proxyTrust, proxyErr := network.NewProxyTrust(cfg.Server.TrustedProxies, cfg.Server.RealIPHeader, cfg.Server.ForwardedProto)
		if proxyErr != nil {
			return fmt.Errorf("proxy trust: %w", proxyErr)
		}
		httpSrv = &http.Server{
			Addr:              fmt.Sprintf(":%d", httpPort),
			Handler:           eveymetrics.InstrumentHandler(tokenRedactMiddleware(securityHeadersMiddleware(proxyTrust.Middleware(netMw(mux))))),
			ReadHeaderTimeout: 10 * time.Second,
			WriteTimeout:      10 * time.Minute,
			IdleTimeout:       120 * time.Second,
		}
	}

	// --- Graceful shutdown ---
	notifCmdSvc.Start(ctx)
	go libScheduler.Run(ctx)

	go func() {
		if err := notifCmdSvc.CleanupExpired(context.Background()); err != nil {
			rootLog.Warn("notification cleanup", slog.Any("err", err))
		}
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := notifCmdSvc.CleanupExpired(context.Background()); err != nil {
					rootLog.Warn("notification cleanup", slog.Any("err", err))
				}
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := refreshTokenRepo.DeleteExpired(context.Background()); err != nil {
					rootLog.Warn("refresh token cleanup", slog.Any("err", err))
				}
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := quickConnectRepo.DeleteExpired(context.Background()); err != nil {
					rootLog.Warn("quick connect cleanup", slog.Any("err", err))
				}
			}
		}
	}()

	// Stale session cleanup: close sessions with no activity for 2h
	go func() {
		// Run immediately on startup to clean existing stale sessions
		if n, err := playbackSvc.CommandService().CleanupStaleSessions(context.Background(), 2*time.Hour); err != nil {
			rootLog.Warn("stale session cleanup", slog.Any("err", err))
		} else if n > 0 {
			rootLog.Info("cleaned stale sessions", slog.Int("count", n))
		}
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n, err := playbackSvc.CommandService().CleanupStaleSessions(context.Background(), 2*time.Hour); err != nil {
					rootLog.Warn("stale session cleanup", slog.Any("err", err))
				} else if n > 0 {
					rootLog.Info("cleaned stale sessions", slog.Int("count", n))
				}
			}
		}
	}()

	if cfg.Backup.IntervalHours > 0 {
		go func() {
			ticker := time.NewTicker(time.Duration(cfg.Backup.IntervalHours) * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if _, err := backupSvc.CreateBackup(context.Background()); err != nil {
						rootLog.Error("scheduled backup failed", slog.Any("err", err))
					} else {
						rootLog.Info("scheduled backup created")
					}
					if cfg.Backup.RetentionDays > 0 {
						if err := backupSvc.CleanupOldBackups(context.Background(), cfg.Backup.RetentionDays); err != nil {
							rootLog.Warn("backup cleanup failed", slog.Any("err", err))
						}
					}
				}
			}
		}()
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

	grpcSrv.Stop()
	if httpSrv != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			slog.Warn("http shutdown", slog.Any("err", err))
		}
	}
	rootLog.Info("shutdown complete")
	return nil
}
