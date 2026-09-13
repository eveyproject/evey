package ffmpeg

import "testing"

type rc = struct {
	c    hwCandidate
	peak float64
	fits bool
}

// The backend policy: proved+correct is decided before this (only certified
// candidates reach it); here a candidate that FITS the pools beats one that
// does not, then the lower peak-pool cost wins, then the taller ceiling, then a
// stable name. No fixed VAAPI/QSV/NVENC order.
func TestBetterCandidate_Policy(t *testing.T) {
	fitCheap := rc{c: hwCandidate{backend: "qsv", best: 2160}, peak: 0.4, fits: true}
	fitDear := rc{c: hwCandidate{backend: "vaapi", best: 2160}, peak: 0.9, fits: true}
	noFit := rc{c: hwCandidate{backend: "nvenc", best: 2160}, peak: 1.4, fits: false}

	if !betterCandidate(fitCheap, fitDear) {
		t.Error("the cheaper fitting candidate must win (QSV over VAAPI here, not detector order)")
	}
	if betterCandidate(fitDear, fitCheap) {
		t.Error("the dearer candidate must not win")
	}
	if !betterCandidate(fitDear, noFit) {
		t.Error("any fitting candidate beats a non-fitting one")
	}
	// Equal fit and peak: taller ceiling wins, then name.
	a := rc{c: hwCandidate{backend: "vaapi", best: 2160}, peak: 0.5, fits: true}
	b := rc{c: hwCandidate{backend: "qsv", best: 1440}, peak: 0.5, fits: true}
	if !betterCandidate(a, b) {
		t.Error("equal peak: the taller ceiling wins")
	}
	c := rc{c: hwCandidate{backend: "nvenc", best: 2160}, peak: 0.5, fits: true}
	if !betterCandidate(c, a) {
		t.Error("equal peak and ceiling: the lexically-smaller backend wins (determinism)")
	}
}
