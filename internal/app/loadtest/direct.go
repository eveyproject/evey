package loadtest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// servedByDirect is the path of a viewer whose player reads the raw file
// (GET /media/{id}/stream): what every client does on a direct-play verdict.
const servedByDirect = "direct"

// playMethodDirectPlay is the stream-info verdict that hands the raw file over.
const playMethodDirectPlay = "PLAY_METHOD_DIRECT_PLAY"

// directDefaultBitrateBps paces a direct-play reader when neither the probe nor
// the file's size and duration give its bitrate.
const directDefaultBitrateBps = 8_000_000

// directHeaderBytes is what a player reads at the head of the file before it
// seeks to its start position (Matroska's segment info, tracks and Cues
// pointer, an MP4's moov when it leads).
const directHeaderBytes = 256 << 10

// streamInfoParams maps a client profile's master query onto stream-info's,
// the question every shipped player asks before it picks a URL.
var streamInfoParams = map[string]string{ //nolint:gosec // G101: query parameter names, no credential ("Passthrough" is an audio mode)
	"videoCodecs":          "supportedCodecs",
	"audioCodecs":          "supportedAudioCodecs",
	"containers":           "supportedContainers",
	"fmp4AudioPassthrough": "fmp4AudioPassthrough",
	"maxAudioChannels":     "maxAudioChannels",
}

func streamInfoURL(cfg Config, mediaID string, p clientProfile) string {
	q := url.Values{}
	for from, to := range streamInfoParams {
		for _, v := range p.params[from] {
			if from == "maxAudioChannels" {
				q.Add(to, v)
				continue
			}
			for item := range strings.SplitSeq(v, ",") {
				q.Add(to, item)
			}
		}
	}
	if p.params.Get("hdr") == "1" {
		q.Set("supportsHdr", "true")
	}
	if p.params.Get("wasmAudio") == "1" {
		q.Set("wasmAudio", "true")
	}
	return strings.TrimRight(cfg.BaseURL, "/") + "/api/v1/playback/" + url.PathEscape(mediaID) + "/stream-info?" + q.Encode()
}

// askPlayMethod asks stream-info what a client viewer's player would be told.
// stop is true when the viewer must end (the event is already sent).
func (r *Runner) askPlayMethod(ctx context.Context, cfg Config, vt viewerType, slot int, media MediaCandidate, p clientProfile, events chan<- viewerEvent) (direct, stop bool) {
	status, header, body, _, err := r.fetchWithRetry(ctx, streamInfoURL(cfg, media.ID, p), cfg.Token, true)
	switch classify(status, header, err) {
	case outcomeOK:
	case outcomeTokenExpired:
		sendEvent(ctx, events, viewerEvent{kind: evTokenExpired, vt: vt, slot: slot, detail: "401 on stream-info"})
		return false, true
	case outcomeAdmissionFull, outcomeServerStalled:
		sendEvent(ctx, events, viewerEvent{kind: evAdmissionFull, vt: vt, slot: slot, detail: "server refused or stalled on stream-info"})
		return false, true
	default:
		sendEvent(ctx, events, viewerEvent{kind: evViewerError, vt: vt, slot: slot, detail: fmt.Sprintf("stream-info failed (status=%d, err=%v)", status, err)})
		return false, true
	}
	var info struct {
		PlayMethod string `json:"playMethod"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		sendEvent(ctx, events, viewerEvent{kind: evViewerError, vt: vt, slot: slot, detail: "unparsable stream-info"})
		return false, true
	}
	return info.PlayMethod == playMethodDirectPlay, false
}

// playDirect reads the raw file the way a player does on a direct play: the
// head first, then from its start position one segment's worth of bytes per
// segment of playback, wrapping at the end, with a jump every seekEveryTicks
// in ModeSeek.
func (r *Runner) playDirect(ctx context.Context, cfg Config, vt viewerType, slot int, asg assignment, events chan<- viewerEvent) {
	u := strings.TrimRight(cfg.BaseURL, "/") + "/api/v1/media/" + url.PathEscape(asg.media.ID) + "/stream"
	consecutiveFail := 0
	// fetch reads [off, off+n) and reports it; stop is true when the viewer ends.
	fetch := func(off, n int64, first bool, segMs int) (size int64, stop bool) {
		status, header, elapsedMs, err := r.fetchRange(ctx, u, cfg.Token, off, n)
		switch classify(status, header, err) {
		case outcomeOK:
			consecutiveFail = 0
			if first {
				sendEvent(ctx, events, viewerEvent{kind: evTTFF, vt: vt, slot: slot, ms: elapsedMs, servedBy: servedByDirect})
			} else {
				sendEvent(ctx, events, viewerEvent{kind: evSegment, vt: vt, slot: slot, ms: elapsedMs, segMs: segMs})
			}
			return contentRangeTotal(header), false
		case outcomeTokenExpired:
			sendEvent(ctx, events, viewerEvent{kind: evTokenExpired, vt: vt, slot: slot, detail: "401 on the raw file"})
		case outcomeAdmissionFull:
			sendEvent(ctx, events, viewerEvent{kind: evAdmissionFull, vt: vt, slot: slot, detail: "server refused the raw file (503)"})
		case outcomeServerStalled:
			sendEvent(ctx, events, viewerEvent{kind: evAdmissionFull, vt: vt, slot: slot, detail: "server stopped answering: a raw-file read timed out"})
		default:
			consecutiveFail++
			if consecutiveFail < maxConsecutiveViewerFailures {
				return 0, false
			}
			sendEvent(ctx, events, viewerEvent{kind: evViewerError, vt: vt, slot: slot, detail: fmt.Sprintf("%d consecutive raw-file read failures", consecutiveFail)})
		}
		return 0, true
	}

	size, stop := fetch(0, directHeaderBytes, true, 0)
	if stop {
		return
	}
	if size <= 0 {
		sendEvent(ctx, events, viewerEvent{kind: evViewerError, vt: vt, slot: slot, detail: "raw file answered without a Content-Range size"})
		return
	}
	bps := asg.media.BitrateBps
	if bps <= 0 && asg.media.DurationSec > 0 {
		bps = size * 8 / int64(asg.media.DurationSec)
	}
	if bps <= 0 {
		bps = directDefaultBitrateBps
	}
	segMs := segmentDefaultMs
	chunk := max(bps/8*int64(segMs)/1000, 1)
	// A start past the last chunk would wrap at once (see startSegmentIndex).
	off := min(int64(asg.startFrac*float64(size)), max(size-chunk, 0))
	next := func(first bool) bool {
		_, stop := fetch(off, min(chunk, size-off), first, segMs)
		off += chunk
		if off >= size {
			off = 0
		}
		return stop
	}
	for i := range burstSegments {
		if ctx.Err() != nil || next(i == 0) {
			return
		}
	}
	ticker := time.NewTicker(time.Duration(segMs) * time.Millisecond)
	defer ticker.Stop()
	seekState := int64(slot)
	for ticks := 1; ; ticks++ {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		first := false
		if cfg.Mode == ModeSeek && ticks%seekEveryTicks == 0 {
			seekState = (seekState*1103515245 + 12345) & 0x7fffffff
			off = seekState % size
			first = true
		}
		if next(first) {
			return
		}
	}
}

// contentRangeTotal reads the size from "bytes a-b/total", 0 when absent.
func contentRangeTotal(h http.Header) int64 {
	cr := h.Get("Content-Range")
	_, total, ok := strings.Cut(cr, "/")
	if !ok {
		return 0
	}
	n, err := strconv.ParseInt(total, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
