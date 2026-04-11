package interceptors_test

import (
	"testing"

	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRBACConfig_PublicMethods(t *testing.T) {
	cfg := interceptors.DefaultRBACConfig()

	expected := []string{
		"/evey.v1.UserService/Login",
		"/evey.v1.UserService/RefreshToken",
		"/evey.v1.UserService/RegisterWithInvite",
		"/evey.v1.AdminSystemService/SetupWizard",
		"/evey.v1.AdminSystemService/GetSetupStatus",
		"/evey.v1.UserService/InitiateQuickConnect",
		"/evey.v1.UserService/CheckQuickConnect",
	}

	require.Len(t, cfg.PublicMethods, len(expected), "public methods count mismatch — was a method added or removed?")

	for _, method := range expected {
		t.Run(method, func(t *testing.T) {
			assert.True(t, cfg.PublicMethods[method], "should be public")
		})
	}
}

func TestDefaultRBACConfig_PublicMethodsNotInMethodRoles(t *testing.T) {
	cfg := interceptors.DefaultRBACConfig()

	for method := range cfg.PublicMethods {
		t.Run(method, func(t *testing.T) {
			_, exists := cfg.MethodRoles[method]
			assert.False(t, exists, "public method should not also appear in MethodRoles")
		})
	}
}

func TestDefaultRBACConfig_AdminMethods(t *testing.T) {
	cfg := interceptors.DefaultRBACConfig()

	expectedAdmin := []string{
		// AdminSystemService
		"/evey.v1.AdminSystemService/GetSystemInfo",
		"/evey.v1.AdminSystemService/GetServerConfig",
		"/evey.v1.AdminSystemService/UpdateServerConfig",
		"/evey.v1.AdminSystemService/GetDashboard",
		// AdminTaskService
		"/evey.v1.AdminTaskService/ListScheduledTasks",
		"/evey.v1.AdminTaskService/UpdateScheduledTask",
		"/evey.v1.AdminTaskService/RunScheduledTask",
		// AdminSessionService
		"/evey.v1.AdminSessionService/GetAuditLog",
		"/evey.v1.AdminSessionService/ListActiveSessions",
		"/evey.v1.AdminSessionService/KillSession",
		// AdminPluginService
		"/evey.v1.AdminPluginService/ListPluginRuntimes",
		"/evey.v1.AdminPluginService/AddPluginRuntime",
		"/evey.v1.AdminPluginService/RemovePluginRuntime",
		"/evey.v1.AdminPluginService/InstallPlugin",
		"/evey.v1.AdminPluginService/UninstallPlugin",
		"/evey.v1.AdminPluginService/EnablePlugin",
		"/evey.v1.AdminPluginService/DisablePlugin",
		"/evey.v1.AdminPluginService/ListPlugins",
		"/evey.v1.AdminPluginService/GetPluginConfig",
		"/evey.v1.AdminPluginService/UpdatePluginConfig",
		"/evey.v1.AdminPluginService/GetPluginLogs",
		// AdminMediaService
		"/evey.v1.AdminMediaService/RefreshAllMetadata",
		"/evey.v1.AdminMediaService/GenerateAllTrickplays",
		"/evey.v1.AdminMediaService/DeleteMediaItem",
		// WebhookService
		"/evey.v1.WebhookService/CreateWebhook",
		"/evey.v1.WebhookService/ListWebhooks",
		"/evey.v1.WebhookService/UpdateWebhook",
		"/evey.v1.WebhookService/DeleteWebhook",
		"/evey.v1.WebhookService/ListWebhookDeliveries",
		// UserService (admin-only)
		"/evey.v1.UserService/CreateUser",
		"/evey.v1.UserService/DeleteUser",
		"/evey.v1.UserService/DisableUser",
		"/evey.v1.UserService/ListUsers",
		"/evey.v1.UserService/CreateAPIKey",
		"/evey.v1.UserService/ListAPIKeys",
		"/evey.v1.UserService/RevokeAPIKey",
		// LibraryService (admin-only)
		"/evey.v1.LibraryService/CreateLibrary",
		"/evey.v1.LibraryService/UpdateLibrary",
		"/evey.v1.LibraryService/DeleteLibrary",
		"/evey.v1.LibraryService/ScanLibrary",
		// FeedService
		"/evey.v1.FeedService/GetAdminFeedConfigs",
		"/evey.v1.FeedService/UpdateAdminFeedConfig",
		// NotificationService
		"/evey.v1.NotificationService/GetGlobalNotificationSettings",
		"/evey.v1.NotificationService/UpdateGlobalNotificationSettings",
		// ActivityService
		"/evey.v1.ActivityService/GetServerStats",
		"/evey.v1.ActivityService/ListServerMetrics",
		// MetadataService
		"/evey.v1.MetadataService/SearchMetadata",
		"/evey.v1.MetadataService/RefreshMetadata",
		"/evey.v1.MetadataService/SetMetadataOverride",
		"/evey.v1.MetadataService/RemoveMetadataOverride",
		// MKVService
		"/evey.v1.MKVService/GetContainerInfo",
		"/evey.v1.MKVService/ListTracks",
		"/evey.v1.MKVService/ExtractTrack",
		"/evey.v1.MKVService/MuxContainer",
		"/evey.v1.MKVService/DemuxContainer",
		"/evey.v1.MKVService/AddTrack",
		"/evey.v1.MKVService/RemoveTrack",
		// TranscodingService
		"/evey.v1.TranscodingService/StopTranscode",
		"/evey.v1.TranscodingService/ListActiveTranscodes",
		// TranscodeCacheService
		"/evey.v1.TranscodeCacheService/GetTranscodeCache",
		"/evey.v1.TranscodeCacheService/ClearTranscodeCache",
		"/evey.v1.TranscodeCacheService/DeleteTranscodeCacheFile",
		// BackupService
		"/evey.v1.BackupService/CreateBackup",
		"/evey.v1.BackupService/ListBackups",
		"/evey.v1.BackupService/DeleteBackup",
		// RemoteAccessService
		"/evey.v1.RemoteAccessService/GetRemoteAccess",
		"/evey.v1.RemoteAccessService/UpdateRemoteAccess",
		"/evey.v1.RemoteAccessService/GetNetworkInfo",
		// PlaybackSettingsService
		"/evey.v1.PlaybackSettingsService/UpdatePlaybackSettings",
		// LibraryAccessService
		"/evey.v1.LibraryAccessService/GetUserLibraryAccess",
		"/evey.v1.LibraryAccessService/SetUserLibraryAccess",
		"/evey.v1.LibraryAccessService/RevokeAllUserLibraryAccess",
		// ServerConfigService
		"/evey.v1.ServerConfigService/GetServerConfigEntry",
		"/evey.v1.ServerConfigService/SetServerConfigEntry",
		"/evey.v1.ServerConfigService/ListServerConfigEntries",
		"/evey.v1.ServerConfigService/DeleteServerConfigEntry",
		// CollectionService
		"/evey.v1.CollectionService/DeleteCollection",
	}

	require.Len(t, cfg.MethodRoles, len(expectedAdmin), "MethodRoles count mismatch — was a mapping added or removed?")

	for _, method := range expectedAdmin {
		t.Run(method, func(t *testing.T) {
			role, exists := cfg.MethodRoles[method]
			require.True(t, exists, "method should be in MethodRoles")
			assert.Equal(t, interceptors.RoleAdmin, role, "should require admin role")
		})
	}
}

func TestDefaultRBACConfig_AllMethodRolesAreAdmin(t *testing.T) {
	cfg := interceptors.DefaultRBACConfig()

	for method, role := range cfg.MethodRoles {
		t.Run(method, func(t *testing.T) {
			assert.Equal(t, interceptors.RoleAdmin, role, "all current MethodRoles should be RoleAdmin")
		})
	}
}

func TestDefaultRBACConfig_ServiceCoverage(t *testing.T) {
	cfg := interceptors.DefaultRBACConfig()

	allMethods := make(map[string]bool)
	for m := range cfg.PublicMethods {
		allMethods[m] = true
	}
	for m := range cfg.MethodRoles {
		allMethods[m] = true
	}

	expectedServices := []string{
		"AdminSystemService",
		"AdminPluginService",
		"AdminTaskService",
		"AdminMediaService",
		"AdminSessionService",
		"UserService",
		"WebhookService",
		"LibraryService",
		"FeedService",
		"NotificationService",
		"ActivityService",
		"MetadataService",
		"MKVService",
		"TranscodingService",
		"TranscodeCacheService",
		"BackupService",
		"RemoteAccessService",
		"PlaybackSettingsService",
		"LibraryAccessService",
		"ServerConfigService",
	}

	for _, svc := range expectedServices {
		t.Run(svc, func(t *testing.T) {
			found := false
			for m := range allMethods {
				if contains(m, svc) {
					found = true
					break
				}
			}
			assert.True(t, found, "service %s should have at least one RBAC entry", svc)
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestDefaultRBACConfig_UnknownMethodAllowsAuthenticatedUser(t *testing.T) {
	cfg := interceptors.DefaultRBACConfig()

	unknownMethod := "/evey.v1.SomeService/UnknownMethod"
	assert.False(t, cfg.PublicMethods[unknownMethod], "unknown method should not be public")
	_, exists := cfg.MethodRoles[unknownMethod]
	assert.False(t, exists, "unknown method should not have a role mapping (defaults to any authenticated user)")
}

func TestDefaultRBACConfig_NoEmptyKeys(t *testing.T) {
	cfg := interceptors.DefaultRBACConfig()

	for method := range cfg.PublicMethods {
		assert.NotEmpty(t, method, "public methods should not have empty key")
	}
	for method := range cfg.MethodRoles {
		assert.NotEmpty(t, method, "method roles should not have empty key")
	}
}

func TestDefaultRBACConfig_NoDuplicatesBetweenPublicAndRoles(t *testing.T) {
	cfg := interceptors.DefaultRBACConfig()

	for method := range cfg.MethodRoles {
		assert.False(t, cfg.PublicMethods[method],
			"method %s is both public and role-restricted", method)
	}
}

func TestRoleConstants(t *testing.T) {
	assert.Equal(t, interceptors.RoleUser, interceptors.Role(0))
	assert.Equal(t, interceptors.RoleAdmin, interceptors.Role(1))
	assert.NotEqual(t, interceptors.RoleUser, interceptors.RoleAdmin)
}
