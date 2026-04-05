package auth

const (
	PermLibraryCreate  = "library.create"
	PermLibraryRead    = "library.read"
	PermLibraryUpdate  = "library.update"
	PermLibraryDelete  = "library.delete"
	PermLibraryScan    = "library.scan"
	PermMediaRead      = "media.read"
	PermMediaUpdate    = "media.update"
	PermMediaDelete    = "media.delete"
	PermUserCreate     = "user.create"
	PermUserRead       = "user.read"
	PermUserUpdate     = "user.update"
	PermUserDelete     = "user.delete"
	PermUserDisable    = "user.disable"
	PermProfileCreate  = "profile.create"
	PermProfileUpdate  = "profile.update"
	PermProfileDelete  = "profile.delete"
	PermPlaybackStart  = "playback.start"
	PermPlaybackStop   = "playback.stop"
	PermTranscodeStart = "transcode.start"
	PermTranscodeStop  = "transcode.stop"
	PermPluginInstall  = "plugin.install"
	PermPluginManage   = "plugin.manage"
	PermMetadataRead   = "metadata.read"
	PermMetadataUpdate = "metadata.update"
	PermMetadataRefresh = "metadata.refresh"
	PermSubtitleWrite  = "subtitle.write"
	PermActivityRead   = "activity.read"
	PermAuditRead      = "audit.read"
	PermServerAdmin    = "server.admin"
)

// MethodPermissions maps gRPC method full names to required permissions.
var MethodPermissions = map[string]string{
	// Library
	"/evey.v1.LibraryService/CreateLibrary":       PermLibraryCreate,
	"/evey.v1.LibraryService/GetLibrary":          PermLibraryRead,
	"/evey.v1.LibraryService/ListLibraries":       PermLibraryRead,
	"/evey.v1.LibraryService/DeleteLibrary":       PermLibraryDelete,
	"/evey.v1.LibraryService/ScanLibrary":         PermLibraryScan,
	"/evey.v1.LibraryService/GetMediaItem":        PermMediaRead,
	"/evey.v1.LibraryService/ListMediaItems":      PermMediaRead,
	"/evey.v1.LibraryService/SearchMedia":         PermMediaRead,
	"/evey.v1.LibraryService/FilterMediaItems":    PermMediaRead,
	"/evey.v1.LibraryService/GetPerson":           PermMediaRead,
	// User
	"/evey.v1.UserService/CreateUser":             PermUserCreate,
	"/evey.v1.UserService/ListUsers":              PermUserRead,
	"/evey.v1.UserService/UpdateUser":             PermUserUpdate,
	"/evey.v1.UserService/DeleteUser":             PermUserDelete,
	"/evey.v1.UserService/DisableUser":            PermUserDisable,
	// Playback
	"/evey.v1.PlaybackService/StartStream":        PermPlaybackStart,
	"/evey.v1.PlaybackService/StopStream":         PermPlaybackStop,
	"/evey.v1.PlaybackService/GetStreamInfo":      PermMediaRead,
	"/evey.v1.PlaybackService/ReportProgress":     PermPlaybackStart,
	"/evey.v1.PlaybackService/GetWatchProgress":   PermMediaRead,
	"/evey.v1.PlaybackService/GetHistory":         PermActivityRead,
	"/evey.v1.PlaybackService/GetResumeItems":     PermMediaRead,
	"/evey.v1.PlaybackService/MarkWatched":        PermPlaybackStart,
	"/evey.v1.PlaybackService/MarkUnwatched":      PermPlaybackStart,
	"/evey.v1.PlaybackService/GetNextUp":          PermMediaRead,
	// Transcoding
	"/evey.v1.TranscodingService/StartTranscode":  PermTranscodeStart,
	"/evey.v1.TranscodingService/StopTranscode":   PermTranscodeStop,
	// Metadata
	"/evey.v1.MetadataService/RefreshMetadata":    PermMetadataRefresh,
	"/evey.v1.MetadataService/SearchMetadata":     PermMetadataRead,
	"/evey.v1.MetadataService/SetMetadataOverride": PermMetadataUpdate,
	"/evey.v1.MetadataService/RemoveMetadataOverride": PermMetadataUpdate,
	"/evey.v1.MetadataService/GetArtwork":         PermMetadataRead,
	"/evey.v1.MetadataService/GetOverrides":       PermMetadataRead,
	// Activity
	"/evey.v1.ActivityService/GetAuditLog":        PermAuditRead,
	"/evey.v1.ActivityService/GetServerStats":     PermActivityRead,
	"/evey.v1.ActivityService/GetHistory":         PermActivityRead,
	"/evey.v1.ActivityService/GetUserStats":       PermActivityRead,
	// Plugin
	"/evey.v1.AdminService/InstallPlugin":         PermPluginInstall,
	"/evey.v1.AdminService/UninstallPlugin":       PermPluginManage,
}

// InternalMethods lists methods that can ONLY be called via JWT (browser/app).
// API keys are blocked from calling these — they are infrastructure/ops/auth
// concerns, not domain operations.
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
	"/evey.v1.UserService/CreateUser":               true,
	"/evey.v1.UserService/DeleteUser":               true,
	"/evey.v1.UserService/DisableUser":              true,
	"/evey.v1.UserService/ListUsers":                true,
	"/evey.v1.UserService/CreateAPIKey":             true,
	"/evey.v1.UserService/ListAPIKeys":              true,
	"/evey.v1.UserService/RevokeAPIKey":             true,

	// Admin / Infrastructure
	"/evey.v1.AdminService/SetupWizard":             true,
	"/evey.v1.AdminService/GetSetupStatus":          true,
	"/evey.v1.AdminService/GetSystemInfo":           true,
	"/evey.v1.AdminService/GetServerConfig":         true,
	"/evey.v1.AdminService/UpdateServerConfig":      true,
	"/evey.v1.AdminService/GetDashboard":            true,
	"/evey.v1.AdminService/ListScheduledTasks":      true,
	"/evey.v1.AdminService/UpdateScheduledTask":     true,
	"/evey.v1.AdminService/RunScheduledTask":        true,
	"/evey.v1.AdminService/GetAuditLog":             true,
	"/evey.v1.AdminService/GetSystemStats":          true,
	"/evey.v1.AdminService/EnrichPeopleBios":        true,
	"/evey.v1.AdminService/GetActiveSessions":       true,
	"/evey.v1.AdminService/KillSession":             true,
	"/evey.v1.AdminService/GetUserBitrateLimit":     true,
	"/evey.v1.AdminService/SetUserBitrateLimit":     true,

	// Plugin management (infra, not plugin API itself)
	"/evey.v1.AdminService/ListPluginRuntimes":      true,
	"/evey.v1.AdminService/AddPluginRuntime":        true,
	"/evey.v1.AdminService/RemovePluginRuntime":     true,
	"/evey.v1.AdminService/InstallPlugin":           true,
	"/evey.v1.AdminService/UninstallPlugin":         true,
	"/evey.v1.AdminService/EnablePlugin":            true,
	"/evey.v1.AdminService/DisablePlugin":           true,
	"/evey.v1.AdminService/ListPlugins":             true,
	"/evey.v1.AdminService/GetPluginConfig":         true,
	"/evey.v1.AdminService/UpdatePluginConfig":      true,
	"/evey.v1.AdminService/GetPluginLogs":           true,

	// Library infra (create/delete/scan)
	"/evey.v1.LibraryService/CreateLibrary":         true,
	"/evey.v1.LibraryService/UpdateLibrary":         true,
	"/evey.v1.LibraryService/DeleteLibrary":         true,

	// Server config store
	"/evey.v1.ServerConfigService/GetServerConfigEntry":    true,
	"/evey.v1.ServerConfigService/SetServerConfigEntry":    true,
	"/evey.v1.ServerConfigService/ListServerConfigEntries": true,
	"/evey.v1.ServerConfigService/DeleteServerConfigEntry": true,

	// Remote access
	"/evey.v1.RemoteAccessService/GetRemoteAccess":   true,
	"/evey.v1.RemoteAccessService/UpdateRemoteAccess": true,
	"/evey.v1.RemoteAccessService/GetNetworkInfo":     true,

	// Backup/Restore
	"/evey.v1.BackupService/CreateBackup":  true,
	"/evey.v1.BackupService/ListBackups":   true,
	"/evey.v1.BackupService/DeleteBackup":  true,

	// Transcode cache
	"/evey.v1.TranscodeCacheService/GetTranscodeCache":        true,
	"/evey.v1.TranscodeCacheService/ClearTranscodeCache":      true,
	"/evey.v1.TranscodeCacheService/DeleteTranscodeCacheFile": true,

	// MKV tools
	"/evey.v1.MKVService/MuxContainer":    true,
	"/evey.v1.MKVService/DemuxContainer":  true,
	"/evey.v1.MKVService/AddTrack":        true,
	"/evey.v1.MKVService/RemoveTrack":     true,

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
	"/evey.v1.TranscodingService/StopTranscode":         true,
	"/evey.v1.TranscodingService/ListActiveTranscodes":  true,
}
