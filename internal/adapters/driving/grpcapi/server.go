package grpcapi

import (
	"context"
	"fmt"
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
	appmedia "github.com/gravity-zero/evey/internal/app/media"
	appmusic "github.com/gravity-zero/evey/internal/app/music"
	appphoto "github.com/gravity-zero/evey/internal/app/photo"
	apprating "github.com/gravity-zero/evey/internal/app/rating"
	appsyncplay "github.com/gravity-zero/evey/internal/app/syncplay"
	appwebhook "github.com/gravity-zero/evey/internal/app/webhook"
	"github.com/gravity-zero/evey/internal/config"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/notification"
	"github.com/gravity-zero/evey/internal/domain/user"
	eveyi18n "github.com/gravity-zero/evey/internal/pkg/i18n"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
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
	pluginInstaller *appplugin.InstallerService
	pluginRuntime   *appplugin.RuntimeService
	pluginQuery     *appplugin.QueryService
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
	translator      *eveyi18n.Translator
	serverCtx       context.Context
	smtpConfigured  bool
	shareSvc        *appmedia.ShareService
	bioEnricher     appmetadata.PeopleBioEnricher
	music           *appmusic.Service
	photo           *appphoto.Service
	pushSubs        notification.PushSubscriptionRepository
	vapidPubKey     string
	pushRegisterRL  *ratelimit.Limiter
	syncplayHub     *appsyncplay.Hub
	thumbRating     domainlibrary.UserRatingRepository
	starRating      *apprating.Service
}

func (s *GRPCServer) SetServerConfigHandler(h *serverConfigHandler) {
	if s.translator != nil {
		h.SetTranslator(s.translator)
	}
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

func (s *GRPCServer) SetServerCtx(ctx context.Context) {
	s.serverCtx = ctx
}

func (s *GRPCServer) SetTranslator(t *eveyi18n.Translator) {
	s.translator = t
}

func (s *GRPCServer) SetSMTPConfigured(v bool) { s.smtpConfigured = v }
func (s *GRPCServer) SetShareService(svc *appmedia.ShareService) { s.shareSvc = svc }
func (s *GRPCServer) SetBioEnricher(e appmetadata.PeopleBioEnricher) { s.bioEnricher = e }
func (s *GRPCServer) SetMusicService(svc *appmusic.Service) { s.music = svc }
func (s *GRPCServer) SetPhotoService(svc *appphoto.Service) { s.photo = svc }
func (s *GRPCServer) SetSyncPlayHub(hub *appsyncplay.Hub) { s.syncplayHub = hub }
func (s *GRPCServer) SetPushDeps(subs notification.PushSubscriptionRepository, vapidKey string, rl *ratelimit.Limiter) {
	s.pushSubs = subs; s.vapidPubKey = vapidKey; s.pushRegisterRL = rl
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
	pluginInstaller *appplugin.InstallerService,
	pluginRuntime *appplugin.RuntimeService,
	pluginQuery *appplugin.QueryService,
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
		mkv:             mkv,
		pluginInstaller: pluginInstaller,
		pluginRuntime:   pluginRuntime,
		pluginQuery:     pluginQuery,
		metadataCmd:     metadataCmd,
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
	eveyv1.RegisterLibraryServiceServer(s.server, &libraryHandler{cmd: s.libCmd, query: s.libQuery, metadataSvc: s.metadataQuery, libraryAccess: s.libraryAccess, profiles: s.profiles, translator: s.translator})
	eveyv1.RegisterUserServiceServer(s.server, &userHandler{auth: s.userAuth, userSvc: s.userSvc, access: s.userAccess, query: s.userQuery, users: s.users, activitySvc: s.activityCmd, translator: s.translator})
	eveyv1.RegisterPlaybackServiceServer(s.server, &playbackHandler{cmd: s.playbackCmd, query: s.playbackQuery, librarySvc: s.libQuery, activitySvc: s.activityCmd, profiles: s.profiles, libraryAccess: s.libraryAccess, translator: s.translator, hlsManager: s.hlsManager})
	eveyv1.RegisterTranscodingServiceServer(s.server, &transcodingHandler{svc: s.transcoding, profiles: s.profiles})
	eveyv1.RegisterMKVServiceServer(s.server, &mkvHandler{svc: s.mkv})
	eveyv1.RegisterMetadataServiceServer(s.server, &metadataHandler{cmd: s.metadataCmd, query: s.metadataQuery})
	srvCtx := s.serverCtx
	if srvCtx == nil {
		srvCtx = context.Background()
	}
	eveyv1.RegisterAdminSystemServiceServer(s.server, &adminSystemHandler{
		userCmd:        s.userAuth,
		userQuery:      s.userQuery,
		transcodingSvc: s.transcoding,
		activitySvc:    s.activityQuery,
		hlsManager:     s.hlsManager,
		libLister:      s.libQuery,
		version:        s.version,
		startTime:      s.startTime,
		ffmpegPath:     s.ffmpegPath,
		translator:     s.translator,
		smtpConfigured: s.smtpConfigured,
	})
	eveyv1.RegisterAdminPluginServiceServer(s.server, &adminPluginHandler{
		pluginInstaller: s.pluginInstaller,
		pluginRuntime:   s.pluginRuntime,
		pluginQuery:     s.pluginQuery,
		logDir:          s.logDir,
	})
	eveyv1.RegisterAdminTaskServiceServer(s.server, &adminTaskHandler{
		scheduler:  s.scheduler,
		libScanner: s.libCmd,
		serverCtx:  srvCtx,
		translator: s.translator,
	})
	eveyv1.RegisterAdminMediaServiceServer(s.server, &adminMediaHandler{
		metadataCmd:      s.metadataCmd,
		libCmd:           s.libCmd,
		heavyOpSemaphore: make(chan struct{}, 1),
		translator:       s.translator,
	})
	eveyv1.RegisterAdminSessionServiceServer(s.server, &adminSessionHandler{
		activitySvc: s.activityQuery,
	})
	eveyv1.RegisterActivityServiceServer(s.server, &activityHandler{svc: s.activityQuery, translator: s.translator})
	eveyv1.RegisterNotificationServiceServer(s.server, &notificationHandler{cmd: s.notifCmd, query: s.notifQuery})
	eveyv1.RegisterFeedServiceServer(s.server, &feedHandler{commands: s.feedCmd, queries: s.feedQuery})
	eveyv1.RegisterWebhookServiceServer(s.server, &webhookHandler{svc: s.webhook})
	eveyv1.RegisterCollectionServiceServer(s.server, &collectionHandler{svc: s.collection})
	eveyv1.RegisterPlaylistServiceServer(s.server, &playlistHandler{svc: s.playlist, translator: s.translator})
	eveyv1.RegisterFavoriteServiceServer(s.server, &favoriteHandler{svc: s.favorite})
	eveyv1.RegisterTranscodeCacheServiceServer(s.server, &transcodeCacheHandler{svc: s.transcoding})
	eveyv1.RegisterBackupServiceServer(s.server, &backupHandler{svc: s.backup})
	eveyv1.RegisterRemoteAccessServiceServer(s.server, &remoteAccessHandler{cfg: s.cfg, store: s.cfgStore})
	eveyv1.RegisterPlaybackSettingsServiceServer(s.server, &playbackSettingsHandler{cfg: s.cfg, translator: s.translator})
	eveyv1.RegisterLibraryAccessServiceServer(s.server, &libraryAccessHandler{accessRepo: s.libraryAccess, libQuery: s.libQuery})
	if s.serverCfgHndlr != nil {
		eveyv1.RegisterServerConfigServiceServer(s.server, s.serverCfgHndlr)
	}
	if s.shareSvc != nil {
		eveyv1.RegisterShareServiceServer(s.server, &shareHandler{svc: s.shareSvc, libQuery: s.libQuery, libraryAccess: s.libraryAccess})
	}
	if s.syncplayHub != nil {
		eveyv1.RegisterSyncPlayServiceServer(s.server, &syncplayHandler{hub: s.syncplayHub, libQuery: s.libQuery, libraryAccess: s.libraryAccess})
	}
	if s.music != nil {
		eveyv1.RegisterMusicServiceServer(s.server, &musicHandler{svc: s.music, libQuery: s.libQuery, libraryAccess: s.libraryAccess})
	}
	if s.photo != nil {
		eveyv1.RegisterPhotoServiceServer(s.server, &photoHandler{svc: s.photo, libQuery: s.libQuery, libraryAccess: s.libraryAccess})
	}
	if s.pushSubs != nil {
		eveyv1.RegisterPushServiceServer(s.server, &webPushHandler{subs: s.pushSubs, profiles: s.profiles, users: s.users, vapidPubKey: s.vapidPubKey})
	}
	eveyv1.RegisterRatingServiceServer(s.server, &ratingHandler{thumbRepo: s.thumbRating, starSvc: s.starRating, libQuery: s.libQuery, libraryAccess: s.libraryAccess})
}

func (s *GRPCServer) Start(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("GRPCServer.Start: %w", err)
	}
	slog.Info("gRPC server listening", slog.String("addr", addr))
	return s.server.Serve(lis)
}

func (s *GRPCServer) Stop() {
	s.server.GracefulStop()
}
