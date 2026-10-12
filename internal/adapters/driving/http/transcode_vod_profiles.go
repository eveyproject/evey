package httpapi

// transcode_vod_profiles.go - the per-request facts the adapter still reads
// beside the resolved plan (the audio rendition shape). The codec, backend,
// route, ladder and class are projections of PlaybackPlan (Resolve,
// transcode_vod_plan.go): the adapter derives none of them on its own.

import (
	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
)

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
