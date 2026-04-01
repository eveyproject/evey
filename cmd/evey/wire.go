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

	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	appbackup "github.com/gravity-zero/evey/internal/app/backup"
	appcollection "github.com/gravity-zero/evey/internal/app/collection"
	appfavorite "github.com/gravity-zero/evey/internal/app/favorite"
	appfeed "github.com/gravity-zero/evey/internal/app/feed"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appmedia "github.com/gravity-zero/evey/internal/app/media"
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
	appsyncplay "github.com/gravity-zero/evey/internal/app/syncplay"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	appwebhook "github.com/gravity-zero/evey/internal/app/webhook"
	"github.com/gravity-zero/evey/internal/adapters/driven/ebml"
	"github.com/gravity-zero/evey/internal/adapters/driven/email"
	webpushadapter "github.com/gravity-zero/evey/internal/adapters/driven/webpush"
	"github.com/gravity-zero/evey/internal/adapters/driven/ffmpeg"
	"github.com/gravity-zero/evey/internal/adapters/driven/filesystem"
	meili "github.com/gravity-zero/evey/internal/adapters/driven/meilisearch"
	"github.com/gravity-zero/evey/internal/adapters/driven/search"
	"github.com/gravity-zero/evey/internal/adapters/driven/webhooksender"
	metadataadapter "github.com/gravity-zero/evey/internal/adapters/driven/metadata"
	"github.com/gravity-zero/evey/internal/adapters/driven/opensubtitles"
	"github.com/gravity-zero/evey/internal/adapters/driven/pluginrunner"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	httpapi "github.com/gravity-zero/evey/internal/adapters/driving/http"
	"github.com/gravity-zero/evey/internal/adapters/driving/pluginhost"
	"github.com/gravity-zero/evey/internal/config"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/metadata"
	domainplugin "github.com/gravity-zero/evey/internal/domain/plugin"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/eventbus"
	eveyi18n "github.com/gravity-zero/evey/internal/pkg/i18n"
	eveylogger "github.com/gravity-zero/evey/internal/pkg/logger"
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

// buildServices constructs all repos, infrastructure adapters, and application services.
// It also starts the filesystem watcher if enabled, and subscribes to bus events.
// ctx is used for watcher and OIDC init; the caller provides the signal context.
func buildServices(ctx context.Context, cfg *config.Config, db *sql.DB, rootLog *slog.Logger) (*appDeps, error) {
	// --- Repositories ---
	r := postgres.NewRepos(db)

	// --- Infrastructure ---
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
	ebmlReader := ebml.NewReader()
	ebmlWriter := ebml.NewWriter("ffmpeg")
	bus := eventbus.New()
	hasher := auth.BcryptHasher{}

	// DB value takes precedence over env var for metadata language
	if langEntry, err2 := r.ServerCfg.Get(context.Background(), "metadata.language"); err2 == nil && langEntry.Value != "" {
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
	tmdbKey := resolveAPIKey(r.ServerCfg, "api.tmdb_key", "EVEY_TMDB_API_KEY")
	tmdbProvider := metadataadapter.NewTMDBProvider(tmdbKey, cfg.Metadata.Language, metadataHTTP)
	imdbProvider := metadataadapter.NewIMDBAPIProvider(metadataHTTP)
	tvmazeProvider := metadataadapter.NewTVMazeProvider(metadataHTTP)
	omdbKey := resolveAPIKey(r.ServerCfg, "api.omdb_key", "EVEY_OMDB_API_KEY")
	omdbProvider := metadataadapter.NewOMDbProvider(omdbKey, metadataHTTP)
	artworkDownloader := metadataadapter.NewArtworkHTTPDownloader(metadataHTTP)
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

	// --- i18n ---
	translator, i18nErr := eveyi18n.NewFromDir("i18n")
	if i18nErr != nil {
		slog.Warn("i18n load failed, using defaults", slog.Any("err", i18nErr))
		translator = eveyi18n.NewEmpty()
	}

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
	libCmdSvc.WithTrickplay(r.Trickplay, trickplayGen, cfg.Transcoding.TrickplayCacheDir, r.ServerCfg)
	if err := libCmdSvc.Validate(); err != nil {
		return nil, fmt.Errorf("library command service: %w", err)
	}
	mediaItemCache := applibrary.NewCachedMediaItemQueryRepo(r.MediaItemQuery, 30*time.Second)
	mediaItemCache.Register(bus)
	personFetcher := metadataadapter.NewTMDBPersonFetcher(tmdbProvider)
	libQuerySvc := applibrary.NewQueryService(r.Library, mediaItemCache, r.MediaStream, searchAdapter).
		WithPeople(r.Person).
		WithPersonFetcher(personFetcher).
		WithTranslations(r.MediaItemTranslation, r.PersonTranslation, personFetcher, eveyi18n.SupportedLocales())

	schedulerLog := eveylogger.NewWith(rootLog, "scheduler")
	libScheduler := applibrary.NewScheduler(r.ScanSchedule, libCmdSvc, schedulerLog)

	userAuthSvc := appuser.NewCommandService(
		r.User, r.Profile, r.RefreshToken,
		r.Role, r.UserRole,
		hasher, tokenGen, bus,
		7*24*time.Hour, r.QuickConnect,
	)
	userAuthSvc.InitSetupState(context.Background())
	userSvc := appuser.NewUserService(r.User, r.Profile, r.RefreshToken, hasher, bus)
	userAccessSvc := appuser.NewAccessService(r.User, r.APIKey, r.InviteLink, r.QuickConnect, hasher, bus).WithProfiles(r.Profile)
	userQuerySvc := appuser.NewQueryService(r.User, r.Profile, r.APIKey, r.InviteLink, r.QuickConnect)

	var oidcSvc *appuser.OIDCService
	if cfg.OIDC.Enabled {
		var oidcErr error
		oidcSvc, oidcErr = appuser.NewOIDCService(
			context.Background(), cfg,
			r.User, r.Profile, r.RefreshToken, r.UserRole,
			tokenGen, bus, 7*24*time.Hour,
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
	enricher := appmetadata.NewEnricher(
		r.Artwork, r.Person, r.MediaPerson, r.MediaItem, searchAdapter,
		tmdbProvider,
		[]metadata.MetadataProvider{omdbProvider, tvmazeProvider, imdbProvider},
		collectionSvc,
	)
	enricher.WithTranslations(r.MediaItemTranslation, r.PersonTranslation, eveyi18n.SupportedLocales())
	metadataCmdSvc.WithEnricher(enricher, r.MediaItem)
	metadataCmdSvc.WithMediaPersons(r.MediaPerson)

	activityCmdSvc := appactivity.NewCommandService(r.LoginHistory, r.ServerMetric, r.AuditLog, bus)
	activityQuerySvc := appactivity.NewQueryService(r.LoginHistory, r.ServerMetric, r.AuditLog, r.PlaySessionStats, r.PlayHistory).
		WithAnalyticsReader(postgres.NewAnalyticsReader(db))

	permChecker := appplugin.NewPermissionChecker(r.Plugin)

	pluginFS := filesystem.NewPluginFileSystem()
	cssSanitizer := pluginrunner.NewCSSSanitizer()
	pluginInstallerSvc := appplugin.NewInstallerService(r.Plugin, pluginRunner, manifestParser, bus).
		WithInstaller(pluginInstaller).
		WithAnalyzer(pluginAnalyzer).
		WithPermissionChecker(permChecker).
		WithFileSystem(pluginFS).
		WithCSSSanitizer(cssSanitizer)
	pluginRuntimeSvc := appplugin.NewRuntimeService(r.Plugin, r.PluginRuntime, pluginRunner, manifestParser, bus).
		WithPermissionChecker(permChecker).
		WithShutdownCtx(ctx)
	pluginRunner.WithIPCCallback(pluginRuntimeSvc.HandleIPCResult)
	pluginQuerySvc := appplugin.NewQueryService(r.Plugin, r.PluginRuntime)

	permInterceptor := appplugin.PluginPermissionInterceptor(permChecker)
	pluginHostSrv := pluginhost.NewHostServer(r.MediaItemQuery, r.Plugin, bus, pluginLogger, permInterceptor)
	pluginLogger.StartCleanup(ctx, &pluginIDLister{repo: r.Plugin})

	// --- Filesystem watcher ---
	_ = startWatcher(ctx, cfg, bus, r.Library, libCmdSvc, rootLog)

	libCmdSvc.WithShutdownCtx(ctx)

	// --- Background workers ---
	// Always run periodic scan for all libraries as a safety net.
	// On WSL2, inotify appears to succeed but never fires events,
	// so we can't rely on failedLibs to detect broken watchers.
	// Scanning an up-to-date library is a no-op (hash check).
	rootLog.Info("periodic scan started for all libraries", slog.Duration("interval", cfg.Library.PeriodicScanInterval))
	libCmdSvc.StartPeriodicScan(ctx, r.Library, cfg.Library.PeriodicScanInterval, nil)
	if tmdbKey != "" {
		metadataCmdSvc.StartEnrichmentWorker(ctx)
	}
	hlsTranscoder := ffmpeg.NewHLSTranscoder()
	hlsMgr := apptranscoding.NewHLSManager(ctx, cfg.Transcoding.CacheDir+"/hls", cfg.Transcoding.MaxConcurrent, hlsTranscoder)
	libCmdSvc.SetTrickplayActiveStreamChecker(hlsMgr)
	libCmdSvc.StartTrickplayWorker(ctx)
	libCmdSvc.StartBackgroundGeneration(ctx)

	if rs, ok := searchAdapter.(*search.ResilientSearch); ok {
		if sa, isMeili := rs.Primary().(*meili.SearchAdapter); isMeili {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic recovered in meilisearch startup index", "error", r, "stack", debug.Stack())
				}
			}()
			indexCtx, indexCancel := context.WithTimeout(ctx, 5*time.Minute)
			defer indexCancel()
			dbCount, err := r.MediaItem.Count(indexCtx, library.ListMediaItemsFilter{})
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
				items, err := r.MediaItem.List(indexCtx, library.ListMediaItemsFilter{Limit: pageSize, Offset: offset})
				if err != nil {
					slog.Warn("startup index: list items", slog.Any("err", err))
					return
				}
				if len(items) == 0 {
					break
				}
				if err := searchAdapter.IndexMediaItemsBatch(indexCtx, items); err != nil {
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

	webhookLog := eveylogger.NewWith(rootLog, "webhook")
	webhookSender := webhooksender.New()
	webhookSvc := appwebhook.NewService(r.Webhook, r.WebhookDelivery, webhookSender, webhookLog, bus)

	notifCmdSvc := appnotification.NewCommandService(r.Notification, r.NotificationSettings, r.WatchHistoryAnalyzer, r.Profile, bus)
	notifCmdSvc.Register()
	notifQuerySvc := appnotification.NewQueryService(r.Notification, r.NotificationSettings)

	vapidPub, vapidPriv := resolveVAPIDKeys(r.ServerCfg, cfg)
	if vapidPub != "" && vapidPriv != "" {
		wpSender := webpushadapter.NewSender(vapidPub, vapidPriv, cfg.VAPID.Subject)
		notifCmdSvc.WithPush(wpSender, r.PushSubscription)
		rootLog.Info("Web Push (VAPID) notifications enabled")
	}

	smtpConfigured := cfg.SMTP.Host != ""
	if smtpConfigured {
		smtpSender := email.NewSMTPSender(cfg.SMTP.Host, cfg.SMTP.Port, cfg.SMTP.Username, cfg.SMTP.Password, cfg.SMTP.From, cfg.SMTP.TLS)
		email.NewNotificationEmailer(smtpSender, r.Profile, r.User).Register(bus)
		email.NewPasswordResetEmailer(smtpSender, cfg.Server.BaseURL).Register(bus)
		rootLog.Info("SMTP email notifications enabled", slog.String("host", cfg.SMTP.Host))
	}
	userAuthSvc.WithPasswordReset(r.PasswordResetToken, r.ServerCfg, smtpConfigured)

	feedCmdSvc := appfeed.NewCommandService(r.FeedConfigCmd, r.FeedConfigQuery).WithEventBus(bus)
	cachedAnalyzer := appfeed.NewCachedAnalyzer(r.WatchHistoryAnalyzer, 5*time.Minute).WithEventBus(bus)
	feedQuerySvc := appfeed.NewQueryService(r.FeedConfigQuery, r.FeedQuerier, cachedAnalyzer)

	// Ensure all existing libraries have feed configs (covers libs created before event system)
	if libs, libErr := r.Library.List(ctx); libErr == nil {
		for _, lib := range libs {
			_ = feedCmdSvc.EnsureLibraryFeedConfig(ctx, lib.ID, lib.Name)
		}
	}

	favoriteSvc := appfavorite.NewService(r.Favorite)
	photoScanner := filesystem.NewPhotoScanner()
	photoSvc := appphoto.NewService(r.Photo, bus, r.Library, photoScanner)
	starRatingSvc := apprating.NewService(r.Rating, bus)
	musicScanner := filesystem.NewMusicScanner()
	musicSvc := appmusic.NewService(r.MusicTrack, bus, r.Library, musicScanner)
	playlistSvc := appplaylist.NewService(r.Playlist, r.PlaylistItem, r.SmartPlaylist)

	// --- Subtitle service ---
	var subtitleSvc *appsubtitle.Service
	if osAPIKey := resolveAPIKey(r.ServerCfg, "api.opensubtitles_key", "EVEY_OPENSUBTITLES_API_KEY"); osAPIKey != "" {
		osClient := opensubtitles.New(osAPIKey, os.Getenv("EVEY_OPENSUBTITLES_USERNAME"), os.Getenv("EVEY_OPENSUBTITLES_PASSWORD"), metadataHTTP)
		subtitleSvc = appsubtitle.NewService(r.Subtitle, osClient, opensubtitles.NewLibraryMediaGetter(libQuerySvc))
		rootLog.Info("opensubtitles integration enabled")
	}

	// --- Backup service ---
	backupSvc := appbackup.NewService(filesystem.NewBackupStore(cfg.Backup.Dir),
		postgres.NewDatabaseDumper(cfg.DB.Host, cfg.DB.Port, cfg.DB.User, cfg.DB.Password, cfg.DB.DBName, cfg.DB.SSLMode),
		version).WithEventBus(bus)

	syncPlayHub := appsyncplay.NewHub(postgres.NewSyncPlayRoomRepo(db))
	streamTokenStore := appmedia.NewStreamTokenStore(appmedia.DefaultStreamTokenTTL)
	shareLinkRepo := postgres.NewShareLinkRepo(db)
	shareSvc := appmedia.NewShareService(shareLinkRepo)

	return &appDeps{
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
			playbackSvc:    playbackSvc,
			transcodingSvc: transcodingSvc,
			hlsManager:     hlsMgr,
			mkvSvc:         mkvSvc,
			musicSvc:       musicSvc,
			photoSvc:       photoSvc,
			subtitleSvc:    subtitleSvc,
			syncPlayHub:      syncPlayHub,
			streamTokenStore: streamTokenStore,
			shareSvc:         shareSvc,
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
			pluginHostServer:   pluginHostSrv,
		},
		auth: authDeps{
			tokenGen: tokenGen,
			hasher:   hasher,
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
		backupSvc:      backupSvc,
		translator:     translator,
		searchHealth:   searchHealth,
		imageConverter: imageConverter,
	}, nil
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
	watcherSvc := applibrary.NewWatcherService(fsWatcher, libraryRepo, libCmdSvc, eveylogger.NewWith(rootLog, "watcher")).
		WithEventBus(bus)
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

func resolveAPIKey(cfgRepo shared.ServerConfigRepository, dbKey, envVar string) string {
	// 1. DB has it (set by admin via panel) → use it
	if entry, err := cfgRepo.Get(context.Background(), dbKey); err == nil && entry.Value != "" {
		return entry.Value
	}
	// 2. Env var fallback (docker/compose) → seed to DB so admin sees it
	if v := os.Getenv(envVar); v != "" {
		if setErr := cfgRepo.Set(context.Background(), dbKey, v); setErr != nil {
			slog.Warn("seed api key to db", slog.String("key", dbKey), slog.Any("err", setErr))
		}
		return v
	}
	// 3. Not configured — admin must set via panel
	return ""
}

func resolveVAPIDKeys(cfgRepo shared.ServerConfigRepository, cfg *config.Config) (string, string) {
	pub := cfg.VAPID.PublicKey
	priv := cfg.VAPID.PrivateKey

	if pub == "" || priv == "" {
		if e, err := cfgRepo.Get(context.Background(), "vapid.public_key"); err == nil && e.Value != "" {
			pub = e.Value
		}
		if e, err := cfgRepo.Get(context.Background(), "vapid.private_key"); err == nil && e.Value != "" {
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
		if err := cfgRepo.Set(context.Background(), "vapid.public_key", pub); err != nil {
			slog.Warn("store VAPID public key", slog.Any("err", err))
		}
		if err := cfgRepo.Set(context.Background(), "vapid.private_key", priv); err != nil {
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
