package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	appactivity "github.com/eveyproject/evey/internal/app/activity"
	appbackup "github.com/eveyproject/evey/internal/app/backup"
	appcast "github.com/eveyproject/evey/internal/app/chromecast"
	appcollection "github.com/eveyproject/evey/internal/app/collection"
	appconfig "github.com/eveyproject/evey/internal/app/config"
	appdlna "github.com/eveyproject/evey/internal/app/dlna"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	apploadtest "github.com/eveyproject/evey/internal/app/loadtest"
	appmaintenance "github.com/eveyproject/evey/internal/app/maintenance"
	appmedia "github.com/eveyproject/evey/internal/app/media"
	appmetadata "github.com/eveyproject/evey/internal/app/metadata"
	appmusic "github.com/eveyproject/evey/internal/app/music"
	appphoto "github.com/eveyproject/evey/internal/app/photo"
	appplaylist "github.com/eveyproject/evey/internal/app/playlist"
	appqoe "github.com/eveyproject/evey/internal/app/qoe"
	appsubtitle "github.com/eveyproject/evey/internal/app/subtitle"
	appsyncplay "github.com/eveyproject/evey/internal/app/syncplay"
	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	appuser "github.com/eveyproject/evey/internal/app/user"
	appwebhook "github.com/eveyproject/evey/internal/app/webhook"
	"github.com/eveyproject/evey/internal/config"
	domainplugin "github.com/eveyproject/evey/internal/domain/plugin"
	domainshared "github.com/eveyproject/evey/internal/domain/shared"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/i18n"
	eveymetrics "github.com/eveyproject/evey/internal/pkg/metrics"
	"github.com/eveyproject/evey/internal/pkg/netmeter"
	"github.com/eveyproject/evey/internal/pkg/quickconnecthub"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
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
	BaseURL func() string
	Version string
	// Commit / BuildDate identify the exact code this process runs, stamped at
	// link time. Both are the literal "unknown" when the build could not be
	// stamped - never blank, so a client can tell "not implemented" apart from
	// "this build cannot say". /healthz reports them so a deployment can be
	// verified over plain HTTP, with no shell on the host.
	Commit    string
	BuildDate string
	// TVRelease and MobileRelease are the app builds released with this
	// server, advertised on /healthz; zero when the build was not stamped.
	TVRelease     ClientRelease
	MobileRelease ClientRelease
	StartTime     time.Time
	LogDir        string
	DataDir       string
	HTTPSOnly     bool
	// HTTPSMode, when set, is read on every request instead of HTTPSOnly, so
	// an https External URL saved in the panel applies without a restart.
	HTTPSMode func() bool

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
	// CastProgress derives watch progress from a cast receiver's fetches (the
	// only playback signal once the sender's browser closes). nil = off.
	CastProgress CastFetchObserver

	// Library
	LibCmdSvc   *applibrary.CommandService
	LibQuerySvc *applibrary.QueryService
	Scheduler   *applibrary.Scheduler
	// Remuxer repackages EBML (mkv/webm) sources into fast-start MP4 for the
	// progressive /remux.mp4 path. Optional: nil disables the route.
	Remuxer Remuxer
	// RemuxLeases holds the /remux.mp4 files a player is reading, shared with
	// the cache budget so none is evicted under it. Optional.
	RemuxLeases *apptranscoding.FileLeases
	// CompatTranscoder produces the phone-compatible download variant (audio
	// re-encoded to stereo AAC, everything else copied) served by the
	// compat-download routes. Optional: nil disables them.
	CompatTranscoder CompatTranscoder
	// ReducedTranscoder produces the size-reduced (720p) download variant as
	// slices the phone joins locally. Optional: nil disables the routes, which
	// is the DEFAULT - the feature costs an hour of NAS CPU per film, so the
	// operator opts in. See downloads.quality_choice.
	ReducedTranscoder ReducedTranscoder
	// ReducedOfferAboveBytes is the source size above which the app offers the
	// lighter copy (downloads.reduce_above_bytes). The phone ORs it with its
	// own "it does not fit on this device" trigger.
	ReducedOfferAboveBytes int64

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
	HLSVODOrch *apptranscoding.SegmentOrchestrator
	// HLSVODSoftware is the shared software StreamProducer (built in wire.go) so
	// the gRPC StopStream / admin-kill path can tear down the very sessions this
	// handler's routing selector creates. nil in hand-built test servers →
	// newHLSVODServe builds a private one.
	HLSVODSoftware *apptranscoding.SoftwareProducer
	HLSVODCache    *apptranscoding.SegmentCache
	// ProbeSignals collects the P3 ceiling-hit signal from the serve path; nil = off.
	ProbeSignals  *apptranscoding.ProbeSignalStore
	HLSTranscoder domaintranscoding.HLSProcess
	// vod is the HLS pipeline's handler once built (Routes); its resolver is
	// the one the info surfaces decide with (PlaybackDecider).
	vod *hlsVODServe
	// AudioOrch manages audio-only HLS rendition sessions (EXT-X-MEDIA).
	// Nil-safe: the audio rendition routes return 404 when unset. Wiring
	// from wire.go lands after the feature flag stabilises.
	AudioOrch *apptranscoding.AudioSegmentOrchestrator
	// ComplexityRepo drives per-title ABR ladder selection. Nil-safe - the
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

	// OnDemandPackager serves the copy rung straight from mkvgo's mp4.PlanHLS
	// (zero-storage, stateless HLS + DASH). Default on, opt-out via
	// transcoding.hls_ondemand=off; nil-safe (feature off → the ffmpeg
	// copy/transcode pipeline is unchanged).
	OnDemandPackager apptranscoding.OnDemandPackager

	// PlaybackActivity is the "someone is watching" tracker the on-demand path
	// feeds. The packager spawns no ffmpeg session, so it is the ONLY way a
	// playback served by mkvgo becomes visible to ActiveSessionCount - and
	// through it to every background worker that yields under live playback.
	// nil-safe (unwired → the counter stays ffmpeg-only, as before).
	PlaybackActivity *apptranscoding.PlaybackActivity

	// OnDemandConcatPackager serves a gapless multi-episode HLS session
	// (mkvgo's mp4.PlanConcat) when a request opts in with ?gapless=1. Off by
	// default, opt-in via transcoding.hls_ondemand_concat=on; nil-safe
	// (feature off → concat is never attempted, per-episode playback
	// unchanged).
	OnDemandConcatPackager apptranscoding.ConcatPackager

	// HLSAESSecret is the server secret the on-demand rung derives per-media
	// AES-128 HLS keys from (HMAC(secret, mediaID)). Only used when
	// transcoding.hls_ondemand_encrypt is on; empty disables encryption.
	HLSAESSecret []byte

	// transcodeCloser tears down the VOD software producers built during route
	// registration (the selector's SoftwareProducer + the optional copy
	// producer). They own long-lived ffmpeg sessions that would otherwise
	// outlive the process as orphans - nothing else closes them. The hardware
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
	MetadataCmdSvc *appmetadata.CommandService
	// SeekIndexSvc serves the sync-preview clip an operator judges a desync on.
	// Optional: without it the route is simply not registered.
	SeekIndexSvc     *appmaintenance.SeekIndexService
	MetadataQuerySvc *appmetadata.QueryService

	// Activity
	ActivityCmdSvc *appactivity.CommandService
	// UpdateChecker answers "is a newer image published?"; nil mounts nothing.
	UpdateChecker UpdateChecker

	// Collections / Playlists / Favorites
	CollectionSvc  *appcollection.QueryService
	PlaylistSvc    *appplaylist.Service
	PlaylistGenSvc *appplaylist.GeneratorService

	// Music
	MusicSvc        *appmusic.Service
	MusicNowPlaying *appmusic.NowPlayingRegistry // "En cours de lecture" heartbeat registry

	// Photo
	PhotoSvc        *appphoto.Service
	PhotoTranscoder PhotoTranscoder
	PhotoCacheDir   string

	// SyncPlay
	SyncPlayHub *appsyncplay.Hub

	// Play On - registry of first-party clients controllable via the server
	DeviceRegistry *appsyncplay.DeviceRegistry

	// Subtitle (optional)
	SubtitleSvc *appsubtitle.Service

	// Backup
	BackupSvc *appbackup.Service

	// WebhookSvc backs the admin "Test" ping (POST /api/v1/admin/webhooks/{id}/test).
	// Every other webhook CRUD route goes through grpc-gateway from the proto
	// definition - this is the one manual HTTP route in the webhook surface,
	// since there is no RPC for it. Nil-safe: unset skips registration, so a
	// server built without this wired keeps working, just without the route.
	WebhookSvc *appwebhook.Service

	// SMTPTester backs the Email / SMTP card's "Send test e-mail"
	// (POST /api/v1/admin/smtp/test). Nil skips the route.
	SMTPTester SMTPTestSender

	// Image conversion (optional)
	ImageConverter ImageConverter

	// Repos / accessors
	ArtworkRepo       ArtworkRepo
	LibraryAccessRepo domainuser.LibraryAccessRepository
	ServerCfgRepo     domainshared.ServerConfigRepository
	ProfileRepo       domainuser.ProfileRepository
	UserRepo          domainuser.UserRepository
	Hasher            domainuser.PasswordHasher
	// ServerCfgSvc is the application's config service (event bus and unit of
	// work wired): multi-key admin saves go through it atomically.
	ServerCfgSvc *appconfig.Service
	// ProfileHours closes kids profiles outside their opening hours (nil:
	// no hours enforced, tests).
	ProfileHours *appuser.ProfileSchedules

	// Plugin
	PluginRepo                 domainplugin.PluginRepository
	PluginSvc                  pluginManager
	PluginDir                  string
	CSSSanitizer               cssSanitizer
	PluginSecurityDashboardSvc pluginSecurityDashboardProvider
	PluginWakener              PluginWakener

	// KeyframeWarmer nudges the bounded on-demand keyframe walk. Optional: it
	// lets the HLS shed path tell a copy-eligible-but-unindexed source apart
	// from a genuinely full host (see copyBlockedOnlyByMissingIndex).
	KeyframeWarmer func(ctx context.Context, mediaID, path string) bool

	// VideoLimiter lets the VOD handler shed a variant playlist early, before a
	// player commits to a stream it cannot be given. Optional.
	VideoLimiter *apptranscoding.VideoSessionLimiter

	// VideoAdmissionShaper serves a lower ladder rung instead of refusing when
	// the top one does not fit the budget. Optional (nil = shed instead).
	VideoAdmissionShaper *apptranscoding.VideoAdmissionShaper
	// RouteExpressibility is the ffmpeg builder's own answer to "can you emit
	// this route", consulted by the plan resolver's backend pick (ADR-0030).
	// Optional (nil = nothing refused).
	RouteExpressibility domaintranscoding.RouteExpressibility

	// DLNA
	DLNASvc *appdlna.Service

	// Chromecast
	CastSvc *appcast.Service
	// CastPresence + CastSessionTouch surface a beaconing receiver in the
	// admin panel: an online "EveyCast (Chromecast)" device entry and a live
	// now-playing session, both independent of the (possibly frozen) phone.
	CastPresence         CastPresenceToucher
	CastResolveUsername  func(ctx context.Context, userID string) string
	CastSessionKeepalive CastSessionTouch
	// CastSessionTerminate ends the session on the receiver's own death beacon,
	// rather than waiting for a silence the alive beat prevents.
	CastSessionTerminate CastSessionEnd
	CastCommandConsume   CastCommandConsumer
	CastAudioModeTouch   CastAudioModeTouch
	// SessionServingRecord tells the play session what a master playlist just
	// handed its player (packager/encoder per track), for the admin panel.
	SessionServingRecord SessionServingRecord
	// SessionAudioLegRecord follows an in-place audio track switch, which
	// fetches a rendition playlist and never re-renders the master.
	SessionAudioLegRecord SessionAudioLegRecord

	// Config store for remote access persistence
	CfgStore remoteAccessPersister

	// Server context (for long-running background ops like photo scan)
	ServerCtx context.Context

	// Rate limiters passed in from main
	LoginRL  *ratelimit.Limiter
	GlobalRL *ratelimit.Limiter
	// StreamingRL is the per-profile request-rate limiter for the high-frequency
	// streaming routes (HLS segments/playlists). nil = disabled (pass-through).
	StreamingRL *ratelimit.Limiter
	// LoadTestActive reports whether the built-in load test is running. While it
	// is, StreamingRL stands down: every simulated viewer carries the same
	// credential, so they would all throttle each other inside one profile's
	// bucket and the run would measure the limiter instead of the server. nil =
	// never active.
	LoadTestActive  func() bool
	AdminHeavyRL    *ratelimit.Limiter
	AdminWriteRL    *ratelimit.Limiter
	PasswordResetRL *ratelimit.Limiter
	OIDCLoginRL     *ratelimit.Limiter
	// QuickConnectPollRL caps the Quick Connect status poll (the television
	// asking every few seconds whether its code was approved) - a read, not a
	// login attempt, so it never rides LoginRL's per-minute login budget.
	// nil = pass-through.
	QuickConnectPollRL *ratelimit.Limiter
	// SnapshotRL caps the frame-extraction route - each request spawns an
	// ffmpeg, so it must not ride the generous global limiter.
	SnapshotRL *ratelimit.Limiter
	// SyncPreviewRL caps the sync-preview clip route. It too spawns an ffmpeg,
	// but unlike the other heavy admin routes it is used in a LOOP - watch,
	// nudge the offset, watch again - so it gets a bucket sized for that
	// instead of sharing AdminHeavyRL's ten-per-minute with trickplay
	// generation and the search reindex.
	SyncPreviewRL *ratelimit.Limiter
	// SubtitleRL caps the subtitle-extraction route. mkvgo's fast path handles
	// most MKV sources without a fork, but any container it can't shortcut
	// (.avi/.ts/.mov, damaged EBML) falls through to exec.CommandContext'ing
	// ffmpeg per request - the same fork cost as a snapshot, so it shares the
	// same ceiling instead of riding the generous global limiter.
	SubtitleRL *ratelimit.Limiter
	// TrickplayRL caps the trickplay manifest + sprite-image routes, shared
	// across both since they're always fetched together (a scrub bar loads
	// the manifest once then many sprite images). No ffmpeg fork here (disk +
	// DB reads only), so it is far more permissive than SubtitleRL/SnapshotRL.
	TrickplayRL *ratelimit.Limiter

	// SubtitleProviderRL caps the subtitle search/download routes, which call
	// out to the subtitle provider on the operator's API key and spend its
	// finite per-day download quota.
	SubtitleProviderRL *ratelimit.Limiter
	// PlayerEventRL caps the playback-error beacon: a decode-error storm on a
	// broken stream can fire dozens of client events per minute, and each
	// accepted one becomes a log line. The client throttles itself too; this
	// is the belt to that suspender. nil = route not registered.
	PlayerEventRL *ratelimit.Limiter

	// i18n
	Translator *i18n.Translator

	// audio normalize getter (closure capturing serverCfgRepo)
	AudioNormalizeGetter func(ctx context.Context) bool
	// AudioEncoderIDGetter names the concrete ffmpeg AAC implementation this
	// host encodes with ("aac"/"libfdk_aac"). nil → native, the historical
	// bytes. It rides the cache fingerprint, so it must be the SAME value the
	// arg builder receives.
	AudioEncoderIDGetter func() string

	// VideoFrameRateProbe and EncoderReorderProbe give a TRANSCODED rung's
	// composition delay (encoder reorder frames / source fps) for the same
	// shift. nil = transcoded ladders keep their audio muxed.
	VideoFrameRateProbe func(ctx context.Context, path string) float64
	EncoderReorderProbe func(ctx context.Context, codec, accel, device string, features []string) (int, bool)

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

// subtitleRouteHandler builds the subtitle-extraction route handler gated by
// rl (nil = pass-through, e.g. tests). mkvgo's fast path handles most MKV
// sources without a fork, but any container it can't shortcut (.avi/.ts/.mov,
// damaged EBML) falls through to exec.CommandContext'ing ffmpeg per request -
// the same fork cost as the snapshot route, so it shares its nil-safe gate
// pattern instead of riding the generous global limiter.
func subtitleRouteHandler(libSvc MediaStreamsGetter, baseURL string, libraryAccess LibraryAccessChecker, parental ParentalChecker, rl *ratelimit.Limiter, cache subtitleCache) http.HandlerFunc {
	h := subtitleHandler(libSvc, baseURL, libraryAccess, parental, cache)
	if rl != nil {
		h = ratelimit.MiddlewareFunc(rl, h)
	}
	return h
}

// trickplayRouteHandlers builds the trickplay manifest + sprite-image route
// handlers sharing one rl (nil = pass-through). Both only read disk/DB (no
// ffmpeg fork), so the ceiling is far more permissive than subtitle/snapshot -
// and shared because a scrub bar loads the manifest once then many sprites.
func trickplayRouteHandlers(libSvc TrickplayGetter, baseURL string, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter, parental ParentalChecker, tokenStore *appmedia.StreamTokenStore, rl *ratelimit.Limiter) (manifest, image http.HandlerFunc) {
	manifest = trickplayManifestHandler(libSvc, baseURL, libraryAccess, mediaGetter, parental, tokenStore)
	image = trickplayImageHandler(libSvc, baseURL, libraryAccess, mediaGetter, parental)
	if rl != nil {
		manifest = ratelimit.MiddlewareFunc(rl, manifest)
		image = ratelimit.MiddlewareFunc(rl, image)
	}
	return manifest, image
}

// RegisterRoutes wires all HTTP routes onto mux.
// gwMux is the grpc-gateway handler (may be nil if gateway failed).
// CloseTranscodeProducers kills the long-lived ffmpeg sessions owned by the
// VOD software/copy producers. Call once at shutdown (idempotent); no-op when
// the VOD pipeline was never wired (tests).
// syncPreviewLimiter returns the preview route's own bucket, falling back to
// the shared heavy-admin one when a caller has not wired it (tests, embedders).
// Never nil: an unlimited preview route would let one operator's nudge loop
// spawn ffmpeg without a ceiling.
func (s *HTTPServer) syncPreviewLimiter() *ratelimit.Limiter {
	if s.SyncPreviewRL != nil {
		return s.SyncPreviewRL
	}
	return s.AdminHeavyRL
}

func (s *HTTPServer) CloseTranscodeProducers() {
	s.transcodeCloserOnce.Do(func() {
		if s.transcodeCloser != nil {
			s.transcodeCloser()
		}
	})
}

func (s *HTTPServer) RegisterRoutes(mux *http.ServeMux, gwMux http.Handler) {
	auth := httpAuthMiddleware(s.TokenGen)
	// streamAuth additionally accepts opaque stream tokens (?token=) - apply ONLY
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

	mux.HandleFunc("GET /api/v1/admin/fs/browse", admin(browseDirsHandler()))
	// The load test asks, through the public address, who handed it the
	// request: the proxy's address, which it then dials directly when it sits
	// on the local network (see apploadtest.PeerHandler).
	mux.HandleFunc("GET /api/v1/admin/load-test/peer", admin(apploadtest.PeerHandler))

	// parentalChk gates the raw byte/decision/token stream handlers so a kids
	// profile that knows an adult media-id out of band can't bypass the gRPC
	// parental filter by hitting these endpoints directly. Gating the HLS master
	// (which creates the session) transitively protects the segment handler.
	parentalChk := NewParentalChecker(s.UserQuerySvc)
	// Kids opening hours, checked inside the two auth middlewares every
	// route below goes through.
	if s.ProfileHours != nil {
		SetProfileAccessGate(s.ProfileHours)
	}
	// Short-TTL caches for the per-request hot paths (HLS segments, direct-play
	// Range requests, trickplay tiles): the access verdict and the library
	// roots are stable across a playback burst, parental stays live.
	hlsAccessCache := NewCachedLibraryAccessChecker(s.LibraryAccessRepo)
	libRootsCache := NewCachedLibraryLister(s.LibQuerySvc)

	// Unconditional, unlike the on-demand rung's tracker: direct play has no
	// feature flag to pair with - it is always a possible serving path.
	mhOpts := []mediaServeOption{withParental(parentalChk), withLibraryLister(libRootsCache), withPlaybackActivity(s.PlaybackActivity), withCastProgress(s.CastProgress)}
	if s.ImageConverter != nil {
		mhOpts = append(mhOpts, withImageConverter(s.ImageConverter, s.ArtworkRepo))
	}
	mh := mediaHandler(s.LibQuerySvc, s.ArtworkRepo, s.BaseURL(), hlsAccessCache, mhOpts...)
	mux.HandleFunc("GET /api/v1/media/{id}/stream", streamAuth(s.StreamMeter.Middleware(mh)))
	// Progressive fast-start MP4 remux of a browser-incompatible EBML source
	// (copy, no re-encode), cached once and served with Range support. Same auth
	// + parental + stream metering as /stream.
	if s.Remuxer != nil {
		mux.HandleFunc("GET /api/v1/media/{id}/remux.mp4", streamAuth(s.StreamMeter.Middleware(remuxHandler(s.LibQuerySvc, s.LibraryAccessRepo, parentalChk, s.LibQuerySvc, s.Remuxer, s.Cfg.Transcoding.CacheDir, s.BaseURL(), s.RemuxLeases))))
	}
	// Artwork is public (non-sensitive): no auth required.
	mux.HandleFunc("GET /api/v1/media/{id}/artwork/{artType}", mh)
	mux.HandleFunc("POST /api/v1/media/{id}/stream", streamAuth(mh))
	// Playback-error beacon: the player posts what IT saw (decode errors,
	// recovery loops, element-level MediaError) so a client-side failure leaves
	// a server-side trace. Registered regardless of the transcoding pipeline -
	// direct play fails too.
	if s.PlayerEventRL != nil {
		// streamAuth (not auth): the cast receiver reports playback deaths with
		// its ?token= stream token - it has no user JWT. The handler pins a
		// stream token to its own media id.
		mux.HandleFunc("POST /api/v1/media/{id}/player-event", streamAuth(ratelimit.MiddlewareFunc(s.PlayerEventRL, playerEventHandler(slog.Default(),
			WithPlayerEventLibraryGuard(s.LibQuerySvc, s.LibraryAccessRepo, parentalChk),
			// And as the admin panel's cast signal: online device + live
			// now-playing session, phone-independent.
			WithPlayerEventCastPresence(s.CastPresence, s.CastResolveUsername),
			WithPlayerEventCastSession(s.CastSessionKeepalive),
			WithPlayerEventCastSessionEnd(s.CastSessionTerminate),
			WithPlayerEventCastAudioMode(s.CastAudioModeTouch),
			WithPlayerEventCastCommand(s.CastCommandConsume),
			// And as the native players' QoE feed: ExoPlayer/AVPlayer send no
			// CMCD of their own, the app posts qoe_sample beacons instead.
			WithPlayerEventQoESink(s.QoESink)))))
	}
	mux.HandleFunc("POST /api/v1/media/{id}/artwork/{artworkId}/select", auth(selectArtworkHandler(s.ArtworkRepo, s.LibQuerySvc, s.LibraryAccessRepo, s.UserQuerySvc)))
	mux.HandleFunc("GET /api/v1/media/{id}/download", streamAuth(ratelimit.MiddlewareFunc(s.GlobalRL, s.StreamMeter.Middleware(downloadHandler(s.LibQuerySvc, s.BaseURL(), s.LibraryAccessRepo, parentalChk)))))
	// Compat download: the server-prepared variant whose audio a phone without
	// AC-3/DTS decoders can render. POST kicks the (minutes-long, cached,
	// once-per-title) production; GET /status is the poll; the bare GET serves
	// the finished MKV under the same guards and metering as the raw download.
	if s.CompatTranscoder != nil {
		compat := newCompatServe(s.LibQuerySvc, s.LibQuerySvc, s.LibraryAccessRepo, parentalChk, s.LibQuerySvc, s.CompatTranscoder, s.Cfg.Transcoding.CacheDir, s.BaseURL())
		mux.HandleFunc("POST /api/v1/media/{id}/compat-download", streamAuth(ratelimit.MiddlewareFunc(s.GlobalRL, compat.handlePrepare)))
		mux.HandleFunc("GET /api/v1/media/{id}/compat-download/status", streamAuth(compat.handleStatus))
		mux.HandleFunc("GET /api/v1/media/{id}/compat-download", streamAuth(ratelimit.MiddlewareFunc(s.GlobalRL, s.StreamMeter.Middleware(compat.handleServe))))
	}
	// Reduced download: the 720p variant for sources too big to fit on a
	// phone, delivered as Matroska slices the device joins itself. /plan is
	// the "should we even offer this" probe (no work started); the slices go
	// out under the same guards and metering as the raw download.
	if s.ReducedTranscoder != nil {
		reduced := newReducedServe(s.LibQuerySvc, s.LibQuerySvc, s.LibraryAccessRepo, parentalChk, s.LibQuerySvc, s.ReducedTranscoder, s.Cfg.Transcoding.CacheDir, s.BaseURL(), s.ReducedOfferAboveBytes)
		reduced.startIdleSweep(s.ServerCtx)
		mux.HandleFunc("GET /api/v1/media/{id}/reduced-download/plan", streamAuth(reduced.handlePlan))
		mux.HandleFunc("POST /api/v1/media/{id}/reduced-download", streamAuth(ratelimit.MiddlewareFunc(s.GlobalRL, reduced.handlePrepare)))
		mux.HandleFunc("GET /api/v1/media/{id}/reduced-download/status", streamAuth(reduced.handleStatus))
		mux.HandleFunc("GET /api/v1/media/{id}/reduced-download/slice/{n}", streamAuth(ratelimit.MiddlewareFunc(s.GlobalRL, s.StreamMeter.Middleware(reduced.handleSlice))))
		mux.HandleFunc("DELETE /api/v1/media/{id}/reduced-download", streamAuth(ratelimit.MiddlewareFunc(s.GlobalRL, reduced.handleDiscard)))
	}
	// Public share download: no auth - the uuid token is the capability.
	// Admin-only to CREATE (RBAC), but the resulting link must resolve without
	// a login. Rate-limited per-IP as defence-in-depth (the uuid entropy is the
	// real guard against token guessing).
	if s.ShareSvc != nil {
		mux.HandleFunc("GET /api/v1/share/{token}/download", ratelimit.MiddlewareFunc(s.GlobalRL, s.StreamMeter.Middleware(shareDownloadHandler(s.ShareSvc, s.LibQuerySvc, s.BaseURL()))))
	}
	mux.HandleFunc("POST /api/v1/media/{id}/stream-token", auth(streamTokenHandler(s.TokenGen, s.StreamTokenStore, s.LibraryAccessRepo, s.LibQuerySvc, parentalChk)))
	// Opaque short-lived token for the NON-media download URLs (collection /
	// playlist / music ZIPs) so the account JWT stops riding in query strings
	// (proxy logs, browser history). See download_token.go for the model.
	mux.HandleFunc("POST /api/v1/download-token/{id}", auth(downloadTokenHandler(s.StreamTokenStore)))

	// The index store is process-wide: the extraction path reaches it from both
	// the route and the prewarmer.
	subtitleIndexes = newSubtitleIndexCache(s.DataDir)
	mux.HandleFunc("GET /api/v1/media/{id}/subtitle/{track}", streamAuth(subtitleRouteHandler(s.LibQuerySvc, s.BaseURL(), s.LibraryAccessRepo, parentalChk, s.SubtitleRL, newSubtitleCache(s.DataDir))))
	mux.HandleFunc("GET /api/v1/media/{id}/chapters", streamAuth(chaptersHandler(s.LibQuerySvc, s.BaseURL(), s.LibraryAccessRepo, s.LibQuerySvc, parentalChk)))
	mux.HandleFunc("GET /api/v1/media/{id}/segments", streamAuth(segmentsHandler(s.LibQuerySvc, s.BaseURL(), s.LibraryAccessRepo, s.LibQuerySvc, func() bool { return s.Cfg.EffectiveSkipPreviewEnabled() }, parentalChk, func() bool { return s.Cfg.EffectiveIntroAutoSkipEnabled() })))
	// Trickplay (manifest + sprite images) is NOT public: the sprites are frames
	// of the film. It was mounted bare on the "non-sensitive" assumption, which
	// let anyone fetch a storyboard of any media id. streamAuth like its
	// siblings: a JWT, or the ?token= stream token a watch-together guest holds
	// (the handlers reject a token whose media id doesn't match the path).
	trickplayManifestH, trickplayImageH := trickplayRouteHandlers(s.LibQuerySvc, s.BaseURL(), hlsAccessCache, s.LibQuerySvc, parentalChk, s.StreamTokenStore, s.TrickplayRL)
	mux.HandleFunc("GET /api/v1/media/{id}/trickplay", streamAuth(trickplayManifestH))
	mux.HandleFunc("GET /api/v1/media/{id}/trickplay/{image}", streamAuth(trickplayImageH))
	snapshotH := snapshotHandler(s.FrameExtractor, s.LibraryAccessRepo, s.LibQuerySvc, s.BaseURL(), parentalChk)
	if s.SnapshotRL != nil {
		snapshotH = ratelimit.MiddlewareFunc(s.SnapshotRL, snapshotH)
	}
	mux.HandleFunc("GET /api/v1/media/{id}/snapshot", streamAuth(snapshotH))
	mux.HandleFunc("POST /api/v1/admin/media/{id}/trickplay/generate", admin(ratelimit.MiddlewareFunc(s.AdminHeavyRL, trickplayGenerateHandler(s.LibCmdSvc, s.ServerCtx))))
	mux.HandleFunc("POST /api/v1/admin/trickplay/generate-all", admin(ratelimit.MiddlewareFunc(s.AdminHeavyRL, trickplayGenerateAllHandler(s.LibCmdSvc))))
	mux.HandleFunc("POST /api/v1/admin/search/reindex", admin(ratelimit.MiddlewareFunc(s.AdminHeavyRL, searchReindexHandler(s.LibCmdSvc, s.ServerCtx))))

	// Encode calibration: read the current capability + force a rebench
	// (typically when the admin changes HW accel settings or migrates
	// the container). Both gated admin + AdminHeavyRL - the recalibrate
	// can hammer CPU for ~30 s, can't be spammed.
	if s.CalibrationSvc != nil {
		mux.HandleFunc("GET /api/v1/admin/transcode/calibration", admin(transcodeCalibrationGetHandler(s.CalibrationSvc)))
		mux.HandleFunc("POST /api/v1/admin/transcode/calibration/recalibrate", admin(ratelimit.MiddlewareFunc(s.AdminHeavyRL, transcodeRecalibrateHandler(s.CalibrationSvc))))
	}
	if s.CapOverrideSvc != nil {
		mux.HandleFunc("GET /api/v1/admin/transcode/calibration/overrides", admin(transcodeOverridesGetHandler(s.CapOverrideSvc)))
		// Reset is per-source-class via query string so the URL stays
		// flat ("DELETE /overrides/2160p" would also work - chose
		// the query form to mirror trickplay/regenerate's pattern).
		mux.HandleFunc("DELETE /api/v1/admin/transcode/calibration/overrides", admin(transcodeOverrideResetHandler(s.CapOverrideSvc)))
	}
	if s.IntroDetector != nil {
		mux.HandleFunc("POST /api/v1/admin/media/{id}/detect-segments", introDetectionHandler(s.TokenGen, s.IntroDetector, s.LibQuerySvc, s.LibraryAccessRepo))
		mux.HandleFunc("POST /api/v1/admin/libraries/{id}/detect-intros", libraryIntroBackfillHandler(s.TokenGen, s.IntroDetector, s.LibraryAccessRepo))
		mux.HandleFunc("PUT /api/v1/admin/media/{id}/segments", setSegmentHandler(s.TokenGen, s.IntroDetector, s.LibraryAccessRepo, s.LibQuerySvc))
		mux.HandleFunc("DELETE /api/v1/admin/media/{id}/segments", deleteSegmentHandler(s.TokenGen, s.IntroDetector, s.LibraryAccessRepo, s.LibQuerySvc))
		mux.HandleFunc("POST /api/v1/admin/media/{id}/segments/propagate", propagateSegmentHandler(s.TokenGen, s.IntroDetector, s.LibraryAccessRepo, s.LibQuerySvc))
	}
	mux.HandleFunc("GET /api/v1/media/{id}/playback-info", streamAuth(playbackInfoHandler(s.PlaybackDecider(), s.LibQuerySvc, s.LibraryAccessRepo, s.ProfilePrefs, parentalChk)))

	// VOD-style HLS pipeline: manifest precomputed in Go from durationMs,
	// segments produced on demand by SegmentOrchestrator into SegmentCache.
	// Replaced the previous ffmpeg-writes-the-playlist path that drifted out
	// after ~9 min of continuous playback (cf features/hls_vod_playlist_refacto).
	// The picture route for the clips drawn outside a session (sync preview):
	// the player's resolver when the HLS pipeline is wired, software otherwise.
	pictureOf := softwarePicturePlanner(s.LibQuerySvc)
	if s.HLSTranscoder != nil && s.HLSVODOrch != nil {
		if vodHandler := newHLSVODServe(s, s.LibQuerySvc, hlsAccessCache, parentalChk); vodHandler != nil {
			s.vod = vodHandler
			pictureOf = vodHandler.picturePlan
			// Audio rendition handler - same auth/parental/cache/manifest
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
				startHint:            func(key string) int { seg, _ := vodHandler.prewarmSegment(key, 0, time.Now()); return seg },
				audioNormalizeGetter: s.AudioNormalizeGetter,
				audioEncoderIDGetter: s.AudioEncoderIDGetter,
				segmentWaitTimeout:   30 * time.Second,
			}

			// Single catch-all - the handler dispatches on the {file} pattern
			// (master.m3u8 / stream_N.m3u8 / stream_N_NNN.ext / stream_N_init.mp4
			// AND audio_T.m3u8 / audio_T_init.mp4 / audio_T_NNN.m4s). The
			// per-profile streaming limiter sits INSIDE streamAuth so it keys on
			// the resolved profile/user; it returns 503+Retry-After (hls.js-safe)
			// on a flood. This is the highest-frequency route (a segment every
			// few seconds per stream) - the DoS vector the GlobalRL was wrong for
			// (its per-IP 429 would 1) false-positive a multi-profile household
			// and 2) stall hls.js).
			mux.HandleFunc("GET /api/v1/media/{id}/hls/{file}", streamAuth(streamingRateLimit(s.StreamingRL, s.LoadTestActive, s.StreamMeter.Middleware(vodHandler.serveHTTP))))
			// Predictive warm-up: the player POSTs at ~85% progress of an
			// episode so the next ep's transcode is pre-spawned and the
			// autoplay handoff feels instant. Auth-required (logged-in user
			// only) but no body - the URL identifies the *current* media.
			// Rate-limited via AdminHeavyRL: each accepted request can spawn
			// an ffmpeg rafale, so we must cap per-IP abuse independently of
			// the frontend's once-per-mount throttle (a JS console can bypass
			// it trivially).
			mux.HandleFunc("POST /api/v1/media/{id}/prewarm-next", auth(ratelimit.MiddlewareFunc(s.AdminHeavyRL, vodHandler.servePrewarmNext)))
			// Cast receiver's next-episode door: streamAuth because the TV's
			// only identity is its cast stream token (no JWT once the phone
			// sleeps); the handler refuses everything but a cast token pinned
			// to {id}. Same AdminHeavyRL as prewarm-next - it too can spawn a
			// warm-up rafale.
			mux.HandleFunc("POST /api/v1/media/{id}/cast-next", streamAuth(ratelimit.MiddlewareFunc(s.AdminHeavyRL, vodHandler.serveCastNext)))
		}
	}
	mux.HandleFunc("GET /api/v1/admin/library-stats", admin(libraryStatsHandler(s.LibQuerySvc, NewLibraryItemCounter(s.LibQuerySvc, s.MusicSvc, s.PhotoSvc))))

	// DLNA
	if s.DLNASvc != nil {
		mux.HandleFunc("GET /api/v1/dlna/renderers", auth(dlnaRenderersHandler(s.DLNASvc)))
		mux.HandleFunc("POST /api/v1/dlna/play", auth(dlnaPlayHandler(s.DLNASvc)))
		mux.HandleFunc("POST /api/v1/dlna/stop", auth(dlnaStopHandler(s.DLNASvc)))
		mux.HandleFunc("POST /api/v1/dlna/pause", auth(dlnaPauseHandler(s.DLNASvc)))
	}

	// Chromecast
	if s.CastSvc != nil {
		mux.HandleFunc("GET /api/v1/cast/devices", auth(castDevicesHandler(s.CastSvc)))
		mux.HandleFunc("POST /api/v1/cast/play", auth(castPlayHandler(s.CastSvc)))
		mux.HandleFunc("POST /api/v1/cast/stop", auth(castStopHandler(s.CastSvc)))
		mux.HandleFunc("POST /api/v1/cast/pause", auth(castPauseHandler(s.CastSvc)))
		mux.HandleFunc("POST /api/v1/cast/seek", auth(castSeekHandler(s.CastSvc)))
		mux.HandleFunc("GET /api/v1/cast/status", auth(castStatusHandler(s.CastSvc)))
	}

	if gwMux != nil {
		mux.HandleFunc("POST /api/v1/auth/login", ratelimit.MiddlewareFunc(s.LoginRL, func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			authCookieMiddleware(gwMux, s.https()).ServeHTTP(w, r)
		}))
		mux.HandleFunc("POST /api/v1/auth/forgot-password", ratelimit.MiddlewareFunc(s.PasswordResetRL, func(w http.ResponseWriter, r *http.Request) {
			gwMux.ServeHTTP(w, r)
		}))
		mux.HandleFunc("POST /api/v1/auth/reset-password", ratelimit.MiddlewareFunc(s.PasswordResetRL, func(w http.ResponseWriter, r *http.Request) {
			gwMux.ServeHTTP(w, r)
		}))
		// The status poll carries the long code the TV was handed (not the
		// short one a user types), and answers "pending" until an authenticated
		// user approves: a read at the poll cadence, on its own limiter. It
		// shared LoginRL until 2026-09-19, whose 10/min prod default refused
		// every other poll of a single television.
		mux.HandleFunc("GET /api/v1/auth/quick-connect/{code}", ratelimit.MiddlewareFunc(s.QuickConnectPollRL, func(w http.ResponseWriter, r *http.Request) {
			authCookieMiddleware(gwMux, s.https()).ServeHTTP(w, withQuickConnectSecretQuery(r))
		}))
		if s.QuickConnectHub != nil {
			// Same limiter as the polling QC route: each open SSE connection
			// holds a goroutine + hub subscription for up to ~2min - unlimited
			// opens were a cheap resource-pinning + passive brute-force vector.
			mux.HandleFunc("GET /api/v1/auth/quick-connect/{code}/events", ratelimit.MiddlewareFunc(s.QuickConnectPollRL, quickConnectSSEHandler(s.QuickConnectHub, &qcCheckerAdapter{query: s.UserQuerySvc, auth: s.UserAuthSvc, cfg: s.Cfg})))
		}
		mux.HandleFunc("POST /api/v1/auth/refresh", ratelimit.MiddlewareFunc(s.LoginRL, func(w http.ResponseWriter, r *http.Request) {
			r = injectRefreshTokenFromCookie(r)
			authCookieMiddleware(gwMux, s.https()).ServeHTTP(w, r)
		}))
		mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
			// The cookie names THIS device's session, so only it is ended.
			r = injectRefreshTokenFromCookie(r)
			clearRefreshCookie(w, s.https())
			// RP-Initiated Logout, only for a session that came through SSO (the
			// id_token cookie): the client navigates to endSessionUrl itself,
			// since a fetch cannot follow a redirect to the provider's origin.
			if c, cErr := r.Cookie(oidcIDTokenCookieName); cErr == nil && c.Value != "" && s.OIDCSvc != nil {
				if ep := s.OIDCSvc.EndSessionEndpoint(r.Context()); ep != "" {
					rec := &discardResponseWriter{header: make(http.Header)}
					gwMux.ServeHTTP(rec, r)
					clearOIDCIDTokenCookie(w, s.https())
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]string{
						"endSessionUrl": oidcEndSessionURL(ep, s.OIDCSvc.PostLogoutRedirectURL(r.Context()), s.BaseURL()+"/login", c.Value),
					})
					return
				}
			}
			clearOIDCIDTokenCookie(w, s.https())
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
	registerPhotoRoutes(mux, s.PhotoSvc, s.TokenGen, s.LibraryAccessRepo, s.LibQuerySvc, s.ServerCtx, s.PhotoTranscoder, s.PhotoCacheDir, s.SnapshotRL) // binary: photo/serve
	registerSyncPlayRoutes(mux, s.SyncPlayHub, s.TokenGen, s.LibQuerySvc, s.LibraryAccessRepo, s.StreamTokenStore, withSyncPlayParental(parentalChk))   // binary: WebSocket
	// PUBLIC: a "watch together" guest exchanges a room code for a streaming-only
	// guest token - the product's only unauthenticated mint. Two brakes: LoginRL
	// per source address, and a global failure gate the handler owns, which a
	// caller rotating source addresses cannot walk past.
	mux.HandleFunc("POST /api/v1/syncplay/join-code", ratelimit.MiddlewareFunc(s.LoginRL,
		syncplayJoinByCodeHandler(s.SyncPlayHub, s.StreamTokenStore,
			ratelimit.NewFailureGate("syncplay-join-code", joinCodeFailureThreshold, joinCodeFailureWindow, joinCodeCooldown))))
	registerDeviceRoutes(mux, s.DeviceRegistry, s.TokenGen)                                                // Play On: WebSocket + REST
	registerBackupRoutes(mux, s.BackupSvc, s.TokenGen, s.StreamTokenStore, s.AdminHeavyRL, s.AdminWriteRL) // binary: download/restore
	if s.WebhookSvc != nil {
		// grpc-gateway serves Create/List/Update/Delete/ListDeliveries from the
		// proto; "test" has no RPC, hence the manual route registered here.
		registerWebhookRoutes(mux, s.WebhookSvc, s.TokenGen, s.AdminHeavyRL)
	}
	if s.SMTPTester != nil {
		registerSMTPTestRoute(mux, s.SMTPTester, s.UserRepo, s.TokenGen, s.AdminHeavyRL)
	}
	registerRemoteAccessRoutes(mux, s.Cfg, s.TokenGen, s.CfgStore, s.AdminWriteRL)
	registerServerSettingsRoutes(mux, s.Cfg, s.CfgStore, s.TokenGen, s.TranscodingSvc, hwModeControllerOf(s.CalibrationSvc), s.LoginRL, s.GlobalRL, s.AdminWriteRL)
	registerBackgroundTasksRoute(mux, s.TokenGen, s.ActivityCmdSvc)
	registerSystemMetricsRoute(mux, s.TokenGen)
	registerUpdateStatusRoute(mux, s.TokenGen, s.UpdateChecker)
	func() {
		var cfgSvc *appconfig.Service
		if s.ServerCfgRepo != nil {
			cfgSvc = appconfig.NewService(s.ServerCfgRepo)
		}
		registerPlaybackSettingsRoutes(mux, s.Cfg, s.TokenGen, s.AdminWriteRL, cfgSvc)
	}()
	parental := registerParentalRoutes(mux, s.ProfileRepo, s.UserRepo, s.Hasher, s.TokenGen)
	registerScheduleRoutes(mux, parental, s.ProfileHours)
	// The web + mobile admin panels both call GET/PATCH
	// .../admin/users/{id}/bitrate-limit, but neither the proto nor this mux
	// ever exposed it - the "max bitrate per user" modal has 404'd since it
	// shipped. UserQuerySvc/UserRepo are the same core deps registerParentalRoutes
	// already uses just above, so this needs no new wiring.
	registerUserBitrateRoutes(mux, s.UserQuerySvc, appuser.NewBitrateLimitSetter(s.UserRepo), s.TokenGen, s.AdminWriteRL)
	if s.DataDir != "" {
		registerAvatarRoutes(mux, s.ProfileRepo, s.TokenGen, s.DataDir)
	}
	if s.MetadataCmdSvc != nil && s.MetadataQuerySvc != nil {
		registerAdminRoutes(mux, s.MetadataCmdSvc, s.MetadataQuerySvc, s.TokenGen, s.AdminHeavyRL, s.AdminWriteRL, s.ServerCtx, s.ActivityCmdSvc)
	}
	registerSyncPreviewRoute(mux, s.SeekIndexSvc, s.TokenGen, s.syncPreviewLimiter(), pictureOf)
	if s.MetadataCmdSvc != nil {
		var cfgSvc *appconfig.Service
		if s.ServerCfgRepo != nil {
			cfgSvc = appconfig.NewService(s.ServerCfgRepo)
		}
		registerArtworkAdminRoutes(mux, s.MetadataCmdSvc, s.TokenGen, s.ServerCtx, s.DataDir, cfgSvc)
		// Admin-supplied poster/backdrop upload. Same guard as the rest of fiche
		// editing, and needs the artwork repo only to drop its memoised list.
		if s.ArtworkRepo != nil {
			registerArtworkUploadRoute(mux, s.MetadataCmdSvc, s.TokenGen, s.LibQuerySvc, s.ArtworkRepo)
		}
	}
	if s.MetadataCmdSvc != nil {
		mux.HandleFunc("POST /api/v1/admin/translations/refresh-all", admin(func(w http.ResponseWriter, r *http.Request) {
			go func() {
				defer func() {
					if r := recover(); r != nil {
						slog.ErrorContext(s.ServerCtx, "panic in RefreshAllTranslations goroutine", slog.Any("recover", r))
					}
				}()
				if err := s.MetadataCmdSvc.RefreshAllTranslations(s.ServerCtx, nil); err != nil {
					slog.ErrorContext(s.ServerCtx, "refresh all translations", slog.Any("err", err))
				} else {
					slog.Info("refresh all translations complete")
				}
			}()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"started"}`))
		}))
	}
	// Ratings, popular, analytics, filter → gRPC (grpcapi/rating_handler, activity_handler, library_handler)
	registerLogsRoute(mux, s.LogDir, s.TokenGen, s.LibQuerySvc)
	registerAvailableRuntimesRoute(mux, s.TokenGen)
	musicOpts := []musicRouteOption{}
	if s.PhotoCacheDir != "" {
		// Track (tag-embedded) art variants live next to the photo cache, under
		// the same transcode cache root: derived data, regenerated on demand.
		musicOpts = append(musicOpts, withTrackArtCacheDir(filepath.Join(s.PhotoCacheDir, "track-art")))
	}
	registerMusicRoutes(mux, s.MusicSvc, s.TokenGen, s.LibraryAccessRepo, s.LibQuerySvc, s.GlobalRL, s.LibQuerySvc, s.ImageConverter, s.MusicNowPlaying, musicOpts...)
	if s.CollectionSvc != nil {
		registerCollectionRoutes(mux, s.CollectionSvc, s.LibQuerySvc, s.TokenGen, s.LibraryAccessRepo, parentalChk, s.BaseURL(), s.GlobalRL)
	}
	if s.PlaylistSvc != nil && s.PlaylistGenSvc != nil {
		registerPlaylistRoutes(mux, s.PlaylistSvc, s.PlaylistGenSvc, s.LibQuerySvc, s.MusicSvc, s.TokenGen, s.LibraryAccessRepo, parentalChk, s.BaseURL(), s.GlobalRL)
	}

	if s.SubtitleSvc != nil {
		registerSubtitleRoutes(mux, s.SubtitleSvc, s.TokenGen, s.LibQuerySvc, s.LibraryAccessRepo, s.SubtitleProviderRL, parentalChk)
	}

	// The page plugin UIs and theme scripts run in (its own CSP, see the handler).
	mux.HandleFunc("GET "+pluginFramePath, pluginFrameHandler)

	if s.PluginRepo != nil && s.PluginDir != "" {
		registerPluginAssetRoutes(mux, s.TokenGen, s.PluginTokenGen, s.PluginRepo, s.PluginDir, s.CSSSanitizer, s.PluginWakener)
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

	configLanguage := "en"
	if s.Cfg != nil && s.Cfg.Metadata.Language != "" {
		configLanguage = s.Cfg.Metadata.Language
	}
	registerConfigDefaultsRoute(mux, s.TokenGen, s.ServerCfgRepo, configLanguage)

	if s.OIDCSvc != nil {
		registerOIDCRoutesFn(mux, s.OIDCSvc, s.https, s.OIDCLoginRL)
		var ssoStore ssoSettingsStore
		switch {
		case s.ServerCfgSvc != nil:
			ssoStore = s.ServerCfgSvc // transactional SetMany
		case s.ServerCfgRepo != nil:
			ssoStore = appconfig.NewService(s.ServerCfgRepo)
		}
		var ssoLibs ssoLibraryLookup
		if s.LibQuerySvc != nil {
			ssoLibs = s.LibQuerySvc
		}
		registerSSOAdminRoutes(mux, s.OIDCSvc, ssoStore, ssoLibs, s.TokenGen, s.AdminWriteRL)
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

// PlaybackDecider is the decider the info surfaces (HTTP playback-info, gRPC
// GetStreamInfo) answer with: the HLS pipeline's own resolver, so the answer
// is the plan's decision with its pins, not the bare core. Read lazily: the
// pipeline is built by Routes, after the gRPC server that asks for this.
func (s *HTTPServer) PlaybackDecider() apptranscoding.PlaybackDecider {
	return playbackDeciderFunc(func(ctx context.Context, req apptranscoding.PlaybackRequest) domaintranscoding.PlaybackDecision {
		if s.vod == nil {
			return apptranscoding.UnwiredDecider().Reported(ctx, req)
		}
		return s.vod.planResolver().Reported(ctx, req)
	})
}

type playbackDeciderFunc func(ctx context.Context, req apptranscoding.PlaybackRequest) domaintranscoding.PlaybackDecision

func (f playbackDeciderFunc) Reported(ctx context.Context, req apptranscoding.PlaybackRequest) domaintranscoding.PlaybackDecision {
	return f(ctx, req)
}

// https reports the HTTPS mode now: the live source when wired, else the
// boot-time flag.
func (s *HTTPServer) https() bool {
	if s.HTTPSMode != nil {
		return s.HTTPSMode()
	}
	return s.HTTPSOnly
}
