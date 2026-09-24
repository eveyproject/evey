package httpapi

import (
	"net/http"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

// renditionTimelineShiftSec is 0: every video Evey packages presents at its
// source time (mkvgo, and ffmpeg fMP4 since presentAtSourceTimeArgs), so its
// audio renditions need no shift. The shift they carried before rode an empty
// edit that neither MSE nor ExoPlayer applies (measured 2026-09-24).
const renditionTimelineShiftSec = 0.0

// audioRenditionCopyCodec names the source codec the ffmpeg audio rendition
// of trackIdx may carry VERBATIM for this client, or "" when it must be
// re-encoded to AAC. The rendition used to re-encode every track no matter
// what: an AAC or AC-3 track the decision had judged "copy" cascaded through
// the AAC encoder the moment the video transcoded, and the stream panel kept
// saying copy. Copy needs the decision's copy verdict for THIS track (codec
// decodable, channels under the client's and the profile's ceilings), a
// codec the fMP4 packaging carries for this client, and no filter on the
// samples (loudnorm, a resync offset).
func (h *hlsVODServe) audioRenditionCopyCodec(r *http.Request, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, trackIdx int) string {
	if item == nil || item.File == nil || audioOffsetMsQuery(r) != 0 {
		return ""
	}
	if h.audioNormalizeGetter != nil && h.audioNormalizeGetter(r.Context()) {
		return ""
	}
	if h.decisionForTrack(r.Context(), item, streams, r, trackIdx).AudioDecision != "copy" {
		return ""
	}
	codec := sourceAudioCodec(streams, trackIdx)
	client := parseClientCapabilities(r, hdrQuerySupported(r))
	if codec == "" || !apptranscoding.AudioCopyCodecAllowedFor(codec, client) {
		return ""
	}
	return codec
}
