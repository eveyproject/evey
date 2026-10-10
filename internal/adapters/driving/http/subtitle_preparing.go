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
// Retry-After: the clients poll and show "preparing" in the meantime.

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
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

// subtitleFlights records when each shared extraction started, so a poll can
// be told how long is left. Keyed like the singleflight.
var subtitleFlights sync.Map

// subtitleFlightStarted reports whether an extraction for key is running and
// since when.
func subtitleFlightStarted(key string) (time.Time, bool) {
	v, ok := subtitleFlights.Load(key)
	if !ok {
		return time.Time{}, false
	}
	t, _ := v.(time.Time)
	return t, true
}

// subtitleRetryAfter is what the client is told to wait before asking again:
// what is left of the estimate, bounded.
func subtitleRetryAfter(estimate time.Duration, started time.Time) time.Duration {
	left := estimate
	if !started.IsZero() {
		left = estimate - time.Since(started)
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
// the header (a <track> element cannot; a fetch can).
func writeSubtitlePreparing(w http.ResponseWriter, r *http.Request, baseURL string, retryAfter time.Duration) {
	secs := max(int(retryAfter.Round(time.Second)/time.Second), 1)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	setCORSHeaders(w, r, baseURL)
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(map[string]any{"state": "preparing", "retryAfterSec": secs}); err != nil {
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
