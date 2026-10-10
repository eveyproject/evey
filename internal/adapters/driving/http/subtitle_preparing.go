package httpapi

// subtitle_preparing.go - answering a subtitle request that cannot be served
// within a few seconds with "it is being prepared" instead of a hung request.
//
// Without a block index, extracting a track walks the container: 4 min 42 s
// measured on an 85 GB UHD remux on a local disk (14.6 GiB actually read; the
// video is skipped by seek, the audio is not), ~10 min paced behind a playing
// stream. The handler used to hold the request open the whole time. Behind the
// shipped Caddy that worked only because the default transport has no read
// timeout; behind an nginx with its 60 s default the viewer got a 504 - and
// every client showed nothing, retried nothing, while the extraction went on
// filling a cache nobody would ask for again (measured 2026-10-10).
//
// So the extraction is started detached and the request is answered 202 with
// Retry-After: the clients poll and show "preparing" in the meantime. The
// delay is first an estimate, then - once the walk has moved - what is left
// at the pace actually observed.

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

// subtitlePreparingThreshold is the estimated extraction time past which the
// request is answered 202 rather than held open.
const subtitlePreparingThreshold = 20 * time.Second

// subtitleIdleReadRate is the walk's effective read rate when no stream is
// being served, used to estimate an extraction without an index. Deduced from
// three cold measurements on a local disk (55 MB/s on a seek-heavy UHD remux,
// 120-134 MB/s on 1080p files read through); a network mount is slower, which
// errs toward answering 202 - the harmless side.
const subtitleIdleReadRate = 100 << 20

// subtitleIndexedBlockCost is the estimated cost of one indexed block: one
// seek and one small read. Measured 1 ms warm and 9.5 ms cold per block on a
// local disk; the middle keeps a 3000-cue track under the threshold warm and
// over it cold.
const subtitleIndexedBlockCost = 3 * time.Millisecond

// subtitleRetryAfterMin/Max bound what the client is told to wait: never a
// hammering poll, never a wait longer than the extraction itself is likely to.
const (
	subtitleRetryAfterMin = 2 * time.Second
	subtitleRetryAfterMax = 30 * time.Second
)

// subtitleProgressFloor is how far the walk must have gone before its own pace
// is trusted over the estimate: the first percent is the container head and
// the window warming up, not the cruising rate.
const subtitleProgressFloor = 0.02

// estimateSubtitleExtraction is how long producing one track is expected to
// take. With an index the cost is per block; without, the whole file is read
// at the idle rate - or the paced rate while a stream is being served, which is
// when the viewer is actually waiting.
func estimateSubtitleExtraction(filePath string, indexedBlocks int, indexed bool) time.Duration {
	if indexed {
		return time.Duration(indexedBlocks) * subtitleIndexedBlockCost
	}
	st, err := os.Stat(filePath)
	if err != nil || st.Size() <= 0 {
		return 0
	}
	rate := float64(subtitleIdleReadRate)
	if subtitleReadPacer.playbackActive(time.Now()) {
		rate = float64(subtitleReadRateWhilePlaying)
	}
	return time.Duration(float64(st.Size()) / rate * float64(time.Second))
}

// subtitleFlight is one running extraction as the polls see it: when it
// started and how far into the file it has read. The walk visits the container
// front to back (and an indexed read seeks forward through it), so the highest
// offset reached over the file size IS the fraction done - bytes read would
// not be, since the video is skipped.
//
// Only the LAST file handle the extraction opened reports, from zero: mkvgo
// opens the container once for its metadata - and that reader ends at the
// Cues, at the very end of the file - then again for the walk. Measured on
// the first two real trials: the metadata read made a flight look 100 % done
// within a second, first by reporting late, then by having reported first.
type subtitleFlight struct {
	started time.Time
	size    int64
	maxOff  atomic.Int64
	// handles counts the files opened so far; the newest one is the walk.
	handles atomic.Int64
}

// open registers one more handle and returns its id; only the newest reports,
// and from zero: whatever the previous handle reached (the metadata reader
// ends at the Cues, at the very end of the file) says nothing about the walk.
func (f *subtitleFlight) open() int64 {
	if f == nil {
		return 0
	}
	id := f.handles.Add(1)
	f.maxOff.Store(0)
	return id
}

// advance records that handle h has reached off. Ignored for a superseded
// handle; monotonic for the current one, so a seek back to re-read a header
// does not make the walk look younger.
func (f *subtitleFlight) advance(h, off int64) {
	if f == nil || h != f.handles.Load() {
		return
	}
	for {
		cur := f.maxOff.Load()
		if off <= cur || f.maxOff.CompareAndSwap(cur, off) {
			return
		}
	}
}

// progress is the fraction of the file reached, 0..1 (0 when unknown).
func (f *subtitleFlight) progress() float64 {
	if f == nil || f.size <= 0 {
		return 0
	}
	p := float64(f.maxOff.Load()) / float64(f.size)
	if p > 1 {
		p = 1
	}
	return p
}

// subtitleFlightCtxKey carries the flight to the reader the extraction opens,
// through the work context: the FS wrapper reports its offsets to it.
type subtitleFlightCtxKey struct{}

func subtitleFlightFromContext(ctx context.Context) *subtitleFlight {
	f, _ := ctx.Value(subtitleFlightCtxKey{}).(*subtitleFlight)
	return f
}

// subtitleFlights records each running shared extraction, so a poll can be told
// how long is left. Keyed like the singleflight.
var subtitleFlights sync.Map

// subtitleFlightStarted reports whether an extraction for key is running, and
// the flight itself.
func subtitleFlightStarted(key string) (*subtitleFlight, bool) {
	v, ok := subtitleFlights.Load(key)
	if !ok {
		return nil, false
	}
	f, _ := v.(*subtitleFlight)
	return f, f != nil
}

// launchSharedSubtitleExtraction starts (or joins) the one extraction for key
// and returns its result channel without waiting on it - the detached form of
// runSharedSubtitleExtraction, for a request answered 202. The work runs on a
// context no request can cancel, with its own deadline: it fills a cache that
// outlives every request. See runSharedSubtitleExtraction for why.
func launchSharedSubtitleExtraction(ctx context.Context, key, filePath string, work func(context.Context) ([]byte, error)) <-chan singleflight.Result {
	return subtitleExtractions.DoChan(key, func() (any, error) {
		fl := &subtitleFlight{started: time.Now()}
		if st, err := os.Stat(filePath); err == nil {
			fl.size = st.Size()
		}
		subtitleFlights.Store(key, fl)
		defer subtitleFlights.Delete(key)
		workCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), subtitleExtractionDeadline(filePath))
		defer cancel()
		// The reader the extraction opens reports its offsets to the flight
		// (subtitle_read_pacer.go), which is what the polls' Retry-After is
		// computed from once the walk has moved.
		return work(context.WithValue(workCtx, subtitleFlightCtxKey{}, fl))
	})
}

// subtitleRetryAfter is what the client is told to wait before asking again.
// For a flight that has moved past the floor, what is left at the pace
// observed so far; for a fresh or barely started one, what is left of the
// estimate. Bounded either way.
func subtitleRetryAfter(estimate time.Duration, fl *subtitleFlight) time.Duration {
	left := estimate
	if fl != nil {
		elapsed := time.Since(fl.started)
		if p := fl.progress(); p >= subtitleProgressFloor {
			left = time.Duration(float64(elapsed) * (1 - p) / p)
		} else {
			left = estimate - elapsed
		}
	}
	if left < subtitleRetryAfterMin {
		return subtitleRetryAfterMin
	}
	if left > subtitleRetryAfterMax {
		return subtitleRetryAfterMax
	}
	return left.Round(time.Second)
}

// writeSubtitlePreparing answers 202: the track is being produced, ask again
// after Retry-After. The body repeats the delay for clients that cannot read
// the header (a <track> element cannot; a fetch can), and the progress when a
// flight is under way.
func writeSubtitlePreparing(w http.ResponseWriter, r *http.Request, baseURL string, retryAfter time.Duration, fl *subtitleFlight) {
	secs := int(retryAfter.Round(time.Second) / time.Second)
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	setCORSHeaders(w, r, baseURL)
	w.WriteHeader(http.StatusAccepted)
	body := map[string]any{"state": "preparing", "retryAfterSec": secs}
	if fl != nil {
		body["progress"] = float64(int(fl.progress()*1000)) / 1000
		body["elapsedSec"] = int(time.Since(fl.started) / time.Second)
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.WarnContext(r.Context(), "subtitle: preparing response write failed", slog.Any("err", err))
	}
}

// writeSubtitleFailed answers 422 with the memorised reason. The reason is a
// decoder's message, not viewer prose; the clients show their own wording and
// keep this for the logs.
func writeSubtitleFailed(w http.ResponseWriter, r *http.Request, baseURL, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	setCORSHeaders(w, r, baseURL)
	w.WriteHeader(http.StatusUnprocessableEntity)
	if err := json.NewEncoder(w).Encode(map[string]any{"message": "subtitle not available", "reason": reason}); err != nil {
		slog.WarnContext(r.Context(), "subtitle: failure response write failed", slog.Any("err", err))
	}
}
