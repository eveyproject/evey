package maintenance_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appmaintenance "github.com/eveyproject/evey/internal/app/maintenance"
	"github.com/eveyproject/evey/internal/domain/mkv"
)

// fakeInterleaveRecorder is the catalog side of the measure; fail makes every
// write refuse.
type fakeInterleaveRecorder struct {
	mu   sync.Mutex
	set  map[string]int64
	fail error
}

func (r *fakeInterleaveRecorder) SetInterleaveBackstep(_ context.Context, p string, ms int64) error {
	if r.fail != nil {
		return r.fail
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.set == nil {
		r.set = map[string]int64{}
	}
	r.set[p] = ms
	return nil
}

// chunkedLibrary is one chunked file, its re-mux and the probe that judges the
// result. The reinterleaver "fixes" the file (probe reads 0 afterwards) unless
// the test rigs the probe otherwise before starting the repair.
func chunkedLibrary(t *testing.T, rec *fakeInterleaveRecorder) (path string, svc *appmaintenance.SeekIndexService, reidx *fakeReindexer, probe fakeInterleaveProber) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "chunked.mkv")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))
	reidx = &fakeReindexer{hasIndex: map[string]bool{path: true}}
	probe = fakeInterleaveProber{backsteps: map[string]float64{path: 366}, errs: map[string]error{}}
	svc = appmaintenance.NewSeekIndexService(reidx, fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: path}}}, nil).
		WithDiagnoser(fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{path: {Healthy: true}}}).
		WithInterleaveProber(probe).
		WithReinterleaver(&fakeReinterleaver{probe: probe})
	if rec != nil {
		svc = svc.WithInterleaveRecorder(rec)
	}
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	return path, svc, reidx, probe
}

// A verification that could not run is not a verification that passed. The
// re-mux used to be declared "applied" - cache cleared, catalog told the file
// plays - whenever the probe errored out, on no evidence at all. The original
// is proven intact by the packet count, so it goes back.
func TestSeekIndexService_Reinterleave_ProbeErrorRollsBack(t *testing.T) {
	rec := &fakeInterleaveRecorder{}
	path, svc, reidx, probe := chunkedLibrary(t, rec)
	rec.set = nil // forget the scan's own record; only the repair's matters below
	probe.errs[path] = errors.New("read: input/output error")

	require.NoError(t, svc.StartReinterleaveMedia(context.Background(), "m1"))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	require.Len(t, st.Last.Failures, 1, "the unverified re-mux is this file's failure")
	assert.Contains(t, st.Last.Failures[0].Error, appmaintenance.ErrReinterleaveUnverified.Error())
	assert.Equal(t, []string{path}, reidx.restored, "the original is put back")

	res, _, _ := svc.CachedScan("")
	require.Len(t, res.Items, 1)
	assert.True(t, res.Items[0].BrokenInterleave(), "the cache keeps the refusal: nothing proved the file plays")
	assert.Empty(t, rec.set, "the catalog is not told the file plays")
}

// The catalog write is what lifts the playback refusal (file_needs_remux). When
// it fails, the file IS repaired - the cache says so, so no batch re-muxes a
// sound file - but the operator must hear that the catalog still refuses it.
func TestSeekIndexService_Reinterleave_CatalogFailureIsReported(t *testing.T) {
	rec := &fakeInterleaveRecorder{}
	_, svc, reidx, _ := chunkedLibrary(t, rec)
	rec.fail = errors.New("pq: connection refused")

	require.NoError(t, svc.StartReinterleaveMedia(context.Background(), "m1"))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	require.Len(t, st.Last.Failures, 1)
	assert.Contains(t, st.Last.Failures[0].Error, appmaintenance.ErrCatalogNotUpdated.Error())
	assert.Empty(t, reidx.restored, "the file is sound: nothing to roll back")

	res, _, _ := svc.CachedScan("")
	require.Len(t, res.Items, 1)
	assert.False(t, res.Items[0].BrokenInterleave(), "the cache reflects the file, not the catalog")
}

// The happy path, for contrast: probe reads 0, catalog told, nothing failed.
func TestSeekIndexService_Reinterleave_VerifiedIsApplied(t *testing.T) {
	rec := &fakeInterleaveRecorder{}
	path, svc, reidx, _ := chunkedLibrary(t, rec)

	require.NoError(t, svc.StartReinterleaveMedia(context.Background(), "m1"))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Zero(t, st.Last.Failed, "%+v", st.Last.Failures)
	assert.Empty(t, reidx.restored)
	assert.Equal(t, int64(0), rec.set[path], "the catalog lifts the refusal")
}
