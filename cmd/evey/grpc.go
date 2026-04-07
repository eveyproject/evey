package main

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"

	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/gravity-zero/evey/internal/config"
	eveylogger "github.com/gravity-zero/evey/internal/pkg/logger"
	"github.com/gravity-zero/evey/internal/pkg/quickconnecthub"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
)

func buildGRPCServer(deps *appDeps, cfg *config.Config, rootLog *slog.Logger, logDir string, serverCtx context.Context, qcHub *quickconnecthub.Hub) *grpcapi.GRPCServer {
	authInterceptor := interceptors.NewAuthInterceptor(deps.auth.tokenGen).WithAPIKeys(deps.repos.apiKeyRepo, deps.repos.userRepo).WithTranslator(deps.translator)
	grpcLog := eveylogger.NewWith(rootLog, "grpc")

	grpcDeps := grpcapi.GRPCServerDeps{
		LibCmd:          deps.library.cmdSvc,
		LibQuery:        deps.library.querySvc,
		UserAuth:        deps.user.authSvc,
		UserSvc:         deps.user.userSvc,
		UserAccess:      deps.user.accessSvc,
		UserQuery:       deps.user.querySvc,
		PlaybackCmd:     deps.media.playbackSvc.CommandService(),
		PlaybackQuery:   deps.media.playbackSvc.QueryService(),
		Transcoding:     deps.media.transcodingSvc,
		Mkv:             deps.media.mkvSvc,
		PluginInstaller: deps.engagement.pluginInstallerSvc,
		PluginRuntime:   deps.engagement.pluginRuntimeSvc,
		PluginQuery:     deps.engagement.pluginQuerySvc,
		MetadataCmd:     deps.content.metadataCmdSvc,
		MetadataQuery:   deps.content.metadataQuerySvc,
		ActivityCmd:     deps.engagement.activityCmdSvc,
		ActivityQuery:   deps.engagement.activityQuerySvc,
		NotifCmd:        deps.engagement.notifCmdSvc,
		NotifQuery:      deps.engagement.notifQuerySvc,
		FeedCmd:         deps.engagement.feedCmdSvc,
		FeedQuery:       deps.engagement.feedQuerySvc,
		Webhook:         deps.engagement.webhookSvc,
		Collection:      deps.content.collectionSvc,
		Playlist:        deps.content.playlistSvc,
		Favorite:        deps.content.favoriteSvc,
		Users:           deps.repos.userRepo,
		Profiles:        deps.repos.profileRepo,
		LibraryAccess:   deps.repos.libraryAccessRepo,
		Backup:          deps.backupSvc,
		Cfg:             cfg,
		CfgStore:        deps.cfgStore,
		Version:         version,
		HLSManager:      deps.media.hlsManager,
		Scheduler:       deps.library.scheduler,
		LogDir:          logDir,
		ServerCtx:       serverCtx,
		Translator:      deps.translator,
		SMTPConfigured:  cfg.SMTP.Host != "",
		ServerCfgRepo:   deps.repos.serverCfgRepo,
		ShareSvc:        deps.media.shareSvc,
		BioEnricher:     deps.content.enricher,
		Music:           deps.media.musicSvc,
		Photo:           deps.media.photoSvc,
		SyncplayHub:     deps.media.syncPlayHub,
		ThumbRating:     deps.repos.userRatingRepo,
		StarRating:      deps.content.starRatingSvc,
		QuickConnectHub: qcHub,
	}

	if deps.repos.pushSubRepo != nil {
		grpcDeps.PushSubs = deps.repos.pushSubRepo
		grpcDeps.VAPIDPubKey = resolveVAPIDPublicKey(deps.repos.serverCfgRepo, cfg)
		grpcDeps.PushRegisterRL = ratelimit.New(10)
	}

	grpcSrv := grpcapi.NewGRPCServer(grpcDeps,
		grpc.MaxRecvMsgSize(16*1024*1024),
		grpc.ChainUnaryInterceptor(
			interceptors.ErrorSanitizerUnary(),
			interceptors.LocaleUnary(),
			interceptors.ClientInfoUnary(),
			interceptors.LoggingUnary(grpcLog),
			authInterceptor.Unary(),
		),
		grpc.ChainStreamInterceptor(
			interceptors.ErrorSanitizerStream(),
			interceptors.LocaleStream(),
			interceptors.ClientInfoStream(),
			interceptors.LoggingStream(grpcLog),
			authInterceptor.Stream(),
		),
	)
	grpcSrv.RegisterServices()
	return grpcSrv
}
