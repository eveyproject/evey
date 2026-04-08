package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	appbackup "github.com/gravity-zero/evey/internal/app/backup"
	appcollection "github.com/gravity-zero/evey/internal/app/collection"
	appfavorite "github.com/gravity-zero/evey/internal/app/favorite"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appmedia "github.com/gravity-zero/evey/internal/app/media"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	appmusic "github.com/gravity-zero/evey/internal/app/music"
	appphoto "github.com/gravity-zero/evey/internal/app/photo"
	appplaylist "github.com/gravity-zero/evey/internal/app/playlist"
	apprating "github.com/gravity-zero/evey/internal/app/rating"
	appsubtitle "github.com/gravity-zero/evey/internal/app/subtitle"
	appcast "github.com/gravity-zero/evey/internal/app/chromecast"
	appdlna "github.com/gravity-zero/evey/internal/app/dlna"
	appsyncplay "github.com/gravity-zero/evey/internal/app/syncplay"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	"github.com/gravity-zero/evey/internal/pkg/quickconnecthub"
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
	BaseURL   func() string
	Version   string
	StartTime time.Time
	LogDir    string
	DataDir   string
	HTTPSOnly bool

	// DB is used for liveness checks; may be nil (check skipped).
	DB DBPinger

	// SearchHealth is used for health checks; nil means search not configured.
	SearchHealth SearchHealthChecker

	TokenGen         *auth.JWTGenerator
	Cfg              *config.Config
	StreamTokenStore *appmedia.StreamTokenStore

	// Library
	LibCmdSvc   *applibrary.CommandService
	LibQuerySvc *applibrary.QueryService
	Scheduler   *applibrary.Scheduler

	// User
	OIDCSvc         *appuser.OIDCService
	UserAuthSvc     *appuser.CommandService
	UserQuerySvc    *appuser.QueryService
	QuickConnectHub *quickconnecthub.Hub

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

	// DLNA
	DLNASvc *appdlna.Service

	// Chromecast
	CastSvc *appcast.Service

	// Config store for remote access persistence
	CfgStore remoteAccessPersister

	// Server context (for long-running background ops like photo scan)
	ServerCtx context.Context

	// Rate limiters passed in from main
	LoginRL          *ratelimit.Limiter
	GlobalRL         *ratelimit.Limiter
	AdminHeavyRL     *ratelimit.Limiter
	AdminWriteRL     *ratelimit.Limiter
	PasswordResetRL  *ratelimit.Limiter
	OIDCLoginRL      *ratelimit.Limiter

	// i18n
	Translator *i18n.Translator

	// audio normalize getter (closure capturing serverCfgRepo)
	AudioNormalizeGetter func(ctx context.Context) bool

	// User stream/bitrate adapters
	UserMaxStreams UserMaxStreamsGetter
	UserBitrate   UserBitrateGetter

	// Frame extraction (for admin snapshot preview)
	FrameExtractor FrameExtractor

	// WebDir is the path to the built frontend dist (e.g. /srv).
	// If set and the directory exists, the server serves the SPA from it.
	WebDir string
}

// RegisterRoutes wires all HTTP routes onto mux.
// gwMux is the grpc-gateway handler (may be nil if gateway failed).
func (s *HTTPServer) RegisterRoutes(mux *http.ServeMux, gwMux http.Handler) {
	mux.Handle("GET /metrics", requireAdmin(s.TokenGen, func(w http.ResponseWriter, r *http.Request) {
		eveymetrics.Handler().ServeHTTP(w, r)
	}))
	mux.HandleFunc("/healthz", s.healthzHandler)

	if s.Translator != nil {
		mux.HandleFunc("GET /api/v1/i18n/{locale}", i18nHandler(s.Translator, s.Cfg.Log.DevMode))
	}

	mux.HandleFunc("GET /api/v1/admin/fs/browse", requireAdmin(s.TokenGen, browseDirsHandler(s.TokenGen)))

	var mhOpts []mediaServeOption
	if s.ImageConverter != nil {
		mhOpts = append(mhOpts, withImageConverter(s.ImageConverter, s.ArtworkRepo))
	}
	mh := mediaHandler(s.TokenGen, s.LibQuerySvc, s.ArtworkRepo, s.BaseURL(), s.LibraryAccessRepo, mhOpts...)
	mux.HandleFunc("GET /api/v1/media/{id}/stream", mh)
	mux.HandleFunc("GET /api/v1/media/{id}/artwork/{artType}", mh)
	mux.HandleFunc("POST /api/v1/media/{id}/stream", mh)
	mux.HandleFunc("POST /api/v1/media/{id}/artwork/{artworkId}/select", selectArtworkHandler(s.TokenGen, s.ArtworkRepo, s.LibQuerySvc, s.LibraryAccessRepo))
	mux.HandleFunc("GET /api/v1/media/{id}/download", downloadHandler(s.TokenGen, s.LibQuerySvc, s.BaseURL(), s.LibraryAccessRepo))
	mux.HandleFunc("POST /api/v1/media/{id}/stream-token", streamTokenHandler(s.TokenGen, s.StreamTokenStore, s.LibraryAccessRepo, s.LibQuerySvc))

	transcodeThrottle := apptranscoding.NewTranscodeThrottle()
	mux.HandleFunc("GET /api/v1/media/{id}/transcode", transcodeHandler(transcodeServe{
		tokens: s.TokenGen, libSvc: s.LibQuerySvc, baseURL: s.BaseURL(),
		libraryAccess: s.LibraryAccessRepo, throttle: transcodeThrottle, userMaxStreams: s.UserMaxStreams,
	}))
	mux.HandleFunc("GET /api/v1/media/{id}/subtitle/{track}", subtitleHandler(s.TokenGen, s.LibQuerySvc, s.BaseURL(), s.LibraryAccessRepo))
	mux.HandleFunc("GET /api/v1/media/{id}/chapters", chaptersHandler(s.TokenGen, s.LibCmdSvc, s.BaseURL(), s.LibraryAccessRepo, s.LibQuerySvc))
	mux.HandleFunc("GET /api/v1/media/{id}/trickplay", trickplayManifestHandler(s.TokenGen, s.LibCmdSvc, s.BaseURL(), s.LibraryAccessRepo, s.LibQuerySvc))
	mux.HandleFunc("GET /api/v1/media/{id}/trickplay/{image}", trickplayImageHandler(s.TokenGen, s.LibCmdSvc, s.BaseURL(), s.LibraryAccessRepo, s.LibQuerySvc))
	mux.HandleFunc("GET /api/v1/media/{id}/snapshot", snapshotHandler(s.TokenGen, s.FrameExtractor, s.LibraryAccessRepo, s.LibQuerySvc, s.BaseURL()))
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

	// DLNA
	if s.DLNASvc != nil {
		mux.HandleFunc("GET /api/v1/dlna/renderers", dlnaRenderersHandler(s.TokenGen, s.DLNASvc))
		mux.HandleFunc("POST /api/v1/dlna/play", dlnaPlayHandler(s.TokenGen, s.DLNASvc))
		mux.HandleFunc("POST /api/v1/dlna/stop", dlnaStopHandler(s.TokenGen, s.DLNASvc))
		mux.HandleFunc("POST /api/v1/dlna/pause", dlnaPauseHandler(s.TokenGen, s.DLNASvc))
	}

	// Chromecast
	if s.CastSvc != nil {
		mux.HandleFunc("GET /api/v1/cast/devices", castDevicesHandler(s.TokenGen, s.CastSvc))
		mux.HandleFunc("POST /api/v1/cast/play", castPlayHandler(s.TokenGen, s.CastSvc))
		mux.HandleFunc("POST /api/v1/cast/stop", castStopHandler(s.TokenGen, s.CastSvc))
		mux.HandleFunc("POST /api/v1/cast/pause", castPauseHandler(s.TokenGen, s.CastSvc))
		mux.HandleFunc("POST /api/v1/cast/seek", castSeekHandler(s.TokenGen, s.CastSvc))
		mux.HandleFunc("GET /api/v1/cast/status", castStatusHandler(s.TokenGen, s.CastSvc))
	}

	if gwMux != nil {
		mux.HandleFunc("POST /api/v1/auth/login", ratelimit.MiddlewareFunc(s.LoginRL, func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			authCookieMiddleware(gwMux, s.HTTPSOnly).ServeHTTP(w, r)
		}))
		mux.HandleFunc("POST /api/v1/auth/forgot-password", ratelimit.MiddlewareFunc(s.PasswordResetRL, func(w http.ResponseWriter, r *http.Request) {
			gwMux.ServeHTTP(w, r)
		}))
		mux.HandleFunc("POST /api/v1/auth/reset-password", ratelimit.MiddlewareFunc(s.PasswordResetRL, func(w http.ResponseWriter, r *http.Request) {
			gwMux.ServeHTTP(w, r)
		}))
		mux.HandleFunc("GET /api/v1/auth/quick-connect/{code}", ratelimit.MiddlewareFunc(s.LoginRL, func(w http.ResponseWriter, r *http.Request) {
			authCookieMiddleware(gwMux, s.HTTPSOnly).ServeHTTP(w, r)
		}))
		if s.QuickConnectHub != nil {
			mux.HandleFunc("GET /api/v1/auth/quick-connect/{code}/events", quickConnectSSEHandler(s.QuickConnectHub, &qcCheckerAdapter{query: s.UserQuerySvc, auth: s.UserAuthSvc, cfg: s.Cfg}))
		}
		mux.HandleFunc("POST /api/v1/auth/refresh", ratelimit.MiddlewareFunc(s.LoginRL, func(w http.ResponseWriter, r *http.Request) {
			r = injectRefreshTokenFromCookie(r)
			authCookieMiddleware(gwMux, s.HTTPSOnly).ServeHTTP(w, r)
		}))
		mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
			clearRefreshCookie(w, s.HTTPSOnly)
			if s.OIDCSvc != nil {
				if ep := s.OIDCSvc.EndSessionEndpoint(); ep != "" {
					rec := &discardResponseWriter{header: make(http.Header)}
					gwMux.ServeHTTP(rec, r)

					// RP-Initiated Logout: build end_session URL with id_token_hint
					q := url.Values{}
					q.Set("post_logout_redirect_uri", s.BaseURL()+"/login")
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

	// JSON CRUD → gRPC: favorites, playlists, metadata overrides, transcode cache/profiles, library access, admin
	registerPhotoRoutes(mux, s.PhotoSvc, s.TokenGen, s.LibraryAccessRepo, s.LibQuerySvc, s.ServerCtx) // binary: photo/serve
	registerSyncPlayRoutes(mux, s.SyncPlayHub, s.TokenGen, s.LibQuerySvc, s.LibraryAccessRepo) // binary: WebSocket
	registerBackupRoutes(mux, s.BackupSvc, s.TokenGen, s.AdminHeavyRL, s.AdminWriteRL) // binary: download/restore
	registerRemoteAccessRoutes(mux, s.Cfg, s.TokenGen, s.CfgStore, s.AdminWriteRL)
	registerServerSettingsRoutes(mux, s.Cfg, s.CfgStore, s.TokenGen, s.TranscodingSvc, s.HLSManager, s.LoginRL, s.GlobalRL, s.AdminWriteRL)
	registerPlaybackSettingsRoutes(mux, s.Cfg, s.TokenGen, s.AdminWriteRL)
	registerParentalRoutes(mux, s.ProfileRepo, s.Hasher, s.TokenGen)
	if s.DataDir != "" {
		registerAvatarRoutes(mux, s.ProfileRepo, s.TokenGen, s.DataDir)
	}
	if s.MetadataCmdSvc != nil {
		mux.HandleFunc("POST /api/v1/admin/translations/refresh-all", requireAdmin(s.TokenGen, func(w http.ResponseWriter, r *http.Request) {
			go func() {
				if err := s.MetadataCmdSvc.RefreshAllTranslations(s.ServerCtx, nil); err != nil {
					slog.Error("refresh all translations", slog.Any("err", err))
				} else {
					slog.Info("refresh all translations complete")
				}
			}()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"started"}`))
		}))
	}
	// Ratings, popular, analytics, filter → gRPC (grpcapi/rating_handler, activity_handler, library_handler)
	registerLogsRoute(mux, s.LogDir, s.TokenGen)
	registerAvailableRuntimesRoute(mux, s.TokenGen)
	registerMusicRoutes(mux, s.MusicSvc, s.TokenGen, s.LibraryAccessRepo, s.LibQuerySvc)


	if s.SubtitleSvc != nil {
		registerSubtitleRoutes(mux, s.SubtitleSvc, s.TokenGen, s.LibQuerySvc, s.LibraryAccessRepo)
	}

	if s.PluginRepo != nil && s.PluginDir != "" {
		registerPluginAssetRoutes(mux, s.TokenGen, s.PluginRepo, s.PluginDir)
		if s.PluginSvc != nil {
			registerPluginUploadRoute(mux, s.TokenGen, s.PluginSvc, s.AdminWriteRL)
			registerPluginPreviewRoute(mux, s.TokenGen, s.PluginSvc, s.AdminWriteRL)
		}
	}


	if s.OIDCSvc != nil {
		registerOIDCRoutes(mux, s.OIDCSvc, s.HTTPSOnly, s.OIDCLoginRL)
	} else {
		mux.HandleFunc("GET /api/v1/auth/oidc/config", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`{"enabled":false}`)); err != nil {
				return
			}
		})
	}

	if gwMux != nil {
		gwHandler := ratelimit.Middleware(s.GlobalRL,
			"/api/v1/playback/progress",
			"/api/v1/playback/start",
			"/api/v1/playback/stop",
		)(apiCacheMiddleware(gwMux))

		webDirOK := false
		if s.WebDir != "" {
			if _, err := os.Stat(filepath.Join(s.WebDir, "index.html")); err == nil {
				webDirOK = true
			}
		}
		if webDirOK {
			// Serve frontend SPA for non-API paths, API goes to gRPC gateway
			spa := spaHandler(s.WebDir)
			mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/api/") {
					gwHandler.ServeHTTP(w, r)
					return
				}
				spa.ServeHTTP(w, r)
			}))
		} else {
			mux.Handle("/", gwHandler)
		}
	}
}
