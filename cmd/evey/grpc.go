package main

import (
	"log/slog"

	"google.golang.org/grpc"

	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/gravity-zero/evey/internal/config"
	eveylogger "github.com/gravity-zero/evey/internal/pkg/logger"
)

func buildGRPCServer(deps *appDeps, cfg *config.Config, rootLog *slog.Logger, logDir string) *grpcapi.GRPCServer {
	authInterceptor := interceptors.NewAuthInterceptor(deps.auth.tokenGen).WithAPIKeys(deps.repos.apiKeyRepo, deps.repos.userRepo)
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
	grpcSrv.SetHLSManager(deps.media.hlsManager)
	grpcSrv.SetScheduler(deps.library.scheduler)
	grpcSrv.SetLogDir(logDir)
	grpcSrv.SetServerConfigHandler(grpcapi.NewServerConfigHandler(deps.repos.serverCfgRepo))
	grpcSrv.RegisterServices()
	return grpcSrv
}
