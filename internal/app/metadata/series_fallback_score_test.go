package metadata

import "testing"

// The fallback ranks series candidates when none matches the disk episode
// count exactly. Designed for the case where an old miniseries outranks a
// recent remake by raw popularity: episode-count proximity must dominate,
// recency tips a near-tie, popularity is capped so it never outvotes either.
func TestSeriesFallbackScore(t *testing.T) {
	diskEp := 9

	// Closest episode count wins even against a newer, more popular show.
	close := seriesFallbackScore(diskEp, 10, new(1990), 500)
	far := seriesFallbackScore(diskEp, 40, new(2025), 500)
	if close <= far {
		t.Fatalf("episode proximity must dominate: close=%.1f far=%.1f", close, far)
	}

	// Same episode count: the more recent series scores higher.
	old := seriesFallbackScore(diskEp, 9, new(2010), 10)
	recent := seriesFallbackScore(diskEp, 9, new(2025), 10)
	if recent <= old {
		t.Fatalf("recency must tip a tie: old=%.1f recent=%.1f", old, recent)
	}
	// Years before the pivot, and an unknown year, add nothing.
	if seriesFallbackScore(diskEp, 9, new(2000), 10) != old {
		t.Fatal("a year before the pivot must score like any other pre-pivot year")
	}
	if seriesFallbackScore(diskEp, 9, nil, 10) != old {
		t.Fatal("an unknown year must score like a pre-pivot year")
	}

	// Popularity is capped: a blockbuster cannot outvote the episode signal.
	capped := seriesFallbackScore(diskEp, 9, nil, 50)
	blockbuster := seriesFallbackScore(diskEp, 9, nil, 100_000)
	if blockbuster != capped {
		t.Fatalf("popularity must be capped: %.1f vs %.1f", blockbuster, capped)
	}
	// The cap equals the loss of ONE episode of distance: a blockbuster one
	// episode off ties an exact match with no popularity (never beats it), and
	// two episodes off it loses outright.
	exact := seriesFallbackScore(diskEp, 9, nil, 0)
	if off := seriesFallbackScore(diskEp, 8, nil, 100_000); off > exact {
		t.Fatalf("one episode off with maximal popularity must not beat an exact match: %.1f vs %.1f", off, exact)
	}
	if off := seriesFallbackScore(diskEp, 7, nil, 100_000); off >= exact {
		t.Fatalf("two episodes off must lose to an exact match whatever the popularity: %.1f vs %.1f", off, exact)
	}

	// Distance is symmetric: one too many and one too few score the same.
	if seriesFallbackScore(diskEp, 8, nil, 0) != seriesFallbackScore(diskEp, 10, nil, 0) {
		t.Fatal("episode distance must be absolute")
	}
}
