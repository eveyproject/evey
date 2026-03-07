package grpcapi

import (
	"log"
	"net"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	appmkv "github.com/gravity-zero/evey/internal/app/mkv"
	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	"github.com/gravity-zero/evey/internal/domain/user"
	"google.golang.org/grpc"
)

type GRPCServer struct {
	server      *grpc.Server
	library     *applibrary.Service
	user        *appuser.Service
	playback    *appplayback.Service
	transcoding *apptranscoding.Service
	mkv         *appmkv.Service
	plugin      *appplugin.Service
	metadata    *appmetadata.Service
	activity    *appactivity.Service
	users       user.UserRepository
	version     string
}

func NewGRPCServer(
	library *applibrary.Service,
	userSvc *appuser.Service,
	playback *appplayback.Service,
	transcoding *apptranscoding.Service,
	mkv *appmkv.Service,
	plugin *appplugin.Service,
	metadata *appmetadata.Service,
	activity *appactivity.Service,
	users user.UserRepository,
	version string,
	opts ...grpc.ServerOption,
) *GRPCServer {
	return &GRPCServer{
		server:      grpc.NewServer(opts...),
		library:     library,
		user:        userSvc,
		playback:    playback,
		transcoding: transcoding,
		mkv:         mkv,
		plugin:      plugin,
		metadata:    metadata,
		activity:    activity,
		users:       users,
		version:     version,
	}
}

func (s *GRPCServer) RegisterServices() {
	eveyv1.RegisterLibraryServiceServer(s.server, &libraryHandler{svc: s.library})
	eveyv1.RegisterUserServiceServer(s.server, &userHandler{svc: s.user, users: s.users})
	eveyv1.RegisterPlaybackServiceServer(s.server, &playbackHandler{svc: s.playback})
	eveyv1.RegisterTranscodingServiceServer(s.server, &transcodingHandler{svc: s.transcoding})
	eveyv1.RegisterMKVServiceServer(s.server, &mkvHandler{svc: s.mkv})
	eveyv1.RegisterMetadataServiceServer(s.server, &metadataHandler{svc: s.metadata})
	eveyv1.RegisterAdminServiceServer(s.server, &adminHandler{
		userSvc:        s.user,
		transcodingSvc: s.transcoding,
		activitySvc:    s.activity,
		pluginSvc:      s.plugin,
		version:        s.version,
	})
	eveyv1.RegisterActivityServiceServer(s.server, &activityHandler{svc: s.activity})
}

func (s *GRPCServer) Start(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("gRPC server listening on %s", addr)
	return s.server.Serve(lis)
}

func (s *GRPCServer) GRPCServer() *grpc.Server {
	return s.server
}

func (s *GRPCServer) Stop() {
	s.server.GracefulStop()
}
