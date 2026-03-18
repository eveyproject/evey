package main

import (
	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	appbackup "github.com/gravity-zero/evey/internal/app/backup"
	appcollection "github.com/gravity-zero/evey/internal/app/collection"
	appfavorite "github.com/gravity-zero/evey/internal/app/favorite"
	appfeed "github.com/gravity-zero/evey/internal/app/feed"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	appmkv "github.com/gravity-zero/evey/internal/app/mkv"
	appmusic "github.com/gravity-zero/evey/internal/app/music"
	appnotification "github.com/gravity-zero/evey/internal/app/notification"
	appphoto "github.com/gravity-zero/evey/internal/app/photo"
	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	appplaylist "github.com/gravity-zero/evey/internal/app/playlist"
	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	apprating "github.com/gravity-zero/evey/internal/app/rating"
	appsubtitle "github.com/gravity-zero/evey/internal/app/subtitle"
	appsyncplay "github.com/gravity-zero/evey/internal/app/syncplay"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	appwebhook "github.com/gravity-zero/evey/internal/app/webhook"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driving/pluginhost"
	httpapi "github.com/gravity-zero/evey/internal/adapters/driving/http"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

// libraryDeps groups library scanning, querying and scheduling services.
type libraryDeps struct {
	cmdSvc    *applibrary.CommandService
	querySvc  *applibrary.QueryService
	scheduler *applibrary.Scheduler
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
	photoSvc       *appphoto.Service
	subtitleSvc    *appsubtitle.Service
	syncPlayHub    *appsyncplay.Hub
}

// contentDeps groups content-organisation services (metadata, collections, ratings).
type contentDeps struct {
	metadataCmdSvc   *appmetadata.CommandService
	metadataQuerySvc *appmetadata.QueryService
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
	pluginInstallerSvc *appplugin.InstallerService
	pluginRuntimeSvc   *appplugin.RuntimeService
	pluginQuerySvc     *appplugin.QueryService
	pluginHostServer   *pluginhost.HostServer
}

// authDeps groups authentication infrastructure.
type authDeps struct {
	tokenGen *auth.JWTGenerator
	hasher   auth.BcryptHasher
}

// repoDeps groups repository adapters used directly by HTTP/gRPC layers.
type repoDeps struct {
	artworkRepo       *postgres.ArtworkRepository
	userRatingRepo    *postgres.UserRatingRepository
	fcmTokenRepo      *postgres.FCMDeviceTokenRepository
	libraryAccessRepo *postgres.LibraryAccessRepository
	profileRepo       *postgres.ProfileRepository
	serverCfgRepo     *postgres.ServerConfigRepository
	mediaItemRepo     *postgres.MediaItemRepository
	userRepo          *postgres.UserRepository
	apiKeyRepo        *postgres.APIKeyRepository
	pluginRepo        *postgres.PluginRepository
	quickConnectRepo  *postgres.QuickConnectRepository
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
	cfgStore   *httpapi.ServerConfigStore
	backupSvc  *appbackup.Service
}
