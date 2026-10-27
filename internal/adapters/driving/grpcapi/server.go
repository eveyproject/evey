package grpcapi

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"time"

	apploadtest "github.com/eveyproject/evey/internal/app/loadtest"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appactivity "github.com/eveyproject/evey/internal/app/activity"
	appbackup "github.com/eveyproject/evey/internal/app/backup"
	appcollection "github.com/eveyproject/evey/internal/app/collection"
	appconfig "github.com/eveyproject/evey/internal/app/config"
	appfavorite "github.com/eveyproject/evey/internal/app/favorite"
	appfeed "github.com/eveyproject/evey/internal/app/feed"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	appmaintenance "github.com/eveyproject/evey/internal/app/maintenance"
	appmedia "github.com/eveyproject/evey/internal/app/media"
	appmediarequest "github.com/eveyproject/evey/internal/app/mediarequest"
	appmetadata "github.com/eveyproject/evey/internal/app/metadata"
	appmkv "github.com/eveyproject/evey/internal/app/mkv"
	appmusic "github.com/eveyproject/evey/internal/app/music"
	appnotification "github.com/eveyproject/evey/internal/app/notification"
	appphoto "github.com/eveyproject/evey/internal/app/photo"
	appplayback "github.com/eveyproject/evey/internal/app/playback"
	appplaylist "github.com/eveyproject/evey/internal/app/playlist"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	apppresence "github.com/eveyproject/evey/internal/app/presence"
	appsyncplay "github.com/eveyproject/evey/internal/app/syncplay"
	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	appuser "github.com/eveyproject/evey/internal/app/user"
	appwebhook "github.com/eveyproject/evey/internal/app/webhook"
	"github.com/eveyproject/evey/internal/config"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/eveyproject/evey/internal/pkg/quickconnecthub"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
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
	PluginToken     *appplugin.TokenService
	// PluginRunner drives the per-plugin resource-limits RPCs (read + apply).
	// Holds *pluginrunner.Runner; nil when the plugin system is disabled.
	PluginRunner           pluginResourceController
	PluginResourceDefaults appplugin.ResourceDefaults
	// Metadata plugins: the fields each may replace and the library walk.
	PluginMetadataSettings *appplugin.MetadataSettingsService
	PluginMetadataRefresh  *appmetadata.PluginTextRefresher
	// PluginData backs the iframe-facing plugin preference RPCs.
	PluginData      plugin.PluginDataRepository
	UserRatings     domainlibrary.UserRatingRepository
	MetadataCmd     *appmetadata.CommandService
	MetadataQuery   *appmetadata.QueryService
	ActivityCmd     *appactivity.CommandService
	ActivityQuery   *appactivity.QueryService
	NotifCmd        *appnotification.CommandService
	NotifQuery      *appnotification.QueryService
	FeedCmd         *appfeed.CommandService
	FeedQuery       *appfeed.QueryService
	Webhook         *appwebhook.Service
	CollectionCmd   *appcollection.CommandService
	CollectionQuery *appcollection.QueryService
	Playlist        *appplaylist.Service
	Favorite        *appfavorite.Service
	MediaRequest    *appmediarequest.Service
	Backup          *appbackup.Service
	BackupScheduler *appbackup.Scheduler
	Cfg             *config.Config
	CfgStore        remoteAccessConfigStore
	Version         string
	// Commit / BuildDate are the link-time build stamps surfaced by
	// GetSystemInfo. Blank is reported as "unknown", never as "".
	Commit    string
	BuildDate string
	// Fields previously set via Set* methods
	HLSVODOrch *apptranscoding.SegmentOrchestrator
	// HLSVODRemover fans RemoveSession across the software AND hardware
	// producers, so an explicit StopStream or admin force-kill tears down a
	// software stream too (the bare orchestrator only knows its HW sessions).
	HLSVODRemover   *apptranscoding.ProducerSelector
	Scheduler       *applibrary.Scheduler
	LogDir          string
	ServerCtx       context.Context
	Translator      *eveyi18n.Translator
	SMTPConfigured  func() bool                  // live: SMTP is editable from the admin panel; nil = off
	ServerCfgSvc    *appconfig.Service           // optional: nil = service not registered
	ShareSvc        *appmedia.ShareService       // optional
	Music           *appmusic.Service            // optional
	MusicNowPlaying *appmusic.NowPlayingRegistry // optional; "En cours de lecture" music sessions
	Photo           *appphoto.Service            // optional
	PushEnabled     bool                         // optional
	VAPIDPubKey     string
	PushRegisterRL  *ratelimit.Limiter
	SyncplayHub     *appsyncplay.Hub // optional
	QuickConnectHub *quickconnecthub.Hub
	IntroCache      IntroCacheInvalidator // optional
	IntroBackfill   IntroBackfiller       // optional
	// SeekIndex backs the admin MKV/WebM seek-index scan/reindex/rollback RPCs.
	// Optional: nil → those RPCs return Unimplemented ("service not configured").
	SeekIndex *appmaintenance.SeekIndexService
	// CalibrationSvc supplies the boot benchmark evidence behind the video
	// transcode capacity reported by GetSystemInfo. Optional (nil = the
	// video_sessions block is omitted).
	CalibrationSvc *apptranscoding.CalibrationService
	// VideoLimiter reports live video-encode session usage. Optional.
	VideoLimiter *apptranscoding.VideoSessionLimiter
	// LoadTest runs the built-in capacity measurement. Optional (nil =
	// Unimplemented). LoadTestBaseURL is the loopback origin viewers hit.
	LoadTest        *apploadtest.Runner
	LoadTestBaseURL string
	// LoadTestPublicURL is the reverse-proxy origin (Caddy) real viewers come
	// through. The runner prefers it when reachable - that is the true delivery
	// path - and falls back to the loopback otherwise.
	LoadTestPublicURL string
	// LoadTestTokens mints the synthetic viewers' credential so it outlives the
	// run. Without it the run borrows the caller's browser token and dies on a
	// 401 mid-ramp, reporting an expiry as if it were a capacity limit.
	LoadTestTokens loadTestTokenMinter

	// StreamMeter exposes the cumulative streaming bytes served (HLS/direct/
	// download) for the dashboard's measured bandwidth. Optional.
	StreamMeter appactivity.BytesTotaler
	// KeyframeOnDemand fills a missing keyframe index inline at GetStreamInfo
	// (bounded fast-path probe), so a freshly added file's FIRST playback can
	// negotiate the copy rung instead of waiting on the paced background
	// backfill. Optional: nil skips the attempt.
	KeyframeOnDemand *apptranscoding.KeyframeOnDemand
	// ProfileHours warns a kids profile's player before its opening ends.
	ProfileHours ProfileHoursReader
	// Presence is the in-memory real-time online-users registry. Optional: nil
	// disables the heartbeat + online-users admin panel.
	Presence *apppresence.Tracker
	// RestartFn triggers a graceful shutdown so a supervisor restarts the
	// process. Optional: nil makes the admin RestartServer RPC return Unavailable.
	RestartFn func()
}

type GRPCServer struct {
	server     *grpc.Server
	deps       GRPCServerDeps
	startTime  time.Time
	ffmpegPath string
	// playback is kept so the composition root can bind the plan's decider
	// (SetPlaybackDecider) once the HTTP pipeline that owns it exists.
	playback *playbackHandler
}

// SetPlaybackDecider binds the decider GetStreamInfo answers with. Called by
// the composition root after the HTTP handler is built and before Start.
func (s *GRPCServer) SetPlaybackDecider(d apptranscoding.PlaybackDecider) {
	if s.playback != nil {
		s.playback.decider = d
	}
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
	// StopStream / admin force-kill (RemoveSession) and the admin active-stream
	// count (ActiveSessionCount) go through the fan-out selector when wired, so
	// they span software + hardware; hand-built test servers that set only
	// HLSVODOrch fall back to it. Genuinely-nil interface when neither is set, so
	// the handlers' `!= nil` guards stay correct.
	var hlsMgr hlsSessionManager
	switch {
	case d.HLSVODRemover != nil:
		hlsMgr = d.HLSVODRemover
	case d.HLSVODOrch != nil:
		hlsMgr = d.HLSVODOrch
	}
	// The shared helpers (requireClaims, resolveProfileID, the media access
	// check) answer in the caller's language too.
	if d.Translator != nil {
		helperTranslator.Store(d.Translator)
	}
	// Operator remote-access bitrate cap, read live from cfg.RemoteAccess (the
	// admin PUT writes it under the same lock). nil cfg (tests) leaves it off.
	playbackRemoteAccess := remoteAccessFromConfig(d.Cfg)
	eveyv1.RegisterLibraryServiceServer(s.server, &libraryHandler{cmd: d.LibCmd, query: d.LibQuery, metadataSvc: d.MetadataQuery, userAccess: d.UserAccess, userQuery: d.UserQuery, scheduler: d.Scheduler, translator: d.Translator})
	eveyv1.RegisterUserServiceServer(s.server, &userHandler{auth: d.UserAuth, userSvc: d.UserSvc, access: d.UserAccess, query: d.UserQuery, activitySvc: d.ActivityCmd, translator: d.Translator, quickConnectHub: d.QuickConnectHub, cfg: d.Cfg})
	s.playback = &playbackHandler{cmd: d.PlaybackCmd, query: d.PlaybackQuery, librarySvc: d.LibQuery, activitySvc: d.ActivityCmd, userQuery: d.UserQuery, userAccess: d.UserAccess, translator: d.Translator, metadataSvc: d.MetadataQuery, hlsManager: hlsMgr, keyframeOnDemand: d.KeyframeOnDemand, remoteAccess: playbackRemoteAccess, hours: d.ProfileHours}
	eveyv1.RegisterPlaybackServiceServer(s.server, s.playback)
	eveyv1.RegisterTranscodingServiceServer(s.server, &transcodingHandler{svc: d.Transcoding, userQuery: d.UserQuery, libQuery: d.LibQuery, userAccess: d.UserAccess, translator: d.Translator})
	eveyv1.RegisterMKVServiceServer(s.server, &mkvHandler{svc: d.Mkv, libQuery: d.LibQuery, userAccess: d.UserAccess, translator: d.Translator})
	eveyv1.RegisterMetadataServiceServer(s.server, &metadataHandler{cmd: d.MetadataCmd, query: d.MetadataQuery, userAccess: d.UserAccess, libQuery: d.LibQuery, userQuery: d.UserQuery, translator: d.Translator})
	srvCtx := d.ServerCtx
	if srvCtx == nil {
		srvCtx = context.Background()
	}
	eveyv1.RegisterAdminSystemServiceServer(s.server, &adminSystemHandler{
		userCmd:           d.UserAuth,
		userQuery:         d.UserQuery,
		transcodingSvc:    d.Transcoding,
		activitySvc:       d.ActivityQuery,
		hlsManager:        hlsMgr,
		libLister:         d.LibQuery,
		streamMeter:       d.StreamMeter,
		version:           d.Version,
		commit:            d.Commit,
		buildDate:         d.BuildDate,
		calibrationSvc:    calibrationProviderOrNil(d.CalibrationSvc),
		videoLimiter:      videoLimiterOrNil(d.VideoLimiter),
		startTime:         s.startTime,
		ffmpegPath:        s.ffmpegPath,
		translator:        d.Translator,
		smtpConfigured:    d.SMTPConfigured,
		serverCfgSvc:      d.ServerCfgSvc,
		cfg:               d.Cfg,
		restartFn:         d.RestartFn,
		loadTest:          loadTestOrNil(d.LoadTest),
		loadTestBaseURL:   d.LoadTestBaseURL,
		loadTestPublicURL: d.LoadTestPublicURL,
		loadTestToken:     d.LoadTestTokens,
	})
	if d.Presence != nil {
		eveyv1.RegisterPresenceServiceServer(s.server, &presenceHandler{tracker: d.Presence, translator: d.Translator})
	}
	var pluginCfgRepo shared.ServerConfigRepository
	if d.ServerCfgSvc != nil { // keep a true-nil interface when the store is absent
		pluginCfgRepo = d.ServerCfgSvc
	}
	adminPlugins := &adminPluginHandler{
		pluginInstaller:  d.PluginInstaller,
		pluginRuntime:    d.PluginRuntime,
		pluginQuery:      d.PluginQuery,
		tokenSvc:         d.PluginToken,
		logDir:           d.LogDir,
		translator:       d.Translator,
		pluginRes:        d.PluginRunner,
		serverCfg:        pluginCfgRepo,
		resDefaults:      d.PluginResourceDefaults,
		enabled:          d.Cfg != nil && d.Cfg.Server.PluginsEnabled,
		metadataSettings: metadataSettingsOrNil(d.PluginMetadataSettings),
		metadataRefresh:  metadataRefreshOrNil(d.PluginMetadataRefresh),
	}
	eveyv1.RegisterAdminPluginServiceServer(s.server, adminPlugins)
	eveyv1.RegisterPluginUserDataServiceServer(s.server, newPluginUserDataHandler(d.PluginData))
	eveyv1.RegisterAdminTaskServiceServer(s.server, &adminTaskHandler{
		scheduler:  d.Scheduler,
		libScanner: d.LibCmd,
		serverCtx:  srvCtx,
		translator: d.Translator,
	})
	eveyv1.RegisterAdminMediaServiceServer(s.server, &adminMediaHandler{
		metadataCmd:      d.MetadataCmd,
		libCmd:           d.LibCmd,
		libQuery:         d.LibQuery,
		seekIndex:        d.SeekIndex,
		heavyOpSemaphore: make(chan struct{}, 1),
		translator:       d.Translator,
		serverCtx:        srvCtx,
		activitySvc:      d.ActivityCmd,
	})
	eveyv1.RegisterAdminSessionServiceServer(s.server, &adminSessionHandler{
		activitySvc: d.ActivityQuery,
		playbackCmd: d.PlaybackCmd,
		playbackQry: d.PlaybackQuery,
		hlsManager:  hlsMgr,
		musicNow:    d.MusicNowPlaying,
		musicSvc:    d.Music,
		userQuery:   d.UserQuery,
		translator:  d.Translator,
	})
	eveyv1.RegisterActivityServiceServer(s.server, &activityHandler{svc: d.ActivityQuery, cmd: d.ActivityCmd, libQuery: d.LibQuery, userAccess: d.UserAccess, userQuery: d.UserQuery, translator: d.Translator})
	eveyv1.RegisterNotificationServiceServer(s.server, &notificationHandler{cmd: d.NotifCmd, query: d.NotifQuery, userAccess: d.UserAccess, libQuery: d.LibQuery, translator: d.Translator})
	eveyv1.RegisterFeedServiceServer(s.server, &feedHandler{commands: d.FeedCmd, queries: d.FeedQuery, userAccess: d.UserAccess, libQuery: d.LibQuery, userQuery: d.UserQuery, translator: d.Translator})
	eveyv1.RegisterWebhookServiceServer(s.server, &webhookHandler{svc: d.Webhook, translator: d.Translator})
	eveyv1.RegisterCollectionServiceServer(s.server, &collectionHandler{cmd: d.CollectionCmd, query: d.CollectionQuery, userAccess: d.UserAccess, libQuery: d.LibQuery, userQuery: d.UserQuery, translator: d.Translator})
	eveyv1.RegisterPlaylistServiceServer(s.server, &playlistHandler{svc: d.Playlist, libQuery: d.LibQuery, userAccess: d.UserAccess, userQuery: d.UserQuery, translator: d.Translator})
	eveyv1.RegisterFavoriteServiceServer(s.server, &favoriteHandler{svc: d.Favorite, libQuery: d.LibQuery, userAccess: d.UserAccess, userQuery: d.UserQuery})
	eveyv1.RegisterMediaRequestServiceServer(s.server, &mediaRequestHandler{svc: d.MediaRequest, libQuery: d.LibQuery, userAccess: d.UserAccess, userQuery: d.UserQuery, translator: d.Translator})
	eveyv1.RegisterAdminMediaRequestServiceServer(s.server, &adminMediaRequestHandler{svc: d.MediaRequest, userQuery: d.UserQuery, translator: d.Translator})
	eveyv1.RegisterTranscodeCacheServiceServer(s.server, &transcodeCacheHandler{svc: d.Transcoding, translator: d.Translator})
	eveyv1.RegisterBackupServiceServer(s.server, &backupHandler{svc: d.Backup})
	if d.BackupScheduler != nil {
		eveyv1.RegisterBackupScheduleServiceServer(s.server, &backupScheduleHandler{scheduler: d.BackupScheduler, translator: d.Translator})
	}
	eveyv1.RegisterRemoteAccessServiceServer(s.server, &remoteAccessHandler{cfg: d.Cfg, store: d.CfgStore, translator: d.Translator})
	pbh := &playbackSettingsHandler{cfg: d.Cfg, configSvc: d.ServerCfgSvc, translator: d.Translator}
	pbh.initFromDB(srvCtx) // srvCtx is nil-guarded (Background fallback); d.ServerCtx may be nil
	eveyv1.RegisterPlaybackSettingsServiceServer(s.server, pbh)
	eveyv1.RegisterLibraryAccessServiceServer(s.server, &libraryAccessHandler{userAccess: d.UserAccess, libQuery: d.LibQuery})
	if d.ServerCfgSvc != nil {
		h := NewServerConfigHandler(d.ServerCfgSvc)
		if d.Translator != nil {
			h.SetTranslator(d.Translator)
		}
		eveyv1.RegisterServerConfigServiceServer(s.server, h)
	}
	if d.ShareSvc != nil {
		eveyv1.RegisterShareServiceServer(s.server, &shareHandler{svc: d.ShareSvc, libQuery: d.LibQuery, userAccess: d.UserAccess, translator: d.Translator})
	}
	if d.SyncplayHub != nil {
		eveyv1.RegisterSyncPlayServiceServer(s.server, &syncplayHandler{hub: d.SyncplayHub, libQuery: d.LibQuery, userAccess: d.UserAccess, userQuery: d.UserQuery, musicSvc: d.Music, playlistSvc: d.Playlist, translator: d.Translator})
	}
	if d.Music != nil {
		eveyv1.RegisterMusicServiceServer(s.server, &musicHandler{svc: d.Music, userAccess: d.UserAccess, libQuery: d.LibQuery, serverCtx: srvCtx, translator: d.Translator})
	}
	if d.Photo != nil {
		eveyv1.RegisterPhotoServiceServer(s.server, &photoHandler{svc: d.Photo, userAccess: d.UserAccess, libQuery: d.LibQuery, translator: d.Translator})
	}
	if d.PushEnabled {
		eveyv1.RegisterPushServiceServer(s.server, &webPushHandler{notifCmd: d.NotifCmd, notifQuery: d.NotifQuery, vapidPubKey: d.VAPIDPubKey, registerRL: d.PushRegisterRL, translator: d.Translator})
	}
	eveyv1.RegisterRatingServiceServer(s.server, &ratingHandler{thumbs: d.UserRatings, libQuery: d.LibQuery, userAccess: d.UserAccess, translator: d.Translator})
	if d.IntroCache != nil || d.IntroBackfill != nil {
		eveyv1.RegisterAdminIntroServiceServer(s.server, &adminIntroHandler{
			cache:      d.IntroCache,
			backfill:   d.IntroBackfill,
			translator: d.Translator,
		})
	}
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

// calibrationProviderOrNil / videoLimiterOrNil keep a TRUE-nil interface when
// the concrete pointer is absent: assigning a typed nil pointer to an
// interface yields a non-nil interface holding a nil value, and the handler's
// `if h.calibrationSvc != nil` guard would then dereference it.
func calibrationProviderOrNil(s *apptranscoding.CalibrationService) calibrationSnapshotProvider {
	if s == nil {
		return nil
	}
	return s
}

func videoLimiterOrNil(l *apptranscoding.VideoSessionLimiter) videoSessionActiveReporter {
	if l == nil {
		return nil
	}
	return l
}

// loadTestOrNil keeps a TRUE-nil interface when no runner is wired, so the
// handler's nil-check answers Unimplemented instead of panicking.
func loadTestOrNil(r *apploadtest.Runner) loadTestRunner {
	if r == nil {
		return nil
	}
	return r
}

// remoteAccessFromConfig reads the remote ceiling and the LAN list live, for
// Stream info. The ceiling is the same combined one the served master applies
// (EffectiveMaxVideoBitrateRemoteKbps), or Stream info would announce a cap
// the stream does not honour. nil cfg (tests) leaves it off.
func remoteAccessFromConfig(cfg *config.Config) func() (int, []string) {
	if cfg == nil {
		return nil
	}
	return func() (maxKbps int, lan []string) {
		cfg.WithReadLock(func() {
			maxKbps = config.RemoteBitrateCapKbps(cfg.Transcoding.MaxVideoBitrateRemoteKbps, cfg.RemoteAccess.MaxRemoteBitrateKbps)
			lan = slices.Clone(cfg.RemoteAccess.LANNetworks)
		})
		return maxKbps, lan
	}
}
