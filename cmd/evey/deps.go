package main

import (
	"github.com/evey-project/evey/internal/adapters/driven/ffmpeg"
	mkvgoadapter "github.com/evey-project/evey/internal/adapters/driven/mkvgo"
	"github.com/evey-project/evey/internal/adapters/driven/pluginrunner"
	grpcapi "github.com/evey-project/evey/internal/adapters/driving/grpcapi"
	httpapi "github.com/evey-project/evey/internal/adapters/driving/http"
	"github.com/evey-project/evey/internal/adapters/driving/pluginhost"
	appactivity "github.com/evey-project/evey/internal/app/activity"
	appbackup "github.com/evey-project/evey/internal/app/backup"
	appcollection "github.com/evey-project/evey/internal/app/collection"
	appconfig "github.com/evey-project/evey/internal/app/config"
	appfavorite "github.com/evey-project/evey/internal/app/favorite"
	appfeed "github.com/evey-project/evey/internal/app/feed"
	applibrary "github.com/evey-project/evey/internal/app/library"
	apploadtest "github.com/evey-project/evey/internal/app/loadtest"
	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	appmedia "github.com/evey-project/evey/internal/app/media"
	appmediarequest "github.com/evey-project/evey/internal/app/mediarequest"
	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	appmkv "github.com/evey-project/evey/internal/app/mkv"
	appmusic "github.com/evey-project/evey/internal/app/music"
	appnotification "github.com/evey-project/evey/internal/app/notification"
	appphoto "github.com/evey-project/evey/internal/app/photo"
	appplayback "github.com/evey-project/evey/internal/app/playback"
	appplaylist "github.com/evey-project/evey/internal/app/playlist"
	appplugin "github.com/evey-project/evey/internal/app/plugin"
	apppresence "github.com/evey-project/evey/internal/app/presence"
	appqoe "github.com/evey-project/evey/internal/app/qoe"
	apprating "github.com/evey-project/evey/internal/app/rating"
	appsubtitle "github.com/evey-project/evey/internal/app/subtitle"
	appsyncplay "github.com/evey-project/evey/internal/app/syncplay"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	appuser "github.com/evey-project/evey/internal/app/user"
	appwebhook "github.com/evey-project/evey/internal/app/webhook"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domainnotif "github.com/evey-project/evey/internal/domain/notification"
	domainplugin "github.com/evey-project/evey/internal/domain/plugin"
	domainshared "github.com/evey-project/evey/internal/domain/shared"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/eventbus"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/netmeter"
)

// libraryDeps groups library scanning, querying and scheduling services.
type libraryDeps struct {
	cmdSvc            *applibrary.CommandService
	querySvc          *applibrary.QueryService
	scheduler         *applibrary.Scheduler
	introDetectionSvc *applibrary.IntroDetectionService
}

// userDeps groups user/auth services.
type userDeps struct {
	authSvc   *appuser.CommandService
	userSvc   *appuser.UserService
	accessSvc *appuser.AccessService
	querySvc  *appuser.QueryService
	oidcSvc   *appuser.OIDCService
}

// mediaDeps groups playback, transcoding, and media-format services.
type mediaDeps struct {
	playbackSvc    *appplayback.Service
	transcodingSvc *apptranscoding.Service
	// hlsVODOrch owns the ffmpeg rafale sessions that produce VOD-style HLS
	// segments. Also satisfies ActiveStreamChecker (trickplay scheduler) and
	// HLSSessionRemover (admin kill-session, playback stop) - the legacy
	// HLSManager that used to sit here is fully retired.
	hlsVODOrch    *apptranscoding.SegmentOrchestrator
	hlsVODCache   *apptranscoding.SegmentCache
	hlsTranscoder *ffmpeg.HLSTranscoder
	// hlsVODSoftware is the single shared software StreamProducer (built in
	// wire.go). The HTTP VOD handler's routing selector and the gRPC StopStream
	// / admin-kill remover both reference it so a stopped software stream is
	// torn down promptly instead of lingering until the producer's idle GC.
	hlsVODSoftware *apptranscoding.SoftwareProducer
	// hlsVODRemover fans RemoveSession out to BOTH the software producer and the
	// hardware orchestrator (StopStream / admin force-kill). Routing unused.
	hlsVODRemover *apptranscoding.ProducerSelector
	// audioOrch manages audio-only HLS rendition sessions. Always
	// wired (uses the same hlsVODCache); the HTTP routes that consume
	// it gate on Cfg.Transcoding.AudioRenditionEnabled so it stays
	// inert until the feature flag flips.
	audioOrch      *apptranscoding.AudioSegmentOrchestrator
	qoeSink        appqoe.EventSink
	qoeQuerySvc    *appqoe.QueryService
	complexityRepo domaintranscoding.ComplexityRepo
	calibrationSvc *apptranscoding.CalibrationService
	videoLimiter   *apptranscoding.VideoSessionLimiter
	// videoShaper pins per-stream ladder ceilings so an over-budget viewer is
	// served a lower rung instead of refused (see VideoAdmissionShaper).
	videoShaper      *apptranscoding.VideoAdmissionShaper
	capsOverrideSvc  *apptranscoding.CapOverrideService
	mkvSvc           *appmkv.Service
	mkvAdapter       *mkvgoadapter.Adapter // also drives the /remux.mp4 fast-start MP4 repackage
	musicSvc         *appmusic.Service
	musicNowPlaying  *appmusic.NowPlayingRegistry
	photoSvc         *appphoto.Service
	photoTranscoder  *ffmpeg.PhotoTranscoder
	subtitleSvc      *appsubtitle.Service
	syncPlayHub      *appsyncplay.Hub
	deviceRegistry   *appsyncplay.DeviceRegistry
	streamTokenStore *appmedia.StreamTokenStore
	mediaItemCache   *applibrary.CachedMediaItemQueryRepo
	shareSvc         *appmedia.ShareService
	// castProgress derives watch progress from cast receivers' fetches; the
	// HTTP serving handlers feed it (see internal/app/playback/cast_progress.go).
	castProgress *appplayback.CastProgressTracker
}

// contentDeps groups content-organisation services (metadata, collections, ratings).
type contentDeps struct {
	metadataCmdSvc   *appmetadata.CommandService
	metadataQuerySvc *appmetadata.QueryService
	enricher         *appmetadata.Enricher
	collectionSvc    *appcollection.Service
	favoriteSvc      *appfavorite.Service
	mediaRequestSvc  *appmediarequest.Service
	starRatingSvc    *apprating.Service
	playlistSvc      *appplaylist.Service
	playlistGenSvc   *appplaylist.GeneratorService
}

// engagementDeps groups activity, notifications, feeds, webhooks and plugins.
type engagementDeps struct {
	activityCmdSvc             *appactivity.CommandService
	activityQuerySvc           *appactivity.QueryService
	metricsSampler             *appactivity.MetricsSampler
	streamMeter                *netmeter.Meter
	notifCmdSvc                *appnotification.CommandService
	notifQuerySvc              *appnotification.QueryService
	feedCmdSvc                 *appfeed.CommandService
	feedQuerySvc               *appfeed.QueryService
	webhookSvc                 *appwebhook.Service
	pluginInstallerSvc         *appplugin.InstallerService
	pluginRuntimeSvc           *appplugin.RuntimeService
	pluginQuerySvc             *appplugin.QueryService
	pluginTokenSvc             *appplugin.TokenService
	pluginHostServer           *pluginhost.HostServer
	pluginSecurityDashboardSvc *appplugin.SecurityDashboardService
	pluginIdleMgr              *appplugin.IdleManager
}

// authDeps groups authentication infrastructure.
type authDeps struct {
	// tokenGen is the raw JWT generator - kept concrete for the plugin-token
	// methods that sit outside user.TokenGenerator.
	tokenGen *auth.JWTGenerator
	// userTokenGen is the epoch-guarded decorator: every user access-token
	// mint/validation in HTTP and gRPC goes through it so token_epoch
	// revocation (disable, role change, password change, session kill) is
	// enforced within the cache TTL instead of the token expiry.
	userTokenGen domainuser.TokenGenerator
	hasher       auth.BcryptHasher
}

// repoDeps groups repository interfaces used directly by HTTP/gRPC layers.
type repoDeps struct {
	artworkRepo       httpapi.ArtworkRepo
	userRatingRepo    domainlibrary.UserRatingRepository
	pushSubRepo       domainnotif.PushSubscriptionRepository
	libraryAccessRepo domainuser.LibraryAccessRepository
	profileRepo       domainuser.ProfileRepository
	serverCfgRepo     domainshared.ServerConfigRepository
	mediaItemRepo     mediaItemCounter
	userRepo          domainuser.UserRepository
	apiKeyRepo        domainuser.APIKeyRepository
	pluginRepo        domainplugin.PluginRepository
	pluginDataRepo    domainplugin.PluginDataRepository
	quickConnectRepo  domainuser.QuickConnectRepository
	introMetaCache    grpcapi.IntroCacheInvalidator
}

// appDeps holds all application-level services and adapters needed by HTTP/gRPC layers.
type appDeps struct {
	library                libraryDeps
	user                   userDeps
	media                  mediaDeps
	content                contentDeps
	engagement             engagementDeps
	auth                   authDeps
	repos                  repoDeps
	bus                    *eventbus.InMemoryEventBus
	cfgStore               *httpapi.ServerConfigStore
	serverCfgSvc           *appconfig.Service
	pluginRunner           *pluginrunner.Runner
	pluginResDefaults      appplugin.ResourceDefaults
	backupSvc              *appbackup.Service
	backupScheduler        *appbackup.Scheduler
	translator             *eveyi18n.Translator
	searchHealth           httpapi.SearchHealthChecker
	imageConverter         httpapi.ImageConverter
	frameExtractor         httpapi.FrameExtractor
	copySeekProber         apptranscoding.SeekLandingProber
	onDemandPackager       apptranscoding.OnDemandPackager
	playbackActivity       *apptranscoding.PlaybackActivity
	onDemandConcatPackager apptranscoding.ConcatPackager
	// presenceTracker is the ONE live "who is online" registry, shared by the
	// gRPC heartbeat (web/mobile/TV apps) and the HTTP cast-beacon route (a
	// beaconing Chromecast counts as an online device of the casting user).
	presenceTracker    *apppresence.Tracker
	hlsAESSecret       []byte
	seekIndexFileState appmaintenance.FileStateRefresher
	// keyframeOnDemand fills a missing keyframe index inline at playback
	// negotiation (bounded fast-path probe shared with the backfill worker).
	keyframeOnDemand *apptranscoding.KeyframeOnDemand
	// loadTestRunner drives the built-in capacity measurement (admin RPC);
	// loadTestBaseURL is the loopback origin its synthetic viewers hit.
	loadTestRunner  *apploadtest.Runner
	loadTestBaseURL string
	// loadTestPublicURL is the reverse-proxy origin real viewers come through.
	// Used when reachable; the loopback is the fallback.
	loadTestPublicURL string
	// bgStoppers holds the blocking Stop/Close of every background worker that
	// owns an in-flight DB write (transcode complexity / level-backfill /
	// keyframe-backfill and the CMCD cap-override sweep). main.go invokes them at
	// shutdown BEFORE the deferred db.Close so a worker finishes its current item
	// instead of racing the closing pool. Only STARTED workers register here, so
	// the slice reflects the conditional wiring; order is not significant (the
	// workers are independent).
	bgStoppers []func()
}
