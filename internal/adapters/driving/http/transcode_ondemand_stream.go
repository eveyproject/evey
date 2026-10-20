package httpapi

// transcode_ondemand_stream.go - serving a packaged media segment from the
// source as it goes out, instead of assembling it whole first.
//
// mkvgo 0.42 can write a Matroska source's segment straight from the file
// through one 256 KiB buffer (plan.Open); until then every segment request
// held its whole segment in memory for as long as the client took to read it,
// which made the server's RSS a function of how slow the audience was (mkvgo's
// measurement: 16 clients at 2 Mb/s on a UHD remux, 940 MiB → 28 MiB). The
// bytes are the same; what changes is that they are never all in hand, so the
// ETag is weak and a read error after the status line cannot be reported in
// the body: the connection is cut instead, which is the one thing a player
// never mistakes for a whole segment.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

// isPlanPlaylist is the names servePlanResourceData rewrites before sending
// (the sub-resource URIs get the packager version and the token baked in):
// those need their bytes in hand and stay on the buffered path.
func isPlanPlaylist(name string) bool {
	return strings.HasSuffix(name, ".m3u8") || strings.HasSuffix(name, ".mpd")
}

// servePlanResourceStreamed writes one streamed resource, servedBy naming the
// path in X-Evey-Served-By (mkvgo, mkvgo-concat): exact Content-Length,
// a weak ETag, one Range at a time, HEAD honoured, and on a write error the
// connection cut short (http.ErrAbortHandler - net/http closes it without a
// final chunk and without logging).
func servePlanResourceStreamed(w http.ResponseWriter, r *http.Request, name string, res apptranscoding.OnDemandResource, servedBy string, span trace.Span) {
	defer closePlanResource(r.Context(), name, res)
	size := res.Size()
	etag := res.ETag()
	setCORSHeaders(w, r, "")
	w.Header().Set("X-Evey-Served-By", servedBy)
	w.Header().Set("ETag", etag)
	w.Header().Set("Accept-Ranges", "bytes")
	if inm := r.Header.Get("If-None-Match"); inm != "" && weakETagMatches(inm, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", res.ContentType())
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	start, n, status, ok := singleByteRange(r.Header.Get("Range"), size)
	if !ok {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
		span.SetStatus(codes.Error, "range not satisfiable")
		http.Error(w, `{"message":"range not satisfiable"}`, http.StatusRequestedRangeNotSatisfiable)
		return
	}
	if status == http.StatusPartialContent {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, start+n-1, size))
	}
	w.Header().Set("Content-Length", strconv.FormatInt(n, 10))
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := res.WriteRange(r.Context(), w, start, n); err != nil {
		// The status line is out and the body is short: a clean end here
		// would hand the player a truncated segment it takes for a whole one.
		span.RecordError(err)
		span.SetStatus(codes.Error, "streamed segment cut short: "+name)
		panic(http.ErrAbortHandler)
	}
}

// singleByteRange parses one "bytes=a-b" Range header against size and returns
// the span to write - always 0 <= start, 0 <= n, start+n <= size - with the
// status: 200 without a usable Range, 206 with one. ok is false only for a
// well-formed range that cannot be satisfied (RFC 9110 14.1.2: first byte at
// or past the end, or an empty suffix), which the caller answers 416. Anything
// malformed - letters, a negative number, more than one dash - is ignored and
// the resource served whole, as the RFC allows; a multi-range request is
// served whole too (no player sends one, multipart/byteranges is not worth its
// own code). Numbers too large for int64 count as past the end.
func singleByteRange(header string, size int64) (start, n int64, status int, ok bool) {
	whole := func() (int64, int64, int, bool) { return 0, size, http.StatusOK, true }
	spec, found := strings.CutPrefix(strings.TrimSpace(header), "bytes=")
	if !found || strings.Contains(spec, ",") {
		return whole()
	}
	a, b, dash := strings.Cut(spec, "-")
	if !dash {
		return whole()
	}
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	// Digits only on both sides: strconv would accept a sign, and "-5-10"
	// must not read as "suffix 5-10" or "from -5".
	digits := func(x string) bool {
		if x == "" {
			return false
		}
		for _, c := range x {
			if c < '0' || c > '9' {
				return false
			}
		}
		return true
	}
	// parse reads a non-negative decimal; too large for int64 saturates.
	parse := func(x string) int64 {
		v, err := strconv.ParseInt(x, 10, 64)
		if err != nil {
			return math.MaxInt64
		}
		return v
	}
	switch {
	case a == "" && b == "":
		return whole()
	case a == "": // suffix: the last b bytes
		if !digits(b) {
			return whole()
		}
		suffix := parse(b)
		if suffix == 0 {
			return 0, 0, 0, false
		}
		suffix = min(suffix, size)
		return size - suffix, suffix, http.StatusPartialContent, true
	}
	if !digits(a) || (b != "" && !digits(b)) {
		return whole()
	}
	first := parse(a)
	if first >= size {
		return 0, 0, 0, false
	}
	last := size - 1
	if b != "" {
		last = parse(b)
		if last < first {
			return whole() // malformed (last-pos < first-pos), not unsatisfiable
		}
		last = min(last, size-1)
	}
	return first, last - first + 1, http.StatusPartialContent, true
}

// weakETagMatches is If-None-Match against a weak tag: the W/ prefix on either
// side is ignored, "*" matches anything.
func weakETagMatches(header, etag string) bool {
	strip := func(s string) string { return strings.TrimPrefix(strings.TrimSpace(s), "W/") }
	want := strip(etag)
	for part := range strings.SplitSeq(header, ",") {
		if part = strings.TrimSpace(part); part == "*" || strip(part) == want {
			return true
		}
	}
	return false
}

// answerPlanResourceByStat answers a HEAD or a revalidation of a segment or an
// init from the plan's Stat (mkvgo 0.56) instead of opening it: Open counts
// the resource served and can take a segment a window walk kept for a viewer
// who has yet to ask for it. Reports whether it answered. It leaves to the
// normal path a plan without Stat, a playlist or a subtitle (Evey rewrites
// those before sending, so the plan's size is not theirs), a HEAD with a
// Range, a revalidation that does not match, and what only Open can size.
func answerPlanResourceByStat(w http.ResponseWriter, r *http.Request, plan any, name, servedBy string) bool {
	inm := r.Header.Get("If-None-Match")
	head := r.Method == http.MethodHead && r.Header.Get("Range") == ""
	if (!head && inm == "") || isPlanPlaylist(name) || strings.HasSuffix(name, ".vtt") {
		return false
	}
	st, ok := plan.(apptranscoding.OnDemandStater)
	if !ok {
		return false
	}
	size, etag, contentType, known, err := st.Stat(r.Context(), name)
	if err != nil || !known {
		return false
	}
	notModified := inm != "" && weakETagMatches(inm, etag)
	if !notModified && !head {
		return false
	}
	setCORSHeaders(w, r, "")
	w.Header().Set("X-Evey-Served-By", servedBy)
	w.Header().Set("ETag", etag)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if notModified {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.WriteHeader(http.StatusOK)
	return true
}

// planResourceErrorStatus is the status of a plan's refusal: a name it does not
// serve, a window inside a damaged region and a source replaced again after
// the replay are 404 (asking again changes nothing), anything else - a read of
// the source that failed - is a 502 a player retries.
func planResourceErrorStatus(err error) int {
	if errors.Is(err, apptranscoding.ErrNoResource) || errors.Is(err, apptranscoding.ErrSourceDamaged) ||
		errors.Is(err, apptranscoding.ErrSourceChanged) {
		return http.StatusNotFound
	}
	return http.StatusBadGateway
}

// writePlanResourceError answers a plan's refusal with planResourceErrorStatus.
func writePlanResourceError(w http.ResponseWriter, err error) {
	if status := planResourceErrorStatus(err); status != http.StatusNotFound {
		http.Error(w, `{"message":"the source could not be read"}`, status)
		return
	}
	http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
}

// closePlanResource releases a handle; mkvgo holds nothing open between
// writes, so a failure here is worth a line, never a status.
func closePlanResource(ctx context.Context, name string, res apptranscoding.OnDemandResource) {
	if err := res.Close(); err != nil {
		slog.WarnContext(ctx, "hls-vod: on-demand resource close failed", slog.String("resource", name), slog.Any("err", err))
	}
}
