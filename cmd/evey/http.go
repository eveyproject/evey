package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	httpapi "github.com/gravity-zero/evey/internal/adapters/driving/http"
	"github.com/gravity-zero/evey/internal/config"
	eveymetrics "github.com/gravity-zero/evey/internal/pkg/metrics"
	"github.com/gravity-zero/evey/internal/pkg/network"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
)

func buildHTTPHandler(gwMux http.Handler, deps *appDeps, cfg *config.Config, startTime time.Time, logDir string, ctx context.Context) (http.Handler, error) {
	mux := http.NewServeMux()

	loginRL := ratelimit.New(cfg.Server.LoginRateLimitMax)
	globalRL := ratelimit.New(100)
	adminHeavyRL := ratelimit.NewWithBurst(3, 6.0)
	go func() {
		<-ctx.Done()
		loginRL.Stop()
		globalRL.Stop()
		adminHeavyRL.Stop()
	}()

	audioNormalizeGetter := func(ctx context.Context) bool {
		entry, err := deps.serverCfgRepo.Get(ctx, "transcode.audio_normalize")
		return err == nil && entry.Value == "true"
	}

	srv := &httpapi.HTTPServer{
		BaseURL:   cfg.Server.BaseURL,
		Version:   version,
		StartTime: startTime,
		LogDir:    logDir,
		HTTPSOnly: cfg.Server.ForwardedProto,

		TokenGen: deps.tokenGen,
		Cfg:      cfg,

		LibCmdSvc:   deps.libCmdSvc,
		LibQuerySvc: deps.libQuerySvc,
		Scheduler:   deps.libScheduler,

		UserCmdSvc:   deps.userCmdSvc,
		UserQuerySvc: deps.userQuerySvc,
		OIDCSvc:      deps.oidcSvc,

		PlaybackSvc: deps.playbackSvc,

		TranscodingSvc: deps.transcodingSvc,
		HLSManager:     deps.hlsManager,

		MetadataCmdSvc:   deps.metadataCmdSvc,
		MetadataQuerySvc: deps.metadataQuerySvc,

		ActivityQuerySvc: deps.activityQuerySvc,

		CollectionSvc: deps.collectionSvc,
		PlaylistSvc:   deps.playlistSvc,
		FavoriteSvc:   deps.favoriteSvc,

		MusicSvc: deps.musicSvc,

		UserRatingRepo: deps.userRatingRepo,
		StarRatingSvc:  deps.starRatingSvc,

		PhotoSvc: deps.photoSvc,

		SyncPlayHub: deps.syncPlayHub,

		SubtitleSvc: deps.subtitleSvc,

		BackupSvc: deps.backupSvc,

		FCMTokenRepo: deps.fcmTokenRepo,

		ArtworkRepo:       deps.artworkRepo,
		LibraryAccessRepo: deps.libraryAccessRepo,
		ServerCfgRepo:     deps.serverCfgRepo,
		ProfileRepo:       deps.profileRepo,
		Hasher:            deps.hasher,

		CfgStore:  deps.cfgStore,
		ServerCtx: ctx,

		LoginRL:      loginRL,
		GlobalRL:     globalRL,
		AdminHeavyRL: adminHeavyRL,

		AudioNormalizeGetter: audioNormalizeGetter,
		UserMaxStreams:        httpapi.NewUserMaxStreamsAdapter(deps.userRepo),
		UserBitrate:          httpapi.NewUserBitrateAdapter(deps.userQuerySvc),
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

	return httpapi.TokenRedactMiddleware(eveymetrics.InstrumentHandler(httpapi.SecurityHeadersMiddleware(proxyTrust.Middleware(netMw(mux))))), nil
}
