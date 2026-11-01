// Package demo holds the policy of the public demo instance: which operations
// a visitor - who is an administrator there - may still perform.
//
// The demo runs the production binary with EVEY_DEMO=1. Nothing in a handler
// knows about it: one gRPC interceptor and one HTTP middleware consult this
// package, so the mode cannot drift from the product because it is the
// product. The lists below are written BY HAND, operation by operation, since
// the names lie: PreviewPluginUpdate downloads a caller-supplied URL,
// ScanMusicDirectory walks an arbitrary path, ListShareLinks returns live
// unauthenticated download tokens, GET .../sync-preview transcodes on every
// call. Classifying by verb or by prefix would ship all of those open.
//
// Deny by default: an admin operation that is in neither list is LOCKED. The
// contractguard test next to the RBAC guard refuses an admin RPC or an admin
// / state-changing HTTP route that is not classified here, so the default is a
// safety net for a skipped build, not a place where decisions accumulate.
//
// Every entry carries its reason, the same posture as the RBAC and route-auth
// guards: an entry nobody has to defend is an entry nobody re-reads.
package demo

import (
	"net/http"
	"slices"
	"strings"
)

// ErrorKey is the catalogue key of the one refusal the demo emits. A visitor
// has to be able to tell a demo boundary from a bug, so it is never a bare 403.
const ErrorKey = "errors.demo.locked"

// Credentials of the shared account, published on every login screen.
const (
	Username = "demo"
	Password = "demo"
)

// RedactedFields are the proto field names the RedactInterceptor blanks in
// every response on the demo: a visitor is an administrator there, and the
// sessions page, the presence list and the shared account's login history
// would otherwise show every other visitor's address and browser.
var RedactedFields = []string{"client_ip", "ip_address", "user_agent"}

// MinPasswordLength is the password floor the demo composes the user service
// with (appuser.WithMinPasswordLength): the published password is four
// characters, the production floor is eight, and the setup wizard that seeds
// the account runs against this same floor.
const MinPasswordLength = len(Password)

// OpenAdminRPCs are the admin-only RPCs a demo visitor may call: reads whose
// answer discloses nothing about the host or about other visitors, and the
// impersonation of a seeded account. Anything the hourly reset cannot undo, that reaches the filesystem, the network or another
// visitor's session, or that changes what every other visitor sees (the
// account is shared: collections, playlists, favourites, ratings, watched
// marks, metadata) is not here - the demo is visual.
var OpenAdminRPCs = map[string]string{ //nolint:gosec // G101: RPC and route names with their reasons, no credential ("reset-password" is a path)
	"/evey.v1.AdminSystemService/GetSystemInfo":     "version, uptime and counters; the dashboard's first card",
	"/evey.v1.AdminSystemService/GetLoadTestStatus": "read of a job nobody can start on the demo",
	"/evey.v1.AdminSystemService/GetServerConfig":   "read; secret keys are masked by the config store (IsSecretKey)",
	"/evey.v1.AdminSystemService/GetDashboard":      "read-only dashboard aggregates",
	"/evey.v1.PresenceService/ListOnline":           "who is online - only the shared demo account can be",
	"/evey.v1.AdminTaskService/ListScheduledTasks":  "read of the task table; running or editing one is locked",

	"/evey.v1.AdminPluginService/ListPluginRuntimes":        "read of the runtimes; adding or removing one is locked",
	"/evey.v1.AdminPluginService/ListPlugins":               "the catalogue stays browsable, that is the part worth showing",
	"/evey.v1.AdminPluginService/ListTrustedPublishers":     "read of public keys and their names, nothing about the host",
	"/evey.v1.AdminPluginService/GetPluginConfig":           "read; secret fields are masked by the handler",
	"/evey.v1.AdminPluginService/GetPluginLogs":             "plugin-scoped logs of the seeded plugin, redacted at write time",
	"/evey.v1.AdminPluginService/GetPluginResourceLimits":   "read of the sandbox limits; changing them is locked",
	"/evey.v1.AdminPluginService/GetPluginMetadataSettings": "read of the fields a metadata plugin may replace; changing them is locked",

	"/evey.v1.AdminMediaService/GetSeekIndexJob":       "read of a job's progress",
	"/evey.v1.AdminMediaService/ScanSeekIndex":         "read of the last scan's cached result (CachedScan); the scan itself is locked",
	"/evey.v1.AdminMediaService/GetMediaRepairHistory": "read of past repairs; every repair itself is locked",
	"/evey.v1.AdminMediaService/ListMediaRepairs":      "read of the repair ledger",

	"/evey.v1.WebhookService/ListWebhooks":          "read; the secret is masked (maskSecret) and the sender is a sink on the demo",
	"/evey.v1.WebhookService/ListWebhookDeliveries": "read of the delivery log of a sink",

	"/evey.v1.UserService/ListUsers":       "the seeded accounts; creating, deleting or impersonating one is locked",
	"/evey.v1.UserService/ListAllProfiles": "the seeded profiles",
	"/evey.v1.UserService/ListAPIKeys":     "prefix and metadata only",
	// The immersive view: the seeded accounts and their profiles are there to be
	// visited. An impersonated visitor is still judged by this policy, as a
	// non-administrator on top of it.
	"/evey.v1.UserService/ImpersonateUser": "connect as one of the seeded accounts; the token is bounded by this policy",

	"/evey.v1.FeedService/GetAdminFeedConfigs":                   "read of the home feed configuration",
	"/evey.v1.NotificationService/GetGlobalNotificationSettings": "read of the notification toggles",
	"/evey.v1.NotificationService/ListAdminNotifications":        "read of admin notifications",
	"/evey.v1.ActivityService/GetServerStats":                    "aggregate counters",
	"/evey.v1.ActivityService/ListServerMetrics":                 "the dashboard's CPU/memory/bandwidth history",
	"/evey.v1.AdminMediaRequestService/ListRequests":             "read of the seeded requests",
	"/evey.v1.AdminMediaRequestService/GetAccessSettings":        "read of who may request",

	"/evey.v1.TranscodingService/ListActiveTranscodes":   "the now-playing panel; stopping one is locked",
	"/evey.v1.AdminSessionService/ListActiveSessions":    "the sessions page; client_ip and user_agent are blanked by the RedactInterceptor (RedactedFields)",
	"/evey.v1.TranscodeCacheService/GetTranscodeCache":   "read of the cache usage",
	"/evey.v1.BackupService/ListBackups":                 "read of the backup list; the files themselves are not downloadable on the demo",
	"/evey.v1.BackupScheduleService/GetBackupSchedule":   "read of the schedule",
	"/evey.v1.RemoteAccessService/GetRemoteAccess":       "read of the toggle and the published URL",
	"/evey.v1.LibraryAccessService/GetUserLibraryAccess": "read of the seeded ACL",

	"/evey.v1.ServerConfigService/GetServerConfigEntry":    "read; IsSecretKey masks credentials before they leave the store",
	"/evey.v1.ServerConfigService/ListServerConfigEntries": "read; same masking",
}

// LockedAdminRPCs are the admin-only RPCs the demo refuses, each with the
// reason. They are listed rather than left to the default so the decision is
// written down and the guard can refuse an RPC that is in neither list.
var LockedAdminRPCs = map[string]string{ //nolint:gosec // G101: RPC names with their reasons, no credential ("CreateAPIKey" is a method name)
	"/evey.v1.AdminSystemService/StartLoadTest":      "spawns synthetic viewers against the demo host",
	"/evey.v1.AdminSystemService/CancelLoadTest":     "no job to cancel; symmetric with the start",
	"/evey.v1.AdminSystemService/RestartServer":      "the machine: takes the demo down for everyone",
	"/evey.v1.AdminSystemService/UpdateServerConfig": "writes operator settings a visitor could turn against the next visitor",
	"/evey.v1.AdminTaskService/UpdateScheduledTask":  "reschedules host work",
	"/evey.v1.AdminTaskService/RunScheduledTask":     "runs host work (scans, retention sweeps) on demand",

	"/evey.v1.AdminSessionService/GetAuditLog":             "carries the addresses and user agents of other visitors",
	"/evey.v1.AdminSessionService/KillSession":             "ejects another visitor",
	"/evey.v1.AdminSessionService/SendSessionCommand":      "drives another visitor's player",
	"/evey.v1.AdminSessionService/SendCastReceiverCommand": "same, on a cast receiver",

	"/evey.v1.UserService/CreateInviteLink": "identity: mints a registration credential",
	"/evey.v1.UserService/CreateUser":       "identity",
	"/evey.v1.UserService/DeleteUser":       "identity",
	"/evey.v1.UserService/DisableUser":      "identity: locks the shared account out for everyone",
	"/evey.v1.UserService/EnableUser":       "identity: the counterpart of DisableUser",
	"/evey.v1.UserService/RevokeAPIKey":     "the keys belong to the shared account: a visitor would revoke another visitor's",
	// Locked 2026-10-01 (the API needs no try-out on the demo): a minted key
	// outlived the visitor and the reset, and revoking it is locked above. The
	// boot and every reset purge the keys minted before (PurgeCredentials).
	"/evey.v1.UserService/CreateAPIKey": "a credential that outlives the visitor, which no visitor can revoke",

	// Shared content: a collection one visitor edits or deletes is gone for
	// the 150 others until the restore - the demo shows them, it does not
	// let them be changed.
	"/evey.v1.CollectionService/CreateCollection":       "shared content",
	"/evey.v1.CollectionService/UpdateCollection":       "shared content",
	"/evey.v1.CollectionService/DeleteCollection":       "removes a collection for everyone",
	"/evey.v1.CollectionService/AddCollectionItem":      "shared content",
	"/evey.v1.CollectionService/RemoveCollectionItem":   "shared content",
	"/evey.v1.CollectionService/ReorderCollectionItems": "shared content",
	"/evey.v1.UserService/SetDefaultProfile":            "identity write on the shared account",

	"/evey.v1.AdminPluginService/AddPluginRuntime":             "installs a language runtime on the host",
	"/evey.v1.AdminPluginService/RemovePluginRuntime":          "removes one",
	"/evey.v1.AdminPluginService/AddTrustedPublisher":          "decides whose plugin code the shared instance trusts, for every visitor",
	"/evey.v1.AdminPluginService/RemoveTrustedPublisher":       "withdraws that trust for every visitor",
	"/evey.v1.AdminPluginService/InstallPlugin":                "runs third-party code on the host",
	"/evey.v1.AdminPluginService/UninstallPlugin":              "removes the seeded plugin for everyone",
	"/evey.v1.AdminPluginService/PreviewPluginUpdate":          "downloads a caller-supplied URL (SSRF)",
	"/evey.v1.AdminPluginService/ApplyPluginUpdate":            "installs code",
	"/evey.v1.AdminPluginService/EnablePlugin":                 "starts a process on the host",
	"/evey.v1.AdminPluginService/DisablePlugin":                "stops the seeded plugin for everyone",
	"/evey.v1.AdminPluginService/UpdatePluginResourceLimits":   "changes sandbox limits",
	"/evey.v1.AdminPluginService/UpdatePluginMetadataSettings": "changes what every visitor reads on every title",
	"/evey.v1.AdminPluginService/RefreshPluginMetadata":        "one click fans out to a plugin's service for the whole library",
	"/evey.v1.AdminPluginService/UpdatePluginConfig":           "the seeded plugin is shared: a visitor's city would become everyone's",

	"/evey.v1.AdminMediaService/RefreshAllMetadata":       "one click fans out to the metadata providers for the whole library",
	"/evey.v1.AdminMediaService/RelinkCollections":        "library-wide rewrite",
	"/evey.v1.AdminMediaService/MatchOrphansInLibrary":    "library-wide provider calls",
	"/evey.v1.AdminMediaService/GenerateAllTrickplays":    "library-wide ffmpeg",
	"/evey.v1.AdminMediaService/DeleteMediaItem":          "removes a title for everyone",
	"/evey.v1.AdminMediaService/StartSeekIndexScan":       "walks the media files; the demo runs one itself after its seed",
	"/evey.v1.AdminMediaService/CancelSeekIndexJob":       "symmetric with the start",
	"/evey.v1.AdminMediaService/ReindexMediaSeekIndex":    "rewrites a media file's index in place",
	"/evey.v1.AdminMediaService/RollbackMediaSeekIndex":   "in-place write",
	"/evey.v1.AdminMediaService/ReindexLibrarySeekIndex":  "in-place writes, library-wide",
	"/evey.v1.AdminMediaService/RollbackLibrarySeekIndex": "same",
	"/evey.v1.AdminMediaService/DeleteSeekIndexBackup":    "deletes files",
	"/evey.v1.AdminMediaService/AckSeekIndexMedia":        "mutates the repair ledger",
	"/evey.v1.AdminMediaService/AckSeekIndexGroup":        "same",
	"/evey.v1.AdminMediaService/ApplyAudioShift":          "rewrites a media file",
	"/evey.v1.AdminMediaService/UndoMediaRepair":          "rewrites a media file",
	"/evey.v1.AdminMediaService/FixContainerMedia":        "rewrites a media file",
	"/evey.v1.AdminMediaService/ReinterleaveMedia":        "rewrites a media file",
	"/evey.v1.AdminMediaService/PurgeSeekIndexBackups":    "deletes files",
	"/evey.v1.AdminMediaService/RemuxAudioMedia":          "rewrites a media file",
	"/evey.v1.AdminMediaService/RemuxAudioLibrary":        "rewrites every media file",

	"/evey.v1.WebhookService/CreateWebhook": "points the server at a caller-supplied URL",
	"/evey.v1.WebhookService/UpdateWebhook": "re-points a webhook at a caller-supplied URL, same relay",
	"/evey.v1.WebhookService/DeleteWebhook": "removes the seeded example for everyone",

	"/evey.v1.LibraryService/CreateLibrary":    "storage: names a host path",
	"/evey.v1.LibraryService/UpdateLibrary":    "storage: can re-point a library at another host path",
	"/evey.v1.LibraryService/DeleteLibrary":    "storage",
	"/evey.v1.LibraryService/ScanLibrary":      "walks the host filesystem on demand",
	"/evey.v1.MusicService/DeleteTrack":        "removes a track for everyone",
	"/evey.v1.MusicService/ScanMusicDirectory": "scans an arbitrary absolute path",
	"/evey.v1.PhotoService/DeletePhoto":        "removes a photo for everyone",

	"/evey.v1.FeedService/UpdateAdminFeedConfig":                    "rewrites the home page of every visitor",
	"/evey.v1.FeedService/BatchUpdateAdminFeedConfigs":              "same",
	"/evey.v1.NotificationService/UpdateGlobalNotificationSettings": "operator setting",
	"/evey.v1.AdminMediaRequestService/AnswerRequest":               "answers on behalf of the operator",
	"/evey.v1.AdminMediaRequestService/SetAccessDefault":            "operator setting",
	"/evey.v1.AdminMediaRequestService/SetUserAccess":               "operator setting",

	"/evey.v1.MKVService/GetContainerInfo":  "takes a host path",
	"/evey.v1.MKVService/ListTracks":        "takes a host path",
	"/evey.v1.MKVService/ExtractTrack":      "writes a file",
	"/evey.v1.MKVService/MuxContainer":      "writes a file",
	"/evey.v1.MKVService/DemuxContainer":    "writes files",
	"/evey.v1.MKVService/AddTrack":          "rewrites a media file",
	"/evey.v1.MKVService/RemoveTrack":       "rewrites a media file",
	"/evey.v1.MKVService/ListAttachments":   "reads arbitrary filesystem paths",
	"/evey.v1.MKVService/ExtractAttachment": "arbitrary file write",

	"/evey.v1.ShareService/CreateShareLink": "mints an unauthenticated download URL that outlives the visitor",
	"/evey.v1.ShareService/ListShareLinks":  "returns the live tokens, each an unauthenticated download URL",
	"/evey.v1.ShareService/RevokeShareLink": "symmetric with creation",

	"/evey.v1.TranscodingService/StopTranscode":               "kills another visitor's playback",
	"/evey.v1.TranscodingService/DetectHardware":              "spawns ffmpeg and enumerates the host's GPUs",
	"/evey.v1.TranscodingService/CreateTranscodeProfile":      "server-wide encode ladder",
	"/evey.v1.TranscodingService/UpdateTranscodeProfile":      "same",
	"/evey.v1.TranscodingService/DeleteTranscodeProfile":      "same",
	"/evey.v1.TranscodeCacheService/ClearTranscodeCache":      "deletes the cache under every playing visitor",
	"/evey.v1.TranscodeCacheService/DeleteTranscodeCacheFile": "same, one file",

	"/evey.v1.BackupService/CreateBackup":                 "writes a full database dump on the host",
	"/evey.v1.BackupService/DeleteBackup":                 "deletes files",
	"/evey.v1.BackupScheduleService/UpdateBackupSchedule": "operator setting",
	"/evey.v1.BackupScheduleService/RunBackupNow":         "writes a dump on demand",

	"/evey.v1.RemoteAccessService/UpdateRemoteAccess":         "network: can lock every off-LAN visitor out, i.e. everyone",
	"/evey.v1.RemoteAccessService/GetNetworkInfo":             "discloses the host's interfaces and addresses",
	"/evey.v1.PlaybackSettingsService/UpdatePlaybackSettings": "operator setting",

	"/evey.v1.LibraryAccessService/SetUserLibraryAccess":       "can hide every library from the shared account",
	"/evey.v1.LibraryAccessService/RevokeAllUserLibraryAccess": "same, all at once",
	"/evey.v1.ServerConfigService/SetServerConfigEntry":        "writes operator settings, including provider keys",
	"/evey.v1.ServerConfigService/DeleteServerConfigEntry":     "same",

	"/evey.v1.AdminIntroService/InvalidateIntroCache":   "drops detection results for everyone",
	"/evey.v1.AdminIntroService/BackfillIntroDetection": "library-wide ffmpeg",
}

// LockedUserRPCs are RPCs every authenticated user may normally call but that
// break the demo for the next visitor: the account is shared, so a write on
// the account itself is a write on everyone.
var LockedUserRPCs = map[string]string{
	"/evey.v1.UserService/UpdateUser":               "changes the shared account's password or e-mail and locks everyone else out",
	"/evey.v1.LibraryService/SetLibrarySchedule":    "admin-in-handler write that schedules host scans",
	"/evey.v1.LibraryService/DeleteLibrarySchedule": "same",
	"/evey.v1.PhotoService/ScanPhotoDirectory":      "admin-in-handler scan of a host path",
	"/evey.v1.MetadataService/DeleteMetadata":       "removes a title's metadata for everyone",
	// The account and its profiles are shared by every visitor at once, so a
	// preference written here is a preference changed under everyone. The
	// clients keep those choices in the browser on the demo instead.
	"/evey.v1.UserService/UpdateProfile":                      "locale, region, theme, avatar, languages of the shared profile",
	"/evey.v1.UserService/CreateProfile":                      "the profile picker is shared",
	"/evey.v1.UserService/DeleteProfile":                      "same",
	"/evey.v1.FeedService/UpdateProfileFeedConfig":            "the home layout of the shared profile",
	"/evey.v1.FeedService/ResetProfileFeedConfig":             "same",
	"/evey.v1.NotificationService/UpdateNotificationSettings": "shared",
	"/evey.v1.NotificationService/MarkAllRead":                "clears everyone's unread list",
	"/evey.v1.NotificationService/DeleteAllNotifications":     "same",
	"/evey.v1.PushService/UpdatePushConsent":                  "shared",
	"/evey.v1.PlaybackService/ClearWatchHistory":              "wipes everyone's continue-watching row",

	// Shared content, same rule as the collections: what one visitor marks,
	// rates, files or edits, every other visitor sees. Watch progress itself
	// stays (playback needs it).
	// MarkAsWatched stays open: the players call it when a title reaches its
	// end, and it writes nothing that ReportProgress at 100 % would not - the
	// watched mark IS watch progress. Only the manual un-mark is locked.
	"/evey.v1.PlaybackService/MarkAsUnwatched":        "rewrites the shared watch history by hand",
	"/evey.v1.FavoriteService/AddFavorite":            "the favourites list is the shared profile's",
	"/evey.v1.FavoriteService/RemoveFavorite":         "same",
	"/evey.v1.RatingService/PutThumbRating":           "shared ratings",
	"/evey.v1.PlaylistService/CreatePlaylist":         "shared playlists",
	"/evey.v1.PlaylistService/UpdatePlaylist":         "same",
	"/evey.v1.PlaylistService/DeletePlaylist":         "removes a playlist for everyone",
	"/evey.v1.PlaylistService/AddPlaylistItem":        "shared playlists",
	"/evey.v1.PlaylistService/RemovePlaylistItem":     "same",
	"/evey.v1.MediaRequestService/CreateRequest":      "files a request in the operator's queue, for everyone to see",
	"/evey.v1.MetadataService/UpdateMetadata":         "rewrites a title's metadata for everyone",
	"/evey.v1.MetadataService/SetMetadataOverride":    "same",
	"/evey.v1.MetadataService/RemoveMetadataOverride": "same",
	"/evey.v1.MetadataService/ResearchMetadata":       "re-matches a title for everyone, with provider calls",
	"/evey.v1.MetadataService/RefreshMetadata":        "re-fetches a title from the providers and rewrites it for everyone",
	// Provider searches: every call spends the operator's metadata keys - on a
	// public host, a quota anyone can drain, and the built-in TMDB key is
	// shared by every Evey install. What each search prepares is locked.
	"/evey.v1.MetadataService/SearchMetadata":         "a search across every metadata provider per call; the match it prepares is locked",
	"/evey.v1.MediaRequestService/SearchTitles":       "a TMDB search per call on the operator's key; the request it prepares is locked",
	"/evey.v1.PluginUserDataService/SetPref":          "plugin preferences of the shared account",
	"/evey.v1.PluginUserDataService/DeletePref":       "same",
	"/evey.v1.ParentalService/UpdateParentalSettings": "a PIN set by one visitor locks the others out of the shared profile",
	"/evey.v1.NotificationService/MarkRead":           "clears an entry of everyone's unread list",
	"/evey.v1.PushService/RegisterWebPush":            "subscribes a stranger's browser on the shared account; no push sender is built on the demo anyway",
	"/evey.v1.PushService/UnregisterWebPush":          "same",
	"/evey.v1.PushService/DeleteWebPushSubscription":  "same",
	"/evey.v1.UserService/RevokeInviteLink":           "identity",
}

// RPCLocked reports whether the demo refuses method. isAdminMethod is the RBAC
// classification of the method (MethodRoles == RoleAdmin), so the deny-by-
// default applies exactly to the admin surface.
func RPCLocked(method string, isAdminMethod bool) bool {
	if _, locked := LockedUserRPCs[method]; locked {
		return true
	}
	if !isAdminMethod {
		return false
	}
	_, open := OpenAdminRPCs[method]
	return !open
}

// OpenRoutes are the HTTP mux patterns (as registered, method included) that
// stay reachable on the demo although they are admin-gated or state-changing.
// Method-less patterns serve every verb, so they are classified too.
var OpenRoutes = map[string]string{
	"/":                              "SPA files and the grpc-gateway; the gateway's RPCs are judged by the gRPC interceptor",
	"/healthz":                       "discovery probe, read-only",
	"/api/v1/devices/ws":             "the visitor's own device channel",
	"/api/v1/syncplay/rooms/{id}/ws": "watch-together is the product",

	// Admin reads that disclose nothing about the host or other visitors.
	"GET /api/v1/admin/artworks/download-all/{jobId}":   "progress of a job nobody can start here",
	"GET /api/v1/admin/artworks/stats":                  "counters",
	"GET /api/v1/admin/background-tasks":                "the task table, read",
	"GET /api/v1/admin/config/defaults":                 "what the locale settings fall back to: a language and a time zone",
	"GET /api/v1/admin/backups":                         "the backup list, read; the download route is locked",
	"GET /api/v1/admin/library-stats":                   "counters",
	"GET /api/v1/admin/libraries/prune-suspended":       "the roots a mass disappearance held back, read",
	"GET /api/v1/admin/plugin-runtimes/available":       "the runtime catalogue, read",
	"GET /api/v1/admin/plugins/security-dashboard":      "read of the sandbox report",
	"GET /api/v1/admin/qoe/dashboard":                   "playback quality aggregates",
	"GET /api/v1/admin/remote-access":                   "read of the toggle",
	"GET /api/v1/admin/server-settings":                 "read; every entry is reported locked so the panel greys the forms",
	"GET /api/v1/admin/sso":                             "read of the SSO card; the client secret is masked",
	"GET /api/v1/admin/system/metrics":                  "CPU/memory history the dashboard draws",
	"GET /api/v1/admin/transcode/calibration":           "read of the bench results",
	"GET /api/v1/admin/transcode/calibration/overrides": "read",
	"GET /api/v1/admin/update-status":                   "read of the cached release check",
	"GET /api/v1/admin/users/{id}/bitrate-limit":        "read",

	// DB-only admin writes the hourly restore undoes, that show the product.

	// Non-admin writes: the product. Own data, undone by the restore.
	"POST /api/v1/auth/login":                        "issues the credential",
	"POST /api/v1/auth/logout":                       "ends the visitor's own session",
	"POST /api/v1/auth/refresh":                      "slides the visitor's own session",
	"POST /api/v1/auth/forgot-password":              "the mail sender is a sink on the demo, so this is a no-op with a friendly answer",
	"POST /api/v1/auth/reset-password":               "needs a token no mail ever delivered",
	"POST /api/v1/auth/oidc/app/start":               "pre-auth SSO for the app; refuses while SSO is off",
	"POST /api/v1/auth/oidc/app/token":               "redeems the visitor's own finished SSO flow",
	"POST /api/v1/cast/pause":                        "the visitor's own cast session",
	"POST /api/v1/cast/play":                         "same",
	"POST /api/v1/cast/seek":                         "same",
	"POST /api/v1/cast/stop":                         "same",
	"POST /api/v1/dlna/pause":                        "own DLNA session",
	"POST /api/v1/dlna/play":                         "same",
	"POST /api/v1/dlna/stop":                         "same",
	"POST /api/v1/download-token/{id}":               "offline download of a freely licensed title",
	"POST /api/v1/media/{id}/cast-next":              "playback",
	"POST /api/v1/media/{id}/compat-download":        "offline download, transcoded server-side; the product",
	"POST /api/v1/media/{id}/player-event":           "the visitor's own QoE telemetry",
	"POST /api/v1/media/{id}/prewarm-next":           "playback",
	"POST /api/v1/media/{id}/reduced-download":       "offline download",
	"DELETE /api/v1/media/{id}/reduced-download":     "cancels the visitor's own download job",
	"POST /api/v1/media/{id}/stream":                 "playback",
	"POST /api/v1/media/{id}/stream-token":           "playback",
	"POST /api/v1/music/nowplaying":                  "own now-playing state",
	"POST /api/v1/music/tracks/{id}/played":          "own play count",
	"POST /api/v1/profiles/{id}/parental/verify-pin": "parental control is the product",
	"POST /api/v1/syncplay/join-code":                "watch-together",
}

// LockedRoutes are the HTTP mux patterns the demo refuses, with the reason.
var LockedRoutes = map[string]string{ //nolint:gosec // G101: RPC and route names with their reasons, no credential ("reset-password" is a path)
	"GET /api/v1/admin/fs/browse":               "browses the host filesystem",
	"GET /api/v1/admin/logs":                    "host logs carry other visitors' addresses even after redaction",
	"GET /api/v1/admin/logs/search":             "host logs carry the addresses of other guests",
	"GET /api/v1/admin/logs/media":              "the media filter of the log search, locked with it",
	"GET /api/v1/admin/logs/stream":             "host logs, live, with the addresses of other guests",
	"GET /api/v1/admin/backups/{id}":            "downloads the database dump - every hash and every setting",
	"GET /api/v1/admin/network-info":            "discloses the host's interfaces and addresses",
	"GET /api/v1/admin/load-test/peer":          "discloses the proxy's address on the host's network",
	"GET /api/v1/admin/media/{id}/sync-preview": "transcodes on every call",
	// Every visitor's browser and phone is a device of the one shared account:
	// "Play on" reached another visitor's screen, the same harm as
	// SendSessionCommand (locked above for that reason).
	"POST /api/v1/devices/{id}/command":            "drives another visitor's player: every device belongs to the shared account",
	"GET /api/v1/admin/media/{id}/search-metadata": "an outbound provider call per request on the operator's key; the match it serves is locked",
	"GET /metrics": "process and host telemetry; the dashboard shows what a visitor needs",

	"DELETE /api/v1/admin/backups/{id}":                    "deletes files",
	"DELETE /api/v1/admin/media/{id}/segments":             "segment edits are library maintenance, not the product",
	"PUT /api/v1/admin/media/{id}/segments":                "same",
	"POST /api/v1/admin/media/{id}/segments/propagate":     "same, season-wide",
	"POST /api/v1/admin/media/{id}/detect-segments":        "ffmpeg on demand",
	"POST /api/v1/admin/libraries/{id}/detect-intros":      "ffmpeg, library-wide",
	"POST /api/v1/admin/libraries/{id}/prune/confirm":      "deletes library items and the watch data hanging off them",
	"DELETE /api/v1/admin/transcode/calibration/overrides": "changes the encode ladder under every viewer",
	"POST /api/v1/admin/transcode/calibration/recalibrate": "benchmarks the host's encoders",
	"PATCH /api/v1/admin/users/{id}/bitrate-limit":         "can throttle the shared account to nothing",
	"POST /api/v1/admin/artworks/backfill-blurhashes":      "library-wide job",
	"POST /api/v1/admin/artworks/download-all":             "library-wide provider calls",
	"POST /api/v1/admin/artworks/purge-variants":           "deletes files",
	"POST /api/v1/admin/artworks/retry-missing":            "library-wide provider calls",
	"POST /api/v1/admin/background-tasks/{name}/resume":    "drives host work",
	"POST /api/v1/admin/background-tasks/{name}/stop":      "stops the scheduled restore-adjacent sweeps",
	"POST /api/v1/admin/backups":                           "writes a dump on the host",
	"POST /api/v1/admin/backups/restore":                   "replaces the database",
	"POST /api/v1/admin/backups/{id}/download-token":       "mints a credential for the dump download",
	"POST /api/v1/admin/media/{id}/artwork":                "file upload from strangers onto the host",
	"POST /api/v1/admin/media/{id}/trickplay/generate":     "ffmpeg on demand",
	"POST /api/v1/admin/metadata/refresh-all":              "library-wide provider calls",
	"POST /api/v1/admin/music/fetch-missing-art":           "library-wide provider calls",
	"POST /api/v1/admin/plugins/preview":                   "analyses a caller-supplied bundle",
	"POST /api/v1/admin/plugins/upload":                    "installs code",
	"POST /api/v1/admin/search/reindex":                    "rebuilds the search index under every visitor",
	"POST /api/v1/admin/smtp/test":                         "sends real mail from the host",
	"POST /api/v1/admin/translations/refresh-all":          "library-wide provider calls",
	"POST /api/v1/admin/trickplay/generate-all":            "ffmpeg, library-wide",
	"POST /api/v1/admin/update-status/check":               "outbound call to the release feed on demand",
	"POST /api/v1/admin/webhooks/{id}/test":                "outbound HTTP to a stored URL (SSRF)",
	"PUT /api/v1/admin/playback-settings":                  "operator setting",
	"PUT /api/v1/admin/remote-access":                      "network: can lock everyone out",
	"PUT /api/v1/admin/server-settings":                    "operator settings",
	"PUT /api/v1/admin/sso":                                "identity provider: can open sign-up or lock everyone out",
	"POST /api/v1/admin/sso/test":                          "outbound HTTP to a caller-supplied issuer URL",

	"POST /api/v1/media/{id}/subtitles/download":         "writes a subtitle file next to the media",
	"GET /api/v1/media/{id}/subtitles/search":            "spends the operator's OpenSubtitles quota (capped per key per day); the download it prepares is locked",
	"DELETE /api/v1/media/{id}/subtitles/{subtitleId}":   "deletes a file next to the media",
	"PUT /api/v1/profiles/{id}/parental":                 "a PIN set by one visitor locks every other visitor out of the shared profile",
	"PUT /api/v1/profiles/{id}/schedule":                 "opening hours set by one visitor close the kids profile on every other visitor",
	"POST /api/v1/profiles/{id}/schedule/unlock":         "same: the hours are locked, so is lifting them",
	"POST /api/v1/profiles/{id}/parental/reset-lockout":  "a shared profile's PIN lockout, cleared by one visitor for every other",
	"POST /api/v1/admin/media/{id}/reset-metadata":       "same",
	"POST /api/v1/admin/media/{id}/refresh-metadata":     "re-identifies a title for everyone",
	"POST /api/v1/media/{id}/artwork/{artworkId}/select": "changes a title's poster for everyone",
	"PUT /api/v1/playlists/{id}/items/reorder":           "same",
	"POST /api/v1/playlists/generate":                    "shared playlists",
	"POST /api/v1/profiles/{profileId}/avatar":           "the shared profile's picture, and a file upload from strangers",
}

// RouteLocked reports whether the demo refuses a request the mux matched to
// pattern. An unclassified pattern is locked when it is an admin route or a
// write - the guard makes sure no registered route relies on this default.
func RouteLocked(method, pattern string) bool {
	if pattern == "" {
		return false
	}
	if _, locked := LockedRoutes[pattern]; locked {
		return true
	}
	if _, open := OpenRoutes[pattern]; open {
		return false
	}
	if strings.Contains(pattern, "/admin/") {
		return true
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// Level is how much of the demo a request is judged against. Public is the
// shared, read-only demo (EVEY_DEMO=1). Extended is a private instance an
// outside guest gets an administrator on: it opens everything Public locks
// except what ExtendedStaysLocked and ExtendedLockedRoutes name.
type Level int

const (
	Public Level = iota
	Extended
)

// ExtendedStaysLockedRPCs are the RPCs that stay refused on the extended demo,
// each with the reason. Every key must also be in LockedAdminRPCs or
// LockedUserRPCs (the contractguard test refuses a stale entry): extended can
// only open what Public locks, it never locks what Public leaves open.
var ExtendedStaysLockedRPCs = map[string]string{ //nolint:gosec // G101: RPC names with their reasons, no credential
	"/evey.v1.AdminSystemService/StartLoadTest":  "burns the host's CPU for nothing a guest needs to see",
	"/evey.v1.AdminSystemService/CancelLoadTest": "symmetric with the start",
	"/evey.v1.AdminSystemService/RestartServer":  "takes the instance down until somebody restarts it",

	"/evey.v1.AdminPluginService/InstallPlugin":       "fetches and runs code from a caller-supplied source",
	"/evey.v1.AdminPluginService/PreviewPluginUpdate": "downloads a caller-supplied URL (SSRF)",
	"/evey.v1.AdminPluginService/ApplyPluginUpdate":   "installs code from a caller-supplied URL",
	"/evey.v1.AdminPluginService/AddPluginRuntime":    "installs a language runtime on the host",
	"/evey.v1.AdminPluginService/RemovePluginRuntime": "removes one",

	"/evey.v1.WebhookService/CreateWebhook": "points the server at a caller-supplied URL: the host would relay requests",
	"/evey.v1.WebhookService/UpdateWebhook": "re-points a webhook at a caller-supplied URL, same relay",

	"/evey.v1.RemoteAccessService/UpdateRemoteAccess": "network: can lock every off-LAN caller out, the guest included",
	"/evey.v1.RemoteAccessService/GetNetworkInfo":     "discloses the host's interfaces and addresses",

	"/evey.v1.AdminSessionService/GetAuditLog": "carries the addresses and user agents of other guests",

	// The MKV service takes a host path and writes beside it: the library
	// editors reach the same engine through the media-health RPCs, which stay
	// open and are bound to the library.
	"/evey.v1.MKVService/GetContainerInfo":  "takes a host path",
	"/evey.v1.MKVService/ListTracks":        "takes a host path",
	"/evey.v1.MKVService/ExtractTrack":      "writes a file at a caller-supplied path",
	"/evey.v1.MKVService/MuxContainer":      "writes a file at a caller-supplied path",
	"/evey.v1.MKVService/DemuxContainer":    "writes files at a caller-supplied path",
	"/evey.v1.MKVService/AddTrack":          "rewrites a file at a caller-supplied path",
	"/evey.v1.MKVService/RemoveTrack":       "rewrites a file at a caller-supplied path",
	"/evey.v1.MKVService/ListAttachments":   "reads arbitrary filesystem paths",
	"/evey.v1.MKVService/ExtractAttachment": "arbitrary file write",

	"/evey.v1.MusicService/ScanMusicDirectory": "scans an arbitrary absolute path",
	"/evey.v1.PhotoService/ScanPhotoDirectory": "scans an arbitrary absolute path",
}

// ExtendedStaysLockedRoutes is ExtendedStaysLockedRPCs for HTTP patterns.
var ExtendedStaysLockedRoutes = map[string]string{ //nolint:gosec // G101: route names with their reasons, no credential
	"GET /api/v1/admin/logs":         "host logs carry the addresses of other guests",
	"GET /api/v1/admin/logs/search":  "host logs carry the addresses of other guests",
	"GET /api/v1/admin/logs/media":   "the media filter of the log search, locked with it",
	"GET /api/v1/admin/logs/stream":  "host logs, live, with the addresses of other guests",
	"GET /api/v1/admin/network-info": "discloses the host's interfaces and addresses",
	"GET /metrics":                   "process and host telemetry",

	"POST /api/v1/admin/backups/restore":     "replaces the database: bricks the instance until the nightly reset",
	"POST /api/v1/admin/plugins/preview":     "analyses a caller-supplied bundle",
	"POST /api/v1/admin/plugins/upload":      "installs code from a stranger",
	"POST /api/v1/admin/webhooks/{id}/test":  "outbound HTTP to a stored URL (SSRF)",
	"POST /api/v1/admin/sso/test":            "outbound HTTP to a caller-supplied issuer URL",
	"PUT /api/v1/admin/sso":                  "identity provider: can lock the guest out or open sign-up to the internet",
	"POST /api/v1/admin/update-status/check": "outbound call to the release feed on demand",
	"PUT /api/v1/admin/remote-access":        "network: can lock everyone out",
}

// ExtendedProtectedUserRPCs are the RPCs that act on the user named by their
// `id` field and that are refused when that user is the guest's own
// account: changing its password, e-mail or role, disabling it or deleting it
// would lock the outside evaluator out. The check needs the message, so the
// interceptor makes it, not RPCLockedAt.
var ExtendedProtectedUserRPCs = map[string]string{
	"/evey.v1.UserService/UpdateUser":  "password, e-mail, username or role of the guest's account",
	"/evey.v1.UserService/DisableUser": "locks the guest out",
	"/evey.v1.UserService/DeleteUser":  "removes the guest's account",
}

// ExtendedLocked is what the extended demo still refuses, RPC full names and
// route patterns together, sorted: the setup status hands it to the clients so
// they grey or hide those controls from the policy itself, not from a copy.
func ExtendedLocked() []string {
	out := make([]string, 0, len(ExtendedStaysLockedRPCs)+len(ExtendedStaysLockedRoutes))
	for k := range ExtendedStaysLockedRPCs {
		out = append(out, k)
	}
	for k := range ExtendedStaysLockedRoutes {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// RPCLockedAt is RPCLocked at a given level.
func RPCLockedAt(level Level, method string, isAdminMethod bool) bool {
	if level == Extended {
		if _, stays := ExtendedStaysLockedRPCs[method]; stays {
			return true
		}
		_, userLocked := LockedUserRPCs[method]
		_, adminLocked := LockedAdminRPCs[method]
		if userLocked || adminLocked {
			return false
		}
	}
	return RPCLocked(method, isAdminMethod)
}

// RouteLockedAt is RouteLocked at a given level.
func RouteLockedAt(level Level, method, pattern string) bool {
	if level == Extended {
		if _, stays := ExtendedStaysLockedRoutes[pattern]; stays {
			return true
		}
		if _, locked := LockedRoutes[pattern]; locked {
			return false
		}
	}
	return RouteLocked(method, pattern)
}
