package maintenance_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appmaintenance "github.com/eveyproject/evey/internal/app/maintenance"
	"github.com/eveyproject/evey/internal/domain/mkv"
)

type fakeDamageRecorder struct {
	mu  sync.Mutex
	set map[string]int64
}

func (r *fakeDamageRecorder) SetContainerDamage(_ context.Context, p string, n int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.set == nil {
		r.set = map[string]int64{}
	}
	r.set[p] = n
	return nil
}

// Every diagnosis of the media-health scan reaches the catalog: a damaged
// container is remuxed from then on, a clean one (repaired, re-acquired) is
// handed over whole again, and a damaged finding without a byte count is
// never recorded as clean.
func TestSeekIndexService_RecordsContainerDamage(t *testing.T) {
	dir := t.TempDir()
	paths := map[string]string{}
	for _, n := range []string{"head", "body", "clean"} {
		p := filepath.Join(dir, n+".mkv")
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
		paths[n] = p
	}
	damaged := mkv.DiagnosisFinding{Kind: "damaged", Detail: "6 undecodable byte(s) ahead of the first Cluster"}
	diag := fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{
		paths["head"]:  {Findings: []mkv.DiagnosisFinding{damaged}, DamagedBytes: 6},
		paths["body"]:  {Findings: []mkv.DiagnosisFinding{damaged}},
		paths["clean"]: {Healthy: true},
	}}
	refs := []appmaintenance.MediaRef{{MediaID: "m1", Path: paths["head"]}, {MediaID: "m2", Path: paths["body"]}, {MediaID: "m3", Path: paths["clean"]}}
	reidx := &fakeReindexer{hasIndex: map[string]bool{paths["head"]: true, paths["body"]: true, paths["clean"]: true}}
	rec := &fakeDamageRecorder{}
	svc := appmaintenance.NewSeekIndexService(reidx, fakeCatalog{files: refs}, nil).
		WithDiagnoser(diag).
		WithContainerDamageRecorder(rec)

	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)

	assert.Equal(t, map[string]int64{paths["head"]: 6, paths["body"]: 1, paths["clean"]: 0}, rec.set)
}
