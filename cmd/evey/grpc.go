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
	authInterceptor := interceptors.NewAuthInterceptor(deps.tokenGen).WithAPIKeys(deps.apiKeyRepo, deps.userRepo)
	grpcLog := eveylogger.NewWith(rootLog, "grpc")
	grpcSrv := grpcapi.NewGRPCServer(
		deps.libCmdSvc, deps.libQuerySvc, deps.userCmdSvc, deps.userQuerySvc,
		deps.playbackSvc.CommandService(), deps.playbackSvc.QueryService(),
		deps.transcodingSvc,
		deps.mkvSvc, deps.pluginSvc, deps.metadataCmdSvc, deps.metadataQuerySvc,
		deps.activityCmdSvc, deps.activityQuerySvc,
		deps.notifCmdSvc, deps.notifQuerySvc, deps.feedCmdSvc, deps.feedQuerySvc, deps.webhookSvc,
		deps.collectionSvc, deps.playlistSvc, deps.favoriteSvc,
		deps.userRepo, deps.profileRepo, deps.libraryAccessRepo,
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
	grpcSrv.SetHLSManager(deps.hlsManager)
	grpcSrv.SetScheduler(deps.libScheduler)
	grpcSrv.SetLogDir(logDir)
	grpcSrv.SetServerConfigHandler(grpcapi.NewServerConfigHandler(deps.serverCfgRepo))
	grpcSrv.RegisterServices()
	return grpcSrv
}
