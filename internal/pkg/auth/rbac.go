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
	PermActivityRead   = "activity.read"
	PermAuditRead      = "audit.read"
	PermServerAdmin    = "server.admin"
)

// Maps gRPC method full names to required permissions
var MethodPermissions = map[string]string{
	"/evey.LibraryService/CreateLibrary":       PermLibraryCreate,
	"/evey.LibraryService/GetLibrary":          PermLibraryRead,
	"/evey.LibraryService/ListLibraries":       PermLibraryRead,
	"/evey.LibraryService/DeleteLibrary":       PermLibraryDelete,
	"/evey.LibraryService/ScanLibrary":         PermLibraryScan,
	"/evey.LibraryService/GetMediaItem":        PermMediaRead,
	"/evey.LibraryService/ListMediaItems":      PermMediaRead,
	"/evey.LibraryService/SearchMedia":         PermMediaRead,
	"/evey.UserService/CreateUser":             PermUserCreate,
	"/evey.UserService/ListUsers":              PermUserRead,
	"/evey.UserService/UpdateUser":             PermUserUpdate,
	"/evey.UserService/DeleteUser":             PermUserDelete,
	"/evey.UserService/DisableUser":            PermUserDisable,
	"/evey.PlaybackService/StartStream":        PermPlaybackStart,
	"/evey.PlaybackService/StopStream":         PermPlaybackStop,
	"/evey.TranscodingService/StartTranscode":  PermTranscodeStart,
	"/evey.TranscodingService/StopTranscode":   PermTranscodeStop,
	"/evey.PluginService/InstallPlugin":        PermPluginInstall,
	"/evey.PluginService/UninstallPlugin":      PermPluginManage,
	"/evey.MetadataService/RefreshMetadata":    PermMetadataUpdate,
	"/evey.MetadataService/SearchMetadata":     PermMetadataRead,
	"/evey.ActivityService/GetAuditLog":        PermAuditRead,
	"/evey.ActivityService/GetServerStats":     PermActivityRead,
}
