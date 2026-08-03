package auth

const (
	PermLibraryCreate   = "library.create"
	PermLibraryRead     = "library.read"
	PermLibraryUpdate   = "library.update"
	PermLibraryDelete   = "library.delete"
	PermLibraryScan     = "library.scan"
	PermMediaRead       = "media.read"
	PermMediaUpdate     = "media.update"
	PermMediaDelete     = "media.delete"
	PermUserCreate      = "user.create"
	PermUserRead        = "user.read"
	PermUserUpdate      = "user.update"
	PermUserDelete      = "user.delete"
	PermUserDisable     = "user.disable"
	PermProfileCreate   = "profile.create"
	PermProfileUpdate   = "profile.update"
	PermProfileDelete   = "profile.delete"
	PermPlaybackStart   = "playback.start"
	PermPlaybackStop    = "playback.stop"
	PermTranscodeStart  = "transcode.start"
	PermTranscodeStop   = "transcode.stop"
	PermPluginInstall   = "plugin.install"
	PermPluginManage    = "plugin.manage"
	PermMetadataRead    = "metadata.read"
	PermMetadataUpdate  = "metadata.update"
	PermMetadataRefresh = "metadata.refresh"
	PermSubtitleWrite   = "subtitle.write"
	PermActivityRead    = "activity.read"
	PermAuditRead       = "audit.read"
	PermServerAdmin     = "server.admin"
)

// MethodPermissions maps gRPC method full names to required permissions.
var MethodPermissions = map[string]string{
	// Library
	"/evey.v1.LibraryService/CreateLibrary":      PermLibraryCreate,
	"/evey.v1.LibraryService/GetLibrary":         PermLibraryRead,
	"/evey.v1.LibraryService/ListLibraries":      PermLibraryRead,
	"/evey.v1.LibraryService/DeleteLibrary":      PermLibraryDelete,
	"/evey.v1.LibraryService/ScanLibrary":        PermLibraryScan,
	"/evey.v1.LibraryService/GetMediaItem":       PermMediaRead,
	"/evey.v1.LibraryService/GetSeriesAggregate": PermMediaRead,
	"/evey.v1.LibraryService/ListMediaItems":     PermMediaRead,
	"/evey.v1.LibraryService/SearchMedia":        PermMediaRead,
	"/evey.v1.LibraryService/FilterMediaItems":   PermMediaRead,
	"/evey.v1.LibraryService/GetPerson":          PermMediaRead,
	// User
	"/evey.v1.UserService/CreateUser":  PermUserCreate,
	"/evey.v1.UserService/ListUsers":   PermUserRead,
	"/evey.v1.UserService/UpdateUser":  PermUserUpdate,
	"/evey.v1.UserService/DeleteUser":  PermUserDelete,
	"/evey.v1.UserService/DisableUser": PermUserDisable,
	// Playback
	"/evey.v1.PlaybackService/StartStream":      PermPlaybackStart,
	"/evey.v1.PlaybackService/StopStream":       PermPlaybackStop,
	"/evey.v1.PlaybackService/GetStreamInfo":    PermMediaRead,
	"/evey.v1.PlaybackService/ReportProgress":   PermPlaybackStart,
	"/evey.v1.PlaybackService/GetWatchProgress": PermMediaRead,
	"/evey.v1.PlaybackService/GetResumeItems":   PermMediaRead,
	"/evey.v1.PlaybackService/GetNextUp":        PermMediaRead,
	// These four were listed under WRONG names until 2026-08-01 (GetHistory,
	// MarkWatched, MarkUnwatched - RPCs that never existed), so they enforced
	// nothing at all: a method absent from every table is RoleUser by default,
	// which means auth.go's step 3 never runs and an API key reaches it whatever
	// its scopes are. Listed here under their real names, a read-only key can no
	// longer write watch state.
	"/evey.v1.PlaybackService/GetWatchHistory":   PermMediaRead,
	"/evey.v1.PlaybackService/MarkAsWatched":     PermPlaybackStart,
	"/evey.v1.PlaybackService/MarkAsUnwatched":   PermPlaybackStart,
	"/evey.v1.PlaybackService/ClearWatchHistory": PermPlaybackStart,
	// Transcoding
	"/evey.v1.TranscodingService/StartTranscode": PermTranscodeStart,
	"/evey.v1.TranscodingService/StopTranscode":  PermTranscodeStop,
	// Metadata
	"/evey.v1.MetadataService/RefreshMetadata":        PermMetadataRefresh,
	"/evey.v1.MetadataService/SearchMetadata":         PermMetadataRead,
	"/evey.v1.MetadataService/SetMetadataOverride":    PermMetadataUpdate,
	"/evey.v1.MetadataService/RemoveMetadataOverride": PermMetadataUpdate,
	"/evey.v1.MetadataService/GetArtwork":             PermMetadataRead,
	// Activity
	"/evey.v1.ActivityService/GetServerStats": PermActivityRead,
	"/evey.v1.ActivityService/GetUserStats":   PermActivityRead,
	// Plugin
	"/evey.v1.AdminPluginService/InstallPlugin":   PermPluginInstall,
	"/evey.v1.AdminPluginService/UninstallPlugin": PermPluginManage,
}

// API key scopes (coarse-grained, user-facing).
const (
	ScopeMediaRead     = "media:read"
	ScopeMediaWrite    = "media:write"
	ScopeAdminRead     = "admin:read"
	ScopeAdminWrite    = "admin:write"
	ScopeUsersRead     = "users:read"
	ScopeUsersWrite    = "users:write"
	ScopePlaybackRead  = "playback:read"
	ScopePlaybackWrite = "playback:write"
)

// AllAPIKeyScopes lists the canonical set of scopes presented to users.
var AllAPIKeyScopes = []string{
	ScopeMediaRead, ScopeMediaWrite,
	ScopeAdminRead, ScopeAdminWrite,
	ScopeUsersRead, ScopeUsersWrite,
	ScopePlaybackRead, ScopePlaybackWrite,
}

// APIKeyScopeFor maps gRPC method full names to the coarse-grained API key scope
// required. Keys that include fine-grained permissions from MethodPermissions
// are also honored as a compatibility fallback.
//
// Every real evey.v1 RPC must resolve to SOMETHING here, in MethodPermissions,
// in InternalMethods, in PublicMethods, or be RoleAdmin in
// interceptors.DefaultRBACConfig().MethodRoles - interceptors/auth.go's
// enforceRBAC denies an API key outright on any method that resolves to none
// of those (fail-closed). Until 2026-08-01, 100 real RPCs had no entry
// anywhere: a method absent from every table let an API key through
// regardless of the scopes it carried, because the scope-check condition
// itself never triggered. That included ParentalService/UpdateParentalSettings
// and VerifyPin - a "read only" key could disable parental controls.
// TestAPIKeyTables_NoUnclassifiedRealMethods in rbac_test.go pins this so a
// newly added RPC that nobody remembers to classify fails the build instead of
// shipping open.
var APIKeyScopeFor = map[string]string{
	// Media read
	"/evey.v1.LibraryService/GetLibrary":                     ScopeMediaRead,
	"/evey.v1.LibraryService/ListLibraries":                  ScopeMediaRead,
	"/evey.v1.LibraryService/GetMediaItem":                   ScopeMediaRead,
	"/evey.v1.LibraryService/GetSeriesAggregate":             ScopeMediaRead,
	"/evey.v1.LibraryService/ListMediaItems":                 ScopeMediaRead,
	"/evey.v1.LibraryService/SearchMedia":                    ScopeMediaRead,
	"/evey.v1.LibraryService/FilterMediaItems":               ScopeMediaRead,
	"/evey.v1.LibraryService/GetPerson":                      ScopeMediaRead,
	"/evey.v1.LibraryService/GetPublicMediaItem":             ScopeMediaRead,
	"/evey.v1.LibraryService/ListPublicMediaItems":           ScopeMediaRead,
	"/evey.v1.LibraryService/ListGenres":                     ScopeMediaRead,
	"/evey.v1.CollectionService/GetCollection":               ScopeMediaRead,
	"/evey.v1.CollectionService/GetCollectionItems":          ScopeMediaRead,
	"/evey.v1.CollectionService/ListCollectionsForMediaItem": ScopeMediaRead,
	"/evey.v1.CollectionService/ListCollections":             ScopeMediaRead,
	"/evey.v1.FavoriteService/IsFavorite":                    ScopeMediaRead,
	"/evey.v1.FavoriteService/ListFavorites":                 ScopeMediaRead,
	"/evey.v1.MKVService/ListChapters":                       ScopeMediaRead,
	"/evey.v1.MediaRequestService/GetRequestPolicy":          ScopeMediaRead,
	"/evey.v1.MediaRequestService/ListMyRequests":            ScopeMediaRead,
	"/evey.v1.MediaRequestService/SearchTitles":              ScopeMediaRead,
	"/evey.v1.MetadataService/GetMetadataProviders":          ScopeMediaRead,
	"/evey.v1.MetadataService/ListMetadataOverrides":         ScopeMediaRead,
	"/evey.v1.MusicService/GetAlbums":                        ScopeMediaRead,
	"/evey.v1.MusicService/GetArtists":                       ScopeMediaRead,
	"/evey.v1.MusicService/GetTrack":                         ScopeMediaRead,
	"/evey.v1.MusicService/ListTracks":                       ScopeMediaRead,
	"/evey.v1.MusicService/SearchMusic":                      ScopeMediaRead,
	"/evey.v1.PhotoService/GetPhoto":                         ScopeMediaRead,
	"/evey.v1.PhotoService/ListPhotoAlbums":                  ScopeMediaRead,
	"/evey.v1.PhotoService/ListPhotos":                       ScopeMediaRead,
	"/evey.v1.PhotoService/SearchPhotos":                     ScopeMediaRead,
	"/evey.v1.PlaylistService/GetPlaylist":                   ScopeMediaRead,
	"/evey.v1.PlaylistService/GetPlaylistItems":              ScopeMediaRead,
	"/evey.v1.PlaylistService/ListPlaylists":                 ScopeMediaRead,
	"/evey.v1.RatingService/GetMyThumbRating":                ScopeMediaRead,
	"/evey.v1.RatingService/GetRatingAggregate":              ScopeMediaRead,
	"/evey.v1.ShareService/ValidateShareLink":                ScopeMediaRead,
	// ActivityService/GetPopularMedia is a catalog stat (what's popular right
	// now), not account activity - grouped with media reads rather than the
	// admin-only ActivityService entries below.
	"/evey.v1.ActivityService/GetPopularMedia": ScopeMediaRead,
	// Media write
	"/evey.v1.LibraryService/ScanLibrary":             ScopeMediaWrite,
	"/evey.v1.MetadataService/RefreshMetadata":        ScopeMediaWrite,
	"/evey.v1.MetadataService/SetMetadataOverride":    ScopeMediaWrite,
	"/evey.v1.MetadataService/RemoveMetadataOverride": ScopeMediaWrite,
	"/evey.v1.MetadataService/UpdateMetadata":         ScopeMediaWrite,
	"/evey.v1.MetadataService/DeleteMetadata":         ScopeMediaWrite,
	// ResearchMetadata mutates: it deletes every override on the item and
	// re-matches it against providers (see app/metadata CommandService.ResearchMetadata).
	"/evey.v1.MetadataService/ResearchMetadata":   ScopeMediaWrite,
	"/evey.v1.FavoriteService/AddFavorite":        ScopeMediaWrite,
	"/evey.v1.FavoriteService/RemoveFavorite":     ScopeMediaWrite,
	"/evey.v1.MediaRequestService/CreateRequest":  ScopeMediaWrite,
	"/evey.v1.PhotoService/ScanPhotoDirectory":    ScopeMediaWrite,
	"/evey.v1.PlaylistService/AddPlaylistItem":    ScopeMediaWrite,
	"/evey.v1.PlaylistService/CreatePlaylist":     ScopeMediaWrite,
	"/evey.v1.PlaylistService/DeletePlaylist":     ScopeMediaWrite,
	"/evey.v1.PlaylistService/RemovePlaylistItem": ScopeMediaWrite,
	"/evey.v1.PlaylistService/UpdatePlaylist":     ScopeMediaWrite,
	"/evey.v1.RatingService/PutThumbRating":       ScopeMediaWrite,
	// RecordPageView is a low-sensitivity telemetry ping with no closer coarse
	// bucket among the four scope domains; grouped under media:write as the
	// least-bad fit rather than left unclassified. Flagged in the dispatch report.
	"/evey.v1.ActivityService/RecordPageView": ScopeMediaWrite,
	// Users read
	"/evey.v1.UserService/ListUsers":                       ScopeUsersRead,
	"/evey.v1.UserService/GetCurrentUser":                  ScopeUsersRead,
	"/evey.v1.UserService/ListInviteLinks":                 ScopeUsersRead,
	"/evey.v1.UserService/ListProfiles":                    ScopeUsersRead,
	"/evey.v1.ParentalService/GetParentalSettings":         ScopeUsersRead,
	"/evey.v1.PluginUserDataService/GetPref":               ScopeUsersRead,
	"/evey.v1.PluginUserDataService/ListPrefs":             ScopeUsersRead,
	"/evey.v1.PushService/GetPushConsent":                  ScopeUsersRead,
	"/evey.v1.PushService/GetVAPIDKey":                     ScopeUsersRead,
	"/evey.v1.PushService/ListDevices":                     ScopeUsersRead,
	"/evey.v1.PushService/ListWebPushSubscriptions":        ScopeUsersRead,
	"/evey.v1.NotificationService/GetNotificationSettings": ScopeUsersRead,
	"/evey.v1.NotificationService/GetUnreadCount":          ScopeUsersRead,
	"/evey.v1.NotificationService/ListNotifications":       ScopeUsersRead,
	// GetLoginHistory defaults to the caller's own history (handler allows
	// admins to view others'); it's account-security data, not admin analytics.
	"/evey.v1.ActivityService/GetLoginHistory": ScopeUsersRead,
	// Users write
	"/evey.v1.UserService/UpdateUser":                         ScopeUsersWrite,
	"/evey.v1.UserService/CreateProfile":                      ScopeUsersWrite,
	"/evey.v1.UserService/DeleteProfile":                      ScopeUsersWrite,
	"/evey.v1.UserService/UpdateProfile":                      ScopeUsersWrite,
	"/evey.v1.UserService/Logout":                             ScopeUsersWrite,
	"/evey.v1.UserService/RevokeInviteLink":                   ScopeUsersWrite,
	"/evey.v1.ParentalService/UpdateParentalSettings":         ScopeUsersWrite,
	"/evey.v1.ParentalService/VerifyPin":                      ScopeUsersWrite,
	"/evey.v1.PluginUserDataService/DeletePref":               ScopeUsersWrite,
	"/evey.v1.PluginUserDataService/SetPref":                  ScopeUsersWrite,
	"/evey.v1.PushService/DeleteWebPushSubscription":          ScopeUsersWrite,
	"/evey.v1.PushService/RegisterWebPush":                    ScopeUsersWrite,
	"/evey.v1.PushService/UnregisterWebPush":                  ScopeUsersWrite,
	"/evey.v1.PushService/UpdatePushConsent":                  ScopeUsersWrite,
	"/evey.v1.NotificationService/DeleteAllNotifications":     ScopeUsersWrite,
	"/evey.v1.NotificationService/MarkAllRead":                ScopeUsersWrite,
	"/evey.v1.NotificationService/MarkRead":                   ScopeUsersWrite,
	"/evey.v1.NotificationService/UpdateNotificationSettings": ScopeUsersWrite,
	// Playback read
	"/evey.v1.PlaybackService/GetStreamInfo":               ScopePlaybackRead,
	"/evey.v1.PlaybackService/GetWatchProgress":            ScopePlaybackRead,
	"/evey.v1.PlaybackService/GetResumeItems":              ScopePlaybackRead,
	"/evey.v1.PlaybackService/GetNextUp":                   ScopePlaybackRead,
	"/evey.v1.PlaybackService/GetWatchHistory":             ScopePlaybackRead,
	"/evey.v1.FeedService/ContinueWatching":                ScopePlaybackRead,
	"/evey.v1.FeedService/GetFeedConfig":                   ScopePlaybackRead,
	"/evey.v1.FeedService/NextEpisode":                     ScopePlaybackRead,
	"/evey.v1.FeedService/RecentlyAdded":                   ScopePlaybackRead,
	"/evey.v1.FeedService/Recommended":                     ScopePlaybackRead,
	"/evey.v1.FeedService/ResumePlayback":                  ScopePlaybackRead,
	"/evey.v1.FeedService/SimilarItems":                    ScopePlaybackRead,
	"/evey.v1.PlaybackSettingsService/GetPlaybackSettings": ScopePlaybackRead,
	"/evey.v1.TranscodingService/GetTranscodeStatus":       ScopePlaybackRead,
	"/evey.v1.TranscodingService/ListTranscodeProfiles":    ScopePlaybackRead,
	"/evey.v1.SyncPlayService/GetRoom":                     ScopePlaybackRead,
	// GetPlayHistory mirrors PlaybackService/GetWatchHistory (per-user viewing
	// history), just served off the activity log instead of watch state.
	"/evey.v1.ActivityService/GetPlayHistory": ScopePlaybackRead,
	// Playback write
	"/evey.v1.PlaybackService/StartStream":         ScopePlaybackWrite,
	"/evey.v1.PlaybackService/StopStream":          ScopePlaybackWrite,
	"/evey.v1.PlaybackService/ReportProgress":      ScopePlaybackWrite,
	"/evey.v1.PlaybackService/MarkAsWatched":       ScopePlaybackWrite,
	"/evey.v1.PlaybackService/MarkAsUnwatched":     ScopePlaybackWrite,
	"/evey.v1.PlaybackService/ClearWatchHistory":   ScopePlaybackWrite,
	"/evey.v1.FeedService/ResetProfileFeedConfig":  ScopePlaybackWrite,
	"/evey.v1.FeedService/UpdateProfileFeedConfig": ScopePlaybackWrite,
	"/evey.v1.SyncPlayService/CreateRoom":          ScopePlaybackWrite,
	"/evey.v1.SyncPlayService/JoinByCode":          ScopePlaybackWrite,
	"/evey.v1.SyncPlayService/JoinRoom":            ScopePlaybackWrite,
	"/evey.v1.SyncPlayService/LeaveRoom":           ScopePlaybackWrite,
	// Heartbeat mutates the in-memory presence tracker on every call; grouped
	// under playback:write (an active-session signal) rather than users:write
	// since it carries no persisted account data. Flagged in the dispatch report.
	"/evey.v1.PresenceService/Heartbeat": ScopePlaybackWrite,
	// Admin read
	"/evey.v1.ActivityService/GetServerStats": ScopeAdminRead,
	"/evey.v1.ActivityService/GetUserStats":   ScopeAdminRead,
	// GetAnalytics is handler-enforced admin-only (see activity_handler.go),
	// same bucket as GetServerStats/GetUserStats.
	"/evey.v1.ActivityService/GetAnalytics":      ScopeAdminRead,
	"/evey.v1.LibraryService/GetLibrarySchedule": ScopeAdminRead,
	// Admin write
	// LibrarySchedule mutations are handler-enforced admin-only (library_handler.go),
	// same bucket as ScanLibrary's server-config siblings.
	"/evey.v1.LibraryService/SetLibrarySchedule":    ScopeAdminWrite,
	"/evey.v1.LibraryService/DeleteLibrarySchedule": ScopeAdminWrite,
	// IssuePluginToken mints an auth token and is callable by any authenticated
	// user (see interceptors/rbac.go comment on MethodRoles), not just admins -
	// admin:write is the least-bad fit among the four scope domains for an
	// AdminPluginService mutation with no per-user analog. Flagged in the
	// dispatch report.
	"/evey.v1.AdminPluginService/IssuePluginToken": ScopeAdminWrite,
}

// InternalMethods lists methods that can ONLY be called via JWT (browser/app).
// API keys are blocked from calling these - they are infrastructure/ops/auth
// concerns, not domain operations.
//
// It is the FIRST of two layers, not the authoritative list: the interceptor
// also denies by default any ADMIN method that maps to no API-key scope
// (interceptors/auth.go, step 2), so a new admin RPC is unreachable by an API
// key even when nobody remembers to add it here. Reading this list as "the
// complete set of what API keys cannot do" repeatedly produced audit findings
// like "RestartServer is missing" - it is covered, one layer down.
// TestAPIKey_AdminMethodsDenyByDefault pins that second layer.
var InternalMethods = map[string]bool{
	// Auth / Session management
	"/evey.v1.UserService/Login":                true,
	"/evey.v1.UserService/RefreshToken":         true,
	"/evey.v1.UserService/RegisterWithInvite":   true,
	"/evey.v1.UserService/InitiateQuickConnect": true,
	"/evey.v1.UserService/CheckQuickConnect":    true,
	// AuthorizeQuickConnect belongs HERE, with its two siblings, and not in
	// APIKeyScopeFor where the 2026-08-01 classification pass first put it under
	// users:write. It hands the CALLER'S OWN SESSION to whatever device is
	// waiting on the code: a restricted API key that could call it would turn
	// itself into a full JWT session on a device the key holder controls. That
	// is privilege escalation, not a user-data write.
	"/evey.v1.UserService/AuthorizeQuickConnect": true,
	"/evey.v1.UserService/ResetPassword":         true,

	// User lifecycle (admin infra)
	"/evey.v1.UserService/CreateUser":   true,
	"/evey.v1.UserService/DeleteUser":   true,
	"/evey.v1.UserService/DisableUser":  true,
	"/evey.v1.UserService/ListUsers":    true,
	"/evey.v1.UserService/CreateAPIKey": true,
	"/evey.v1.UserService/ListAPIKeys":  true,
	"/evey.v1.UserService/RevokeAPIKey": true,

	// AdminSystemService
	"/evey.v1.AdminSystemService/SetupWizard":        true,
	"/evey.v1.AdminSystemService/GetSetupStatus":     true,
	"/evey.v1.AdminSystemService/GetSystemInfo":      true,
	"/evey.v1.AdminSystemService/GetServerConfig":    true,
	"/evey.v1.AdminSystemService/UpdateServerConfig": true,
	"/evey.v1.AdminSystemService/GetDashboard":       true,

	// AdminTaskService
	"/evey.v1.AdminTaskService/ListScheduledTasks":  true,
	"/evey.v1.AdminTaskService/UpdateScheduledTask": true,
	"/evey.v1.AdminTaskService/RunScheduledTask":    true,

	// AdminSessionService
	"/evey.v1.AdminSessionService/GetAuditLog": true,

	// AdminMediaService is not listed here: its real RPCs (RefreshAllMetadata,
	// MatchOrphansInLibrary, ScanSeekIndex, ...) are RoleAdmin in
	// interceptors.DefaultRBACConfig().MethodRoles, so an API key is already
	// blocked by the deny-by-default admin check (auth.go step 2) without
	// needing a redundant entry here. Proven by
	// TestAdminMediaService_RealMethodsDenyByDefaultForAPIKeys.

	// AdminPluginService
	"/evey.v1.AdminPluginService/ListPluginRuntimes":  true,
	"/evey.v1.AdminPluginService/AddPluginRuntime":    true,
	"/evey.v1.AdminPluginService/RemovePluginRuntime": true,
	"/evey.v1.AdminPluginService/InstallPlugin":       true,
	"/evey.v1.AdminPluginService/UninstallPlugin":     true,
	"/evey.v1.AdminPluginService/EnablePlugin":        true,
	"/evey.v1.AdminPluginService/DisablePlugin":       true,
	"/evey.v1.AdminPluginService/ListPlugins":         true,
	"/evey.v1.AdminPluginService/GetPluginConfig":     true,
	"/evey.v1.AdminPluginService/UpdatePluginConfig":  true,
	"/evey.v1.AdminPluginService/GetPluginLogs":       true,

	// Library infra (create/delete/scan)
	"/evey.v1.LibraryService/CreateLibrary": true,
	"/evey.v1.LibraryService/UpdateLibrary": true,
	"/evey.v1.LibraryService/DeleteLibrary": true,

	// Server config store
	"/evey.v1.ServerConfigService/GetServerConfigEntry":    true,
	"/evey.v1.ServerConfigService/SetServerConfigEntry":    true,
	"/evey.v1.ServerConfigService/ListServerConfigEntries": true,
	"/evey.v1.ServerConfigService/DeleteServerConfigEntry": true,

	// Remote access
	"/evey.v1.RemoteAccessService/GetRemoteAccess":    true,
	"/evey.v1.RemoteAccessService/UpdateRemoteAccess": true,
	"/evey.v1.RemoteAccessService/GetNetworkInfo":     true,

	// Backup/Restore
	"/evey.v1.BackupService/CreateBackup": true,
	"/evey.v1.BackupService/ListBackups":  true,
	"/evey.v1.BackupService/DeleteBackup": true,

	// Transcode cache
	"/evey.v1.TranscodeCacheService/GetTranscodeCache":        true,
	"/evey.v1.TranscodeCacheService/ClearTranscodeCache":      true,
	"/evey.v1.TranscodeCacheService/DeleteTranscodeCacheFile": true,

	// MKV tools
	"/evey.v1.MKVService/MuxContainer":   true,
	"/evey.v1.MKVService/DemuxContainer": true,
	"/evey.v1.MKVService/AddTrack":       true,
	"/evey.v1.MKVService/RemoveTrack":    true,

	// Playback settings (write)
	"/evey.v1.PlaybackSettingsService/UpdatePlaybackSettings": true,

	// Library access management
	"/evey.v1.LibraryAccessService/GetUserLibraryAccess":       true,
	"/evey.v1.LibraryAccessService/SetUserLibraryAccess":       true,
	"/evey.v1.LibraryAccessService/RevokeAllUserLibraryAccess": true,

	// Feed admin
	"/evey.v1.FeedService/GetAdminFeedConfigs":         true,
	"/evey.v1.FeedService/UpdateAdminFeedConfig":       true,
	"/evey.v1.FeedService/BatchUpdateAdminFeedConfigs": true,

	// Notification admin
	"/evey.v1.NotificationService/GetGlobalNotificationSettings":    true,
	"/evey.v1.NotificationService/UpdateGlobalNotificationSettings": true,

	// Transcoding admin
	"/evey.v1.TranscodingService/StopTranscode":        true,
	"/evey.v1.TranscodingService/ListActiveTranscodes": true,
}
