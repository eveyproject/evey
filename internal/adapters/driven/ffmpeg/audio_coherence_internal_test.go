package ffmpeg

import (
	"math"
	"math/rand"
	"testing"
)

// filmEnvelope fakes what a soundtrack looks like to astats: a slow musical
// swell with scene changes on top, which is exactly the structure the
// correlation locks onto. seed drives the "different mix" variants.
func filmEnvelope(n int, seed int64) []float64 {
	r := rand.New(rand.NewSource(seed)) //nolint:gosec // deterministic test fixture
	out := make([]float64, n)
	level := -35.0
	for i := range out {
		if i%137 == 0 { // a scene change
			level = -45 + r.Float64()*20
		}
		out[i] = level + math.Sin(float64(i)/40)*3 + r.NormFloat64()*0.6
	}
	return out
}

// shift returns env delayed by d samples (padded with room tone), the shape a
// track that has been moved leaves behind.
func shift(env []float64, d int) []float64 {
	out := make([]float64, len(env))
	for i := range out {
		src := i - d
		if src < 0 || src >= len(env) {
			out[i] = -60
			continue
		}
		out[i] = env[src]
	}
	return out
}

// Two tracks carrying the same film line up at zero, confidently - the case
// that must NEVER be reported as a defect, whatever offsets the container
// declares. This is the deliberate dub alignment.
func TestCorrelateEnvelopes_SameFilmAgreesAtZero(t *testing.T) {
	a := filmEnvelope(3000, 1)
	b := make([]float64, len(a))
	r := rand.New(rand.NewSource(99)) //nolint:gosec // deterministic test fixture
	for i := range b {
		b[i] = a[i] + r.NormFloat64()*1.5 // a different mix of the same film
	}

	lag, confidence, ok := correlateEnvelopes(a, b)
	if !ok {
		t.Fatal("two mixes of one film must be comparable")
	}
	if math.Abs(lag) > 0.25 {
		t.Fatalf("lag = %+.3fs, want ~0 for two tracks of the same film", lag)
	}
	if confidence < 0.5 {
		t.Fatalf("confidence = %.2f, want a believable agreement", confidence)
	}
}

// A track moved by a known amount is found at that amount, with the sign that
// says which way. This is the a prod title shape: 12 s of displacement.
func TestCorrelateEnvelopes_FindsAKnownDisplacement(t *testing.T) {
	a := filmEnvelope(4000, 7)
	step := float64(coherenceWindowSec) / 4000
	for _, wantSec := range []float64{-12, -4.5, 7.5, 12} {
		d := int(math.Round(wantSec / step))
		b := shift(a, d)
		lag, confidence, ok := correlateEnvelopes(a, b)
		if !ok {
			t.Fatalf("%+.1fs: not comparable", wantSec)
		}
		if math.Abs(lag-wantSec) > 0.5 {
			t.Errorf("displacement %+.1fs read as %+.2fs", wantSec, lag)
		}
		if confidence < 0.5 {
			t.Errorf("displacement %+.1fs: confidence %.2f too low to act on", wantSec, confidence)
		}
	}
}

// Two unrelated soundtracks (a commentary, a wildly different mix) must come
// back with a confidence the caller will DISCARD. Nine files in the real
// library sat here, and accusing them would have been the same mistake all
// over again.
func TestCorrelateEnvelopes_UnrelatedTracksAreNotEvidence(t *testing.T) {
	a := filmEnvelope(3000, 11)
	b := filmEnvelope(3000, 12345)
	_, confidence, ok := correlateEnvelopes(a, b)
	if ok && confidence >= 0.5 {
		t.Fatalf("unrelated tracks scored %.2f - that is an accusation drawn from noise", confidence)
	}
}

// Too little signal, or a silent track, yields no verdict at all rather than a
// confident zero.
func TestCorrelateEnvelopes_RefusesWithoutSignal(t *testing.T) {
	if _, _, ok := correlateEnvelopes(filmEnvelope(50, 3), filmEnvelope(50, 4)); ok {
		t.Error("a handful of samples must not produce a verdict")
	}
	flat := make([]float64, 3000)
	for i := range flat {
		flat[i] = coherenceSilenceDB
	}
	if _, _, ok := correlateEnvelopes(flat, filmEnvelope(3000, 5)); ok {
		t.Error("a silent track correlates with nothing and must say so")
	}
}

// The envelope's time step is DERIVED from the data - window length over
// sample count - never assumed from the resolution asked of astats. astats
// emits one value per decoded audio frame whatever length it is given (32 ms
// for AC-3 at 48 kHz, not the 0.2 s requested), and a script that assumed the
// requested step once read every lag 1.5625 times too large, "proved" a
// timestamp fault that did not exist, and got two films rewritten for nothing
// (2026-09-01). This test refuses the "simplification" that would put a
// constant back: the same displacement must come out right at a frame count
// the requested step cannot explain.
func TestCorrelateEnvelopes_StepIsDerivedFromTheData(t *testing.T) {
	// 240 s of 32 ms frames: 7500 samples, a step of 0.032 s - nowhere near
	// coherenceEnvelopeStep.
	const n = 7500
	a := filmEnvelope(n, 21)
	const d = 373 // frames, the a prod title displacement (373 x 32 ms = 11.936 s)
	b := shift(a, d)
	lag, _, ok := correlateEnvelopes(a, b)
	if !ok {
		t.Fatal("not comparable")
	}
	want := float64(d) * float64(coherenceWindowSec) / n
	if math.Abs(lag-want) > 0.1 {
		t.Fatalf("lag = %.3fs, want %.3fs (derived step); a constant step would read %.3fs",
			lag, want, float64(d)*coherenceEnvelopeStep)
	}
}

// A displacement beyond the search window must come back as "could not
// conclude", not as the best peak the wrong window happened to hold.
func TestCorrelateEnvelopes_BeyondTheWindowIsNoVerdict(t *testing.T) {
	a := filmEnvelope(4000, 31)
	step := float64(coherenceWindowSec) / 4000
	b := shift(a, int(math.Round(45/step))) // 45 s, past coherenceMaxLagSec
	if lag, conf, ok := correlateEnvelopes(a, b); ok {
		t.Fatalf("a 45 s displacement was reported as %+.2fs (r=%.2f) - a false peak", lag, conf)
	}
}
