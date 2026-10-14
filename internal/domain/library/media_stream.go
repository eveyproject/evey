package library

import (
	"errors"
	"fmt"
	"strings"
)

// ErrStreamSetChanged: a probe of an unchanged file found other container
// streams (index, type, codec) than the stored ones, so a field-by-field
// refresh has no row to write to (see MediaStreamRepository.RefreshProbedFields).
var ErrStreamSetChanged = errors.New("library: the probed streams are not the stored ones")

// Strict enum for MediaStream.StreamType. The underlying StreamType type and
// the legacy short-form constants (Video, Audio, Subtitle) live in model.go
// for historical reasons; this file augments the enum with the canonical
// StreamType-prefixed names, the missing "attachment" variant, and the
// IsValid/String surface used by validators.
//
// The MediaStream.StreamType struct field is the typed StreamType: postgres,
// ffmpeg, http, grpcapi and app/* call sites all consume the typed value.
// The legacy short-form constants in model.go are kept as aliases for the
// StreamType-prefixed names; equivalence is pinned by TestStreamType_LegacyAliases.

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
// model.go for backward compat), this also accepts "attachment" - the value
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

// ErrInvalidStreamType signals a MediaStream whose StreamType is not one of
// the four valid values. Surfaced from Validate() so the postgres adapter
// can reject the row before it hits the database - a defense-in-depth net
// against future call sites bypassing codecTypeToStreamType.
var ErrInvalidStreamType = fmt.Errorf("media stream: invalid StreamType")

// Validate enforces strict invariants on a MediaStream before persistence.
// Currently checks only the StreamType enum; extend here as fields are added.
func (s *MediaStream) Validate() error {
	if !s.StreamType.IsValidStrict() {
		return fmt.Errorf("%w: %q", ErrInvalidStreamType, s.StreamType)
	}
	return nil
}

// stillImageCodecs are the codecs a muxer uses for cover art riding as a
// "video" track. ffprobe flags most of them attached_pic, but a Matroska PNG
// cover stored as a real video track (seen in prod, 2026-09-04: a 1920x804
// H.264 film with a 200x258 "cover" PNG track) carries no such disposition
// and still is not a picture anyone can play.
var stillImageCodecs = map[string]bool{
	"png": true, "mjpeg": true, "bmp": true, "webp": true, "tiff": true,
}

// IsStillImageCodec reports whether codec is a still-image codec (png, mjpeg,
// bmp, webp, tiff) - cover art when it sits next to a real video stream.
func IsStillImageCodec(codec string) bool {
	return stillImageCodecs[codec]
}

// IsCoverArtStream reports whether s is a still-image "video" track shadowing
// a real video stream: only then is it cover art. A file whose ONLY video is
// MJPEG (camera footage) keeps that stream as its picture.
func IsCoverArtStream(s *MediaStream, hasRealVideo bool) bool {
	return s != nil && s.StreamType == StreamTypeVideo && hasRealVideo && IsStillImageCodec(s.Codec)
}

// HasRealVideoStream reports whether streams carry at least one video track
// whose codec is not a still image.
func HasRealVideoStream(streams []*MediaStream) bool {
	for _, s := range streams {
		if s != nil && s.StreamType == StreamTypeVideo && !IsStillImageCodec(s.Codec) {
			return true
		}
	}
	return false
}

// ScanType is the interlacing verdict every decision path reads (the
// deinterlacer gate, the idet backfill). The persisted MediaStream.FieldOrder
// is the prober's raw word and is never matched anywhere else: ffprobe names
// the field order, mkvgo the verdict, and both vocabularies are historical
// data this reader keeps understanding.
type ScanType string

const (
	ScanUnknown     ScanType = ""
	ScanProgressive ScanType = "progressive"
	ScanInterlaced  ScanType = "interlaced"
)

// fieldOrderScanTypes is the ONE vocabulary table: ffprobe's field_order
// (progressive, tt, bb, tb, bt) and mkvgo's verdict (progressive, interlaced -
// Matroska FlagInterlaced or the H.264 frame_mbs_only_flag). A word outside
// it (ffprobe's "unknown", a future prober's token) is ScanUnknown, never
// progressive: a session-level deinterlacer must not run blind, and a
// progressive film must not lose its cheaper backend on a guess. The table
// is pinned to mkvgo's own mkv.ScanTypeOf by the prober adapter's parity
// test (the domain imports no third party).
var fieldOrderScanTypes = map[string]ScanType{
	"progressive": ScanProgressive,
	"interlaced":  ScanInterlaced,
	"tt":          ScanInterlaced,
	"bb":          ScanInterlaced,
	"tb":          ScanInterlaced,
	"bt":          ScanInterlaced,
}

// ScanTypeOf reads a raw field_order in either prober's vocabulary.
func ScanTypeOf(fieldOrder string) ScanType {
	return fieldOrderScanTypes[strings.ToLower(strings.TrimSpace(fieldOrder))]
}

// NormalizeFieldOrder is what a prober persists: the trimmed, lower-cased word
// when it belongs to a known vocabulary, nil otherwise. The word itself is
// kept (a "tt" stays finer than "interlaced") so historical rows and new ones
// stay comparable; only ScanTypeOf interprets it.
func NormalizeFieldOrder(raw string) *string {
	fo := strings.ToLower(strings.TrimSpace(raw))
	if ScanTypeOf(fo) == ScanUnknown {
		return nil
	}
	return &fo
}

// ScanType is the stream's interlacing verdict from its persisted field order.
func (s *MediaStream) ScanType() ScanType {
	if s == nil || s.FieldOrder == nil {
		return ScanUnknown
	}
	return ScanTypeOf(*s.FieldOrder)
}
