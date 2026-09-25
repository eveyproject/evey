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
	"github.com/evey-project/evey/internal/config"
	eveylogger "github.com/evey-project/evey/internal/pkg/logger"
	"github.com/evey-project/evey/internal/pkg/quickconnecthub"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

// selfRestartFn returns a graceful-restart trigger, or nil when nothing would
// bring the server back. Restart = mark the intent, then SIGTERM ourselves so
// the ordinary shutdown drain runs; main then exits with restartExitCode and
// docker/entrypoint.sh relaunches us.
//
// It used to key on `getpid() == 1`, which was a proxy for "am I supervised?"
// and got the answer exactly backwards. The entrypoint exec'd evey into PID 1
// only WITHOUT an embedded database; with one - the single-file install the
// project recommends - it stayed PID 1 itself and ran evey as a child, so the
// admin panel answered "server restart is not available in this deployment" on
// precisely the deployment shape we ship (reported in prod 2026-09-04).
//
// The entrypoint now supervises in every case and says so with EVEY_SUPERVISED,
// which is a fact about the launcher rather than a guess from the process tree.
// Nothing inside a container can read Docker's `restart:` policy, so a restart
// that relies on the container dying can only be hoped for; one the entrypoint
// performs itself can be verified. A bare binary, or any other launcher, leaves
// the variable unset and the RPC correctly reports itself unavailable rather
// than exiting into nothing.
func selfRestartFn() func() {
	if os.Getenv("EVEY_SUPERVISED") != "1" {
		return nil
	}
	return func() {
		slog.Warn("admin restart: sending SIGTERM to self for graceful shutdown")
		restartRequested.Store(true)
		if p, err := os.FindProcess(os.Getpid()); err == nil {
			if sigErr := p.Signal(syscall.SIGTERM); sigErr != nil {
				// Fired by time.AfterFunc after the grace delay, detached from
				// the RPC that requested the restart - no request ctx survives.
				slog.Error("admin restart: SIGTERM self failed", slog.Any("err", sigErr))
				restartRequested.Store(false)
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
	// file to the scanner. mkvgo's Diagnose is one head-mostly read per file that
	// subsumes the index + audio-delay probes and additionally classifies
	// corruption as truncated (re-acquire the source) vs damaged (resync,
	// N bytes lost) - the signal that routes each file's repair. No ffprobe.
	seekIndexSvc := appmaintenance.NewSeekIndexService(
		mkvgo.New(),
		seekIndexCatalog{lib: deps.library.querySvc},
		eveylogger.NewWith(rootLog, "seek-index"),
	).WithFileStateRefresher(deps.seekIndexFileState).
		WithDiagnoser(mkvgo.New()).
		// Every rewrite leaves an audit_log entry: what the file was, what was
		// done, on what evidence, and what it became. A repair nobody recorded
		// is a repair nobody can undo - 98 files learned that the hard way.
		WithRepairJournal(deps.repos.repairJournal).
		// The way back, without keeping a copy of every repaired file: the
		// journal holds the shift that was applied, and this replays its
		// opposite - losslessly, whatever the codec.
		WithTrackRetimer(mkvgo.NewTrackRetimer()).
		// A few seconds of picture with one audio track, rendered small enough
		// for a browser. Nothing measurable here says whether the sound follows
		// the IMAGE - only someone watching can, and this is what they watch.
		WithSyncPreviewRenderer(ffmpeg.NewSyncPreview("ffmpeg")).
		// The PROOF behind an audio start offset: do the file's own audio
		// tracks agree once their declared offsets are applied? Two tracks of
		// one film share their music and effects bed, so they line up when the
		// offsets are RIGHT - a deliberate dub alignment - and disagree when a
		// track has been moved. Runs on every file with two or more audio
		// tracks: a track pulled back onto the picture declares no offset,
		// so a trigger keyed on the declaration would never see the damage.
		WithAudioCoherenceProber(ffmpeg.NewAudioCoherence("ffmpeg", "ffprobe")).
		// The guard between a repair and a silent file: every rewrite is
		// measured for AUDIBLE tracks before and after, and one that costs the
		// file a soundtrack is undone from its own backup. A structural
		// verification passes a silenced track happily - that is how two films
		// lost their French audio in prod (2026-09-01).
		WithAudibleTracksProber(ffmpeg.NewAudioSanity("ffmpeg", "ffprobe")).
		// The exhaustive form of that guard, for the rewrites that MOVE or
		// DROP audio bytes (a resync, an undone retime): every track measured
		// end to end - silence map and frame-size uniformity - before and
		// after, and a loss puts the original back. Three sampled windows
		// cover a tenth of a film and pass a track whose frames change format
		// at a joint, which decodes cleanly and plays silent (prod 2026-09-01).
		WithAudioIntegrityProber(ffmpeg.NewAudioIntegrity("ffmpeg", "ffprobe")).
		// Chunked interleave (minutes of video, then minutes of audio): every
		// header healthy, no sequential player gets past the first block.
		// Every cluster's timestamp (mkvgo Analyze, one sequential read of the
		// file, in the background) for the scan and the repair's own check: a
		// thin block the library scan's sampling steps over is caught here.
		WithInterleaveProber(mkvgo.ExactInterleaveProber{Adapter: mkvgo.New()}).
		// The measure reaches the catalog so playback refuses such a file
		// (file_needs_remux) instead of stalling on it; cleared by the repair.
		WithInterleaveRecorder(deps.interleaveRecorder).
		// The re-mux itself: three demuxers of the file and a muxer that waits
		// for no block, proven packet for packet before the swap.
		WithReinterleaver(ffmpeg.NewReinterleaver("ffmpeg", "ffprobe")).
		// Sound vs PICTURE, off the file's own subtitles: the only measure that
		// sees every track pulled early together (2026-07-12). Gated on
		// ffmpeg-written or retimed files by the muxer probe.
		WithMuxerProber(mkvgo.New()).
		WithAudioPictureProber(ffmpeg.NewSubtitleSync("ffmpeg", "ffprobe", "")).
		// Audio tail-gap probe: does the audio content reach the declared
		// duration? Catches the truncated-audio-track mux (structurally
		// healthy file, dead audio over the credits) ops.Diagnose cannot
		// see. mkvgo-only: per-track DURATION tags at the head, Cue-seek
		// cluster walk of the tail as fallback. NOT repairable - the tail is
		// absent from the file; playback fills it with silence, the operator
		// re-acquires for a real fix.
		// Wrong-container repair: a .mkv whose bytes are MP4/MOV is renamed to
		// .mp4 and the catalog repointed - the extension IS the defect, there
		// is no Matroska index to build. Without this the file stays a chore
		// the page can only describe.
		WithMediaRelocator(seekIndexRelocator{cmd: deps.library.cmdSvc}).
		// Scans survive restarts and batch jobs reuse them as phase 1; a job
		// killed by a redeploy resurfaces as "interrupted" instead of
		// vanishing (the operator used to believe the batch had finished).
		WithStateDir(cfg.Library.SeekIndexStateDir).
		// Stand aside while anyone is watching. These jobs rewrite multi-GB
		// files in place on the library disk, and running them against a live
		// viewer is what starved playback of I/O on 2026-07-19. Same source as
		// the trickplay generator and the transcode calibrator use.
		WithActiveStreamChecker(deps.media.hlsVODRemover).
		// A job is a child of the server's lifetime: SIGTERM cancels it at its
		// next file boundary. Stop then waits for it to write its journal entry
		// and clear its marker before the pool closes (stopBackgroundWorkers
		// runs ahead of the deferred db.Close).
		WithBaseContext(serverCtx)
	deps.bgStoppers = append(deps.bgStoppers, seekIndexSvc.Stop)
	deps.seekIndexSvc = seekIndexSvc

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
		UserRatings:            deps.repos.userRatingRepo,
		MetadataCmd:            deps.content.metadataCmdSvc,
		MetadataQuery:          deps.content.metadataQuerySvc,
		ActivityCmd:            deps.engagement.activityCmdSvc,
		ActivityQuery:          deps.engagement.activityQuerySvc,
		NotifCmd:               deps.engagement.notifCmdSvc,
		NotifQuery:             deps.engagement.notifQuerySvc,
		FeedCmd:                deps.engagement.feedCmdSvc,
		FeedQuery:              deps.engagement.feedQuerySvc,
		Webhook:                deps.engagement.webhookSvc,
		CollectionCmd:          deps.content.collectionCmdSvc,
		CollectionQuery:        deps.content.collectionQuerySvc,
		Playlist:               deps.content.playlistSvc,
		Favorite:               deps.content.favoriteSvc,
		MediaRequest:           deps.content.mediaRequestSvc,
		Backup:                 deps.backupSvc,
		BackupScheduler:        deps.backupScheduler,
		Cfg:                    cfg,
		CfgStore:               deps.cfgStore,
		Version:                buildID().Version,
		Commit:                 buildID().Commit,
		BuildDate:              buildID().BuildDate,
		HLSVODOrch:             deps.media.hlsVODOrch,
		HLSVODRemover:          deps.media.hlsVODRemover,
		Scheduler:              deps.library.scheduler,
		LogDir:                 logDir,
		ServerCtx:              serverCtx,
		Translator:             deps.translator,
		SMTPConfigured:         cfg.SMTP.Host != "",
		ServerCfgSvc:           deps.serverCfgSvc,
		ShareSvc:               deps.media.shareSvc,
		Music:                  deps.media.musicSvc,
		MusicNowPlaying:        deps.media.musicNowPlaying,
		Photo:                  deps.media.photoSvc,
		SyncplayHub:            deps.media.syncPlayHub,
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
		LoadTestPublicURL:      deps.loadTestPublicURL,
		LoadTestTokens:         deps.auth.tokenGen,
		Presence:               deps.presenceTracker,
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
			// Request id FIRST: recovery/sanitizer/ratelimit log too, and
			// before this they did so without a request_id (the id was only
			// stamped by LoggingUnary, six interceptors deep).
			interceptors.RequestIDUnary(),
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
			interceptors.RequestIDStream(),
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
