package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	appactivity "github.com/evey-project/evey/internal/app/activity"
	appbackup "github.com/evey-project/evey/internal/app/backup"
	appcast "github.com/evey-project/evey/internal/app/chromecast"
	appcollection "github.com/evey-project/evey/internal/app/collection"
	appconfig "github.com/evey-project/evey/internal/app/config"
	appdlna "github.com/evey-project/evey/internal/app/dlna"
	appfavorite "github.com/evey-project/evey/internal/app/favorite"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmedia "github.com/evey-project/evey/internal/app/media"
	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	appmusic "github.com/evey-project/evey/internal/app/music"
	appphoto "github.com/evey-project/evey/internal/app/photo"
	appplaylist "github.com/evey-project/evey/internal/app/playlist"
	appqoe "github.com/evey-project/evey/internal/app/qoe"
	apprating "github.com/evey-project/evey/internal/app/rating"
	appsubtitle "github.com/evey-project/evey/internal/app/subtitle"
	appsyncplay "github.com/evey-project/evey/internal/app/syncplay"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/config"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/notification"
	domainplugin "github.com/evey-project/evey/internal/domain/plugin"
	domainshared "github.com/evey-project/evey/internal/domain/shared"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/i18n"
	eveymetrics "github.com/evey-project/evey/internal/pkg/metrics"
	"github.com/evey-project/evey/internal/pkg/netmeter"
	"github.com/evey-project/evey/internal/pkg/quickconnecthub"
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

	// TokenGen validates and mints user access tokens. An interface so wire
	// can inject the epoch-guarded decorator (token_epoch revocation) rather
	// than the raw JWT generator.
	TokenGen domainuser.TokenGenerator
	// PluginTokenGen parses scoped plugin tokens (outside user.TokenGenerator).
	PluginTokenGen   PluginTokenParser
	Cfg              *config.Config
	StreamTokenStore *appmedia.StreamTokenStore
	ShareSvc         *appmedia.ShareService

	// Library
	LibCmdSvc   *applibrary.CommandService
	LibQuerySvc *applibrary.QueryService
	Scheduler   *applibrary.Scheduler
	// Remuxer repackages EBML (mkv/webm) sources into fast-start MP4 for the
	// progressive /remux.mp4 path. Optional: nil disables the route.
	Remuxer Remuxer

	// User
	OIDCSvc         *appuser.OIDCService
	UserAuthSvc     *appuser.CommandService
	UserQuerySvc    *appuser.QueryService
	QuickConnectHub *quickconnecthub.Hub

	// Transcoding
	TranscodingSvc *apptranscoding.Service
	// HLSVODOrch owns the VOD pipeline's ffmpeg rafale sessions (cache
	// addressing, cancel-on-seek, inactivity GC). Also satisfies the legacy
	// HLSSessionRemover + ActiveStreamChecker contracts so admin kill-session
	// and the trickplay scheduler keep working without a separate HLSManager.
	HLSVODOrch    *apptranscoding.SegmentOrchestrator
	// HLSVODSoftware is the shared software StreamProducer (built in wire.go) so
	// the gRPC StopStream / admin-kill path can tear down the very sessions this
	// handler's routing selector creates. nil in hand-built test servers →
	// newHLSVODServe builds a private one.
	HLSVODSoftware *apptranscoding.SoftwareProducer
	HLSVODCache    *apptranscoding.SegmentCache
	HLSTranscoder  domaintranscoding.HLSProcess
	// AudioOrch manages audio-only HLS rendition sessions (EXT-X-MEDIA).
	// Nil-safe: the audio rendition routes return 404 when unset. Wiring
	// from wire.go lands after the feature flag stabilises.
	AudioOrch *apptranscoding.AudioSegmentOrchestrator
	// ComplexityRepo drives per-title ABR ladder selection. Nil-safe — the
	// VOD handler falls back to LadderFilmNormal when unset.
	ComplexityRepo domaintranscoding.ComplexityRepo
	// CalibrationSvc supplies the host's calibrated encode ceiling
	// (libx264 + optional HW). Nil-safe: the VOD handler stays on the
	// safe 720p floor when unset (tests).
	CalibrationSvc *apptranscoding.CalibrationService
	// CapOverrideSvc supplies the CMCD-feedback adaptive override on
	// top of the benchmark-set cap. Nil-safe: the VOD handler skips
	// the override branch when unset.
	CapOverrideSvc *apptranscoding.CapOverrideService
	// QoESink ingests CMCD telemetry from every segment / master / variant
	// request. Nil-safe: the VOD handler short-circuits when this is unset.
	QoESink appqoe.EventSink

	// CopySeekProber resolves where a pre-input -ss really lands for the copy
	// rung's mid-stream sessions (sparse container seek indexes land on the
	// preceding cue point). Nil-safe: without it, copy seeks assume an exact
	// landing (only safe for dense-cue sources).
	CopySeekProber apptranscoding.SeekLandingProber

	// transcodeCloser tears down the VOD software producers built during route
	// registration (the selector's SoftwareProducer + the optional copy
	// producer). They own long-lived ffmpeg sessions that would otherwise
	// outlive the process as orphans — nothing else closes them. The hardware
	// orchestrator is deliberately NOT in here: main already defers its
	// CleanupAll (an alias of Close), and Close is not idempotent. Set by
	// newHLSVODServe; invoked exactly once via CloseTranscodeProducers.
	transcodeCloser     func()
	transcodeCloserOnce sync.Once

	// QoEQuerySvc serves the admin dashboard for QoE telemetry. Nil-safe:
	// the route registration short-circuits and admins get a 404 instead of
	// a 500 when CMCD ingest isn't wired (tests, dev w/o Postgres).
	QoEQuerySvc *appqoe.QueryService

	// Metadata
	MetadataCmdSvc   *appmetadata.CommandService
	MetadataQuerySvc *appmetadata.QueryService

	// Activity
	ActivityQuerySvc *appactivity.QueryService
	ActivityCmdSvc   *appactivity.CommandService

	// Collections / Playlists / Favorites
	CollectionSvc  *appcollection.Service
	PlaylistSvc    *appplaylist.Service
	PlaylistGenSvc *appplaylist.GeneratorService
	FavoriteSvc    *appfavorite.Service

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

	// Play On — registry of first-party clients controllable via the server
	DeviceRegistry *appsyncplay.DeviceRegistry

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
	LoginRL  *ratelimit.Limiter
	GlobalRL *ratelimit.Limiter
	// StreamingRL is the per-profile request-rate limiter for the high-frequency
	// streaming routes (HLS segments/playlists). nil = disabled (pass-through).
	StreamingRL     *ratelimit.Limiter
	AdminHeavyRL    *ratelimit.Limiter
	AdminWriteRL    *ratelimit.Limiter
	PasswordResetRL *ratelimit.Limiter
	OIDCLoginRL     *ratelimit.Limiter
	// SnapshotRL caps the frame-extraction route — each request spawns an
	// ffmpeg, so it must not ride the generous global limiter.
	SnapshotRL *ratelimit.Limiter

	// i18n
	Translator *i18n.Translator

	// audio normalize getter (closure capturing serverCfgRepo)
	AudioNormalizeGetter func(ctx context.Context) bool

	// User stream/bitrate adapters
	UserMaxStreams UserMaxStreamsGetter
	UserBitrate    UserBitrateGetter
	ProfilePrefs   ProfilePrefsGetter

	// Frame extraction (for admin snapshot preview)
	FrameExtractor FrameExtractor

	// IntroDetector triggers async intro/outro detection for a media item.
	IntroDetector IntroDetectionTrigger

	// StreamMeter counts the bytes served by the streaming routes (direct
	// play, HLS, downloads) for the admin dashboard's measured bandwidth.
	// nil = no counting (middleware is a pass-through).
	StreamMeter *netmeter.Meter

	// WebDir is the path to the built frontend dist (e.g. /srv).
	// If set and the directory exists, the server serves the SPA from it.
	WebDir string
}

// RegisterRoutes wires all HTTP routes onto mux.
// gwMux is the grpc-gateway handler (may be nil if gateway failed).
// CloseTranscodeProducers kills the long-lived ffmpeg sessions owned by the
// VOD software/copy producers. Call once at shutdown (idempotent); no-op when
// the VOD pipeline was never wired (tests).
func (s *HTTPServer) CloseTranscodeProducers() {
	s.transcodeCloserOnce.Do(func() {
		if s.transcodeCloser != nil {
			s.transcodeCloser()
		}
	})
}

func (s *HTTPServer) RegisterRoutes(mux *http.ServeMux, gwMux http.Handler) {
	auth := httpAuthMiddleware(s.TokenGen)
	// streamAuth additionally accepts opaque stream tokens (?token=) — apply ONLY
	// to /api/v1/media/{id}/* playback routes the player loads as raw URLs.
	streamAuth := httpStreamAuthMiddleware(s.TokenGen)
	admin := httpAdminMiddleware(s.TokenGen)

	scrapeOrAdmin := httpScrapeOrAdminMiddleware(s.TokenGen, s.Cfg.Metrics.ScrapeToken)
	mux.HandleFunc("GET /metrics", scrapeOrAdmin(func(w http.ResponseWriter, r *http.Request) {
		eveymetrics.Handler().ServeHTTP(w, r)
	}))
	mux.HandleFunc("/healthz", s.healthzHandler)

	if s.Translator != nil {
		mux.HandleFunc("GET /api/v1/i18n/{locale}", i18nHandler(s.Translator, s.Cfg.Log.DevMode))
	}

	// OpenAPI spec for the public (API-key) surface, consumed by the in-app
	// Swagger UI linked from the API keys page.
	mux.HandleFunc("GET /api/v1/openapi.json", auth(openAPIPublicHandler()))

	mux.HandleFunc("GET /api/v1/admin/fs/browse", admin(browseDirsHandler(s.TokenGen)))

	// parentalChk gates the raw byte/decision/token stream handlers so a kids
	// profile that knows an adult media-id out of band can't bypass the gRPC
	// parental filter by hitting these endpoints directly. Gating the HLS master
	// (which creates the session) transitively protects the segment handler.
	parentalChk := NewParentalChecker(s.UserQuerySvc)

	mhOpts := []mediaServeOption{withParental(parentalChk), withLibraryLister(s.LibQuerySvc)}
	if s.ImageConverter != nil {
		mhOpts = append(mhOpts, withImageConverter(s.ImageConverter, s.ArtworkRepo))
	}
	mh := mediaHandler(s.TokenGen, s.LibQuerySvc, s.ArtworkRepo, s.BaseURL(), s.LibraryAccessRepo, mhOpts...)
	mux.HandleFunc("GET /api/v1/media/{id}/stream", streamAuth(s.StreamMeter.Middleware(mh)))
	// Progressive fast-start MP4 remux of a browser-incompatible EBML source
	// (copy, no re-encode), cached once and served with Range support. Same auth
	// + parental + stream metering as /stream.
	if s.Remuxer != nil {
		mux.HandleFunc("GET /api/v1/media/{id}/remux.mp4", streamAuth(s.StreamMeter.Middleware(remuxHandler(s.LibQuerySvc, s.LibraryAccessRepo, parentalChk, s.LibQuerySvc, s.Remuxer, s.Cfg.Transcoding.CacheDir, s.BaseURL()))))
	}
	// Artwork is public (non-sensitive): no auth required.
	mux.HandleFunc("GET /api/v1/media/{id}/artwork/{artType}", mh)
	mux.HandleFunc("POST /api/v1/media/{id}/stream", streamAuth(mh))
	mux.HandleFunc("POST /api/v1/media/{id}/artwork/{artworkId}/select", auth(selectArtworkHandler(s.TokenGen, s.ArtworkRepo, s.LibQuerySvc, s.LibraryAccessRepo)))
	mux.HandleFunc("GET /api/v1/media/{id}/download", streamAuth(ratelimit.MiddlewareFunc(s.GlobalRL, s.StreamMeter.Middleware(downloadHandler(s.TokenGen, s.LibQuerySvc, s.BaseURL(), s.LibraryAccessRepo, parentalChk)))))
	// Public share download: no auth — the uuid token is the capability.
	// Admin-only to CREATE (RBAC), but the resulting link must resolve without
	// a login. Rate-limited per-IP as defence-in-depth (the uuid entropy is the
	// real guard against token guessing).
	if s.ShareSvc != nil {
		mux.HandleFunc("GET /api/v1/share/{token}/download", ratelimit.MiddlewareFunc(s.GlobalRL, s.StreamMeter.Middleware(shareDownloadHandler(s.ShareSvc, s.LibQuerySvc, s.BaseURL()))))
	}
	mux.HandleFunc("POST /api/v1/media/{id}/stream-token", auth(streamTokenHandler(s.TokenGen, s.StreamTokenStore, s.LibraryAccessRepo, s.LibQuerySvc, parentalChk)))

	transcodeThrottle := apptranscoding.NewTranscodeThrottle()
	mux.HandleFunc("GET /api/v1/media/{id}/transcode", streamAuth(transcodeHandler(transcodeServe{
		tokens: s.TokenGen, libSvc: s.LibQuerySvc, baseURL: s.BaseURL(),
		libraryAccess: s.LibraryAccessRepo, throttle: transcodeThrottle, userMaxStreams: s.UserMaxStreams,
		parental: parentalChk,
	})))
	mux.HandleFunc("GET /api/v1/media/{id}/subtitle/{track}", streamAuth(subtitleHandler(s.TokenGen, s.LibQuerySvc, s.BaseURL(), s.LibraryAccessRepo, parentalChk)))
	mux.HandleFunc("GET /api/v1/media/{id}/chapters", streamAuth(chaptersHandler(s.TokenGen, s.LibQuerySvc, s.BaseURL(), s.LibraryAccessRepo, s.LibQuerySvc, parentalChk)))
	mux.HandleFunc("GET /api/v1/media/{id}/segments", streamAuth(segmentsHandler(s.TokenGen, s.LibQuerySvc, s.BaseURL(), s.LibraryAccessRepo, s.LibQuerySvc, func() bool { return s.Cfg.EffectiveSkipPreviewEnabled() }, parentalChk)))
	// Trickplay (manifest + sprite images) is public (non-sensitive): no auth required.
	mux.HandleFunc("GET /api/v1/media/{id}/trickplay", trickplayManifestHandler(s.TokenGen, s.LibQuerySvc, s.BaseURL(), s.LibraryAccessRepo, s.LibQuerySvc, parentalChk))
	mux.HandleFunc("GET /api/v1/media/{id}/trickplay/{image}", trickplayImageHandler(s.TokenGen, s.LibQuerySvc, s.BaseURL(), s.LibraryAccessRepo, s.LibQuerySvc, parentalChk))
	snapshotH := snapshotHandler(s.TokenGen, s.FrameExtractor, s.LibraryAccessRepo, s.LibQuerySvc, s.BaseURL(), parentalChk)
	if s.SnapshotRL != nil {
		snapshotH = ratelimit.MiddlewareFunc(s.SnapshotRL, snapshotH)
	}
	mux.HandleFunc("GET /api/v1/media/{id}/snapshot", streamAuth(snapshotH))
	mux.HandleFunc("POST /api/v1/admin/media/{id}/trickplay/generate", admin(ratelimit.MiddlewareFunc(s.AdminHeavyRL, trickplayGenerateHandler(s.TokenGen, s.LibCmdSvc, s.ServerCtx))))
	mux.HandleFunc("POST /api/v1/admin/trickplay/generate-all", admin(ratelimit.MiddlewareFunc(s.AdminHeavyRL, trickplayGenerateAllHandler(s.TokenGen, s.LibCmdSvc))))
	mux.HandleFunc("POST /api/v1/admin/search/reindex", admin(ratelimit.MiddlewareFunc(s.AdminHeavyRL, searchReindexHandler(s.LibCmdSvc, s.ServerCtx))))

	// Encode calibration: read the current capability + force a rebench
	// (typically when the admin changes HW accel settings or migrates
	// the container). Both gated admin + AdminHeavyRL — the recalibrate
	// can hammer CPU for ~30 s, can't be spammed.
	if s.CalibrationSvc != nil {
		mux.HandleFunc("GET /api/v1/admin/transcode/calibration", admin(transcodeCalibrationGetHandler(s.CalibrationSvc)))
		mux.HandleFunc("POST /api/v1/admin/transcode/calibration/recalibrate", admin(ratelimit.MiddlewareFunc(s.AdminHeavyRL, transcodeRecalibrateHandler(s.CalibrationSvc))))
	}
	if s.CapOverrideSvc != nil {
		mux.HandleFunc("GET /api/v1/admin/transcode/calibration/overrides", admin(transcodeOverridesGetHandler(s.CapOverrideSvc)))
		// Reset is per-source-class via query string so the URL stays
		// flat ("DELETE /overrides/2160p" would also work — chose
		// the query form to mirror trickplay/regenerate's pattern).
		mux.HandleFunc("DELETE /api/v1/admin/transcode/calibration/overrides", admin(transcodeOverrideResetHandler(s.CapOverrideSvc)))
	}
	if s.IntroDetector != nil {
		mux.HandleFunc("POST /api/v1/admin/media/{id}/detect-segments", introDetectionHandler(s.TokenGen, s.IntroDetector))
		mux.HandleFunc("POST /api/v1/admin/libraries/{id}/detect-intros", libraryIntroBackfillHandler(s.TokenGen, s.IntroDetector))
		mux.HandleFunc("PUT /api/v1/admin/media/{id}/segments", setSegmentHandler(s.TokenGen, s.IntroDetector, s.LibraryAccessRepo, s.LibQuerySvc))
		mux.HandleFunc("DELETE /api/v1/admin/media/{id}/segments", deleteSegmentHandler(s.TokenGen, s.IntroDetector, s.LibraryAccessRepo, s.LibQuerySvc))
		mux.HandleFunc("POST /api/v1/admin/media/{id}/segments/propagate", propagateSegmentHandler(s.TokenGen, s.IntroDetector, s.LibraryAccessRepo, s.LibQuerySvc))
	}
	mux.HandleFunc("GET /api/v1/media/{id}/playback-info", streamAuth(playbackInfoHandler(s.TokenGen, s.LibQuerySvc, s.LibraryAccessRepo, s.ProfilePrefs, parentalChk)))
	hlsAccessCache := NewCachedLibraryAccessChecker(s.LibraryAccessRepo)

	// VOD-style HLS pipeline: manifest precomputed in Go from durationMs,
	// segments produced on demand by SegmentOrchestrator into SegmentCache.
	// Replaced the previous ffmpeg-writes-the-playlist path that drifted out
	// after ~9 min of continuous playback (cf features/hls_vod_playlist_refacto).
	if s.HLSTranscoder != nil && s.HLSVODOrch != nil {
		if vodHandler := newHLSVODServe(s, s.LibQuerySvc, hlsAccessCache, parentalChk); vodHandler != nil {
			// Audio rendition handler — same auth/parental/cache/manifest
			// dependencies as the video pipeline, plus the AudioOrch nil-safe
			// wrap. Routed via vodHandler.serveHTTP (it dispatches the
			// audio_T.m3u8 / audio_T_init.mp4 / audio_T_NNN.m4s shapes from
			// the single /hls/{file} catch-all so the relative URLs the
			// master playlist emits resolve correctly).
			aOrch := AudioSegmentEnsurer(nil)
			if s.AudioOrch != nil {
				aOrch = s.AudioOrch
			}
			vodHandler.audio = &audioServe{
				cfg:                  s.Cfg,
				libSvc:               s.LibQuerySvc,
				libraryAccess:        hlsAccessCache,
				parental:             parentalChk,
				manifestGen:          apptranscoding.NewManifestGenerator(),
				cache:                s.HLSVODCache,
				audioOrch:            aOrch,
				audioNormalizeGetter: s.AudioNormalizeGetter,
				segmentWaitTimeout:   30 * time.Second,
			}

			// Single catch-all — the handler dispatches on the {file} pattern
			// (master.m3u8 / stream_N.m3u8 / stream_N_NNN.ext / stream_N_init.mp4
			// AND audio_T.m3u8 / audio_T_init.mp4 / audio_T_NNN.m4s). The
			// per-profile streaming limiter sits INSIDE streamAuth so it keys on
			// the resolved profile/user; it returns 503+Retry-After (hls.js-safe)
			// on a flood. This is the highest-frequency route (a segment every
			// few seconds per stream) — the DoS vector the GlobalRL was wrong for
			// (its per-IP 429 would 1) false-positive a multi-profile household
			// and 2) stall hls.js).
			mux.HandleFunc("GET /api/v1/media/{id}/hls/{file}", streamAuth(streamingRateLimit(s.StreamingRL, s.StreamMeter.Middleware(vodHandler.serveHTTP))))
			// Predictive warm-up: the player POSTs at ~85% progress of an
			// episode so the next ep's transcode is pre-spawned and the
			// autoplay handoff feels instant. Auth-required (logged-in user
			// only) but no body — the URL identifies the *current* media.
			// Rate-limited via AdminHeavyRL: each accepted request can spawn
			// an ffmpeg rafale, so we must cap per-IP abuse independently of
			// the frontend's once-per-mount throttle (a JS console can bypass
			// it trivially).
			mux.HandleFunc("POST /api/v1/media/{id}/prewarm-next", auth(ratelimit.MiddlewareFunc(s.AdminHeavyRL, vodHandler.servePrewarmNext)))
		}
	}
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
			// Same limiter as the polling QC route: the SSE stream is an auth
			// path (it mints tokens) and each open connection holds a goroutine
			// + hub subscription for up to ~2min — unlimited opens were a cheap
			// resource-pinning + passive brute-force vector.
			mux.HandleFunc("GET /api/v1/auth/quick-connect/{code}/events", ratelimit.MiddlewareFunc(s.LoginRL, quickConnectSSEHandler(s.QuickConnectHub, &qcCheckerAdapter{query: s.UserQuerySvc, auth: s.UserAuthSvc, cfg: s.Cfg})))
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
	registerSyncPlayRoutes(mux, s.SyncPlayHub, s.TokenGen, s.LibQuerySvc, s.LibraryAccessRepo)                                            // binary: WebSocket
	// PUBLIC: a "watch together" guest exchanges a room code for a streaming-only
	// guest token. Rate-limited (LoginRL) against 5-digit code brute-force.
	mux.HandleFunc("POST /api/v1/syncplay/join-code", ratelimit.MiddlewareFunc(s.LoginRL, syncplayJoinByCodeHandler(s.SyncPlayHub, s.StreamTokenStore)))
	registerDeviceRoutes(mux, s.DeviceRegistry, s.TokenGen)                                                                               // Play On: WebSocket + REST
	registerBackupRoutes(mux, s.BackupSvc, s.TokenGen, s.AdminHeavyRL, s.AdminWriteRL)                                                    // binary: download/restore
	registerRemoteAccessRoutes(mux, s.Cfg, s.TokenGen, s.CfgStore, s.AdminWriteRL)
	registerServerSettingsRoutes(mux, s.Cfg, s.CfgStore, s.TokenGen, s.TranscodingSvc, s.HLSVODOrch, hwModeControllerOf(s.CalibrationSvc), s.LoginRL, s.GlobalRL, s.AdminWriteRL)
	registerBackgroundTasksRoute(mux, s.TokenGen)
	registerPlaybackSettingsRoutes(mux, s.Cfg, s.TokenGen, s.AdminWriteRL)
	registerParentalRoutes(mux, s.ProfileRepo, s.UserRepo, s.Hasher, s.TokenGen)
	if s.DataDir != "" {
		registerAvatarRoutes(mux, s.ProfileRepo, s.TokenGen, s.DataDir)
	}
	if s.MetadataCmdSvc != nil && s.MetadataQuerySvc != nil {
		registerAdminRoutes(mux, s.MetadataCmdSvc, s.MetadataQuerySvc, s.TokenGen, s.AdminHeavyRL, s.AdminWriteRL, s.ServerCtx, s.ActivityCmdSvc)
	}
	if s.MetadataCmdSvc != nil {
		var cfgSvc *appconfig.Service
		if s.ServerCfgRepo != nil {
			cfgSvc = appconfig.NewService(s.ServerCfgRepo)
		}
		registerArtworkAdminRoutes(mux, s.MetadataCmdSvc, s.TokenGen, s.ServerCtx, s.DataDir, cfgSvc)
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
	registerMusicRoutes(mux, s.MusicSvc, s.TokenGen, s.LibraryAccessRepo, s.LibQuerySvc, s.GlobalRL, s.LibQuerySvc)
	if s.CollectionSvc != nil {
		registerCollectionRoutes(mux, s.CollectionSvc, s.LibQuerySvc, s.TokenGen, s.LibraryAccessRepo, s.BaseURL(), s.GlobalRL)
	}
	if s.PlaylistSvc != nil && s.PlaylistGenSvc != nil {
		registerPlaylistRoutes(mux, s.PlaylistSvc, s.PlaylistGenSvc, s.LibQuerySvc, s.MusicSvc, s.TokenGen, s.LibraryAccessRepo, s.BaseURL(), s.GlobalRL)
	}

	if s.SubtitleSvc != nil {
		registerSubtitleRoutes(mux, s.SubtitleSvc, s.TokenGen, s.LibQuerySvc, s.LibraryAccessRepo)
	}

	if s.PluginRepo != nil && s.PluginDir != "" {
		registerPluginAssetRoutes(mux, s.TokenGen, s.PluginTokenGen, s.PluginRepo, s.PluginDir, s.CSSSanitizer)
		registerThemeAssetRoutes(mux, s.TokenGen, s.PluginRepo, s.PluginDir)
		if s.PluginSvc != nil {
			registerPluginUploadRoute(mux, s.TokenGen, s.PluginSvc, s.AdminWriteRL)
			registerPluginPreviewRoute(mux, s.TokenGen, s.PluginSvc, s.AdminWriteRL)
		}
	}
	if s.PluginRepo != nil {
		registerPluginPublicRoutes(mux, s.TokenGen, s.PluginRepo)
	}
	if s.PluginSecurityDashboardSvc != nil {
		registerPluginSecurityDashboardRoute(mux, s.TokenGen, s.PluginSecurityDashboardSvc)
	}
	if s.QoEQuerySvc != nil {
		registerQoEDashboardRoute(mux, s.TokenGen, s.QoEQuerySvc)
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
