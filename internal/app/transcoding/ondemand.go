package transcoding

import "context"

// ondemand.go — the port for zero-storage HLS/DASH packaging (the copy rung
// served straight from mkvgo's mp4.PlanHLS instead of a paced `ffmpeg -c copy`
// + fMP4 rebase). A plan does a few bounded reads up front (metadata head +
// seek index) and then builds any single resource — playlist, init, segment,
// DASH manifest — on request, byte-identical to a full pass, with nothing
// pre-generated and no server-side session state. The mkvgo driven adapter
// implements OnDemandPackager.

// OnDemandPlan serves one source file's HLS/DASH presentation resource by
// resource. It is immutable after the packager returns it and safe for
// concurrent Resource calls (the hot path a segment request takes).
type OnDemandPlan interface {
	// Resource returns the bytes and Content-Type for a player-facing resource
	// name (master.m3u8, playlist.m3u8, init.mp4, seg00042.m4s, audio1.m3u8,
	// manifest.mpd, iframe.m3u8, sub1.vtt …). The name is canonical — any URL
	// decoration (prefix, token) is stripped by the caller before this call.
	Resource(ctx context.Context, name string) (data []byte, contentType string, err error)
	// Resources lists every servable canonical name (playlists, init, every
	// segment, subtitles). Cheap to call but O(segments) — avoid on the hot path.
	Resources() []string
	// NumSegments is the video rendition's segment count.
	NumSegments() int
	// HasDASH reports whether a manifest.mpd is available (false for an
	// AES-128-encrypted presentation — HLS only).
	HasDASH() bool
}

// OnDemandOptions configures a plan. The zero value packages a plain HLS/DASH
// presentation with the default segment duration and canonical resource names.
type OnDemandOptions struct {
	// SegmentMs is the target media-segment duration in milliseconds. 0 selects
	// the packager default (6 s). Segments are cut on video keyframes, so the
	// real duration is the first keyframe at/after each multiple.
	SegmentMs int64
	// URLPrefix is prepended to every resource URI the playlists and the DASH
	// manifest reference, so the on-the-wire names never collide with the
	// transcode pipeline's (stream_N_*, audio_N_*) and the dispatcher can route
	// them by prefix. Resource() still takes the canonical (unprefixed) name.
	URLPrefix string
	// AESKey, when set (16 bytes), AES-128-encrypts every media segment and the
	// playlists advertise EXT-X-KEY pointing at AESKeyURI. HLS only — no DASH
	// manifest is emitted for an encrypted presentation.
	AESKey    []byte
	AESKeyURI string
}

// OnDemandPackager builds an OnDemandPlan for a source file — the copy rung, no
// transcode. It returns an error when the source cannot be packaged verbatim
// (a codec CMAF can't carry, a Matroska without a Cues seek index), which is
// the caller's signal to fall back to the ffmpeg pipeline. Implementations
// cache plans, so repeated calls for the same source are cheap.
type OnDemandPackager interface {
	Plan(ctx context.Context, srcPath string, opts OnDemandOptions) (OnDemandPlan, error)
}
