package httpapi

import (
	"net/http"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
)

// bakeClientInputs copies the client's declared capabilities from the request
// onto a ManifestInput, so every URI the manifest emits re-asks for the same
// client the master was decided for. ONE function for the master and for
// every child playlist: the audio playlist used to re-bake its own subset and
// dropped fmp4AudioPassthrough, so its segments re-decided without the opt-in
// and came out AAC under a master that advertised the copied codec (Opus,
// measured 2026-09-29). TestChildURIsCarryTheClientProfile pins that
// parseClientCapabilities reads the same profile off a child URI as off the
// request that produced it.
func bakeClientInputs(r *http.Request, in *apptranscoding.ManifestInput) {
	if r == nil {
		return
	}
	in.VideoCodecs = splitCSVQuery(r, "videoCodecs")
	in.AudioCodecs = domaintranscoding.WithoutBrokenPlayerCodecs(r.UserAgent(), splitCSVQuery(r, "audioCodecs"))
	in.Containers = splitCSVQuery(r, "containers")
	in.FMP4AudioPassthrough = splitCSVQuery(r, "fmp4AudioPassthrough")
	in.PixelFormats = domaintranscoding.NormalizePixelFormats(splitCSVQuery(r, "pixelFormats"))
	in.HDRCapable = hdrQuerySupported(r)
	in.MaxAudioChannels = maxAudioChannelsQuery(r)
	in.ScreenHeight = parseIntQuery(r, "screenHeight", 0)
	in.MaxHeight = parseIntQuery(r, "maxHeight", 0)
	in.MaxH264Level = parseIntQuery(r, "maxH264Level", 0)
	in.NetworkType = networkTypeQuery(r)
}
