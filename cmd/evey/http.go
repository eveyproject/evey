package main

import (
	"context"
	"fmt"
	"github.com/evey-project/evey/internal/adapters/driven/ffmpeg"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/pluginrunner"
	httpapi "github.com/evey-project/evey/internal/adapters/driving/http"
	appcast "github.com/evey-project/evey/internal/app/chromecast"
	appdlna "github.com/evey-project/evey/internal/app/dlna"
	appplayback "github.com/evey-project/evey/internal/app/playback"
	"github.com/evey-project/evey/internal/config"
	"github.com/evey-project/evey/internal/domain/shared"
	eveymetrics "github.com/evey-project/evey/internal/pkg/metrics"
	"github.com/evey-project/evey/internal/pkg/network"
	"github.com/evey-project/evey/internal/pkg/quickconnecthub"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

func buildHTTPHandler(gwMux http.Handler, deps *appDeps, cfg *config.Config, startTime time.Time, logDir string, ctx context.Context, qcHub *quickconnecthub.Hub) (http.Handler, error) {
	mux := http.NewServeMux()

	ratelimit.SetTrustedProxies(cfg.Server.TrustedProxies)

	loginRL := ratelimit.New(cfg.Server.LoginRateLimitMax)
	globalRL := ratelimit.New(cfg.Server.GlobalRateLimitMax)
	// Per-profile streaming limiter: deliberately generous. A profile streaming
	// even on several devices with heavy seeking stays far below this; the
	// ceiling only cuts a cheap-request FLOOD (404s, cache hits). burst 600 +
	// 1200/min refill (20/s) leaves wide headroom over legitimate use (~20-60
	// req/min steady per profile) while a flood of hundreds/s is throttled.
	streamingRL := ratelimit.NewWithBurst(600, 1200.0)
	adminHeavyRL := ratelimit.NewWithBurst(10, 10.0)
	adminWriteRL := ratelimit.New(30)
	passwordResetRL := ratelimit.New(5)
	oidcLoginRL := ratelimit.New(10)
	// Snapshot/frame extraction spawns one ffmpeg per request: burst covers an
	// admin thumbnail grid, the 1/s refill caps sustained spawn abuse.
	snapshotRL := ratelimit.NewWithBurst(30, 60.0)
	// Subtitle extraction falls back to exec.CommandContext'ing ffmpeg whenever
	// mkvgo's fast path can't shortcut the container (.avi/.ts/.mov, damaged
	// EBML) - one fork per request, same cost as a snapshot, so it shares the
	// identical ceiling instead of the generous global limiter.
	subtitleRL := ratelimit.NewWithBurst(30, 60.0)
	// The sync preview is a LOOP, not a one-off: an operator watches a clip,
	// nudges the candidate offset, watches again, a dozen times before the ear
	// settles. Sharing adminHeavyRL's burst of 10 refilling at 10/min meant the
	// tool ran out of budget mid-judgement and refused with "too many requests"
	// - and, because that limiter is shared, took trickplay generation and the
	// search reindex down with it. Its own bucket: a burst that covers a whole
	// session of nudging, a refill of one render every two seconds. Each render
	// is still one ffmpeg, and the procfork budget is what caps concurrency.
	syncPreviewRL := ratelimit.NewWithBurst(40, 30.0)
	// Trickplay (manifest + sprite images) only reads disk/DB, no fork - far
	// cheaper than subtitle/snapshot. Burst of 120 covers loading a full
	// storyboard sprite sheet while scrubbing; refill still caps a flood.
	trickplayRL := ratelimit.NewWithBurst(120, 240.0)
	// Subtitle search/download call the provider on the operator's API key and
	// spend its per-day download quota, so this is tighter than the CPU-bound
	// limits above: a burst covers picking through a few candidates for one
	// episode, the slow refill stops one account draining the whole allowance.
	subtitleProviderRL := ratelimit.NewWithBurst(12, 12.0)
	// Playback-error beacon: each accepted event is one log line. Burst covers
	// a genuine error storm's first wave; the refill caps a hostile or looping
	// client at a rate the log files shrug off.
	playerEventRL := ratelimit.NewWithBurst(20, 30.0)
	go func() {
		<-ctx.Done()
		loginRL.Stop()
		globalRL.Stop()
		streamingRL.Stop()
		adminHeavyRL.Stop()
		adminWriteRL.Stop()
		passwordResetRL.Stop()
		oidcLoginRL.Stop()
		snapshotRL.Stop()
		subtitleRL.Stop()
		syncPreviewRL.Stop()
		trickplayRL.Stop()
		subtitleProviderRL.Stop()
	}()

	audioNormalizeGetter := func(ctx context.Context) bool {
		entry, err := deps.repos.serverCfgRepo.Get(ctx, "transcode.audio_normalize")
		return err == nil && entry.Value == "true"
	}

	// Compat download (server-prepared variant with phone-renderable AAC
	// audio): ON by default with an operator opt-out, the house posture for
	// transcoding features. nil leaves the routes unregistered entirely.
	var compatTranscoder httpapi.CompatTranscoder
	if v := resolveFlag(ctx, deps.repos.serverCfgRepo, "transcoding.compat_download", "EVEY_COMPAT_DOWNLOAD"); v == "off" || v == "false" {
		slog.Info("compat-download disabled (transcoding.compat_download=off)")
	} else {
		compatTranscoder = ffmpeg.NewCompatTranscoder("ffmpeg")
	}

	// Reduced (720p) download: OFF by default, the opposite posture from
	// compat-download and deliberately so. Compat re-encodes audio only and
	// clears a film in minutes; this re-encodes VIDEO at roughly realtime on a
	// weak NAS, so an operator who never asked for it must never discover it
	// by watching their box grind through a two-hour film.
	var reducedTranscoder httpapi.ReducedTranscoder
	if v := resolveFlag(ctx, deps.repos.serverCfgRepo, "downloads.quality_choice", "EVEY_DOWNLOAD_QUALITY_CHOICE"); v == "on" || v == "true" {
		reducedTranscoder = ffmpeg.NewReducedTranscoder("ffmpeg")
		slog.Info("reduced (720p) download enabled (downloads.quality_choice=on)")
	}
	// The size above which the app offers the lighter copy. The phone ORs this
	// with its own "it does not fit here" check, which no server can know.
	reducedOfferAbove := int64(4 << 30)
	if v := resolveFlag(ctx, deps.repos.serverCfgRepo, "downloads.reduce_above_bytes", "EVEY_DOWNLOAD_REDUCE_ABOVE_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			reducedOfferAbove = n
		} else {
			slog.Warn("downloads.reduce_above_bytes is not a positive byte count, keeping the default",
				slog.String("value", v), slog.Int64("default", reducedOfferAbove))
		}
	}

	srv := &httpapi.HTTPServer{
		BaseURL:   cfg.EffectiveBaseURL,
		Version:   buildID().Version,
		Commit:    buildID().Commit,
		BuildDate: buildID().BuildDate,
		StartTime: startTime,
		LogDir:    logDir,
		HTTPSOnly: cfg.Server.ForwardedProto,

		SearchHealth: deps.searchHealth,

		TokenGen:         deps.auth.userTokenGen,
		PluginTokenGen:   deps.auth.tokenGen,
		Cfg:              cfg,
		StreamTokenStore: deps.media.streamTokenStore,
		CastProgress:     deps.media.castProgress,
		ShareSvc:         deps.media.shareSvc,

		LibCmdSvc:        deps.library.cmdSvc,
		LibQuerySvc:      deps.library.querySvc,
		Scheduler:        deps.library.scheduler,
		Remuxer:          deps.media.mkvAdapter,
		CompatTranscoder: compatTranscoder,

		ReducedTranscoder:      reducedTranscoder,
		ReducedOfferAboveBytes: reducedOfferAbove,

		OIDCSvc:         deps.user.oidcSvc,
		UserAuthSvc:     deps.user.authSvc,
		UserQuerySvc:    deps.user.querySvc,
		QuickConnectHub: qcHub,

		TranscodingSvc: deps.media.transcodingSvc,
		HLSVODOrch:     deps.media.hlsVODOrch,
		HLSVODSoftware: deps.media.hlsVODSoftware,
		HLSVODCache:    deps.media.hlsVODCache,
		HLSTranscoder:  deps.media.hlsTranscoder,
		AudioOrch:      deps.media.audioOrch,
		QoESink:        deps.media.qoeSink,
		QoEQuerySvc:    deps.media.qoeQuerySvc,
		ComplexityRepo: deps.media.complexityRepo,
		CalibrationSvc: deps.media.calibrationSvc,
		CapOverrideSvc: deps.media.capsOverrideSvc,

		MetadataCmdSvc: deps.content.metadataCmdSvc,
		// The preview clip route: a few seconds of picture with one audio
		// track, which is the only way to tell whether the sound follows the
		// image. Nil until the seek-index service exists.
		SeekIndexSvc:     deps.seekIndexSvc,
		MetadataQuerySvc: deps.content.metadataQuerySvc,

		ActivityQuerySvc: deps.engagement.activityQuerySvc,
		ActivityCmdSvc:   deps.engagement.activityCmdSvc,

		CollectionSvc:  deps.content.collectionSvc,
		PlaylistSvc:    deps.content.playlistSvc,
		PlaylistGenSvc: deps.content.playlistGenSvc,
		FavoriteSvc:    deps.content.favoriteSvc,

		MusicSvc:        deps.media.musicSvc,
		MusicNowPlaying: deps.media.musicNowPlaying,

		UserRatingRepo: deps.repos.userRatingRepo,
		StarRatingSvc:  deps.content.starRatingSvc,

		PhotoSvc:        deps.media.photoSvc,
		PhotoTranscoder: deps.media.photoTranscoder,
		PhotoCacheDir:   cfg.Transcoding.CacheDir,

		SyncPlayHub:    deps.media.syncPlayHub,
		DeviceRegistry: deps.media.deviceRegistry,

		SubtitleSvc: deps.media.subtitleSvc,

		BackupSvc:  deps.backupSvc,
		WebhookSvc: deps.engagement.webhookSvc,

		PushSubRepo:    deps.repos.pushSubRepo,
		VAPIDPublicKey: resolveVAPIDPublicKey(deps.repos.serverCfgRepo, cfg),

		ImageConverter:    deps.imageConverter,
		ArtworkRepo:       deps.repos.artworkRepo,
		LibraryAccessRepo: deps.repos.libraryAccessRepo,
		ServerCfgRepo:     deps.repos.serverCfgRepo,
		ProfileRepo:       deps.repos.profileRepo,
		UserRepo:          deps.repos.userRepo,
		Hasher:            deps.auth.hasher,

		PluginRepo:                 deps.repos.pluginRepo,
		PluginSvc:                  deps.engagement.pluginInstallerSvc,
		CSSSanitizer:               pluginrunner.SanitizeCSS,
		PluginSecurityDashboardSvc: deps.engagement.pluginSecurityDashboardSvc,
		PluginWakener:              deps.engagement.pluginIdleMgr,
		KeyframeWarmer:             deps.keyframeOnDemand.TryIndex,
		VideoLimiter:               deps.media.videoLimiter,
		VideoAdmissionShaper:       deps.media.videoShaper,
		DataDir:                    "/data",
		PluginDir:                  "/data/plugins",
		WebDir:                     cfg.Server.WebDir,
		DLNASvc:                    appdlna.New(cfg.EffectiveBaseURL()),
		CastSvc:                    appcast.New(cfg.EffectiveBaseURL()),
		// Receiver beacons feed the same presence registry as the app
		// heartbeats, plus a session keepalive: a cast whose phone froze stays
		// a visible, positioned now-playing session driven by the TV itself.
		CastPresence: deps.presenceTracker,
		CastResolveUsername: func(ctx context.Context, userID string) string {
			u, err := deps.user.querySvc.GetUserByID(ctx, userID)
			if err != nil || u == nil {
				return ""
			}
			return u.Username
		},
		CastSessionKeepalive: func(ctx context.Context, b httpapi.CastBeacon) {
			_, err := deps.media.playbackSvc.CommandService().TouchCastSession(ctx, appplayback.CastBeacon{
				ProfileID:   b.ProfileID,
				MediaItemID: b.MediaItemID,
				PositionMs:  b.PositionMs,
				AllowCreate: b.AllowCreate,
				Paused:      b.Paused,
			})
			if err != nil {
				slog.WarnContext(ctx, "cast session keepalive failed",
					slog.String("profile_id", b.ProfileID), slog.String("media_id", b.MediaItemID), slog.Any("err", err))
			}
		},
		CastSessionTerminate: func(ctx context.Context, profileID, mediaItemID string) {
			if err := deps.media.playbackSvc.CommandService().EndCastSession(ctx, profileID, mediaItemID); err != nil {
				slog.WarnContext(ctx, "cast session end failed",
					slog.String("profile_id", profileID), slog.String("media_id", mediaItemID), slog.Any("err", err))
			}
		},
		CastAudioModeTouch: func(ctx context.Context, profileID, mediaItemID, mode string) {
			if err := deps.media.playbackSvc.CommandService().SetCastAudioPath(ctx, profileID, mediaItemID, mode); err != nil {
				slog.WarnContext(ctx, "cast audio path update failed",
					slog.String("profile_id", profileID), slog.String("media_id", mediaItemID), slog.Any("err", err))
			}
		},
		CastCommandConsume: func(ctx context.Context, profileID, mediaItemID string) string {
			cmd, err := deps.media.playbackSvc.CommandService().ConsumeCastCommand(ctx, profileID, mediaItemID)
			if err != nil {
				slog.WarnContext(ctx, "cast command consume failed",
					slog.String("profile_id", profileID), slog.String("media_id", mediaItemID), slog.Any("err", err))
				return ""
			}
			return cmd
		},

		CfgStore:  deps.cfgStore,
		ServerCtx: ctx,

		LoginRL:     loginRL,
		GlobalRL:    globalRL,
		StreamingRL: streamingRL,
		// While the built-in load test runs, the per-profile streaming limiter
		// stands down: every simulated viewer carries the same credential, so
		// they would all throttle each other in one bucket and the run would
		// measure the limiter instead of the server.
		LoadTestActive: func() bool {
			if deps.loadTestRunner == nil {
				return false
			}
			return deps.loadTestRunner.Status().Running
		},
		AdminHeavyRL:       adminHeavyRL,
		SyncPreviewRL:      syncPreviewRL,
		SnapshotRL:         snapshotRL,
		SubtitleRL:         subtitleRL,
		TrickplayRL:        trickplayRL,
		SubtitleProviderRL: subtitleProviderRL,
		PlayerEventRL:      playerEventRL,
		AdminWriteRL:       adminWriteRL,
		PasswordResetRL:    passwordResetRL,
		OIDCLoginRL:        oidcLoginRL,

		Translator:           deps.translator,
		AudioNormalizeGetter: audioNormalizeGetter,
		AudioEncoderIDGetter: ffmpeg.AACEncoder,
		// Audio renditions ride the video's timeline. mkvgo presents at 0, the
		// ffmpeg copy fallback presents late by has_b_frames/fps - the HTTP layer
		// asks this probe how much to shift by. Best-effort: it returns 0 on any
		// failure, which is the uncompensated behaviour.
		VideoCompositionDelayProbe: ffmpeg.BundledVideoCompositionDelaySec,
		UserMaxStreams:             httpapi.NewUserMaxStreamsAdapter(deps.repos.userRepo),
		UserBitrate:                httpapi.NewUserBitrateAdapter(deps.user.querySvc),
		ProfilePrefs:               httpapi.NewProfilePrefsAdapter(deps.user.querySvc),
		FrameExtractor:             deps.frameExtractor,
		CopySeekProber:             deps.copySeekProber,
		OnDemandPackager:           deps.onDemandPackager,
		PlaybackActivity:           deps.playbackActivity,
		OnDemandConcatPackager:     deps.onDemandConcatPackager,
		HLSAESSecret:               deps.hlsAESSecret,
		IntroDetector:              deps.library.introDetectionSvc,
		StreamMeter:                deps.engagement.streamMeter,
	}

	srv.RegisterRoutes(mux, gwMux)

	// Kill the VOD software/copy producers' long-lived ffmpeg sessions when the
	// app context is cancelled (shutdown signal). Their session contexts are
	// detached from ctx by design (a request must never inherit a dying ctx),
	// so without this they outlive the process as orphaned ffmpeg children.
	// Idempotent; the HW orchestrator has its own deferred CleanupAll in main.
	go func() {
		<-ctx.Done()
		srv.CloseTranscodeProducers()
		// Stop the on-demand plan cache's janitor goroutine (created
		// unconditionally in wire, independent of the VOD handler).
		if c, ok := deps.onDemandPackager.(interface{ Close() }); ok {
			c.Close()
		}
		if c, ok := deps.onDemandConcatPackager.(interface{ Close() }); ok {
			c.Close()
		}
	}()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "panic recovered in backup retention cleanup", "error", r, "stack", string(debug.Stack()))
			}
		}()
		if err := deps.backupSvc.CleanupOldBackups(ctx, cfg.Backup.RetentionDays); err != nil {
			slog.WarnContext(ctx, "backup retention cleanup", slog.Any("err", err))
		}
	}()

	netDetector := network.NewDetector(cfg.RemoteAccess.LANNetworks)
	netMw := networkMiddleware(netDetector, cfg.EffectiveRemoteAccessEnabled, slog.Default())

	proxyTrust, err := network.NewProxyTrust(cfg.Server.TrustedProxies, cfg.Server.RealIPHeader, cfg.Server.ForwardedProto)
	if err != nil {
		return nil, fmt.Errorf("proxy trust: %w", err)
	}

	return httpapi.RequestIDMiddleware(httpapi.TokenRedactMiddlewareWithStore(srv.StreamTokenStore, cfg.Server.ForwardedProto, eveymetrics.InstrumentHandler(httpapi.SecurityHeadersMiddleware(cfg.Server.ForwardedProto, httpapi.CSRFMiddleware(cfg.Server.ForwardedProto, proxyTrust.Middleware(httpapi.BodyReadTimeoutMiddleware(netMw(mux)))))))), nil
}

func resolveVAPIDPublicKey(cfgRepo shared.ServerConfigRepository, cfg *config.Config) string {
	if cfg.VAPID.PublicKey != "" {
		return cfg.VAPID.PublicKey
	}
	if e, err := cfgRepo.Get(context.Background(), "vapid.public_key"); err == nil && e.Value != "" {
		return e.Value
	}
	return ""
}
