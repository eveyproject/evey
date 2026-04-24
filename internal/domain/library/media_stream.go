package library

// Strict enum for MediaStream.StreamType. The underlying StreamType type and
// the legacy short-form constants (Video, Audio, Subtitle) live in model.go
// for historical reasons; this file augments the enum with the canonical
// StreamType-prefixed names, the missing "attachment" variant, and the
// IsValid/String surface used by future validators.
//
// The MediaStream.StreamType struct field stays as `string` for now: the
// cascade to postgres, ffmpeg, http, grpcapi and app/* call sites is out of
// scope for this change (tracked as a follow-up). Once those sites migrate
// to the typed value the struct field can be promoted to StreamType and the
// legacy short-form constants in model.go can be removed.

const (
	// StreamTypeVideo is the canonical typed alias for Video. Equivalent to
	// the legacy Video constant.
	StreamTypeVideo StreamType = "video"
	// StreamTypeAudio is the canonical typed alias for Audio. Equivalent to
	// the legacy Audio constant.
	StreamTypeAudio StreamType = "audio"
	// StreamTypeSubtitle is the canonical typed alias for Subtitle. Equivalent
	// to the legacy Subtitle constant.
	StreamTypeSubtitle StreamType = "subtitle"
	// StreamTypeAttachment covers fonts, cover art and other non-AV streams
	// the prober collapses into the "attachment" bucket.
	StreamTypeAttachment StreamType = "attachment"
)

// String returns the underlying string form of the stream type. Useful when
// passing a StreamType to APIs that still accept string (e.g. the postgres
// repo and grpc converters until the cascade migrates).
func (t StreamType) String() string { return string(t) }

// IsValidStrict reports whether t is one of the four recognised stream types
// (video, audio, subtitle, attachment). Unlike the legacy IsValid (kept in
// model.go for backward compat), this also accepts "attachment" — the value
// the ffmpeg prober assigns to fonts and unknown codec types.
//
// Empty string is rejected: the prober never emits it (codecTypeToStreamType
// defaults unknowns to "attachment"), so an empty StreamType in the wild is
// always a bug rather than an unknown-but-valid value.
func (t StreamType) IsValidStrict() bool {
	switch t {
	case StreamTypeVideo, StreamTypeAudio, StreamTypeSubtitle, StreamTypeAttachment:
		return true
	}
	return false
}
