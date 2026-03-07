package notification

import "time"

type Event struct {
	ID        string
	Type      string
	Source    string
	Payload   map[string]interface{}
	CreatedAt time.Time
}

type Subscription struct {
	ID           string
	SubscriberID string
	EventType    string
	Callback     func(Event)
}

const (
	EventLibraryCreated       = "library.created"
	EventLibraryDeleted       = "library.deleted"
	EventLibraryScanStarted   = "library.scan.started"
	EventLibraryScanCompleted = "library.scan.completed"
	EventMediaItemAdded       = "media.item.added"
	EventMediaItemUpdated     = "media.item.updated"
	EventMediaItemRemoved     = "media.item.removed"
	EventUserCreated          = "user.created"
	EventUserDeleted          = "user.deleted"
	EventUserDisabled         = "user.disabled"
	EventProfileCreated       = "user.profile.created"
	EventLoginSucceeded       = "user.login.succeeded"
	EventLoginFailed          = "user.login.failed"
	EventPlaybackStarted      = "playback.started"
	EventPlaybackStopped      = "playback.stopped"
	EventPlaybackProgress     = "playback.progress.reported"
	EventMediaMarkedWatched   = "media.marked.watched"
	EventTranscodeStarted     = "transcode.started"
	EventTranscodeCompleted   = "transcode.completed"
	EventTranscodeFailed      = "transcode.failed"
	EventTranscodeCancelled   = "transcode.cancelled"
	EventContainerParsed      = "mkv.container.parsed"
	EventTrackExtracted       = "mkv.track.extracted"
	EventContainerMuxed       = "mkv.container.muxed"
	EventPluginInstalled      = "plugin.installed"
	EventPluginUninstalled    = "plugin.uninstalled"
	EventPluginStarted        = "plugin.started"
	EventPluginStopped        = "plugin.stopped"
	EventPluginError          = "plugin.error"
	EventMetadataRefreshed    = "metadata.refreshed"
	EventMetadataOverrideSet  = "metadata.override.set"
	EventMetadataOverrideRm   = "metadata.override.removed"
	EventArtworkDownloaded    = "metadata.artwork.downloaded"
	EventLoginRecorded        = "activity.login.recorded"
	EventMetricRecorded       = "activity.metric.recorded"
	EventAuditLogCreated      = "activity.audit.created"
)
