package grpcapi

import (
	"log/slog"
	"net"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	appfeed "github.com/gravity-zero/evey/internal/app/feed"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	appmkv "github.com/gravity-zero/evey/internal/app/mkv"
	appnotification "github.com/gravity-zero/evey/internal/app/notification"
	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	appwebhook "github.com/gravity-zero/evey/internal/app/webhook"
	"github.com/gravity-zero/evey/internal/domain/user"
	"google.golang.org/grpc"
)

type GRPCServer struct {
	server        *grpc.Server
	libCmd        *applibrary.CommandService
	libQuery      *applibrary.QueryService
	user          *appuser.Service
	playbackCmd   *appplayback.CommandService
	playbackQuery *appplayback.QueryService
	transcoding   *apptranscoding.Service
	mkv           *appmkv.Service
	plugin        *appplugin.Service
	metadata      *appmetadata.Service
	activity      *appactivity.Service
	notification  *appnotification.Service
	feedCmd       *appfeed.CommandService
	feedQuery     *appfeed.QueryService
	webhook       *appwebhook.Service
	users         user.UserRepository
	profiles      user.ProfileRepository
	libraryAccess user.LibraryAccessRepository
	hlsManager    *apptranscoding.HLSManager
	version       string
}

func (s *GRPCServer) SetHLSManager(m *apptranscoding.HLSManager) {
	s.hlsManager = m
}

func NewGRPCServer(
	libCmd *applibrary.CommandService,
	libQuery *applibrary.QueryService,
	userSvc *appuser.Service,
	playbackCmd *appplayback.CommandService,
	playbackQuery *appplayback.QueryService,
	transcoding *apptranscoding.Service,
	mkv *appmkv.Service,
	plugin *appplugin.Service,
	metadata *appmetadata.Service,
	activity *appactivity.Service,
	notification *appnotification.Service,
	feedCmd *appfeed.CommandService,
	feedQuery *appfeed.QueryService,
	webhook *appwebhook.Service,
	users user.UserRepository,
	profiles user.ProfileRepository,
	libraryAccess user.LibraryAccessRepository,
	version string,
	opts ...grpc.ServerOption,
) *GRPCServer {
	return &GRPCServer{
		server:        grpc.NewServer(opts...),
		libCmd:        libCmd,
		libQuery:      libQuery,
		user:          userSvc,
		playbackCmd:   playbackCmd,
		playbackQuery: playbackQuery,
		transcoding:   transcoding,
		mkv:           mkv,
		plugin:        plugin,
		metadata:      metadata,
		activity:      activity,
		notification:  notification,
		feedCmd:       feedCmd,
		feedQuery:     feedQuery,
		webhook:       webhook,
		users:         users,
		profiles:      profiles,
		libraryAccess: libraryAccess,
		version:       version,
	}
}

func (s *GRPCServer) RegisterServices() {
	eveyv1.RegisterLibraryServiceServer(s.server, &libraryHandler{cmd: s.libCmd, query: s.libQuery, metadataSvc: s.metadata, libraryAccess: s.libraryAccess, profiles: s.profiles})
	eveyv1.RegisterUserServiceServer(s.server, &userHandler{svc: s.user, users: s.users, activitySvc: s.activity})
	eveyv1.RegisterPlaybackServiceServer(s.server, &playbackHandler{cmd: s.playbackCmd, query: s.playbackQuery, librarySvc: s.libQuery})
	eveyv1.RegisterTranscodingServiceServer(s.server, &transcodingHandler{svc: s.transcoding, profiles: s.profiles})
	eveyv1.RegisterMKVServiceServer(s.server, &mkvHandler{svc: s.mkv})
	eveyv1.RegisterMetadataServiceServer(s.server, &metadataHandler{svc: s.metadata})
	eveyv1.RegisterAdminServiceServer(s.server, &adminHandler{
		userSvc:        s.user,
		transcodingSvc: s.transcoding,
		activitySvc:    s.activity,
		pluginSvc:      s.plugin,
		hlsManager:     s.hlsManager,
		libLister:      s.libQuery,
		version:        s.version,
	})
	eveyv1.RegisterActivityServiceServer(s.server, &activityHandler{svc: s.activity})
	eveyv1.RegisterNotificationServiceServer(s.server, &notificationHandler{svc: s.notification})
	eveyv1.RegisterFeedServiceServer(s.server, &feedHandler{commands: s.feedCmd, queries: s.feedQuery})
	eveyv1.RegisterWebhookServiceServer(s.server, &webhookHandler{svc: s.webhook})
}

func (s *GRPCServer) Start(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	slog.Info("gRPC server listening", slog.String("addr", addr))
	return s.server.Serve(lis)
}

func (s *GRPCServer) GRPCServer() *grpc.Server {
	return s.server
}

func (s *GRPCServer) Stop() {
	s.server.GracefulStop()
}
