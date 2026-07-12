package main

import (
	"context"
	"log/slog"
	"os"
	"syscall"

	"google.golang.org/grpc"

	"github.com/evey-project/evey/internal/adapters/driven/ffmpeg"
	"github.com/evey-project/evey/internal/adapters/driven/mkvgo"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	apppresence "github.com/evey-project/evey/internal/app/presence"
	"github.com/evey-project/evey/internal/config"
	eveylogger "github.com/evey-project/evey/internal/pkg/logger"
	"github.com/evey-project/evey/internal/pkg/quickconnecthub"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

// selfRestartFn returns a graceful-restart trigger, or nil when the process is
// not supervised. Restart = send SIGTERM to self so the existing shutdown drain
// runs and a supervisor brings us back. We only wire it when running as PID 1
// (Docker `exec /evey` / `restart: unless-stopped`); under dev (Air is PID 1)
// or a bare run, exiting would just stop the server, so the RPC stays disabled.
func selfRestartFn() func() {
	if os.Getpid() != 1 {
		return nil
	}
	return func() {
		slog.Warn("admin restart: sending SIGTERM to self for graceful shutdown")
		if p, err := os.FindProcess(os.Getpid()); err == nil {
			if sigErr := p.Signal(syscall.SIGTERM); sigErr != nil {
				slog.Error("admin restart: SIGTERM self failed", slog.Any("err", sigErr))
			}
		}
	}
}

func buildGRPCServer(deps *appDeps, cfg *config.Config, rootLog *slog.Logger, logDir string, serverCtx context.Context, qcHub *quickconnecthub.Hub) *grpcapi.GRPCServer {
	authInterceptor := interceptors.NewAuthInterceptor(deps.auth.userTokenGen).WithAPIKeys(deps.repos.apiKeyRepo, deps.repos.userRepo).WithTranslator(deps.translator)
	pluginTokenInterceptor := interceptors.NewPluginTokenInterceptor(deps.auth.tokenGen)
	publicRL := interceptors.DefaultPublicMethodLimiters()
	if cfg.Log.DevMode {
		for _, ml := range publicRL {
			ml.Limiter = ratelimit.NewWithBurst(10000, 10000)
			ml.RetryAfterSecs = 0
		}
	}
	grpcLog := eveylogger.NewWith(rootLog, "grpc")

	// Admin MKV/WebM seek-index maintenance (scan/reindex/rollback). The mkvgo
	// adapter is stateless, and the catalog paginates the library to feed every
	// file to the scanner. The ffprobe audio-delay prober adds the "audio starts
	// after video" diagnosis (repack mux defect → constant A/V desync).
	seekIndexSvc := appmaintenance.NewSeekIndexService(
		mkvgo.New(),
		seekIndexCatalog{lib: deps.library.querySvc},
		eveylogger.NewWith(rootLog, "seek-index"),
	).WithFileStateRefresher(deps.seekIndexFileState).
		WithAudioDelayProber(ffmpeg.NewProber("ffprobe")).
		// Audio start-delay repair: mkvgo per-track retime (fixes multi-track
		// files whose tracks disagree on the delay - the dominant real case),
		// with the ffmpeg uniform-shift remux as the fallback for layouts the
		// retime refuses (unknown-size/streamed segments).
		WithAudioRemuxer(mkvgo.NewAudioRetimer(ffmpeg.NewAudioRemuxer("ffmpeg", "ffprobe"))).
		// Scans survive restarts and batch jobs reuse them as phase 1; a job
		// killed by a redeploy resurfaces as "interrupted" instead of
		// vanishing (the operator used to believe the batch had finished).
		WithStateDir(cfg.Library.SeekIndexStateDir)

	grpcDeps := grpcapi.GRPCServerDeps{
		LibCmd:                 deps.library.cmdSvc,
		LibQuery:               deps.library.querySvc,
		UserAuth:               deps.user.authSvc,
		UserSvc:                deps.user.userSvc,
		UserAccess:             deps.user.accessSvc,
		UserQuery:              deps.user.querySvc,
		PlaybackCmd:            deps.media.playbackSvc.CommandService(),
		PlaybackQuery:          deps.media.playbackSvc.QueryService(),
		Transcoding:            deps.media.transcodingSvc,
		Mkv:                    deps.media.mkvSvc,
		PluginInstaller:        deps.engagement.pluginInstallerSvc,
		PluginRuntime:          deps.engagement.pluginRuntimeSvc,
		PluginQuery:            deps.engagement.pluginQuerySvc,
		PluginToken:            deps.engagement.pluginTokenSvc,
		PluginRunner:           deps.pluginRunner,
		PluginResourceDefaults: deps.pluginResDefaults,
		PluginData:             deps.repos.pluginDataRepo,
		MetadataCmd:            deps.content.metadataCmdSvc,
		MetadataQuery:          deps.content.metadataQuerySvc,
		ActivityCmd:            deps.engagement.activityCmdSvc,
		ActivityQuery:          deps.engagement.activityQuerySvc,
		NotifCmd:               deps.engagement.notifCmdSvc,
		NotifQuery:             deps.engagement.notifQuerySvc,
		FeedCmd:                deps.engagement.feedCmdSvc,
		FeedQuery:              deps.engagement.feedQuerySvc,
		Webhook:                deps.engagement.webhookSvc,
		Collection:             deps.content.collectionSvc,
		Playlist:               deps.content.playlistSvc,
		Favorite:               deps.content.favoriteSvc,
		Backup:                 deps.backupSvc,
		BackupScheduler:        deps.backupScheduler,
		Cfg:                    cfg,
		CfgStore:               deps.cfgStore,
		Version:                version,
		HLSVODOrch:             deps.media.hlsVODOrch,
		HLSVODRemover:          deps.media.hlsVODRemover,
		Scheduler:              deps.library.scheduler,
		LogDir:                 logDir,
		ServerCtx:              serverCtx,
		Translator:             deps.translator,
		SMTPConfigured:         cfg.SMTP.Host != "",
		ServerCfgSvc:           deps.serverCfgSvc,
		ShareSvc:               deps.media.shareSvc,
		BioEnricher:            deps.content.enricher,
		Music:                  deps.media.musicSvc,
		Photo:                  deps.media.photoSvc,
		SyncplayHub:            deps.media.syncPlayHub,
		StarRating:             deps.content.starRatingSvc,
		QuickConnectHub:        qcHub,
		IntroCache:             deps.repos.introMetaCache,
		IntroBackfill:          deps.library.introDetectionSvc,
		SeekIndex:              seekIndexSvc,
		KeyframeOnDemand:       deps.keyframeOnDemand,
		StreamMeter:            deps.engagement.streamMeter,
		CalibrationSvc:         deps.media.calibrationSvc,
		VideoLimiter:           deps.media.videoLimiter,
		LoadTest:               deps.loadTestRunner,
		LoadTestBaseURL:        deps.loadTestBaseURL,
		Presence:               apppresence.NewTracker(),
		RestartFn:              selfRestartFn(),
	}

	if deps.repos.pushSubRepo != nil {
		grpcDeps.PushEnabled = true
		grpcDeps.VAPIDPubKey = resolveVAPIDPublicKey(deps.repos.serverCfgRepo, cfg)
		grpcDeps.PushRegisterRL = ratelimit.New(10)
	}

	grpcSrv := grpcapi.NewGRPCServer(grpcDeps,
		grpc.MaxRecvMsgSize(16*1024*1024),
		grpc.ChainUnaryInterceptor(
			interceptors.RecoveryUnary(),
			interceptors.ErrorSanitizerUnary(),
			interceptors.LocaleUnary(),
			interceptors.ClientInfoUnary(),
			interceptors.RateLimitUnary(publicRL),
			interceptors.LoggingUnary(grpcLog),
			pluginTokenInterceptor.Unary(),
			authInterceptor.Unary(),
		),
		grpc.ChainStreamInterceptor(
			interceptors.RecoveryStream(),
			interceptors.ErrorSanitizerStream(),
			interceptors.LocaleStream(),
			interceptors.ClientInfoStream(),
			interceptors.RateLimitStream(publicRL),
			interceptors.LoggingStream(grpcLog),
			pluginTokenInterceptor.Stream(),
			authInterceptor.Stream(),
		),
	)
	grpcSrv.RegisterServices()
	return grpcSrv
}
