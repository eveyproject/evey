package httpapi

// transcode_vod_profiles.go - the codec/HW routing and ladder shaping moved
// to the app-layer PlanResolver (app/transcoding/playback_plan_resolver.go).
// These wrappers keep the handler-method surface the rest of the adapter and
// the tests use; each delegates to a resolver assembled from the handler's
// live fields (planResolver, transcode_vod_plan.go).

import (
	"context"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

func (h *hlsVODServe) effectiveVideoCodec(decision domaintranscoding.PlaybackDecision, streams []*domainlibrary.MediaStream) (codec, accel, device string, features []string, usable, softwareDecode bool) {
	return h.planResolver().EffectiveVideoCodec(decision, streams)
}

func (h *hlsVODServe) resolveHW(outputCodec, sourceCodec string, sourceTenBit, hwTonemap bool) (accel, device string, features []string, usable, softwareDecode bool) {
	return h.planResolver().ResolveHW(outputCodec, sourceCodec, sourceTenBit, hwTonemap)
}

// hwRouteUsable is the selector's routing gate (see PlanResolver.HWRouteUsable);
// newHLSVODServe hands this method to NewProducerSelector so the routing
// matches exactly what tags the fingerprint.
func (h *hlsVODServe) hwRouteUsable(decision domaintranscoding.PlaybackDecision, src apptranscoding.SourceDims) bool {
	return h.planResolver().HWRouteUsable(decision, src)
}

func (h *hlsVODServe) hwTonemapWanted(decision domaintranscoding.PlaybackDecision) bool {
	return h.planResolver().HWTonemapWanted(decision)
}

func (h *hlsVODServe) profilesForDecision(ctx context.Context, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, decision domaintranscoding.PlaybackDecision) []domaintranscoding.HLSProfile {
	req := apptranscoding.PlaybackRequest{
		MediaID:     itemID(item),
		Streams:     streams,
		SourceBytes: sourceBytesOf(item),
	}
	return h.planResolver().ShapeLadder(ctx, req, decision)
}

func (h *hlsVODServe) ladderClassFor(decision domaintranscoding.PlaybackDecision, streams []*domainlibrary.MediaStream) apptranscoding.LadderClass {
	return h.planResolver().LadderClassFor(decision, streams)
}

// audioRenditionsFor reports whether THIS request serves its audio as a separate
// HLS rendition (#EXT-X-MEDIA) rather than muxed into the video segments.
//
// Only when the VIDEO IS COPIED. The rendition is placed on the video's timeline
// by offsetting it with the SOURCE's composition delay (has_b_frames/fps), which
// is only the truth when the video is copied bit-for-bit. On a transcode the
// delay belongs to the ENCODER: a source with no B-frames at all comes out of
// libx264 presented 83 ms late, and the offset would be computed as zero - the
// audio would run early again, audibly.
//
// A transcode does not need the split anyway: the video already goes through
// ffmpeg, so the audio rides the same process and stays in sync by construction.
// Renditions exist to unlock the HYBRID rung - mkvgo packaging a COPIED video
// with zero ffmpeg, while its non-browser audio is re-encoded on the side.
func (h *hlsVODServe) audioRenditionsFor(decision domaintranscoding.PlaybackDecision) bool {
	return h.audioRenditionEnabled && decision.VideoDecision == "copy"
}
