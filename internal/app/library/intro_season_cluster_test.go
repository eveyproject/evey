package library

import (
	"math/bits"
	"math/rand"
	"testing"
)

// noiseFrames returns n pseudo-random fingerprint frames (deterministic per rng).
func noiseFrames(rng *rand.Rand, n int) []uint32 {
	out := make([]uint32, n)
	for i := range out {
		out[i] = rng.Uint32()
	}
	return out
}

// flipMask returns a 32-bit mask with exactly k bits set (k <= 32).
func flipMask(rng *rand.Rand, k int) uint32 {
	var m uint32
	for bits.OnesCount32(m) < k {
		m |= 1 << uint(rng.Intn(32))
	}
	return m
}

// embedSignature builds an episode fingerprint of totalLen frames: noise
// everywhere except the signature copied at `offset`, with bitsFlipped bits
// flipped per signature frame (0 = verbatim copy).
func embedSignature(rng *rand.Rand, offset int, sig []uint32, totalLen, bitsFlipped int) []uint32 {
	fp := noiseFrames(rng, totalLen)
	for i, v := range sig {
		if bitsFlipped > 0 {
			v ^= flipMask(rng, bitsFlipped)
		}
		fp[offset+i] = v
	}
	return fp
}

func carriersByID(c []clusterCarrier) map[string]clusterCarrier {
	m := make(map[string]clusterCarrier, len(c))
	for _, x := range c {
		m[x.id] = x
	}
	return m
}

// A signature shared by 5 episodes at different offsets (variable intro
// position) plus 2 noise-only episodes must yield a 5-carrier cluster with the
// correct per-episode offsets, and exclude the noise episodes.
func TestBuildSeasonCluster_FindsSharedSignatureAtVariableOffsets(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	sig := noiseFrames(rng, 120) // intro signature, ~16s, > MinRunFrames(110)

	const totalLen = 1100
	offsets := map[string]int{
		"ep1": 200, // all >= MinStartFrames (~189) so any anchor finds the run
		"ep2": 500,
		"ep3": 350,
		"ep4": 800,
		"ep5": 230,
	}
	eps := []episodeFP{
		{id: "ep1", fp: embedSignature(rng, 200, sig, totalLen, 0)},
		{id: "ep2", fp: embedSignature(rng, 500, sig, totalLen, 0)},
		{id: "ep3", fp: embedSignature(rng, 350, sig, totalLen, 0)},
		{id: "ep4", fp: embedSignature(rng, 800, sig, totalLen, 0)},
		{id: "ep5", fp: embedSignature(rng, 230, sig, totalLen, 0)},
		{id: "noiseA", fp: noiseFrames(rng, totalLen)},
		{id: "noiseB", fp: noiseFrames(rng, totalLen)},
	}

	cluster, found := buildSeasonCluster(eps, defaultClusterParams(len(eps)))
	if !found {
		t.Fatal("expected a season cluster to be found")
	}
	if len(cluster.carriers) != 5 {
		t.Fatalf("expected 5 carriers, got %d: %+v", len(cluster.carriers), cluster.carriers)
	}
	byID := carriersByID(cluster.carriers)
	if _, ok := byID["noiseA"]; ok {
		t.Error("noiseA must not be a carrier")
	}
	if _, ok := byID["noiseB"]; ok {
		t.Error("noiseB must not be a carrier")
	}
	for id, want := range offsets {
		c, ok := byID[id]
		if !ok {
			t.Errorf("%s should be a carrier", id)
			continue
		}
		if c.offsetFrames != want {
			t.Errorf("%s offset = %d, want %d", id, c.offsetFrames, want)
		}
	}
}

// A signature shared by only 2 of 8 episodes is below KMin = ceil(8/2) = 4 and
// must NOT be accepted as the season intro.
func TestBuildSeasonCluster_BelowKMinRejected(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	sig := noiseFrames(rng, 130)
	const totalLen = 1000

	eps := []episodeFP{
		{id: "ep1", fp: embedSignature(rng, 250, sig, totalLen, 0)},
		{id: "ep2", fp: embedSignature(rng, 300, sig, totalLen, 0)},
	}
	for i := range 6 {
		eps = append(eps, episodeFP{id: "noise" + string(rune('A'+i)), fp: noiseFrames(rng, totalLen)})
	}

	if _, found := buildSeasonCluster(eps, defaultClusterParams(len(eps))); found {
		t.Fatal("expected no cluster: only 2/8 episodes share the signature (< KMin)")
	}
}

// A SHORT recurring run (a "previously on" recap, < MinRunFrames) shared by all
// episodes must not be mistaken for the intro.
func TestBuildSeasonCluster_ShortRecapRejected(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	recap := noiseFrames(rng, 60) // < MinRunFrames(110)
	const totalLen = 900

	var eps []episodeFP
	for i := range 6 {
		// Same short recap at offset 250 in every episode, rest unique noise.
		eps = append(eps, episodeFP{
			id: "ep" + string(rune('1'+i)),
			fp: embedSignature(rng, 250, recap, totalLen, 0),
		})
	}

	if _, found := buildSeasonCluster(eps, defaultClusterParams(len(eps))); found {
		t.Fatal("expected no cluster: the shared run is shorter than MinRunFrames")
	}
}

// Real intros match with a low (not zero) bit-error rate. A signature copied
// with a few bits flipped per frame must still be a carrier; one with many bits
// flipped (unrelated audio) must not.
func TestBuildSeasonCluster_BitErrorTolerance(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	sig := noiseFrames(rng, 120)
	const totalLen = 1100

	eps := []episodeFP{
		{id: "exact", fp: embedSignature(rng, 200, sig, totalLen, 0)},
		{id: "lowBER1", fp: embedSignature(rng, 300, sig, totalLen, 4)},  // ~0.875 sim -> carrier
		{id: "lowBER2", fp: embedSignature(rng, 250, sig, totalLen, 5)},  // ~0.84 sim -> carrier
		{id: "lowBER3", fp: embedSignature(rng, 400, sig, totalLen, 3)},  // ~0.91 sim -> carrier
		{id: "highBER", fp: embedSignature(rng, 350, sig, totalLen, 20)}, // ~0.375 sim -> NOT carrier
	}

	cluster, found := buildSeasonCluster(eps, defaultClusterParams(len(eps)))
	if !found {
		t.Fatal("expected a cluster from the low-BER carriers")
	}
	byID := carriersByID(cluster.carriers)
	for _, id := range []string{"exact", "lowBER1", "lowBER2", "lowBER3"} {
		if _, ok := byID[id]; !ok {
			t.Errorf("%s should be a carrier (within bit tolerance)", id)
		}
	}
	if _, ok := byID["highBER"]; ok {
		t.Error("highBER (20 bits flipped) must not be a carrier")
	}
}

func TestChooseAnchorIndices(t *testing.T) {
	cases := []struct {
		n, max int
		want   []int
	}{
		{0, 5, nil},
		{3, 5, []int{0, 1, 2}},   // n <= max -> all
		{7, 5, []int{0, 1, 3, 4, 6}}, // spread incl. first and last
		{10, 1, []int{0}},
	}
	for _, c := range cases {
		got := chooseAnchorIndices(c.n, c.max)
		if len(got) != len(c.want) {
			t.Errorf("chooseAnchorIndices(%d,%d) = %v, want %v", c.n, c.max, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("chooseAnchorIndices(%d,%d) = %v, want %v", c.n, c.max, got, c.want)
				break
			}
		}
	}
}
