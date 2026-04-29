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

	appactivity "github.com/evey-project/evey/internal/app/activity"
	appbackup "github.com/evey-project/evey/internal/app/backup"
	appcollection "github.com/evey-project/evey/internal/app/collection"
	appfavorite "github.com/evey-project/evey/internal/app/favorite"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmedia "github.com/evey-project/evey/internal/app/media"
	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	appmusic "github.com/evey-project/evey/internal/app/music"
	appphoto "github.com/evey-project/evey/internal/app/photo"
	appplaylist "github.com/evey-project/evey/internal/app/playlist"
	apprating "github.com/evey-project/evey/internal/app/rating"
	appsubtitle "github.com/evey-project/evey/internal/app/subtitle"
	appcast "github.com/evey-project/evey/internal/app/chromecast"
	appdlna "github.com/evey-project/evey/internal/app/dlna"
	appsyncplay "github.com/evey-project/evey/internal/app/syncplay"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/pkg/quickconnecthub"
	"github.com/evey-project/evey/internal/config"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/notification"
	domainplugin "github.com/evey-project/evey/internal/domain/plugin"
	domainshared "github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/i18n"
	eveymetrics "github.com/evey-project/evey/internal/pkg/metrics"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
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
	PhotoSvc        *appphoto.Service
	PhotoTranscoder PhotoTranscoder
	PhotoCacheDir   string

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
	PluginRepo                 domainplugin.PluginRepository
	PluginSvc                  pluginManager
	PluginDir                  string
	CSSSanitizer               cssSanitizer
	PluginSecurityDashboardSvc pluginSecurityDashboardProvider

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
	ProfilePrefs  ProfilePrefsGetter

	// Frame extraction (for admin snapshot preview)
	FrameExtractor FrameExtractor

	// IntroDetector triggers async intro/outro detection for a media item.
	IntroDetector IntroDetectionTrigger

	// WebDir is the path to the built frontend dist (e.g. /srv).
	// If set and the directory exists, the server serves the SPA from it.
	WebDir string
}

// RegisterRoutes wires all HTTP routes onto mux.
// gwMux is the grpc-gateway handler (may be nil if gateway failed).
func (s *HTTPServer) RegisterRoutes(mux *http.ServeMux, gwMux http.Handler) {
	auth := httpAuthMiddleware(s.TokenGen)
	admin := httpAdminMiddleware(s.TokenGen)

	mux.HandleFunc("GET /metrics", admin(func(w http.ResponseWriter, r *http.Request) {
		eveymetrics.Handler().ServeHTTP(w, r)
	}))
	mux.HandleFunc("/healthz", s.healthzHandler)

	if s.Translator != nil {
		mux.HandleFunc("GET /api/v1/i18n/{locale}", i18nHandler(s.Translator, s.Cfg.Log.DevMode))
	}

	mux.HandleFunc("GET /api/v1/admin/fs/browse", admin(browseDirsHandler(s.TokenGen)))

	var mhOpts []mediaServeOption
	if s.ImageConverter != nil {
		mhOpts = append(mhOpts, withImageConverter(s.ImageConverter, s.ArtworkRepo))
	}
	mh := mediaHandler(s.TokenGen, s.LibQuerySvc, s.ArtworkRepo, s.BaseURL(), s.LibraryAccessRepo, mhOpts...)
	mux.HandleFunc("GET /api/v1/media/{id}/stream", auth(mh))
	// Artwork is public (non-sensitive): no auth required.
	mux.HandleFunc("GET /api/v1/media/{id}/artwork/{artType}", mh)
	mux.HandleFunc("POST /api/v1/media/{id}/stream", auth(mh))
	mux.HandleFunc("POST /api/v1/media/{id}/artwork/{artworkId}/select", auth(selectArtworkHandler(s.TokenGen, s.ArtworkRepo, s.LibQuerySvc, s.LibraryAccessRepo)))
	mux.HandleFunc("GET /api/v1/media/{id}/download", auth(downloadHandler(s.TokenGen, s.LibQuerySvc, s.BaseURL(), s.LibraryAccessRepo)))
	mux.HandleFunc("POST /api/v1/media/{id}/stream-token", auth(streamTokenHandler(s.TokenGen, s.StreamTokenStore, s.LibraryAccessRepo, s.LibQuerySvc)))

	transcodeThrottle := apptranscoding.NewTranscodeThrottle()
	mux.HandleFunc("GET /api/v1/media/{id}/transcode", auth(transcodeHandler(transcodeServe{
		tokens: s.TokenGen, libSvc: s.LibQuerySvc, baseURL: s.BaseURL(),
		libraryAccess: s.LibraryAccessRepo, throttle: transcodeThrottle, userMaxStreams: s.UserMaxStreams,
	})))
	mux.HandleFunc("GET /api/v1/media/{id}/subtitle/{track}", auth(subtitleHandler(s.TokenGen, s.LibQuerySvc, s.BaseURL(), s.LibraryAccessRepo)))
	mux.HandleFunc("GET /api/v1/media/{id}/chapters", auth(chaptersHandler(s.TokenGen, s.LibQuerySvc, s.BaseURL(), s.LibraryAccessRepo, s.LibQuerySvc)))
	mux.HandleFunc("GET /api/v1/media/{id}/segments", auth(segmentsHandler(s.TokenGen, s.LibQuerySvc, s.BaseURL(), s.LibraryAccessRepo, s.LibQuerySvc)))
	// Trickplay (manifest + sprite images) is public (non-sensitive): no auth required.
	mux.HandleFunc("GET /api/v1/media/{id}/trickplay", trickplayManifestHandler(s.TokenGen, s.LibQuerySvc, s.BaseURL(), s.LibraryAccessRepo, s.LibQuerySvc))
	mux.HandleFunc("GET /api/v1/media/{id}/trickplay/{image}", trickplayImageHandler(s.TokenGen, s.LibQuerySvc, s.BaseURL(), s.LibraryAccessRepo, s.LibQuerySvc))
	mux.HandleFunc("GET /api/v1/media/{id}/snapshot", auth(snapshotHandler(s.TokenGen, s.FrameExtractor, s.LibraryAccessRepo, s.LibQuerySvc, s.BaseURL())))
	mux.HandleFunc("POST /api/v1/admin/media/{id}/trickplay/generate", admin(ratelimit.MiddlewareFunc(s.AdminHeavyRL, trickplayGenerateHandler(s.TokenGen, s.LibCmdSvc, s.ServerCtx))))
	mux.HandleFunc("POST /api/v1/admin/trickplay/generate-all", admin(ratelimit.MiddlewareFunc(s.AdminHeavyRL, trickplayGenerateAllHandler(s.TokenGen, s.LibCmdSvc))))
	if s.IntroDetector != nil {
		mux.HandleFunc("POST /api/v1/admin/media/{id}/detect-segments", introDetectionHandler(s.TokenGen, s.IntroDetector))
		mux.HandleFunc("PUT /api/v1/admin/media/{id}/segments", setSegmentHandler(s.TokenGen, s.IntroDetector, s.LibraryAccessRepo, s.LibQuerySvc))
		mux.HandleFunc("DELETE /api/v1/admin/media/{id}/segments", deleteSegmentHandler(s.TokenGen, s.IntroDetector, s.LibraryAccessRepo, s.LibQuerySvc))
		mux.HandleFunc("POST /api/v1/admin/media/{id}/segments/propagate", propagateSegmentHandler(s.TokenGen, s.IntroDetector, s.LibraryAccessRepo, s.LibQuerySvc))
	}
	mux.HandleFunc("GET /api/v1/media/{id}/playback-info", auth(playbackInfoHandler(s.TokenGen, s.LibQuerySvc, s.LibraryAccessRepo, s.ProfilePrefs)))
	mux.HandleFunc("GET /api/v1/media/{id}/hls/master.m3u8", auth(hlsMasterHandler(hlsMasterServe{
		tokens: s.TokenGen, libSvc: s.LibQuerySvc, hlsManager: s.HLSManager,
		libraryAccess: s.LibraryAccessRepo, userMaxStreams: s.UserMaxStreams,
		audioNormalizeGetter: s.AudioNormalizeGetter, bitrateGetter: s.UserBitrate,
		profilePrefs: s.ProfilePrefs,
	})))
	hlsMediaCache := NewCachedMediaItemGetter(s.LibQuerySvc)
	hlsAccessCache := NewCachedLibraryAccessChecker(s.LibraryAccessRepo)
	mux.HandleFunc("GET /api/v1/media/{id}/hls/{file}", auth(hlsSegmentHandler(s.TokenGen, hlsMediaCache, s.HLSManager, hlsAccessCache)))
	mux.HandleFunc("GET /api/v1/admin/library-stats", admin(libraryStatsHandler(s.TokenGen, s.LibQuerySvc, NewLibraryItemCounter(s.LibQuerySvc, s.MusicSvc, s.PhotoSvc))))

	// DLNA
	if s.DLNASvc != nil {
		mux.HandleFunc("GET /api/v1/dlna/renderers", auth(dlnaRenderersHandler(s.TokenGen, s.DLNASvc)))
		mux.HandleFunc("POST /api/v1/dlna/play", auth(dlnaPlayHandler(s.TokenGen, s.DLNASvc)))
		mux.HandleFunc("POST /api/v1/dlna/stop", auth(dlnaStopHandler(s.TokenGen, s.DLNASvc)))
		mux.HandleFunc("POST /api/v1/dlna/pause", auth(dlnaPauseHandler(s.TokenGen, s.DLNASvc)))
	}

	// Chromecast
	if s.CastSvc != nil {
		mux.HandleFunc("GET /api/v1/cast/devices", auth(castDevicesHandler(s.TokenGen, s.CastSvc)))
		mux.HandleFunc("POST /api/v1/cast/play", auth(castPlayHandler(s.TokenGen, s.CastSvc)))
		mux.HandleFunc("POST /api/v1/cast/stop", auth(castStopHandler(s.TokenGen, s.CastSvc)))
		mux.HandleFunc("POST /api/v1/cast/pause", auth(castPauseHandler(s.TokenGen, s.CastSvc)))
		mux.HandleFunc("POST /api/v1/cast/seek", auth(castSeekHandler(s.TokenGen, s.CastSvc)))
		mux.HandleFunc("GET /api/v1/cast/status", auth(castStatusHandler(s.TokenGen, s.CastSvc)))
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
			// Logout is best-effort: cookie cleared above. If there's no auth,
			// don't call the gateway (it would return 401 → frontend retry loops).
			if r.Header.Get("Authorization") == "" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
				return
			}
			gwMux.ServeHTTP(w, r)
		})
	}

	// JSON CRUD → gRPC: favorites, playlists, metadata overrides, transcode cache/profiles, library access, admin
	registerPhotoRoutes(mux, s.PhotoSvc, s.TokenGen, s.LibraryAccessRepo, s.LibQuerySvc, s.ServerCtx, s.PhotoTranscoder, s.PhotoCacheDir) // binary: photo/serve
	registerSyncPlayRoutes(mux, s.SyncPlayHub, s.TokenGen, s.LibQuerySvc, s.LibraryAccessRepo) // binary: WebSocket
	registerBackupRoutes(mux, s.BackupSvc, s.TokenGen, s.AdminHeavyRL, s.AdminWriteRL) // binary: download/restore
	registerRemoteAccessRoutes(mux, s.Cfg, s.TokenGen, s.CfgStore, s.AdminWriteRL)
	registerServerSettingsRoutes(mux, s.Cfg, s.CfgStore, s.TokenGen, s.TranscodingSvc, s.HLSManager, s.LoginRL, s.GlobalRL, s.AdminWriteRL)
	registerPlaybackSettingsRoutes(mux, s.Cfg, s.TokenGen, s.AdminWriteRL)
	registerParentalRoutes(mux, s.ProfileRepo, s.UserRepo, s.Hasher, s.TokenGen)
	if s.DataDir != "" {
		registerAvatarRoutes(mux, s.ProfileRepo, s.TokenGen, s.DataDir)
	}
	if s.MetadataCmdSvc != nil && s.MetadataQuerySvc != nil {
		registerAdminRoutes(mux, s.MetadataCmdSvc, s.MetadataQuerySvc, s.TokenGen, s.AdminHeavyRL, s.AdminWriteRL, s.ServerCtx, s.ActivityCmdSvc)
	}
	if s.MetadataCmdSvc != nil {
		mux.HandleFunc("POST /api/v1/admin/translations/refresh-all", admin(func(w http.ResponseWriter, r *http.Request) {
			go func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("panic in RefreshAllTranslations goroutine", slog.Any("recover", r))
					}
				}()
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
		registerPluginAssetRoutes(mux, s.TokenGen, s.PluginRepo, s.PluginDir, s.CSSSanitizer)
		registerThemeAssetRoutes(mux, s.TokenGen, s.PluginRepo, s.PluginDir)
		if s.PluginSvc != nil {
			registerPluginUploadRoute(mux, s.TokenGen, s.PluginSvc, s.AdminWriteRL)
			registerPluginPreviewRoute(mux, s.TokenGen, s.PluginSvc, s.AdminWriteRL)
		}
	}
	if s.PluginSecurityDashboardSvc != nil {
		registerPluginSecurityDashboardRoute(mux, s.TokenGen, s.PluginSecurityDashboardSvc)
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
