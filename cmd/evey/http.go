package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	httpapi "github.com/gravity-zero/evey/internal/adapters/driving/http"
	appconfig "github.com/gravity-zero/evey/internal/app/config"
	appdlna "github.com/gravity-zero/evey/internal/app/dlna"
	"github.com/gravity-zero/evey/internal/config"
	"github.com/gravity-zero/evey/internal/domain/shared"
	eveymetrics "github.com/gravity-zero/evey/internal/pkg/metrics"
	"github.com/gravity-zero/evey/internal/pkg/network"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
)

func buildHTTPHandler(gwMux http.Handler, deps *appDeps, cfg *config.Config, startTime time.Time, logDir string, ctx context.Context) (http.Handler, error) {
	mux := http.NewServeMux()

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

		LibCmdSvc:   deps.library.cmdSvc,
		LibQuerySvc: deps.library.querySvc,
		Scheduler:   deps.library.scheduler,

		UserSvc:      deps.user.userSvc,
		UserQuerySvc: deps.user.querySvc,
		OIDCSvc:      deps.user.oidcSvc,

		PlaybackSvc: deps.media.playbackSvc,

		TranscodingSvc: deps.media.transcodingSvc,
		HLSManager:     deps.media.hlsManager,

		MetadataCmdSvc:   deps.content.metadataCmdSvc,
		MetadataQuerySvc: deps.content.metadataQuerySvc,

		ActivityQuerySvc: deps.engagement.activityQuerySvc,
		ActivityCmdSvc:   deps.engagement.activityCmdSvc,

		CollectionSvc: deps.content.collectionSvc,
		PlaylistSvc:   deps.content.playlistSvc,
		FavoriteSvc:   deps.content.favoriteSvc,

		MusicSvc: deps.media.musicSvc,

		UserRatingRepo: deps.repos.userRatingRepo,
		StarRatingSvc:  deps.content.starRatingSvc,

		PhotoSvc: deps.media.photoSvc,

		SyncPlayHub: deps.media.syncPlayHub,

		SubtitleSvc: deps.media.subtitleSvc,

		BackupSvc: deps.backupSvc,

		PushSubRepo:    deps.repos.pushSubRepo,
		VAPIDPublicKey: resolveVAPIDPublicKey(deps.repos.serverCfgRepo, cfg),

		ImageConverter:    deps.imageConverter,
		ArtworkRepo:       deps.repos.artworkRepo,
		LibraryAccessRepo: deps.repos.libraryAccessRepo,
		ServerCfgSvc:      appconfig.NewService(deps.repos.serverCfgRepo),
		ServerCfgRepo:     deps.repos.serverCfgRepo,
		ProfileRepo:       deps.repos.profileRepo,
		UserRepo:          deps.repos.userRepo,
		Hasher:            deps.auth.hasher,

		PluginRepo: deps.repos.pluginRepo,
		PluginSvc:  deps.engagement.pluginInstallerSvc,
		DataDir:     "/data",
		PluginDir:   "/data/plugins",
		WebDir:      cfg.Server.WebDir,
		BioEnricher: deps.content.enricher,

		FeedCmdSvc:   deps.engagement.feedCmdSvc,
		FeedQuerySvc: deps.engagement.feedQuerySvc,

		DLNASvc: appdlna.New(cfg.EffectiveBaseURL()),

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
	}

	srv.RegisterRoutes(mux, gwMux)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in backup retention cleanup", "error", r, "stack", debug.Stack())
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

	return httpapi.TokenRedactMiddlewareWithStore(srv.StreamTokenStore, eveymetrics.InstrumentHandler(httpapi.SecurityHeadersMiddleware(httpapi.CSRFMiddleware(cfg.Server.ForwardedProto, proxyTrust.Middleware(netMw(mux)))))), nil
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
