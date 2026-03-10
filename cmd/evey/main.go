package main

import (
	"context"
	"database/sql"
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
	appwebhook "github.com/gravity-zero/evey/internal/app/webhook"
	"github.com/gravity-zero/evey/internal/adapters/driven/ebml"
	"github.com/gravity-zero/evey/internal/adapters/driven/ffmpeg"
	"github.com/gravity-zero/evey/internal/adapters/driven/filesystem"
	meili "github.com/gravity-zero/evey/internal/adapters/driven/meilisearch"
	metadataadapter "github.com/gravity-zero/evey/internal/adapters/driven/metadata"
	"github.com/gravity-zero/evey/internal/adapters/driven/pluginrunner"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/gravity-zero/evey/internal/config"
	"github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/eventbus"
	eveylogger "github.com/gravity-zero/evey/internal/pkg/logger"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	libraryevent "github.com/gravity-zero/evey/internal/domain/library"
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
	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}
	rootLog.Info("connected to postgres")

	if err := postgres.RunMigrations(db); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	// --- Repositories ---
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

	// --- Infrastructure ---
	searchAdapter, err := meili.NewSearchAdapter(cfg.Meilisearch.URL, cfg.Meilisearch.APIKey)
	if err != nil {
		slog.Warn("meilisearch unavailable, search disabled", slog.Any("err", err))
	}
	scanner := filesystem.NewScanner()
	prober := ffmpeg.NewProber("ffprobe")
	transcoder := ffmpeg.NewTranscoder("ffmpeg", cfg.Transcoding.CacheDir)
	hwDetector := ffmpeg.NewHWDetector("ffmpeg")
	ebmlReader := ebml.NewReader()
	ebmlWriter := ebml.NewWriter("ffmpeg")
	bus := eventbus.New()
	hasher := auth.BcryptHasher{}
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
	libQuerySvc := applibrary.NewQueryService(
		libraryRepo, mediaItemQueryRepo,
		mediaStreamRepo, searchAdapter,
	)

	userSvc := appuser.NewService(
		userRepo,
		profileRepo, refreshTokenRepo, apiKeyRepo,
		inviteLinkRepo, quickConnectRepo, roleRepo, userRoleRepo,
		hasher, tokenGen, bus,
	)
	userSvc.InitSetupState(context.Background())

	playbackSvc := appplayback.NewService(
		watchProgressRepo, playSessionRepo, userRepo, profileRepo, bus,
	)

	transcodingSvc := apptranscoding.NewService(
		transcoder, hwDetector,
		transcodeJobRepo, transcodeProfileRepo,
		bus, cfg.Transcoding.MaxConcurrent,
	)
	cacheStore := filesystem.NewCacheStore(cfg.Transcoding.CacheDir)
	transcodingSvc.WithCacheStore(cacheStore)

	mkvSvc := appmkv.NewService(ebmlReader, ebmlWriter)

	metadataSvc := appmetadata.NewService(
		metadataOverrideRepo, artworkRepo, artworkDownloader,
		bus, []metadata.MetadataProvider{tmdbProvider, omdbProvider, tvmazeProvider, imdbProvider},
	)
	enricher := metadataadapter.NewEnricher(
		artworkRepo, personRepo, mediaPersonRepo, mediaItemRepo, searchAdapter,
		tmdbProvider,
		[]metadata.MetadataProvider{omdbProvider, tvmazeProvider, imdbProvider},
	)
	metadataSvc.WithEnricher(enricher, mediaItemRepo)

	activitySvc := appactivity.NewService(
		loginHistoryRepo, serverMetricRepo, auditLogRepo, playSessionStatsRepo, playHistoryRepo,
		bus,
	)

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
					if e, ok := ev.(libraryevent.LibraryCreated); ok {
						lib, err := libraryRepo.GetByID(context.Background(), e.LibraryID)
						if err == nil {
							watcherSvc.AddLibrary(lib)
						}
					}
				})
				bus.Subscribe("library.deleted", func(_ context.Context, ev shared.Event) {
					if e, ok := ev.(libraryevent.LibraryDeleted); ok {
						watcherSvc.RemoveLibraryByID(e.LibraryID)
					}
				})
				defer watcherSvc.Stop()
			}
		}
	}

	// --- Metadata enrichment ---
	if os.Getenv("EVEY_TMDB_API_KEY") != "" {
		go func() {
			ctx := context.Background()
			items, err := mediaItemRepo.List(ctx, domainlibrary.ListMediaItemsFilter{Limit: 5000})
			if err == nil {
				enricher.AutoEnrichAll(ctx, items, rootLog)
			}
		}()
		bus.Subscribe("media.item.added", func(ctx context.Context, ev shared.Event) {
			if e, ok := ev.(libraryevent.MediaItemAdded); ok {
				go func() {
					item, err := mediaItemRepo.GetByID(context.Background(), e.MediaItemID)
					if err == nil {
						enricher.AutoEnrichItem(context.Background(), item, rootLog)
					}
				}()
			}
		})
	}

	webhookLog := eveylogger.NewWith(rootLog, "webhook")
	webhookSvc := appwebhook.NewService(webhookRepo, webhookDeliveryRepo, webhookLog, bus)

	notifSvc := appnotification.NewService(
		notificationRepo, notificationSettingsRepo, watchHistoryAnalyzer,
		profileRepo, bus,
	)
	go func() {
		if err := notifSvc.CleanupExpired(context.Background()); err != nil {
			rootLog.Warn("notification cleanup", slog.Any("err", err))
		}
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := notifSvc.CleanupExpired(context.Background()); err != nil {
				rootLog.Warn("notification cleanup", slog.Any("err", err))
			}
		}
	}()

	feedCmdSvc := appfeed.NewCommandService(feedCmdRepo)
	feedQuerySvc := appfeed.NewQueryService(feedQueryRepo, feedQuerier, watchHistoryAnalyzer)

	favoriteRepo := postgres.NewFavoriteRepository(db)
	favoriteSvc := appfavorite.NewService(favoriteRepo)

	// --- Auth interceptor ---
	authInterceptor := interceptors.NewAuthInterceptor(tokenGen)
	grpcLog := eveylogger.NewWith(rootLog, "grpc")

	// --- gRPC server ---
	grpcSrv := grpcapi.NewGRPCServer(
		libCmdSvc, libQuerySvc, userSvc,
		playbackSvc.CommandService(), playbackSvc.QueryService(),
		transcodingSvc,
		mkvSvc, pluginSvc, metadataSvc, activitySvc,
		notifSvc, feedCmdSvc, feedQuerySvc, webhookSvc,
		userRepo, version,
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
	grpcSrv.RegisterServices()

	grpcAddr := fmt.Sprintf(":%d", cfg.Server.Port)

	// --- gRPC-Gateway HTTP proxy ---
	httpPort := cfg.Server.Port + 1
	gwMux := registerGateway(grpcAddr)
	var httpSrv *http.Server
	if gwMux != nil {
		mux := http.NewServeMux()
		loginRL := ratelimit.New(10)
		globalRL := ratelimit.New(100)

		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok","version":"` + version + `"}`))
		})
		mux.HandleFunc("/api/v1/admin/fs/browse", browseDirsHandler(tokenGen))
		mh := mediaHandler(tokenGen, libQuerySvc, artworkRepo, cfg.Server.BaseURL)
		mux.HandleFunc("GET /api/v1/media/{id}/stream", mh)
		mux.HandleFunc("GET /api/v1/media/{id}/artwork/{artType}", mh)
		mux.HandleFunc("POST /api/v1/media/{id}/stream", mh)
		mux.HandleFunc("POST /api/v1/media/{id}/artwork/{artworkId}/select", selectArtworkHandler(tokenGen, artworkRepo))
		mux.HandleFunc("GET /api/v1/media/{id}/download", downloadHandler(tokenGen, libQuerySvc, cfg.Server.BaseURL))
		mux.HandleFunc("GET /api/v1/media/{id}/transcode", transcodeHandler(tokenGen, libQuerySvc, cfg.Server.BaseURL))
		mux.HandleFunc("GET /api/v1/media/{id}/subtitle/{track}", subtitleHandler(tokenGen, libQuerySvc, cfg.Server.BaseURL))
		mux.HandleFunc("GET /api/v1/media/{id}/chapters", chaptersHandler(tokenGen, libCmdSvc))
		mux.HandleFunc("GET /api/v1/admin/library-stats", libraryStatsHandler(tokenGen, libQuerySvc))
		mux.HandleFunc("POST /api/v1/auth/login", ratelimit.MiddlewareFunc(loginRL, func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			gwMux.ServeHTTP(w, r)
		}))
		registerAdminRoutes(mux, metadataSvc, tokenGen)
		registerFavoriteRoutes(mux, favoriteSvc, tokenGen)
		registerMetadataOverrideRoutes(mux, metadataSvc, tokenGen)
		registerTranscodeCacheRoutes(mux, transcodingSvc, tokenGen)
		mux.Handle("/", ratelimit.Middleware(globalRL)(gwMux))
		httpSrv = &http.Server{
			Addr:              fmt.Sprintf(":%d", httpPort),
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
			WriteTimeout:      10 * time.Minute,
			IdleTimeout:       120 * time.Second,
		}
	}

	// --- Graceful shutdown ---
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
		httpSrv.Shutdown(shutdownCtx)
	}
	rootLog.Info("shutdown complete")
	return nil
}
