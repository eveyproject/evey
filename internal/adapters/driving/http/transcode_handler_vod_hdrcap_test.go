package httpapi

// Tests for the HDR-aware ABR ladder cap in profilesForDecision.
//
// The cap is the safety valve that keeps a weak host real-time on 4K HDR:
// when software tone-mapping is required (hwUsable=false) and the content
// carries the "hdr_tonemapping_needed" reason, the ladder is capped to
// EncodeCapability.SoftwareHDRMaxHeight, which is lower than the normal
// SoftwareMaxHeight.  These tests lock in that behaviour so a refactor
// can't silently remove or invert the gate.

import (
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// hdrCapTestCapability returns a software-only EncodeCapability for a weak
// host: software ceiling 1080p, HDR software ceiling 480p, no HW backend.
// This drives the soft-HDR cap path (hwUsable=false for all decisions).
func hdrCapTestCapability() domaintranscoding.EncodeCapability {
	return domaintranscoding.EncodeCapability{
		SoftwareMaxHeight:    1080,
		SoftwareHDRMaxHeight: 480,
		HWMaxHeight:          0, // no HW
	}
}

// hdrCapHWCapability returns a capability that enables the HW path:
// NVENC h264 + hevc-decode + 10-bit VPP + HW ceiling 2160p.
// Used for case (c): hwUsable=true → the SoftwareHDRMaxHeight cap is bypassed.
func hdrCapHWCapability() domaintranscoding.EncodeCapability {
	return domaintranscoding.EncodeCapability{
		SoftwareMaxHeight:    1080,
		SoftwareHDRMaxHeight: 480,
		HWMaxHeight:          2160,
		HWBackend:            "nvenc",
		HWCodecs:             []string{"h264"},
		HWDecodeCodecs:       []string{"hevc", "hevc10"},
		HW10BitTranscode:     true, // allows the 10-bit source through the HW gate
	}
}

// build4KHDRStreams returns a slice with one video stream: 3840×2160,
// HEVC Main 10 (10-bit), mimicking a real 4K HDR source. The 4K source
// dimension ensures CapLadderToSource doesn't pre-trim the ladder before
// the HDR cap has a chance to act.
func build4KHDRStreams() []*domainlibrary.MediaStream {
	w, h, bitDepth := 3840, 2160, 10
	return []*domainlibrary.MediaStream{
		{
			StreamType: domainlibrary.StreamTypeVideo,
			Codec:      "hevc",
			Profile:    "Main 10",
			Width:      &w,
			Height:     &h,
			BitDepth:   &bitDepth,
		},
	}
}

// buildHDRDecision returns a PlaybackDecision that carries the
// hdr_tonemapping_needed reason, which gates the HDR cap.
func buildHDRDecision() domaintranscoding.PlaybackDecision {
	return domaintranscoding.PlaybackDecision{
		TranscodeReasons: []string{"hdr_tonemapping_needed"},
	}
}
