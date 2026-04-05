package main

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"

	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/gravity-zero/evey/internal/config"
	eveylogger "github.com/gravity-zero/evey/internal/pkg/logger"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
)

func buildGRPCServer(deps *appDeps, cfg *config.Config, rootLog *slog.Logger, logDir string, serverCtx context.Context) *grpcapi.GRPCServer {
	authInterceptor := interceptors.NewAuthInterceptor(deps.auth.tokenGen).WithAPIKeys(deps.repos.apiKeyRepo, deps.repos.userRepo).WithTranslator(deps.translator)
	grpcLog := eveylogger.NewWith(rootLog, "grpc")
	grpcSrv := grpcapi.NewGRPCServer(
		deps.library.cmdSvc, deps.library.querySvc, deps.user.authSvc, deps.user.userSvc, deps.user.accessSvc, deps.user.querySvc,
		deps.media.playbackSvc.CommandService(), deps.media.playbackSvc.QueryService(),
		deps.media.transcodingSvc,
		deps.media.mkvSvc, deps.engagement.pluginInstallerSvc, deps.engagement.pluginRuntimeSvc, deps.engagement.pluginQuerySvc, deps.content.metadataCmdSvc, deps.content.metadataQuerySvc,
		deps.engagement.activityCmdSvc, deps.engagement.activityQuerySvc,
		deps.engagement.notifCmdSvc, deps.engagement.notifQuerySvc, deps.engagement.feedCmdSvc, deps.engagement.feedQuerySvc, deps.engagement.webhookSvc,
		deps.content.collectionSvc, deps.content.playlistSvc, deps.content.favoriteSvc,
		deps.repos.userRepo, deps.repos.profileRepo, deps.repos.libraryAccessRepo,
		deps.backupSvc, cfg, deps.cfgStore,
		version,
		grpc.MaxRecvMsgSize(16 * 1024 * 1024),
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
	grpcSrv.SetTranslator(deps.translator)
	grpcSrv.SetHLSManager(deps.media.hlsManager)
	grpcSrv.SetScheduler(deps.library.scheduler)
	grpcSrv.SetLogDir(logDir)
	grpcSrv.SetServerCtx(serverCtx)
	grpcSrv.SetServerConfigHandler(grpcapi.NewServerConfigHandler(deps.repos.serverCfgRepo))
	grpcSrv.SetSMTPConfigured(cfg.SMTP.Host != "")
	grpcSrv.SetShareService(deps.media.shareSvc)
	grpcSrv.SetSyncPlayHub(deps.media.syncPlayHub)
	if deps.content.enricher != nil {
		grpcSrv.SetBioEnricher(deps.content.enricher)
	}
	grpcSrv.SetMusicService(deps.media.musicSvc)
	grpcSrv.SetPhotoService(deps.media.photoSvc)
	if deps.repos.pushSubRepo != nil {
		grpcSrv.SetPushDeps(deps.repos.pushSubRepo, resolveVAPIDPublicKey(deps.repos.serverCfgRepo, cfg), ratelimit.New(10))
	}
	grpcSrv.RegisterServices()
	return grpcSrv
}
