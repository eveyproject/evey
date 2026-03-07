package grpcapi

import (
	"log"
	"net"

	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	appmkv "github.com/gravity-zero/evey/internal/app/mkv"
	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	appuser "github.com/gravity-zero/evey/internal/app/user"
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
}

func NewGRPCServer(
	library *applibrary.Service,
	user *appuser.Service,
	playback *appplayback.Service,
	transcoding *apptranscoding.Service,
	mkv *appmkv.Service,
	plugin *appplugin.Service,
	metadata *appmetadata.Service,
	activity *appactivity.Service,
	opts ...grpc.ServerOption,
) *GRPCServer {
	return &GRPCServer{
		server:      grpc.NewServer(opts...),
		library:     library,
		user:        user,
		playback:    playback,
		transcoding: transcoding,
		mkv:         mkv,
		plugin:      plugin,
		metadata:    metadata,
		activity:    activity,
	}
}

func (s *GRPCServer) RegisterServices() {
	// TODO: register generated protobuf service implementations
	// e.g. pb.RegisterLibraryServiceServer(s.server, &libraryHandler{svc: s.library})
}

func (s *GRPCServer) Start(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("gRPC server listening on %s", addr)
	return s.server.Serve(lis)
}

func (s *GRPCServer) Stop() {
	s.server.GracefulStop()
}
