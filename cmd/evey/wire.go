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
	"runtime"
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
	proberpkg "github.com/evey-project/evey/internal/adapters/driven/prober"
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
	appqoe "github.com/evey-project/evey/internal/app/qoe"
	apprating "github.com/evey-project/evey/internal/app/rating"
	appsubtitle "github.com/evey-project/evey/internal/app/subtitle"
	appsyncplay "github.com/evey-project/evey/internal/app/syncplay"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	appuser "github.com/evey-project/evey/internal/app/user"
	appwebhook "github.com/evey-project/evey/internal/app/webhook"
	"github.com/evey-project/evey/internal/config"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/music"
	domainplugin "github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/cpuutil"
	"github.com/evey-project/evey/internal/pkg/eventbus"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	eveylogger "github.com/evey-project/evey/internal/pkg/logger"
	eveymetrics "github.com/evey-project/evey/internal/pkg/metrics"
	"github.com/evey-project/evey/internal/pkg/netmeter"
	"github.com/evey-project/evey/internal/pkg/sysinfo"
)

// Event subscription map (bus.Subscribe calls):
//
//   "media.item.added"      → notification (handleMediaItemAdded)
//                           → trickplay worker (auto-generate on new media)
//                           → metadata enricher (auto-enrich on new media)
//   "backup.failed"         → notification (handleBackupFailed)
//   "library.scan.completed" → keyframe backfill worker (nudge after 10-min settle)
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
	musicSearch       music.MusicSearchIndex
	scanner           *filesystem.Scanner
	prober            *ffmpeg.Prober
	transcoder        *ffmpeg.Transcoder
	hwDetector        *ffmpeg.HWDetector
	trickplayGen      *ffmpeg.TrickplayGenerator
	introDetector     *ffmpeg.IntroDetector
	imageConverter    *ffmpeg.ImageConverter
	mkvAdapter        *mkvgoadapter.Adapter
	onDemandPackager  *mkvgoadapter.OnDemandPackager
	bus               *eventbus.InMemoryEventBus
	hasher            auth.BcryptHasher
	tokenGen          *auth.JWTGenerator
	hlsAESSecret      []byte
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
		searchAdapter = search.NewResilient(sa, sqlFallback).WithQueue(r.PendingSearchWrite)
		searchHealth = sa
	}

	// Music has its own Meilisearch index (dedicated doc shape, library-scoped
	// or global) with the track repo's SQL ILIKE as fallback.
	musicSQLFallback := search.NewMusicRepoFallback(r.MusicTrack)
	var musicSearch music.MusicSearchIndex
	if msa, err := meili.NewMusicSearchAdapter(cfg.Meilisearch.URL, cfg.Meilisearch.APIKey); err != nil {
		slog.Warn("meilisearch music index unavailable, using SQL search fallback", slog.Any("err", err))
		musicSearch = search.NewResilientMusic(nil, musicSQLFallback)
	} else {
		musicSearch = search.NewResilientMusic(msa, musicSQLFallback)
	}

	scanner := filesystem.NewScanner()
	prober := ffmpeg.NewProber("ffprobe")
	transcoder := ffmpeg.NewTranscoder("ffmpeg", cfg.Transcoding.CacheDir)
	hwDetector := ffmpeg.NewHWDetector("ffmpeg")
	// Boot safety net: warn (once, detached, non-blocking) if this ffmpeg build
	// ships no AV1 decoder, so AV1 source files don't fail playback with a
	// cryptic error on a host whose ffmpeg lacks dav1d.
	go func() {
		probeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ffmpeg.WarnIfNoAV1Decoder(probeCtx, "ffmpeg", slog.Default())
	}()
	trickplayGen := ffmpeg.NewTrickplayGenerator("ffmpeg", "ffprobe")
	introDetector := ffmpeg.NewIntroDetector("ffmpeg", "ffprobe")
	imageConverter := ffmpeg.NewImageConverter("ffmpeg")
	mkvAdapter := mkvgoadapter.New()
	onDemandPackager := mkvgoadapter.NewOnDemandPackager()
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
	// nil → the downloader builds its own ssrf.SafeHTTPClient. Artwork URLs come
	// from provider/plugin responses (e.g. OMDb's Poster passthrough), which are
	// untrusted in Evey's threat model — they must not reach a plain client that
	// can hit loopback/link-local. The host-pinned API providers keep metadataHTTP.
	artworkDownloader := metadataadapter.NewArtworkHTTPDownloader(nil)
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

	// Secret for deriving per-media AES-128 HLS keys (on-demand encryption,
	// opt-in). Bootstrapped next to the JWT key, same first-run model. A
	// corrupt/unreadable secret must NOT block boot — encryption is an optional
	// hardening, so degrade to "encryption unavailable" (the handler requires a
	// non-empty secret) and let the operator repair the file.
	hlsAESSecret, secErr := auth.LoadOrGenerateSecret(filepath.Join(filepath.Dir(cfg.JWT.PrivateKeyPath), "hls_aes.key"), 32)
	if secErr != nil {
		slog.Error("hls aes secret unavailable — on-demand HLS encryption disabled until the file is repaired or removed",
			slog.Any("err", secErr))
		hlsAESSecret = nil
	}

	subScanner := filesystem.NewSubtitleScanner()

	return &infraDeps{
		repos:             r,
		searchAdapter:     searchAdapter,
		searchHealth:      searchHealth,
		musicSearch:       musicSearch,
		scanner:           scanner,
		prober:            prober,
		transcoder:        transcoder,
		hwDetector:        hwDetector,
		trickplayGen:      trickplayGen,
		introDetector:     introDetector,
		imageConverter:    imageConverter,
		mkvAdapter:        mkvAdapter,
		onDemandPackager:  onDemandPackager,
		bus:               bus,
		hasher:            hasher,
		tokenGen:          tokenGen,
		hlsAESSecret:      hlsAESSecret,
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
	applibrary.ApplyParserConfig(ctx, r.ServerCfg, filesystem.SetExtraParserTokens)
	// Live-reload: re-apply whenever the admin edits parser.config via the
	// server-config UI. The subscriber stays for the process lifetime; no
	// unsubscribe needed (single global instance).
	bus.Subscribe(shared.ServerConfigChanged{}.EventName(), func(evtCtx context.Context, evt shared.Event) {
		if c, ok := evt.(shared.ServerConfigChanged); ok && c.Key == applibrary.ParserConfigKey {
			applibrary.ApplyParserConfig(evtCtx, r.ServerCfg, filesystem.SetExtraParserTokens)
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

	// Stream-probing fast-path: route .mkv/.mp4 through the in-process mkvgo reader
	// (no ffprobe fork, head-only read), falling back to ffprobe for anything it
	// cannot render field-equivalent (unknown codec, HDR/DoVi ground truth, parse
	// panic). Validated field-equivalent against ffprobe on the real library, so it
	// is always on — there is no scenario where disabling it helps (the fallback is
	// the safety net), hence no toggle/env. nil gates == always on.
	scanProber := proberpkg.New(infra.prober, nil, nil)

	libCmdSvc := applibrary.NewCommandService(
		r.Library, r.Library, r.MediaItem,
		r.MediaStream, r.Person, r.MediaPerson,
		infra.scanner, scanProber, infra.searchAdapter, bus,
		r.Chapter, infra.mkvAdapter,
		r.Trickplay, infra.trickplayGen, cfg.Transcoding.TrickplayCacheDir, r.ServerCfg,
		applibrary.WithShutdownCtx(ctx),
		applibrary.WithSearchReindex(r.MediaItemQuery, r.MediaItemTranslation),
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
	// guardedTokens enforces users.token_epoch on every access-token mint and
	// validation: bumping the epoch (disable, role change, password change,
	// admin session kill) revokes outstanding tokens within ~30s instead of
	// the full 15min expiry. Every consumer below gets the guard; the concrete
	// JWTGenerator stays only where plugin-token methods are needed.
	guardedTokens, stopEpochCache := appuser.NewEpochGuardedTokens(infra.tokenGen, r.User)
	_ = stopEpochCache // cache janitor lives for the process lifetime
	userAuthOpts := []appuser.CommandServiceOption{
		appuser.WithPasswordReset(r.PasswordResetToken, r.ServerCfg, smtpConfigured),
		appuser.WithUnitOfWork(postgres.NewTxUnit(db)),
		appuser.WithLoginAttemptRepository(r.LoginAttempt),
		appuser.CommandWithTokenRevoker(guardedTokens),
	}
	userAuthSvc := appuser.NewCommandService(
		r.User, r.Profile, r.RefreshToken,
		r.Role, r.UserRole,
		infra.hasher, guardedTokens, bus,
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
	userSvc := appuser.NewUserService(r.User, r.Profile, r.RefreshToken, infra.hasher, bus, postgres.NewTxUnit(db), appuser.WithDefaultLocale(serverLocaleFn), appuser.WithTokenRevoker(guardedTokens))
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
			guardedTokens, bus, postgres.NewTxUnit(db), 7*24*time.Hour,
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
		// Item update + artwork replace + cast upsert commit or roll back
		// together — a mid-enrichment failure no longer strands the item with a
		// fresh title but no artwork/cast (unrepairable except by a manual
		// "Refresh metadata", since the re-scan trigger keys on empty Overview).
		appmetadata.EnricherWithUnitOfWork(postgres.NewTxUnit(db)),
	)
	metadataCmdSvc := appmetadata.NewCommandService(r.MetadataOverride, r.Artwork, infra.artworkDL, bus, metaProviders,
		appmetadata.WithEnricher(enricher, r.MediaItem),
		appmetadata.WithMediaPersons(r.MediaPerson),
	)
	// Fire the artwork auto-download the moment the enricher persists a new record
	// (no-op unless the admin toggle artwork.auto_download is on, checked inside
	// maybeAutoDownload). Set here rather than as a NewEnricher option because
	// metadataCmdSvc is built WITH the enricher — the hook back-reference only
	// exists once both do.
	enricher.SetArtworkPostSaveHook(metadataCmdSvc.ArtworkAutoDownloadHook())

	activityCmdSvc := appactivity.NewCommandService(r.LoginHistory, r.ServerMetric, r.AuditLog, bus)
	activityQuerySvc := appactivity.NewQueryService(r.LoginHistory, r.ServerMetric, r.AuditLog, r.PlaySessionStats, r.PlayHistory,
		appactivity.WithAnalyticsReader(postgres.NewAnalyticsReader(db)),
	)
	// Counts the bytes the streaming routes actually serve; the sampler turns
	// that into the server_metrics history (dashboard rate + consumed total).
	streamMeter := &netmeter.Meter{}
	metricsSampler := appactivity.NewMetricsSampler(activityCmdSvc, r.PlaySessionStats, streamMeter,
		func() appactivity.SysSample {
			s := sysinfo.Collect()
			return appactivity.SysSample{CPUPercent: s.CPUPercent, MemUsedBytes: s.MemUsedBytes, MemTotalBytes: s.MemTotalBytes}
		}, rootLog)

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
	pluginSecurityDashboardSvc := appplugin.NewSecurityDashboardService(r.Plugin, bus, infra.pluginRunner.EffectiveSandboxMode).
		WithSandboxStatus(func() appplugin.SandboxStatus {
			return appplugin.SandboxStatus(infra.pluginRunner.SandboxStatus())
		})

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
	starRatingSvc := apprating.NewService(r.UserRating)
	musicScanner := filesystem.NewMusicScanner()
	musicSvc := appmusic.NewService(r.MusicTrack, bus, r.Library, musicScanner)
	musicSvc.WithPlayRepository(r.MusicPlay)
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
	libCmdSvc.WithMusicScanner(musicSvc)
	libCmdSvc.WithPhotoScanner(photoSvc)
	playlistSvc := appplaylist.NewService(r.Playlist, r.PlaylistItem, r.SmartPlaylist, r.MediaItemQuery)
	playlistSvc.WithMusicTrackChecker(musicTrackCheckerAdapter{repo: r.MusicTrack})
	// AddItem resolves the media item (for its library) and gates it on the
	// acting profile's library access, so a profile cannot add — and then read
	// the metadata of — media from a library it has no access to.
	playlistSvc.WithMediaChecker(mediaItemCheckerAdapter{repo: r.MediaItem})
	playlistSvc.WithLibraryAccessChecker(playlistLibraryAccessAdapter{profiles: r.Profile, access: r.LibraryAccess})
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
	// Bound the sibling fast-start MP4 remux cache (<CacheDir>/remux) on its own
	// budget. Source-fingerprint keying orphans the old file on every re-scan, so
	// without this it grows without bound. Same lifetime/stop semantics as above.
	_ = apptranscoding.NewRemuxCacheJanitor(filepath.Join(cfg.Transcoding.CacheDir, "remux")).
		StartEviction(apptranscoding.EvictionPolicy{
			MaxBytes: apptranscoding.DefaultRemuxCacheBytes,
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
	// Global concurrency backstop for the HLS-VOD orchestrator: cap the total
	// number of concurrent ffmpeg rafales at the host's core count. This bounds a
	// hash-fanout DoS (an authenticated client spawning N software x264 sessions
	// across distinct media or diversified decision hashes) without throttling
	// realistic multi-user streaming: a weak NAS caps at its real capacity (2
	// cores -> 2, which it could not exceed acceptably anyway), a capable host
	// stays generous. Past the cap the player receives 503 + Retry-After and
	// resumes once a slot frees. NumCPU is a coarse default; surfacing it in the
	// admin transcoding panel or deriving it from encode calibration is a follow-up.
	// EVEY_TRANSCODING_MAX_SESSIONS overrides it: a weak NAS that can barely
	// sustain ONE 4K HDR software transcode must cap this at 1-2, or concurrent
	// sessions (ABR probes + seeks) starve each other and the orchestrator
	// kills/evicts them mid-segment (the 360p-with-buffering thrash). 0 = NumCPU.
	maxHLSSessions := cfg.Transcoding.MaxSessions
	if maxHLSSessions <= 0 {
		maxHLSSessions = runtime.NumCPU()
	}
	hlsVODOrch := apptranscoding.NewSegmentOrchestrator(hlsVODCache, hlsTranscoder, sourceResolver, apptranscoding.OrchestratorOptions{
		ThrottleEnabled:       true,
		MaxActiveSessions:     maxHLSSessions,
		HWFirstSegmentTimeout: cfg.Transcoding.HWFirstSegmentTimeout,
		HWPerVariant:          cfg.Transcoding.HWPerVariant,
		SingleVariant:         cfg.Transcoding.SingleVariant,
	}, slog.Default())

	// The software StreamProducer is built HERE (not in the HTTP route
	// registration) so it is a SINGLE instance shared between two consumers: the
	// HTTP VOD handler's routing selector (which creates its sessions) and the
	// gRPC StopStream / admin-kill path (which must be able to tear them down).
	// Without the shared instance, a stopped software stream would linger until
	// the producer's idle GC. Same sourceResolver as the orchestrator → SW and
	// HW resolve the source identically.
	hlsVODSoftware := apptranscoding.NewSoftwareProducer(
		hlsVODCache, hlsTranscoder, sourceResolver,
		apptranscoding.SoftwareProducerOptions{}, slog.Default(),
	)
	// Fan-out remover for explicit StopStream / admin force-kill: RemoveSession
	// reaches BOTH producers, and ActiveSessionCount sums them. For()/routing is
	// unused on this instance (the HTTP handler owns the routing selector with
	// the real resolveHW predicate), so the predicate is nil.
	hlsVODRemover := apptranscoding.NewProducerSelector(hlsVODSoftware, hlsVODOrch, nil)
	// Trickplay generation throttles while any stream is live — feed it the
	// fan-out selector so SOFTWARE streams also hold it off, not just HW ones (a
	// weak NAS must not burn CPU on trickplay during an SW transcode).
	libCmdSvc.SetTrickplayActiveStreamChecker(hlsVODRemover)

	// Audio rendition orchestrator: shares the segment cache with the
	// video pipeline but addresses sessions by AudioFingerprint (its own
	// hash space). Always instantiated; the HTTP audio handlers gate on
	// the Cfg.Transcoding.AudioRenditionEnabled flag, so leaving this
	// wired even when the feature is off costs nothing (no ffmpeg
	// spawned until a request comes in via a route that's also gated).
	audioOrch := apptranscoding.NewAudioSegmentOrchestrator(hlsVODCache, hlsTranscoder)

	// Per-title complexity classifier: background worker samples 30 frames
	// per file, computes YDEV + scenecut metrics, maps to a content class
	// stored on media_items. profilesForDecision reads it at manifest time
	// to pick the appropriate ABR ladder. Repo nil-safe across the path —
	// unclassified items keep getting LadderFilmNormal.
	complexityRepo := postgres.NewMediaComplexityRepo(db)
	complexityClassifier := ffmpeg.NewComplexityClassifier("ffmpeg", "ffprobe")

	// Encode capability calibration: bench libx264 + any HW backend
	// against a fixed ladder so the HLS-VOD handler can cap the ladder
	// at what THIS host actually sustains in realtime. Without this,
	// the 4K rung on Raspberry Pi would lock the multi-variant ffmpeg
	// pipeline below realtime and seg 0 would never land in time.
	//
	// Bench is blocking on first boot (~20 s) — the safe 720p fallback
	// in the service constructor keeps requests served until it returns.
	// Persisted across restarts via server_settings + CPU signature
	// invalidation, so the 20 s only hits once per host lifetime.
	capabilityRepo := apptranscoding.NewCapabilityRepoAdapter(r.ServerCfg)
	// The HW-acceleration mode is admin-selectable (auto/off/nvenc/vaapi/qsv).
	// Bootstrap precedence: an explicit transcoding.hw_mode (persisted admin
	// override, else env) wins; otherwise derive it from the legacy
	// HWAccelEnabled + HWBackend knobs. We read the persisted overrides DIRECTLY
	// here because the calibration service is built before main's loadOverride
	// pass — without this, an admin's saved mode would be ignored until they
	// re-set it. Calibration honours the mode (pins/benches the chosen backend,
	// or software-only on off); the admin panel flips it live via SetHWMode.
	cfgOverride := func(key string) string {
		if entry, err := r.ServerCfg.Get(ctx, key); err == nil && entry != nil {
			return entry.Value
		}
		return ""
	}
	hwAccelEnabled := cfg.Transcoding.HWAccelEnabled
	if v := cfgOverride("transcoding.hw_accel_enabled"); v != "" {
		hwAccelEnabled = v == "true"
	}
	hwModeStr := cfg.Transcoding.HWMode
	if v := cfgOverride("transcoding.hw_mode"); v != "" {
		hwModeStr = v
	}
	hwMode := domaintranscoding.ResolveHWMode(hwModeStr, hwAccelEnabled, cfg.Transcoding.HWBackend)
	// AV1 software ceiling is admin-settable (server_settings), so — like hw_mode
	// above — read the persisted override DIRECTLY from the store here: main's
	// loadOverride pass that hydrates cfg.Transcoding.SoftwareAV1MaxHeightOverride
	// runs AFTER this, so trusting the cfg field would always seed 0 and the
	// admin's saved ceiling would be lost on every restart.
	swAV1Override := cfg.Transcoding.SoftwareAV1MaxHeightOverride
	if v := cfgOverride("transcoding.software_av1_max_height"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			swAV1Override = n
		}
	}
	// The bench must model the runtime pipeline shape: single-variant probes
	// one rung (decode + 1 encode), the cap a single on-demand session can
	// actually sustain. Benching the parallel ladder while the runtime serves
	// single rungs pins the HDR cap far below what the host serves.
	calibrationRunner := ffmpeg.NewBenchmarkRunner(ffmpeg.BenchmarkOpts{
		SingleVariant:   cfg.Transcoding.SingleVariant,
		SDRSamplePath:   cfg.Transcoding.BenchSDRSamplePath,
		SDRSampleURL:    cfg.Transcoding.BenchSDRSampleURL,
		SDRSampleSHA256: cfg.Transcoding.BenchSDRSampleSHA256,
		HDRSamplePath:   cfg.Transcoding.BenchHDRSamplePath,
		HDRSampleURL:    cfg.Transcoding.BenchHDRSampleURL,
		HDRSampleSHA256: cfg.Transcoding.BenchHDRSampleSHA256,
	}, slog.Default())
	calibrationSvc := apptranscoding.NewCalibrationService(capabilityRepo, calibrationRunner, slog.Default(),
		apptranscoding.WithHWMode(hwMode),
		apptranscoding.WithSoftwareCapOverrides(
			cfg.Transcoding.SoftwareMaxHeightOverride,
			cfg.Transcoding.SoftwareHDRMaxHeightOverride),
		apptranscoding.WithSoftwareAV1MaxOverride(swAV1Override))
	// Idle-gate the admin "recalibrate" button on the SAME live-session counter
	// trickplay uses: a manual recalibration run while transcodes are active
	// measures contended throughput and would pin the cap a rung too low. First-boot
	// EnsureCalibrated below is unaffected (it bypasses Recalibrate; no streams at boot).
	calibrationSvc.SetActiveStreamChecker(hlsVODRemover.ActiveSessionCount)
	// Yield ALL background analysis ffmpeg (intro detection, complexity sampling,
	// chromaprint) to real work: each SIGSTOPs itself while a playback/transcode
	// session is live OR a recalibration is running, and resumes when the box is
	// idle — CPU/memory-bandwidth/I/O go to what matters, the decode keeps its
	// progress (no re-run).
	isBusy := func() bool {
		return hlsVODRemover.ActiveSessionCount() > 0 || calibrationSvc.IsBenchmarking()
	}
	if infra.introDetector != nil {
		infra.introDetector.WithBusyChecker(isBusy)
	}
	complexityClassifier.WithBusyChecker(isBusy)
	if err := calibrationSvc.EnsureCalibrated(ctx); err != nil {
		slog.Warn("calibration: EnsureCalibrated returned error, falling back to 720p", slog.Any("err", err))
	}
	// Level-2 HW safety net: a hardware-encode failure demotes the calibrated
	// capability so the client's retry re-resolves a lower tier (encoder-agnostic
	// cache ⇒ transparent). DemoteCapability returns whether it changed; the
	// orchestrator sink ignores the bool.
	hlsVODOrch.SetHWDemoteSink(func(ctx context.Context, class domaintranscoding.FailureClass, codec string) {
		calibrationSvc.DemoteCapability(ctx, class, codec)
	})

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
	)
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
		levelBackfillWorker.Start(ctx)
	}

	// Keyframe backfill: index keyframe times for copy-eligible media so the
	// byte-range/per-segment copy rung can cut deterministic, keyframe-aligned
	// segments. ON by default since the mkvgo era (2026-07-08 decision): the
	// probe reads the container's own seek index in milliseconds and even a
	// cue-less MKV costs a bounded in-process scan, so the historical
	// ffprobe-era caution (opt-in + ultra-slow pacing) no longer applies —
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
	// — a file the fast-path can't index stays on the worker's slow path.
	keyframeOnDemand := apptranscoding.NewKeyframeOnDemand(r.MediaStream, kfProber, slog.Default())

	if resolveFlag(ctx, r.ServerCfg, "transcoding.keyframe_backfill", "EVEY_KEYFRAME_BACKFILL") != "off" {
		kfOpts := apptranscoding.KeyframeBackfillOpts{}
		if v := resolveFlag(ctx, r.ServerCfg, "transcoding.keyframe_backfill_batch", "EVEY_KEYFRAME_BACKFILL_BATCH"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				kfOpts.BatchSize = n
			}
		}
		if v := resolveFlag(ctx, r.ServerCfg, "transcoding.keyframe_backfill_pause_ms", "EVEY_KEYFRAME_BACKFILL_PAUSE_MS"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				kfOpts.ItemPause = time.Duration(n) * time.Millisecond
			}
		}
		if v := resolveFlag(ctx, r.ServerCfg, "transcoding.keyframe_backfill_idle_sec", "EVEY_KEYFRAME_BACKFILL_IDLE_SEC"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				kfOpts.IdleInterval = time.Duration(n) * time.Second
			}
		}
		keyframeBackfillWorker := apptranscoding.NewKeyframeBackfillWorker(
			r.MediaStream,
			mediaItemReaderFunc(func(ctx context.Context, id shared.MediaItemID) (*library.MediaItem, error) {
				return libQuerySvc.GetMediaItem(ctx, string(id))
			}),
			kfProber,
			kfOpts,
			slog.Default(),
		).WithActivePlayback(hlsVODRemover.ActiveSessionCount)
		keyframeBackfillWorker.Start(ctx)
		// Fresh additions: once the library has been quiet for 10 min after a
		// scan, drain the queue right away instead of waiting the idle cycle.
		// (Playback that starts sooner is covered by the on-demand inline fill.)
		bus.Subscribe(library.LibraryScanCompleted{}.EventName(), func(context.Context, shared.Event) {
			keyframeBackfillWorker.NudgeAfterQuiet(10 * time.Minute)
		})
		slog.Default().Info("keyframe backfill worker enabled (default-on; opt out with server_config transcoding.keyframe_backfill=off)")
	}

	// CMCD (CTA-5004) ingest sink. The HTTP handler parses the CMCD query
	// param on every segment / master / variant fetch and ships an event
	// here; the buffered worker drains into playback_qoe_events. Phase 1 is
	// pure observation — no feedback loop yet — so the only failure path we
	// care about is "buffer full" which surfaces via metrics + warn logs.
	qoeStorage := qoeStorageAdapter{repo: postgres.NewPlaybackQoERepo(db)}
	qoeSink := appqoe.NewBufferedSink(qoeStorage, appqoe.SinkOptions{
		BufferSize: 2048, // ~30 min of segments for a busy 5-user evening
		DropMetric: eveymetrics.QoEIngestDrops,
	}, slog.Default())
	qoeQuerySvc := appqoe.NewQueryService(qoeReaderAdapter{reader: postgres.NewPlaybackQoEReader(db)})

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
	introDetectionSvc.WithAudioFingerprintCache(r.IntroAudioFingerprintCache)
	introDetectionSvc.WithChapterRepo(r.Chapter)
	chroma := ffmpeg.NewChromaprinter("fpcalc")
	chroma.WithBusyChecker(isBusy)
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

	// The on-disk segment cache is bounded by hlsVODCache.StartEviction (above):
	// per-hash LRU scoped to <cacheDir>/segments, sorted by atime, with the 4
	// most-recent hashes protected so an actively-played (incl. copy/remux)
	// session is never evicted out from under the player.
	//
	// A second standalone CacheEvictor used to run here on cfg.Transcoding.CacheDir
	// — the PARENT of segments/. It treated segments/, photos/ and
	// music_album_art/ as three LRU units, so on overflow it would have removed a
	// whole sibling cache (every photo thumbnail / all album art) or the entire
	// segments blob at once instead of evicting per-hash. Removed: StartEviction is
	// the single, correctly-scoped evictor. (The old whole-subdir CacheEvictor,
	// which evicted by mtime with no pendingRefs guard, was deleted — it would
	// have evicted a hash mid-write if ever re-wired.)

	return &appDeps{
		bus: bus,
		// The media-item repo doubles as the seek-index fingerprint refresher, so a
		// reindexed file isn't re-probed (and its keyframe index dropped) by the
		// next scan. r.MediaItem satisfies appmaintenance.FileStateRefresher.
		seekIndexFileState: r.MediaItem,
		keyframeOnDemand:   keyframeOnDemand,
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
			hlsVODSoftware:   hlsVODSoftware,
			hlsVODRemover:    hlsVODRemover,
			audioOrch:        audioOrch,
			qoeSink:          qoeSink,
			qoeQuerySvc:      qoeQuerySvc,
			complexityRepo:   complexityRepo,
			calibrationSvc:   calibrationSvc,
			capsOverrideSvc:  capsOverrideSvc,
			mkvSvc:           mkvSvc,
			mkvAdapter:       infra.mkvAdapter,
			musicSvc:         musicSvc,
			photoSvc:         photoSvc,
			photoTranscoder:  ffmpeg.NewPhotoTranscoder("ffmpeg"),
			subtitleSvc:      subtitleSvc,
			syncPlayHub:      syncPlayHub,
			deviceRegistry:   deviceRegistry,
			streamTokenStore: streamTokenStore,
			mediaItemCache:   mediaItemCache,
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
			metricsSampler:             metricsSampler,
			streamMeter:                streamMeter,
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
			tokenGen:     infra.tokenGen,
			userTokenGen: guardedTokens,
			hasher:       infra.hasher,
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
		// Snapshot fast path: seek the nearest keyframe through the Cues (mkvgo,
		// bounded reads) and decode just that frame (ffmpeg), falling back to the
		// exact -ss extractor for non-Matroska / too-distant keyframes / errors.
		// DEFAULT ON with the standard opt-out (server_config
		// transcoding.snapshot_fastpath=off or EVEY_SNAPSHOT_FASTPATH=off, applied
		// on restart) so a bad mkvgo keyframe read can be ruled out from the admin
		// panel without a rebuild.
		frameExtractor:   snapshotFrameExtractor(ctx, r.ServerCfg, infra),
		copySeekProber:   infra.prober,
		onDemandPackager: infra.onDemandPackager,
		hlsAESSecret:     infra.hlsAESSecret,
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
			// Reconcile the media search index against the DB. The resilient wrapper
			// swallows writes while Meili is down, so an item created/edited during an
			// outage stays out of the index. ReconcileSearchIndex rebuilds when the
			// engine's doc count diverges from the DB count. Pass `sa` (the PRIMARY,
			// not the swallowing wrapper) so a still-down engine surfaces its error
			// and the drift isn't falsely cleared. Run at boot AND on a ticker so a
			// mid-session outage self-heals without a restart.
			reconcileSearch := func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("panic recovered in search reconcile", "error", r, "stack", string(debug.Stack()))
					}
				}()
				rctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
				defer cancel()
				if _, err := applibrary.ReconcileSearchIndex(rctx, infra.repos.MediaItemQuery, sa, sa, infra.repos.MediaItemTranslation); err != nil {
					slog.Warn("search reconcile failed", slog.Any("err", err))
				}
			}
			go reconcileSearch()
			go func() {
				ticker := time.NewTicker(15 * time.Minute)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						reconcileSearch()
					}
				}
			}()

			// Drain the durable search write-queue: replay writes the engine
			// rejected while it was down (recorded by the resilient wrapper into the
			// pending_search_index outbox). This is the PRIMARY drift-prevention path
			// — no write is lost — so the count-based reconcile above is left only for
			// the total-index-loss case it handles well. Runs often and cheap (the
			// queue is normally empty), so a recovered engine catches up within a
			// minute. Uses `sa` (the PRIMARY) so a still-down engine surfaces its error
			// and the ops stay queued instead of being dropped.
			drainSearch := func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("panic recovered in search write-queue drain", "error", r, "stack", string(debug.Stack()))
					}
				}()
				dctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
				defer cancel()
				if _, err := applibrary.DrainSearchWriteQueue(dctx, infra.repos.PendingSearchWrite, infra.repos.MediaItemQuery, sa, infra.repos.MediaItemTranslation); err != nil {
					slog.Warn("search write-queue drain failed (engine still down?)", slog.Any("err", err))
				}
			}
			go drainSearch()
			go func() {
				ticker := time.NewTicker(1 * time.Minute)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						drainSearch()
					}
				}
			}()
		}
	}

	// Reconcile the music search index against the DB — same drift as the media
	// index: the resilient wrapper swallows writes while Meili is down, so a track
	// added during an outage stays unsearchable. ReconcileTracks rebuilds when the
	// engine's doc count diverges from the DB track count (this also covers the
	// first-boot empty-index backfill: 0 docs vs N tracks is a divergence). Targets
	// the PRIMARY (msa) so a still-down engine surfaces its error. Boot + ticker so
	// a mid-session outage self-heals without a restart.
	if rm, ok := infra.musicSearch.(*search.ResilientMusicSearch); ok {
		if msa, isMeili := rm.Primary().(*meili.MusicSearchAdapter); isMeili {
			reconcileMusic := func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("panic recovered in music search reconcile", "error", r, "stack", string(debug.Stack()))
					}
				}()
				rctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
				defer cancel()
				if _, err := deps.media.musicSvc.CommandService().ReconcileTracks(rctx, msa, msa); err != nil {
					slog.Warn("music search reconcile failed", slog.Any("err", err))
				}
			}
			go reconcileMusic()
			go func() {
				ticker := time.NewTicker(15 * time.Minute)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						reconcileMusic()
					}
				}
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

// resolveFlag returns the effective value of an operational (non-secret) flag:
// a server_config DB row wins, else the process env var, else "". This lets an
// admin flip the toggle from /admin/server-config instead of editing the
// container env — a restart is still required, since these flags are read once
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
		slog.Info("snapshot: mkvgo keyframe fast path disabled (transcoding.snapshot_fastpath=off) — exact ffmpeg -ss extractor only")
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

// mediaItemCheckerAdapter adapts the library MediaItemRepository (typed id) to
// playlist.MediaItemExistsChecker (string id). It returns the item so AddItem
// can read its LibraryID for the library-access gate.
type mediaItemCheckerAdapter struct {
	repo library.MediaItemRepository
}

func (a mediaItemCheckerAdapter) GetByID(ctx context.Context, id string) (*library.MediaItem, error) {
	return a.repo.GetByID(ctx, shared.MediaItemID(id))
}

// playlistLibraryAccessAdapter implements playlist.PlaylistLibraryAccessChecker:
// it resolves the profile to its owning user, then consults library_access.
type playlistLibraryAccessAdapter struct {
	profiles domainuser.ProfileRepository
	access   domainuser.LibraryAccessRepository
}

func (a playlistLibraryAccessAdapter) HasAccessForProfile(ctx context.Context, profileID, libraryID string) (bool, error) {
	p, err := a.profiles.GetByID(ctx, shared.ProfileID(profileID))
	if err != nil {
		return false, err
	}
	return a.access.HasAccess(ctx, p.UserID, shared.LibraryID(libraryID))
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
