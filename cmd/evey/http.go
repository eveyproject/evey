package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	httpapi "github.com/evey-project/evey/internal/adapters/driving/http"
	appcast "github.com/evey-project/evey/internal/app/chromecast"
	appdlna "github.com/evey-project/evey/internal/app/dlna"
	"github.com/evey-project/evey/internal/adapters/driven/pluginrunner"
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
	adminHeavyRL := ratelimit.NewWithBurst(10, 10.0)
	adminWriteRL := ratelimit.New(30)
	passwordResetRL := ratelimit.New(5)
	oidcLoginRL := ratelimit.New(10)
	go func() {
		<-ctx.Done()
		loginRL.Stop()
		globalRL.Stop()
		adminHeavyRL.Stop()
		adminWriteRL.Stop()
		passwordResetRL.Stop()
		oidcLoginRL.Stop()
	}()

	audioNormalizeGetter := func(ctx context.Context) bool {
		entry, err := deps.repos.serverCfgRepo.Get(ctx, "transcode.audio_normalize")
		return err == nil && entry.Value == "true"
	}

	srv := &httpapi.HTTPServer{
		BaseURL:   cfg.EffectiveBaseURL,
		Version:   version,
		StartTime: startTime,
		LogDir:    logDir,
		HTTPSOnly: cfg.Server.ForwardedProto,

		SearchHealth: deps.searchHealth,

		TokenGen:         deps.auth.tokenGen,
		Cfg:              cfg,
		StreamTokenStore: deps.media.streamTokenStore,
		ShareSvc:         deps.media.shareSvc,

		LibCmdSvc:   deps.library.cmdSvc,
		LibQuerySvc: deps.library.querySvc,
		Scheduler:   deps.library.scheduler,

		OIDCSvc:         deps.user.oidcSvc,
		UserAuthSvc:     deps.user.authSvc,
		UserQuerySvc:    deps.user.querySvc,
		QuickConnectHub: qcHub,

		TranscodingSvc: deps.media.transcodingSvc,
		HLSVODOrch:     deps.media.hlsVODOrch,
		HLSVODCache:    deps.media.hlsVODCache,
		HLSTranscoder:  deps.media.hlsTranscoder,
		QoESink:        deps.media.qoeSink,
		QoEQuerySvc:    deps.media.qoeQuerySvc,
		ComplexityRepo: deps.media.complexityRepo,
		CalibrationSvc: deps.media.calibrationSvc,
		CapOverrideSvc: deps.media.capsOverrideSvc,

		MetadataCmdSvc:   deps.content.metadataCmdSvc,
		MetadataQuerySvc: deps.content.metadataQuerySvc,

		ActivityQuerySvc: deps.engagement.activityQuerySvc,
		ActivityCmdSvc:   deps.engagement.activityCmdSvc,

		CollectionSvc:  deps.content.collectionSvc,
		PlaylistSvc:    deps.content.playlistSvc,
		PlaylistGenSvc: deps.content.playlistGenSvc,
		FavoriteSvc:    deps.content.favoriteSvc,

		MusicSvc: deps.media.musicSvc,

		UserRatingRepo: deps.repos.userRatingRepo,
		StarRatingSvc:  deps.content.starRatingSvc,

		PhotoSvc:        deps.media.photoSvc,
		PhotoTranscoder: deps.media.photoTranscoder,
		PhotoCacheDir:   cfg.Transcoding.CacheDir,

		SyncPlayHub:    deps.media.syncPlayHub,
		DeviceRegistry: deps.media.deviceRegistry,

		SubtitleSvc: deps.media.subtitleSvc,

		BackupSvc: deps.backupSvc,

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
		DataDir:     "/data",
		PluginDir:   "/data/plugins",
		WebDir:      cfg.Server.WebDir,
		DLNASvc: appdlna.New(cfg.EffectiveBaseURL()),
		CastSvc: appcast.New(cfg.EffectiveBaseURL()),

		CfgStore: deps.cfgStore,
		ServerCtx: ctx,

		LoginRL:         loginRL,
		GlobalRL:        globalRL,
		AdminHeavyRL:    adminHeavyRL,
		AdminWriteRL:    adminWriteRL,
		PasswordResetRL: passwordResetRL,
		OIDCLoginRL:     oidcLoginRL,

		Translator:           deps.translator,
		AudioNormalizeGetter: audioNormalizeGetter,
		UserMaxStreams:        httpapi.NewUserMaxStreamsAdapter(deps.repos.userRepo),
		UserBitrate:          httpapi.NewUserBitrateAdapter(deps.user.querySvc),
		ProfilePrefs:         httpapi.NewProfilePrefsAdapter(deps.user.querySvc),
		FrameExtractor:       deps.frameExtractor,
		IntroDetector:        deps.library.introDetectionSvc,
	}

	srv.RegisterRoutes(mux, gwMux)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in backup retention cleanup", "error", r, "stack", string(debug.Stack()))
			}
		}()
		if err := deps.backupSvc.CleanupOldBackups(ctx, cfg.Backup.RetentionDays); err != nil {
			slog.Warn("backup retention cleanup", slog.Any("err", err))
		}
	}()

	netDetector := network.NewDetector(cfg.RemoteAccess.LANNetworks)
	netMw := networkMiddleware(netDetector, &cfg.RemoteAccess.Enabled)

	proxyTrust, err := network.NewProxyTrust(cfg.Server.TrustedProxies, cfg.Server.RealIPHeader, cfg.Server.ForwardedProto)
	if err != nil {
		return nil, fmt.Errorf("proxy trust: %w", err)
	}

	return httpapi.RequestIDMiddleware(httpapi.TokenRedactMiddlewareWithStore(srv.StreamTokenStore, eveymetrics.InstrumentHandler(httpapi.SecurityHeadersMiddleware(cfg.Server.ForwardedProto, httpapi.CSRFMiddleware(cfg.Server.ForwardedProto, proxyTrust.Middleware(netMw(mux))))))), nil
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
