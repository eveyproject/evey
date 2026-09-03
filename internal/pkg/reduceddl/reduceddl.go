// Package reduceddl holds the vocabulary of the size-reduced (720p) download:
// the naming of the slices on disk, the readiness signal, and the sizing
// arithmetic. It lives outside both the ffmpeg adapter that PRODUCES slices
// and the HTTP handler that SERVES them, because both need the same answers
// and neither may depend on the other.
//
// The sizing constants are shared for a blunt reason: the number the phone
// shows the user before committing to an hour of NAS work and the number the
// encoder is capped at have to be the same, or the estimate is a lie.
package reduceddl

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	// TargetHeight is the one reduced quality on offer ("Original + 720p,
	// rien d'autre"): a ladder buys nothing here and multiplies the NAS work.
	TargetHeight = 720

	// VideoKbps / AudioKbps drive the encoder's VBV cap AND EstimatedBytes.
	VideoKbps = 2500
	AudioKbps = 128

	// SliceSec is the delivery granularity: what a dropped connection costs,
	// and what the server holds on disk at once. Ten minutes is ~190 MB at the
	// bitrates above - cheap to re-fetch, and a feature film is ~12 slices
	// rather than hundreds.
	SliceSec = 600

	// KeyframeSec is the keyframe grid of the reduced encode, and it is
	// deliberately NOT SliceSec.
	//
	// The segment muxer cuts at the first keyframe at or after
	// (previous slice start + SliceSec), so keyframes have to be dense enough
	// that one always turns up shortly after a boundary. Putting keyframes
	// ONLY on the slice grid looks tidier and fails: containers start a few
	// tens of milliseconds off zero, that offset shifts every boundary, the
	// one keyframe on the grid lands just before it, and the muxer sails past
	// to the next one - producing slices of twice the intended length (seen
	// on the first integration run: 20 s and 5 s instead of 10/10/5).
	//
	// Two seconds also happens to be what makes the DOWNLOADED file pleasant
	// to seek in, which a ten-minute GOP very much would not be.
	KeyframeSec = 2
)

// SlicePattern is the ffmpeg segment-muxer output template; SliceName is the
// same name resolved for one index. Five digits covers a 5-day film.
const SlicePattern = "slice-%05d.mkv"

func SliceName(index int) string { return fmt.Sprintf(SlicePattern, index) }

// listPrefix/listSuffix bracket the segment muxer's own list file. A line
// appears in it when a slice is CLOSED, which is the only trustworthy
// readiness signal: the slice file merely existing means ffmpeg is still
// writing into it.
//
// The name carries the run's FIRST slice index because a production that died
// halfway is resumed by a second ffmpeg process, and the muxer truncates the
// list path it is given. One list per run, each naming where it started, keeps
// the readiness count reconstructible across any number of resumes.
const (
	listPrefix = "slices-"
	listSuffix = ".csv"
)

func ListName(startSlice int) string {
	return fmt.Sprintf("%s%05d%s", listPrefix, startSlice, listSuffix)
}

// ListStart recovers the first slice index a list file covers. ok is false for
// any other name, so a stray file in the job dir cannot inflate the count.
func ListStart(name string) (int, bool) {
	body, ok := strings.CutPrefix(name, listPrefix)
	if !ok {
		return 0, false
	}
	body, ok = strings.CutSuffix(body, listSuffix)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(body)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// Spec is everything the encoder needs that is not a path. It lives here so
// the HTTP handler can build one without importing the ffmpeg adapter (the
// port/adapter direction the rest of the server keeps).
type Spec struct {
	// Source picture, for sizing the decoder (zero = worst case).
	SourceWidth, SourceHeight int
	SourceTenBit              bool
	// Width of the delivered video, even, already clamped to the source (the
	// height follows from the aspect ratio: -2). Width rather than height
	// because the tonemap filter folds its downscale in as scale=W:-2.
	Width int
	// SliceSec is the slice duration; keyframes are forced on its multiples so
	// the segment muxer cuts exactly there.
	SliceSec int
	// StartSlice > 0 resumes a production that died partway. This is the ONLY
	// case where two encoder instances contribute to one film, so it is also
	// the only place a single-boundary A/V rounding can appear (bounded by one
	// AAC frame, ~21 ms, non-recurring).
	StartSlice int
	// StartSec is where in the source that resume picks up: the END time the
	// muxer recorded for the last finished slice. It is NOT StartSlice*SliceSec
	// - slices run a little long (the cut waits for the next keyframe), so the
	// arithmetic guess drifts further out with every slice and would leave a
	// gap or a repeat in the middle of the film.
	StartSec float64
	// ToneMap re-grades HDR to SDR bt709. A 720p offline copy is watched on a
	// phone screen: shipping untonemapped HDR would look washed out on every
	// device that cannot render it.
	ToneMap bool
}

// EstimatedBytes is what the reduced variant will weigh, from its duration
// alone - the figure shown to the user before they commit.
func EstimatedBytes(durationMs int64) int64 {
	if durationMs <= 0 {
		return 0
	}
	return durationMs * int64(VideoKbps+AudioKbps) * 1000 / 8 / 1000
}

// SliceCount is how many slices a film of this length yields. The phone needs
// it up front to show "slice 3 of 12" and to know when it is done.
func SliceCount(durationMs int64) int {
	if durationMs <= 0 {
		return 0
	}
	sec := (durationMs + 999) / 1000
	return int((sec + SliceSec - 1) / SliceSec)
}

// WidthFor returns the delivery width for a source of the given dimensions, or
// 0 when a reduced variant would be pointless (already at or below the target
// height, or unknown size). Even: yuv420p chroma cannot do odd.
func WidthFor(srcWidth, srcHeight int) int {
	if srcWidth <= 0 || srcHeight <= 0 || srcHeight <= TargetHeight {
		return 0
	}
	w := srcWidth * TargetHeight / srcHeight
	w -= w % 2
	if w < 2 {
		return 0
	}
	return w
}

// ClosedSlices reads the segment muxer's list and reports how many slices
// ffmpeg has FINISHED writing, plus the source time the last of them ends at.
// The csv shape is "<filename>,<start>,<end>" per line.
//
// A trailing partial line is ignored rather than trusted: the list is read
// while the muxer appends to it, and counting a half-flushed line would
// advertise a slice ffmpeg is still writing into - the phone would then join a
// truncated Matroska onto its file.
//
// endSec is what a resume seeks to; see Spec.StartSec for why the arithmetic
// guess is not good enough. Returns 0, 0 for a nil/absent reader, which is
// exactly what an absent list means.
func ClosedSlices(r io.Reader) (n int, endSec float64) {
	if r == nil {
		return 0, 0
	}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			continue // half-flushed write, not a slice
		}
		end, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
		if err != nil {
			continue // ditto: the end time was still being written
		}
		n++
		endSec = end
	}
	return n, endSec
}
