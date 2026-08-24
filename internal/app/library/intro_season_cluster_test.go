package library

import (
	"context"
	"math/bits"
	"math/rand"
	"strconv"
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// fakeChromaprinter returns a canned fingerprint per file path, so the season
// clustering path can be exercised end-to-end without fpcalc/ffmpeg.
type fakeChromaprinter struct{ byPath map[string][]uint32 }

func (f *fakeChromaprinter) Available(context.Context) error { return nil }
func (f *fakeChromaprinter) Compute(_ context.Context, path string, _, _ float64) ([]uint32, error) {
	return f.byPath[path], nil
}

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

// clusterSeasonIntro end-to-end (service glue) with a canned chromaprinter:
// 5 episodes carry the same signature at different offsets, 2 are noise. The
// method must return a per-carrier "audio_cluster" hint with the right start
// and exclude the noise episodes.
func TestClusterSeasonIntro_ReturnsCarrierHints(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	sig := noiseFrames(rng, 130)
	const totalLen = 1100
	offsets := map[string]int{"ep1": 200, "ep2": 520, "ep3": 360, "ep4": 800, "ep5": 240}
	fps := map[string][]uint32{
		"ep1":    embedSignature(rng, 200, sig, totalLen, 0),
		"ep2":    embedSignature(rng, 520, sig, totalLen, 0),
		"ep3":    embedSignature(rng, 360, sig, totalLen, 0),
		"ep4":    embedSignature(rng, 800, sig, totalLen, 0),
		"ep5":    embedSignature(rng, 240, sig, totalLen, 0),
		"noiseA": noiseFrames(rng, totalLen),
		"noiseB": noiseFrames(rng, totalLen),
	}
	var siblings []*domainlibrary.MediaItem
	for _, id := range []string{"ep1", "ep2", "ep3", "ep4", "ep5", "noiseA", "noiseB"} {
		siblings = append(siblings, &domainlibrary.MediaItem{
			ID:       shared.MediaItemID(id),
			ParentID: "season-1",
			Type:     domainlibrary.Episode,
			File:     &domainlibrary.MediaFile{Path: id},
		})
	}
	svc := NewIntroDetectionService(
		&fakeSegmentRepo{},
		&fakeTrickplayRepo{},
		&fakeMediaItemRepo{siblings: siblings},
		&fakeConfigRepo{},
		&fakeDetector{},
		t.TempDir(),
	)
	svc.WithChromaprinter(&fakeChromaprinter{byPath: fps})

	hints := svc.clusterSeasonIntro(context.Background(), "season-1")
	if len(hints) != 5 {
		t.Fatalf("expected 5 carrier hints, got %d: %+v", len(hints), hints)
	}
	for id, off := range offsets {
		h, ok := hints[id]
		if !ok {
			t.Errorf("%s should have a cluster hint", id)
			continue
		}
		if h.Type != "intro" || h.Source != "audio_cluster" {
			t.Errorf("%s hint = %q/%q, want intro/audio_cluster", id, h.Type, h.Source)
		}
		// The raw cluster window can wobble by a frame or two where noise
		// coincidentally aligns next to the signature; the downstream precision
		// pass tightens it. A few-frame tolerance is the realistic contract.
		if want, got := framesToMs(off), h.StartMs; got < want-400 || got > want+400 {
			t.Errorf("%s start = %d ms, want ~%d (+/-400)", id, got, want)
		}
	}
	if _, ok := hints["noiseA"]; ok {
		t.Error("noiseA must not carry the cluster")
	}
}

func TestChooseAnchorIndices(t *testing.T) {
	cases := []struct {
		n, max int
		want   []int
	}{
		{0, 5, nil},
		{3, 5, []int{0, 1, 2}},       // n <= max -> all
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

// MaxIntroStartMs is the ONE thing standing between a recurring in-episode
// element - a chapter card, a flashback, a "previously on" block that happens
// to share an audio signature across the season - and a "Skip Intro" button
// offered nine minutes into an episode. The cap had no test anywhere in the
// repo (zero occurrences of MaxIntroStartMs in any *_test.go), on either of
// its two call sites, so raising or dropping it would have been invisible.
//
// Same fixture as TestClusterSeasonIntro_ReturnsCarrierHints, with the cap
// pulled below the deepest carrier: the shallow ones must survive, the deep
// one must be dropped, and nothing else may change.
func TestClusterSeasonIntro_DropsCarriersPastTheStartCap(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	sig := noiseFrames(rng, 130)
	const totalLen = 1100
	// ep4 sits at frame 800, far deeper than the others.
	fps := map[string][]uint32{
		"ep1": embedSignature(rng, 200, sig, totalLen, 0),
		"ep2": embedSignature(rng, 520, sig, totalLen, 0),
		"ep3": embedSignature(rng, 360, sig, totalLen, 0),
		"ep4": embedSignature(rng, 800, sig, totalLen, 0),
		"ep5": embedSignature(rng, 240, sig, totalLen, 0),
	}
	var siblings []*domainlibrary.MediaItem
	for _, id := range []string{"ep1", "ep2", "ep3", "ep4", "ep5"} {
		siblings = append(siblings, &domainlibrary.MediaItem{
			ID:       shared.MediaItemID(id),
			ParentID: "season-1",
			Type:     domainlibrary.Episode,
			File:     &domainlibrary.MediaFile{Path: id},
		})
	}

	// Cap between ep3 (frame 360) and ep4 (frame 800).
	capMs := framesToMs(600)
	cfgRepo := &fakeConfigRepo{values: map[string]string{
		IntroDetectionConfigKey: `{"max_intro_start_ms": ` + strconv.FormatInt(capMs, 10) + `}`,
	}}
	svc := NewIntroDetectionService(
		&fakeSegmentRepo{}, &fakeTrickplayRepo{},
		&fakeMediaItemRepo{siblings: siblings}, cfgRepo, &fakeDetector{}, t.TempDir(),
	)
	svc.WithChromaprinter(&fakeChromaprinter{byPath: fps})

	hints := svc.clusterSeasonIntro(context.Background(), "season-1")

	if _, ok := hints["ep4"]; ok {
		t.Errorf("ep4 starts past the cap (%d ms) and must be dropped, got %+v", capMs, hints["ep4"])
	}
	for _, id := range []string{"ep1", "ep2", "ep3", "ep5"} {
		if _, ok := hints[id]; !ok {
			t.Errorf("%s starts before the cap and must survive it", id)
		}
	}
}
