package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	pkgcache "github.com/evey-project/evey/internal/pkg/cache"
)

// transcode_audio_sync.go: by how much must a session's audio rendition be
// shifted to land on the video's timeline? The rendition starts at 0; mkvgo
// presents the video's first frame at 0 too, but ffmpeg `-c:v copy` in fMP4
// presents it at has_b_frames/fps (MP4 forbids a negative DTS), so the audio
// runs early by that much (42 ms for 1 B-frame at 23.976 fps, audible past ~45 ms).

// videoCompositionDelaySec returns the shift the audio rendition needs, or 0
// when nothing must be compensated. Only a `-c:v copy` rung NOT served by mkvgo
// qualifies: the probe reads the SOURCE's has_b_frames, which describes copied
// frames verbatim, not a re-encode. Every uncertainty resolves to 0 (shifting
// against an unshifted video would create the very desync this removes).
func (h *hlsVODServe) videoCompositionDelaySec(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) float64 {
	if h.compositionDelayProbe == nil || h.onDemand == nil || item == nil || item.File == nil || item.File.Path == "" {
		return 0
	}
	decision := h.decisionFor(r.Context(), item, streams, r)
	if decision.VideoDecision == "transcode" {
		return h.transcodeCompositionDelaySec(r, mediaID, item, streams)
	}
	if !videoCopyEligible(decision, streams) {
		return 0
	}
	if h.planBuildsForSource(r, mediaID, item, streams) {
		return 0 // mkvgo packages it, and mkvgo presents at 0
	}
	return h.compositionDelayProbe(r.Context(), item.File.Path)
}

// transcodeCompositionDelaySec is the shift for a TRANSCODED video: the
// encoder's reorder frames (probed once per encoder configuration) over the
// source frame rate. 0 when either probe is unavailable or fails.
func (h *hlsVODServe) transcodeCompositionDelaySec(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) float64 {
	if h.encoderReorderProbe == nil || h.frameRateProbe == nil {
		return 0
	}
	rp, err := h.probePlan(r, mediaID, item, streams)
	if err != nil || rp.IsCopy {
		return 0
	}
	fps := h.frameRateProbe(r.Context(), item.File.Path)
	frames, ok := h.encoderReorderProbe(r.Context(), rp.Plan.VideoCodec, rp.Plan.HWAccel, rp.FP.HWDevice, rp.FP.HWFeatures)
	if !ok || fps <= 0 || frames <= 0 {
		return 0
	}
	return float64(frames) / fps
}

// planBuildsForSource reports whether mp4.PlanHLS succeeds for this source,
// memoised by file identity. The memo is a CORRECTNESS guard: the delay is
// hashed into the audio fingerprint, so a value flipping mid-playback would
// fork the cache under the viewer. Only a failure that is a property of the
// FILE is recorded; a client cancellation mid-build says nothing about it and
// is reported as "packages fine", unmemoised.
func (h *hlsVODServe) planBuildsForSource(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) bool {
	key, ok := sourceIdentity(item.File.Path)
	if ok {
		if v, hit := planBuildMemo.Get(key); hit {
			return v
		}
	}
	_, err := h.buildPlan(r, mediaID, item, streams, h.planOf(r, mediaID, item, streams))
	if err == nil {
		if ok {
			planBuildMemo.Set(key, true, planBuildMemoTTL)
		}
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true // tells us nothing about the file - do not memoise, do not shift
	}
	if ok {
		planBuildMemo.Set(key, false, planBuildMemoTTL)
	}
	return false
}

// planBuildMemoTTL matches onDemandPlanTTL (mkvgo adapter, 30 min): the memo
// answers "does mkvgo's plan cache have a plan for this identity", so the two
// stay in lock-step and a transient refusal self-heals.
const planBuildMemoTTL = 30 * time.Minute

// planBuildMemo caches the per-file "does PlanHLS build?" verdict, keyed by
// path+size+mtime so a replaced file re-proves itself. Bounded TTL cache: a
// library whose files keep getting replaced would otherwise grow it forever.
var planBuildMemo, _ = pkgcache.New[bool](0)

// resetPlanBuildMemoCache swaps planBuildMemo for one capped at maxSize (test-only).
func resetPlanBuildMemoCache(maxSize int) {
	planBuildMemo, _ = pkgcache.NewWithMaxSize[bool](0, maxSize)
}

func sourceIdentity(path string) (string, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("%s|%d|%d", path, fi.Size(), fi.ModTime().UnixNano()), true
}

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
	client := parseClientCapabilities(r, r.URL.Query().Get("hdr") == "1")
	if codec == "" || !apptranscoding.AudioCopyCodecAllowedFor(codec, client) {
		return ""
	}
	return codec
}
