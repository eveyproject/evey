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
	"strconv"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/email"
	"github.com/evey-project/evey/internal/adapters/driven/ffmpeg"
	"github.com/evey-project/evey/internal/adapters/driven/filesystem"
	"github.com/evey-project/evey/internal/adapters/driven/introproviders"
	itunesadapter "github.com/evey-project/evey/internal/adapters/driven/itunes"
	meili "github.com/evey-project/evey/internal/adapters/driven/meilisearch"
	metadataadapter "github.com/evey-project/evey/internal/adapters/driven/metadata"
	mkvgoadapter "github.com/evey-project/evey/internal/adapters/driven/mkvgo"
	"github.com/evey-project/evey/internal/adapters/driven/opensubtitles"
	"github.com/evey-project/evey/internal/adapters/driven/pluginrunner"
	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/search"
	"github.com/evey-project/evey/internal/adapters/driven/webhooksender"
	webpushadapter "github.com/evey-project/evey/internal/adapters/driven/webpush"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi"
	httpapi "github.com/evey-project/evey/internal/adapters/driving/http"
	"github.com/evey-project/evey/internal/adapters/driving/pluginhost"
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
	"github.com/evey-project/evey/internal/config"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/metadata"
	domainplugin "github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/cpuutil"
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
	repos             *postgres.Repos
	searchAdapter     library.SearchIndex
	searchHealth      httpapi.SearchHealthChecker
	scanner           *filesystem.Scanner
	prober            *ffmpeg.Prober
	transcoder        *ffmpeg.Transcoder
	hwDetector        *ffmpeg.HWDetector
	trickplayGen      *ffmpeg.TrickplayGenerator
	introDetector     *ffmpeg.IntroDetector
	imageConverter    *ffmpeg.ImageConverter
	mkvAdapter        *mkvgoadapter.Adapter
	bus               *eventbus.InMemoryEventBus
	hasher            auth.BcryptHasher
	tokenGen          *auth.JWTGenerator
	translator        *eveyi18n.Translator
	metadataHTTP      *http.Client
	tmdbProvider      *metadataadapter.TMDBProvider
	imdbProvider      *metadataadapter.IMDBAPIProvider
	tvmazeProvider    *metadataadapter.TVMazeProvider
	omdbProvider      *metadataadapter.OMDbProvider
	artworkDL         *metadataadapter.ArtworkHTTPDownloader
	personFetcher     *metadataadapter.TMDBPersonFetcher
	pluginRunner      *pluginrunner.Runner
	pluginLogger      *pluginrunner.PluginLogger
	manifestParser    *pluginrunner.YAMLManifestParser
	pluginInstaller   *pluginrunner.ZipInstaller
	pluginAnalyzer    *pluginrunner.StaticAnalyzer
	pluginSigVerifier *pluginrunner.CosignVerifier
	subScanner        *filesystem.SubtitleScanner
	tmdbKey           string
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
	introDetector := ffmpeg.NewIntroDetector("ffmpeg", "ffprobe")
	imageConverter := ffmpeg.NewImageConverter("ffmpeg")
	mkvAdapter := mkvgoadapter.New()
	bus := eventbus.New()
	hasher := auth.BcryptHasher{}

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
	// Resolution order: metadata.language override → server.language (hydrates
	// everything when set via /setup or admin) → hardcoded default ("en").
	tmdbProvider := metadataadapter.NewTMDBProviderWithGetter(tmdbKey, func() string {
		bg := context.Background()
		if entry, err2 := serverCfgForLang.Get(bg, "metadata.language"); err2 == nil && entry.Value != "" {
			return entry.Value
		}
		if entry, err2 := serverCfgForLang.Get(bg, "server.language"); err2 == nil && entry.Value != "" {
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
	// Dev mode can run without /app/libevey-sandbox.so (e.g. on a developer
	// laptop). Production is strict: a plugin without the 'subprocess' perm
	// that can't be sandboxed fails closed.
	//
	// SECURITY: emit a prominent warning when the sandbox bypass is active so
	// that ops can catch a misconfigured production instance immediately.
	if cfg.Log.DevMode {
		// Hard refusal: DevMode disables the sandbox; an operator who also
		// declares EVEY_ENV=production almost certainly mis-set DevMode and
		// would ship an unsandboxed plugin runtime. Fail closed.
		if err := refuseDevModeInProd(os.Getenv("EVEY_ENV")); err != nil {
			return nil, err
		}
		slog.Warn("SECURITY WARNING: plugin sandbox bypass is ENABLED — " +
			"DevMode=true must NEVER be set in production; " +
			"plugins without the 'subprocess' permission will start unsandboxed")
	}
	pluginRunner := pluginrunner.NewRunner().
		WithLogger(pluginLogger).
		WithAllowMissingSandbox(cfg.Log.DevMode).
		WithSandboxMode(cfg.Plugins.SandboxMode).
		WithResourceLimits(cfg.Plugins.MemoryMB*1024*1024, cfg.Plugins.PIDsMax).
		WithCPUNice(cfg.Plugins.CPUNice).
		WithDiskQuota(int64(cfg.Plugins.DiskMaxMB) * 1024 * 1024).
		WithUIDSeparation(cfg.Plugins.UIDSeparation).
		WithEventBus(bus)
	manifestParser := pluginrunner.NewManifestParser()
	pluginInstaller := pluginrunner.NewZipInstaller()
	pluginAnalyzer := pluginrunner.NewStaticAnalyzer()
	pluginSigVerifier := pluginrunner.NewCosignVerifier(cfg.Plugins.CosignPolicy, cfg.Plugins.CosignPublicKey)

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
		repos:             r,
		searchAdapter:     searchAdapter,
		searchHealth:      searchHealth,
		scanner:           scanner,
		prober:            prober,
		transcoder:        transcoder,
		hwDetector:        hwDetector,
		trickplayGen:      trickplayGen,
		introDetector:     introDetector,
		imageConverter:    imageConverter,
		mkvAdapter:        mkvAdapter,
		bus:               bus,
		hasher:            hasher,
		tokenGen:          tokenGen,
		translator:        translator,
		metadataHTTP:      metadataHTTP,
		tmdbProvider:      tmdbProvider,
		imdbProvider:      imdbProvider,
		tvmazeProvider:    tvmazeProvider,
		omdbProvider:      omdbProvider,
		artworkDL:         artworkDownloader,
		personFetcher:     personFetcher,
		pluginRunner:      pluginRunner,
		pluginLogger:      pluginLogger,
		manifestParser:    manifestParser,
		pluginInstaller:   pluginInstaller,
		pluginAnalyzer:    pluginAnalyzer,
		pluginSigVerifier: pluginSigVerifier,
		subScanner:        subScanner,
		tmdbKey:           tmdbKey,
	}, nil
}

// buildAppServices creates all application-level services from infra dependencies.
func buildAppServices(ctx context.Context, cfg *config.Config, db *sql.DB, infra *infraDeps, rootLog *slog.Logger) (*appDeps, error) {
	r := infra.repos
	bus := infra.bus

	// Push the admin-configured parser token extensions into the filesystem
	// package before any scan can fire. ApplyParserConfig is a no-op when
	// the server_config row is absent (fresh install) or invalid JSON, so
	// it's safe to call unconditionally on every boot.
	applibrary.ApplyParserConfig(ctx, r.ServerCfg)
	// Live-reload: re-apply whenever the admin edits parser.config via the
	// server-config UI. The subscriber stays for the process lifetime; no
	// unsubscribe needed (single global instance).
	bus.Subscribe(shared.ServerConfigChanged{}.EventName(), func(evtCtx context.Context, evt shared.Event) {
		if c, ok := evt.(shared.ServerConfigChanged); ok && c.Key == applibrary.ParserConfigKey {
			applibrary.ApplyParserConfig(evtCtx, r.ServerCfg)
		}
	})

	// Plugin resource caps: overlay any admin overrides (server_config) on the
	// env/boot defaults and push them into the runner. Applied at boot (saved
	// overrides take effect immediately) and live on every plugins.* change —
	// memory/PID/disk on the next monitor sweep, nice/sandbox on the next plugin
	// start. An absent/invalid row falls back to the default, never blocking.
	pluginResDefaults := appplugin.ResourceDefaults{
		MemoryMB:    int(cfg.Plugins.MemoryMB),
		PIDsMax:     cfg.Plugins.PIDsMax,
		DiskMaxMB:   cfg.Plugins.DiskMaxMB,
		CPUNice:     cfg.Plugins.CPUNice,
		SandboxMode: cfg.Plugins.SandboxMode,
	}
	appplugin.ApplyResourceConfig(ctx, r.ServerCfg, infra.pluginRunner, pluginResDefaults)
	bus.Subscribe(shared.ServerConfigChanged{}.EventName(), func(evtCtx context.Context, evt shared.Event) {
		if c, ok := evt.(shared.ServerConfigChanged); ok && appplugin.ResourceConfigKeys[c.Key] {
			appplugin.ApplyResourceConfig(evtCtx, r.ServerCfg, infra.pluginRunner, pluginResDefaults)
		}
	})

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
	cachingPersonRepo := applibrary.NewCachingPersonRepo(r.Person, infra.personFetcher, infra.personFetcher, r.PersonTranslation, eveyi18n.SupportedLocales())
	libQuerySvc := applibrary.NewQueryService(r.Library, mediaItemCache, r.MediaStream, infra.searchAdapter,
		applibrary.WithPeople(cachingPersonRepo),
		applibrary.WithTranslations(r.MediaItemTranslation, r.PersonTranslation),
		applibrary.WithChapters(r.Chapter),
		applibrary.WithTrickplay(r.Trickplay),
		applibrary.WithSegments(r.MediaSegment),
	)

	schedulerLog := eveylogger.NewWith(rootLog, "scheduler")
	libScheduler := applibrary.NewScheduler(r.ScanSchedule, libCmdSvc, schedulerLog)

	// SMTP status is known here (before userAuthSvc is built) so password
	// reset is wired through constructor options, not post-hoc. The actual
	// email Register() calls happen further down after the services exist.
	smtpConfigured := cfg.SMTP.Host != ""
	userAuthOpts := []appuser.CommandServiceOption{
		appuser.WithPasswordReset(r.PasswordResetToken, r.ServerCfg, smtpConfigured),
		appuser.WithUnitOfWork(postgres.NewTxUnit(db)),
		appuser.WithLoginAttemptRepository(r.LoginAttempt),
	}
	userAuthSvc := appuser.NewCommandService(
		r.User, r.Profile, r.RefreshToken,
		r.Role, r.UserRole,
		infra.hasher, infra.tokenGen, bus,
		7*24*time.Hour, r.QuickConnect,
		userAuthOpts...,
	)
	userAuthSvc.InitSetupState(ctx)
	// server.language (admin-editable, set via /setup) drives the default UI
	// locale for new profiles. Fallback: server.language → cfg default ("en").
	serverLocaleFn := func() string {
		bg := context.Background()
		if entry, err2 := r.ServerCfg.Get(bg, "server.language"); err2 == nil && entry.Value != "" {
			return entry.Value
		}
		return cfg.Metadata.Language
	}
	userSvc := appuser.NewUserService(r.User, r.Profile, r.RefreshToken, infra.hasher, bus, postgres.NewTxUnit(db), appuser.WithDefaultLocale(serverLocaleFn))
	userAccessSvc := appuser.NewAccessService(r.User, r.APIKey, r.InviteLink, r.QuickConnect, infra.hasher, bus, appuser.WithProfiles(r.Profile), appuser.WithLibraryAccess(r.LibraryAccess), appuser.WithAccessDefaultLocale(serverLocaleFn), appuser.WithAccessUnitOfWork(postgres.NewTxUnit(db)))
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

	// Buffer player-tick progress writes so the (~5s) hot path skips DB I/O.
	// Periodic flusher upserts the latest state per (profile, media). On
	// crash up to one flush interval of progress is lost — acceptable for a
	// self-hosted single instance per the backlog tradeoff.
	progressBuffer := appplayback.NewWatchProgressBuffer(r.WatchProgress, appplayback.DefaultWatchProgressFlushInterval)
	// Wire the cached media-item lookup into ReportProgressBySession so it
	// can auto-mark items as completed at 90% without round-tripping
	// client-side state. The mediaItemCache is in-memory (30s TTL) and
	// drops the per-tick cost to a map lookup after the first hit.
	durationGetter := mediaItemDurationGetter{cache: mediaItemCache}
	playbackSvc := appplayback.NewService(
		r.WatchProgress, r.WatchProgress, r.PlaySession, r.PlaySession, r.User, r.Profile, bus,
		appplayback.WithUnitOfWork(postgres.NewTxUnit(db)),
		appplayback.WithWatchProgressBuffer(progressBuffer),
		appplayback.WithMediaItemDurationGetter(durationGetter),
	)

	transcodingSvc := apptranscoding.NewService(
		infra.transcoder, infra.hwDetector, r.TranscodeJob, r.TranscodeProfile,
		bus, cfg.Transcoding.MaxConcurrent,
		apptranscoding.WithCacheStore(filesystem.NewCacheStore(cfg.Transcoding.CacheDir)),
	)

	mkvSvc := appmkv.NewService(infra.mkvAdapter, infra.mkvAdapter)
	collectionSvc := appcollection.NewService(r.Collection, r.Collection, postgres.NewCollectionSmartQuerier(db), r.MediaItem)

	// Wrap each provider with retry + TTL cache. Retry covers transient
	// network/5xx failures; cache memoizes repeat search/get-by-id for
	// scan/refresh cycles. Unwrap preserves capability discovery
	// (EpisodeDetailProvider, TranslationProvider).
	decorate := func(p metadata.MetadataProvider) metadata.MetadataProvider {
		return appmetadata.NewCachedProvider(
			appmetadata.NewRetryProvider(p, 3, 250*time.Millisecond),
			10*time.Minute,
		)
	}
	tmdbWrapped := decorate(infra.tmdbProvider)
	omdbWrapped := decorate(infra.omdbProvider)
	tvmazeWrapped := decorate(infra.tvmazeProvider)
	imdbWrapped := decorate(infra.imdbProvider)
	metaProviders := []metadata.MetadataProvider{tmdbWrapped, omdbWrapped, tvmazeWrapped, imdbWrapped}
	metadataQuerySvc := appmetadata.NewQueryService(r.MetadataOverride, r.Artwork, metaProviders)
	enricher := appmetadata.NewEnricher(
		r.Artwork, r.Person, r.MediaPerson, r.MediaItem, infra.searchAdapter,
		tmdbWrapped,
		[]metadata.MetadataProvider{omdbWrapped, tvmazeWrapped, imdbWrapped},
		collectionSvc,
		appmetadata.EnricherWithTranslations(r.MediaItemTranslation, r.PersonTranslation, eveyi18n.SupportedLocales()),
		appmetadata.EnricherWithPersonFetcher(infra.personFetcher),
		appmetadata.EnricherWithOverrides(r.MetadataOverride),
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
	pluginSecurityDashboardSvc := appplugin.NewSecurityDashboardService(r.Plugin, bus, infra.pluginRunner.EffectiveSandboxMode)

	permInterceptor := grpcapi.PluginPermissionInterceptor(permChecker)
	pluginHostSrv := pluginhost.NewHostServer(r.MediaItemQuery, r.Plugin, bus, infra.pluginLogger, permInterceptor).
		WithDataStore(r.PluginData)
	infra.pluginRunner.WithHostRegistry(pluginHostSrv)
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
	notifOpts = append(notifOpts, appnotification.WithAdminLister(r.Profile))
	if infra.translator != nil {
		notifOpts = append(notifOpts, appnotification.WithTranslator(infra.translator))
	}
	notifCmdSvc := appnotification.NewCommandService(r.Notification, r.NotificationSettings, r.WatchHistoryAnalyzer, r.Profile, bus, notifOpts...)
	notifCmdSvc.Register()
	notifQuerySvc := appnotification.NewQueryService(r.Notification, r.NotificationSettings, appnotification.WithPushSubs(r.PushSubscription))

	if smtpConfigured {
		smtpSender := email.NewSMTPSender(cfg.SMTP.Host, cfg.SMTP.Port, cfg.SMTP.Username, cfg.SMTP.Password, cfg.SMTP.From, cfg.SMTP.TLS)
		email.NewNotificationEmailer(smtpSender, r.Profile, r.User).Register(bus)
		email.NewPasswordResetEmailer(smtpSender, cfg.EffectiveBaseURL).Register(bus)
		rootLog.Info("SMTP email notifications enabled", slog.String("host", cfg.SMTP.Host))
	}
	// userAuthSvc already wired with password reset + UoW options above.

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

	favoriteSvc := appfavorite.NewService(r.Favorite)
	photoScanner := filesystem.NewPhotoScanner()
	photoSvc := appphoto.NewService(r.Photo, bus, r.Library, photoScanner)
	starRatingSvc := apprating.NewService(r.Rating, bus, apprating.WithThumbRating(r.UserRating))
	musicScanner := filesystem.NewMusicScanner()
	musicSvc := appmusic.NewService(r.MusicTrack, bus, r.Library, musicScanner)
	musicSvc.WithPlayRepository(r.MusicPlay)
	musicSvc.CommandService().WithProber(infra.prober)
	musicSvc.CommandService().WithAlbumArtFetcher(itunesadapter.NewAlbumArtFetcher(infra.metadataHTTP), cfg.Transcoding.CacheDir)
	libCmdSvc.WithMusicScanner(musicSvc)
	libCmdSvc.WithPhotoScanner(photoSvc)
	playlistSvc := appplaylist.NewService(r.Playlist, r.PlaylistItem, r.SmartPlaylist, r.MediaItemQuery)
	playlistSvc.WithMusicTrackChecker(musicTrackCheckerAdapter{repo: r.MusicTrack})
	playlistGenSvc := appplaylist.NewGeneratorService(r.MusicTrack, r.MusicPlay, r.Playlist, r.PlaylistItem)

	var subtitleSvc *appsubtitle.Service
	if osAPIKey := resolveAPIKey(ctx, r.ServerCfg, "api.opensubtitles_key", "EVEY_OPENSUBTITLES_API_KEY"); osAPIKey != "" {
		osClient := opensubtitles.New(osAPIKey, os.Getenv("EVEY_OPENSUBTITLES_USERNAME"), os.Getenv("EVEY_OPENSUBTITLES_PASSWORD"), infra.metadataHTTP)
		subtitleSvc = appsubtitle.NewService(r.Subtitle, osClient, opensubtitles.NewLibraryMediaGetter(libQuerySvc))
		rootLog.Info("opensubtitles integration enabled")
	}

	serverCfgSvc := appconfig.NewService(r.ServerCfg, appconfig.WithEventBus(bus))

	backupSvc := appbackup.NewService(filesystem.NewBackupStore(cfg.Backup.Dir),
		postgres.NewDatabaseDumper(cfg.DB.Host, cfg.DB.Port, cfg.DB.User, cfg.DB.Password, cfg.DB.DBName, cfg.DB.SSLMode),
		version, appbackup.WithEventBus(bus))
	backupSchedulerLog := eveylogger.NewWith(rootLog, "backup-scheduler")
	backupScheduler := appbackup.NewScheduler(r.BackupSchedule, backupSvc, backupSchedulerLog)

	syncPlayHub := appsyncplay.NewHub(postgres.NewSyncPlayRoomRepo(db))
	deviceRegistry := appsyncplay.NewDeviceRegistry()
	streamTokenStore := appmedia.NewStreamTokenStore(appmedia.DefaultStreamTokenTTL)
	shareLinkRepo := postgres.NewShareLinkRepo(db)
	shareSvc := appmedia.NewShareService(shareLinkRepo)

	hlsTranscoder := ffmpeg.NewHLSTranscoder()
	hlsVODCache, err := apptranscoding.NewSegmentCache(cfg.Transcoding.CacheDir)
	if err != nil {
		return nil, fmt.Errorf("hls vod cache: %w", err)
	}
	// LRU sweep on the on-disk segment cache. Cleanup token dropped: the
	// orchestrator runs for the process lifetime, so the cache sweeper is
	// stopped implicitly when the binary exits.
	_ = hlsVODCache.StartEviction(apptranscoding.EvictionPolicy{
		MaxBytes: int64(cfg.Transcoding.CacheMaxGB) * 1024 * 1024 * 1024,
	}, slog.Default())
	// Inline SourceResolver: the orchestrator hands a DecisionFingerprint
	// back, we look up the media item via the library query service. Streams
	// are filled in later (Phase 3 follow-up) — Phase 1+2 only need the path.
	sourceResolver := apptranscoding.SourceResolverFunc(func(ctx context.Context, fp apptranscoding.DecisionFingerprint) (string, []apptranscoding.SourceStream, error) {
		item, err := libQuerySvc.GetMediaItem(ctx, fp.MediaID)
		if err != nil {
			return "", nil, fmt.Errorf("resolveSource: %w", err)
		}
		if item.File == nil || item.File.Path == "" {
			return "", nil, fmt.Errorf("resolveSource: media %s has no file", fp.MediaID)
		}
		return item.File.Path, nil, nil
	})
	hlsVODOrch := apptranscoding.NewSegmentOrchestrator(hlsVODCache, hlsTranscoder, sourceResolver, apptranscoding.OrchestratorOptions{}, slog.Default())
	libCmdSvc.SetTrickplayActiveStreamChecker(hlsVODOrch)

	introDetectionSvc := applibrary.NewIntroDetectionService(
		r.MediaSegment,
		r.Trickplay,
		r.MediaItem,
		r.ServerCfg,
		infra.introDetector,
		cfg.Transcoding.TrickplayCacheDir,
	)
	introDetectionSvc.WithShutdownCtx(ctx)
	introDetectionSvc.WithFingerprintCache(r.IntroFingerprintCache)
	introDetectionSvc.WithChapterRepo(r.Chapter)
	chroma := ffmpeg.NewChromaprinter("fpcalc")
	if err := chroma.Available(ctx); err == nil {
		introDetectionSvc.WithChromaprinter(chroma)
		slog.Info("intro detection: chromaprint (fpcalc) available, will be used as primary audio confirmation")
	} else {
		slog.Info("intro detection: chromaprint unavailable, falling back to banded Pearson", slog.Any("err", err))
	}
	if v := os.Getenv("EVEY_INTRO_DETECTION_MAX_PARALLEL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			introDetectionSvc.WithMaxParallelDetections(n)
			slog.Info("intro detection: max parallel overridden by env", slog.Int("max", n))
		} else {
			slog.Warn("EVEY_INTRO_DETECTION_MAX_PARALLEL not a number, keeping default", slog.String("value", v))
		}
	} else {
		// Cale la concurrence intro detection sur le budget CPU trickplay :
		// même origine de coût (ffmpeg + analyses GPU-less par épisode), le
		// même réglage `trickplay.adaptive_cpu` doit gouverner les deux. Un
		// défaut hardcodé à 2 ne tient pas compte d'une box 16-core ni d'une
		// box 4-core : on aligne sur cpuutil.TrickplayWorkers (1-4 selon
		// runtime.NumCPU et le mode adaptive).
		adaptive := true
		if r.ServerCfg != nil {
			if entry, err := r.ServerCfg.Get(ctx, "trickplay.adaptive_cpu"); err == nil && entry.Value == "false" {
				adaptive = false
			}
		}
		n := cpuutil.TrickplayWorkers(adaptive)
		introDetectionSvc.WithMaxParallelDetections(n)
		slog.Info("intro detection: max parallel calé sur trickplay budget",
			slog.Int("max", n), slog.Bool("adaptive_cpu", adaptive))
	}

	// External intro/recap/credits providers (TIDB primary, IntroDB fallback)
	// run ahead of MKV chapter pass when enabled. Off by default — opt-in
	// because they require outbound HTTP. The chain layer caches every
	// answer in postgres so a typical scan only hits the network once per
	// episode for the catalog's lifetime.
	//
	// Resolution order for both flags: server_config DB key wins → env var
	// fallback (seeded into DB on first run via resolveAPIKey for the
	// secret). Admin can flip either from the /admin/server-config UI; a
	// restart is required for the providers to (de)activate.
	introProvidersEnabled := cfg.Metadata.IntroProvidersEnabled
	if r.ServerCfg != nil {
		if entry, err := r.ServerCfg.Get(ctx, "intro_providers.enabled"); err == nil && entry.Value != "" {
			introProvidersEnabled = entry.Value == "true"
		}
	}
	tidbKey := resolveAPIKey(ctx, r.ServerCfg, "api.tidb_key", "EVEY_INTRO_TIDB_API_KEY")
	if introProvidersEnabled {
		resolver := introproviders.NewAncestorResolver(r.MediaItem)
		clients := []introproviders.Client{
			introproviders.NewTIDBClient(tidbKey),
			introproviders.NewIntroDBClient(),
		}
		chain := introproviders.NewChainedProvider(resolver, clients...)
		cached := introproviders.NewCached(chain, resolver, r.IntroMetadataCache).
			WithNegativeTTL(30 * 24 * time.Hour)
		introDetectionSvc.WithIntroMetadataProvider(cached)
		slog.Info("intro detection: external providers enabled (TIDB + IntroDB)",
			slog.Bool("tidb_authenticated", tidbKey != ""))

		// Opt-in community contribution: submit high-confidence local detections
		// back to TIDB. Requires an API key and the tidb_submit_enabled toggle.
		tidbSubmitEnabled := false
		if r.ServerCfg != nil {
			if entry, err := r.ServerCfg.Get(ctx, "intro_providers.tidb_submit_enabled"); err == nil {
				tidbSubmitEnabled = entry.Value == "true"
			}
		}
		if tidbKey != "" && tidbSubmitEnabled {
			submitter := introproviders.NewTIDBSubmitter(tidbKey)
			introDetectionSvc.WithSegmentContributor(introproviders.NewTIDBContributor(submitter, resolver))
			slog.Info("intro detection: TIDB community contribution enabled")
		}
	}

	if cfg.Transcoding.CacheMaxGB > 0 {
		evictor := apptranscoding.NewCacheEvictor(
			cfg.Transcoding.CacheDir,
			float64(cfg.Transcoding.CacheMaxGB),
			10*time.Minute,
		)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic recovered in transcode cache evictor", "error", r, "stack", string(debug.Stack()))
				}
			}()
			evictor.Run(ctx)
		}()
		slog.Info("transcode cache evictor started", "cacheDir", cfg.Transcoding.CacheDir, "maxGB", cfg.Transcoding.CacheMaxGB)
	} else {
		slog.Info("transcode cache evictor disabled (CacheMaxGB <= 0)")
	}

	return &appDeps{
		bus: bus,
		library: libraryDeps{
			cmdSvc:            libCmdSvc,
			querySvc:          libQuerySvc,
			scheduler:         libScheduler,
			introDetectionSvc: introDetectionSvc,
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
			hlsVODOrch:       hlsVODOrch,
			hlsVODCache:      hlsVODCache,
			hlsTranscoder:    hlsTranscoder,
			mkvSvc:           mkvSvc,
			musicSvc:         musicSvc,
			photoSvc:         photoSvc,
			photoTranscoder:  ffmpeg.NewPhotoTranscoder("ffmpeg"),
			subtitleSvc:      subtitleSvc,
			syncPlayHub:      syncPlayHub,
			deviceRegistry:   deviceRegistry,
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
			playlistGenSvc:   playlistGenSvc,
		},
		engagement: engagementDeps{
			activityCmdSvc:             activityCmdSvc,
			activityQuerySvc:           activityQuerySvc,
			notifCmdSvc:                notifCmdSvc,
			notifQuerySvc:              notifQuerySvc,
			feedCmdSvc:                 feedCmdSvc,
			feedQuerySvc:               feedQuerySvc,
			webhookSvc:                 webhookSvc,
			pluginInstallerSvc:         pluginInstallerSvc,
			pluginRuntimeSvc:           pluginRuntimeSvc,
			pluginQuerySvc:             pluginQuerySvc,
			pluginTokenSvc:             pluginTokenSvc,
			pluginHostServer:           pluginHostSrv,
			pluginSecurityDashboardSvc: pluginSecurityDashboardSvc,
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
			pluginDataRepo:    r.PluginData,
			quickConnectRepo:  r.QuickConnect,
			introMetaCache:    r.IntroMetadataCache,
		},
		cfgStore:          httpapi.NewServerConfigStore(r.ServerCfg),
		serverCfgSvc:      serverCfgSvc,
		pluginRunner:      infra.pluginRunner,
		pluginResDefaults: pluginResDefaults,
		backupSvc:         backupSvc,
		backupScheduler:   backupScheduler,
		translator:        infra.translator,
		searchHealth:      infra.searchHealth,
		imageConverter:    infra.imageConverter,
		frameExtractor:    infra.trickplayGen,
	}, nil
}

// buildEventWiring starts background workers, filesystem watcher, and event bus subscriptions.
func buildEventWiring(ctx context.Context, cfg *config.Config, infra *infraDeps, deps *appDeps, rootLog *slog.Logger) {
	r := infra.repos
	bus := infra.bus

	// Apply any admin DB overrides for the library watcher + periodic scan
	// BEFORE they start (these settings are consumed here, not re-read later, so
	// a change takes effect on the next restart — see /admin/server-settings).
	loadOverride(ctx, deps.cfgStore, "library.watcher_enabled", parseBool, &cfg.Library.WatcherEnabled)
	loadOverride(ctx, deps.cfgStore, "library.periodic_scan_interval", parseDuration, &cfg.Library.PeriodicScanInterval)

	startWatcher(ctx, cfg, bus, r.Library, deps.library.cmdSvc, rootLog)

	rootLog.Info("periodic scan started for all libraries", slog.Duration("interval", cfg.Library.PeriodicScanInterval))
	deps.library.cmdSvc.StartPeriodicScan(ctx, r.Library, cfg.Library.PeriodicScanInterval, nil)

	// One-shot async backfill of media_items.sort_title for rows scanned
	// before the column was populated. Idempotent — exits cheaply once
	// every row has a non-empty value. Async so we never block boot on
	// it; the only consumer (the lib listing keyset cursor) tolerates
	// a brief window of UUID-sorted results.
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				rootLog.Error("sort_title backfill panic", slog.Any("error", rec))
			}
		}()
		n, err := r.MediaItem.BackfillEmptySortTitles(ctx)
		if err != nil {
			rootLog.Warn("sort_title backfill", slog.Any("err", err))
			return
		}
		if n > 0 {
			rootLog.Info("sort_title backfill complete", slog.Int("rows_updated", n))
		}
	}()
	if infra.tmdbKey != "" {
		deps.content.metadataCmdSvc.StartEnrichmentWorker(ctx)
	}
	// Periodic reconciliation of artwork records that have a SourceURL but
	// no local file on disk — heals transient CDN/network failures from
	// the initial enrichment burst without admin intervention. Runs
	// regardless of provider keys: catches up records inserted by any
	// metadata path (TMDB, OMDb, manual override).
	go deps.content.metadataCmdSvc.RetryMissingArtworksLoop(ctx, time.Hour)

	// Live-reload the artwork auto-download flag on server-config change.
	// Without this the admin toggle would only take effect on next server
	// restart — defeats the point of having a UI switch.
	bus.Subscribe(shared.ServerConfigChanged{}.EventName(), func(_ context.Context, e shared.Event) {
		ev, ok := e.(shared.ServerConfigChanged)
		if !ok || ev.Key != "artwork.auto_download" {
			return
		}
		// Deletion / empty value falls back to the default (true), matching
		// resolveAutoDownloadDefault's semantics on cold start.
		enabled := ev.Deleted || ev.Value != "false"
		deps.content.metadataCmdSvc.SetAutoDownload("/data", enabled)
		slog.Info("artwork auto-download toggled via config", slog.Bool("enabled", enabled))
	})
	deps.library.cmdSvc.StartTrickplayWorker(ctx)
	deps.library.cmdSvc.StartBackgroundGeneration(ctx)
	if deps.library.introDetectionSvc != nil {
		deps.library.introDetectionSvc.WithEventBus(bus)
		deps.library.introDetectionSvc.StartWorker(ctx)
	}

	if rs, ok := infra.searchAdapter.(*search.ResilientSearch); ok {
		if sa, isMeili := rs.Primary().(*meili.SearchAdapter); isMeili {
			go func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("panic recovered in meilisearch startup index", "error", r, "stack", string(debug.Stack()))
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
				// Keyset pagination via AfterID — avoids O(n²) OFFSET scans
				// on large libs (cf backlog #5).
				const pageSize = 500
				indexed := 0
				afterID := ""
				for {
					if indexCtx.Err() != nil {
						slog.Warn("startup index: cancelled", slog.Int("indexed", indexed))
						return
					}
					items, err := infra.repos.MediaItem.List(indexCtx, library.ListMediaItemsFilter{
						Limit:   pageSize,
						AfterID: afterID,
					})
					if err != nil {
						slog.Warn("startup index: list items", slog.Any("err", err))
						return
					}
					if len(items) == 0 {
						break
					}
					if err := infra.searchAdapter.IndexMediaItemsBatch(indexCtx, items); err != nil {
						slog.Warn("startup index batch", slog.Int("indexed", indexed), slog.Any("err", err))
					}
					indexed += len(items)
					afterID = string(items[len(items)-1].ID)
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

// musicTrackCheckerAdapter adapts the music TrackRepository to playlist.MusicTrackChecker.
// Postgres repo returns (nil, nil) when not found; we surface that as ok=false.
type musicTrackCheckerAdapter struct {
	repo *postgres.MusicTrackRepository
}

func (a musicTrackCheckerAdapter) Exists(ctx context.Context, id string) (bool, error) {
	t, err := a.repo.GetByID(ctx, id)
	if err != nil {
		return false, err
	}
	return t != nil, nil
}

// mediaItemDurationGetter adapts the cached MediaItem query repo to the
// playback.MediaItemDurationGetter port. Returns 0 when the item has no
// file (container types) or the file probe never recorded a duration —
// the playback command service skips auto-completion in that case.
type mediaItemDurationGetter struct {
	cache *applibrary.CachedMediaItemQueryRepo
}

func (g mediaItemDurationGetter) GetMediaItemDurationMs(ctx context.Context, mediaItemID string) (int64, error) {
	item, err := g.cache.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil || item == nil || item.File == nil {
		return 0, err
	}
	return item.File.Duration.Milliseconds(), nil
}
