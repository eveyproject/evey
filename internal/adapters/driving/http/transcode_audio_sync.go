package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	pkgcache "github.com/evey-project/evey/internal/pkg/cache"
)

// transcode_audio_sync.go answers one question for the audio-rendition path:
// by how much must this session's audio be shifted to land on the video's
// timeline?
//
// The audio rendition is its own stream and always starts at 0. Whether that
// is correct depends on WHICH muxer packaged the video for the same session:
//
//   - mkvgo (the on-demand packager): presents its first frame at 0 -> no shift.
//   - ffmpeg `-c:v copy` in fMP4 (the fallback when mkvgo is not eligible or
//     PlanHLS refuses the source): presents at has_b_frames/fps, because MP4
//     forbids a negative DTS -> the audio runs EARLY by exactly that much.
//
// Measured on real files: 42 ms for 1 B-frame, 83 ms for 2 at 23.976 fps -
// past the ~45 ms audibility threshold for EARLY audio (ITU-R BT.1359), which
// is how the operator heard it. Proven live in prod: masters served with
// mkvgo_video=false alongside a separate audio rendition.

// videoCompositionDelaySec returns the shift the audio rendition needs for this
// request, or 0 when the video is packaged by mkvgo (nothing to compensate).
// Best-effort throughout: any probe or plan failure yields 0, which is exactly
// the pre-fix behaviour - a rendition must never fail to start because a probe
// did.
// The shift applies to exactly ONE configuration, and the guards below exist to
// keep it there:
//
//   - the video must be a `-c:v copy` rung. The probe reads the SOURCE's
//     has_b_frames, which describes the copied frames verbatim. A re-encoded
//     video's presentation delay is Evey's own encoder's, not the source's, so
//     the probe would be measuring the wrong thing;
//   - mkvgo must not be the one serving that copy, since it presents at 0;
//   - the packager must be wired at all, so "mkvgo did not take it" is a real
//     answer rather than an artefact of an unwired handler.
//
// Anything else returns 0. Shifting audio against a video that was NOT shifted
// would create the very desync this removes, so every uncertainty resolves to
// today's behaviour.
func (h *hlsVODServe) videoCompositionDelaySec(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) float64 {
	if h.compositionDelayProbe == nil || h.onDemand == nil || item == nil || item.File == nil || item.File.Path == "" {
		return 0
	}
	decision := h.makeDecision(r.Context(), streams, containerOf(item), sourcePathOf(item), r)
	if !videoCopyEligible(decision, streams) {
		return 0 // re-encoded video: the delay is the encoder's, not the source's
	}
	if h.planBuildsForSource(r, mediaID, item, streams) {
		return 0 // mkvgo packages it, and mkvgo presents at 0
	}
	return h.compositionDelayProbe(r.Context(), item.File.Path)
}

// planBuildsForSource reports whether mp4.PlanHLS succeeds for this source,
// memoised by file identity.
//
// The memo is not a performance tweak, it is a CORRECTNESS guard. The delay is
// hashed into the audio fingerprint, so a value that flipped mid-playback would
// fork the segment cache and respawn ffmpeg under the viewer. Plan builds fail
// for two very different reasons:
//
//   - the source genuinely cannot be packaged (`invalid VINT`, a codec CMAF
//     cannot carry): a property of the FILE, stable, worth remembering;
//   - the client went away mid-build (`context canceled`): says nothing about
//     the file and is common in prod (~2 in 3 of the observed failures).
//
// Only the first kind is recorded. A cancellation is reported as "packages
// fine" for this request and left unmemoised, so the answer stays stable
// instead of oscillating with client behaviour.
func (h *hlsVODServe) planBuildsForSource(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) bool {
	key, ok := sourceIdentity(item.File.Path)
	if ok {
		if v, hit := planBuildMemo.Get(key); hit {
			return v
		}
	}
	_, err := h.buildPlan(r, mediaID, item, streams)
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

// planBuildMemoTTL bounds how long a per-file verdict is trusted. Matched to
// onDemandPlanTTL (internal/adapters/driven/mkvgo/ondemand.go, 30 min): this
// memo answers "does mkvgo's own plan cache have (or build) a plan for this
// identity", so keeping the two in lock-step means the memo is never trusted
// past what it describes. It also self-heals a refusal caused by something
// OTHER than the file's own bytes (a transient stat/read hiccup that is not a
// context cancellation) within the same order of magnitude as the underlying
// plan cache, instead of holding it wrong for the rest of the process.
const planBuildMemoTTL = 30 * time.Minute

// planBuildMemo caches the per-file "does PlanHLS build?" verdict. Keyed by
// path+size+mtime like the ffmpeg adapter's own probe caches, so a repaired or
// replaced file re-proves itself via a fresh key rather than inheriting a
// stale refusal. A bounded TTL cache (not a bare sync.Map): a NAS running for
// weeks against a library whose files keep getting replaced/re-encoded would
// otherwise accumulate one entry per stale identity forever.
var planBuildMemo, _ = pkgcache.New[bool](0)

// resetPlanBuildMemoCache swaps planBuildMemo for one capped at maxSize.
// Test-only (unexported, unused outside *_test.go): production always wants
// the package default; a test proving the cache stays bounded wants a small,
// fast-to-fill cap instead of inserting thousands of real entries.
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
