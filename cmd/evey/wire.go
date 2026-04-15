package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	appactivity "github.com/evey-project/evey/internal/app/activity"
	appbackup "github.com/evey-project/evey/internal/app/backup"
	appcollection "github.com/evey-project/evey/internal/app/collection"
	appconfig "github.com/evey-project/evey/internal/app/config"
	appfavorite "github.com/evey-project/evey/internal/app/favorite"
	appfeed "github.com/evey-project/evey/internal/app/feed"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmedia "github.com/evey-project/evey/internal/app/media"
	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	appmkv "github.com/evey-project/evey/internal/app/mkv"
	appmusic "github.com/evey-project/evey/internal/app/music"
	appnotification "github.com/evey-project/evey/internal/app/notification"
	appphoto "github.com/evey-project/evey/internal/app/photo"
	appplayback "github.com/evey-project/evey/internal/app/playback"
	appplaylist "github.com/evey-project/evey/internal/app/playlist"
	appplugin "github.com/evey-project/evey/internal/app/plugin"
	apprating "github.com/evey-project/evey/internal/app/rating"
	appsubtitle "github.com/evey-project/evey/internal/app/subtitle"
	appsyncplay "github.com/evey-project/evey/internal/app/syncplay"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	appuser "github.com/evey-project/evey/internal/app/user"
	appwebhook "github.com/evey-project/evey/internal/app/webhook"
	mkvgoadapter "github.com/evey-project/evey/internal/adapters/driven/mkvgo"
	"github.com/evey-project/evey/internal/adapters/driven/email"
	webpushadapter "github.com/evey-project/evey/internal/adapters/driven/webpush"
	"github.com/evey-project/evey/internal/adapters/driven/ffmpeg"
	"github.com/evey-project/evey/internal/adapters/driven/filesystem"
	meili "github.com/evey-project/evey/internal/adapters/driven/meilisearch"
	"github.com/evey-project/evey/internal/adapters/driven/search"
	"github.com/evey-project/evey/internal/adapters/driven/webhooksender"
	metadataadapter "github.com/evey-project/evey/internal/adapters/driven/metadata"
	"github.com/evey-project/evey/internal/adapters/driven/opensubtitles"
	"github.com/evey-project/evey/internal/adapters/driven/pluginrunner"
	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	httpapi "github.com/evey-project/evey/internal/adapters/driving/http"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi"
	"github.com/evey-project/evey/internal/adapters/driving/pluginhost"
	"github.com/evey-project/evey/internal/config"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/metadata"
	domainplugin "github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/eventbus"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	eveylogger "github.com/evey-project/evey/internal/pkg/logger"
)

// Event subscription map (bus.Subscribe calls):
//
//   "media.item.added"      → notification (handleMediaItemAdded)
//                           → trickplay worker (auto-generate on new media)
//                           → metadata enricher (auto-enrich on new media)
//   "backup.failed"         → notification (handleBackupFailed)
//   "library.created"       → filesystem watcher (watch new library paths)
//   "library.deleted"       → filesystem watcher (stop watching deleted library)
//   "notification.created"  → email adapter (send email, only when SMTP configured)
//   "*"                     → webhook dispatcher (delivers matching webhooks)
//
// All other domain events (user.*, playback.*, transcode.*, plugin.*, rating.*,
// activity.*, music.*, photo.*, mkv.*, metadata.*) are published but have no
// bus subscribers — they are consumed by the webhook "*" catch-all only.

// infraDeps holds infrastructure adapters and low-level dependencies shared across app services.
type infraDeps struct {
	repos            *postgres.Repos
	searchAdapter    library.SearchIndex
	searchHealth     httpapi.SearchHealthChecker
	scanner          *filesystem.Scanner
	prober           *ffmpeg.Prober
	transcoder       *ffmpeg.Transcoder
	hwDetector       *ffmpeg.HWDetector
	trickplayGen     *ffmpeg.TrickplayGenerator
	imageConverter   *ffmpeg.ImageConverter
	mkvAdapter       *mkvgoadapter.Adapter
	bus              *eventbus.InMemoryEventBus
	hasher           auth.BcryptHasher
	tokenGen         *auth.JWTGenerator
	translator       *eveyi18n.Translator
	metadataHTTP     *http.Client
	tmdbProvider     *metadataadapter.TMDBProvider
	imdbProvider     *metadataadapter.IMDBAPIProvider
	tvmazeProvider   *metadataadapter.TVMazeProvider
	omdbProvider     *metadataadapter.OMDbProvider
	artworkDL        *metadataadapter.ArtworkHTTPDownloader
	personFetcher    *metadataadapter.TMDBPersonFetcher
	pluginRunner     *pluginrunner.Runner
	pluginLogger     *pluginrunner.PluginLogger
	manifestParser   *pluginrunner.YAMLManifestParser
	pluginInstaller  *pluginrunner.ZipInstaller
	pluginAnalyzer   *pluginrunner.StaticAnalyzer
	subScanner       *filesystem.SubtitleScanner
	tmdbKey          string
}

// buildInfra creates repos, infrastructure adapters, and low-level dependencies.
func buildInfra(ctx context.Context, cfg *config.Config, db *sql.DB, rootLog *slog.Logger) (*infraDeps, error) {
	r := postgres.NewRepos(db)

	sqlFallback := postgres.NewSQLSearchFallback(db)
	var searchAdapter library.SearchIndex
	var searchHealth httpapi.SearchHealthChecker
	if sa, err := meili.NewSearchAdapter(cfg.Meilisearch.URL, cfg.Meilisearch.APIKey); err != nil {
		slog.Warn("meilisearch unavailable, using SQL search fallback", slog.Any("err", err))
		searchAdapter = search.NewResilient(nil, sqlFallback)
	} else {
		searchAdapter = search.NewResilient(sa, sqlFallback)
		searchHealth = sa
	}

	scanner := filesystem.NewScanner()
	prober := ffmpeg.NewProber("ffprobe")
	transcoder := ffmpeg.NewTranscoder("ffmpeg", cfg.Transcoding.CacheDir)
	hwDetector := ffmpeg.NewHWDetector("ffmpeg")
	trickplayGen := ffmpeg.NewTrickplayGenerator("ffmpeg", "ffprobe")
	imageConverter := ffmpeg.NewImageConverter("ffmpeg")
	mkvAdapter := mkvgoadapter.New()
	bus := eventbus.New()
	hasher := auth.BcryptHasher{}

	if langEntry, err2 := r.ServerCfg.Get(ctx, "metadata.language"); err2 == nil && langEntry.Value != "" {
		cfg.Metadata.Language = langEntry.Value
	}
	metadataHTTP := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			DialContext:         (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		},
	}
	tmdbKey := resolveAPIKey(ctx, r.ServerCfg, "api.tmdb_key", "EVEY_TMDB_API_KEY")
	serverCfgForLang := r.ServerCfg
	defaultLang := cfg.Metadata.Language
	tmdbProvider := metadataadapter.NewTMDBProviderWithGetter(tmdbKey, func() string {
		if entry, err2 := serverCfgForLang.Get(context.Background(), "metadata.language"); err2 == nil && entry.Value != "" {
			return entry.Value
		}
		return defaultLang
	}, metadataHTTP)
	imdbProvider := metadataadapter.NewIMDBAPIProvider(metadataHTTP)
	tvmazeProvider := metadataadapter.NewTVMazeProvider(metadataHTTP)
	omdbKey := resolveAPIKey(ctx, r.ServerCfg, "api.omdb_key", "EVEY_OMDB_API_KEY")
	omdbProvider := metadataadapter.NewOMDbProvider(omdbKey, metadataHTTP)
	artworkDownloader := metadataadapter.NewArtworkHTTPDownloader(metadataHTTP)
	personFetcher := metadataadapter.NewTMDBPersonFetcher(tmdbProvider)

	pluginLogDir := filepath.Join(cfg.Log.Dir, "plugins")
	pluginRetention := cfg.Log.PluginRetentionDays
	if pluginRetention <= 0 {
		pluginRetention = 30
	}
	pluginLogger := pluginrunner.NewPluginLogger(pluginLogDir, pluginRetention)
	pluginRunner := pluginrunner.NewRunner().WithLogger(pluginLogger)
	manifestParser := pluginrunner.NewManifestParser()
	pluginInstaller := pluginrunner.NewZipInstaller()
	pluginAnalyzer := pluginrunner.NewStaticAnalyzer()

	translator, i18nErr := eveyi18n.NewFromDir("i18n")
	if i18nErr != nil {
		slog.Warn("i18n load failed, using defaults", slog.Any("err", i18nErr))
		translator = eveyi18n.NewEmpty()
	}

	privateKey, err := auth.LoadOrGenerateRSAKey(cfg.JWT.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("jwt key: %w", err)
	}
	tokenGen := auth.NewJWTGenerator(privateKey, cfg.JWT.AccessTokenTTL)

	subScanner := filesystem.NewSubtitleScanner()

	return &infraDeps{
		repos:           r,
		searchAdapter:   searchAdapter,
		searchHealth:    searchHealth,
		scanner:         scanner,
		prober:          prober,
		transcoder:      transcoder,
		hwDetector:      hwDetector,
		trickplayGen:    trickplayGen,
		imageConverter:  imageConverter,
		mkvAdapter:      mkvAdapter,
		bus:             bus,
		hasher:          hasher,
		tokenGen:        tokenGen,
		translator:      translator,
		metadataHTTP:    metadataHTTP,
		tmdbProvider:    tmdbProvider,
		imdbProvider:    imdbProvider,
		tvmazeProvider:  tvmazeProvider,
		omdbProvider:    omdbProvider,
		artworkDL:       artworkDownloader,
		personFetcher:   personFetcher,
		pluginRunner:    pluginRunner,
		pluginLogger:    pluginLogger,
		manifestParser:  manifestParser,
		pluginInstaller: pluginInstaller,
		pluginAnalyzer:  pluginAnalyzer,
		subScanner:      subScanner,
		tmdbKey:         tmdbKey,
	}, nil
}

// buildAppServices creates all application-level services from infra dependencies.
func buildAppServices(ctx context.Context, cfg *config.Config, db *sql.DB, infra *infraDeps, rootLog *slog.Logger) (*appDeps, error) {
	r := infra.repos
	bus := infra.bus

	libCmdSvc := applibrary.NewCommandService(
		r.Library, r.Library, r.MediaItem,
		r.MediaStream, r.Person, r.MediaPerson,
		infra.scanner, infra.prober, infra.searchAdapter, bus,
		r.Chapter, infra.mkvAdapter,
		r.Trickplay, infra.trickplayGen, cfg.Transcoding.TrickplayCacheDir, r.ServerCfg,
		applibrary.WithShutdownCtx(ctx),
	)
	libCmdSvc.WithSubtitleScanner(infra.subScanner)
	mediaItemCache := applibrary.NewCachedMediaItemQueryRepo(r.MediaItemQuery, 30*time.Second)
	mediaItemCache.Register(bus)
	libQuerySvc := applibrary.NewQueryService(r.Library, mediaItemCache, r.MediaStream, infra.searchAdapter,
		applibrary.WithPeople(r.Person),
		applibrary.WithPersonFetcher(infra.personFetcher),
		applibrary.WithTranslations(r.MediaItemTranslation, r.PersonTranslation, infra.personFetcher, eveyi18n.SupportedLocales()),
		applibrary.WithChapters(r.Chapter),
		applibrary.WithTrickplay(r.Trickplay),
	)

	schedulerLog := eveylogger.NewWith(rootLog, "scheduler")
	libScheduler := applibrary.NewScheduler(r.ScanSchedule, libCmdSvc, schedulerLog)

	userAuthSvc := appuser.NewCommandService(
		r.User, r.Profile, r.RefreshToken,
		r.Role, r.UserRole,
		infra.hasher, infra.tokenGen, bus,
		7*24*time.Hour, r.QuickConnect,
	)
	userAuthSvc.InitSetupState(ctx)
	userSvc := appuser.NewUserService(r.User, r.Profile, r.RefreshToken, infra.hasher, bus, postgres.NewTxUnit(db))
	userAccessSvc := appuser.NewAccessService(r.User, r.APIKey, r.InviteLink, r.QuickConnect, infra.hasher, bus, appuser.WithProfiles(r.Profile), appuser.WithLibraryAccess(r.LibraryAccess))
	userQuerySvc := appuser.NewQueryService(r.User, r.Profile, r.APIKey, r.InviteLink, r.QuickConnect)

	var oidcSvc *appuser.OIDCService
	if cfg.OIDC.Enabled {
		var oidcErr error
		oidcSvc, oidcErr = appuser.NewOIDCService(
			ctx, appuser.OIDCConfig{
				Enabled:             cfg.OIDC.Enabled,
				Issuer:              cfg.OIDC.Issuer,
				ClientID:            cfg.OIDC.ClientID,
				ClientSecret:        cfg.OIDC.ClientSecret,
				RedirectURL:         cfg.OIDC.RedirectURL,
				AllowedEmailDomains: cfg.OIDC.AllowedEmailDomains,
			},
			r.User, r.Profile, r.RefreshToken, r.UserRole,
			infra.tokenGen, bus, 7*24*time.Hour,
		)
		if oidcErr != nil {
			return nil, fmt.Errorf("oidc init: %w", oidcErr)
		}
		rootLog.Info("OIDC SSO enabled", slog.String("issuer", cfg.OIDC.Issuer))
	}

	playbackSvc := appplayback.NewService(r.WatchProgress, r.WatchProgress, r.PlaySession, r.PlaySession, r.User, r.Profile, bus)

	transcodingSvc := apptranscoding.NewService(
		infra.transcoder, infra.hwDetector, r.TranscodeJob, r.TranscodeProfile,
		bus, cfg.Transcoding.MaxConcurrent,
		apptranscoding.WithCacheStore(filesystem.NewCacheStore(cfg.Transcoding.CacheDir)),
	)

	mkvSvc := appmkv.NewService(infra.mkvAdapter, infra.mkvAdapter)
	collectionSvc := appcollection.NewService(r.Collection, r.Collection, postgres.NewCollectionSmartQuerier(db), r.MediaItem)

	metaProviders := []metadata.MetadataProvider{infra.tmdbProvider, infra.omdbProvider, infra.tvmazeProvider, infra.imdbProvider}
	metadataQuerySvc := appmetadata.NewQueryService(r.MetadataOverride, r.Artwork, metaProviders)
	enricher := appmetadata.NewEnricher(
		r.Artwork, r.Person, r.MediaPerson, r.MediaItem, infra.searchAdapter,
		infra.tmdbProvider,
		[]metadata.MetadataProvider{infra.omdbProvider, infra.tvmazeProvider, infra.imdbProvider},
		collectionSvc,
		appmetadata.EnricherWithTranslations(r.MediaItemTranslation, r.PersonTranslation, eveyi18n.SupportedLocales()),
		appmetadata.EnricherWithPersonFetcher(infra.personFetcher),
	)
	metadataCmdSvc := appmetadata.NewCommandService(r.MetadataOverride, r.Artwork, infra.artworkDL, bus, metaProviders,
		appmetadata.WithEnricher(enricher, r.MediaItem),
		appmetadata.WithMediaPersons(r.MediaPerson),
	)

	activityCmdSvc := appactivity.NewCommandService(r.LoginHistory, r.ServerMetric, r.AuditLog, bus)
	activityQuerySvc := appactivity.NewQueryService(r.LoginHistory, r.ServerMetric, r.AuditLog, r.PlaySessionStats, r.PlayHistory,
		appactivity.WithAnalyticsReader(postgres.NewAnalyticsReader(db)),
	)

	permChecker := appplugin.NewPermissionChecker(r.Plugin)
	pluginFS := filesystem.NewPluginFileSystem()
	cssSanitizer := pluginrunner.NewCSSSanitizer()
	pluginInstallerSvc := appplugin.NewInstallerService(r.Plugin, infra.pluginRunner, infra.manifestParser, bus,
		appplugin.InstallerWithInstaller(infra.pluginInstaller),
		appplugin.InstallerWithAnalyzer(infra.pluginAnalyzer),
		appplugin.InstallerWithPermissionChecker(permChecker),
		appplugin.InstallerWithFileSystem(pluginFS),
		appplugin.InstallerWithCSSSanitizer(cssSanitizer),
	)
	pluginRuntimeSvc := appplugin.NewRuntimeService(r.Plugin, r.PluginRuntime, infra.pluginRunner, infra.manifestParser, bus,
		appplugin.RuntimeWithPermissionChecker(permChecker),
		appplugin.RuntimeWithShutdownCtx(ctx),
	)
	infra.pluginRunner.WithIPCCallback(pluginRuntimeSvc.HandleIPCResult)
	pluginQuerySvc := appplugin.NewQueryService(r.Plugin, r.PluginRuntime)
	pluginTokenSvc := appplugin.NewTokenService(r.Plugin, infra.tokenGen)

	permInterceptor := grpcapi.PluginPermissionInterceptor(permChecker)
	pluginHostSrv := pluginhost.NewHostServer(r.MediaItemQuery, r.Plugin, bus, infra.pluginLogger, permInterceptor)
	infra.pluginLogger.StartCleanup(ctx, &pluginIDLister{repo: r.Plugin})

	webhookLog := eveylogger.NewWith(rootLog, "webhook")
	webhookSender := webhooksender.New()
	webhookSvc := appwebhook.NewService(r.Webhook, r.WebhookDelivery, webhookSender, webhookLog, bus)

	var notifOpts []appnotification.Option
	vapidPub, vapidPriv := resolveVAPIDKeys(ctx, r.ServerCfg, cfg)
	if vapidPub != "" && vapidPriv != "" {
		wpSender := webpushadapter.NewSender(vapidPub, vapidPriv, cfg.VAPID.Subject)
		notifOpts = append(notifOpts, appnotification.WithPush(wpSender, r.PushSubscription))
		rootLog.Info("Web Push (VAPID) notifications enabled")
	}
	notifCmdSvc := appnotification.NewCommandService(r.Notification, r.NotificationSettings, r.WatchHistoryAnalyzer, r.Profile, bus, notifOpts...)
	notifCmdSvc.Register()
	notifQuerySvc := appnotification.NewQueryService(r.Notification, r.NotificationSettings, appnotification.WithPushSubs(r.PushSubscription))

	smtpConfigured := cfg.SMTP.Host != ""
	if smtpConfigured {
		smtpSender := email.NewSMTPSender(cfg.SMTP.Host, cfg.SMTP.Port, cfg.SMTP.Username, cfg.SMTP.Password, cfg.SMTP.From, cfg.SMTP.TLS)
		email.NewNotificationEmailer(smtpSender, r.Profile, r.User).Register(bus)
		email.NewPasswordResetEmailer(smtpSender, cfg.EffectiveBaseURL).Register(bus)
		rootLog.Info("SMTP email notifications enabled", slog.String("host", cfg.SMTP.Host))
	}
	appuser.WithPasswordReset(r.PasswordResetToken, r.ServerCfg, smtpConfigured)(userAuthSvc)
	appuser.WithUnitOfWork(postgres.NewTxUnit(db))(userAuthSvc)

	feedCmdSvc := appfeed.NewCommandService(r.FeedConfigCmd, r.FeedConfigQuery, appfeed.CommandWithEventBus(bus))
	cachedAnalyzer := appfeed.NewCachedAnalyzer(r.WatchHistoryAnalyzer, 5*time.Minute, appfeed.CacheWithEventBus(bus))
	feedQuerySvc := appfeed.NewQueryService(r.FeedConfigQuery, r.FeedQuerier, cachedAnalyzer)

	if libs, libErr := r.Library.List(ctx); libErr == nil {
		for _, lib := range libs {
			if err := feedCmdSvc.EnsureLibraryFeedConfig(ctx, lib.ID, lib.Name); err != nil {
				slog.Warn("ensure library feed config", slog.String("library", string(lib.ID)), slog.Any("err", err))
			}
		}
	}

	favoriteSvc := appfavorite.NewService(r.Favorite)
	photoScanner := filesystem.NewPhotoScanner()
	photoSvc := appphoto.NewService(r.Photo, bus, r.Library, photoScanner)
	starRatingSvc := apprating.NewService(r.Rating, bus, apprating.WithThumbRating(r.UserRating))
	musicScanner := filesystem.NewMusicScanner()
	musicSvc := appmusic.NewService(r.MusicTrack, bus, r.Library, musicScanner)
	musicSvc.CommandService().WithProber(infra.prober)
	libCmdSvc.WithMusicScanner(musicSvc)
	libCmdSvc.WithPhotoScanner(photoSvc)
	playlistSvc := appplaylist.NewService(r.Playlist, r.PlaylistItem, r.SmartPlaylist)

	var subtitleSvc *appsubtitle.Service
	if osAPIKey := resolveAPIKey(ctx, r.ServerCfg, "api.opensubtitles_key", "EVEY_OPENSUBTITLES_API_KEY"); osAPIKey != "" {
		osClient := opensubtitles.New(osAPIKey, os.Getenv("EVEY_OPENSUBTITLES_USERNAME"), os.Getenv("EVEY_OPENSUBTITLES_PASSWORD"), infra.metadataHTTP)
		subtitleSvc = appsubtitle.NewService(r.Subtitle, osClient, opensubtitles.NewLibraryMediaGetter(libQuerySvc))
		rootLog.Info("opensubtitles integration enabled")
	}

	serverCfgSvc := appconfig.NewService(r.ServerCfg)

	backupSvc := appbackup.NewService(filesystem.NewBackupStore(cfg.Backup.Dir),
		postgres.NewDatabaseDumper(cfg.DB.Host, cfg.DB.Port, cfg.DB.User, cfg.DB.Password, cfg.DB.DBName, cfg.DB.SSLMode),
		version, appbackup.WithEventBus(bus))

	syncPlayHub := appsyncplay.NewHub(postgres.NewSyncPlayRoomRepo(db))
	streamTokenStore := appmedia.NewStreamTokenStore(appmedia.DefaultStreamTokenTTL)
	shareLinkRepo := postgres.NewShareLinkRepo(db)
	shareSvc := appmedia.NewShareService(shareLinkRepo)

	hlsTranscoder := ffmpeg.NewHLSTranscoder()
	hlsMgr := apptranscoding.NewHLSManager(ctx, cfg.Transcoding.CacheDir+"/hls", cfg.Transcoding.MaxConcurrent, hlsTranscoder)
	libCmdSvc.SetTrickplayActiveStreamChecker(hlsMgr)

	return &appDeps{
		bus: bus,
		library: libraryDeps{
			cmdSvc:    libCmdSvc,
			querySvc:  libQuerySvc,
			scheduler: libScheduler,
		},
		user: userDeps{
			authSvc:   userAuthSvc,
			userSvc:   userSvc,
			accessSvc: userAccessSvc,
			querySvc:  userQuerySvc,
			oidcSvc:   oidcSvc,
		},
		media: mediaDeps{
			playbackSvc:      playbackSvc,
			transcodingSvc:   transcodingSvc,
			hlsManager:       hlsMgr,
			mkvSvc:           mkvSvc,
			musicSvc:         musicSvc,
			photoSvc:         photoSvc,
			photoTranscoder:  ffmpeg.NewPhotoTranscoder("ffmpeg"),
			subtitleSvc:      subtitleSvc,
			syncPlayHub:      syncPlayHub,
			streamTokenStore: streamTokenStore,
			shareSvc:         shareSvc,
		},
		content: contentDeps{
			metadataCmdSvc:   metadataCmdSvc,
			metadataQuerySvc: metadataQuerySvc,
			enricher:         enricher,
			collectionSvc:    collectionSvc,
			favoriteSvc:      favoriteSvc,
			starRatingSvc:    starRatingSvc,
			playlistSvc:      playlistSvc,
		},
		engagement: engagementDeps{
			activityCmdSvc:     activityCmdSvc,
			activityQuerySvc:   activityQuerySvc,
			notifCmdSvc:        notifCmdSvc,
			notifQuerySvc:      notifQuerySvc,
			feedCmdSvc:         feedCmdSvc,
			feedQuerySvc:       feedQuerySvc,
			webhookSvc:         webhookSvc,
			pluginInstallerSvc: pluginInstallerSvc,
			pluginRuntimeSvc:   pluginRuntimeSvc,
			pluginQuerySvc:     pluginQuerySvc,
			pluginTokenSvc:     pluginTokenSvc,
			pluginHostServer:   pluginHostSrv,
		},
		auth: authDeps{
			tokenGen: infra.tokenGen,
			hasher:   infra.hasher,
		},
		repos: repoDeps{
			artworkRepo:       r.Artwork,
			userRatingRepo:    r.UserRating,
			pushSubRepo:       r.PushSubscription,
			libraryAccessRepo: r.LibraryAccess,
			profileRepo:       r.Profile,
			serverCfgRepo:     r.ServerCfg,
			mediaItemRepo:     r.MediaItem,
			userRepo:          r.User,
			apiKeyRepo:        r.APIKey,
			pluginRepo:        r.Plugin,
			quickConnectRepo:  r.QuickConnect,
		},
		cfgStore:       httpapi.NewServerConfigStore(r.ServerCfg),
		serverCfgSvc:   serverCfgSvc,
		backupSvc:      backupSvc,
		translator:     infra.translator,
		searchHealth:   infra.searchHealth,
		imageConverter: infra.imageConverter,
		frameExtractor: infra.trickplayGen,
	}, nil
}

// buildEventWiring starts background workers, filesystem watcher, and event bus subscriptions.
func buildEventWiring(ctx context.Context, cfg *config.Config, infra *infraDeps, deps *appDeps, rootLog *slog.Logger) {
	r := infra.repos
	bus := infra.bus

	startWatcher(ctx, cfg, bus, r.Library, deps.library.cmdSvc, rootLog)

	rootLog.Info("periodic scan started for all libraries", slog.Duration("interval", cfg.Library.PeriodicScanInterval))
	deps.library.cmdSvc.StartPeriodicScan(ctx, r.Library, cfg.Library.PeriodicScanInterval, nil)
	if infra.tmdbKey != "" {
		deps.content.metadataCmdSvc.StartEnrichmentWorker(ctx)
	}
	deps.library.cmdSvc.StartTrickplayWorker(ctx)
	deps.library.cmdSvc.StartBackgroundGeneration(ctx)

	if rs, ok := infra.searchAdapter.(*search.ResilientSearch); ok {
		if sa, isMeili := rs.Primary().(*meili.SearchAdapter); isMeili {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic recovered in meilisearch startup index", "error", r, "stack", debug.Stack())
				}
			}()
			indexCtx, indexCancel := context.WithTimeout(ctx, 5*time.Minute)
			defer indexCancel()
			dbCount, err := infra.repos.MediaItem.Count(indexCtx, library.ListMediaItemsFilter{})
			if err != nil {
				slog.Warn("startup index: count db items", slog.Any("err", err))
				return
			}
			meiliCount, err := sa.DocumentCount(indexCtx)
			if err == nil && meiliCount == int64(dbCount) {
				slog.Info("startup index: already up to date", slog.Int("items", dbCount))
				return
			}
			const pageSize = 500
			indexed := 0
			offset := 0
			for {
				if indexCtx.Err() != nil {
					slog.Warn("startup index: cancelled", slog.Int("indexed", indexed))
					return
				}
				items, err := infra.repos.MediaItem.List(indexCtx, library.ListMediaItemsFilter{Limit: pageSize, Offset: offset})
				if err != nil {
					slog.Warn("startup index: list items", slog.Any("err", err))
					return
				}
				if len(items) == 0 {
					break
				}
				if err := infra.searchAdapter.IndexMediaItemsBatch(indexCtx, items); err != nil {
					slog.Warn("startup index batch", slog.Int("offset", offset), slog.Any("err", err))
				}
				indexed += len(items)
				offset += len(items)
				if len(items) < pageSize {
					break
				}
			}
			slog.Info("startup index complete", slog.Int("indexed", indexed))
		}()
		}
	}
}

// buildServices constructs all repos, infrastructure adapters, and application services.
// It also starts the filesystem watcher if enabled, and subscribes to bus events.
func buildServices(ctx context.Context, cfg *config.Config, db *sql.DB, rootLog *slog.Logger) (*appDeps, error) {
	infra, err := buildInfra(ctx, cfg, db, rootLog)
	if err != nil {
		return nil, err
	}
	deps, err := buildAppServices(ctx, cfg, db, infra, rootLog)
	if err != nil {
		return nil, err
	}
	buildEventWiring(ctx, cfg, infra, deps, rootLog)
	return deps, nil
}

func startWatcher(ctx context.Context, cfg *config.Config, bus shared.EventBus, libraryRepo library.LibraryRepository, libCmdSvc applibrary.LibraryScanner, rootLog *slog.Logger) *applibrary.WatcherService {
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
	)
	if err := watcherSvc.Start(ctx); err != nil {
		rootLog.Warn("watcher start failed", slog.Any("err", err))
		return nil
	}
	rootLog.Info("filesystem watcher started")
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

// pluginIDLister adapts postgres.PluginRepository to pluginrunner.PluginLister.
type pluginIDLister struct {
	repo interface {
		List(ctx context.Context) ([]*domainplugin.Plugin, error)
	}
}

func (a *pluginIDLister) ListIDs(ctx context.Context) ([]string, error) {
	plugins, err := a.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(plugins))
	for i, p := range plugins {
		ids[i] = p.ID
	}
	return ids, nil
}
