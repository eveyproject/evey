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
	"/evey.v1.PlaybackService/GetHistory":       PermActivityRead,
	"/evey.v1.PlaybackService/GetResumeItems":   PermMediaRead,
	"/evey.v1.PlaybackService/MarkWatched":      PermPlaybackStart,
	"/evey.v1.PlaybackService/MarkUnwatched":    PermPlaybackStart,
	"/evey.v1.PlaybackService/GetNextUp":        PermMediaRead,
	// Transcoding
	"/evey.v1.TranscodingService/StartTranscode": PermTranscodeStart,
	"/evey.v1.TranscodingService/StopTranscode":  PermTranscodeStop,
	// Metadata
	"/evey.v1.MetadataService/RefreshMetadata":        PermMetadataRefresh,
	"/evey.v1.MetadataService/SearchMetadata":         PermMetadataRead,
	"/evey.v1.MetadataService/SetMetadataOverride":    PermMetadataUpdate,
	"/evey.v1.MetadataService/RemoveMetadataOverride": PermMetadataUpdate,
	"/evey.v1.MetadataService/GetArtwork":             PermMetadataRead,
	"/evey.v1.MetadataService/GetOverrides":           PermMetadataRead,
	// Activity
	"/evey.v1.ActivityService/GetAuditLog":    PermAuditRead,
	"/evey.v1.ActivityService/GetServerStats": PermActivityRead,
	"/evey.v1.ActivityService/GetHistory":     PermActivityRead,
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
var APIKeyScopeFor = map[string]string{
	// Media read
	"/evey.v1.LibraryService/GetLibrary":         ScopeMediaRead,
	"/evey.v1.LibraryService/ListLibraries":      ScopeMediaRead,
	"/evey.v1.LibraryService/GetMediaItem":       ScopeMediaRead,
	"/evey.v1.LibraryService/GetSeriesAggregate": ScopeMediaRead,
	"/evey.v1.LibraryService/ListMediaItems":     ScopeMediaRead,
	"/evey.v1.LibraryService/SearchMedia":        ScopeMediaRead,
	"/evey.v1.LibraryService/FilterMediaItems":   ScopeMediaRead,
	"/evey.v1.LibraryService/GetPerson":          ScopeMediaRead,
	// Media write
	"/evey.v1.LibraryService/ScanLibrary":             ScopeMediaWrite,
	"/evey.v1.MetadataService/RefreshMetadata":        ScopeMediaWrite,
	"/evey.v1.MetadataService/SetMetadataOverride":    ScopeMediaWrite,
	"/evey.v1.MetadataService/RemoveMetadataOverride": ScopeMediaWrite,
	// Users read
	"/evey.v1.UserService/ListUsers": ScopeUsersRead,
	// Users write
	"/evey.v1.UserService/UpdateUser": ScopeUsersWrite,
	// Playback read
	"/evey.v1.PlaybackService/GetStreamInfo":    ScopePlaybackRead,
	"/evey.v1.PlaybackService/GetWatchProgress": ScopePlaybackRead,
	"/evey.v1.PlaybackService/GetHistory":       ScopePlaybackRead,
	"/evey.v1.PlaybackService/GetResumeItems":   ScopePlaybackRead,
	"/evey.v1.PlaybackService/GetNextUp":        ScopePlaybackRead,
	// Playback write
	"/evey.v1.PlaybackService/StartStream":    ScopePlaybackWrite,
	"/evey.v1.PlaybackService/StopStream":     ScopePlaybackWrite,
	"/evey.v1.PlaybackService/ReportProgress": ScopePlaybackWrite,
	"/evey.v1.PlaybackService/MarkWatched":    ScopePlaybackWrite,
	"/evey.v1.PlaybackService/MarkUnwatched":  ScopePlaybackWrite,
	// Admin read
	"/evey.v1.ActivityService/GetAuditLog":    ScopeAdminRead,
	"/evey.v1.ActivityService/GetServerStats": ScopeAdminRead,
	"/evey.v1.ActivityService/GetUserStats":   ScopeAdminRead,
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
	"/evey.v1.UserService/ChangePassword":       true,
	"/evey.v1.UserService/ForgotPassword":       true,
	"/evey.v1.UserService/ResetPassword":        true,
	"/evey.v1.UserService/VerifyEmail":          true,

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

	// AdminMediaService
	"/evey.v1.AdminMediaService/EnrichPeopleBios":    true,
	"/evey.v1.AdminMediaService/GetUserBitrateLimit": true,
	"/evey.v1.AdminMediaService/SetUserBitrateLimit": true,

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
