package mkv

import "errors"

var (
	// ErrNotEBML is returned when an operation requires a Matroska/EBML
	// container but the file at the given path is not one (wrong extension or
	// unrecognised header).
	ErrNotEBML = errors.New("not an EBML container")

	// ErrTrackNotFound is returned when the requested TrackID does not exist in
	// the container or is not of the expected type.
	ErrTrackNotFound = errors.New("track not found")

	// ErrUnsupportedCodec is returned when the requested output format is
	// incompatible with the source track's codec (e.g. requesting ASS output
	// from an SRT track).
	ErrUnsupportedCodec = errors.New("unsupported codec for requested format")
)
