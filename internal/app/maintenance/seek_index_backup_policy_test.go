package maintenance_test

// The July batch DID keep a backup of every file it rewrote - and then deleted
// them all. The rolling window, meant as a short abort horizon for a job the
// operator is watching, purged the only copies of 98 originals as it advanced.
// Seven weeks later there was nothing to restore.
//
// A window is fine for a repair that preserves the media's content (a reindex
// rebuilds a container's index, the picture and sound are the same bytes). It
// is not fine for one that alters it: a resync drops the damaged span, a retime
// moves a track. Those must keep their backup whatever the window says.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	"github.com/evey-project/evey/internal/domain/mkv"
)

// backupPolicyLibrary builds three damaged files so a window of 1 has
// something to purge.
func backupPolicyLibrary(t *testing.T) (*appmaintenance.SeekIndexService, *fakeReindexer) {
	t.Helper()
	dir := t.TempDir()
	reidx := &fakeReindexer{hasIndex: map[string]bool{}}
	diag := fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{}}
	var refs []appmaintenance.MediaRef
	for _, n := range []string{"a", "b", "c"} {
		p := filepath.Join(dir, n+".mkv")
		require.NoError(t, os.WriteFile(p, []byte("bytes"), 0o644))
		diag.byPath[p] = &mkv.Diagnosis{
			Findings:     []mkv.DiagnosisFinding{{Kind: "no-index"}, {Kind: "damaged"}},
			DamagedBytes: 4096,
		}
		refs = append(refs, appmaintenance.MediaRef{MediaID: "m-" + n, Title: n, Path: p})
	}
	reidx.onReindexed = func(p string) { diag.byPath[p] = &mkv.Diagnosis{Healthy: true} }
	svc := appmaintenance.NewSeekIndexService(reidx, fakeCatalog{files: refs}, nil).WithDiagnoser(diag)
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	return svc, reidx
}

// A repair that DROPS BYTES keeps every backup, even under a window. Losing
// the original of a file whose damaged span was just discarded leaves the
// operator with nothing at all.
func TestResyncRepair_KeepsItsBackupsDespiteTheWindow(t *testing.T) {
	svc, reidx := backupPolicyLibrary(t)

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "",
		appmaintenance.ReindexRunOptions{Resync: true, BackupWindow: 1}))
	waitIdle(t, svc)

	assert.Len(t, reidx.reindexed, 3, "all three must be repaired")
	assert.Empty(t, reidx.deleted,
		"a repair that discards bytes must never have its only rollback copy purged")
}

// The window still does its job for a content-preserving reindex: that is what
// it is for, and a volume must not fill up with copies of files whose picture
// and sound were never touched.
func TestPlainReindex_StillPurgesUnderTheWindow(t *testing.T) {
	svc, reidx := backupPolicyLibrary(t)

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "",
		appmaintenance.ReindexRunOptions{BackupWindow: 1}))
	waitIdle(t, svc)

	assert.Len(t, reidx.reindexed, 3)
	assert.NotEmpty(t, reidx.deleted, "the window must still bound a lossless repair")
}

// Trading the backup away is refused for a repair that alters the content:
// deep-verify proves the new file is well formed, never that the operator
// wanted what it now contains.
func TestResyncRepair_RefusesToRunWithoutABackup(t *testing.T) {
	svc, reidx := backupPolicyLibrary(t)

	err := svc.StartReindexLibrary(context.Background(), "",
		appmaintenance.ReindexRunOptions{Resync: true, SkipBackup: true})
	require.Error(t, err)
	require.ErrorIs(t, err, appmaintenance.ErrBackupRequired)
	assert.Empty(t, reidx.reindexed, "nothing may be rewritten once the run is refused")
}
