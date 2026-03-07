package metadata

import "time"

type MetadataRefreshed struct {
	MediaItemID string
	Provider    string
	OccurredAt  time.Time
}

func (e MetadataRefreshed) EventName() string { return "metadata.refreshed" }

type MetadataOverrideSet struct {
	MediaItemID string
	FieldName   string
	EditedBy    string
	OccurredAt  time.Time
}

func (e MetadataOverrideSet) EventName() string { return "metadata.override.set" }

type MetadataOverrideRemoved struct {
	MediaItemID string
	FieldName   string
	OccurredAt  time.Time
}

func (e MetadataOverrideRemoved) EventName() string { return "metadata.override.removed" }

type ArtworkDownloaded struct {
	MediaItemID string
	Type        string
	LocalPath   string
	OccurredAt  time.Time
}

func (e ArtworkDownloaded) EventName() string { return "metadata.artwork.downloaded" }
