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
	appmedia "github.com/gravity-zero/evey/internal/app/media"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	appmkv "github.com/gravity-zero/evey/internal/app/mkv"
	appmusic "github.com/gravity-zero/evey/internal/app/music"
	appnotification "github.com/gravity-zero/evey/internal/app/notification"
	appphoto "github.com/gravity-zero/evey/internal/app/photo"
	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	appplaylist "github.com/gravity-zero/evey/internal/app/playlist"
	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	apprating "github.com/gravity-zero/evey/internal/app/rating"
	appsyncplay "github.com/gravity-zero/evey/internal/app/syncplay"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	appwebhook "github.com/gravity-zero/evey/internal/app/webhook"
	"github.com/gravity-zero/evey/internal/config"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/notification"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/user"
	eveyi18n "github.com/gravity-zero/evey/internal/pkg/i18n"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
	"google.golang.org/grpc"
)

type GRPCServerDeps struct {
	LibCmd          *applibrary.CommandService
	LibQuery        *applibrary.QueryService
	UserAuth        *appuser.CommandService
	UserSvc         *appuser.UserService
	UserAccess      *appuser.AccessService
	UserQuery       *appuser.QueryService
	PlaybackCmd     *appplayback.CommandService
	PlaybackQuery   *appplayback.QueryService
	Transcoding     *apptranscoding.Service
	Mkv             *appmkv.Service
	PluginInstaller *appplugin.InstallerService
	PluginRuntime   *appplugin.RuntimeService
	PluginQuery     *appplugin.QueryService
	MetadataCmd     *appmetadata.CommandService
	MetadataQuery   *appmetadata.QueryService
	ActivityCmd     *appactivity.CommandService
	ActivityQuery   *appactivity.QueryService
	NotifCmd        *appnotification.CommandService
	NotifQuery      *appnotification.QueryService
	FeedCmd         *appfeed.CommandService
	FeedQuery       *appfeed.QueryService
	Webhook         *appwebhook.Service
	Collection      *appcollection.Service
	Playlist        *appplaylist.Service
	Favorite        *appfavorite.Service
	Users           user.UserRepository
	Profiles        user.ProfileRepository
	LibraryAccess   user.LibraryAccessRepository
	Backup          *appbackup.Service
	Cfg             *config.Config
	CfgStore        remoteAccessConfigStore
	Version         string
	// Fields previously set via Set* methods
	HLSManager     *apptranscoding.HLSManager
	Scheduler      *applibrary.Scheduler
	LogDir         string
	ServerCtx      context.Context
	Translator     *eveyi18n.Translator
	SMTPConfigured bool
	ServerCfgRepo  shared.ServerConfigRepository // optional: nil = service not registered
	ShareSvc       *appmedia.ShareService   // optional
	BioEnricher    appmetadata.PeopleBioEnricher // optional
	Music          *appmusic.Service    // optional
	Photo          *appphoto.Service    // optional
	PushSubs       notification.PushSubscriptionRepository // optional
	VAPIDPubKey    string
	PushRegisterRL *ratelimit.Limiter
	SyncplayHub    *appsyncplay.Hub     // optional
	ThumbRating    domainlibrary.UserRatingRepository
	StarRating     *apprating.Service
}

type GRPCServer struct {
	server    *grpc.Server
	deps      GRPCServerDeps
	startTime time.Time
	ffmpegPath string
}

func NewGRPCServer(deps GRPCServerDeps, opts ...grpc.ServerOption) *GRPCServer {
	return &GRPCServer{
		server:     grpc.NewServer(opts...),
		deps:       deps,
		startTime:  time.Now(),
		ffmpegPath: "ffmpeg",
	}
}

func (s *GRPCServer) RegisterServices() {
	d := &s.deps
	eveyv1.RegisterLibraryServiceServer(s.server, &libraryHandler{cmd: d.LibCmd, query: d.LibQuery, metadataSvc: d.MetadataQuery, libraryAccess: d.LibraryAccess, profiles: d.Profiles, translator: d.Translator})
	eveyv1.RegisterUserServiceServer(s.server, &userHandler{auth: d.UserAuth, userSvc: d.UserSvc, access: d.UserAccess, query: d.UserQuery, users: d.Users, activitySvc: d.ActivityCmd, translator: d.Translator})
	eveyv1.RegisterPlaybackServiceServer(s.server, &playbackHandler{cmd: d.PlaybackCmd, query: d.PlaybackQuery, librarySvc: d.LibQuery, activitySvc: d.ActivityCmd, profiles: d.Profiles, libraryAccess: d.LibraryAccess, translator: d.Translator, hlsManager: d.HLSManager})
	eveyv1.RegisterTranscodingServiceServer(s.server, &transcodingHandler{svc: d.Transcoding, profiles: d.Profiles})
	eveyv1.RegisterMKVServiceServer(s.server, &mkvHandler{svc: d.Mkv})
	eveyv1.RegisterMetadataServiceServer(s.server, &metadataHandler{cmd: d.MetadataCmd, query: d.MetadataQuery})
	srvCtx := d.ServerCtx
	if srvCtx == nil {
		srvCtx = context.Background()
	}
	eveyv1.RegisterAdminSystemServiceServer(s.server, &adminSystemHandler{
		userCmd:        d.UserAuth,
		userQuery:      d.UserQuery,
		transcodingSvc: d.Transcoding,
		activitySvc:    d.ActivityQuery,
		hlsManager:     d.HLSManager,
		libLister:      d.LibQuery,
		version:        d.Version,
		startTime:      s.startTime,
		ffmpegPath:     s.ffmpegPath,
		translator:     d.Translator,
		smtpConfigured: d.SMTPConfigured,
	})
	eveyv1.RegisterAdminPluginServiceServer(s.server, &adminPluginHandler{
		pluginInstaller: d.PluginInstaller,
		pluginRuntime:   d.PluginRuntime,
		pluginQuery:     d.PluginQuery,
		logDir:          d.LogDir,
	})
	eveyv1.RegisterAdminTaskServiceServer(s.server, &adminTaskHandler{
		scheduler:  d.Scheduler,
		libScanner: d.LibCmd,
		serverCtx:  srvCtx,
		translator: d.Translator,
	})
	eveyv1.RegisterAdminMediaServiceServer(s.server, &adminMediaHandler{
		metadataCmd:      d.MetadataCmd,
		libCmd:           d.LibCmd,
		heavyOpSemaphore: make(chan struct{}, 1),
		translator:       d.Translator,
	})
	eveyv1.RegisterAdminSessionServiceServer(s.server, &adminSessionHandler{
		activitySvc: d.ActivityQuery,
		playbackCmd: d.PlaybackCmd,
		playbackQry: d.PlaybackQuery,
	})
	eveyv1.RegisterActivityServiceServer(s.server, &activityHandler{svc: d.ActivityQuery, translator: d.Translator})
	eveyv1.RegisterNotificationServiceServer(s.server, &notificationHandler{cmd: d.NotifCmd, query: d.NotifQuery})
	eveyv1.RegisterFeedServiceServer(s.server, &feedHandler{commands: d.FeedCmd, queries: d.FeedQuery})
	eveyv1.RegisterWebhookServiceServer(s.server, &webhookHandler{svc: d.Webhook})
	eveyv1.RegisterCollectionServiceServer(s.server, &collectionHandler{svc: d.Collection})
	eveyv1.RegisterPlaylistServiceServer(s.server, &playlistHandler{svc: d.Playlist, translator: d.Translator})
	eveyv1.RegisterFavoriteServiceServer(s.server, &favoriteHandler{svc: d.Favorite})
	eveyv1.RegisterTranscodeCacheServiceServer(s.server, &transcodeCacheHandler{svc: d.Transcoding})
	eveyv1.RegisterBackupServiceServer(s.server, &backupHandler{svc: d.Backup})
	eveyv1.RegisterRemoteAccessServiceServer(s.server, &remoteAccessHandler{cfg: d.Cfg, store: d.CfgStore})
	eveyv1.RegisterPlaybackSettingsServiceServer(s.server, &playbackSettingsHandler{cfg: d.Cfg, translator: d.Translator})
	eveyv1.RegisterLibraryAccessServiceServer(s.server, &libraryAccessHandler{accessRepo: d.LibraryAccess, libQuery: d.LibQuery})
	if d.ServerCfgRepo != nil {
		h := NewServerConfigHandler(d.ServerCfgRepo)
		if d.Translator != nil {
			h.SetTranslator(d.Translator)
		}
		eveyv1.RegisterServerConfigServiceServer(s.server, h)
	}
	if d.ShareSvc != nil {
		eveyv1.RegisterShareServiceServer(s.server, &shareHandler{svc: d.ShareSvc, libQuery: d.LibQuery, libraryAccess: d.LibraryAccess})
	}
	if d.SyncplayHub != nil {
		eveyv1.RegisterSyncPlayServiceServer(s.server, &syncplayHandler{hub: d.SyncplayHub, libQuery: d.LibQuery, libraryAccess: d.LibraryAccess})
	}
	if d.Music != nil {
		eveyv1.RegisterMusicServiceServer(s.server, &musicHandler{svc: d.Music, libQuery: d.LibQuery, libraryAccess: d.LibraryAccess})
	}
	if d.Photo != nil {
		eveyv1.RegisterPhotoServiceServer(s.server, &photoHandler{svc: d.Photo, libQuery: d.LibQuery, libraryAccess: d.LibraryAccess})
	}
	if d.PushSubs != nil {
		eveyv1.RegisterPushServiceServer(s.server, &webPushHandler{subs: d.PushSubs, profiles: d.Profiles, users: d.Users, vapidPubKey: d.VAPIDPubKey})
	}
	eveyv1.RegisterRatingServiceServer(s.server, &ratingHandler{thumbRepo: d.ThumbRating, starSvc: d.StarRating, libQuery: d.LibQuery, libraryAccess: d.LibraryAccess})
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
