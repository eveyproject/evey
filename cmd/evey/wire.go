package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/email"
	"github.com/eveyproject/evey/internal/adapters/driven/ffmpeg"
	"github.com/eveyproject/evey/internal/adapters/driven/filesystem"
	"github.com/eveyproject/evey/internal/adapters/driven/introproviders"
	metadataadapter "github.com/eveyproject/evey/internal/adapters/driven/metadata"
	mkvgoadapter "github.com/eveyproject/evey/internal/adapters/driven/mkvgo"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	proberpkg "github.com/eveyproject/evey/internal/adapters/driven/prober"
	"github.com/eveyproject/evey/internal/adapters/driven/webhooksender"
	httpapi "github.com/eveyproject/evey/internal/adapters/driving/http"
	appactivity "github.com/eveyproject/evey/internal/app/activity"
	appcollection "github.com/eveyproject/evey/internal/app/collection"
	appconfig "github.com/eveyproject/evey/internal/app/config"
	appdemo "github.com/eveyproject/evey/internal/app/demo"
	appfavorite "github.com/eveyproject/evey/internal/app/favorite"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	apploadtest "github.com/eveyproject/evey/internal/app/loadtest"
	appmediarequest "github.com/eveyproject/evey/internal/app/mediarequest"
	appmetadata "github.com/eveyproject/evey/internal/app/metadata"
	appmkv "github.com/eveyproject/evey/internal/app/mkv"
	appphoto "github.com/eveyproject/evey/internal/app/photo"
	appplayback "github.com/eveyproject/evey/internal/app/playback"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	apppresence "github.com/eveyproject/evey/internal/app/presence"
	appqoe "github.com/eveyproject/evey/internal/app/qoe"
	appsubtitle "github.com/eveyproject/evey/internal/app/subtitle"
	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	appuser "github.com/eveyproject/evey/internal/app/user"
	appwebhook "github.com/eveyproject/evey/internal/app/webhook"
	"github.com/eveyproject/evey/internal/config"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/shared"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/eveyproject/evey/internal/domain/webhook"
	"github.com/eveyproject/evey/internal/pkg/bgtasks"
	"github.com/eveyproject/evey/internal/pkg/coalesce"
	"github.com/eveyproject/evey/internal/pkg/cpubudget"
	"github.com/eveyproject/evey/internal/pkg/demo"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	eveylogger "github.com/eveyproject/evey/internal/pkg/logger"
	eveymetrics "github.com/eveyproject/evey/internal/pkg/metrics"
	"github.com/eveyproject/evey/internal/pkg/netmeter"
	"github.com/eveyproject/evey/internal/pkg/procfork"
	"github.com/eveyproject/evey/internal/pkg/sysinfo"
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
// bus subscribers - they are consumed by the webhook "*" catch-all only.

// buildAppServices creates all application-level services from infra dependencies.
func buildAppServices(ctx context.Context, cfg *config.Config, db *sql.DB, infra *infraDeps, rootLog *slog.Logger) (*appDeps, error) {
	r := infra.repos
	bus := infra.bus

	// The background CPU budget: one share (25 % by default, 50 % on the
	// admin's say-so) every background task draws from, and a share of its
	// own for the heavy tasks the admin singles out. Live: a change under
	// Server settings applies to the next item. Every Scan-class fork
	// reserves its threads in it (procfork), the scan and the intro
	// detection size their fan-out on it, the trickplay its decoder threads.
	cpuBudget := cpubudget.New(sysinfo.CPUCapacity())
	appconfig.ApplyCPUBudget(ctx, r.ServerCfg, bus, cpuBudget)
	// Kids opening hours are read in the server's zone, swapped live when an
	// admin changes it.
	serverClock := appconfig.NewServerClock(nil)
	appconfig.ApplyServerTimezone(ctx, r.ServerCfg, bus, serverClock)
	profileHours := appuser.NewProfileSchedules(r.Profile, infra.hasher, serverClock)
	procfork.SetCPUBudget(cpuBudget)
	slog.Info("cpu budget: background tasks bounded",
		slog.Float64("capacity_cores", cpuBudget.Capacity()),
		slog.Int("global_percent", cpuBudget.Policy().GlobalPercent),
		slog.Int("ceiling_cores", cpuBudget.Ceiling()))

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
	// overrides take effect immediately) and live on every plugins.* change -
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
	// is always on - there is no scenario where disabling it helps (the fallback is
	// the safety net), hence no toggle/env. nil gates == always on.
	scanProber := proberpkg.New(infra.prober, nil, nil)

	libCmdSvc := applibrary.NewCommandService(
		r.Library, r.Library, r.MediaItem,
		r.MediaStream, r.Person, r.MediaPerson,
		infra.scanner, scanProber, infra.searchAdapter, bus,
		r.Chapter, infra.mkvAdapter,
		r.Trickplay, infra.trickplayGen, cfg.Transcoding.TrickplayCacheDir, r.ServerCfg,
		applibrary.WithShutdownCtx(ctx),
		applibrary.WithSearchReindex(r.MediaItemQuery, r.MediaItemTranslation, r.MediaPerson),
		applibrary.WithCPUBudget(cpuBudget),
		applibrary.WithFileSettle(applibrary.DefaultFileSettle),
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
		applibrary.WithServerConfig(r.ServerCfg),
	)

	schedulerLog := eveylogger.NewWith(rootLog, "scheduler")
	libScheduler := applibrary.NewScheduler(r.ScanSchedule, libCmdSvc, schedulerLog)

	// SMTP settings in force: the Admin -> Server Config card wins over
	// EVEY_SMTP_*, re-read on every smtp.* change so password recovery, the
	// SMTPConfigured flag and the emailers follow the panel without a restart.
	// On the demo it is disabled whatever the panel or env say: every send is
	// a no-op and password recovery reports itself off.
	liveSMTP := appconfig.NewLiveSMTP(ctx, r.ServerCfg, appconfig.SMTPSettings{
		Host: cfg.SMTP.Host, Port: cfg.SMTP.Port, Username: cfg.SMTP.Username,
		Password: cfg.SMTP.Password, From: cfg.SMTP.From, TLS: cfg.SMTP.TLS,
	}, cfg.Server.Demo || cfg.Server.BenchInstance)
	liveSMTP.Watch(bus)
	// guardedTokens enforces users.token_epoch on every access-token mint and
	// validation: bumping the epoch (disable, role change, password change,
	// admin session kill) revokes outstanding tokens within ~30s instead of
	// the full 15min expiry. Every consumer below gets the guard; the concrete
	// JWTGenerator stays only where plugin-token methods are needed.
	guardedTokens, stopEpochCache := appuser.NewEpochGuardedTokens(infra.tokenGen, r.User)
	_ = stopEpochCache // cache janitor lives for the process lifetime
	// On a security event (ban / password reset) the epoch bump kills JWTs
	// within ~30s, but opaque STREAM tokens are not epoch-guarded and a
	// cast/native token lives up to 12h. Wrap the epoch revoker so the same
	// revoke path also purges the user's stream tokens. purgeStreams is bound
	// after streamTokenStore is constructed (further down); it is only ever
	// invoked at runtime on a real security event, long after wiring completes.
	securityRevoker := &compositeTokenRevoker{epoch: guardedTokens}
	userAuthOpts := []appuser.CommandServiceOption{
		appuser.WithPasswordReset(r.PasswordResetToken, r.ServerCfg, liveSMTP.Configured()),
		appuser.WithLiveSMTPConfigured(liveSMTP.Configured),
		appuser.WithUnitOfWork(postgres.NewTxUnit(db)),
		appuser.WithLoginAttemptRepository(r.LoginAttempt),
		appuser.CommandWithTokenRevoker(securityRevoker),
	}
	if cfg.Server.Demo && !cfg.Server.DemoExtended {
		// The published demo password is shorter than the production floor;
		// the floor is a parameter of the service, chosen here, not a mode it
		// knows about.
		userAuthOpts = append(userAuthOpts, appuser.WithMinPasswordLength(demo.MinPasswordLength))
	}
	if cfg.Server.DemoExtended {
		userAuthOpts = append(userAuthOpts, appuser.CommandWithLoginLockExempt(cfg.Server.DemoUser))
	}
	userAuthSvc := appuser.NewCommandService(
		r.User, r.Profile, r.RefreshToken,
		r.Role, r.UserRole,
		infra.hasher, guardedTokens, bus,
		cfg.JWT.RefreshTokenTTL, r.QuickConnect,
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
	userSvcOpts := []appuser.UserServiceOption{appuser.WithDefaultLocale(serverLocaleFn), appuser.WithTokenRevoker(securityRevoker)}
	if cfg.Server.Demo && !cfg.Server.DemoExtended {
		// Same floor as the setup path: the bootstrap creates the published
		// `demo` administrator through CreateUser when an administrator exists.
		userSvcOpts = append(userSvcOpts, appuser.UserWithMinPasswordLength(demo.MinPasswordLength))
	}
	userSvc := appuser.NewUserService(r.User, r.Profile, r.RefreshToken, infra.hasher, bus, postgres.NewTxUnit(db), userSvcOpts...)
	userAccessSvc := appuser.NewAccessService(r.User, r.APIKey, r.InviteLink, r.QuickConnect, infra.hasher, bus, appuser.WithProfiles(r.Profile), appuser.WithLibraryAccess(r.LibraryAccess), appuser.WithAccessDefaultLocale(serverLocaleFn), appuser.WithAccessUnitOfWork(postgres.NewTxUnit(db)))
	userQuerySvc := appuser.NewQueryService(r.User, r.Profile, r.APIKey, r.InviteLink, r.QuickConnect)

	// SSO is always wired: the admin panel can switch it on at runtime, and
	// discovery is lazy so an unreachable provider never blocks the boot.
	oidcSettings := appuser.OIDCSettingsSource{
		Env: appuser.OIDCConfig{
			Enabled:             cfg.OIDC.Enabled,
			Issuer:              cfg.OIDC.Issuer,
			ClientID:            cfg.OIDC.ClientID,
			ClientSecret:        cfg.OIDC.ClientSecret,
			RedirectURL:         cfg.OIDC.RedirectURL,
			AllowedEmailDomains: cfg.OIDC.AllowedEmailDomains,
		},
		EnvAllowSignup: cfg.OIDC.AllowSignup,
		Store:          r.ServerCfg,
		BaseURL:        cfg.EffectiveBaseURL,
	}
	oidcSvc, oidcErr := appuser.NewOIDCService(
		oidcSettings.Load,
		r.User, r.Profile, r.RefreshToken, r.UserRole,
		guardedTokens, bus, postgres.NewTxUnit(db), cfg.JWT.RefreshTokenTTL,
		appuser.WithOIDCLibraryAccess(r.LibraryAccess),
	)
	if oidcErr != nil {
		return nil, fmt.Errorf("oidc init: %w", oidcErr)
	}
	bus.Subscribe(shared.ServerConfigChanged{}.EventName(), func(_ context.Context, evt shared.Event) {
		if c, ok := evt.(shared.ServerConfigChanged); ok && appuser.OIDCConfigKeys[c.Key] {
			oidcSvc.Invalidate()
		}
	})

	// Buffer player-tick progress writes so the (~5s) hot path skips DB I/O.
	// Periodic flusher upserts the latest state per (profile, media). On
	// crash up to one flush interval of progress is lost - acceptable for a
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
		appplayback.WithSessionRetention(cfg.EffectiveSessionRetentionDays),
	)
	// Cast progress: once the sender's browser closes, the receiver's fetches
	// on its cast token are the only playback signal left; the HTTP serving
	// handlers feed this tracker, which lands derived positions through the
	// same ReportProgress path the clients use (90% auto-complete included).
	castProgress := appplayback.NewCastProgressTracker(
		func(ctx context.Context, profileID, mediaItemID string, positionMs int64) error {
			return playbackSvc.CommandService().ReportProgress(ctx, profileID, mediaItemID, positionMs, false)
		})
	castProgress.Start(ctx)

	transcodingSvc := apptranscoding.NewService(
		infra.transcoder, infra.hwDetector, r.TranscodeJob, r.TranscodeProfile,
		bus, cfg.Transcoding.MaxConcurrent,
		apptranscoding.WithCacheStore(filesystem.NewCacheStore(cfg.Transcoding.CacheDir)),
		// The subtitle store under /data goes with the cache wipe: it was the one
		// derived store the button left behind.
		apptranscoding.WithDerivedCachePurge(httpapi.SubtitleCachePurger("/data")),
	)
	// The cap the service will actually enforce. Logged because it is taken by
	// VALUE here: main.go must have hydrated cfg from the DB overrides BEFORE
	// buildServices, or the operator's saved setting is silently ignored.
	slog.Info("transcoding: max concurrent jobs", slog.Int("max", transcodingSvc.MaxConcurrent()))

	mkvSvc := appmkv.NewService(infra.mkvAdapter)
	collectionCmdSvc := appcollection.NewCommandService(r.Collection, r.Collection, r.MediaItem)
	collectionQuerySvc := appcollection.NewQueryService(r.Collection, postgres.NewCollectionSmartQuerier(db))

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
		r.Artwork, r.Person, r.MediaPerson, r.MediaItemQuery, r.MediaItem, infra.searchAdapter,
		tmdbWrapped,
		[]metadata.MetadataProvider{omdbWrapped, tvmazeWrapped, imdbWrapped},
		collectionCmdSvc,
		collectionQuerySvc,
		appmetadata.EnricherWithTranslations(r.MediaItemTranslation, r.PersonTranslation, eveyi18n.SupportedLocales()),
		appmetadata.EnricherWithPersonFetcher(infra.personFetcher),
		appmetadata.EnricherWithOverrides(r.MetadataOverride),
		// Item update + artwork replace + cast upsert commit or roll back
		// together - a mid-enrichment failure no longer strands the item with a
		// fresh title but no artwork/cast (unrepairable except by a manual
		// "Refresh metadata", since the re-scan trigger keys on empty Overview).
		appmetadata.EnricherWithUnitOfWork(postgres.NewTxUnit(db)),
	)
	metadataCmdSvc := appmetadata.NewCommandService(r.MetadataOverride, r.Artwork, infra.artworkDL, bus, metaProviders,
		appmetadata.WithEnricher(enricher, r.MediaItemQuery, r.MediaItem),
		appmetadata.WithMediaPersons(r.MediaPerson),
		appmetadata.WithShutdownCtx(ctx),
		appmetadata.WithServerConfig(r.ServerCfg),
	)
	// Fire the artwork auto-download the moment the enricher persists a new record
	// (no-op unless the admin toggle artwork.auto_download is on, checked inside
	// maybeAutoDownload). Set here rather than as a NewEnricher option because
	// metadataCmdSvc is built WITH the enricher - the hook back-reference only
	// exists once both do.
	enricher.SetArtworkPostSaveHook(metadataCmdSvc.ArtworkAutoDownloadHook())

	activityCmdSvc := appactivity.NewCommandService(r.LoginHistory, r.ServerMetric, r.AuditLog, bus)
	activityQuerySvc := appactivity.NewQueryService(r.LoginHistory, r.ServerMetric, r.AuditLog, r.PlaySessionStats, r.PlayHistory,
		appactivity.WithAnalyticsReader(postgres.NewAnalyticsReader(db)),
		appactivity.WithServerConfig(r.ServerCfg),
	)
	// Counts the bytes the streaming routes actually serve; the sampler turns
	// that into the server_metrics history (dashboard rate + consumed total).
	streamMeter := &netmeter.Meter{}
	// Its own windows, not the dashboard's cached ones: each history row
	// covers the whole minute since the previous row.
	historySys := sysinfo.NewSampler()
	metricsSampler := appactivity.NewMetricsSampler(activityCmdSvc, r.PlaySessionStats, streamMeter,
		func() appactivity.SysSample {
			s := historySys.Collect()
			return appactivity.SysSample{
				CPUPercent: s.CPUPercent, MemUsedBytes: s.MemUsedBytes, MemTotalBytes: s.MemTotalBytes,
				IOWaitPercent: s.IOWaitPercent, Disks: s.Disks, Net: s.Net,
			}
		}, rootLog).
		// The library disks' usage, grouped per physical disk like the
		// dashboard's storage cards, so the history shows how fast they fill.
		WithStorage(func() (used, total int64) {
			libs, err := libQuerySvc.ListLibraries(ctx)
			if err != nil {
				return 0, 0
			}
			var paths []string
			for _, lib := range libs {
				paths = append(paths, lib.Paths...)
			}
			for _, st := range sysinfo.GetStorageStats(paths) {
				used += st.UsedBytes
				total += st.TotalBytes
			}
			return used, total
		}).
		// Honour EVEY_METRICS_SAMPLE_INTERVAL_SEC: it was parsed into config and
		// then dropped on the floor, so an operator changing it saw no effect.
		WithSampleInterval(time.Duration(cfg.Metrics.SampleIntervalSec) * time.Second)

	pluginInstallerSvc, pluginRuntimeSvc, pluginQuerySvc, pluginTokenSvc, pluginSecurityDashboardSvc, pluginIdleMgr, pluginHostSrv, memProfile := buildPluginSubsystem(ctx, infra, rootLog)

	webhookLog := eveylogger.NewWith(rootLog, "webhook")
	var webhookSender webhook.Sender = webhooksender.New()
	if cfg.Server.Demo || cfg.Server.BenchInstance {
		// The stored webhooks keep firing on events, into a sink: the delivery
		// log still shows the product, the host makes no outbound call. A bench
		// instance runs on a copy of a production database: its webhooks must
		// not fire from a load test.
		webhookSender = webhooksender.Discard{}
	}
	webhookSvc := appwebhook.NewService(r.Webhook, r.WebhookDelivery, webhookSender, webhookLog, bus)
	// The delivery log is append-only and purely diagnostic; without a sweep it
	// grew for the lifetime of the deployment. Rides the same daily retention
	// loop as audit_log and the QoE events.
	activityCmdSvc.WithWebhookDeliveryPurger(r.WebhookDelivery)

	notifCmdSvc, notifQuerySvc := buildNotificationServices(ctx, cfg, infra, rootLog)

	// Always registered: the sender reads liveSMTP at send time and is a no-op
	// while no host is set, so SMTP configured later starts sending at once.
	smtpSender := email.NewLiveSMTPSender(liveSMTP, cfg.EffectiveServerName)
	email.NewNotificationEmailer(smtpSender, r.Profile, r.User, cfg.EffectiveServerName,
		email.WithNotificationLocale(infra.translator, serverLocaleFn), email.WithNotificationBaseURL(cfg.EffectiveBaseURL)).Register(bus)
	email.NewPasswordResetEmailer(smtpSender, cfg.EffectiveBaseURL, cfg.EffectiveServerName).Register(bus)
	if liveSMTP.Configured() {
		rootLog.Info("SMTP email notifications enabled", slog.String("host", liveSMTP.Current().Host))
	}
	// userAuthSvc already wired with password reset + UoW options above.

	feedCmdSvc, feedQuerySvc := buildFeedServices(ctx, infra)

	favoriteSvc := appfavorite.NewService(r.Favorite)

	// Media requests search movie/tv via the shared metadata providers and
	// music via MusicBrainz (decorated with the same retry+cache wrapper).
	mediaRequestProviders := append([]metadata.MetadataProvider{}, metaProviders...)
	mediaRequestProviders = append(mediaRequestProviders, decorate(metadataadapter.NewMusicBrainzProvider()))
	mediaRequestSvc := appmediarequest.NewService(r.MediaRequest, r.MediaRequestAccess, mediaRequestProviders, bus).
		WithLibraryLookup(postgres.NewLibraryLookupRepository(db))
	photoScanner := filesystem.NewPhotoScanner()
	photoSvc := appphoto.NewService(r.Photo, bus, r.Library, photoScanner)
	musicSvc, musicNowPlaying := buildMusicServices(ctx, cfg, infra)
	libCmdSvc.WithMusicScanner(musicSvc)
	libCmdSvc.WithPhotoScanner(photoSvc)
	playlistSvc, playlistGenSvc := buildPlaylistServices(infra)

	subtitleSvc := buildSubtitleService(ctx, infra, libQuerySvc, rootLog)
	// Rows downloaded before the key was removed are still served: cleaned up
	// whether or not OpenSubtitles is configured now.
	appsubtitle.SubscribeReplacedFileCleanup(bus, r.Subtitle)

	serverCfgSvc := appconfig.NewService(r.ServerCfg, appconfig.WithEventBus(bus), appconfig.WithUnitOfWork(postgres.NewTxUnit(db)))

	backupSvc, backupScheduler := buildBackupServices(cfg, infra, rootLog)

	syncPlayHub, deviceRegistry, streamTokenStore, shareSvc := buildSyncPlayServices(db)
	// Bind the stream-token purge into the security revoker now that the store
	// exists: a ban/password-reset cuts the user's live cast/native streams.
	securityRevoker.purgeStreams = streamTokenStore.RevokeByUser

	hlsTranscoder := ffmpeg.NewHLSTranscoder()
	hlsVODCache, err := apptranscoding.NewSegmentCache(cfg.Transcoding.CacheDir)
	if err != nil {
		return nil, fmt.Errorf("hls vod cache: %w", err)
	}
	// P3 ceiling-hit signal store: the serve path feeds it, the probe worker
	// drains it. In-memory, transient, always on.
	probeSignals := apptranscoding.NewProbeSignalStore(0)
	// The fast-start MP4 remuxes (<CacheDir>/remux) and the compat downloads
	// (<CacheDir>/compat) count in the SAME budget as the segments: the sweeper
	// is started below, once the playback counter it backs off on exists.
	// Source-fingerprint keying orphans the old file on every re-scan, so
	// without the sweep both grow without bound.
	// The remuxes a player is reading are leased by the /remux.mp4 handler:
	// the budget spares them in every pass.
	remuxLeases := apptranscoding.NewFileLeases()
	remuxJanitor := apptranscoding.NewRemuxCacheJanitor(filepath.Join(cfg.Transcoding.CacheDir, "remux")).WithLeases(remuxLeases)
	compatJanitor := apptranscoding.NewRemuxCacheJanitor(filepath.Join(cfg.Transcoding.CacheDir, "compat"))
	// Inline SourceResolver: the orchestrator hands a DecisionFingerprint
	// back, we look up the media item via the library query service. Streams
	// are filled in later (Phase 3 follow-up) - Phase 1+2 only need the path.
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
	// ffmpeg -progress recording: OFF unless an operator turns it on with
	// `transcode.ffmpeg_progress_log`. Read per SESSION (not captured once
	// here) so flipping the key applies to the next encoder rather than the
	// next restart - the same live-read contract as transcode.audio_normalize.
	//
	// It is the only record of what ffmpeg ACHIEVED over time; everything else
	// this server logs is what it DECIDED. The argv, the spawn, the kill and
	// the reach verdict all describe intent, and on 2026-09-06 that left the
	// question "was the encoder actually too far behind when the client hung
	// up?" answerable only by deduction. One file per encoder is real disk and
	// real writes on a weak NAS, hence off by default and worth every byte when
	// an operator is actually chasing something.
	ffmpegProgressLogDir := func(ctx context.Context) string {
		entry, err := r.ServerCfg.Get(ctx, "transcode.ffmpeg_progress_log")
		if err != nil || entry.Value != "true" {
			return ""
		}
		return filepath.Join(cfg.Log.Dir, "ffmpeg")
	}

	hlsVODOrch := apptranscoding.NewSegmentOrchestrator(hlsVODCache, hlsTranscoder, sourceResolver, apptranscoding.OrchestratorOptions{
		ThrottleEnabled:       true,
		MaxActiveSessions:     maxHLSSessions,
		HWFirstSegmentTimeout: cfg.Transcoding.HWFirstSegmentTimeout,
		HWPerVariant:          cfg.Transcoding.HWPerVariant,
		SingleVariant:         cfg.Transcoding.SingleVariant,
		FFmpegProgressLogDir:  ffmpegProgressLogDir,
		// Debounce rapid seeks so a burst on a heavy rung spawns one encoder at
		// the final position, not one per seek that the next seek kills before it
		// produces (prod 2026-09-15). ~150 ms is under a fragment RTT: a single
		// seek is unaffected in practice, a rafale of seeks collapses to one spawn.
		SeekCoalesceWindow: 150 * time.Millisecond,
	}, slog.Default())

	// Software producer lifecycle knobs. The low-memory profile trims them hard:
	// a short inactivity timeout, a single session/rung per hash, and
	// KillPausedAfter reclaims a long-suspended encoder's ~1 GB RSS - a weak NAS
	// cannot afford to keep a paused ffmpeg resident on the off-chance a viewer
	// resumes. ResumeCushionSegs is what keeps that kill latency-invisible: the
	// player drains the disk cache instantly while a fresh -ss ffmpeg cold-starts
	// behind it. Unlike the plugin idle manager (re-evaluated live via
	// pluginIdleMgr.SetEnabled above), these knobs are read once here at boot -
	// the software producer's GC loop is not re-wired on a live profile change,
	// so switching the memory profile only takes effect for HLS after a restart.
	// KillPausedAfter is armed in EVERY profile since admission slots exist: a
	// SIGSTOPped abandoned encoder consumes no CPU but HOLDS its video
	// admission slot, and the normal profile's 30min inactivity reap meant up
	// to half an hour of "server full" after a viewer rush (seen live after a
	// load test: two paused ffmpeg pinned 2 slots for 13+ minutes). The resume
	// cushion keeps the kill latency-invisible for a genuine returnee; low
	// tightens it further for RAM, not just slots.
	swProducerOpts := apptranscoding.SoftwareProducerOptions{
		KillPausedAfter:   5 * time.Minute,
		ResumeCushionSegs: 8,
	}
	if memProfile == appplugin.MemoryProfileLow {
		swProducerOpts = apptranscoding.SoftwareProducerOptions{
			InactivityTimeout:            5 * time.Minute,
			MaxSessionsPerHash:           1,
			MaxConcurrentVariantsPerHash: 1,
			KillPausedAfter:              3 * time.Minute,
			ResumeCushionSegs:            8,
		}
		rootLog.Info("low-memory profile: software HLS producer knobs armed (requires a restart to take effect)",
			slog.Duration("inactivity_timeout", swProducerOpts.InactivityTimeout),
			slog.Int("max_sessions_per_hash", swProducerOpts.MaxSessionsPerHash),
			slog.Int("max_concurrent_variants_per_hash", swProducerOpts.MaxConcurrentVariantsPerHash),
			slog.Duration("kill_paused_after", swProducerOpts.KillPausedAfter),
			slog.Int("resume_cushion_segs", swProducerOpts.ResumeCushionSegs))
	}
	// Set AFTER the profile branch on purpose: the low-memory path REPLACES the
	// whole struct, so anything assigned before it would be silently dropped on
	// exactly the boxes most likely to need the diagnostic.
	swProducerOpts.FFmpegProgressLogDir = ffmpegProgressLogDir

	// The software StreamProducer is built HERE (not in the HTTP route
	// registration) so it is a SINGLE instance shared between two consumers: the
	// HTTP VOD handler's routing selector (which creates its sessions) and the
	// gRPC StopStream / admin-kill path (which must be able to tear them down).
	// Without the shared instance, a stopped software stream would linger until
	// the producer's idle GC. Same sourceResolver as the orchestrator → SW and
	// HW resolve the source identically.
	hlsVODSoftware := apptranscoding.NewSoftwareProducer(
		hlsVODCache, hlsTranscoder, sourceResolver,
		swProducerOpts, slog.Default(),
	)
	// Fan-out remover for explicit StopStream / admin force-kill: RemoveSession
	// reaches BOTH producers, and ActiveSessionCount sums them. For()/routing is
	// unused on this instance (the HTTP handler owns the routing selector with
	// the real resolveHW predicate), so the predicate is nil.
	// Global bound on CONCURRENT VIDEO-ENCODE sessions. Shared by both producers
	// (software + hardware orchestrator); the copy rung, which merely remuxes
	// video and may transcode audio, runs on its own SoftwareProducer instance
	// and never consumes a slot. Starts unlimited (0) and is armed below, once
	// the benchmark has measured what this box can actually sustain.
	videoLimiter := apptranscoding.NewVideoSessionLimiter(0)
	hlsVODSoftware.WithVideoSessionLimiter(videoLimiter)
	hlsVODOrch.WithVideoSessionLimiter(videoLimiter)
	videoShaper := apptranscoding.NewVideoAdmissionShaper(videoLimiter)

	// Background transcodes yield to live playback: while a viewer's transcode is
	// encoding (videoLimiter.Active() > 0), running background jobs are SIGSTOPped
	// and resumed once playback goes idle. The live capacity is measured on an
	// idle box, so background work must step aside rather than eat into it. On by
	// default; server_config transcoding.pause_bg_on_playback=off disables it.
	if resolveFlag(ctx, r.ServerCfg, "transcoding.pause_bg_on_playback", "EVEY_TRANSCODING_PAUSE_BG_ON_PLAYBACK") != "off" {
		transcodingSvc.SetPlaybackGauge(videoLimiter)
		go transcodingSvc.SuperviseBackgroundPreemption(ctx)
	}

	// On-demand playback tracker: the mkvgo rung serves a whole playback without
	// spawning a single ffmpeg process, so both producer counters read 0 for it.
	// Folding it into the selector is what makes an mkvgo viewer visible to
	// EVERY consumer of ActiveSessionCount at once (drain suspension, trickplay,
	// intro/complexity SIGSTOP, level backfill, calibration and load-test gates)
	// - they all read that one method, so wiring it anywhere else would have
	// fixed some and left the others blind.
	playbackActivity := apptranscoding.NewPlaybackActivity(apptranscoding.PlaybackActivityTTL)
	hlsVODRemover := apptranscoding.NewProducerSelector(hlsVODSoftware, hlsVODOrch).
		WithPlaybackActivity(playbackActivity)

	// The one sweep of the transcode cache: segments, remuxes and compat
	// downloads under the admin's budget (10 GB by default, an absolute
	// number - the operator decides what the rest of the volume is for), a
	// 2 GiB free-space floor whatever the budget, and a week's life for a
	// download nobody fetches. Cleanup token dropped: the sweeper runs for
	// the process lifetime.
	//
	// Started HERE, after the selector exists, rather than next to the cache
	// construction: the sweep walks the whole cache root (one lstat per file)
	// on EVERY tick whatever the fill level, so it must be able to back off
	// while someone is watching. On a weak NAS the cache shares a spindle with
	// the media being read.
	// Free-space floor shared by the background sweeper (EvictionPolicy) and the
	// on-demand AdmitWrite guard on the serve path, so both agree on "full".
	// EVEY_TRANSCODE_CACHE_MIN_FREE_MB overrides the 2 GiB default (a small box
	// or the ops harness sets it low); 0 disables the floor and the guard.
	cacheMinFree := int64(apptranscoding.DefaultMinFreeBytes)
	if v := os.Getenv("EVEY_TRANSCODE_CACHE_MIN_FREE_MB"); v != "" {
		if mb, err := strconv.Atoi(v); err == nil && mb >= 0 {
			cacheMinFree = int64(mb) << 20
		}
	}
	hlsVODCache.SetWriteFloor(cacheMinFree)
	// Same floor for the subtitle store under /data: nothing else bounds it.
	// A refused store pages the admins (once an hour): the bytes are still
	// served, but every subtitle is now extracted again on each request.
	httpapi.SetSubtitleCacheWriteFloor(cacheMinFree)
	httpapi.OnSubtitleCacheFloorHit(func(dir string, free, floor int64) {
		ev := domaintranscoding.DerivedCacheFull{Store: "subtitles", Dir: dir, FreeBytes: free, FloorBytes: floor, OccurredAt: time.Now()}
		if err := bus.Publish(ctx, ev); err != nil {
			slog.Warn("subtitle cache floor: publish failed", slog.Any("err", err))
		}
	})
	_ = apptranscoding.NewCacheBudget(hlsVODCache, remuxJanitor, compatJanitor).Start(apptranscoding.EvictionPolicy{
		MaxBytes:       int64(cfg.Transcoding.CacheMaxGB) * 1024 * 1024 * 1024,
		MinFreeBytes:   cacheMinFree,
		ActivePlayback: hlsVODRemover.ActivePlaybackCount,
	}, slog.Default())

	// mkvgo Playability shadow (log-only): compares the playback decision against
	// ops.Playability once per (title, client, hour) unless
	// transcoding.playability_shadow=off. Divergences are throttled WARNs - the
	// decision engine itself is never influenced.
	apptranscoding.SetPlayabilityShadow(apptranscoding.NewPlayabilityShadow(infra.mkvAdapter, r.ServerCfg, slog.Default()))

	// Scan-time serving-plan precomputation (Phase 2, mkvgo ops.Ingest),
	// gated by library.ingest_serving_plan (off | plan | plan+repair).
	libCmdSvc.WithIngestPlanning(r.ServerCfg, infra.mkvAdapter, infra.mkvAdapter)
	libCmdSvc.WithIngestActivePlayback(hlsVODRemover.ActivePlaybackCount)
	// A version re-probe leaves alone the files a play session is open on.
	libCmdSvc.WithPlayingMedia(func(ctx context.Context) (map[shared.MediaItemID]bool, error) {
		active, err := r.PlaySession.ListActiveSessions(ctx)
		if err != nil {
			return nil, err
		}
		playing := make(map[shared.MediaItemID]bool, len(active))
		for _, a := range active {
			playing[a.MediaItemID] = true
		}
		return playing, nil
	})
	// Trickplay generation throttles while any stream is live - feed it the
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
	// to pick the appropriate ABR ladder. Repo nil-safe across the path -
	// unclassified items keep getting LadderFilmNormal.
	complexityRepo := postgres.NewMediaComplexityRepo(db)
	complexityClassifier := ffmpeg.NewComplexityClassifier("ffmpeg", "ffprobe")

	// Encode capability calibration: bench libx264 + any HW backend
	// against a fixed ladder so the HLS-VOD handler can cap the ladder
	// at what THIS host actually sustains in realtime. Without this,
	// the 4K rung on Raspberry Pi would lock the multi-variant ffmpeg
	// pipeline below realtime and seg 0 would never land in time.
	//
	// Bench is blocking on first boot (~20 s) - the safe 720p fallback
	// in the service constructor keeps requests served until it returns.
	// Persisted across restarts via server_settings + CPU signature
	// invalidation, so the 20 s only hits once per host lifetime.
	capabilityRepo := apptranscoding.NewCapabilityRepoAdapter(r.ServerCfg)
	// The HW-acceleration mode is admin-selectable (auto/off/nvenc/vaapi/qsv).
	// Bootstrap precedence: an explicit transcoding.hw_mode (persisted admin
	// override, else env) wins; otherwise derive it from the legacy
	// HWAccelEnabled + HWBackend knobs. We read the persisted overrides DIRECTLY
	// here because the calibration service is built before main's loadOverride
	// pass - without this, an admin's saved mode would be ignored until they
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
	// AV1 software ceiling is admin-settable (server_settings), so - like hw_mode
	// above - read the persisted override DIRECTLY from the store here: main's
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
		apptranscoding.WithHWDevice(cfg.Transcoding.HWDevice),
		apptranscoding.WithSoftwareCapOverrides(
			cfg.Transcoding.SoftwareMaxHeightOverride,
			cfg.Transcoding.SoftwareHDRMaxHeightOverride),
		apptranscoding.WithSoftwareAV1MaxOverride(swAV1Override),
		// Host-wide load, not Evey's: a first boot during a library import
		// measured a shared box and armed a capacity of one stream on a
		// 25-stream host. The bench keeps its result but marks it provisional,
		// and the tick below re-benches it once the host is idle.
		apptranscoding.WithHostLoadProbe(func() float64 { return sysinfo.CPUBusyPercent(time.Second) }))
	// Idle-gate the admin "recalibrate" button on the SAME live-session counter
	// trickplay uses: a manual recalibration run while transcodes are active
	// measures contended throughput and would pin the cap a rung too low. First-boot
	// EnsureCalibrated below is unaffected (it bypasses Recalibrate; no streams at boot).
	calibrationSvc.SetActiveStreamChecker(hlsVODRemover.ActivePlaybackCount)
	// Yield ALL background analysis ffmpeg (intro detection, complexity sampling,
	// chromaprint) to real work: each SIGSTOPs itself while a playback/transcode
	// session is live OR a recalibration is running, and resumes when the box is
	// idle - CPU/memory-bandwidth/I/O go to what matters, the decode keeps its
	// progress (no re-run).
	isBusy := func() bool {
		return hlsVODRemover.ActivePlaybackCount() > 0 || calibrationSvc.IsBenchmarking()
	}
	if infra.introDetector != nil {
		infra.introDetector.WithBusyChecker(isBusy)
	}
	complexityClassifier.WithBusyChecker(isBusy)
	infra.trickplayGen.WithBusyChecker(isBusy)
	// Boot never benchmarks (a bench here blocked the whole server for 2-5 min
	// and measured the migrations/scans booting alongside it): this publishes
	// what is persisted and flags a pending bench, run by the opportune-moment
	// loop below once the host has been idle for a whole probe window.
	if err := calibrationSvc.EnsureCalibrated(ctx); err != nil {
		slog.Warn("calibration: EnsureCalibrated returned error, falling back to 720p", slog.Any("err", err))
	}

	// Arm the video-session cap from the benchmark: the tallest software rung
	// that sustained realtime tells us how much headroom one encode leaves, so
	// FPS/RequiredFPS is how many realtime sessions this box can serve. The
	// admin may only REDUCE it (transcoding.max_video_sessions).
	applyVideoSessionLimit := func(ctx context.Context) {
		current := calibrationSvc.Current()
		samples := current.BenchSamples
		// The decode half of the calibration, when the host has re-benched since
		// it started being measured: it lets a session be priced by the source it
		// actually decodes rather than by its output rung alone. Absent (older
		// persisted capability, synthetic-sample fallback) the model prices
		// exactly as it always did.
		costs := apptranscoding.NewVideoCostModel(samples).
			WithDecodeEvidence(current.BenchSource, current.DecodeSamples)
		videoLimiter.WithCostModel(costs)
		// The per-route cost model: what a warm, route-faithful single session of
		// each route actually cost (RouteSamples). Consulted BEFORE the table for
		// a route it measured, pricing both the accelerator AND the CPU pool a GPU
		// route really spends; the table stays the fallback. nil (a capability
		// benched before route evidence existed) leaves table pricing unchanged.
		videoLimiter.WithRouteModel(apptranscoding.NewRouteCostModel(current.RouteSamples, sysinfo.CPUCapacity()))

		// Admission is governed by PER-POOL cost budgets: every software flavor
		// (plain/HDR/AV1) spends the CPU pool, hw encodes spend the GPU pool, so
		// a NVENC box serves its GPU streams without starving software ones and
		// vice versa. The count cap stays a belt across BOTH pools, derived by
		// the limiter itself (AutoCountCap) so the admin panel reports the very
		// number enforced here. The admin may only lower it.
		maxStreams := videoLimiter.AutoCountCap()
		gpuStreams := videoLimiter.AutoCountCapFor(apptranscoding.PoolGPU)
		effective := apptranscoding.ResolveVideoSessionLimit(ctx, r.ServerCfg, maxStreams)
		videoLimiter.SetLimit(effective)

		auto, ev := apptranscoding.AutoVideoSessionCapacity(samples, sysinfo.CPUBudget())
		slog.Info("video transcode capacity armed",
			slog.Int("streams_at_top_rung", auto), slog.Int("count_cap", effective),
			slog.Int("gpu_streams_at_cheapest_rung", gpuStreams),
			slog.Float64("cpu_budget_pool", videoLimiter.BudgetFor(apptranscoding.PoolCPU)),
			slog.Float64("gpu_budget_pool", videoLimiter.BudgetFor(apptranscoding.PoolGPU)),
			slog.Int("rung_height", ev.Height), slog.Float64("fps", ev.FPS),
			slog.Float64("required_fps", ev.RequiredFPS), slog.Int("cpu_budget", ev.CPUBudget),
			slog.Int("route_samples", len(current.RouteSamples)),
			slog.Bool("derived_from_benchmark", ev.Derived))
	}
	applyVideoSessionLimit(ctx)
	// Re-arm on every capability change (bench completion, persisted load,
	// runtime HW demotion): admission prices bench samples, and pricing a
	// demoted GPU with yesterday's table was exactly the dead-snapshot failure
	// seen live (NVENC init failure -> nothing repriced).
	//
	// Through a coalesced single-goroutine runner, for two audited reasons:
	// OnCapabilityChange callbacks run SYNCHRONOUSLY under the calibration
	// service's demoteMu+currentMu, and applyVideoSessionLimit does a DB read
	// (ResolveVideoSessionLimit) - inline it serialized HW demotions on a
	// Postgres round-trip. And two concurrent triggers (demotion + config
	// change) interleaved read-compute-SetLimit, so a limit computed from the
	// PRE-demotion capability could apply last and win (lost update). The
	// runner makes triggers non-blocking and every run read AFTER the
	// previous one applied.
	reapplyVideoLimit := coalesce.Start(ctx, applyVideoSessionLimit)
	calibrationSvc.OnCapabilityChange(reapplyVideoLimit.Trigger)
	bus.Subscribe(shared.ServerConfigChanged{}.EventName(), func(_ context.Context, evt shared.Event) {
		if c, ok := evt.(shared.ServerConfigChanged); ok && c.Key == apptranscoding.MaxVideoSessionsConfigKey {
			reapplyVideoLimit.Trigger()
		}
	})
	// Level-2 HW safety net: a hardware-encode failure demotes the calibrated
	// capability so the client's retry re-resolves a lower tier (encoder-agnostic
	// cache ⇒ transparent). DemoteCapability returns whether it changed; the
	// orchestrator sink ignores the bool.
	// Per-title fallback first: a title whose GPU session failed plays on the
	// CPU at its next request, whatever the host verdict below turns out to
	// be. A new calibration (or a confirmed demotion) re-proves the routes and
	// forgets the blocks.
	hlsVODOrch.WithHWRouteBlocklist(apptranscoding.NewHWRouteBlocklist(0, 0))
	calibrationSvc.OnCapabilityChange(hlsVODOrch.ClearHWRouteBlocks)
	hlsVODOrch.SetHWFailureSink(func(ctx context.Context, report apptranscoding.HWFailureReport) {
		calibrationSvc.DemoteRoute(ctx, report)
	})
	// ...and the way back. Without this the demotion above is permanent: it is
	// persisted, so a GPU that failed once stays off until something else happens
	// to re-bench. Production lost ~27h of hardware acceleration for every user
	// that way, from a single 4K HEVC play. ReprobeDemotedHW is a no-op until the
	// demotion is older than HWDemotionTTL and only restores on a real encode
	// probe, so the tick is cheap and a genuinely dead GPU stays off.
	//
	// The same loop runs the opportune-moment bench: while a bench is due
	// (deferred at boot, or provisional after a bench on a busy host) the host
	// is probed every few seconds and benched once it has been idle for a whole
	// probe window (5 samples 4 s apart, no stream, CPU under 20 %); otherwise
	// the loop idles on the 5-minute clock.
	go func() {
		if benchInstance {
			return
		}
		reprobe := time.NewTicker(5 * time.Minute)
		defer reprobe.Stop()
		for {
			wait := 5 * time.Minute
			if calibrationSvc.NeedsBench() {
				wait = 4 * time.Second
			}
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-reprobe.C:
				timer.Stop()
				calibrationSvc.ReprobeDemotedHW(ctx)
			case <-timer.C:
				calibrationSvc.BenchWhenOpportune(ctx)
			}
		}
	}()

	capsOverrideSvc, keyframeOnDemand, kfProber, bgStoppers := startTranscodingWorkers(ctx, db, infra, libCmdSvc, libQuerySvc, complexityRepo, complexityClassifier, hlsVODRemover)

	// Built-in load test: synthetic viewers against this server's OWN stack,
	// through the loopback (real routing, auth, middleware - not a shortcut).
	// The gate reuses the same live-session counter as the calibration bench:
	// it never runs over real viewers.
	loadTestRunner, loadTestBaseURL, loadTestPublicURL := buildLoadTestRunner(cfg, libQuerySvc, hlsVODRemover)
	// A run counts as playback from its first second, the way a first real
	// viewer does, so background workers yield before the storage is measured.
	loadTestRunner.WithPlaybackSignal(func() func() { return playbackActivity.Begin("loadtest") })
	// The run clears the activity its own viewers raised, and drops the plans
	// its eligibility check built, when it is done with them.
	loadTestRunner.WithPlaybackForget(func(ids []string) { playbackActivity.Forget(ids...) })
	loadTestRunner.WithPlanDrop(infra.onDemandPackager.DropSources)
	// The run logs what it cost the packager in window reads, rebuilds apart.
	loadTestRunner.WithWindowStats(infra.onDemandPackager.WindowStats)
	// The bytes the packager read from its sources, counted in-process: the
	// "app read" figure /proc/self/io would give if the server could read it
	// (it is non-dumpable).
	loadTestRunner.WithAppReadCounter(func() int64 { return infra.onDemandPackager.WindowStats()["source_read_bytes"] })
	// A "clients:history" run plays the mix of the last 30 days' playbacks.
	loadTestRunner.WithClientHistory(func(ctx context.Context) (map[apploadtest.PlaybackDevice]int, error) {
		counts, err := r.PlaySessionStats.CountByDeviceSince(ctx, time.Now().Add(-30*24*time.Hour))
		out := make(map[apploadtest.PlaybackDevice]int, len(counts))
		for k, n := range counts {
			out[apploadtest.PlaybackDevice{Type: k.Type, Name: k.Name}] = n
		}
		return out, err
	})
	// Subtitle segments read the blocks the stored index points at, when the
	// /subtitle route or the prewarmer has already built one for the source.
	infra.onDemandPackager.WithSubtitleIndex(httpapi.SubtitleIndexLookup("/data"))
	// Walks in flight and the per-plan counters (slot waits, streamed segments,
	// Cues-indexed subtitle tracks...) on /metrics.
	startMkvgoMetrics(ctx, infra.onDemandPackager.WindowStats, mkvgoadapter.WalksInFlight)

	if !benchInstance && resolveFlag(ctx, r.ServerCfg, "transcoding.keyframe_backfill", "EVEY_KEYFRAME_BACKFILL") != "off" {
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
		).WithActivePlayback(hlsVODRemover.ActivePlaybackCount).
			// Same paced pass as the keyframe walk: classify CFR/VFR via
			// mkvgo ops.Analyze and persist it, so the decision engine can
			// gate the copy rung off VFR sources (HLS copy segments drift
			// A/V on them). Best-effort: analysis failures never fail the
			// keyframe backfill itself.
			WithFrameRateAnalyzer(infra.mkvAdapter, r.MediaStream)
		keyframeBackfillWorker.Start(ctx)
		bgStoppers = append(bgStoppers, keyframeBackfillWorker.Stop)
		// Fresh additions: once the library has been quiet for 10 min after a
		// scan, drain the queue right away instead of waiting the idle cycle.
		// (Playback that starts sooner is covered by the on-demand inline fill.)
		bus.Subscribe(library.LibraryScanCompleted{}.EventName(), func(context.Context, shared.Event) {
			keyframeBackfillWorker.NudgeAfterQuiet(10 * time.Minute)
		})
		slog.Default().Info("keyframe backfill worker enabled (default-on; opt out with server_config transcoding.keyframe_backfill=off)")
	}
	// P3 per-title probe worker (default-on). Triggers only: missing metrics,
	// motion out of the model's fitted range, a sustained ceiling hit recorded
	// by the serve path, or the animation weak spot. Runs one probe at a time
	// at scan priority (nice/ionice), SIGSTOPped under playback, and skips a
	// tick entirely while a stream is served; results resume from the shared
	// point cache. A failure here never touches playback or P1/P2.
	if !benchInstance && resolveFlag(ctx, infra.repos.ServerCfg, "transcoding.title_probe", "EVEY_TITLE_PROBE") != "off" {
		prober := ffmpeg.NewTitleProber(filepath.Join(cfg.Transcoding.CacheDir, "probe-cache"), "", isBusy)
		probeWorker := apptranscoding.NewProbeWorker(
			complexityRepo,
			titleProberFunc{prober},
			mediaItemReaderFunc(func(ctx context.Context, id shared.MediaItemID) (*library.MediaItem, error) {
				return libQuerySvc.GetMediaItem(ctx, string(id))
			}),
			probeSignals,
			domaintranscoding.CostModelV2,
			apptranscoding.ProbeWorkerOpts{},
			slog.Default(),
		).WithActivePlayback(hlsVODRemover.ActivePlaybackCount)
		probeWorker.Start(ctx)
		bgStoppers = append(bgStoppers, probeWorker.Stop)
		slog.Default().Info("title probe worker enabled (default-on; opt out with server_config transcoding.title_probe=off)")
	}
	// Library-wide progress and ETA of the heavy workers for the system-health
	// view: one COUNT per worker a minute, from the counts they registered.
	coverageCtx, stopCoverage := context.WithCancel(ctx)
	if !benchInstance {
		go bgtasks.RunCoverageSampler(coverageCtx, time.Minute)
	}
	bgStoppers = append(bgStoppers, stopCoverage)
	// The cast tracker's Stop performs the FINAL flush of derived positions -
	// the common end of a cast is the user turning everything off, and losing
	// that last write would defeat the tracker. Registered on the same drain
	// as the transcode workers (before db.Close).
	bgStoppers = append(bgStoppers, castProgress.Stop)
	// Batch transcode jobs run on a context detached from their RPC (they must
	// outlive the progress stream) - the server's shutdown is the one thing
	// they must NOT outlive. Without this, SIGTERM mid-encode orphans ffmpeg.
	bgStoppers = append(bgStoppers, infra.transcoder.StopAll)

	// CMCD (CTA-5004) ingest sink. The HTTP handler parses the CMCD query
	// param on every segment / master / variant fetch and ships an event
	// here; the buffered worker drains into playback_qoe_events. Phase 1 is
	// pure observation - no feedback loop yet - so the only failure path we
	// care about is "buffer full" which surfaces via metrics + warn logs.
	qoeRepo := postgres.NewPlaybackQoERepo(db)
	qoeStorage := qoeStorageAdapter{repo: qoeRepo}
	// Retention for this table lives on the activity service's daily sweep (the
	// one that already bounds audit_log and server_metrics) rather than in a
	// second loop. Wired here because that service is built long before the
	// repo exists.
	activityCmdSvc.WithQoEEventPurger(qoeRepo)
	qoeSink := appqoe.NewBufferedSink(qoeStorage, appqoe.SinkOptions{
		BufferSize: 2048, // ~30 min of segments for a busy 5-user evening
		DropMetric: eveymetrics.QoEIngestDrops,
	}, slog.Default())
	qoeQuerySvc := appqoe.NewQueryService(qoeReaderAdapter{reader: postgres.NewPlaybackQoEReader(db)})

	introDetectionSvc := applibrary.NewIntroDetectionService(
		r.MediaSegment,
		r.Trickplay,
		r.MediaItemQuery,
		r.ServerCfg,
		infra.introDetector,
		cfg.Transcoding.TrickplayCacheDir,
	)
	introDetectionSvc.WithShutdownCtx(ctx)
	introDetectionSvc.WithActivePlayback(hlsVODRemover.ActivePlaybackCount)
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
		// The operator sizing the fan-out by hand wins over the budget (the
		// forks it makes are still reserved in it).
		if n, err := strconv.Atoi(v); err == nil {
			introDetectionSvc.WithMaxParallelDetections(n)
			slog.Info("intro detection: max parallel overridden by env", slog.Int("max", n))
		} else {
			slog.Warn("EVEY_INTRO_DETECTION_MAX_PARALLEL not a number, keeping default", slog.String("value", v))
		}
	} else {
		// The fan-out is this task's CPU share, read at every run: the admin's
		// per-task setting under Server settings, else the global share.
		introDetectionSvc.WithCPUBudget(cpuBudget)
		slog.Info("intro detection: max parallel drawn from the cpu budget",
			slog.Int("max", cpuBudget.Slots("intro_detection")))
	}

	// External intro/recap/credits providers (TIDB primary, IntroDB fallback)
	// run ahead of MKV chapter pass when enabled. Off by default - opt-in
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
	// - the PARENT of segments/. It treated segments/, photos/ and
	// music_album_art/ as three LRU units, so on overflow it would have removed a
	// whole sibling cache (every photo thumbnail / all album art) or the entire
	// segments blob at once instead of evicting per-hash. Removed: StartEviction is
	// the single, correctly-scoped evictor. (The old whole-subdir CacheEvictor,
	// which evicted by mtime with no pendingRefs guard, was deleted - it would
	// have evicted a hash mid-write if ever re-wired.)

	return &appDeps{
		bus:             bus,
		presenceTracker: apppresence.NewTracker(),
		bgStoppers:      bgStoppers,
		// The media-item repo doubles as the seek-index fingerprint refresher, so a
		// reindexed file isn't re-probed (and its keyframe index dropped) by the
		// next scan. r.MediaItem satisfies appmaintenance.FileStateRefresher.
		seekIndexFileState: r.MediaItem,
		interleaveRecorder: r.MediaItem,
		keyframeOnDemand:   keyframeOnDemand,
		loadTestRunner:     loadTestRunner,
		loadTestBaseURL:    loadTestBaseURL,
		loadTestPublicURL:  loadTestPublicURL,
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
			castProgress:     castProgress,
			transcodingSvc:   transcodingSvc,
			hlsVODOrch:       hlsVODOrch,
			hlsVODCache:      hlsVODCache,
			remuxLeases:      remuxLeases,
			probeSignals:     probeSignals,
			hlsTranscoder:    hlsTranscoder,
			hlsVODSoftware:   hlsVODSoftware,
			hlsVODRemover:    hlsVODRemover,
			audioOrch:        audioOrch,
			qoeSink:          qoeSink,
			qoeQuerySvc:      qoeQuerySvc,
			complexityRepo:   complexityRepo,
			calibrationSvc:   calibrationSvc,
			videoLimiter:     videoLimiter,
			videoShaper:      videoShaper,
			capsOverrideSvc:  capsOverrideSvc,
			mkvSvc:           mkvSvc,
			mkvAdapter:       infra.mkvAdapter,
			musicSvc:         musicSvc,
			musicNowPlaying:  musicNowPlaying,
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
			metadataCmdSvc:     metadataCmdSvc,
			metadataQuerySvc:   metadataQuerySvc,
			enricher:           enricher,
			collectionCmdSvc:   collectionCmdSvc,
			collectionQuerySvc: collectionQuerySvc,
			favoriteSvc:        favoriteSvc,
			mediaRequestSvc:    mediaRequestSvc,
			playlistSvc:        playlistSvc,
			playlistGenSvc:     playlistGenSvc,
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
			pluginIdleMgr:              pluginIdleMgr,
		},
		auth: authDeps{
			tokenGen:     infra.tokenGen,
			userTokenGen: guardedTokens,
			hasher:       infra.hasher,
			profileHours: profileHours,
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
			repairJournal:     postgres.NewRepairJournalRepository(r.AuditLog),
		},
		cfgStore:          httpapi.NewServerConfigStore(r.ServerCfg),
		serverCfgSvc:      serverCfgSvc,
		liveSMTP:          liveSMTP,
		smtpSender:        smtpSender,
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
		frameExtractor:         snapshotFrameExtractor(ctx, r.ServerCfg, infra),
		copySeekProber:         infra.prober,
		onDemandPackager:       infra.onDemandPackager,
		playbackActivity:       playbackActivity,
		onDemandConcatPackager: infra.onDemandConcatPackager,
		hlsAESSecret:           infra.hlsAESSecret,
	}, nil
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
	// The demo sets itself up when the flag is on and cleans up after itself
	// when the flag is gone (a marker in the server config says it ran here).
	startDemoLifecycle(ctx, cfg, infra, deps, rootLog)
	return deps, nil
}

// startDemoBootstrap makes the demo instance self-sufficient: the `demo`
// administrator, the visitor accounts, the four libraries and the freely
// licensed media, from the product's own services. Detached from the boot
// path (the media alone is gigabytes on a slow mirror), bounded by the server
// context, recovered like every fire-and-forget goroutine.
func startDemoLifecycle(ctx context.Context, cfg *config.Config, infra *infraDeps, deps *appDeps, rootLog *slog.Logger) {
	r := infra.repos
	boot := appdemo.NewBootstrapper(r.User, r.Profile, deps.user.authSvc, deps.user.userSvc,
		r.Library, deps.library.cmdSvc, infra.hasher, cfg.Server.DemoMediaDir, eveylogger.NewWith(rootLog, "demo")).
		WithLibraryAccess(r.LibraryAccess).
		WithMarker(r.ServerCfg).
		WithSafetyBackup(func(ctx context.Context) (string, error) {
			if deps.backupSvc == nil {
				return "", errors.New("no backup service")
			}
			m, err := deps.backupSvc.CreateBackup(ctx)
			if err != nil {
				return "", err
			}
			return m.ID, nil
		}).
		WithPlugins(r.Plugin, deps.engagement.pluginInstallerSvc, deps.engagement.pluginRuntimeSvc, cfg.Server.DemoPluginDir).
		WithHomeLayout(deps.engagement.feedCmdSvc).
		WithHistory(r.MediaItemQuery, r.WatchProgress, deps.media.playbackSvc.CommandService()).
		WithReset(deps.engagement.activityQuerySvc, r.WatchProgress).
		WithCredentialPurge(demoCredentials{keys: r.APIKey, invites: r.InviteLink}).
		WithAdminAccountIf(cfg.Server.DemoExtended, cfg.Server.DemoUser, cfg.Server.DemoPassword).
		WithAssembler(demoAssembler).
		WithActivity(func() bool {
			if hls := deps.media.hlsVODRemover; hls != nil && hls.ActivePlaybackCount() > 0 {
				return true
			}
			now := deps.media.musicNowPlaying
			return now != nil && len(now.List()) > 0
		})
	if !cfg.Server.Demo {
		// Cheap check on the boot path, the teardown itself detached.
		probe, cancel := context.WithTimeout(ctx, 10*time.Second)
		was := boot.WasBootstrapped(probe)
		cancel()
		if !was {
			return
		}
	}
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered in demo lifecycle", slog.Any("panic", rec), slog.Any("stack", debug.Stack()))
			}
		}()
		if cfg.Server.Demo {
			if err := boot.Run(ctx); err != nil && ctx.Err() == nil {
				slog.Error("demo bootstrap", slog.Any("err", err))
			}
			// Media health shows the last scan, kept in memory: one after the
			// seed, so the page is not empty. Visitors cannot start one.
			if svc := deps.demoSeekIndex.Load(); svc != nil {
				if err := svc.StartScan(ctx, ""); err != nil {
					slog.Warn("demo: media health scan", slog.Any("err", err))
				}
			}
			// Hourly when nobody is watching, daily regardless: the demo keeps
			// itself at its seed, no host cron needed.
			if cfg.Server.DemoExtended {
				// A normal server with the network piloted: nothing is reset,
				// the operator restores the VM snapshot when he wants it clean.
				return
			}
			boot.RunResets(ctx, time.Hour, 24*time.Hour)
			return
		}
		// Bounded like the other background writes: the deletes are a few
		// rows and a few gigabytes of files, not a download.
		tctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		if err := boot.Teardown(tctx); err != nil {
			slog.Error("demo teardown", slog.Any("err", err))
		}
	}()
}
