package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	httpapi "github.com/gravity-zero/evey/internal/adapters/driving/http"
	"github.com/gravity-zero/evey/internal/config"
	"github.com/gravity-zero/evey/internal/domain/shared"
	eveymetrics "github.com/gravity-zero/evey/internal/pkg/metrics"
	"github.com/gravity-zero/evey/internal/pkg/network"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
)

func buildHTTPHandler(gwMux http.Handler, deps *appDeps, cfg *config.Config, startTime time.Time, logDir string, ctx context.Context) (http.Handler, error) {
	mux := http.NewServeMux()

	loginRL := ratelimit.New(cfg.Server.LoginRateLimitMax)
	globalRL := ratelimit.New(100)
	adminHeavyRL := ratelimit.NewWithBurst(10, 10.0)
	go func() {
		<-ctx.Done()
		loginRL.Stop()
		globalRL.Stop()
		adminHeavyRL.Stop()
	}()

	audioNormalizeGetter := func(ctx context.Context) bool {
		entry, err := deps.repos.serverCfgRepo.Get(ctx, "transcode.audio_normalize")
		return err == nil && entry.Value == "true"
	}

	srv := &httpapi.HTTPServer{
		BaseURL:   cfg.Server.BaseURL,
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

		UserSvc:      deps.user.userSvc,
		UserQuerySvc: deps.user.querySvc,
		OIDCSvc:      deps.user.oidcSvc,

		PlaybackSvc: deps.media.playbackSvc,

		TranscodingSvc: deps.media.transcodingSvc,
		HLSManager:     deps.media.hlsManager,

		MetadataCmdSvc:   deps.content.metadataCmdSvc,
		MetadataQuerySvc: deps.content.metadataQuerySvc,

		ActivityQuerySvc: deps.engagement.activityQuerySvc,

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

		ArtworkRepo:       deps.repos.artworkRepo,
		LibraryAccessRepo: deps.repos.libraryAccessRepo,
		ServerCfgRepo:     deps.repos.serverCfgRepo,
		ProfileRepo:       deps.repos.profileRepo,
		UserRepo:          deps.repos.userRepo,
		Hasher:            deps.auth.hasher,

		PluginRepo: deps.repos.pluginRepo,
		PluginSvc:  deps.engagement.pluginInstallerSvc,
		PluginDir:  "/data/plugins",

		FeedCmdSvc:   deps.engagement.feedCmdSvc,
		FeedQuerySvc: deps.engagement.feedQuerySvc,

		CfgStore: deps.cfgStore,
		ServerCtx: ctx,

		LoginRL:      loginRL,
		GlobalRL:     globalRL,
		AdminHeavyRL: adminHeavyRL,

		Translator:           deps.translator,
		AudioNormalizeGetter: audioNormalizeGetter,
		UserMaxStreams:        httpapi.NewUserMaxStreamsAdapter(deps.repos.userRepo),
		UserBitrate:          httpapi.NewUserBitrateAdapter(deps.user.querySvc),
	}

	srv.RegisterRoutes(mux, gwMux)

	go func() {
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

	return httpapi.TokenRedactMiddlewareWithStore(srv.StreamTokenStore, eveymetrics.InstrumentHandler(httpapi.SecurityHeadersMiddleware(proxyTrust.Middleware(netMw(mux))))), nil
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
