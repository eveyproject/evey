package grpcapi

import (
	"log/slog"
	"net"
	"time"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	appbackup "github.com/gravity-zero/evey/internal/app/backup"
	appcollection "github.com/gravity-zero/evey/internal/app/collection"
	appfavorite "github.com/gravity-zero/evey/internal/app/favorite"
	appfeed "github.com/gravity-zero/evey/internal/app/feed"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	appmkv "github.com/gravity-zero/evey/internal/app/mkv"
	appnotification "github.com/gravity-zero/evey/internal/app/notification"
	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	appplaylist "github.com/gravity-zero/evey/internal/app/playlist"
	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	appwebhook "github.com/gravity-zero/evey/internal/app/webhook"
	"github.com/gravity-zero/evey/internal/config"
	"github.com/gravity-zero/evey/internal/domain/user"
	"google.golang.org/grpc"
)

type GRPCServer struct {
	server        *grpc.Server
	libCmd        *applibrary.CommandService
	libQuery      *applibrary.QueryService
	userAuth      *appuser.CommandService
	userSvc       *appuser.UserService
	userAccess    *appuser.AccessService
	userQuery     *appuser.QueryService
	playbackCmd   *appplayback.CommandService
	playbackQuery *appplayback.QueryService
	transcoding   *apptranscoding.Service
	mkv           *appmkv.Service
	plugin        *appplugin.Service
	metadataCmd   *appmetadata.CommandService
	metadataQuery *appmetadata.QueryService
	activityCmd   *appactivity.CommandService
	activityQuery *appactivity.QueryService
	notifCmd      *appnotification.CommandService
	notifQuery    *appnotification.QueryService
	feedCmd       *appfeed.CommandService
	feedQuery     *appfeed.QueryService
	webhook       *appwebhook.Service
	collection    *appcollection.Service
	playlist      *appplaylist.Service
	favorite      *appfavorite.Service
	users         user.UserRepository
	profiles      user.ProfileRepository
	libraryAccess user.LibraryAccessRepository
	hlsManager    *apptranscoding.HLSManager
	scheduler     *applibrary.Scheduler
	logDir        string
	version       string
	startTime     time.Time
	ffmpegPath    string
	backup          *appbackup.Service
	cfg             *config.Config
	cfgStore        remoteAccessConfigStore
	serverCfgHndlr  *serverConfigHandler
}

func (s *GRPCServer) SetServerConfigHandler(h *serverConfigHandler) {
	s.serverCfgHndlr = h
}

func (s *GRPCServer) SetHLSManager(m *apptranscoding.HLSManager) {
	s.hlsManager = m
}

func (s *GRPCServer) SetScheduler(sc *applibrary.Scheduler) {
	s.scheduler = sc
}

func (s *GRPCServer) SetLogDir(dir string) {
	s.logDir = dir
}

func NewGRPCServer(
	libCmd *applibrary.CommandService,
	libQuery *applibrary.QueryService,
	userAuth *appuser.CommandService,
	userSvc *appuser.UserService,
	userAccess *appuser.AccessService,
	userQuery *appuser.QueryService,
	playbackCmd *appplayback.CommandService,
	playbackQuery *appplayback.QueryService,
	transcoding *apptranscoding.Service,
	mkv *appmkv.Service,
	plugin *appplugin.Service,
	metadataCmd *appmetadata.CommandService,
	metadataQuery *appmetadata.QueryService,
	activityCmd *appactivity.CommandService,
	activityQuery *appactivity.QueryService,
	notifCmd *appnotification.CommandService,
	notifQuery *appnotification.QueryService,
	feedCmd *appfeed.CommandService,
	feedQuery *appfeed.QueryService,
	webhook *appwebhook.Service,
	collection *appcollection.Service,
	playlist *appplaylist.Service,
	favorite *appfavorite.Service,
	users user.UserRepository,
	profiles user.ProfileRepository,
	libraryAccess user.LibraryAccessRepository,
	backup *appbackup.Service,
	cfg *config.Config,
	cfgStore remoteAccessConfigStore,
	version string,
	opts ...grpc.ServerOption,
) *GRPCServer {
	return &GRPCServer{
		server:        grpc.NewServer(opts...),
		libCmd:        libCmd,
		libQuery:      libQuery,
		userAuth:      userAuth,
		userSvc:       userSvc,
		userAccess:    userAccess,
		userQuery:     userQuery,
		playbackCmd:   playbackCmd,
		playbackQuery: playbackQuery,
		transcoding:   transcoding,
		mkv:           mkv,
		plugin:        plugin,
		metadataCmd:   metadataCmd,
		metadataQuery: metadataQuery,
		activityCmd:   activityCmd,
		activityQuery: activityQuery,
		notifCmd:      notifCmd,
		notifQuery:    notifQuery,
		feedCmd:       feedCmd,
		feedQuery:     feedQuery,
		webhook:       webhook,
		collection:    collection,
		playlist:      playlist,
		favorite:      favorite,
		users:         users,
		profiles:      profiles,
		libraryAccess: libraryAccess,
		backup:        backup,
		cfg:           cfg,
		cfgStore:      cfgStore,
		version:       version,
		startTime:     time.Now(),
		ffmpegPath:    "ffmpeg",
	}
}

func (s *GRPCServer) RegisterServices() {
	eveyv1.RegisterLibraryServiceServer(s.server, &libraryHandler{cmd: s.libCmd, query: s.libQuery, metadataSvc: s.metadataQuery, libraryAccess: s.libraryAccess, profiles: s.profiles})
	eveyv1.RegisterUserServiceServer(s.server, &userHandler{auth: s.userAuth, userSvc: s.userSvc, access: s.userAccess, query: s.userQuery, users: s.users, activitySvc: s.activityCmd})
	eveyv1.RegisterPlaybackServiceServer(s.server, &playbackHandler{cmd: s.playbackCmd, query: s.playbackQuery, librarySvc: s.libQuery, activitySvc: s.activityCmd, profiles: s.profiles})
	eveyv1.RegisterTranscodingServiceServer(s.server, &transcodingHandler{svc: s.transcoding, profiles: s.profiles})
	eveyv1.RegisterMKVServiceServer(s.server, &mkvHandler{svc: s.mkv})
	eveyv1.RegisterMetadataServiceServer(s.server, &metadataHandler{cmd: s.metadataCmd, query: s.metadataQuery})
	eveyv1.RegisterAdminServiceServer(s.server, &adminHandler{
		userCmd:          s.userAuth,
		userQuery:        s.userQuery,
		transcodingSvc:   s.transcoding,
		activitySvc:      s.activityQuery,
		pluginSvc:        s.plugin,
		hlsManager:       s.hlsManager,
		libLister:        s.libQuery,
		scheduler:        s.scheduler,
		libScanner:       s.libCmd,
		libCmd:           s.libCmd,
		metadataCmd:      s.metadataCmd,
		logDir:           s.logDir,
		version:          s.version,
		startTime:        s.startTime,
		ffmpegPath:       s.ffmpegPath,
		heavyOpSemaphore: make(chan struct{}, 1),
	})
	eveyv1.RegisterActivityServiceServer(s.server, &activityHandler{svc: s.activityQuery})
	eveyv1.RegisterNotificationServiceServer(s.server, &notificationHandler{cmd: s.notifCmd, query: s.notifQuery})
	eveyv1.RegisterFeedServiceServer(s.server, &feedHandler{commands: s.feedCmd, queries: s.feedQuery})
	eveyv1.RegisterWebhookServiceServer(s.server, &webhookHandler{svc: s.webhook})
	eveyv1.RegisterCollectionServiceServer(s.server, &collectionHandler{svc: s.collection})
	eveyv1.RegisterPlaylistServiceServer(s.server, &playlistHandler{svc: s.playlist})
	eveyv1.RegisterFavoriteServiceServer(s.server, &favoriteHandler{svc: s.favorite})
	eveyv1.RegisterTranscodeCacheServiceServer(s.server, &transcodeCacheHandler{svc: s.transcoding})
	eveyv1.RegisterBackupServiceServer(s.server, &backupHandler{svc: s.backup})
	eveyv1.RegisterRemoteAccessServiceServer(s.server, &remoteAccessHandler{cfg: s.cfg, store: s.cfgStore})
	eveyv1.RegisterPlaybackSettingsServiceServer(s.server, &playbackSettingsHandler{cfg: s.cfg})
	eveyv1.RegisterLibraryAccessServiceServer(s.server, &libraryAccessHandler{accessRepo: s.libraryAccess, libQuery: s.libQuery})
	if s.serverCfgHndlr != nil {
		eveyv1.RegisterServerConfigServiceServer(s.server, s.serverCfgHndlr)
	}
}

func (s *GRPCServer) Start(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	slog.Info("gRPC server listening", slog.String("addr", addr))
	return s.server.Serve(lis)
}

func (s *GRPCServer) Stop() {
	s.server.GracefulStop()
}
