package httpapi

// client_capabilities.go - the client's capabilities as the HLS requests
// declare them (?videoCodecs, ?audioCodecs, ?containers, ?hdr, ?maxHeight,
// ...): ONE reading shared by the master's decision, the URIs the manifests
// bake and the audio rendition's fingerprint, so a segment request re-derives
// the SAME decision the master was built from.

import (
	"net/http"
	"strconv"
	"strings"

	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// hdrQuerySupported reports whether the client advertised an HDR-capable
// display. Web and mobile send hdr=1; hdr=true is the older spelling some
// callers used - accepting only one of the two silently strips the capability
// and forces a tonemap for clients that decode HDR natively.
func hdrQuerySupported(r *http.Request) bool {
	v := r.URL.Query().Get("hdr")
	return v == "1" || v == "true"
}

func parseClientCapabilities(r *http.Request, hdrSupported bool) domaintranscoding.ClientProfile {
	client := domaintranscoding.DefaultWebProfile
	if v := r.URL.Query().Get("videoCodecs"); v != "" {
		client.VideoCodecs = strings.Split(v, ",")
	}
	if v := r.URL.Query().Get("audioCodecs"); v != "" {
		client.AudioCodecs = strings.Split(v, ",")
	}
	if v := r.URL.Query().Get("containers"); v != "" {
		client.Containers = strings.Split(v, ",")
	}
	// Audio codecs this client accepts as a RAW passthrough inside our fMP4
	// segments - deliberately not `audioCodecs`, which every web and iOS client
	// fills with mp3 whether or not it can take it in that container. This one
	// rides the segment URLs like the codec lists do, so a segment request
	// re-derives the SAME decision the master was built from.
	if v := r.URL.Query().Get("fmp4AudioPassthrough"); v != "" {
		client.FMP4AudioPassthrough = strings.Split(v, ",")
	}
	// Channel ceiling of the playback DEVICE (a 2.0 TV, a stereo-only cast
	// target...). Web players normally omit it: browsers decode multichannel
	// AAC and downmix to the output device themselves, so the profile default
	// applies. Clamped to a sane layout range; the decision additionally caps
	// AAC targets at 5.1 (aacMaxTargetChannels).
	if n := maxAudioChannelsQuery(r); n > 0 {
		client.MaxAudioChannels = n
	}
	if n := parseIntQuery(r, "maxH264Level", 0); n > 0 {
		client.MaxH264Level = n
	}
	// The device's H.264 decoder ceiling when it stops below ?maxHeight (a cast
	// receiver that plays 2160p HEVC and 1080p H.264).
	if n := parseIntQuery(r, "maxH264Height", 0); n >= 240 && n <= 8640 {
		client.MaxH264Height = n
	}
	// Height ceiling of the DECODER or of an explicit quality pick (mobile/TV
	// menu, cast receiver profile): the decision transcodes above it and the
	// ladder stops at it. Unlike ?screenHeight, which only trims the ladder.
	if n := parseIntQuery(r, "maxHeight", 0); n >= 240 && n <= 8640 {
		client.MaxHeight = n
	}
	if hdrSupported {
		client.SupportsHDR = true
	}
	return client
}

// maxAudioChannelsQuery is the ONE reading of the client's channel ceiling.
// The master's decision, the URIs the manifest bakes and the audio rendition's
// own fingerprint all derive from it: two readings that disagree serve a
// rendition the master never negotiated - exactly the 5.1 a cast receiver kept
// getting after asking for stereo (2026-08-08). Returns 0 when nothing
// usable was declared.
func maxAudioChannelsQuery(r *http.Request) int {
	if r == nil {
		return 0
	}
	if v := r.URL.Query().Get("maxAudioChannels"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 8 {
			return n
		}
	}
	return 0
}
