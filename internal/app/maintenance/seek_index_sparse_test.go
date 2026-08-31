package maintenance_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	"github.com/evey-project/evey/internal/domain/mkv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// restagingDiagnoser answers with `before` until a path has been repaired, then
// with `after` - the real sequence a repair batch goes through, which a static
// map cannot express. Repaired paths are declared by the test through Repaired.
type restagingDiagnoser struct {
	mu       sync.Mutex
	before   map[string]*mkv.Diagnosis
	after    map[string]*mkv.Diagnosis
	repaired map[string]bool
}

func (d *restagingDiagnoser) Diagnose(_ context.Context, p string) (*mkv.Diagnosis, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.repaired[p] {
		if a, ok := d.after[p]; ok {
			return a, nil
		}
	}
	b, ok := d.before[p]
	if !ok {
		return nil, os.ErrInvalid
	}
	return b, nil
}

func (d *restagingDiagnoser) markRepaired(p string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.repaired[p] = true
}

// sparse is the diagnosis of a file whose video cues exist but sit too far
// apart to seek into (mkvgo "index-sparse").
func sparseDiagnosis() *mkv.Diagnosis {
	return &mkv.Diagnosis{Findings: []mkv.DiagnosisFinding{{Kind: "index-sparse"}}}
}

// A repair must tell the truth about what it achieved, and must never re-offer
// a file it cannot improve.
//
// Two files enter with the same verdict (index-sparse). The rewrite fixes one -
// its cues were thinner than its keyframes - and cannot fix the other, whose
// keyframes themselves are a minute apart: no index can be denser than the
// keyframes it names. Before this, the repair blind-set HasIndex = true on both
// (the panel showed a fix that had not happened), and the next scan re-flagged
// the unfixable one for a repair that would skip it again, forever.
func TestRepairLibrary_SparseIndex_TruthAfterRepairAndNoReloop(t *testing.T) {
	dir := t.TempDir()
	fixable := filepath.Join(dir, "fixable.mkv")     // sparse cues over dense keyframes
	intrinsic := filepath.Join(dir, "intrinsic.mkv") // keyframes themselves are sparse
	for _, p := range []string{fixable, intrinsic} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}

	diag := &restagingDiagnoser{
		before: map[string]*mkv.Diagnosis{fixable: sparseDiagnosis(), intrinsic: sparseDiagnosis()},
		after: map[string]*mkv.Diagnosis{
			fixable:   {Healthy: true},
			intrinsic: sparseDiagnosis(), // the rewrite changed nothing measurable
		},
		repaired: map[string]bool{},
	}
	reidx := &fakeReindexer{hasIndex: map[string]bool{}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m1", Title: "Fixable", Path: fixable},
		{MediaID: "m2", Title: "Intrinsic", Path: intrinsic},
	}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).WithDiagnoser(diag)

	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	require.Equal(t, 2, res.MissingIndex, "both files enter flagged: a sparse index is not a usable one")

	// The batch reindexes both; the diagnoser re-stages as each file is done.
	origReindex := reidx.onReindexed
	reidx.onReindexed = func(p string) {
		diag.markRepaired(p)
		if origReindex != nil {
			origReindex(p)
		}
	}
	require.NoError(t, svc.StartRepairLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, 2, st.Last.Done)
	assert.Zero(t, st.Last.Failed)
	assert.ElementsMatch(t, []string{fixable, intrinsic}, reidx.reindexed)

	res, _, ok = svc.CachedScan("")
	require.True(t, ok)
	byID := map[string]appmaintenance.SeekStatus{}
	for _, it := range res.Items {
		byID[it.MediaID] = it
	}
	assert.True(t, byID["m1"].HasIndex, "the rebuilt index is healthy - the file is fixed")
	assert.False(t, byID["m1"].SeekCoarse(), "a fixed file is not coarse")
	assert.False(t, byID["m2"].HasIndex, "the index is STILL too coarse - claiming otherwise is a lie the next scan exposes")
	assert.True(t, byID["m2"].SeekCoarse(),
		"rebuilt and still sparse = the encode's own keyframes are too far apart; no repair can improve it")
	assert.Equal(t, 1, res.SeekCoarse, "the unfixable file is counted in its own bucket")
	assert.Zero(t, res.MissingIndex,
		"neither file belongs in the repair queue anymore: one is fixed, the other is beyond repair")

	// The loop-closing assertion: a second "repair all" must find nothing to do.
	reidx.reindexed = nil
	require.NoError(t, svc.StartRepairLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	st = waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Zero(t, st.Last.Total, "a second run has no candidate - this is the sterile loop that must not reopen")
	assert.Empty(t, reidx.reindexed)
}

// A file whose index is simply absent keeps the plain repair path: the
// coarse-index bucket must not swallow the ordinary case.
func TestRepairLibrary_NoIndex_StillRepairedNormally(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "no_index.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))

	diag := &restagingDiagnoser{
		before:   map[string]*mkv.Diagnosis{p: {Findings: []mkv.DiagnosisFinding{{Kind: "no-index"}}}},
		after:    map[string]*mkv.Diagnosis{p: {Healthy: true}},
		repaired: map[string]bool{},
	}
	reidx := &fakeReindexer{hasIndex: map[string]bool{}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Title: "NoIndex", Path: p}}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).WithDiagnoser(diag)
	reidx.onReindexed = diag.markRepaired

	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	require.NoError(t, svc.StartRepairLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)

	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	assert.True(t, res.Items[0].HasIndex)
	assert.False(t, res.Items[0].SeekCoarse())
	assert.Zero(t, res.SeekCoarse)
	assert.Zero(t, res.MissingIndex)
}
