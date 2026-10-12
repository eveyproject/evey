package metadata

import (
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type MetadataRefreshed struct {
	MediaItemID shared.MediaItemID
	Provider    string
	OccurredAt  time.Time
}

func (e MetadataRefreshed) EventName() string { return "metadata.refreshed" }

type MetadataOverrideSet struct {
	MediaItemID shared.MediaItemID
	FieldName   string
	EditedBy    shared.UserID
	OccurredAt  time.Time
}

func (e MetadataOverrideSet) EventName() string { return "metadata.override.set" }

type MetadataOverrideRemoved struct {
	MediaItemID shared.MediaItemID
	FieldName   string
	OccurredAt  time.Time
}

func (e MetadataOverrideRemoved) EventName() string { return "metadata.override.removed" }

type ArtworkDownloaded struct {
	MediaItemID shared.MediaItemID
	Type        string
	LocalPath   string
	OccurredAt  time.Time
}

func (e ArtworkDownloaded) EventName() string { return "metadata.artwork.downloaded" }
