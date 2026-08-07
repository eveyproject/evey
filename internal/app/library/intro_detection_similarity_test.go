package library

import (
	"math"
	"testing"
)

// slidingChromaprint is the audio-fingerprint half of intro confirmation, and
// it had no test at all - pearsonImpl and slidingBestPearsonBandedImpl, its
// float-domain siblings, are covered in intro_detection_helpers_test.go, but
// the Chromaprint hamming match was never exercised. Found on 2026-08-08 by a
// first mutation-testing pass over internal/app/library (68.77% efficacy, 267
// uncovered mutants, 223 of them in the intro-detection files).
//
// What is asserted here is the SCALE and the SEARCH, both objectively
// checkable: similarity is 1 - hamming/(32*len(target)), so identical
// fingerprints score exactly 1, fully inverted ones exactly 0, and half the
// bits differing lands exactly on 0.5. Every audio threshold in the pipeline is
// read off that scale, so a drift at either end silently moves them all.
//
// No algorithm VERDICT is pinned here ("this intro starts at 42s"): the ground
// truth for that comes from user feedback, and freezing a judgement currently
// rated poor would make improving it harder rather than safer.

const chromaprintEpsilon = 1e-9

func TestSlidingChromaprint_ExactMatchScoresOneAtItsOffset(t *testing.T) {
	target := []uint32{0xDEADBEEF, 0x12345678}
	reference := []uint32{0x00000000, 0xFFFFFFFF, 0xDEADBEEF, 0x12345678, 0xAAAAAAAA}

	offset, similarity := slidingChromaprint(target, reference)

	if offset != 2 {
		t.Errorf("offset = %d, want 2", offset)
	}
	if math.Abs(similarity-1) > chromaprintEpsilon {
		t.Errorf("similarity = %v, want 1 for an exact match", similarity)
	}
}

func TestSlidingChromaprint_FullyInvertedBitsScoreZero(t *testing.T) {
	target := []uint32{0x00000000, 0x00000000}
	reference := []uint32{0xFFFFFFFF, 0xFFFFFFFF}

	_, similarity := slidingChromaprint(target, reference)

	if math.Abs(similarity) > chromaprintEpsilon {
		t.Errorf("similarity = %v, want 0 when every bit differs", similarity)
	}
}

// Half the bits differing must land exactly halfway, not merely "somewhere in
// between": the confirmation thresholds are read off this scale, so a
// non-linear or mis-normalised similarity would move all of them at once.
func TestSlidingChromaprint_SimilarityScaleIsLinearInDifferingBits(t *testing.T) {
	// 0x0000FFFF differs from 0 in exactly 16 of its 32 bits.
	target := []uint32{0x00000000}
	reference := []uint32{0x0000FFFF}

	_, similarity := slidingChromaprint(target, reference)

	if math.Abs(similarity-0.5) > chromaprintEpsilon {
		t.Errorf("similarity = %v, want 0.5 for 16 of 32 bits differing", similarity)
	}
}

// The slide has to keep looking after its first window: an intro sits at an
// arbitrary offset in the sibling, which is the entire reason this function
// slides rather than comparing head to head.
func TestSlidingChromaprint_PicksTheClosestWindowNotTheFirst(t *testing.T) {
	target := []uint32{0x0000FFFF}
	// Offset 0 differs in 16 bits; offset 1 is exact.
	reference := []uint32{0x00000000, 0x0000FFFF}

	offset, similarity := slidingChromaprint(target, reference)

	if offset != 1 {
		t.Errorf("offset = %d, want 1 (the exact window, not the first one)", offset)
	}
	if math.Abs(similarity-1) > chromaprintEpsilon {
		t.Errorf("similarity = %v, want 1", similarity)
	}
}

// Degenerate inputs must return the (0, 0) sentinel rather than reading past
// the end of the reference - a similarity of 0 is also the honest answer when
// there is nothing to compare.
func TestSlidingChromaprint_RejectsInputsItCannotSlide(t *testing.T) {
	if off, sim := slidingChromaprint(nil, []uint32{1, 2}); off != 0 || sim != 0 {
		t.Errorf("empty target: got (%d, %v), want the (0, 0) sentinel", off, sim)
	}
	if off, sim := slidingChromaprint([]uint32{1, 2, 3}, []uint32{1, 2}); off != 0 || sim != 0 {
		t.Errorf("reference shorter than target: got (%d, %v), want the (0, 0) sentinel", off, sim)
	}
}
