package library

import (
	"errors"
	"strings"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

// Sentinel errors returned by NewMediaItem so call sites can branch on
// the specific invariant violated rather than string-matching messages.
// The struct's Validate() method (model.go) keeps its bare errors.New
// for backward compatibility with existing call sites.
var (
	// ErrInvalidLibraryID is returned when the LibraryID is empty.
	ErrInvalidLibraryID = errors.New("media item: invalid library ID")
	// ErrInvalidMediaItemType is returned when the MediaItemType is not one
	// of the recognised enum values (see MediaItemType.IsValid).
	ErrInvalidMediaItemType = errors.New("media item: invalid type")
	// ErrEmptyTitle is returned when the title is empty after trimming whitespace.
	ErrEmptyTitle = errors.New("media item: empty title")
	// ErrMissingParent is returned when an Episode or Season is constructed
	// without a ParentID - the hierarchy invariant Series > Season > Episode
	// requires every non-root node to point to its parent.
	ErrMissingParent = errors.New("media item: missing parent ID for episode/season")
)

// MediaItemOption customises a MediaItem at construction. Required
// identity fields (libraryID, type, title) are positional in NewMediaItem;
// every other field is opt-in via these options. Sub-aggregates (File,
// Catalog, Classification) get dedicated WithMediaItemX setters so the
// caller can construct them inline without fiddling with pointers.
type MediaItemOption func(*MediaItem)

// WithMediaItemID sets the canonical identifier. Optional because the
// scanner can let the postgres adapter mint a UUID at insert time, but
// most call sites already have an ID in hand.
func WithMediaItemID(id shared.MediaItemID) MediaItemOption {
	return func(m *MediaItem) { m.ID = id }
}

// WithMediaItemParent sets the parent ID - required for Episode/Season
// (see ErrMissingParent) and ignored by validation otherwise.
func WithMediaItemParent(parentID string) MediaItemOption {
	return func(m *MediaItem) { m.ParentID = parentID }
}

// WithMediaItemScanTitle sets the filesystem-derived title used as the
// stable lookup key for re-enrichment. Defaults to the construction
// title when not provided.
func WithMediaItemScanTitle(scanTitle string) MediaItemOption {
	return func(m *MediaItem) { m.ScanTitle = scanTitle }
}

// WithMediaItemSortTitle sets the sort-friendly title (typically the
// title with leading articles removed).
func WithMediaItemSortTitle(sortTitle string) MediaItemOption {
	return func(m *MediaItem) { m.SortTitle = sortTitle }
}

// WithMediaItemOverview sets the long-form synopsis.
func WithMediaItemOverview(overview string) MediaItemOption {
	return func(m *MediaItem) { m.Overview = overview }
}

// WithMediaItemTagline sets the marketing tagline.
func WithMediaItemTagline(tagline string) MediaItemOption {
	return func(m *MediaItem) { m.Tagline = tagline }
}

// WithMediaItemIndexNumber sets the episode/track ordinal within its
// parent container.
func WithMediaItemIndexNumber(n int) MediaItemOption {
	return func(m *MediaItem) {
		v := n
		m.IndexNumber = &v
	}
}

// WithMediaItemYear is a shortcut: stamps the year on the catalog
// sub-aggregate, allocating it when nil.
func WithMediaItemYear(year int) MediaItemOption {
	return func(m *MediaItem) {
		if m.Catalog == nil {
			m.Catalog = &MediaCatalog{}
		}
		v := year
		m.Catalog.Year = &v
	}
}

// WithMediaItemFile attaches the on-disk artefact sub-aggregate.
func WithMediaItemFile(file *MediaFile) MediaItemOption {
	return func(m *MediaItem) { m.File = file }
}

// NewMediaItem is the canonical constructor: stamps AddedAt/UpdatedAt,
// applies options, then validates the four core invariants (library ID,
// type validity, non-empty title, parent presence for episode/season).
//
// The struct stays exported and zero-value instantiation via &MediaItem{...}
// keeps working for existing call sites - this constructor adds an
// optional validated path so new code can express the invariants up
// front rather than discover them at repository Create time.
func NewMediaItem(libraryID shared.LibraryID, itemType MediaItemType, title string, opts ...MediaItemOption) (*MediaItem, error) {
	if libraryID == "" {
		return nil, ErrInvalidLibraryID
	}
	if !itemType.IsValid() {
		return nil, ErrInvalidMediaItemType
	}
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return nil, ErrEmptyTitle
	}

	now := time.Now()
	m := &MediaItem{
		LibraryID: libraryID,
		Type:      itemType,
		Title:     trimmed,
		ScanTitle: trimmed,
		AddedAt:   now,
		UpdatedAt: now,
	}
	for _, opt := range opts {
		opt(m)
	}

	if (itemType == Episode || itemType == Season) && m.ParentID == "" {
		return nil, ErrMissingParent
	}

	return m, nil
}
