package transcoding

import "context"

// concat.go - the port for gapless multi-episode HLS packaging (mkvgo's
// mp4.PlanConcat): several sources - consecutive episodes - served as ONE
// continuous HLS session, so the player never reloads and never sees a
// session boundary. Same zero-storage contract as OnDemandPackager: nothing
// pre-generated, every resource built on request. v1 has no encrypted or
// single-file variant and no combined DASH manifest - see mkvgo's concat.go.

// ConcatPlan serves a concatenated multi-source HLS session resource by
// resource. Resource names mirror mkvgo's ConcatPlan: "master.m3u8",
// "playlist.m3u8", "audio{j}.m3u8", "sub{j}.m3u8"/"sub{j}.vtt" at the top, and
// "p{k}/<name>" (0-based) for a resource of part k.
type ConcatPlan interface {
	Resource(ctx context.Context, name string) (data []byte, contentType string, err error)
	Resources() []string
	// NumParts is how many sources the concatenated session carries.
	NumParts() int
}

// ConcatOptions configures a concatenated plan. Deliberately narrower than
// OnDemandOptions (v1 has no AES/SingleFile - PlanConcat refuses them).
type ConcatOptions struct {
	SegmentMs int64
	URLPrefix string
}

// ConcatPackager builds a ConcatPlan for an ordered list of sources (at least
// two). It returns an error when the sources can't be packaged as one session
// - incompatible video codec or audio layout across parts, or any part
// individually unpackageable (the same refusals OnDemandPackager.Plan makes)
// - which is the caller's signal to fall back to per-episode playback.
type ConcatPackager interface {
	Plan(ctx context.Context, sources []string, opts ConcatOptions) (ConcatPlan, error)
}
