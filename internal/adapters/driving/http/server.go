package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"time"

	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	appbackup "github.com/gravity-zero/evey/internal/app/backup"
	appcollection "github.com/gravity-zero/evey/internal/app/collection"
	appfavorite "github.com/gravity-zero/evey/internal/app/favorite"
	appfeed "github.com/gravity-zero/evey/internal/app/feed"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appmedia "github.com/gravity-zero/evey/internal/app/media"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	appmusic "github.com/gravity-zero/evey/internal/app/music"
	appphoto "github.com/gravity-zero/evey/internal/app/photo"
	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	appplaylist "github.com/gravity-zero/evey/internal/app/playlist"
	apprating "github.com/gravity-zero/evey/internal/app/rating"
	appsubtitle "github.com/gravity-zero/evey/internal/app/subtitle"
	appsyncplay "github.com/gravity-zero/evey/internal/app/syncplay"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	"github.com/gravity-zero/evey/internal/config"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/notification"
	domainplugin "github.com/gravity-zero/evey/internal/domain/plugin"
	domainshared "github.com/gravity-zero/evey/internal/domain/shared"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/i18n"
	eveymetrics "github.com/gravity-zero/evey/internal/pkg/metrics"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
)

// DBPinger is satisfied by *pgxpool.Pool and any test double.
type DBPinger interface {
	Ping(ctx context.Context) error
}

// SearchHealthChecker checks Meilisearch connectivity; may be nil (check skipped).
type SearchHealthChecker interface {
	HealthCheck(ctx context.Context) error
}

// HTTPServer holds shared dependencies for HTTP handlers.
type HTTPServer struct {
	BaseURL   string
	Version   string
	StartTime time.Time
	LogDir    string
	HTTPSOnly bool

	// DB is used for liveness checks; may be nil (check skipped).
	DB DBPinger

	// SearchHealth is used for health checks; nil means search not configured.
	SearchHealth SearchHealthChecker

	TokenGen         *auth.JWTGenerator
	Cfg              *config.Config
	StreamTokenStore *appmedia.StreamTokenStore
	ShareSvc         *appmedia.ShareService

	// Library
	LibCmdSvc   *applibrary.CommandService
	LibQuerySvc *applibrary.QueryService
	Scheduler   *applibrary.Scheduler

	// User
	UserSvc      *appuser.UserService
	UserQuerySvc *appuser.QueryService
	OIDCSvc      *appuser.OIDCService

	// Playback
	PlaybackSvc *appplayback.Service

	// Transcoding
	TranscodingSvc *apptranscoding.Service
	HLSManager     *apptranscoding.HLSManager

	// Metadata
	MetadataCmdSvc   *appmetadata.CommandService
	MetadataQuerySvc *appmetadata.QueryService

	// Activity
	ActivityQuerySvc *appactivity.QueryService
	ActivityCmdSvc   *appactivity.CommandService

	// Collections / Playlists / Favorites
	CollectionSvc *appcollection.Service
	PlaylistSvc   *appplaylist.Service
	FavoriteSvc   *appfavorite.Service

	// Music
	MusicSvc *appmusic.Service

	// Rating
	UserRatingRepo domainlibrary.UserRatingRepository
	StarRatingSvc  *apprating.Service

	// Photo
	PhotoSvc *appphoto.Service

	// SyncPlay
	SyncPlayHub *appsyncplay.Hub

	// Subtitle (optional)
	SubtitleSvc *appsubtitle.Service

	// Backup
	BackupSvc *appbackup.Service

	// Push
	PushSubRepo    notification.PushSubscriptionRepository
	VAPIDPublicKey string

	// Image conversion (optional)
	ImageConverter ImageConverter

	// Repos / accessors
	ArtworkRepo       ArtworkRepo
	LibraryAccessRepo domainuser.LibraryAccessRepository
	ServerCfgRepo     domainshared.ServerConfigRepository
	ProfileRepo       domainuser.ProfileRepository
	UserRepo          domainuser.UserRepository
	Hasher            domainuser.PasswordHasher

	// Plugin
	PluginRepo domainplugin.PluginRepository
	PluginSvc  pluginManager
	PluginDir  string

	// Feed
	FeedCmdSvc   *appfeed.CommandService
	FeedQuerySvc *appfeed.QueryService

	// Config store for remote access persistence
	CfgStore remoteAccessPersister

	// Server context (for long-running background ops like photo scan)
	ServerCtx context.Context

	// Rate limiters passed in from main
	LoginRL      *ratelimit.Limiter
	GlobalRL     *ratelimit.Limiter
	AdminHeavyRL *ratelimit.Limiter

	// i18n
	Translator *i18n.Translator

	// audio normalize getter (closure capturing serverCfgRepo)
	AudioNormalizeGetter func(ctx context.Context) bool

	// User stream/bitrate adapters
	UserMaxStreams UserMaxStreamsGetter
	UserBitrate   UserBitrateGetter
}

// RegisterRoutes wires all HTTP routes onto mux.
// gwMux is the grpc-gateway handler (may be nil if gateway failed).
func (s *HTTPServer) RegisterRoutes(mux *http.ServeMux, gwMux http.Handler) {
	mux.Handle("GET /metrics", eveymetrics.Handler())
	mux.HandleFunc("/healthz", s.healthzHandler)

	if s.Translator != nil {
		mux.HandleFunc("GET /api/v1/i18n/{locale}", i18nHandler(s.Translator, s.Cfg.Log.DevMode))
	}

	mux.HandleFunc("GET /api/v1/admin/fs/browse", requireAdmin(s.TokenGen, browseDirsHandler(s.TokenGen)))

	var mhOpts []mediaServeOption
	if s.ImageConverter != nil {
		mhOpts = append(mhOpts, withImageConverter(s.ImageConverter, s.ArtworkRepo))
	}
	mh := mediaHandler(s.TokenGen, s.LibQuerySvc, s.ArtworkRepo, s.BaseURL, s.LibraryAccessRepo, mhOpts...)
	mux.HandleFunc("GET /api/v1/media/{id}/stream", mh)
	mux.HandleFunc("GET /api/v1/media/{id}/artwork/{artType}", mh)
	mux.HandleFunc("POST /api/v1/media/{id}/stream", mh)
	mux.HandleFunc("POST /api/v1/media/{id}/artwork/{artworkId}/select", selectArtworkHandler(s.TokenGen, s.ArtworkRepo, s.LibQuerySvc, s.LibraryAccessRepo))
	mux.HandleFunc("GET /api/v1/media/{id}/download", downloadHandler(s.TokenGen, s.LibQuerySvc, s.BaseURL, s.LibraryAccessRepo))
	mux.HandleFunc("POST /api/v1/media/{id}/stream-token", streamTokenHandler(s.TokenGen, s.StreamTokenStore, s.LibraryAccessRepo, s.LibQuerySvc))

	transcodeThrottle := apptranscoding.NewTranscodeThrottle()
	mux.HandleFunc("GET /api/v1/media/{id}/transcode", transcodeHandler(transcodeServe{
		tokens: s.TokenGen, libSvc: s.LibQuerySvc, baseURL: s.BaseURL,
		libraryAccess: s.LibraryAccessRepo, throttle: transcodeThrottle, userMaxStreams: s.UserMaxStreams,
	}))
	mux.HandleFunc("GET /api/v1/media/{id}/subtitle/{track}", subtitleHandler(s.TokenGen, s.LibQuerySvc, s.BaseURL, s.LibraryAccessRepo))
	mux.HandleFunc("GET /api/v1/media/{id}/chapters", chaptersHandler(s.TokenGen, s.LibCmdSvc, s.BaseURL, s.LibraryAccessRepo, s.LibQuerySvc))
	mux.HandleFunc("GET /api/v1/media/{id}/trickplay", trickplayManifestHandler(s.TokenGen, s.LibCmdSvc, s.BaseURL, s.LibraryAccessRepo, s.LibQuerySvc))
	mux.HandleFunc("GET /api/v1/media/{id}/trickplay/{image}", trickplayImageHandler(s.TokenGen, s.LibCmdSvc, s.BaseURL, s.LibraryAccessRepo, s.LibQuerySvc))
	mux.HandleFunc("POST /api/v1/admin/media/{id}/trickplay/generate", requireAdmin(s.TokenGen, ratelimit.MiddlewareFunc(s.AdminHeavyRL, trickplayGenerateHandler(s.TokenGen, s.LibCmdSvc, s.ServerCtx))))
	mux.HandleFunc("POST /api/v1/admin/trickplay/generate-all", requireAdmin(s.TokenGen, ratelimit.MiddlewareFunc(s.AdminHeavyRL, trickplayGenerateAllHandler(s.TokenGen, s.LibCmdSvc))))
	mux.HandleFunc("GET /api/v1/media/{id}/playback-info", playbackInfoHandler(s.TokenGen, s.LibQuerySvc, s.LibraryAccessRepo))
	mux.HandleFunc("GET /api/v1/media/{id}/hls/master.m3u8", hlsMasterHandler(hlsMasterServe{
		tokens: s.TokenGen, libSvc: s.LibQuerySvc, hlsManager: s.HLSManager,
		libraryAccess: s.LibraryAccessRepo, userMaxStreams: s.UserMaxStreams,
		audioNormalizeGetter: s.AudioNormalizeGetter, bitrateGetter: s.UserBitrate,
	}))
	hlsMediaCache := NewCachedMediaItemGetter(s.LibQuerySvc)
	hlsAccessCache := NewCachedLibraryAccessChecker(s.LibraryAccessRepo)
	mux.HandleFunc("GET /api/v1/media/{id}/hls/{file}", hlsSegmentHandler(s.TokenGen, hlsMediaCache, s.HLSManager, hlsAccessCache))
	mux.HandleFunc("GET /api/v1/admin/library-stats", requireAdmin(s.TokenGen, libraryStatsHandler(s.TokenGen, s.LibQuerySvc)))

	if gwMux != nil {
		mux.HandleFunc("POST /api/v1/auth/login", ratelimit.MiddlewareFunc(s.LoginRL, func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			authCookieMiddleware(gwMux, s.HTTPSOnly).ServeHTTP(w, r)
		}))
		mux.HandleFunc("GET /api/v1/auth/quick-connect/{code}", func(w http.ResponseWriter, r *http.Request) {
			authCookieMiddleware(gwMux, s.HTTPSOnly).ServeHTTP(w, r)
		})
		mux.HandleFunc("POST /api/v1/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
			r = injectRefreshTokenFromCookie(r)
			authCookieMiddleware(gwMux, s.HTTPSOnly).ServeHTTP(w, r)
		})
		mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
			clearRefreshCookie(w, s.HTTPSOnly)
			if s.OIDCSvc != nil {
				if ep := s.OIDCSvc.EndSessionEndpoint(); ep != "" {
					rec := &discardResponseWriter{header: make(http.Header)}
					gwMux.ServeHTTP(rec, r)

					// RP-Initiated Logout: build end_session URL with id_token_hint
					q := url.Values{}
					q.Set("post_logout_redirect_uri", s.BaseURL+"/login")
					if c, err := r.Cookie(oidcIDTokenCookieName); err == nil && c.Value != "" {
						q.Set("id_token_hint", c.Value)
					}
					clearOIDCIDTokenCookie(w, s.HTTPSOnly)
					http.Redirect(w, r, ep+"?"+q.Encode(), http.StatusFound)
					return
				}
			}
			clearOIDCIDTokenCookie(w, s.HTTPSOnly)
			gwMux.ServeHTTP(w, r)
		})
	}

	registerFeedRoutes(mux, s.FeedCmdSvc, s.FeedQuerySvc, s.TokenGen)
	registerAdminRoutes(mux, s.MetadataCmdSvc, s.MetadataQuerySvc, s.TokenGen, s.AdminHeavyRL, s.ServerCtx, s.ActivityCmdSvc)
	registerUserAdminRoutes(mux, s.UserSvc, s.UserQuerySvc, s.TokenGen, s.ActivityCmdSvc)
	registerFavoriteRoutes(mux, s.FavoriteSvc, s.LibQuerySvc, s.TokenGen)
	registerPhotoRoutes(mux, s.PhotoSvc, s.TokenGen, s.LibraryAccessRepo, s.LibQuerySvc, s.ServerCtx)
	registerMetadataOverrideRoutes(mux, s.MetadataCmdSvc, s.MetadataQuerySvc, s.TokenGen)
	registerTranscodeCacheRoutes(mux, s.TranscodingSvc, s.TokenGen, s.LibQuerySvc, s.ActivityCmdSvc)
	registerTranscodeProfileRoutes(mux, s.TranscodingSvc, s.TokenGen, s.ActivityCmdSvc)
	registerCollectionRoutes(mux, s.CollectionSvc, s.TokenGen)
	registerScheduleRoutes(mux, s.Scheduler, s.TokenGen)
	registerLibraryAccessRoutes(mux, s.LibraryAccessRepo, s.LibQuerySvc, s.TokenGen)
	registerPlaylistRoutes(mux, s.PlaylistSvc, s.TokenGen)

	registerSyncPlayRoutes(mux, s.SyncPlayHub, s.TokenGen, s.LibQuerySvc, s.LibraryAccessRepo)
	if s.ShareSvc != nil {
		registerShareRoutes(mux, s.TokenGen, s.ShareSvc, s.LibQuerySvc, s.LibraryAccessRepo)
	}
	registerBackupRoutes(mux, s.BackupSvc, s.TokenGen, s.AdminHeavyRL)
	registerRemoteAccessRoutes(mux, s.Cfg, s.TokenGen, s.CfgStore)
	registerPlaybackSettingsRoutes(mux, s.Cfg, s.TokenGen)
	registerParentalRoutes(mux, s.ProfileRepo, s.Hasher, s.TokenGen)
	registerRatingRoutes(mux, s.UserRatingRepo, s.TokenGen, s.LibQuerySvc, s.LibraryAccessRepo)
	registerStarRatingRoutes(mux, s.StarRatingSvc, s.TokenGen, s.LibQuerySvc, s.LibraryAccessRepo)
	registerPopularRoute(mux, s.ActivityQuerySvc, s.LibQuerySvc, s.TokenGen)
	registerAnalyticsRoutes(mux, s.ActivityQuerySvc, s.TokenGen)
	registerFilterRoutes(mux, s.LibQuerySvc, s.TokenGen, s.LibraryAccessRepo)
	registerSystemStatsRoute(mux, s.StartTime, s.TokenGen)
	registerServerConfigRoutes(mux, s.ServerCfgRepo, s.TokenGen, s.ActivityCmdSvc)
	registerLogsRoute(mux, s.LogDir, s.TokenGen)
	registerAvailableRuntimesRoute(mux, s.TokenGen)
	registerMusicRoutes(mux, s.MusicSvc, s.TokenGen, s.LibraryAccessRepo, s.LibQuerySvc)
	registerPushRoutes(mux, s.PushSubRepo, s.ProfileRepo, s.UserRepo, s.TokenGen, s.VAPIDPublicKey)

	if s.SubtitleSvc != nil {
		registerSubtitleRoutes(mux, s.SubtitleSvc, s.TokenGen, s.LibQuerySvc, s.LibraryAccessRepo)
	}

	if s.PluginRepo != nil && s.PluginDir != "" {
		registerPluginAssetRoutes(mux, s.TokenGen, s.PluginRepo, s.PluginDir)
		if s.PluginSvc != nil {
			registerPluginUploadRoute(mux, s.TokenGen, s.PluginSvc)
			registerPluginPreviewRoute(mux, s.TokenGen, s.PluginSvc)
		}
	}

	mux.HandleFunc("GET /api/v1/admin/sessions/active", requireAdmin(s.TokenGen, activeSessionsHandler(s.TokenGen, s.PlaybackSvc)))
	mux.HandleFunc("DELETE /api/v1/admin/sessions/{id}", requireAdmin(s.TokenGen, killSessionHandler(s.TokenGen, s.PlaybackSvc)))

	if s.OIDCSvc != nil {
		registerOIDCRoutes(mux, s.OIDCSvc, s.HTTPSOnly)
	} else {
		mux.HandleFunc("GET /api/v1/auth/oidc/config", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`{"enabled":false}`)); err != nil {
				return
			}
		})
	}

	if gwMux != nil {
		mux.Handle("/", ratelimit.Middleware(s.GlobalRL,
			"/api/v1/playback/progress",
			"/api/v1/playback/start",
			"/api/v1/playback/stop",
		)(apiCacheMiddleware(gwMux)))
	}
}
