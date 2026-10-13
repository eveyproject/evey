package library

import (
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type LibraryCreated struct {
	LibraryID  shared.LibraryID
	Name       string
	Type       string
	OccurredAt time.Time
}

func (e LibraryCreated) EventName() string { return "library.created" }

type LibraryDeleted struct {
	LibraryID  shared.LibraryID
	OccurredAt time.Time
}

func (e LibraryDeleted) EventName() string { return "library.deleted" }

type LibraryScanStarted struct {
	LibraryID  shared.LibraryID
	OccurredAt time.Time
}

func (e LibraryScanStarted) EventName() string { return "library.scan.started" }

type LibraryScanCompleted struct {
	LibraryID    shared.LibraryID
	ItemsFound   int
	ItemsAdded   int
	ItemsUpdated int
	ItemsRemoved int
	// FailedPaths are the roots the scan could not read: non-empty means the
	// scan is partial, not complete.
	FailedPaths []string
	OccurredAt  time.Time
}

func (e LibraryScanCompleted) EventName() string { return "library.scan.completed" }

type MediaItemAdded struct {
	MediaItemID shared.MediaItemID
	LibraryID   shared.LibraryID
	Title       string
	SeriesTitle string // non-empty for episodes/seasons
	Type        MediaItemType
	Genres      []string
	OccurredAt  time.Time
}

func (e MediaItemAdded) EventName() string { return "media.item.added" }

type MediaItemRemoved struct {
	MediaItemID shared.MediaItemID
	LibraryID   shared.LibraryID
	OccurredAt  time.Time
}

func (e MediaItemRemoved) EventName() string { return "media.item.removed" }

// MediaItemUpdated is a change a viewer can see: the metadata was edited,
// reset or refreshed, or the file was replaced on disk (Reason says which).
// The scan's own bookkeeping (a move, a backfilled mtime, a re-probe) is not
// one - a rescan must not flood the webhooks.
type MediaItemUpdated struct {
	MediaItemID shared.MediaItemID
	LibraryID   shared.LibraryID // empty when the emitter did not load the item
	Reason      string           // metadata_edited, metadata_reset, metadata_refreshed, file_replaced
	// PreviousDurationMs and DurationMs are the file's duration before and
	// after a file_replaced; 0 when unknown (never probed, or the probe of
	// the new file failed).
	PreviousDurationMs int64
	DurationMs         int64
	OccurredAt         time.Time
}

func (e MediaItemUpdated) EventName() string { return "media.item.updated" }

// Reasons carried by MediaItemUpdated.
const (
	UpdateReasonMetadataEdited    = "metadata_edited"
	UpdateReasonMetadataReset     = "metadata_reset"
	UpdateReasonMetadataRefreshed = "metadata_refreshed"
	UpdateReasonFileReplaced      = "file_replaced"
)

// TrickplayGenerated fires after a trickplay manifest + sidecar are written.
// Consumers (e.g. the intro detection worker) trigger downstream work that
// depends on the trickplay sprites being present on disk. Unlike
// MediaItemAdded, this also fires on regenerations of existing media - that's
// the point: rewipe trickplays + this event re-runs intro detection cleanly.
type TrickplayGenerated struct {
	MediaItemID shared.MediaItemID
	ParentID    shared.MediaItemID // empty for movies
	Type        MediaItemType
	OccurredAt  time.Time
}

func (e TrickplayGenerated) EventName() string { return "trickplay.generated" }
