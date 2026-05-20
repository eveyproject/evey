package interceptors

// Role represents an access level required to call a gRPC method.
type Role int

const (
	RoleUser  Role = iota // any authenticated user
	RoleAdmin             // admin only
)

// RBACConfig holds the method-to-role mapping and public (unauthenticated) methods.
type RBACConfig struct {
	PublicMethods map[string]bool
	MethodRoles   map[string]Role
}

// DefaultRBACConfig returns the built-in RBAC policy.
func DefaultRBACConfig() RBACConfig {
	return RBACConfig{
		PublicMethods: map[string]bool{
			"/evey.v1.UserService/Login":                true,
			"/evey.v1.UserService/RefreshToken":         true,
			"/evey.v1.UserService/RegisterWithInvite":   true,
			"/evey.v1.AdminSystemService/SetupWizard":    true,
			"/evey.v1.AdminSystemService/GetSetupStatus": true,
			"/evey.v1.UserService/InitiateQuickConnect":  true,
			"/evey.v1.UserService/CheckQuickConnect":     true,
			"/evey.v1.UserService/RequestPasswordReset": true,
			"/evey.v1.UserService/ResetPassword":        true,
		},
		MethodRoles: map[string]Role{
			"/evey.v1.AdminSystemService/GetSystemInfo":          RoleAdmin,
			"/evey.v1.AdminSystemService/GetServerConfig":       RoleAdmin,
			"/evey.v1.AdminSystemService/UpdateServerConfig":    RoleAdmin,
			"/evey.v1.AdminSystemService/GetDashboard":          RoleAdmin,
			"/evey.v1.AdminTaskService/ListScheduledTasks":      RoleAdmin,
			"/evey.v1.AdminTaskService/UpdateScheduledTask":     RoleAdmin,
			"/evey.v1.AdminTaskService/RunScheduledTask":        RoleAdmin,
			"/evey.v1.AdminSessionService/GetAuditLog":          RoleAdmin,
			"/evey.v1.AdminSessionService/ListActiveSessions":  RoleAdmin,
			"/evey.v1.AdminSessionService/KillSession":         RoleAdmin,
			"/evey.v1.AdminSessionService/SendSessionCommand":  RoleAdmin,
			"/evey.v1.UserService/CreateInviteLink":            RoleAdmin,
			"/evey.v1.AdminPluginService/ListPluginRuntimes":    RoleAdmin,
			"/evey.v1.AdminPluginService/AddPluginRuntime":      RoleAdmin,
			"/evey.v1.AdminPluginService/RemovePluginRuntime":   RoleAdmin,
			"/evey.v1.AdminPluginService/InstallPlugin":         RoleAdmin,
			"/evey.v1.AdminPluginService/UninstallPlugin":       RoleAdmin,
			"/evey.v1.AdminPluginService/EnablePlugin":          RoleAdmin,
			"/evey.v1.AdminPluginService/DisablePlugin":         RoleAdmin,
			"/evey.v1.AdminPluginService/ListPlugins":           RoleAdmin,
			"/evey.v1.AdminPluginService/GetPluginConfig":       RoleAdmin,
			"/evey.v1.AdminPluginService/UpdatePluginConfig":    RoleAdmin,
			"/evey.v1.AdminPluginService/GetPluginLogs":         RoleAdmin,
			// Note: IssuePluginToken intentionally absent from MethodRoles —
			// it resolves to the default RoleUser since iframe plugin consumers
			// (any authenticated user) need to mint tokens. Scopes are filtered
			// via FilterPluginScopes in the service layer (admin/system stripped).
			"/evey.v1.AdminMediaService/RefreshAllMetadata":    RoleAdmin,
			"/evey.v1.AdminMediaService/MatchOrphansInLibrary": RoleAdmin,
			"/evey.v1.AdminMediaService/GenerateAllTrickplays": RoleAdmin,
			"/evey.v1.AdminMediaService/DeleteMediaItem":       RoleAdmin,
			"/evey.v1.WebhookService/CreateWebhook":                         RoleAdmin,
			"/evey.v1.WebhookService/ListWebhooks":                          RoleAdmin,
			"/evey.v1.WebhookService/UpdateWebhook":                         RoleAdmin,
			"/evey.v1.WebhookService/DeleteWebhook":                         RoleAdmin,
			"/evey.v1.WebhookService/ListWebhookDeliveries":                 RoleAdmin,
			"/evey.v1.UserService/CreateUser":                               RoleAdmin,
			"/evey.v1.UserService/DeleteUser":                               RoleAdmin,
			"/evey.v1.UserService/DisableUser":                              RoleAdmin,
			"/evey.v1.UserService/ListUsers":                                RoleAdmin,
			"/evey.v1.UserService/SetDefaultProfile":                        RoleAdmin,
			"/evey.v1.UserService/CreateAPIKey":                             RoleAdmin,
			"/evey.v1.UserService/ListAPIKeys":                              RoleAdmin,
			"/evey.v1.UserService/RevokeAPIKey":                             RoleAdmin,
			"/evey.v1.LibraryService/CreateLibrary":                         RoleAdmin,
			"/evey.v1.LibraryService/UpdateLibrary":                         RoleAdmin,
			"/evey.v1.LibraryService/DeleteLibrary":                         RoleAdmin,
			"/evey.v1.LibraryService/ScanLibrary":                           RoleAdmin,
			"/evey.v1.MusicService/DeleteTrack":                             RoleAdmin,
			"/evey.v1.MusicService/ScanMusicDirectory":                      RoleAdmin,
			"/evey.v1.FeedService/GetAdminFeedConfigs":                      RoleAdmin,
			"/evey.v1.FeedService/UpdateAdminFeedConfig":                    RoleAdmin,
			"/evey.v1.FeedService/BatchUpdateAdminFeedConfigs":              RoleAdmin,
			"/evey.v1.NotificationService/GetGlobalNotificationSettings":    RoleAdmin,
			"/evey.v1.NotificationService/UpdateGlobalNotificationSettings": RoleAdmin,
			"/evey.v1.NotificationService/ListAdminNotifications":           RoleAdmin,
			"/evey.v1.ActivityService/GetServerStats":                       RoleAdmin,
			"/evey.v1.ActivityService/ListServerMetrics":                    RoleAdmin,
			// MetadataService is intentionally NOT gated to admin: editing
			// metadata is a per-item action, gated inside the handler via
			// checkMediaItemAccess so a non-admin can only refresh / override
			// items in libraries they actually have access to.
			"/evey.v1.MKVService/GetContainerInfo":                          RoleAdmin,
			"/evey.v1.MKVService/ListTracks":                                RoleAdmin,
			"/evey.v1.MKVService/ExtractTrack":                              RoleAdmin,
			"/evey.v1.MKVService/MuxContainer":                              RoleAdmin,
			"/evey.v1.MKVService/DemuxContainer":                            RoleAdmin,
			"/evey.v1.MKVService/AddTrack":                                  RoleAdmin,
			"/evey.v1.MKVService/RemoveTrack":                               RoleAdmin,
			// ListAttachments reads arbitrary filesystem paths; ExtractAttachment
			// also WRITES the attachment to a caller-supplied OutputPath (arbitrary
			// file write). Neither is used by the player, so gate both to admin.
			// ListChapters is intentionally NOT here: the player's MKV chapter /
			// skip-intro fallback calls it for regular users.
			"/evey.v1.MKVService/ListAttachments":                          RoleAdmin,
			"/evey.v1.MKVService/ExtractAttachment":                        RoleAdmin,
			"/evey.v1.TranscodingService/StopTranscode":                     RoleAdmin,
			"/evey.v1.TranscodingService/ListActiveTranscodes":              RoleAdmin,
			"/evey.v1.TranscodeCacheService/GetTranscodeCache":              RoleAdmin,
			"/evey.v1.TranscodeCacheService/ClearTranscodeCache":            RoleAdmin,
			"/evey.v1.TranscodeCacheService/DeleteTranscodeCacheFile":       RoleAdmin,
			"/evey.v1.BackupService/CreateBackup":                           RoleAdmin,
			"/evey.v1.BackupService/ListBackups":                            RoleAdmin,
			"/evey.v1.BackupService/DeleteBackup":                           RoleAdmin,
			"/evey.v1.BackupScheduleService/GetBackupSchedule":              RoleAdmin,
			"/evey.v1.BackupScheduleService/UpdateBackupSchedule":           RoleAdmin,
			"/evey.v1.BackupScheduleService/RunBackupNow":                   RoleAdmin,
			"/evey.v1.RemoteAccessService/GetRemoteAccess":                  RoleAdmin,
			"/evey.v1.RemoteAccessService/UpdateRemoteAccess":               RoleAdmin,
			"/evey.v1.RemoteAccessService/GetNetworkInfo":                   RoleAdmin,
			"/evey.v1.PlaybackSettingsService/UpdatePlaybackSettings":       RoleAdmin,
			"/evey.v1.LibraryAccessService/GetUserLibraryAccess":            RoleAdmin,
			"/evey.v1.LibraryAccessService/SetUserLibraryAccess":            RoleAdmin,
			"/evey.v1.LibraryAccessService/RevokeAllUserLibraryAccess":      RoleAdmin,
			"/evey.v1.ServerConfigService/GetServerConfigEntry":             RoleAdmin,
			"/evey.v1.ServerConfigService/SetServerConfigEntry":             RoleAdmin,
			"/evey.v1.ServerConfigService/ListServerConfigEntries":          RoleAdmin,
			"/evey.v1.ServerConfigService/DeleteServerConfigEntry":          RoleAdmin,
			"/evey.v1.CollectionService/DeleteCollection":                   RoleAdmin,
			"/evey.v1.CollectionService/CreateCollection":                   RoleAdmin,
			"/evey.v1.CollectionService/UpdateCollection":                   RoleAdmin,
			"/evey.v1.CollectionService/AddCollectionItem":                  RoleAdmin,
			"/evey.v1.PhotoService/DeletePhoto":                             RoleAdmin,
			"/evey.v1.AdminIntroService/InvalidateIntroCache":               RoleAdmin,
			"/evey.v1.AdminIntroService/BackfillIntroDetection":             RoleAdmin,
		},
	}
}
