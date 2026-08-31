package maintenance_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	"github.com/evey-project/evey/internal/domain/mkv"
)

// The page owes ONE answer per file. This table is that answer.
func TestSeekStatus_Verdict(t *testing.T) {
	cases := map[string]struct {
		st   appmaintenance.SeekStatus
		want appmaintenance.Verdict
	}{
		"healthy":                      {appmaintenance.SeekStatus{HasIndex: true}, appmaintenance.VerdictHealthy},
		"no index":                     {appmaintenance.SeekStatus{HasIndex: false}, appmaintenance.VerdictRepairable},
		"damaged, index fine":          {appmaintenance.SeekStatus{HasIndex: true, DamagedBytes: 512}, appmaintenance.VerdictRepairable},
		"audio delayed":                {appmaintenance.SeekStatus{HasIndex: true, AudioDelaySec: 0.9}, appmaintenance.VerdictRepairable},
		"wrong container":              {appmaintenance.SeekStatus{HasIndex: true, WrongContainer: true}, appmaintenance.VerdictRepairable},
		"truncated wins over no index": {appmaintenance.SeekStatus{HasIndex: false, Truncated: true}, appmaintenance.VerdictManual},
		"rebuilt and still coarse":     {appmaintenance.SeekStatus{HasIndex: false, IndexSparse: true, ReindexAttempted: true}, appmaintenance.VerdictManual},
		"sparse, never rebuilt":        {appmaintenance.SeekStatus{HasIndex: false, IndexSparse: true}, appmaintenance.VerdictRepairable},
		"audio short":                  {appmaintenance.SeekStatus{HasIndex: true, AudioShortfallSec: 40}, appmaintenance.VerdictInformational},
		"trailing junk":                {appmaintenance.SeekStatus{HasIndex: true, TrailingJunk: true}, appmaintenance.VerdictInformational},
		"junk plus audio delay":        {appmaintenance.SeekStatus{HasIndex: true, TrailingJunk: true, AudioDelaySec: 0.5}, appmaintenance.VerdictRepairable},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, c.want, c.st.Verdict())
		})
	}
	assert.False(t, appmaintenance.SeekStatus{HasIndex: false}.Ackable(), "a repairable defect is repaired, not acknowledged")
	assert.True(t, appmaintenance.SeekStatus{HasIndex: true, TrailingJunk: true}.Ackable())
	assert.True(t, appmaintenance.SeekStatus{Truncated: true}.Ackable())
}

// triageLibrary: one file per verdict, diagnosed by the fake diagnoser.
func triageLibrary(t *testing.T) (dir string, paths map[string]string, svc *appmaintenance.SeekIndexService, reidx *fakeReindexer, remux *fakeAudioRemuxer, reloc *fakeRelocator) {
	t.Helper()
	dir = t.TempDir()
	paths = map[string]string{}
	for _, n := range []string{"noindex", "damaged", "delayed", "wrongct", "truncated", "short", "junk", "healthy"} {
		paths[n] = filepath.Join(dir, n+".mkv")
		require.NoError(t, os.WriteFile(paths[n], []byte("x"), 0o644))
	}
	reidx = &fakeReindexer{hasIndex: map[string]bool{}}
	var refs []appmaintenance.MediaRef
	for n, p := range paths {
		refs = append(refs, appmaintenance.MediaRef{MediaID: "m-" + n, Title: n, Path: p})
	}
	diag := fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{
		paths["noindex"]:   {Findings: []mkv.DiagnosisFinding{{Kind: "no-index"}}},
		paths["damaged"]:   {Findings: []mkv.DiagnosisFinding{{Kind: "damaged"}}, DamagedBytes: 4096},
		paths["delayed"]:   {Findings: []mkv.DiagnosisFinding{{Kind: "audio-delay"}}, AudioDelaysMs: map[uint64]int64{2: 900}},
		paths["wrongct"]:   {Findings: []mkv.DiagnosisFinding{{Kind: "wrong-container"}}},
		paths["truncated"]: {Findings: []mkv.DiagnosisFinding{{Kind: "no-index"}, {Kind: "damaged"}}, DamagedBytes: 16, Truncated: true},
		paths["short"]:     {Healthy: true},
		paths["junk"]:      {Findings: []mkv.DiagnosisFinding{{Kind: "trailing-junk"}}},
		paths["healthy"]:   {Healthy: true},
	}}
	// A rewrite really repairs the file: once rebuilt, the diagnoser must see
	// a healthy one (the service re-diagnoses after every rewrite).
	reidx.onReindexed = func(p string) { diag.byPath[p] = &mkv.Diagnosis{Healthy: true} }
	remux = &fakeAudioRemuxer{}
	reloc = &fakeRelocator{}
	svc = appmaintenance.NewSeekIndexService(reidx, fakeCatalog{files: refs}, nil).
		WithDiagnoser(diag).
		WithAudioDelayProber(fakeAudioDelayProber{delays: map[string]float64{paths["delayed"]: 0.9}}).
		WithAudioTailProber(&fakeAudioTailProber{gaps: map[string]float64{paths["short"]: 42}}).
		WithAudioRemuxer(remux).
		WithMediaRelocator(reloc)
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	return dir, paths, svc, reidx, remux, reloc
}

type fakeRelocator struct {
	mu    sync.Mutex
	moved map[string]string // mediaID -> new path
	fail  error
}

func (f *fakeRelocator) RelocateMediaFile(_ context.Context, mediaID, newPath string) error {
	if f.fail != nil {
		return f.fail
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.moved == nil {
		f.moved = map[string]string{}
	}
	f.moved[mediaID] = newPath
	return nil
}

func TestSeekIndexService_Scan_RollsUpTheTriage(t *testing.T) {
	_, _, svc, _, _, _ := triageLibrary(t)
	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	assert.Equal(t, 8, res.Total)
	// noindex, damaged, delayed, wrongct
	assert.Equal(t, 4, res.Repairable, "exactly what Repair all acts on")
	// truncated (rebuilt-and-coarse needs a repair first, none here)
	assert.Equal(t, 1, res.Manual)
	// short, junk
	assert.Equal(t, 2, res.Informational)
	assert.Zero(t, res.Acked)
}

func TestSeekIndexService_Ack_MovesFilesOutOfTheQueue(t *testing.T) {
	_, paths, svc, _, _, _ := triageLibrary(t)
	ctx := context.Background()

	// A fixable defect is never waved through.
	require.ErrorIs(t, svc.AckMedia(ctx, "m-noindex", true), appmaintenance.ErrAckNotApplicable)
	require.ErrorIs(t, svc.AckMedia(ctx, "m-unknown", true), appmaintenance.ErrNotInScan)

	require.NoError(t, svc.AckMedia(ctx, "m-junk", true))
	res, _, _ := svc.CachedScan("")
	assert.Equal(t, 1, res.Informational, "junk left the pending pile")
	assert.Equal(t, 1, res.Acked)
	for _, it := range res.Items {
		if it.Path == paths["junk"] {
			assert.True(t, it.Acked())
		}
	}

	// Withdrawing puts it back.
	require.NoError(t, svc.AckMedia(ctx, "m-junk", false))
	res, _, _ = svc.CachedScan("")
	assert.Equal(t, 2, res.Informational)
	assert.Zero(t, res.Acked)

	// A whole group at once - and a repairable group is refused.
	n, err := svc.AckGroup(ctx, "", appmaintenance.VerdictManual)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	_, err = svc.AckGroup(ctx, "", appmaintenance.VerdictRepairable)
	require.ErrorIs(t, err, appmaintenance.ErrAckNotApplicable)
	res, _, _ = svc.CachedScan("")
	assert.Zero(t, res.Manual)
	assert.Equal(t, 1, res.Acked)
}

// The acknowledgement is a judgement on THESE bytes: it survives restarts and
// unchanged re-scans, and vanishes the moment the file changes.
func TestSeekIndexService_Ack_PersistsUntilTheFileChanges(t *testing.T) {
	dir := t.TempDir()
	junk := filepath.Join(dir, "junk.mkv")
	require.NoError(t, os.WriteFile(junk, []byte("x"), 0o644))
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(junk, old, old))
	state := t.TempDir()
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: junk}}}
	diag := fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{junk: {Findings: []mkv.DiagnosisFinding{{Kind: "trailing-junk"}}}}}
	newSvc := func() *appmaintenance.SeekIndexService {
		return appmaintenance.NewSeekIndexService(&fakeReindexer{hasIndex: map[string]bool{junk: true}}, cat, nil).
			WithDiagnoser(diag).WithStateDir(state)
	}
	svc := newSvc()
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	require.NoError(t, svc.AckMedia(context.Background(), "m1", true))

	// Restart + unchanged re-scan: still acknowledged.
	svc2 := newSvc()
	res, _, ok := svc2.CachedScan("")
	require.True(t, ok)
	assert.Equal(t, 1, res.Acked, "the acknowledgement is persisted")
	require.NoError(t, svc2.StartScan(context.Background(), ""))
	waitIdle(t, svc2)
	res, _, _ = svc2.CachedScan("")
	assert.Equal(t, 1, res.Acked, "an unchanged file keeps its acknowledgement across scans")

	// The file changes (re-acquired): judged afresh.
	require.NoError(t, os.WriteFile(junk, []byte("xy"), 0o644))
	require.NoError(t, svc2.StartScan(context.Background(), ""))
	waitIdle(t, svc2)
	res, _, _ = svc2.CachedScan("")
	assert.Zero(t, res.Acked, "new bytes, new verdict, no stale acknowledgement")
	assert.Equal(t, 1, res.Informational)
}

// "Repair all" repairs everything repairable - no hidden checkbox - except the
// lossy resync of damaged files, which needs the operator's explicit consent.
func TestSeekIndexService_RepairAll_CoversEveryRepairableVerdict(t *testing.T) {
	_, paths, svc, reidx, remux, reloc := triageLibrary(t)
	ctx := context.Background()

	require.NoError(t, svc.StartRepairLibrary(ctx, "", appmaintenance.ReindexRunOptions{}))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Zero(t, st.Last.Failed, "%+v", st.Last.Failures)
	assert.Equal(t, []string{paths["noindex"]}, reidx.reindexed, "index rebuilt; the damaged file waits for consent, truncated is never touched")
	assert.Equal(t, []string{paths["delayed"]}, remux.remuxed, "audio repaired without being asked twice")
	assert.Equal(t, filepath.Join(filepath.Dir(paths["wrongct"]), "wrongct.mp4"), reloc.moved["m-wrongct"], "the .mkv that is an MP4 is renamed and the catalog follows")
	_, statErr := os.Stat(paths["wrongct"])
	assert.True(t, os.IsNotExist(statErr), "old .mkv path gone")
	assert.Equal(t, 3, st.Last.Total, "1 index + 1 audio + 1 container")

	res, _, _ := svc.CachedScan("")
	assert.Equal(t, 7, res.Total, "the renamed file is no longer the scan's business")
	assert.Equal(t, 1, res.Repairable, "only the damaged file (awaiting consent) remains repairable")

	// With consent, the damaged file is resynced too - and keeps its backup.
	reidx.reindexed = nil
	require.NoError(t, svc.StartRepairLibrary(ctx, "", appmaintenance.ReindexRunOptions{ResyncDamaged: true, BackupWindow: 1}))
	waitIdle(t, svc)
	assert.Equal(t, []string{paths["damaged"]}, reidx.reindexed)
	res, _, _ = svc.CachedScan("")
	assert.Zero(t, res.Repairable)
	for _, it := range res.Items {
		if it.Path == paths["damaged"] {
			assert.True(t, it.HasBackup, "a lossy repair keeps its original whatever the window says")
		}
	}
}

// Without a relocator the rename would orphan the catalog entry: the repair
// is simply not offered, and the file stays a manual chore.
func TestSeekIndexService_RepairAll_NoRelocator_LeavesWrongContainerAlone(t *testing.T) {
	dir := t.TempDir()
	wrong := filepath.Join(dir, "w.mkv")
	require.NoError(t, os.WriteFile(wrong, []byte("x"), 0o644))
	svc := appmaintenance.NewSeekIndexService(&fakeReindexer{hasIndex: map[string]bool{}}, fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: wrong}}}, nil).
		WithDiagnoser(fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{wrong: {Findings: []mkv.DiagnosisFinding{{Kind: "wrong-container"}}}}})
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	assert.False(t, svc.SupportsContainerFix())
	require.NoError(t, svc.StartRepairLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)
	_, err := os.Stat(wrong)
	assert.NoError(t, err, "file untouched")
}

// A catalog that refuses the new path must not leave disk and database apart.
func TestSeekIndexService_FixContainer_RenamesBackWhenCatalogRefuses(t *testing.T) {
	dir := t.TempDir()
	wrong := filepath.Join(dir, "w.mkv")
	require.NoError(t, os.WriteFile(wrong, []byte("x"), 0o644))
	reloc := &fakeRelocator{fail: os.ErrPermission}
	svc := appmaintenance.NewSeekIndexService(&fakeReindexer{hasIndex: map[string]bool{}}, fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: wrong}}}, nil).
		WithDiagnoser(fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{wrong: {Findings: []mkv.DiagnosisFinding{{Kind: "wrong-container"}}}}}).
		WithMediaRelocator(reloc)
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	require.NoError(t, svc.StartFixContainerMedia(context.Background(), "m1"))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, 1, st.Last.Failed)
	_, err := os.Stat(wrong)
	require.NoError(t, err, "renamed back: the .mkv is where the catalog says it is")
	_, err = os.Stat(filepath.Join(dir, "w.mp4"))
	assert.True(t, os.IsNotExist(err))
}

// A detector fix must reach the files already diagnosed. The incremental
// re-scan reuses a status while the file's fingerprint holds - right for the
// file, wrong for the detector: a probe corrected in July kept a month of
// stale false positives alive in production, because no unchanged file was
// ever re-judged. A status from an older detector version is re-diagnosed.
func TestSeekIndexService_Rescan_RediagnosesStatusesFromOlderDetectors(t *testing.T) {
	dir := t.TempDir()
	stable := filepath.Join(dir, "stable.mkv")
	require.NoError(t, os.WriteFile(stable, []byte("x"), 0o644))
	old := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(stable, old, old))
	state := t.TempDir()
	diag := fakeDiagnoser{calls: map[string]int{}, byPath: map[string]*mkv.Diagnosis{stable: {Healthy: true}}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: stable}}}
	newSvc := func() *appmaintenance.SeekIndexService {
		return appmaintenance.NewSeekIndexService(&fakeReindexer{hasIndex: map[string]bool{stable: true}}, cat, nil).
			WithDiagnoser(diag).WithStateDir(state)
	}
	svc := newSvc()
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	require.Equal(t, 1, diag.calls[stable])

	// Same file, same detectors: reused, as before.
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	require.Equal(t, 1, diag.calls[stable], "unchanged file + unchanged detectors = reused")
	res, _, _ := svc.CachedScan("")
	assert.Equal(t, appmaintenance.SeekDiagVersion, res.Items[0].DiagVersion)

	// Simulate a status persisted by an older detector version (or before the
	// field existed): the fingerprint still holds, the verdict must not.
	raw, err := os.ReadFile(filepath.Join(state, "scans.json"))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"DiagVersion":`)
	stale := strings.ReplaceAll(string(raw), `"DiagVersion":`+strconv.Itoa(appmaintenance.SeekDiagVersion), `"DiagVersion":1`)
	require.NoError(t, os.WriteFile(filepath.Join(state, "scans.json"), []byte(stale), 0o644))

	svc2 := newSvc()
	require.NoError(t, svc2.StartScan(context.Background(), ""))
	waitIdle(t, svc2)
	assert.Equal(t, 2, diag.calls[stable], "an older detector's verdict is re-judged even though the bytes did not change")
	res, _, _ = svc2.CachedScan("")
	assert.Equal(t, appmaintenance.SeekDiagVersion, res.Items[0].DiagVersion, "and re-stamped")
}

// The operator sees mkvgo's own account of a sparse index, not a generic story
// about keyframes: live, 3 of 5 "coarse" files had cues every 2-10 s and the
// hole was the final credits alone.
func TestSeekIndexService_Scan_CarriesTheSparseDetail(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sparse.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	diag := fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{p: {Findings: []mkv.DiagnosisFinding{{Kind: "index-sparse", Detail: "the video cues leave a 44s hole - a seek there lands that far from its target"}}}}}
	svc := appmaintenance.NewSeekIndexService(&fakeReindexer{hasIndex: map[string]bool{p: true}}, fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: p}}}, nil).WithDiagnoser(diag)
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	res, _, _ := svc.CachedScan("")
	require.Len(t, res.Items, 1)
	assert.True(t, res.Items[0].IndexSparse)
	assert.Contains(t, res.Items[0].IndexSparseDetail, "44s hole")
}

// mkvgo (ticket 2026-08-31) tells a hole in the cues from a hole in the
// PICTURE: the latter has nothing to index, so it goes straight to the
// operator - no "rebuild once, then give up" round trip, no wasted rewrite.
func TestSeekIndexService_PictureMissing_IsManualWithoutARebuild(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "holes.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	reidx := &fakeReindexer{hasIndex: map[string]bool{p: true}}
	diag := fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{p: {Findings: []mkv.DiagnosisFinding{{
		Kind: "index-sparse", Detail: "65s hole at 00:40:34 - the picture itself is missing there (183s short)",
		Remedy: "re-acquire the source: the video track has no picture in the hole",
	}}}}}
	svc := appmaintenance.NewSeekIndexService(reidx, fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: p}}}, nil).WithDiagnoser(diag)
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	res, _, _ := svc.CachedScan("")
	require.Len(t, res.Items, 1)
	assert.True(t, res.Items[0].PictureMissing)
	assert.Equal(t, appmaintenance.VerdictManual, res.Items[0].Verdict(), "nothing to index: the operator's call, at once")
	assert.Equal(t, 1, res.Manual)
	assert.Zero(t, res.Repairable)

	require.NoError(t, svc.StartRepairLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)
	assert.Empty(t, reidx.reindexed, "no rewrite for a hole that has no picture to index")
}
