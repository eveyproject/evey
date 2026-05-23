package main

import (
	appactivity "github.com/evey-project/evey/internal/app/activity"
	appbackup "github.com/evey-project/evey/internal/app/backup"
	appcollection "github.com/evey-project/evey/internal/app/collection"
	appfavorite "github.com/evey-project/evey/internal/app/favorite"
	appfeed "github.com/evey-project/evey/internal/app/feed"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmedia "github.com/evey-project/evey/internal/app/media"
	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	appmkv "github.com/evey-project/evey/internal/app/mkv"
	appmusic "github.com/evey-project/evey/internal/app/music"
	appnotification "github.com/evey-project/evey/internal/app/notification"
	appphoto "github.com/evey-project/evey/internal/app/photo"
	"github.com/evey-project/evey/internal/adapters/driven/ffmpeg"
	appplayback "github.com/evey-project/evey/internal/app/playback"
	appplaylist "github.com/evey-project/evey/internal/app/playlist"
	appplugin "github.com/evey-project/evey/internal/app/plugin"
	appconfig "github.com/evey-project/evey/internal/app/config"
	apprating "github.com/evey-project/evey/internal/app/rating"
	appsubtitle "github.com/evey-project/evey/internal/app/subtitle"
	appsyncplay "github.com/evey-project/evey/internal/app/syncplay"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	appuser "github.com/evey-project/evey/internal/app/user"
	appwebhook "github.com/evey-project/evey/internal/app/webhook"
	"github.com/evey-project/evey/internal/adapters/driven/pluginrunner"
	"github.com/evey-project/evey/internal/adapters/driving/pluginhost"
	grpcapi "github.com/evey-project/evey/internal/adapters/driving/grpcapi"
	httpapi "github.com/evey-project/evey/internal/adapters/driving/http"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domainnotif "github.com/evey-project/evey/internal/domain/notification"
	domainplugin "github.com/evey-project/evey/internal/domain/plugin"
	domainshared "github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/eventbus"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
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
	hlsManager     *apptranscoding.HLSManager
	mkvSvc         *appmkv.Service
	musicSvc       *appmusic.Service
	photoSvc        *appphoto.Service
	photoTranscoder *ffmpeg.PhotoTranscoder
	subtitleSvc    *appsubtitle.Service
	syncPlayHub      *appsyncplay.Hub
	deviceRegistry   *appsyncplay.DeviceRegistry
	streamTokenStore *appmedia.StreamTokenStore
	shareSvc         *appmedia.ShareService
}

// contentDeps groups content-organisation services (metadata, collections, ratings).
type contentDeps struct {
	metadataCmdSvc   *appmetadata.CommandService
	metadataQuerySvc *appmetadata.QueryService
	enricher         *appmetadata.Enricher
	collectionSvc    *appcollection.Service
	favoriteSvc      *appfavorite.Service
	starRatingSvc    *apprating.Service
	playlistSvc      *appplaylist.Service
}

// engagementDeps groups activity, notifications, feeds, webhooks and plugins.
type engagementDeps struct {
	activityCmdSvc     *appactivity.CommandService
	activityQuerySvc   *appactivity.QueryService
	notifCmdSvc        *appnotification.CommandService
	notifQuerySvc      *appnotification.QueryService
	feedCmdSvc         *appfeed.CommandService
	feedQuerySvc       *appfeed.QueryService
	webhookSvc         *appwebhook.Service
	pluginInstallerSvc         *appplugin.InstallerService
	pluginRuntimeSvc           *appplugin.RuntimeService
	pluginQuerySvc             *appplugin.QueryService
	pluginTokenSvc             *appplugin.TokenService
	pluginHostServer           *pluginhost.HostServer
	pluginSecurityDashboardSvc *appplugin.SecurityDashboardService
}

// authDeps groups authentication infrastructure.
type authDeps struct {
	tokenGen *auth.JWTGenerator
	hasher   auth.BcryptHasher
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
	library    libraryDeps
	user       userDeps
	media      mediaDeps
	content    contentDeps
	engagement engagementDeps
	auth       authDeps
	repos      repoDeps
	bus               *eventbus.InMemoryEventBus
	cfgStore          *httpapi.ServerConfigStore
	serverCfgSvc      *appconfig.Service
	pluginRunner      *pluginrunner.Runner
	pluginResDefaults appplugin.ResourceDefaults
	backupSvc       *appbackup.Service
	backupScheduler *appbackup.Scheduler
	translator     *eveyi18n.Translator
	searchHealth   httpapi.SearchHealthChecker
	imageConverter httpapi.ImageConverter
	frameExtractor httpapi.FrameExtractor
}
