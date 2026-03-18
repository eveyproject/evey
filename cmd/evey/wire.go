package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"

	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	appbackup "github.com/gravity-zero/evey/internal/app/backup"
	appcollection "github.com/gravity-zero/evey/internal/app/collection"
	appfavorite "github.com/gravity-zero/evey/internal/app/favorite"
	appfeed "github.com/gravity-zero/evey/internal/app/feed"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	appmkv "github.com/gravity-zero/evey/internal/app/mkv"
	appmusic "github.com/gravity-zero/evey/internal/app/music"
	appnotification "github.com/gravity-zero/evey/internal/app/notification"
	appphoto "github.com/gravity-zero/evey/internal/app/photo"
	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	appplaylist "github.com/gravity-zero/evey/internal/app/playlist"
	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	apprating "github.com/gravity-zero/evey/internal/app/rating"
	appsubtitle "github.com/gravity-zero/evey/internal/app/subtitle"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	appwebhook "github.com/gravity-zero/evey/internal/app/webhook"
	"github.com/gravity-zero/evey/internal/adapters/driven/ebml"
	"github.com/gravity-zero/evey/internal/adapters/driven/email"
	fcmadapter "github.com/gravity-zero/evey/internal/adapters/driven/fcm"
	"github.com/gravity-zero/evey/internal/adapters/driven/ffmpeg"
	"github.com/gravity-zero/evey/internal/adapters/driven/filesystem"
	meili "github.com/gravity-zero/evey/internal/adapters/driven/meilisearch"
	metadataadapter "github.com/gravity-zero/evey/internal/adapters/driven/metadata"
	"github.com/gravity-zero/evey/internal/adapters/driven/opensubtitles"
	"github.com/gravity-zero/evey/internal/adapters/driven/pluginrunner"
	"github.com/gravity-zero/evey/internal/adapters/driving/pluginhost"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	httpapi "github.com/gravity-zero/evey/internal/adapters/driving/http"
	"github.com/gravity-zero/evey/internal/config"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/gravity-zero/evey/internal/pkg/auth"
"github.com/gravity-zero/evey/internal/pkg/eventbus"
	eveylogger "github.com/gravity-zero/evey/internal/pkg/logger"
	appsyncplay "github.com/gravity-zero/evey/internal/app/syncplay"
)

// libraryDeps groups library scanning, querying and scheduling services.
type libraryDeps struct {
	cmdSvc    *applibrary.CommandService
	querySvc  *applibrary.QueryService
	scheduler *applibrary.Scheduler
}

// userDeps groups user/auth services.
type userDeps struct {
	cmdSvc   *appuser.CommandService
	querySvc *appuser.QueryService
	oidcSvc  *appuser.OIDCService
}

// mediaDeps groups playback, transcoding, and media-format services.
type mediaDeps struct {
	playbackSvc    *appplayback.Service
	transcodingSvc *apptranscoding.Service
	hlsManager     *apptranscoding.HLSManager
	mkvSvc         *appmkv.Service
	musicSvc       *appmusic.Service
	photoSvc       *appphoto.Service
	subtitleSvc    *appsubtitle.Service
	syncPlayHub    *appsyncplay.Hub
}

// contentDeps groups content-organisation services (metadata, collections, ratings).
type contentDeps struct {
	metadataCmdSvc   *appmetadata.CommandService
	metadataQuerySvc *appmetadata.QueryService
	collectionSvc    *appcollection.Service
	favoriteSvc      *appfavorite.Service
	starRatingSvc    *apprating.Service
	playlistSvc      *appplaylist.Service
}

// engagementDeps groups activity, notifications, feeds, webhooks and plugins.
type engagementDeps struct {
	activityCmdSvc   *appactivity.CommandService
	activityQuerySvc *appactivity.QueryService
	notifCmdSvc      *appnotification.CommandService
	notifQuerySvc    *appnotification.QueryService
	feedCmdSvc       *appfeed.CommandService
	feedQuerySvc     *appfeed.QueryService
	webhookSvc       *appwebhook.Service
	pluginSvc        *appplugin.Service
	pluginHostServer *pluginhost.HostServer
}

// authDeps groups authentication infrastructure.
type authDeps struct {
	tokenGen *auth.JWTGenerator
	hasher   auth.BcryptHasher
}

// repoDeps groups repository adapters used directly by HTTP/gRPC layers.
type repoDeps struct {
	artworkRepo       *postgres.ArtworkRepository
	userRatingRepo    *postgres.UserRatingRepository
	fcmTokenRepo      *postgres.FCMDeviceTokenRepository
	libraryAccessRepo *postgres.LibraryAccessRepository
	profileRepo       *postgres.ProfileRepository
	serverCfgRepo     *postgres.ServerConfigRepository
	mediaItemRepo     *postgres.MediaItemRepository
	userRepo          *postgres.UserRepository
	apiKeyRepo        *postgres.APIKeyRepository
	pluginRepo        *postgres.PluginRepository
}

// appDeps holds all application-level services and adapters needed by HTTP/gRPC layers.
type appDeps struct {
	library    libraryDeps
	user       userDeps
	media      mediaDeps
	content    contentDeps
	engagement engagementDeps
	auth       authDeps
	repos      repoDeps
	cfgStore   *httpapi.ServerConfigStore
	backupSvc  *appbackup.Service
}

// Event subscription map (bus.Subscribe calls):
//
//   "media.item.added"      → notification (handleMediaItemAdded)
//                           → trickplay worker (auto-generate on new media)
//                           → metadata enricher (auto-enrich on new media)
//   "library.created"       → filesystem watcher (watch new library paths)
//   "library.deleted"       → filesystem watcher (stop watching deleted library)
//   "notification.created"  → email adapter (send email, only when SMTP configured)
//   "*"                     → webhook dispatcher (delivers matching webhooks)
//
// All other domain events (user.*, playback.*, transcode.*, plugin.*, rating.*,
// activity.*, music.*, photo.*, mkv.*, metadata.*) are published but have no
// bus subscribers — they are consumed by the webhook "*" catch-all only.

// buildServices constructs all repos, infrastructure adapters, and application services.
// It also starts the filesystem watcher if enabled, and subscribes to bus events.
// ctx is used for watcher and OIDC init; the caller provides the signal context.
func buildServices(ctx context.Context, cfg *config.Config, db *sql.DB, rootLog *slog.Logger) (*appDeps, error) {
	// --- Repositories ---
	r := postgres.NewRepos(db)

	// --- Infrastructure ---
	var searchAdapter library.SearchIndex
	if sa, err := meili.NewSearchAdapter(cfg.Meilisearch.URL, cfg.Meilisearch.APIKey); err != nil {
		slog.Warn("meilisearch unavailable, search disabled", slog.Any("err", err))
	} else {
		searchAdapter = sa
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
	if langEntry, err2 := r.ServerCfg.Get(context.Background(), "metadata.language"); err2 == nil && langEntry.Value != "" {
		cfg.Metadata.Language = langEntry.Value
	}
	tmdbProvider := metadataadapter.NewTMDBProvider(os.Getenv("EVEY_TMDB_API_KEY"), cfg.Metadata.Language, nil)
	imdbProvider := metadataadapter.NewIMDBAPIProvider(nil)
	tvmazeProvider := metadataadapter.NewTVMazeProvider(nil)
	omdbProvider := metadataadapter.NewOMDbProvider(os.Getenv("EVEY_OMDB_API_KEY"), nil)
	artworkDownloader := metadataadapter.NewArtworkHTTPDownloader(nil)
	pluginLogger := pluginrunner.NewPluginLogger(cfg.Log.Dir + "/plugins")
	pluginRunner := pluginrunner.NewRunner().WithLogger(pluginLogger)
	manifestParser := pluginrunner.NewManifestParser()
	pluginInstaller := pluginrunner.NewZipInstaller()
	pluginAnalyzer := pluginrunner.NewStaticAnalyzer()

	// --- JWT ---
	privateKey, err := auth.LoadOrGenerateRSAKey(cfg.JWT.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("jwt key: %w", err)
	}
	tokenGen := auth.NewJWTGenerator(privateKey, cfg.JWT.AccessTokenTTL)

	// --- Application services ---
	subScanner := filesystem.NewSubtitleScanner()
	libCmdSvc := applibrary.NewCommandService(
		r.Library, r.Library, r.MediaItem,
		r.MediaStream, r.Person, r.MediaPerson,
		scanner, prober, searchAdapter, bus,
	)
	libCmdSvc.WithChapters(r.Chapter, ebmlReader)
	libCmdSvc.WithSubtitleScanner(subScanner)
	libCmdSvc.WithTrickplay(r.Trickplay, trickplayGen, cfg.Transcoding.TrickplayCacheDir)
	libCmdSvc.Validate()
	libQuerySvc := applibrary.NewQueryService(r.Library, r.MediaItemQuery, r.MediaStream, searchAdapter)

	schedulerLog := eveylogger.NewWith(rootLog, "scheduler")
	libScheduler := applibrary.NewScheduler(r.ScanSchedule, libCmdSvc, schedulerLog)

	userCmdSvc := appuser.NewCommandService(
		r.User, r.Profile, r.RefreshToken, r.APIKey,
		r.InviteLink, r.QuickConnect, r.Role, r.UserRole,
		hasher, tokenGen, bus,
	)
	userCmdSvc.InitSetupState(context.Background())
	userQuerySvc := appuser.NewQueryService(r.User, r.Profile, r.APIKey, r.InviteLink, r.QuickConnect)

	var oidcSvc *appuser.OIDCService
	if cfg.OIDC.Enabled {
		var oidcErr error
		oidcSvc, oidcErr = appuser.NewOIDCService(
			context.Background(), cfg,
			r.User, r.Profile, r.RefreshToken, r.UserRole,
			tokenGen, bus,
		)
		if oidcErr != nil {
			return nil, fmt.Errorf("oidc init: %w", oidcErr)
		}
		rootLog.Info("OIDC SSO enabled", slog.String("issuer", cfg.OIDC.Issuer))
	}

	playbackSvc := appplayback.NewService(r.WatchProgress, r.WatchProgress, r.PlaySession, r.PlaySession, r.User, r.Profile, bus)

	transcodingSvc := apptranscoding.NewService(
		transcoder, hwDetector, r.TranscodeJob, r.TranscodeProfile,
		bus, cfg.Transcoding.MaxConcurrent,
	)
	transcodingSvc.WithCacheStore(filesystem.NewCacheStore(cfg.Transcoding.CacheDir))

	mkvSvc := appmkv.NewService(ebmlReader, ebmlWriter)
	collectionSvc := appcollection.NewService(r.Collection, r.Collection, postgres.NewCollectionSmartQuerier(db), r.MediaItem)

	metaProviders := []metadata.MetadataProvider{tmdbProvider, omdbProvider, tvmazeProvider, imdbProvider}
	metadataCmdSvc := appmetadata.NewCommandService(r.MetadataOverride, r.Artwork, artworkDownloader, bus, metaProviders)
	metadataQuerySvc := appmetadata.NewQueryService(r.MetadataOverride, r.Artwork, metaProviders)
	enricher := metadataadapter.NewEnricher(
		r.Artwork, r.Person, r.MediaPerson, r.MediaItem, searchAdapter,
		tmdbProvider,
		[]metadata.MetadataProvider{omdbProvider, tvmazeProvider, imdbProvider},
		collectionSvc,
	)
	metadataCmdSvc.WithEnricher(enricher, r.MediaItem)

	activityCmdSvc := appactivity.NewCommandService(r.LoginHistory, r.ServerMetric, r.AuditLog, bus)
	activityQuerySvc := appactivity.NewQueryService(r.LoginHistory, r.ServerMetric, r.AuditLog, r.PlaySessionStats, r.PlayHistory).
		WithAnalyticsReader(postgres.NewAnalyticsReader(db))

	permChecker := appplugin.NewPermissionChecker(r.Plugin)

	pluginRunner.WithIPCCallback(func(pluginID string, ipcErr error) {
		p, getErr := r.Plugin.GetByID(context.Background(), pluginID)
		if getErr != nil {
			return
		}
		if ipcErr != nil {
			p.Status = "error"
			p.ErrorMessage = ipcErr.Error()
		} else {
			p.Status = "running"
			p.ErrorMessage = ""
		}
		if upErr := r.Plugin.Update(context.Background(), p); upErr != nil {
			slog.Warn("update plugin status after IPC", slog.String("pluginID", pluginID), slog.Any("err", upErr))
		}
	})

	pluginSvc := appplugin.NewService(r.Plugin, r.PluginRuntime, pluginRunner, manifestParser, bus).
		WithInstaller(pluginInstaller).
		WithAnalyzer(pluginAnalyzer).
		WithPermissionChecker(permChecker)

	permInterceptor := appplugin.PluginPermissionInterceptor(permChecker)
	pluginHostSrv := pluginhost.NewHostServer(r.MediaItemQuery, r.Plugin, bus, pluginLogger, permInterceptor)

	// --- Filesystem watcher ---
	startWatcher(ctx, cfg, bus, r.Library, libCmdSvc, rootLog)

	libCmdSvc.WithShutdownCtx(ctx)

	// --- Background workers ---
	if os.Getenv("EVEY_TMDB_API_KEY") != "" {
		metadataCmdSvc.StartEnrichmentWorker(ctx)
	}
	libCmdSvc.StartTrickplayWorker(ctx)

	webhookLog := eveylogger.NewWith(rootLog, "webhook")
	webhookSvc := appwebhook.NewService(r.Webhook, r.WebhookDelivery, webhookLog, bus)

	notifCmdSvc := appnotification.NewCommandService(r.Notification, r.NotificationSettings, r.WatchHistoryAnalyzer, r.Profile, bus)
	notifQuerySvc := appnotification.NewQueryService(r.Notification, r.NotificationSettings)

	if cfg.FCM.Enabled && cfg.FCM.ServiceAccountJSON != "" {
		fcmSender, fcmErr := fcmadapter.NewSender(ctx, cfg.FCM.ServiceAccountJSON)
		if fcmErr != nil {
			rootLog.Warn("FCM init failed, push notifications disabled", slog.Any("err", fcmErr))
		} else {
			notifCmdSvc.WithPush(fcmSender, r.FCMToken)
			rootLog.Info("FCM push notifications enabled", slog.String("project", cfg.FCM.ProjectID))
		}
	}

	if cfg.SMTP.Host != "" {
		smtpSender := email.NewSMTPSender(cfg.SMTP.Host, cfg.SMTP.Port, cfg.SMTP.Username, cfg.SMTP.Password, cfg.SMTP.From, cfg.SMTP.TLS)
		email.NewNotificationEmailer(smtpSender, r.Profile, r.User).Register(bus)
		rootLog.Info("SMTP email notifications enabled", slog.String("host", cfg.SMTP.Host))
	}

	feedCmdSvc := appfeed.NewCommandService(r.FeedConfigCmd)
	feedQuerySvc := appfeed.NewQueryService(r.FeedConfigQuery, r.FeedQuerier, r.WatchHistoryAnalyzer)

	favoriteSvc := appfavorite.NewService(r.Favorite)
	photoSvc := appphoto.NewService(r.Photo, bus, r.Library)
	starRatingSvc := apprating.NewService(r.Rating, bus)
	musicSvc := appmusic.NewService(r.MusicTrack, bus, r.Library)
	playlistSvc := appplaylist.NewService(r.Playlist, r.PlaylistItem, r.SmartPlaylist)

	// --- Subtitle service ---
	var subtitleSvc *appsubtitle.Service
	if osAPIKey := os.Getenv("EVEY_OPENSUBTITLES_API_KEY"); osAPIKey != "" {
		osClient := opensubtitles.New(osAPIKey, os.Getenv("EVEY_OPENSUBTITLES_USERNAME"), os.Getenv("EVEY_OPENSUBTITLES_PASSWORD"))
		subtitleSvc = appsubtitle.NewService(r.Subtitle, osClient, opensubtitles.NewLibraryMediaGetter(libQuerySvc))
		rootLog.Info("opensubtitles integration enabled")
	}

	// --- Backup service ---
	backupSvc := appbackup.NewService(filesystem.NewBackupStore(cfg.Backup.Dir),
		postgres.NewDatabaseDumper(cfg.DB.Host, cfg.DB.Port, cfg.DB.User, cfg.DB.Password, cfg.DB.DBName, cfg.DB.SSLMode),
		version)

	syncPlayHub := appsyncplay.NewHub(postgres.NewSyncPlayRoomRepo(db))

	return &appDeps{
		library: libraryDeps{
			cmdSvc:    libCmdSvc,
			querySvc:  libQuerySvc,
			scheduler: libScheduler,
		},
		user: userDeps{
			cmdSvc:   userCmdSvc,
			querySvc: userQuerySvc,
			oidcSvc:  oidcSvc,
		},
		media: mediaDeps{
			playbackSvc:    playbackSvc,
			transcodingSvc: transcodingSvc,
			hlsManager:     apptranscoding.NewHLSManager(cfg.Transcoding.CacheDir+"/hls", cfg.Transcoding.MaxConcurrent),
			mkvSvc:         mkvSvc,
			musicSvc:       musicSvc,
			photoSvc:       photoSvc,
			subtitleSvc:    subtitleSvc,
			syncPlayHub:    syncPlayHub,
		},
		content: contentDeps{
			metadataCmdSvc:   metadataCmdSvc,
			metadataQuerySvc: metadataQuerySvc,
			collectionSvc:    collectionSvc,
			favoriteSvc:      favoriteSvc,
			starRatingSvc:    starRatingSvc,
			playlistSvc:      playlistSvc,
		},
		engagement: engagementDeps{
			activityCmdSvc:   activityCmdSvc,
			activityQuerySvc: activityQuerySvc,
			notifCmdSvc:      notifCmdSvc,
			notifQuerySvc:    notifQuerySvc,
			feedCmdSvc:       feedCmdSvc,
			feedQuerySvc:     feedQuerySvc,
			webhookSvc:       webhookSvc,
			pluginSvc:        pluginSvc,
			pluginHostServer: pluginHostSrv,
		},
		auth: authDeps{
			tokenGen: tokenGen,
			hasher:   hasher,
		},
		repos: repoDeps{
			artworkRepo:       r.Artwork,
			userRatingRepo:    r.UserRating,
			fcmTokenRepo:      r.FCMToken,
			libraryAccessRepo: r.LibraryAccess,
			profileRepo:       r.Profile,
			serverCfgRepo:     r.ServerCfg,
			mediaItemRepo:     r.MediaItem,
			userRepo:          r.User,
			apiKeyRepo:        r.APIKey,
			pluginRepo:        r.Plugin,
		},
		cfgStore:  httpapi.NewServerConfigStore(r.ServerCfg),
		backupSvc: backupSvc,
	}, nil
}

func startWatcher(ctx context.Context, cfg *config.Config, bus *eventbus.InMemoryEventBus, libraryRepo *postgres.LibraryRepository, libCmdSvc *applibrary.CommandService, rootLog *slog.Logger) {
	if !cfg.Library.WatcherEnabled {
		return
	}
	fsWatcher, err := filesystem.NewFSWatcher()
	if err != nil {
		rootLog.Warn("filesystem watcher init failed, realtime monitoring disabled", slog.Any("err", err))
		return
	}
	watcherSvc := applibrary.NewWatcherService(fsWatcher, libraryRepo, libCmdSvc, eveylogger.NewWith(rootLog, "watcher")).
		WithEventBus(bus)
	if err := watcherSvc.Start(context.Background()); err != nil {
		rootLog.Warn("watcher start failed", slog.Any("err", err))
		return
	}
	rootLog.Info("filesystem watcher started")
	go func() {
		<-ctx.Done()
		watcherSvc.Stop()
	}()
}
