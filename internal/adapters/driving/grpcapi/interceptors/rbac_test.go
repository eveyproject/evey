package interceptors_test

import (
	"strings"
	"testing"

	_ "github.com/evey-project/evey/gen/evey/v1" // register proto service descriptors
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func TestDefaultRBACConfig_PublicMethods(t *testing.T) {
	cfg := interceptors.DefaultRBACConfig()

	expected := []string{
		"/evey.v1.UserService/Login",
		"/evey.v1.UserService/RefreshToken",
		"/evey.v1.UserService/RegisterWithInvite",
		"/evey.v1.UserService/ValidateInvite",
		"/evey.v1.AdminSystemService/SetupWizard",
		"/evey.v1.AdminSystemService/GetSetupStatus",
		"/evey.v1.UserService/InitiateQuickConnect",
		"/evey.v1.UserService/CheckQuickConnect",
		"/evey.v1.UserService/RequestPasswordReset",
		"/evey.v1.UserService/ResetPassword",
	}

	require.Len(t, cfg.PublicMethods, len(expected), "public methods count mismatch - was a method added or removed?")

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
		"/evey.v1.AdminSystemService/RestartServer",
		"/evey.v1.AdminSystemService/GetServerConfig",
		"/evey.v1.AdminSystemService/UpdateServerConfig",
		"/evey.v1.AdminSystemService/GetDashboard",
		"/evey.v1.AdminSystemService/StartLoadTest",
		"/evey.v1.AdminSystemService/GetLoadTestStatus",
		"/evey.v1.AdminSystemService/CancelLoadTest",
		// PresenceService
		"/evey.v1.PresenceService/ListOnline",
		// AdminTaskService
		"/evey.v1.AdminTaskService/ListScheduledTasks",
		"/evey.v1.AdminTaskService/UpdateScheduledTask",
		"/evey.v1.AdminTaskService/RunScheduledTask",
		// AdminSessionService
		"/evey.v1.AdminSessionService/GetAuditLog",
		"/evey.v1.AdminSessionService/ListActiveSessions",
		"/evey.v1.AdminSessionService/KillSession",
		"/evey.v1.AdminSessionService/SendSessionCommand",
		"/evey.v1.AdminSessionService/SendCastReceiverCommand",
		// AdminPluginService
		"/evey.v1.AdminPluginService/ListPluginRuntimes",
		"/evey.v1.AdminPluginService/AddPluginRuntime",
		"/evey.v1.AdminPluginService/RemovePluginRuntime",
		"/evey.v1.AdminPluginService/InstallPlugin",
		"/evey.v1.AdminPluginService/UninstallPlugin",
		"/evey.v1.AdminPluginService/PreviewPluginUpdate",
		"/evey.v1.AdminPluginService/ApplyPluginUpdate",
		"/evey.v1.AdminPluginService/EnablePlugin",
		"/evey.v1.AdminPluginService/DisablePlugin",
		"/evey.v1.AdminPluginService/ListPlugins",
		"/evey.v1.AdminPluginService/GetPluginConfig",
		"/evey.v1.AdminPluginService/UpdatePluginConfig",
		"/evey.v1.AdminPluginService/GetPluginLogs",
		"/evey.v1.AdminPluginService/GetPluginResourceLimits",
		"/evey.v1.AdminPluginService/UpdatePluginResourceLimits",
		// AdminMediaService
		"/evey.v1.AdminMediaService/RefreshAllMetadata",
		"/evey.v1.AdminMediaService/RelinkCollections",
		"/evey.v1.AdminMediaService/MatchOrphansInLibrary",
		"/evey.v1.AdminMediaService/GenerateAllTrickplays",
		"/evey.v1.AdminMediaService/DeleteMediaItem",
		"/evey.v1.AdminMediaService/ScanSeekIndex",
		"/evey.v1.AdminMediaService/StartSeekIndexScan",
		"/evey.v1.AdminMediaService/GetSeekIndexJob",
		"/evey.v1.AdminMediaService/CancelSeekIndexJob",
		"/evey.v1.AdminMediaService/ReindexMediaSeekIndex",
		"/evey.v1.AdminMediaService/RollbackMediaSeekIndex",
		"/evey.v1.AdminMediaService/ReindexLibrarySeekIndex",
		"/evey.v1.AdminMediaService/RollbackLibrarySeekIndex",
		"/evey.v1.AdminMediaService/DeleteSeekIndexBackup",
		"/evey.v1.AdminMediaService/AckSeekIndexMedia",
		"/evey.v1.AdminMediaService/GetMediaRepairHistory",
		"/evey.v1.AdminMediaService/ApplyAudioShift",
		"/evey.v1.AdminMediaService/UndoMediaRepair",
		"/evey.v1.AdminMediaService/ListMediaRepairs",
		"/evey.v1.AdminMediaService/AckSeekIndexGroup",
		"/evey.v1.AdminMediaService/FixContainerMedia",
		"/evey.v1.AdminMediaService/ReinterleaveMedia",
		"/evey.v1.AdminMediaService/PurgeSeekIndexBackups",
		"/evey.v1.AdminMediaService/RemuxAudioMedia",
		"/evey.v1.AdminMediaService/RemuxAudioLibrary",
		// AdminIntroService
		"/evey.v1.AdminIntroService/BackfillIntroDetection",
		"/evey.v1.AdminIntroService/InvalidateIntroCache",
		// WebhookService
		"/evey.v1.WebhookService/CreateWebhook",
		"/evey.v1.WebhookService/ListWebhooks",
		"/evey.v1.WebhookService/UpdateWebhook",
		"/evey.v1.WebhookService/DeleteWebhook",
		"/evey.v1.WebhookService/ListWebhookDeliveries",
		// UserService (admin-only)
		"/evey.v1.UserService/CreateUser",
		"/evey.v1.UserService/ImpersonateUser",
		"/evey.v1.UserService/DeleteUser",
		"/evey.v1.UserService/DisableUser",
		"/evey.v1.UserService/ListUsers",
		"/evey.v1.UserService/ListAllProfiles",
		"/evey.v1.UserService/SetDefaultProfile",
		"/evey.v1.UserService/CreateAPIKey",
		"/evey.v1.UserService/ListAPIKeys",
		"/evey.v1.UserService/RevokeAPIKey",
		"/evey.v1.UserService/CreateInviteLink",
		// LibraryService (admin-only)
		"/evey.v1.LibraryService/CreateLibrary",
		"/evey.v1.LibraryService/UpdateLibrary",
		"/evey.v1.LibraryService/DeleteLibrary",
		"/evey.v1.LibraryService/ScanLibrary",
		// FeedService
		"/evey.v1.FeedService/GetAdminFeedConfigs",
		"/evey.v1.FeedService/UpdateAdminFeedConfig",
		"/evey.v1.FeedService/BatchUpdateAdminFeedConfigs",
		// NotificationService
		"/evey.v1.NotificationService/GetGlobalNotificationSettings",
		"/evey.v1.NotificationService/UpdateGlobalNotificationSettings",
		"/evey.v1.NotificationService/ListAdminNotifications",
		// ActivityService
		"/evey.v1.ActivityService/GetServerStats",
		"/evey.v1.ActivityService/ListServerMetrics",
		// MetadataService is gated per-item via checkMediaItemAccess in the
		// handler, not at the RBAC layer.
		// MKVService
		"/evey.v1.MKVService/GetContainerInfo",
		"/evey.v1.MKVService/ListTracks",
		"/evey.v1.MKVService/ExtractTrack",
		"/evey.v1.MKVService/MuxContainer",
		"/evey.v1.MKVService/DemuxContainer",
		"/evey.v1.MKVService/AddTrack",
		"/evey.v1.MKVService/RemoveTrack",
		"/evey.v1.MKVService/ListAttachments",
		"/evey.v1.MKVService/ExtractAttachment",
		"/evey.v1.ShareService/CreateShareLink",
		"/evey.v1.ShareService/ListShareLinks",
		"/evey.v1.ShareService/RevokeShareLink",
		// TranscodingService
		"/evey.v1.TranscodingService/StopTranscode",
		"/evey.v1.TranscodingService/ListActiveTranscodes",
		"/evey.v1.TranscodingService/DetectHardware",
		"/evey.v1.TranscodingService/CreateTranscodeProfile",
		"/evey.v1.TranscodingService/UpdateTranscodeProfile",
		"/evey.v1.TranscodingService/DeleteTranscodeProfile",
		// TranscodeCacheService
		"/evey.v1.TranscodeCacheService/GetTranscodeCache",
		"/evey.v1.TranscodeCacheService/ClearTranscodeCache",
		"/evey.v1.TranscodeCacheService/DeleteTranscodeCacheFile",
		// BackupService
		"/evey.v1.BackupService/CreateBackup",
		"/evey.v1.BackupService/ListBackups",
		"/evey.v1.BackupService/DeleteBackup",
		// BackupScheduleService
		"/evey.v1.BackupScheduleService/GetBackupSchedule",
		"/evey.v1.BackupScheduleService/UpdateBackupSchedule",
		"/evey.v1.BackupScheduleService/RunBackupNow",
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
		"/evey.v1.CollectionService/CreateCollection",
		"/evey.v1.CollectionService/UpdateCollection",
		"/evey.v1.CollectionService/AddCollectionItem",
		"/evey.v1.CollectionService/RemoveCollectionItem",
		"/evey.v1.CollectionService/ReorderCollectionItems",
		// PhotoService
		"/evey.v1.PhotoService/DeletePhoto",
		// MusicService (destructive / admin-only)
		"/evey.v1.MusicService/DeleteTrack",
		"/evey.v1.MusicService/ScanMusicDirectory",
		// AdminMediaRequestService (operator moderation)
		"/evey.v1.AdminMediaRequestService/ListRequests",
		"/evey.v1.AdminMediaRequestService/AnswerRequest",
		"/evey.v1.AdminMediaRequestService/GetAccessSettings",
		"/evey.v1.AdminMediaRequestService/SetAccessDefault",
		"/evey.v1.AdminMediaRequestService/SetUserAccess",
	}

	require.Len(t, cfg.MethodRoles, len(expectedAdmin), "MethodRoles count mismatch - was a mapping added or removed?")

	for _, method := range expectedAdmin {
		t.Run(method, func(t *testing.T) {
			role, exists := cfg.MethodRoles[method]
			require.True(t, exists, "method should be in MethodRoles")
			assert.Equal(t, interceptors.RoleAdmin, role, "should require admin role")
		})
	}
}

// TestAllAdminServiceMethodsAreGated walks the real gRPC service descriptors
// (not a hand-maintained list) and asserts every method of every Admin*-named
// service is either gated RoleAdmin or explicitly allowlisted as public. This is
// the guard the hardcoded expectedAdmin list lacked: it fails the build when a
// new admin RPC ships without a MethodRoles entry - exactly the gap that left
// ApplyPluginUpdate/PreviewPluginUpdate callable by any authenticated user.
func TestAllAdminServiceMethodsAreGated(t *testing.T) {
	cfg := interceptors.DefaultRBACConfig()

	// Methods under an Admin*-named service that are deliberately NOT admin-gated.
	// The default expectation is RoleAdmin; every exception needs a justification
	// here so a genuinely-missing gate (the ApplyPluginUpdate gap) can't hide.
	allowedNonAdmin := map[string]string{
		// Setup bootstrap, reachable before any admin account exists (public).
		"/evey.v1.AdminSystemService/SetupWizard":    "public setup bootstrap",
		"/evey.v1.AdminSystemService/GetSetupStatus": "public setup status",
		// Any authenticated user (iframe plugin consumer) mints plugin tokens;
		// FilterPluginScopes strips admin/system scopes at the service layer and
		// plugin tokens are floor-denied from every RoleAdmin method.
		"/evey.v1.AdminPluginService/IssuePluginToken": "RoleUser by design; scopes filtered",
	}

	var checked int
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		svcs := fd.Services()
		for i := range svcs.Len() {
			svc := svcs.Get(i)
			if !strings.HasPrefix(string(svc.Name()), "Admin") {
				continue
			}
			methods := svc.Methods()
			for j := range methods.Len() {
				full := "/" + string(svc.FullName()) + "/" + string(methods.Get(j).Name())
				checked++
				if _, ok := allowedNonAdmin[full]; ok {
					continue
				}
				// t.Errorf (not require) so every offending method is reported in
				// one run, not just the first.
				role, ok := cfg.MethodRoles[full]
				if !ok {
					t.Errorf("admin RPC %s has no RBAC entry - any authenticated user can call it; add it to MethodRoles (or to allowedNonAdmin with a justification)", full)
					continue
				}
				assert.Equalf(t, interceptors.RoleAdmin, role,
					"admin RPC %s must require RoleAdmin", full)
			}
		}
		return true
	})

	require.Positive(t, checked, "no Admin* service methods discovered - proto descriptors not registered?")
}

// notGRPCRegisteredServices lists evey.v1 services that exist in the proto
// descriptors but are deliberately NOT registered on the gRPC server (served by
// hand-written HTTP handlers instead). They are skipped by the classification
// guard below because the gRPC RBAC interceptor never sees their methods.
// ⚠️ If you register one of these on the gRPC server, REMOVE it here and classify
// every one of its methods - e.g. ParentalService/UpdateParentalSettings must be
// gated, not left to default RoleUser.
var notGRPCRegisteredServices = map[string]bool{
	"ParentalService": true,
}

// allowedUserCallable is the explicit allowlist of evey.v1 RPCs that resolve to
// the default RoleUser at the RBAC layer (any authenticated user). Every entry
// carries the reason it is safe at that layer - almost always "finer access is
// enforced inside the handler" (own-only, per-item checkMediaItemAccess /
// checkLibraryAccess, or an explicit admin/owner check). This is the guard the
// Admin*-prefix test (TestAllAdminServiceMethodsAreGated) could not provide:
// a NEW privileged RPC added to a non-Admin service (LibraryService, UserService,
// WebhookService, …) would otherwise silently default to RoleUser with no red
// test. With this allowlist, any unclassified method fails TestAllRegisteredMethodsAreClassified,
// forcing the author to decide RoleAdmin (MethodRoles) vs RoleUser (here, justified).
var allowedUserCallable = map[string]string{
	// ActivityService
	"/evey.v1.ActivityService/GetPlayHistory":  "own / lib-access-filtered read",
	"/evey.v1.ActivityService/GetUserStats":    "own / lib-access-filtered read",
	"/evey.v1.ActivityService/GetLoginHistory": "own history; admin-can-view-other enforced in handler",
	"/evey.v1.ActivityService/GetPopularMedia": "lib-access-filtered read",
	"/evey.v1.ActivityService/GetAnalytics":    "admin enforced in handler (PermissionDenied for non-admin)",
	"/evey.v1.ActivityService/RecordPageView":  "own navigation logged as audit page_view; writes only the caller's own path",
	// AdminPluginService
	"/evey.v1.AdminPluginService/IssuePluginToken": "RoleUser by design; FilterPluginScopes strips admin scopes, tokens floor-denied from admin methods",
	// PresenceService
	"/evey.v1.PresenceService/Heartbeat": "any authed user announces their own presence; records only the caller's own claims, no read of others",
	// CollectionService
	"/evey.v1.CollectionService/ListCollections":             "read",
	"/evey.v1.CollectionService/GetCollection":               "read",
	"/evey.v1.CollectionService/GetCollectionItems":          "lib-access-filtered read",
	"/evey.v1.CollectionService/ListCollectionsForMediaItem": "lib-access-filtered read",
	// FavoriteService
	"/evey.v1.FavoriteService/ListFavorites":  "own; parental-filtered",
	"/evey.v1.FavoriteService/AddFavorite":    "own; media-access checked",
	"/evey.v1.FavoriteService/RemoveFavorite": "own",
	"/evey.v1.FavoriteService/IsFavorite":     "own",
	// Batch form of IsFavorite: same scope, and every id goes through the
	// library-access + parental filter before it is answered, so it cannot be
	// used to probe media the caller has no access to.
	"/evey.v1.FavoriteService/BatchIsFavorite": "own; media-access checked; parental-filtered",
	// MediaRequestService (requester-facing; scoped to the caller's own profile/user from JWT claims)
	"/evey.v1.MediaRequestService/SearchTitles":     "read-only metadata provider search; no owned data",
	"/evey.v1.MediaRequestService/CreateRequest":    "own; profile/user from claims, policy-gated (ErrDisabled)",
	"/evey.v1.MediaRequestService/ListMyRequests":   "own; profile from claims, never from request",
	"/evey.v1.MediaRequestService/GetRequestPolicy": "own; reports whether the caller may request",
	// FeedService
	"/evey.v1.FeedService/GetFeedConfig":           "read",
	"/evey.v1.FeedService/ContinueWatching":        "own; lib-access-filtered",
	"/evey.v1.FeedService/RecentlyAdded":           "lib-access-filtered",
	"/evey.v1.FeedService/Recommended":             "lib-access-filtered",
	"/evey.v1.FeedService/SimilarItems":            "lib-access-filtered",
	"/evey.v1.FeedService/UpdateProfileFeedConfig": "own profile",
	"/evey.v1.FeedService/ResetProfileFeedConfig":  "own profile",
	"/evey.v1.FeedService/ResumePlayback":          "own; parental-filtered",
	"/evey.v1.FeedService/NextEpisode":             "own; lib-access-filtered",
	// LibraryService
	"/evey.v1.LibraryService/GetLibrary":            "read",
	"/evey.v1.LibraryService/ListLibraries":         "lib-access-filtered",
	"/evey.v1.LibraryService/GetMediaItem":          "media-access checked",
	"/evey.v1.LibraryService/ListMediaItems":        "lib-access + parental filtered",
	"/evey.v1.LibraryService/GetPublicMediaItem":    "public endpoint (parental-filtered)",
	"/evey.v1.LibraryService/ListPublicMediaItems":  "public endpoint (parental-filtered)",
	"/evey.v1.LibraryService/SearchMedia":           "lib-access + parental filtered",
	"/evey.v1.LibraryService/GetPerson":             "read",
	"/evey.v1.LibraryService/GetLibrarySchedule":    "read",
	"/evey.v1.LibraryService/SetLibrarySchedule":    "admin enforced in handler (PermissionDenied)",
	"/evey.v1.LibraryService/DeleteLibrarySchedule": "admin enforced in handler (PermissionDenied)",
	"/evey.v1.LibraryService/FilterMediaItems":      "lib-access + parental filtered",
	"/evey.v1.LibraryService/ListGenres":            "read",
	"/evey.v1.LibraryService/GetSeriesAggregate":    "media-access + parental checked",
	// MetadataService - not RBAC-gated by design; per-item access via checkMediaItemAccess
	"/evey.v1.MetadataService/SearchMetadata":         "per-item access in handler",
	"/evey.v1.MetadataService/RefreshMetadata":        "per-item access in handler",
	"/evey.v1.MetadataService/GetArtwork":             "per-item access in handler",
	"/evey.v1.MetadataService/SetMetadataOverride":    "per-item access in handler",
	"/evey.v1.MetadataService/RemoveMetadataOverride": "per-item access in handler",
	"/evey.v1.MetadataService/ListMetadataOverrides":  "per-item access in handler",
	"/evey.v1.MetadataService/GetMetadataProviders":   "read",
	"/evey.v1.MetadataService/UpdateMetadata":         "per-item access in handler",
	"/evey.v1.MetadataService/DeleteMetadata":         "per-item access in handler",
	"/evey.v1.MetadataService/ResearchMetadata":       "per-item access in handler",
	// MKVService
	"/evey.v1.MKVService/ListChapters": "player skip-intro fallback; per-path access in handler (checkPathAccess)",
	// MusicService
	"/evey.v1.MusicService/ListTracks":  "lib-access checked",
	"/evey.v1.MusicService/GetTrack":    "lib-access checked (per-track library)",
	"/evey.v1.MusicService/GetAlbums":   "lib-access checked",
	"/evey.v1.MusicService/GetArtists":  "lib-access checked",
	"/evey.v1.MusicService/SearchMusic": "lib-access checked",
	// NotificationService
	"/evey.v1.NotificationService/ListNotifications":          "own",
	"/evey.v1.NotificationService/MarkRead":                   "own",
	"/evey.v1.NotificationService/MarkAllRead":                "own",
	"/evey.v1.NotificationService/GetUnreadCount":             "own",
	"/evey.v1.NotificationService/DeleteAllNotifications":     "own",
	"/evey.v1.NotificationService/GetNotificationSettings":    "own",
	"/evey.v1.NotificationService/UpdateNotificationSettings": "own",
	// PhotoService
	"/evey.v1.PhotoService/ListPhotos":         "lib-access checked",
	"/evey.v1.PhotoService/GetPhoto":           "lib-access checked",
	"/evey.v1.PhotoService/ListPhotoAlbums":    "lib-access checked",
	"/evey.v1.PhotoService/SearchPhotos":       "lib-access checked",
	"/evey.v1.PhotoService/ScanPhotoDirectory": "admin enforced in handler (PermissionDenied)",
	// PlaybackService
	"/evey.v1.PlaybackService/GetStreamInfo":     "media-access checked",
	"/evey.v1.PlaybackService/StartStream":       "media-access + caps checked",
	"/evey.v1.PlaybackService/StopStream":        "own session",
	"/evey.v1.PlaybackService/ReportProgress":    "own",
	"/evey.v1.PlaybackService/GetWatchProgress":  "own",
	"/evey.v1.PlaybackService/GetWatchHistory":   "own",
	"/evey.v1.PlaybackService/GetResumeItems":    "own; parental-filtered",
	"/evey.v1.PlaybackService/GetNextUp":         "own; parental-filtered",
	"/evey.v1.PlaybackService/MarkAsWatched":     "own",
	"/evey.v1.PlaybackService/MarkAsUnwatched":   "own",
	"/evey.v1.PlaybackService/ClearWatchHistory": "own",
	// PlaybackSettingsService
	"/evey.v1.PlaybackSettingsService/GetPlaybackSettings": "read",
	// PlaylistService - all own-scoped in handler
	"/evey.v1.PlaylistService/ListPlaylists":      "own",
	"/evey.v1.PlaylistService/GetPlaylist":        "own",
	"/evey.v1.PlaylistService/CreatePlaylist":     "own",
	"/evey.v1.PlaylistService/UpdatePlaylist":     "own",
	"/evey.v1.PlaylistService/DeletePlaylist":     "own",
	"/evey.v1.PlaylistService/GetPlaylistItems":   "own",
	"/evey.v1.PlaylistService/AddPlaylistItem":    "own",
	"/evey.v1.PlaylistService/RemovePlaylistItem": "own",
	// PluginUserDataService - (plugin,user)-scoped by token
	"/evey.v1.PluginUserDataService/GetPref":    "own (plugin,user)-scoped",
	"/evey.v1.PluginUserDataService/SetPref":    "own (plugin,user)-scoped",
	"/evey.v1.PluginUserDataService/DeletePref": "own (plugin,user)-scoped",
	"/evey.v1.PluginUserDataService/ListPrefs":  "own (plugin,user)-scoped",
	// PushService - own device/subscription scoped
	"/evey.v1.PushService/ListDevices":               "own",
	"/evey.v1.PushService/GetVAPIDKey":               "public VAPID key read",
	"/evey.v1.PushService/RegisterWebPush":           "own",
	"/evey.v1.PushService/UnregisterWebPush":         "own",
	"/evey.v1.PushService/ListWebPushSubscriptions":  "own",
	"/evey.v1.PushService/DeleteWebPushSubscription": "own",
	"/evey.v1.PushService/GetPushConsent":            "own",
	"/evey.v1.PushService/UpdatePushConsent":         "own",
	// RatingService
	"/evey.v1.RatingService/PutThumbRating":     "own",
	"/evey.v1.RatingService/GetMyThumbRating":   "own",
	"/evey.v1.RatingService/GetRatingAggregate": "read aggregate",
	// ShareService
	"/evey.v1.ShareService/ValidateShareLink": "harmless metadata; public download itself guarded by uuid token",
	// SyncPlayService - room membership scoped in handler
	"/evey.v1.SyncPlayService/CreateRoom": "own room",
	"/evey.v1.SyncPlayService/JoinByCode": "own",
	"/evey.v1.SyncPlayService/GetRoom":    "member",
	"/evey.v1.SyncPlayService/JoinRoom":   "own",
	"/evey.v1.SyncPlayService/LeaveRoom":  "own",
	// TranscodingService
	"/evey.v1.TranscodingService/StartTranscode":        "media-access (IDOR) + per-profile bitrate cap in handler",
	"/evey.v1.TranscodingService/GetTranscodeStatus":    "own job (checkJobAccess)",
	"/evey.v1.TranscodingService/ListTranscodeProfiles": "read-only profile list",
	// UserService
	"/evey.v1.UserService/Logout":                "own session",
	"/evey.v1.UserService/GetCurrentUser":        "own",
	"/evey.v1.UserService/UpdateUser":            "self-only + no admin/caps escalation enforced in handler",
	"/evey.v1.UserService/CreateProfile":         "own account",
	"/evey.v1.UserService/UpdateProfile":         "own profiles",
	"/evey.v1.UserService/DeleteProfile":         "own profiles",
	"/evey.v1.UserService/ListProfiles":          "own",
	"/evey.v1.UserService/ListInviteLinks":       "own (creator-scoped)",
	"/evey.v1.UserService/RevokeInviteLink":      "owner-or-admin enforced in handler",
	"/evey.v1.UserService/AuthorizeQuickConnect": "authenticated user approves their own device",
}

// TestAllRegisteredMethodsAreClassified walks EVERY evey.v1 service method from
// the proto descriptors and asserts each one is classified into exactly one of:
// PublicMethods, MethodRoles (admin), or allowedUserCallable (RoleUser, justified).
// This generalises TestAllAdminServiceMethodsAreGated to non-Admin services: it
// fails the build when ANY new RPC (on any service) ships unclassified - closing
// the gap where a privileged RPC on, say, LibraryService would silently default
// to RoleUser. Services served only over HTTP (notGRPCRegisteredServices) are
// skipped because the gRPC interceptor never governs them.
func TestAllRegisteredMethodsAreClassified(t *testing.T) {
	cfg := interceptors.DefaultRBACConfig()

	realMethods := make(map[string]bool)
	var checked int
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		svcs := fd.Services()
		for i := range svcs.Len() {
			svc := svcs.Get(i)
			if !strings.HasPrefix(string(svc.FullName()), "evey.v1.") {
				continue // ignore google.* / grpc.* well-known/infra services
			}
			if notGRPCRegisteredServices[string(svc.Name())] {
				continue
			}
			methods := svc.Methods()
			for j := range methods.Len() {
				full := "/" + string(svc.FullName()) + "/" + string(methods.Get(j).Name())
				realMethods[full] = true
				checked++

				inPublic := cfg.PublicMethods[full]
				_, inRoles := cfg.MethodRoles[full]
				_, inUser := allowedUserCallable[full]

				count := 0
				for _, b := range []bool{inPublic, inRoles, inUser} {
					if b {
						count++
					}
				}
				switch count {
				case 0:
					t.Errorf("RPC %s is unclassified - add it to MethodRoles (RoleAdmin) "+
						"or allowedUserCallable (RoleUser, with justification); a new privileged "+
						"RPC must not silently default to RoleUser", full)
				case 1:
					// exactly one classification - correct
				default:
					t.Errorf("RPC %s is classified in more than one of "+
						"{PublicMethods, MethodRoles, allowedUserCallable}", full)
				}
			}
		}
		return true
	})

	require.Positive(t, checked, "no evey.v1 service methods discovered - proto descriptors not registered?")

	// Reverse check: no stale allowedUserCallable entry for a method that no
	// longer exists (catches typos and renamed/removed RPCs).
	for method := range allowedUserCallable {
		assert.Truef(t, realMethods[method],
			"allowedUserCallable lists %s but no such registered evey.v1 method exists", method)
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
		// MetadataService is intentionally not RBAC-gated at the
		// interceptor layer; per-item access is enforced inside the
		// handler.
		"MKVService",
		"TranscodingService",
		"TranscodeCacheService",
		"BackupService",
		"BackupScheduleService",
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
