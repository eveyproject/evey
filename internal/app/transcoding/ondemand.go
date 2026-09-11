package transcoding

import "context"

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
