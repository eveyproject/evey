package transcoding

import (
	"context"
	"io"
)

// ondemand.go - the port for zero-storage HLS/DASH packaging: the copy rung
// served straight from mkvgo's mp4.PlanHLS. A plan does a few bounded reads up
// front, then builds any resource on request, byte-identical to a full pass,
// with no on-disk segments and no session state.

// OnDemandPlan serves one source file's HLS/DASH presentation resource by
// resource. Immutable once returned, safe for concurrent Resource calls.
type OnDemandPlan interface {
	// Resource returns the bytes and Content-Type for a canonical resource name
	// (master.m3u8, playlist.m3u8, init.mp4, seg00042.m4s, audio1.m3u8,
	// manifest.mpd, iframe.m3u8, sub1.vtt …); URL decoration is stripped by the caller.
	Resource(ctx context.Context, name string) (data []byte, contentType string, err error)
	// Resources lists every servable canonical name. O(segments) - avoid on the hot path.
	Resources() []string
	// NumSegments is the video rendition's segment count.
	NumSegments() int
	// HasDASH reports whether a manifest.mpd is available (false when AES-128 encrypted).
	HasDASH() bool
}

// OnDemandOpener is the streaming side of a plan: a media segment is written
// from the source as it goes out, through one small buffer, instead of being
// assembled whole in memory first. A plan that can stream implements it
// alongside OnDemandPlan; the HTTP adapter uses it for segments when present
// (measured by mkvgo: 16 slow clients on a UHD remux, RSS 940 MiB → 28 MiB).
type OnDemandOpener interface {
	// Open opens a resource by its canonical name. The handle's bytes equal
	// Resource's exactly; whether they are in hand or streamed is Bytes().
	Open(ctx context.Context, name string) (OnDemandResource, error)
}

// OnDemandResource is one opened resource: its type and exact size up front,
// its bytes on demand.
type OnDemandResource interface {
	ContentType() string
	Size() int64
	// ETag is strong when the bytes are in hand, weak when they stream.
	ETag() string
	// Bytes is the whole resource when the handle holds it, nil when it
	// streams from the source (then WriteRange is the only way to the bytes).
	Bytes() []byte
	// WriteRange writes bytes [off, off+n). A streamed segment read from the
	// source can fail midway and leave the output short: the caller must turn
	// that into a broken connection, never a clean end.
	WriteRange(ctx context.Context, w io.Writer, off, n int64) (int64, error)
	Close() error
}

// OnDemandOptions configures a plan. The zero value packages a plain HLS/DASH
// presentation with the default segment duration and canonical resource names.
type OnDemandOptions struct {
	// SegmentMs is the target media-segment duration; 0 = the packager default
	// (6 s). Segments are cut on keyframes, so the real duration may exceed it.
	SegmentMs int64
	// URLPrefix is prepended to every resource URI the playlists reference so
	// the on-the-wire names never collide with the transcode pipeline's and the
	// dispatcher routes them by prefix. Resource() still takes the canonical name.
	URLPrefix string
	// AESKey (16 bytes) AES-128-encrypts every media segment; the playlists
	// advertise EXT-X-KEY pointing at AESKeyURI. HLS only.
	AESKey    []byte
	AESKeyURI string
	// AESRotateSegments > 0 with at least two AESKeys rotates the key every N
	// segments (a captured key decrypts only its period); supersedes AESKey.
	AESRotateSegments int
	AESKeys           []OnDemandAESKey
	// KeepAudioTrack restricts the packaged audio to this single track (the
	// 0-based ordinal among the audio streams, ffmpeg's 0:a:N); video and every
	// subtitle stay. nil packages every audio track. Keyed separately in the cache.
	KeepAudioTrack *int
}

// OnDemandAESKey is one rotation key: its 16 bytes plus the URI the EXT-X-KEY
// line advertises for it.
type OnDemandAESKey struct {
	Key []byte
	URI string
}

// OnDemandPackager builds an OnDemandPlan for a source file. An error means
// the source cannot be packaged verbatim - the caller's signal to fall back to
// the ffmpeg pipeline. Implementations cache plans.
type OnDemandPackager interface {
	Plan(ctx context.Context, srcPath string, opts OnDemandOptions) (OnDemandPlan, error)
}
