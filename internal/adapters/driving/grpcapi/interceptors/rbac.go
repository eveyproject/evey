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
			"/evey.v1.AdminService/SetupWizard":         true,
			"/evey.v1.AdminService/GetSetupStatus":      true,
			"/evey.v1.UserService/InitiateQuickConnect": true,
			"/evey.v1.UserService/CheckQuickConnect":    true,
		},
		MethodRoles: map[string]Role{
			"/evey.v1.AdminService/GetSystemInfo":                           RoleAdmin,
			"/evey.v1.AdminService/GetServerConfig":                         RoleAdmin,
			"/evey.v1.AdminService/UpdateServerConfig":                      RoleAdmin,
			"/evey.v1.AdminService/GetDashboard":                            RoleAdmin,
			"/evey.v1.AdminService/ListScheduledTasks":                      RoleAdmin,
			"/evey.v1.AdminService/UpdateScheduledTask":                     RoleAdmin,
			"/evey.v1.AdminService/RunScheduledTask":                        RoleAdmin,
			"/evey.v1.AdminService/GetAuditLog":                             RoleAdmin,
			"/evey.v1.AdminService/ListPluginRuntimes":                      RoleAdmin,
			"/evey.v1.AdminService/AddPluginRuntime":                        RoleAdmin,
			"/evey.v1.AdminService/RemovePluginRuntime":                     RoleAdmin,
			"/evey.v1.AdminService/InstallPlugin":                           RoleAdmin,
			"/evey.v1.AdminService/UninstallPlugin":                         RoleAdmin,
			"/evey.v1.AdminService/EnablePlugin":                            RoleAdmin,
			"/evey.v1.AdminService/DisablePlugin":                           RoleAdmin,
			"/evey.v1.AdminService/ListPlugins":                             RoleAdmin,
			"/evey.v1.AdminService/GetPluginConfig":                         RoleAdmin,
			"/evey.v1.AdminService/UpdatePluginConfig":                      RoleAdmin,
			"/evey.v1.AdminService/GetPluginLogs":                           RoleAdmin,
			"/evey.v1.WebhookService/CreateWebhook":                         RoleAdmin,
			"/evey.v1.WebhookService/ListWebhooks":                          RoleAdmin,
			"/evey.v1.WebhookService/UpdateWebhook":                         RoleAdmin,
			"/evey.v1.WebhookService/DeleteWebhook":                         RoleAdmin,
			"/evey.v1.WebhookService/ListWebhookDeliveries":                 RoleAdmin,
			"/evey.v1.UserService/CreateUser":                               RoleAdmin,
			"/evey.v1.UserService/DeleteUser":                               RoleAdmin,
			"/evey.v1.UserService/DisableUser":                              RoleAdmin,
			"/evey.v1.UserService/ListUsers":                                RoleAdmin,
			"/evey.v1.LibraryService/CreateLibrary":                         RoleAdmin,
			"/evey.v1.LibraryService/DeleteLibrary":                         RoleAdmin,
			"/evey.v1.LibraryService/ScanLibrary":                           RoleAdmin,
			"/evey.v1.FeedService/GetAdminFeedConfigs":                      RoleAdmin,
			"/evey.v1.FeedService/UpdateAdminFeedConfig":                    RoleAdmin,
			"/evey.v1.NotificationService/GetGlobalNotificationSettings":    RoleAdmin,
			"/evey.v1.NotificationService/UpdateGlobalNotificationSettings": RoleAdmin,
			"/evey.v1.ActivityService/GetServerStats":                       RoleAdmin,
			"/evey.v1.ActivityService/ListServerMetrics":                    RoleAdmin,
			"/evey.v1.MetadataService/RefreshMetadata":                      RoleAdmin,
			"/evey.v1.MetadataService/SetMetadataOverride":                  RoleAdmin,
			"/evey.v1.MetadataService/RemoveMetadataOverride":               RoleAdmin,
			"/evey.v1.MKVService/MuxContainer":                              RoleAdmin,
			"/evey.v1.MKVService/DemuxContainer":                            RoleAdmin,
			"/evey.v1.MKVService/AddTrack":                                  RoleAdmin,
			"/evey.v1.MKVService/RemoveTrack":                               RoleAdmin,
			"/evey.v1.TranscodingService/StopTranscode":                     RoleAdmin,
			"/evey.v1.TranscodingService/ListActiveTranscodes":              RoleAdmin,
			"/evey.v1.TranscodeCacheService/GetTranscodeCache":              RoleAdmin,
			"/evey.v1.TranscodeCacheService/ClearTranscodeCache":            RoleAdmin,
			"/evey.v1.TranscodeCacheService/DeleteTranscodeCacheFile":       RoleAdmin,
			"/evey.v1.BackupService/CreateBackup":                           RoleAdmin,
			"/evey.v1.BackupService/ListBackups":                            RoleAdmin,
			"/evey.v1.BackupService/DeleteBackup":                           RoleAdmin,
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
		},
	}
}
