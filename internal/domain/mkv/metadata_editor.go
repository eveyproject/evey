package mkv

import "context"

// MetadataEditor edits MKV container metadata: segment title and per-track
// fields (language, name, default/forced flags). It does NOT transcode or
// repackage audio/video - only the header region is touched.
type MetadataEditor interface {
	// EditMetadata applies req to srcPath, writing the result to dstPath.
	// dstPath may equal srcPath only if the implementation guarantees atomic
	// replacement (write-to-temp + rename). The domain layer never assumes
	// in-place safety; callers that want in-place behavior must use EditInPlace.
	EditMetadata(ctx context.Context, srcPath, dstPath string, req EditMetadataRequest) error

	// EditInPlace rewrites only the metadata region of path on disk.
	// It is fast (no cluster copy) but NOT crash-safe: a crash mid-write can
	// corrupt the file when the new metadata is larger than the existing region
	// EditInPlace returns an error. For precious data prefer EditMetadata.
	EditInPlace(ctx context.Context, path string, req EditMetadataRequest) error
}

// EditMetadataRequest carries the mutations to apply to a container. A nil
// optional pointer means "leave as-is".
type EditMetadataRequest struct {
	// Title, when non-nil, sets the SegmentInfo title.
	Title *string

	// TrackEdits lists per-track mutations keyed by TrackID (Matroska
	// TrackNumber). Tracks absent from the map are left untouched.
	TrackEdits []TrackEdit
}

// TrackEdit describes mutations for a single track. A nil pointer means
// "leave as-is".
type TrackEdit struct {
	// TrackID is the Matroska TrackNumber (1-based) of the track to edit.
	TrackID   uint64
	Language  *string
	Name      *string
	IsDefault *bool
	IsForced  *bool
}
