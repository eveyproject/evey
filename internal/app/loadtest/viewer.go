package loadtest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Real Evey response contract for capacity/readiness backpressure (mirrors
// internal/adapters/driving/http/transcode_handler_vod.go): a 503 carries
// this header naming which condition applies.
const (
	headerEveyError       = "X-Evey-Error"
	errCodeTooManyStreams = "too_many_streams"
	errCodeFileNotReady   = "file_not_ready"
)

// headerServedBy names which subsystem actually served an HLS resource
// (master/variant/segment). An mkvgo-typed viewer requires it on the master
// playlist (eligibility) and, as a defense in depth, on the first segment.
const (
	headerServedBy = "X-Evey-Served-By"
	servedByMkvgo  = "mkvgo"
)

// headerLoadTest marks every request of a simulated viewer, so the server
// skips the work a real playback start buys for later (the subtitle prewarm:
// a full read of the film nobody will watch). Mirrors
// internal/adapters/driving/http/transcode_handler_vod.go.
const headerLoadTest = "X-Evey-Load-Test"

// maxConsecutiveViewerFailures is how many fetch failures in a row (outside
// admission/auth signals, which are handled separately) turn a viewer into a
// StopError verdict.
const maxConsecutiveViewerFailures = 3

// burstSegments is how many segments a viewer fetches back-to-back on
// startup, mirroring a real player filling its initial buffer.
const burstSegments = 3

type fetchOutcome int

const (
	outcomeOK fetchOutcome = iota
	outcomeTokenExpired
	outcomeAdmissionFull
	// outcomeServerStalled: the request timed out with no answer at all. A
	// server that stops answering IS a saturated server - the most emphatic
	// capacity signal there is - so this must never be filed as a harness error.
	outcomeServerStalled
	outcomeOtherFail
)

// runViewer simulates one real player: master playlist, median variant,
// buffer-filling burst, then real-time-paced segment fetches until ctx is
// cancelled. It never panics the run: any panic is recovered and reported as
// a viewer error.
func (r *Runner) runViewer(ctx context.Context, cfg Config, tierAtSpawn int, vt viewerType, asg assignment, forked bool, events chan<- viewerEvent, wg *sync.WaitGroup) {
	defer wg.Done()
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Warn("loadtest viewer recovered from panic", "panic", fmt.Sprint(rec))
			sendEvent(ctx, events, viewerEvent{kind: evViewerError, vt: vt, slot: tierAtSpawn, detail: "viewer panic"})
		}
	}()

	masterURL := buildMasterURL(cfg, vt, asg.media, forked, tierAtSpawn)
	masterBase, parseErr := url.Parse(masterURL)
	if parseErr != nil {
		sendEvent(ctx, events, viewerEvent{kind: evViewerError, vt: vt, slot: tierAtSpawn, detail: "invalid master url"})
		return
	}

	status, header, body, _, err := r.fetchWithRetry(ctx, masterURL, cfg.Token, true)
	switch classify(status, header, err) {
	case outcomeTokenExpired:
		sendEvent(ctx, events, viewerEvent{kind: evTokenExpired, vt: vt, slot: tierAtSpawn, detail: "401 on master playlist"})
		return
	case outcomeAdmissionFull:
		sendEvent(ctx, events, viewerEvent{kind: evAdmissionFull, vt: vt, slot: tierAtSpawn, detail: "server refused the stream (503) on the master playlist"})
		return
	case outcomeServerStalled:
		sendEvent(ctx, events, viewerEvent{kind: evAdmissionFull, vt: vt, slot: tierAtSpawn, detail: "server stopped answering: the master playlist timed out"})
		return
	case outcomeOtherFail:
		sendEvent(ctx, events, viewerEvent{kind: evViewerError, vt: vt, slot: tierAtSpawn, detail: fmt.Sprintf("master playlist fetch failed (status=%d, err=%v)", status, err)})
		return
	}

	// mkvgo eligibility: the master playlist itself must have been packaged
	// on demand by mkvgo. If not, this candidate isn't part of the "mkvgo
	// alone" measurement - abandon it cleanly, no error, no data point; the
	// run loop retries the same slot against the next distinct candidate.
	if vt == viewerTypeMkvgo && header.Get(headerServedBy) != servedByMkvgo {
		sendEvent(ctx, events, viewerEvent{kind: evCandidateSkipped, vt: vt, slot: tierAtSpawn, mediaID: asg.media.ID, detail: "master not served by mkvgo"})
		return
	}

	master, err := parseMasterPlaylist(body, masterBase)
	if err != nil {
		sendEvent(ctx, events, viewerEvent{kind: evViewerError, vt: vt, slot: tierAtSpawn, detail: "unparsable master playlist"})
		return
	}
	chosen := pickMedianVariant(master.variants)

	status, header, body, _, err = r.fetchWithRetry(ctx, chosen.uri.String(), cfg.Token, true)
	switch classify(status, header, err) {
	case outcomeTokenExpired:
		sendEvent(ctx, events, viewerEvent{kind: evTokenExpired, vt: vt, slot: tierAtSpawn, detail: "401 on variant playlist"})
		return
	case outcomeAdmissionFull:
		sendEvent(ctx, events, viewerEvent{kind: evAdmissionFull, vt: vt, slot: tierAtSpawn, detail: "server refused the stream (503) on the variant playlist"})
		return
	case outcomeServerStalled:
		sendEvent(ctx, events, viewerEvent{kind: evAdmissionFull, vt: vt, slot: tierAtSpawn, detail: "server stopped answering: the variant playlist timed out"})
		return
	case outcomeOtherFail:
		sendEvent(ctx, events, viewerEvent{kind: evViewerError, vt: vt, slot: tierAtSpawn, detail: fmt.Sprintf("variant playlist fetch failed (status=%d, err=%v)", status, err)})
		return
	}

	targetMs, segments, err := parseVariantPlaylist(body, chosen.uri)
	if err != nil {
		sendEvent(ctx, events, viewerEvent{kind: evViewerError, vt: vt, slot: tierAtSpawn, detail: "unparsable variant playlist"})
		return
	}
	sendEvent(ctx, events, viewerEvent{kind: evTargetDuration, vt: vt, slot: tierAtSpawn, ms: targetMs})
	segDurs := segmentDurationsMs(body)

	// The AUDIO the player will actually listen to.
	//
	// Evey's direct-play path DEMUXES the source: the video and each audio track
	// are separate renditions, on separate playlists. A real player therefore
	// fetches TWO segments for every moment of playback - and the server, to
	// answer them, has to produce both from the same window of the source. A
	// simulated viewer that only ever pulled the video was costing the server
	// roughly half of what a real one does, and never exercised that path at all:
	// the run was measuring a client nobody runs.
	var audioSegments []*url.URL
	if a, ok := pickAudioRendition(master.audio); ok {
		st, hdr, ab, _, aerr := r.fetchWithRetry(ctx, a.uri.String(), cfg.Token, true)
		if classify(st, hdr, aerr) == outcomeOK {
			if _, segs, perr := parseVariantPlaylist(ab, a.uri); perr == nil {
				audioSegments = segs
			}
		}
		// A failure here is not fatal: the viewer still exercises the video path,
		// and the run says what it measured rather than dying on a rendition.
	}

	// Start where the pool placed this viewer. Two viewers on one title read
	// different offsets, so neither is served the other's bytes from the page
	// cache - which is what makes a title reusable without faking the numbers.
	idx := startSegmentIndex(asg.startFrac, len(segments))
	consecutiveFail := 0
	fetchNext := func(isFirst bool) (stop bool) {
		segURL := segments[idx]
		segMs := 0
		if idx < len(segDurs) {
			segMs = segDurs[idx]
		}
		// The audio of the SAME moment, pulled alongside - which is what a player
		// does, and what makes the server produce both renditions from one read of
		// the window. hls.js issues the two requests in parallel, so this one goes
		// out first and is collected after the video's, rather than serialising
		// them into a slower, gentler load than the real thing.
		var audioDone chan int64
		if len(audioSegments) > 0 {
			audioURL := audioSegments[idx%len(audioSegments)]
			audioDone = make(chan int64, 1)
			go func() {
				// This detached goroutine is NOT covered by runViewer's own
				// defer/recover (that guards only the goroutine that installed it);
				// an unrecovered panic here would crash the whole server. Recover
				// and unblock the receiver with a failure sentinel.
				defer func() {
					if rec := recover(); rec != nil {
						slog.ErrorContext(ctx, "load-test audio fetch goroutine panicked", slog.Any("panic", rec))
						audioDone <- -1
					}
				}()
				st, _, _, ms, aerr := r.fetchWithRetry(ctx, audioURL.String(), cfg.Token, false)
				if st == http.StatusOK && aerr == nil {
					audioDone <- int64(ms)
					return
				}
				audioDone <- -1
			}()
		}
		idx = (idx + 1) % len(segments)

		status, header, _, elapsedMs, err := r.fetchWithRetry(ctx, segURL.String(), cfg.Token, false)
		if audioDone != nil {
			// Wait for the audio too: a viewer is not "served" until it has both.
			// Its bytes are already counted (fetchOnce tallies every response), so
			// this only holds the pacing honest.
			<-audioDone
		}
		outcome := classify(status, header, err)
		// Defense in depth: the first segment of an mkvgo-typed viewer must
		// also carry the header, or a silent fallback to transcode on the
		// segment path would flatter the "mkvgo alone" measurement.
		if outcome == outcomeOK && isFirst && vt == viewerTypeMkvgo && header.Get(headerServedBy) != servedByMkvgo {
			outcome = outcomeOtherFail
		}
		switch outcome {
		case outcomeOK:
			consecutiveFail = 0
			if isFirst {
				sendEvent(ctx, events, viewerEvent{kind: evTTFF, vt: vt, slot: tierAtSpawn, ms: elapsedMs, servedBy: header.Get(headerServedBy)})
			} else {
				sendEvent(ctx, events, viewerEvent{kind: evSegment, vt: vt, slot: tierAtSpawn, ms: elapsedMs, segMs: segMs})
			}
			return false
		case outcomeTokenExpired:
			sendEvent(ctx, events, viewerEvent{kind: evTokenExpired, vt: vt, slot: tierAtSpawn, detail: "401 on segment fetch"})
			return true
		case outcomeAdmissionFull:
			sendEvent(ctx, events, viewerEvent{kind: evAdmissionFull, vt: vt, slot: tierAtSpawn, detail: "server refused the stream (503) on a segment"})
			return true
		case outcomeServerStalled:
			sendEvent(ctx, events, viewerEvent{kind: evAdmissionFull, vt: vt, slot: tierAtSpawn, detail: "server stopped answering: a segment fetch timed out"})
			return true
		default:
			consecutiveFail++
			if consecutiveFail >= maxConsecutiveViewerFailures {
				sendEvent(ctx, events, viewerEvent{kind: evViewerError, vt: vt, slot: tierAtSpawn, detail: fmt.Sprintf("%d consecutive segment fetch failures", consecutiveFail)})
				return true
			}
			return false
		}
	}

	burst := min(burstSegments, len(segments))
	for i := range burst {
		if ctx.Err() != nil {
			return
		}
		if fetchNext(i == 0) {
			return
		}
	}

	pace := time.Duration(targetMs) * time.Millisecond
	if pace <= 0 {
		pace = segmentDefaultMs * time.Millisecond
	}
	ticker := time.NewTicker(pace)
	defer ticker.Stop()

	// seekState is an LCG (next = (state*1103515245+12345) & 0x7fffffff)
	// seeded by this viewer's own ramp slot, never math/rand or time-based,
	// so two runs of the same config always jump to the same sequence of
	// positions - the load test's own output must be reproducible even
	// though it is simulating "everyone scrubs the timeline" chaos.
	seekState := int64(tierAtSpawn)
	ticks := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ticks++
			if cfg.Mode == ModeSeek && ticks%seekEveryTicks == 0 {
				seekState = (seekState*1103515245 + 12345) & 0x7fffffff
				idx = int(seekState % int64(len(segments)))
				// The first fetch after a jump is the seek latency itself -
				// recorded as a TTFF sample, exactly like a fresh player
				// would report "time to first frame" after a scrub.
				if fetchNext(true) {
					return
				}
				continue
			}
			if fetchNext(false) {
				return
			}
		}
	}
}

// seekEveryTicks is how many regular pace ticks (i.e. segment-durations)
// elapse between two seeks in ModeSeek: "every 10x segment-duration".
const seekEveryTicks = 10

// startSegmentIndex maps a start fraction of the title onto a segment index,
// clamped so the last segment is never a valid start (a viewer placed there
// would immediately wrap around to the beginning of the file).
func startSegmentIndex(startFrac float64, segCount int) int {
	if segCount <= 1 || startFrac <= 0 {
		return 0
	}
	idx := int(startFrac * float64(segCount))
	return min(max(idx, 0), segCount-1)
}

// buildMasterURL constructs the master playlist request a real player would
// issue. forked viewers (a transcode-typed viewer reusing a media item
// because the library ran out of distinct candidates) add an audioOffsetMs
// so the server's decision hash forks into a distinct session instead of
// sharing the original viewer's.
func buildMasterURL(cfg Config, vt viewerType, media MediaCandidate, forked bool, forkTag int) string {
	u := strings.TrimRight(cfg.BaseURL, "/") + "/api/v1/media/" + media.ID + "/hls/master.m3u8"
	q := url.Values{}
	switch vt {
	case viewerTypeTranscode:
		q.Set("vd", "transcode")
		q.Set("ad", "transcode")
	case viewerTypeMkvgo:
		// A direct-play user has a CAPABLE client by definition - that is what
		// makes them direct-play. Without these the negotiation assumes the
		// default web profile (AAC-only audio), an AC3/DTS title needs an
		// audio transcode, and the full mkvgo packager is never eligible: the
		// scenario would measure the library's audio codecs, not the packager.
		q.Set("videoCodecs", "h264,hevc,av1")
		q.Set("audioCodecs", "aac,ac3,eac3,dts,truehd,opus,flac,mp3")
		q.Set("containers", "mp4,mkv,webm")
		q.Set("hdr", "1")
	default:
		// A client viewer declares what its player declares, and the server
		// decides its path on that, as it would for the real one.
		if p, ok := clientProfileOf(vt); ok {
			for k, v := range p.params {
				q[k] = append([]string(nil), v...)
			}
		}
	}
	if forked {
		q.Set("audioOffsetMs", strconv.Itoa(forkTag))
	}
	if len(q) == 0 {
		return u
	}
	return u + "?" + q.Encode()
}

// classify maps a fetch's status/header/error into the outcome the run loop
// or viewer cares about.
//
// EVERY 503 is a capacity answer, not a harness error. The server says "I can't
// serve this right now" - whether it names the reason (too_many_streams), throttles
// the profile, or just gives up under load. Demanding a particular header before
// believing it meant a real refusal read as "an error interrupted the test", which
// threw away the very measurement the run existed to take. (file_not_ready is the
// one exception, and it never reaches here: fetchWithRetry retries it first.)
func classify(status int, header http.Header, err error) fetchOutcome {
	if err != nil {
		// A request that timed out waiting for headers means the server never
		// answered. That is not a failure of the test - it is the loudest
		// possible statement that the box has run out of capacity, and the run
		// must report it as such instead of "an error interrupted the test".
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return outcomeServerStalled
		}
		return outcomeOtherFail
	}
	switch status {
	case http.StatusOK:
		return outcomeOK
	case http.StatusUnauthorized:
		return outcomeTokenExpired
	case http.StatusServiceUnavailable, http.StatusTooManyRequests:
		return outcomeAdmissionFull
	default:
		return outcomeOtherFail
	}
}

// fetchWithRetry performs one GET, retrying exactly once (after the server's
// Retry-After) when the response is a file_not_ready 503 - a transient
// "still preparing the segment" signal, not a capacity or auth verdict.
func (r *Runner) fetchWithRetry(ctx context.Context, rawURL, token string, keepBody bool) (status int, header http.Header, body []byte, elapsedMs int, err error) {
	status, header, body, elapsedMs, err = r.fetchOnce(ctx, rawURL, token, keepBody)
	if err == nil && status == http.StatusServiceUnavailable && header.Get(headerEveyError) == errCodeFileNotReady {
		wait := retryAfter(header)
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return status, header, body, elapsedMs, ctx.Err()
		}
		status, header, body, elapsedMs, err = r.fetchOnce(ctx, rawURL, token, keepBody)
	}
	return status, header, body, elapsedMs, err
}

// retryAfter parses a Retry-After header in seconds, defaulting to 1s.
func retryAfter(header http.Header) time.Duration {
	raw := header.Get("Retry-After")
	if raw == "" {
		return time.Second
	}
	secs, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || secs < 0 {
		return time.Second
	}
	return time.Duration(secs) * time.Second
}

func (r *Runner) fetchOnce(ctx context.Context, rawURL, token string, keepBody bool) (status int, header http.Header, body []byte, elapsedMs int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, nil, nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set(headerLoadTest, "1")

	start := time.Now()
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return 0, nil, nil, int(time.Since(start).Milliseconds()), err
	}
	defer func() { _ = resp.Body.Close() }()

	// A segment is only counted, never inspected: buffering it would make this
	// harness hold every in-flight segment of every viewer inside the server
	// process, and that memory is then reported as the server's own.
	var b []byte
	var n int64
	var readErr error
	if keepBody {
		b, readErr = io.ReadAll(resp.Body)
		n = int64(len(b))
	} else {
		n, readErr = io.Copy(io.Discard, resp.Body)
	}
	elapsedMs = int(time.Since(start).Milliseconds())
	// Every byte read here is a byte the server had to produce and push, so
	// this is where served throughput is tallied - playlists included, exactly
	// like a real player's traffic.
	r.servedBytes.Add(n)
	if readErr != nil {
		return resp.StatusCode, resp.Header, nil, elapsedMs, readErr
	}
	return resp.StatusCode, resp.Header, b, elapsedMs, nil
}

// sendEvent forwards ev to the run loop, giving up only if ctx has already
// ended (the run loop is buffered generously, so this practically never
// blocks in steady state).
//
// A viewer whose context is already cancelled says NOTHING. It has been shed -
// by the bisection narrowing the audience, or by the run ending - and its
// in-flight requests are failing precisely BECAUSE it was cancelled. Reporting
// those failures would turn a routine shed into a StopError and kill the run,
// or worse, blame the server for a latency the harness itself caused.
func sendEvent(ctx context.Context, events chan<- viewerEvent, ev viewerEvent) {
	if ctx.Err() != nil {
		return
	}
	select {
	case events <- ev:
	case <-ctx.Done():
	}
}
