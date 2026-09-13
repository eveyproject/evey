package ffmpeg

import "testing"

// The backend policy RANKS certified candidates (only proved+correct ones reach
// it); it does NOT gate on pool fit - that is admission's job against the
// remaining pools. The cheaper (lower peak-pool cost) backend wins, then the
// taller ceiling, then a stable name. No fixed VAAPI/QSV/NVENC order.
func TestBetterCandidate_Policy(t *testing.T) {
	cheap := rankedCandidate{c: hwCandidate{backend: "qsv", best: 2160}, peak: 0.4}
	dear := rankedCandidate{c: hwCandidate{backend: "vaapi", best: 2160}, peak: 0.9}
	if !betterCandidate(cheap, dear) {
		t.Error("the cheaper candidate must win (QSV over VAAPI here, not detector order)")
	}
	if betterCandidate(dear, cheap) {
		t.Error("the dearer candidate must not win")
	}
	// A candidate that would be tight on an EMPTY pool is still ranked, never
	// eliminated here (admission decides fit against the remaining pools).
	tight := rankedCandidate{c: hwCandidate{backend: "nvenc", best: 2160}, peak: 1.4}
	looser := rankedCandidate{c: hwCandidate{backend: "vaapi", best: 2160}, peak: 0.9}
	if !betterCandidate(looser, tight) {
		t.Error("the looser candidate ranks first, but the tight one is not dropped")
	}
	// Equal peak: taller ceiling wins, then name.
	a := rankedCandidate{c: hwCandidate{backend: "vaapi", best: 2160}, peak: 0.5}
	b := rankedCandidate{c: hwCandidate{backend: "qsv", best: 1440}, peak: 0.5}
	if !betterCandidate(a, b) {
		t.Error("equal peak: the taller ceiling wins")
	}
	c := rankedCandidate{c: hwCandidate{backend: "nvenc", best: 2160}, peak: 0.5}
	if !betterCandidate(c, a) {
		t.Error("equal peak and ceiling: the lexically-smaller backend wins (determinism)")
	}
}
