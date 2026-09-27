package library

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"sync"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

// Visual widening of an audio-confirmed intro. A title sequence keeps its pictures from
// one episode to the next while its mix changes (dub, remix, dialogue over the theme):
// Billions S06 shares 12s of identical audio but 20s of identical pictures (measured
// 2026-09-27). Around each edge of the audio window, low-res frames are compared with
// two other carriers at their audio alignment, and the edge moves outward while the
// pictures stay the same. It only widens a confirmed intro, so it cannot create one.
const (
	edgeFPS          = 4.0
	edgeOuterSec     = 15.0 // how far past an edge the pictures may extend the intro
	edgeInnerSec     = 5.0  // inside the audio window: aligns the two episodes' frames
	edgeMatchMaxDiff = 12.0 // mean |grey diff| per pixel to WIDEN: same frame 0-2, a cut ~8, other shots 35+
	// edgeRejectMaxCorr: in a season whose audio signature is ambiguous, an intro is
	// dropped when even its best neighbour edge correlates less. Correlation ignores
	// contrast, which the mean difference does not: two different dark scenes differ by
	// only ~16. Dev 2026-09-27: false positives 0.35-0.65, real ambiguous intros 0.86+
	// (a thin sample of ~20 windows).
	edgeRejectMaxCorr = 0.75
	// ambiguousSigSec and ambiguousSpreadSec define the audio profile the picture check
	// may veto: a short signature at scattered positions. All 15 dev false positives had
	// it; long or stable signatures (Stranger Things 62s, Billions 14s at +-10s) never do.
	ambiguousSigSec    = 20.0
	ambiguousSpreadSec = 60.0
	edgeFlatMaxStd     = 3.0 // a frame this flat (black, solid) proves nothing; dark titles sit at 5 (Billions)
	edgeBridgeFrames   = 4   // up to 1s of flat or differing frames may be crossed
	edgeMaxShift       = 12  // +-3s: audio and picture offsets differ by up to ~2s across releases (Billions S06E01/E06)
	edgeCacheMax       = 512 // windows kept in memory (a window is ~46 KB)
)

func frameDiff(a, b []byte) float64 {
	n := min(len(a), len(b))
	if n == 0 {
		return math.MaxFloat64
	}
	var sum int
	for i := range n {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		sum += d
	}
	return float64(sum) / float64(n)
}

// frameCorr is the Pearson correlation of two frames: 1 for the same picture at any
// contrast, near 0 for unrelated ones.
func frameCorr(a, b []byte) float64 {
	n := min(len(a), len(b))
	if n == 0 {
		return 0
	}
	var sa, sb float64
	for i := range n {
		sa += float64(a[i])
		sb += float64(b[i])
	}
	ma, mb := sa/float64(n), sb/float64(n)
	var cov, va, vb float64
	for i := range n {
		x, y := float64(a[i])-ma, float64(b[i])-mb
		cov += x * y
		va += x * x
		vb += y * y
	}
	if va == 0 || vb == 0 {
		return 0
	}
	return cov / math.Sqrt(va*vb)
}

func frameStd(a []byte) float64 {
	if len(a) == 0 {
		return 0
	}
	var sum, sq float64
	for _, v := range a {
		f := float64(v)
		sum += f
		sq += f * f
	}
	m := sum / float64(len(a))
	return math.Sqrt(max(0, sq/float64(len(a))-m*m))
}

// widenFrames returns how many frames the edge at ownPivot moves outward (dir -1 for a
// start edge, +1 for an end edge) given the matching window of a reference episode
// whose same edge sits at refPivot. The two windows are first aligned on the frames
// just inside the edge, on textured frames only: two dark passages always look alike.
// diff is the best aligned mean difference there and corr the best mean correlation;
// judged is false when too few frames are textured to tell (a title that fades in from
// black). Only diff <= edgeMatchMaxDiff widens.
func widenFrames(own, ref [][]byte, ownPivot, refPivot, dir, inner int) (frames int, diff, corr float64, judged bool) {
	pair := func(i, k int) ([]byte, []byte, bool) {
		j := i - ownPivot + refPivot + k
		if i < 0 || i >= len(own) || j < 0 || j >= len(ref) {
			return nil, nil, false
		}
		return own[i], ref[j], true
	}
	// A start edge's pivot frame is the intro's first; an end edge's is the first after it.
	inside := func(t int) int { // t-th frame inside the intro, counted from the edge
		if dir < 0 {
			return ownPivot + t
		}
		return ownPivot - 1 - t
	}
	outside := func(step int) int { // step-th frame outside the intro (step >= 1)
		if dir < 0 {
			return ownPivot - step
		}
		return ownPivot + step - 1
	}
	textured := 0
	for t := range inner {
		if i := inside(t); i >= 0 && i < len(own) && frameStd(own[i]) >= edgeFlatMaxStd {
			textured++
		}
	}
	if textured < inner/2 {
		return 0, 0, 0, false
	}
	bestK, bestDiff, bestCorr := 0, math.MaxFloat64, -1.0
	for k := -edgeMaxShift; k <= edgeMaxShift; k++ {
		var sum, csum float64
		n := 0
		for t := range inner {
			if a, b, ok := pair(inside(t), k); ok && frameStd(a) >= edgeFlatMaxStd {
				sum += frameDiff(a, b)
				csum += frameCorr(a, b)
				n++
			}
		}
		if n < inner/2 {
			continue
		}
		if sum/float64(n) < bestDiff {
			bestK, bestDiff = k, sum/float64(n)
		}
		bestCorr = max(bestCorr, csum/float64(n))
	}
	if bestDiff > edgeMatchMaxDiff {
		return 0, bestDiff, bestCorr, true
	}
	last, gap := 0, 0
	for step := 1; ; step++ {
		a, b, ok := pair(outside(step), bestK)
		if !ok {
			break
		}
		if frameStd(a) >= edgeFlatMaxStd && frameDiff(a, b) <= edgeMatchMaxDiff {
			last, gap = step, 0
			continue
		}
		gap++
		if gap > edgeBridgeFrames {
			break
		}
	}
	return last, bestDiff, bestCorr, true
}

// edgeWindow is the decode window around one edge and the edge's index inside it.
func edgeWindow(edgeSec float64, isStart bool) (startSec, durSec float64, pivot int) {
	before, after := edgeInnerSec, edgeOuterSec
	if isStart {
		before, after = edgeOuterSec, edgeInnerSec
	}
	startSec = max(0, edgeSec-before)
	return startSec, edgeSec + after - startSec, int(math.Round((edgeSec - startSec) * edgeFPS))
}

type edgeFrameCache struct {
	mu sync.Mutex
	m  map[string][][]byte
}

func (s *IntroDetectionService) edgeFrames(ctx context.Context, path string, startSec, durSec float64) ([][]byte, error) {
	key := fmt.Sprintf("%s|%.3f|%.3f", path, startSec, durSec)
	c := &s.edgeCache
	c.mu.Lock()
	if f, ok := c.m[key]; ok {
		c.mu.Unlock()
		return f, nil
	}
	c.mu.Unlock()
	f, err := s.detector.GrayThumbnails(ctx, path, startSec, durSec, edgeFPS)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.m == nil || len(c.m) >= edgeCacheMax {
		c.m = make(map[string][][]byte)
	}
	c.m[key] = f
	c.mu.Unlock()
	return f, nil
}

// widenHintsByFrames widens every carrier's hint in place, and drops a hint whose pictures
// match none of the neighbours it could be compared with. It widens comparing it with up to four
// neighbours in season order and keeping the largest extension: one release that differs
// (crop, grade, MULTI vs FRENCH) must not veto the others, and a frame only counts when
// textured AND identical at the aligned instant. A carrier without a usable neighbour,
// or whose own frames fail to decode, keeps its window.
func (s *IntroDetectionService) widenHintsByFrames(ctx context.Context, order []string, paths map[string]string, hints map[string]domainlibrary.MediaSegment, ambiguous bool) {
	if s.detector == nil || len(order) < 2 {
		return
	}
	inner := int(edgeInnerSec * edgeFPS)
	// Every comparison aligns on the AUDIO windows: widening one carrier first would
	// shift its neighbours' alignment by the seconds it gained.
	audio := make(map[string]domainlibrary.MediaSegment, len(hints))
	maps.Copy(audio, hints)
	widened, rejected := 0, 0
	for i, id := range order {
		h, ok := audio[id]
		if !ok {
			continue
		}
		var refs []string
		for _, d := range []int{-1, 1, -2, 2} {
			if j := i + d; j >= 0 && j < len(order) {
				if _, ok := audio[order[j]]; ok {
					refs = append(refs, order[j])
				}
			}
		}
		if len(refs) == 0 {
			continue
		}
		// grow returns the edge's extension and the best correlation any neighbour showed
		// just inside the edge (-1 when none could be judged).
		grow := func(isStart bool) (ext int, best float64) {
			best = -1
			edge := func(seg domainlibrary.MediaSegment) float64 {
				if isStart {
					return float64(seg.StartMs) / 1000
				}
				return float64(seg.EndMs) / 1000
			}
			dir := 1
			if isStart {
				dir = -1
			}
			ws, wd, ownPivot := edgeWindow(edge(h), isStart)
			own, err := s.edgeFrames(ctx, paths[id], ws, wd)
			if err != nil || len(own) == 0 {
				return 0, best
			}
			for _, r := range refs {
				rs, rd, refPivot := edgeWindow(edge(audio[r]), isStart)
				ref, err := s.edgeFrames(ctx, paths[r], rs, rd)
				if err != nil || len(ref) == 0 {
					continue
				}
				n, _, corr, judged := widenFrames(own, ref, ownPivot, refPivot, dir, inner)
				if judged {
					best = max(best, corr)
					ext = max(ext, n)
				}
			}
			return ext, best
		}
		before, bestStart := grow(true)
		after, bestEnd := grow(false)
		// A recurring cue under different scenes (City of Blood, Unfamiliar) or a network
		// card before them passes the audio cluster; its pictures match no neighbour.
		if best := max(bestStart, bestEnd); ambiguous && best >= 0 && best < edgeRejectMaxCorr {
			slog.InfoContext(ctx, "intro_detection: cluster intro rejected, pictures differ from every neighbour",
				slog.String("media_id", id), slog.Int64("start_ms", h.StartMs), slog.Int64("end_ms", h.EndMs))
			delete(hints, id)
			rejected++
			continue
		}
		if before == 0 && after == 0 {
			continue
		}
		newStart := max(0, h.StartMs-int64(float64(before)/edgeFPS*1000))
		newEnd := h.EndMs + int64(float64(after)/edgeFPS*1000)
		slog.DebugContext(ctx, "intro_detection.visual_widen",
			slog.String("media_id", id), slog.Int("refs", len(refs)), slog.Int64("start_ms", h.StartMs), slog.Int64("end_ms", h.EndMs),
			slog.Int64("widened_start_ms", newStart), slog.Int64("widened_end_ms", newEnd))
		h.StartMs, h.EndMs = newStart, newEnd
		hints[id] = h
		widened++
	}
	slog.InfoContext(ctx, "intro_detection: visual edge widening", slog.Int("carriers", len(order)), slog.Int("widened", widened), slog.Int("rejected", rejected))
}

// ambiguousSignature reports the audio profile a picture check may veto: a signature
// shorter than ambiguousSigSec whose carriers' start positions spread (population std)
// beyond ambiguousSpreadSec.
func ambiguousSignature(sigSec float64, startsSec []float64) bool {
	if sigSec >= ambiguousSigSec || len(startsSec) < 2 {
		return false
	}
	var sum, sq float64
	for _, s := range startsSec {
		sum += s
		sq += s * s
	}
	m := sum / float64(len(startsSec))
	return math.Sqrt(max(0, sq/float64(len(startsSec))-m*m)) > ambiguousSpreadSec
}
