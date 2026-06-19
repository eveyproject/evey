package main

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"

	"github.com/evey-project/evey/internal/adapters/driven/mkvgo"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	"github.com/evey-project/evey/internal/config"
	eveylogger "github.com/evey-project/evey/internal/pkg/logger"
	"github.com/evey-project/evey/internal/pkg/quickconnecthub"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

func buildGRPCServer(deps *appDeps, cfg *config.Config, rootLog *slog.Logger, logDir string, serverCtx context.Context, qcHub *quickconnecthub.Hub) *grpcapi.GRPCServer {
	authInterceptor := interceptors.NewAuthInterceptor(deps.auth.userTokenGen).WithAPIKeys(deps.repos.apiKeyRepo, deps.repos.userRepo).WithTranslator(deps.translator)
	pluginTokenInterceptor := interceptors.NewPluginTokenInterceptor(deps.auth.tokenGen)
	publicRL := interceptors.DefaultPublicMethodLimiters()
	if cfg.Log.DevMode {
		for _, ml := range publicRL {
			ml.Limiter = ratelimit.NewWithBurst(10000, 10000)
			ml.RetryAfterSecs = 0
		}
	}
	grpcLog := eveylogger.NewWith(rootLog, "grpc")

	// Admin MKV/WebM seek-index maintenance (scan/reindex/rollback). The mkvgo
	// adapter is stateless, and the catalog paginates the library to feed every
	// file to the scanner.
	seekIndexSvc := appmaintenance.NewSeekIndexService(
		mkvgo.New(),
		seekIndexCatalog{lib: deps.library.querySvc},
		eveylogger.NewWith(rootLog, "seek-index"),
	)

	grpcDeps := grpcapi.GRPCServerDeps{
		LibCmd:                 deps.library.cmdSvc,
		LibQuery:               deps.library.querySvc,
		UserAuth:               deps.user.authSvc,
		UserSvc:                deps.user.userSvc,
		UserAccess:             deps.user.accessSvc,
		UserQuery:              deps.user.querySvc,
		PlaybackCmd:            deps.media.playbackSvc.CommandService(),
		PlaybackQuery:          deps.media.playbackSvc.QueryService(),
		Transcoding:            deps.media.transcodingSvc,
		Mkv:                    deps.media.mkvSvc,
		PluginInstaller:        deps.engagement.pluginInstallerSvc,
		PluginRuntime:          deps.engagement.pluginRuntimeSvc,
		PluginQuery:            deps.engagement.pluginQuerySvc,
		PluginToken:            deps.engagement.pluginTokenSvc,
		PluginRunner:           deps.pluginRunner,
		PluginResourceDefaults: deps.pluginResDefaults,
		PluginData:             deps.repos.pluginDataRepo,
		MetadataCmd:            deps.content.metadataCmdSvc,
		MetadataQuery:          deps.content.metadataQuerySvc,
		ActivityCmd:            deps.engagement.activityCmdSvc,
		ActivityQuery:          deps.engagement.activityQuerySvc,
		NotifCmd:               deps.engagement.notifCmdSvc,
		NotifQuery:             deps.engagement.notifQuerySvc,
		FeedCmd:                deps.engagement.feedCmdSvc,
		FeedQuery:              deps.engagement.feedQuerySvc,
		Webhook:                deps.engagement.webhookSvc,
		Collection:             deps.content.collectionSvc,
		Playlist:               deps.content.playlistSvc,
		Favorite:               deps.content.favoriteSvc,
		Backup:                 deps.backupSvc,
		BackupScheduler:        deps.backupScheduler,
		Cfg:                    cfg,
		CfgStore:               deps.cfgStore,
		Version:                version,
		HLSVODOrch:             deps.media.hlsVODOrch,
		HLSVODRemover:          deps.media.hlsVODRemover,
		Scheduler:              deps.library.scheduler,
		LogDir:                 logDir,
		ServerCtx:              serverCtx,
		Translator:             deps.translator,
		SMTPConfigured:         cfg.SMTP.Host != "",
		ServerCfgSvc:           deps.serverCfgSvc,
		ShareSvc:               deps.media.shareSvc,
		BioEnricher:            deps.content.enricher,
		Music:                  deps.media.musicSvc,
		Photo:                  deps.media.photoSvc,
		SyncplayHub:            deps.media.syncPlayHub,
		StarRating:             deps.content.starRatingSvc,
		QuickConnectHub:        qcHub,
		IntroCache:             deps.repos.introMetaCache,
		IntroBackfill:          deps.library.introDetectionSvc,
		SeekIndex:              seekIndexSvc,
		StreamMeter:            deps.engagement.streamMeter,
	}

	if deps.repos.pushSubRepo != nil {
		grpcDeps.PushEnabled = true
		grpcDeps.VAPIDPubKey = resolveVAPIDPublicKey(deps.repos.serverCfgRepo, cfg)
		grpcDeps.PushRegisterRL = ratelimit.New(10)
	}

	grpcSrv := grpcapi.NewGRPCServer(grpcDeps,
		grpc.MaxRecvMsgSize(16*1024*1024),
		grpc.ChainUnaryInterceptor(
			interceptors.RecoveryUnary(),
			interceptors.ErrorSanitizerUnary(),
			interceptors.LocaleUnary(),
			interceptors.ClientInfoUnary(),
			interceptors.RateLimitUnary(publicRL),
			interceptors.LoggingUnary(grpcLog),
			pluginTokenInterceptor.Unary(),
			authInterceptor.Unary(),
		),
		grpc.ChainStreamInterceptor(
			interceptors.RecoveryStream(),
			interceptors.ErrorSanitizerStream(),
			interceptors.LocaleStream(),
			interceptors.ClientInfoStream(),
			interceptors.RateLimitStream(publicRL),
			interceptors.LoggingStream(grpcLog),
			pluginTokenInterceptor.Stream(),
			authInterceptor.Stream(),
		),
	)
	grpcSrv.RegisterServices()
	return grpcSrv
}
