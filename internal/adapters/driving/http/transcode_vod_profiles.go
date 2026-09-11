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

func (h *hlsVODServe) hwTonemapWanted(decision domaintranscoding.PlaybackDecision, hdrRange string) bool {
	return h.planResolver().HWTonemapWanted(decision, hdrRange)
}

func (h *hlsVODServe) profilesForDecision(ctx context.Context, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, decision domaintranscoding.PlaybackDecision) []domaintranscoding.HLSProfile {
	req := apptranscoding.PlaybackRequest{
		MediaID:           itemID(item),
		Streams:           streams,
		SourceBytes:       sourceBytesOf(item),
		SourceBitrateKbps: sourceBitrateKbpsOf(item, streams),
	}
	return h.planResolver().ShapeLadder(ctx, req, decision)
}

func (h *hlsVODServe) ladderClassFor(decision domaintranscoding.PlaybackDecision, streams []*domainlibrary.MediaStream) apptranscoding.LadderClass {
	return h.planResolver().LadderClassFor(decision, streams)
}

// audioRenditionsFor reports whether THIS request serves its audio as a separate
// HLS rendition (#EXT-X-MEDIA): one AAC encode shared by every rung, and a rung
// switch that never re-fetches audio. The rendition is placed on the video's
// timeline with the video's composition delay (transcode_audio_sync.go), so a
// source whose frame rate the transcode changes (inverse telecine) stays muxed.
func (h *hlsVODServe) audioRenditionsFor(decision domaintranscoding.PlaybackDecision, streams []*domainlibrary.MediaStream) bool {
	if !h.audioRenditionEnabled {
		return false
	}
	if decision.VideoDecision == "transcode" {
		return h.encoderReorderProbe != nil && h.frameRateProbe != nil && !apptranscoding.SourceTelecine(streams)
	}
	return decision.VideoDecision == "copy"
}
