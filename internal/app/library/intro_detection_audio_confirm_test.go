package library

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// Mechanical contracts of the Chromaprint confirmation stage: guards, the
// three rejection paths and the accepted shape. No verdict on real media is
// pinned here - that truth comes from the user (intro_ground_truth_user).

// pathChromaprinter answers per path and can fail for some of them.
type pathChromaprinter struct {
	byPath map[string][]uint32
	failOn map[string]bool
	calls  int
}

func (f *pathChromaprinter) Available(context.Context) error { return nil }
func (f *pathChromaprinter) Compute(_ context.Context, path string, _, _ float64) ([]uint32, error) {
	f.calls++
	if f.failOn[path] {
		return nil, errors.New("fpcalc failed")
	}
	return f.byPath[path], nil
}

func audioConfirmService(t *testing.T, cp Chromaprinter) *IntroDetectionService {
	t.Helper()
	svc := NewIntroDetectionService(&fakeSegmentRepo{}, &fakeTrickplayRepo{}, &fakeMediaItemRepo{}, &fakeConfigRepo{}, &fakeDetector{}, t.TempDir())
	if cp != nil {
		svc.WithChromaprinter(cp)
	}
	return svc
}

func episodeItem(id string) *domainlibrary.MediaItem {
	return &domainlibrary.MediaItem{ID: shared.MediaItemID(id), Type: domainlibrary.Episode, File: &domainlibrary.MediaFile{Path: id}}
}

func siblingsNamed(ids ...string) []*domainlibrary.MediaItem {
	out := make([]*domainlibrary.MediaItem, 0, len(ids))
	for _, id := range ids {
		out = append(out, episodeItem(id))
	}
	return out
}

var oneCandidate = []pHashCandidate{{startMs: 30_000, endMs: 90_000, matchCount: 12}}

func TestConfirmAudioChromaprint_GuardsReturnNil(t *testing.T) {
	ctx := context.Background()
	cfg := defaultIntroConfig()
	result := &pHashResult{candidates: oneCandidate, siblings: siblingsNamed("s1", "s2", "s3")}

	// No chromaprinter wired.
	if got := audioConfirmService(t, nil).confirmAudioCandidatesChromaprint(ctx, episodeItem("t"), result, cfg); got != nil {
		t.Errorf("no chromaprinter: got %v, want nil", got)
	}
	svc := audioConfirmService(t, &pathChromaprinter{})
	// Target without a file, or with an empty path.
	if got := svc.confirmAudioCandidatesChromaprint(ctx, &domainlibrary.MediaItem{ID: "t"}, result, cfg); got != nil {
		t.Errorf("nil file: got %v, want nil", got)
	}
	if got := svc.confirmAudioCandidatesChromaprint(ctx, &domainlibrary.MediaItem{ID: "t", File: &domainlibrary.MediaFile{}}, result, cfg); got != nil {
		t.Errorf("empty path: got %v, want nil", got)
	}
	// No siblings to confirm against.
	if got := svc.confirmAudioCandidatesChromaprint(ctx, episodeItem("t"), &pHashResult{candidates: oneCandidate}, cfg); got != nil {
		t.Errorf("no siblings: got %v, want nil", got)
	}
}

func TestConfirmAudioChromaprint_NoCandidatesIsEmptyNotNil(t *testing.T) {
	svc := audioConfirmService(t, &pathChromaprinter{})
	got := svc.confirmAudioCandidatesChromaprint(context.Background(), episodeItem("t"), &pHashResult{siblings: siblingsNamed("s1")}, defaultIntroConfig())
	if got == nil || len(got) != 0 {
		t.Fatalf("got %v, want an empty slice", got)
	}
}

func TestConfirmAudioChromaprint_IdenticalSiblingsConfirm(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	sig := noiseFrames(rng, 400)
	cp := &pathChromaprinter{byPath: map[string][]uint32{"t": sig, "s1": sig, "s2": sig, "s3": sig}}
	svc := audioConfirmService(t, cp)
	result := &pHashResult{candidates: oneCandidate, siblings: siblingsNamed("s1", "s2", "s3")}

	got := svc.confirmAudioCandidatesChromaprint(context.Background(), episodeItem("t"), result, defaultIntroConfig())
	if len(got) != 1 {
		t.Fatalf("got %d confirmed windows, want 1", len(got))
	}
	w := got[0]
	if w.startMs != 30_000 || w.endMs != 90_000 || w.matchCount != 12 {
		t.Errorf("window = %+v, want the candidate's bounds and match count", w)
	}
	if w.confidence != 0.97 {
		t.Errorf("confidence = %v, want 0.97", w.confidence)
	}
	// One target extraction plus one per sibling, nothing more.
	if cp.calls != 4 {
		t.Errorf("chromaprinter called %d times, want 4", cp.calls)
	}
}

func TestConfirmAudioChromaprint_TargetExtractFailureRejects(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	sig := noiseFrames(rng, 400)
	cp := &pathChromaprinter{byPath: map[string][]uint32{"s1": sig, "s2": sig, "s3": sig}, failOn: map[string]bool{"t": true}}
	svc := audioConfirmService(t, cp)
	result := &pHashResult{candidates: oneCandidate, siblings: siblingsNamed("s1", "s2", "s3")}
	if got := svc.confirmAudioCandidatesChromaprint(context.Background(), episodeItem("t"), result, defaultIntroConfig()); len(got) != 0 {
		t.Fatalf("got %v, want no confirmation", got)
	}
	if cp.calls != 1 {
		t.Errorf("siblings must not be extracted when the target failed, got %d calls", cp.calls)
	}
}

func TestConfirmAudioChromaprint_TooFewMatchesRejects(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	sig := noiseFrames(rng, 400)
	// Both siblings match perfectly (coherence 1.0), but 2 < MinMatches (3):
	// the floor on the NUMBER of witnesses is what rejects here.
	cp := &pathChromaprinter{byPath: map[string][]uint32{"t": sig, "s1": sig, "s2": sig}}
	svc := audioConfirmService(t, cp)
	result := &pHashResult{candidates: oneCandidate, siblings: siblingsNamed("s1", "s2")}
	if got := svc.confirmAudioCandidatesChromaprint(context.Background(), episodeItem("t"), result, defaultIntroConfig()); len(got) != 0 {
		t.Fatalf("got %v, want no confirmation", got)
	}
}

// Siblings that all match but at scattered offsets are a recurring shot, not
// an intro: the drifts do not cluster, the coherence ratio falls under MinRatio.
func TestConfirmAudioChromaprint_ScatteredDriftsReject(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	sig := noiseFrames(rng, 100)
	const total = 800
	cp := &pathChromaprinter{byPath: map[string][]uint32{
		"t":  sig,
		"s1": embedSignature(rng, 0, sig, total, 0),
		"s2": embedSignature(rng, 200, sig, total, 0),
		"s3": embedSignature(rng, 400, sig, total, 0),
		"s4": embedSignature(rng, 600, sig, total, 0),
	}}
	svc := audioConfirmService(t, cp)
	result := &pHashResult{candidates: oneCandidate, siblings: siblingsNamed("s1", "s2", "s3", "s4")}
	if got := svc.confirmAudioCandidatesChromaprint(context.Background(), episodeItem("t"), result, defaultIntroConfig()); len(got) != 0 {
		t.Fatalf("got %v, want rejection on drift incoherence", got)
	}

	// The same siblings with the signature at ONE offset cluster and confirm.
	for _, id := range []string{"s1", "s2", "s3", "s4"} {
		cp.byPath[id] = embedSignature(rng, 300, sig, total, 0)
	}
	if got := svc.confirmAudioCandidatesChromaprint(context.Background(), episodeItem("t"), result, defaultIntroConfig()); len(got) != 1 {
		t.Fatalf("aligned siblings: got %d windows, want 1", len(got))
	}
}

// --- banded Pearson path (confirmAudioCandidates) ---

// bandedDetector answers AudioBandedFingerprint per path.
type bandedDetector struct {
	fakeDetector
	byPath map[string][]float64
	failOn map[string]bool
	calls  int
}

func (d *bandedDetector) AudioBandedFingerprint(_ context.Context, path string, _, _ float64, _ int) ([]float64, error) {
	d.calls++
	if d.failOn[path] {
		return nil, errors.New("ffmpeg failed")
	}
	return d.byPath[path], nil
}

func bandedNoise(rng *rand.Rand, bins, bands int) []float64 {
	out := make([]float64, bins*bands)
	for i := range out {
		out[i] = rng.Float64()
	}
	return out
}

// embedBanded copies sig into a noise fingerprint at bin offset `atBin`.
func embedBanded(rng *rand.Rand, atBin int, sig []float64, totalBins, bands int) []float64 {
	fp := bandedNoise(rng, totalBins, bands)
	copy(fp[atBin*bands:], sig)
	return fp
}

func bandedService(t *testing.T, d *bandedDetector) *IntroDetectionService {
	t.Helper()
	return NewIntroDetectionService(&fakeSegmentRepo{}, &fakeTrickplayRepo{}, &fakeMediaItemRepo{}, &fakeConfigRepo{}, d, t.TempDir())
}

func TestConfirmAudioBanded_GuardsReturnNil(t *testing.T) {
	ctx := context.Background()
	cfg := defaultIntroConfig()
	svc := bandedService(t, &bandedDetector{})
	result := &pHashResult{candidates: oneCandidate, siblings: siblingsNamed("s1", "s2", "s3")}
	if got := svc.confirmAudioCandidates(ctx, &domainlibrary.MediaItem{ID: "t"}, result, cfg); got != nil {
		t.Errorf("nil file: got %v, want nil", got)
	}
	if got := svc.confirmAudioCandidates(ctx, &domainlibrary.MediaItem{ID: "t", File: &domainlibrary.MediaFile{}}, result, cfg); got != nil {
		t.Errorf("empty path: got %v, want nil", got)
	}
	if got := svc.confirmAudioCandidates(ctx, episodeItem("t"), &pHashResult{candidates: oneCandidate}, cfg); got != nil {
		t.Errorf("no siblings: got %v, want nil", got)
	}
}

func TestConfirmAudioBanded_IdenticalSiblingsConfirm(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	cfg := defaultIntroConfig()
	sig := bandedNoise(rng, 240, cfg.NumBands)
	d := &bandedDetector{byPath: map[string][]float64{"t": sig, "s1": sig, "s2": sig, "s3": sig}}
	svc := bandedService(t, d)
	result := &pHashResult{candidates: oneCandidate, siblings: siblingsNamed("s1", "s2", "s3")}

	got := svc.confirmAudioCandidates(context.Background(), episodeItem("t"), result, cfg)
	if len(got) != 1 {
		t.Fatalf("got %d confirmed windows, want 1", len(got))
	}
	if w := got[0]; w.startMs != 30_000 || w.endMs != 90_000 || w.matchCount != 12 || w.confidence != 0.95 {
		t.Errorf("window = %+v, want the candidate's bounds, match count and confidence 0.95", w)
	}
	if d.calls != 4 {
		t.Errorf("detector called %d times, want 4", d.calls)
	}
}

func TestConfirmAudioBanded_TargetExtractFailureRejects(t *testing.T) {
	rng := rand.New(rand.NewSource(12))
	cfg := defaultIntroConfig()
	sig := bandedNoise(rng, 240, cfg.NumBands)
	d := &bandedDetector{byPath: map[string][]float64{"s1": sig, "s2": sig, "s3": sig}, failOn: map[string]bool{"t": true}}
	svc := bandedService(t, d)
	result := &pHashResult{candidates: oneCandidate, siblings: siblingsNamed("s1", "s2", "s3")}
	if got := svc.confirmAudioCandidates(context.Background(), episodeItem("t"), result, cfg); len(got) != 0 {
		t.Fatalf("got %v, want no confirmation", got)
	}
	if d.calls != 1 {
		t.Errorf("siblings must not be extracted when the target failed, got %d calls", d.calls)
	}
}

func TestConfirmAudioBanded_TooFewMatchesRejects(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	cfg := defaultIntroConfig()
	sig := bandedNoise(rng, 240, cfg.NumBands)
	d := &bandedDetector{byPath: map[string][]float64{"t": sig, "s1": sig, "s2": sig}}
	svc := bandedService(t, d)
	result := &pHashResult{candidates: oneCandidate, siblings: siblingsNamed("s1", "s2")}
	if got := svc.confirmAudioCandidates(context.Background(), episodeItem("t"), result, cfg); len(got) != 0 {
		t.Fatalf("2 witnesses < MinMatches: got %v, want no confirmation", got)
	}
}

func TestConfirmAudioBanded_ScatteredDriftsReject(t *testing.T) {
	rng := rand.New(rand.NewSource(14))
	cfg := defaultIntroConfig()
	const total = 800
	sig := bandedNoise(rng, 100, cfg.NumBands)
	d := &bandedDetector{byPath: map[string][]float64{
		"t":  sig,
		"s1": embedBanded(rng, 0, sig, total, cfg.NumBands),
		"s2": embedBanded(rng, 200, sig, total, cfg.NumBands),
		"s3": embedBanded(rng, 400, sig, total, cfg.NumBands),
		"s4": embedBanded(rng, 600, sig, total, cfg.NumBands),
	}}
	svc := bandedService(t, d)
	result := &pHashResult{candidates: oneCandidate, siblings: siblingsNamed("s1", "s2", "s3", "s4")}
	if got := svc.confirmAudioCandidates(context.Background(), episodeItem("t"), result, cfg); len(got) != 0 {
		t.Fatalf("got %v, want rejection on drift incoherence", got)
	}
	for _, id := range []string{"s1", "s2", "s3", "s4"} {
		d.byPath[id] = embedBanded(rng, 300, sig, total, cfg.NumBands)
	}
	if got := svc.confirmAudioCandidates(context.Background(), episodeItem("t"), result, cfg); len(got) != 1 {
		t.Fatalf("aligned siblings: got %d windows, want 1", len(got))
	}
}
