package maintenance_test

import (
	"context"
	"fmt"
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
		"wrong container":              {appmaintenance.SeekStatus{HasIndex: true, WrongContainer: true}, appmaintenance.VerdictRepairable},
		"truncated wins over no index": {appmaintenance.SeekStatus{HasIndex: false, Truncated: true}, appmaintenance.VerdictInformational},
		"rebuilt and still coarse":     {appmaintenance.SeekStatus{HasIndex: false, IndexSparse: true, ReindexAttempted: true}, appmaintenance.VerdictInformational},
		"sparse, never rebuilt":        {appmaintenance.SeekStatus{HasIndex: false, IndexSparse: true}, appmaintenance.VerdictRepairable},
		"audio short":                  {appmaintenance.SeekStatus{HasIndex: true, AudioShortfallSec: 40}, appmaintenance.VerdictInformational},
		"trailing junk":                {appmaintenance.SeekStatus{HasIndex: true, TrailingJunk: true}, appmaintenance.VerdictInformational},
		"audio delayed":                {appmaintenance.SeekStatus{HasIndex: true, AudioDelaySec: 0.5}, appmaintenance.VerdictInformational},
		"junk plus audio delay":        {appmaintenance.SeekStatus{HasIndex: true, TrailingJunk: true, AudioDelaySec: 0.5}, appmaintenance.VerdictInformational},
		"audio tracks disagree":        {appmaintenance.SeekStatus{HasIndex: true, AudioTracksLagSec: 11.9, AudioTracksChecked: true}, appmaintenance.VerdictInformational},
		"offset proven coherent":       {appmaintenance.SeekStatus{HasIndex: true, AudioDelaySec: 12, AudioTracksLagSec: 0, AudioTracksChecked: true}, appmaintenance.VerdictInformational},
		"retime fingerprint":           {appmaintenance.SeekStatus{HasIndex: true, AudioTrackZeroed: true}, appmaintenance.VerdictInformational},
		"chunked interleave":           {appmaintenance.SeekStatus{HasIndex: true, InterleaveBackstepSec: 366}, appmaintenance.VerdictRepairable},
		"sound off the picture":        {appmaintenance.SeekStatus{HasIndex: true, AudioPictureChecked: true, AudioPictureLagSec: 0.9}, appmaintenance.VerdictInformational},
		"sound on the picture":         {appmaintenance.SeekStatus{HasIndex: true, AudioPictureChecked: true, AudioPictureLagSec: -0.04}, appmaintenance.VerdictHealthy},
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

// triageProbers keeps the fixture's fakes reachable for the tests that change
// a file after the scan (the diagnoser and the re-mux probe decide what the
// re-diagnosis sees). Set by triageLibrary; the tests are sequential.
var triageProbers struct {
	diag  fakeDiagnoser
	reint *fakeReinterleaver
}

func diagOf(t *testing.T) fakeDiagnoser {
	t.Helper()
	return triageProbers.diag
}

func reinterleaverOf(t *testing.T) *fakeReinterleaver {
	t.Helper()
	return triageProbers.reint
}

// triageLibrary: one file per verdict, diagnosed by the fake diagnoser.
func triageLibrary(t *testing.T) (dir string, paths map[string]string, svc *appmaintenance.SeekIndexService, reidx *fakeReindexer, reloc *fakeRelocator) {
	t.Helper()
	dir = t.TempDir()
	paths = map[string]string{}
	for _, n := range []string{"noindex", "damaged", "delayed", "wrongct", "truncated", "short", "junk", "chunked", "early", "healthy"} {
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
		paths["short"]:     {Healthy: true, AudioShortfallMs: 42_000},
		paths["junk"]:      {Findings: []mkv.DiagnosisFinding{{Kind: "trailing-junk"}}},
		paths["chunked"]:   {Healthy: true},
		paths["early"]:     {Healthy: true},
		paths["healthy"]:   {Healthy: true},
	}}
	// A rewrite really repairs the file: once rebuilt, the diagnoser must see
	// a healthy one (the service re-diagnoses after every rewrite).
	reidx.onReindexed = func(p string) { diag.byPath[p] = &mkv.Diagnosis{Healthy: true} }
	reloc = &fakeRelocator{}
	// The re-mux "fixes" the file: the probe must then read 0 on it.
	probe := fakeInterleaveProber{backsteps: map[string]float64{paths["chunked"]: 366, paths["healthy"]: 4}}
	reint := &fakeReinterleaver{probe: probe}
	triageProbers.diag, triageProbers.reint = diag, reint
	svc = appmaintenance.NewSeekIndexService(reidx, fakeCatalog{files: refs}, nil).
		WithDiagnoser(diag).
		WithAudioDelayProber(fakeAudioDelayProber{delays: map[string]float64{paths["delayed"]: 0.9}}).
		// Every header of the chunked file is healthy; only the sampled
		// cluster order tells it apart.
		WithInterleaveProber(probe).
		WithReinterleaver(reint).
		// Sound pulled 0.9 s early by an ffmpeg remux: the tracks still agree
		// with each other and the container declares nothing; only the
		// picture measure sees it - and only ffmpeg-written files get it.
		WithMuxerProber(fakeMuxerProber{apps: map[string]string{paths["early"]: "Lavf61.7.103", paths["healthy"]: "libebml v1.4.4 + libmatroska v1.7.1"}}).
		WithAudioPictureProber(fakeAudioPictureProber{lags: map[string][]appmaintenance.AudioPictureLag{
			paths["early"]:   {{Track: 0, LagSec: 0.9, Confidence: 0.3, Windows: 2}, {Track: 1, LagSec: 0.88, Confidence: 0.4, Windows: 3}},
			paths["healthy"]: {{Track: 0, LagSec: 5, Confidence: 0.9, Windows: 3}}, // never measured: not ffmpeg-written
		}}).
		WithMediaRelocator(reloc)
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	return dir, paths, svc, reidx, reloc
}

type fakeInterleaveProber struct {
	backsteps map[string]float64
	errs      map[string]error // a path whose probe fails outright
}

func (f fakeInterleaveProber) ProbeInterleaveBackstepSec(_ context.Context, p string) (float64, error) {
	if err := f.errs[p]; err != nil {
		return 0, err
	}
	return f.backsteps[p], nil
}

type fakeMuxerProber struct{ apps map[string]string }

func (f fakeMuxerProber) ProbeMuxingApp(_ context.Context, p string) (string, error) {
	return f.apps[p], nil
}

type fakeAudioPictureProber struct {
	lags map[string][]appmaintenance.AudioPictureLag
}

func (f fakeAudioPictureProber) ProbeAudioPictureLag(_ context.Context, p string) ([]appmaintenance.AudioPictureLag, error) {
	return f.lags[p], nil
}

type fakeReinterleaver struct {
	mu    sync.Mutex
	done  []string
	probe fakeInterleaveProber
}

func (f *fakeReinterleaver) Reinterleave(_ context.Context, p string) (appmaintenance.ReinterleaveResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.done = append(f.done, p)
	if f.probe.backsteps != nil {
		f.probe.backsteps[p] = 0
	}
	return appmaintenance.ReinterleaveResult{BackupPath: p + ".mkvgobak"}, nil
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
	_, paths, svc, _, _ := triageLibrary(t)
	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	assert.Equal(t, 10, res.Total)
	// noindex, damaged, wrongct, chunked - the delayed file is NOT here any
	// more: a start offset is a fact, not something Repair all rewrites.
	assert.Equal(t, 4, res.Repairable, "exactly what Repair all acts on")
	// short, junk, delayed, early + truncated: everything Evey cannot fix
	// here is one and the same answer now - a remark, never an instruction.
	assert.Equal(t, 5, res.Informational)
	assert.Equal(t, 1, res.AudioPictureOff)
	for _, it := range res.Items {
		switch it.Path {
		case paths["early"]:
			assert.True(t, it.MuxedByFFmpeg)
			assert.InDelta(t, 0.9, it.AudioPictureLagSec, 1e-9, "the worst track decides the file")
		case paths["healthy"]:
			assert.False(t, it.AudioPictureChecked, "an mkvmerge file is not measured")
		}
	}
	assert.Equal(t, 1, res.BrokenInterleave, "a 4 s backstep on the healthy file stays under the floor")
	assert.Zero(t, res.Acked)
}

func TestSeekIndexService_Ack_MovesFilesOutOfTheQueue(t *testing.T) {
	_, paths, svc, _, _ := triageLibrary(t)
	ctx := context.Background()

	// A fixable defect is never waved through.
	require.ErrorIs(t, svc.AckMedia(ctx, "m-noindex", true), appmaintenance.ErrAckNotApplicable)
	require.ErrorIs(t, svc.AckMedia(ctx, "m-unknown", true), appmaintenance.ErrNotInScan)

	require.NoError(t, svc.AckMedia(ctx, "m-junk", true))
	res, _, _ := svc.CachedScan("")
	// short, delayed, early - junk left the pending pile (the offset file is
	// informational now: a start offset is a fact, never a repair).
	assert.Equal(t, 4, res.Informational, "junk left the pending pile")
	assert.Equal(t, 1, res.Acked)
	for _, it := range res.Items {
		if it.Path == paths["junk"] {
			assert.True(t, it.Acked())
		}
	}

	// Withdrawing puts it back.
	require.NoError(t, svc.AckMedia(ctx, "m-junk", false))
	res, _, _ = svc.CachedScan("")
	assert.Equal(t, 5, res.Informational)
	assert.Zero(t, res.Acked)

	// A whole group at once - and a repairable group is refused.
	n, err := svc.AckGroup(ctx, "", appmaintenance.VerdictInformational)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	_, err = svc.AckGroup(ctx, "", appmaintenance.VerdictRepairable)
	require.ErrorIs(t, err, appmaintenance.ErrAckNotApplicable)
	res, _, _ = svc.CachedScan("")
	assert.Zero(t, res.Informational)
	assert.Equal(t, 5, res.Acked)
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
// lossy resync of damaged files, which needs the operator's explicit consent,
// and the audio START OFFSET, which is not a defect at all (see
// TestSeekIndexService_AudioOffsetIsNeverRepaired).
func TestSeekIndexService_RepairAll_CoversEveryRepairableVerdict(t *testing.T) {
	_, paths, svc, reidx, reloc := triageLibrary(t)
	ctx := context.Background()

	require.NoError(t, svc.StartRepairLibrary(ctx, "", appmaintenance.ReindexRunOptions{}))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Zero(t, st.Last.Failed, "%+v", st.Last.Failures)
	assert.Equal(t, []string{paths["noindex"]}, reidx.reindexed, "index rebuilt; the damaged file waits for consent, truncated is never touched, and a start offset is a fact about the file, not something a bulk job rewrites")
	assert.Equal(t, filepath.Join(filepath.Dir(paths["wrongct"]), "wrongct.mp4"), reloc.moved["m-wrongct"], "the .mkv that is an MP4 is renamed and the catalog follows")
	_, statErr := os.Stat(paths["wrongct"])
	assert.True(t, os.IsNotExist(statErr), "old .mkv path gone")
	assert.Equal(t, 3, st.Last.Total, "1 index + 1 container + 1 re-mux - the offset file is deliberately left alone")

	res, _, _ := svc.CachedScan("")
	assert.Equal(t, 9, res.Total, "the renamed file is no longer the scan's business")
	assert.Equal(t, 1, res.Repairable, "only the damaged file (awaiting consent) remains repairable")
	for _, it := range res.Items {
		if it.Path == paths["chunked"] {
			assert.Zero(t, it.InterleaveBackstepSec, "the re-muxed file is no longer stored in blocks")
			assert.True(t, it.HasBackup, "its original is kept")
		}
	}

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

// The scan is head-mostly on purpose, so a corrupted cluster stream is only
// ever discovered BY a repair - and before the answer was recorded, the file
// failed the same way on every run for ever: "Repair all" kept sending it down
// the strict path that had just refused it, and the operator got a raw parser
// message with nothing naming the remedy. Reported from prod on 2026-09-07
// ("top-level element: invalid VINT: leading zero byte ... the source is
// structurally corrupt", one of three failures in a four-file batch).
func TestSeekIndexService_StrictRepairRefusal_RemembersThatOnlyAResyncCanHelp(t *testing.T) {
	_, paths, svc, reidx, _ := triageLibrary(t)
	ctx := context.Background()
	corrupt := paths["noindex"]
	reidx.failStrictWith = map[string]error{
		corrupt: fmt.Errorf("reindex: rebuild: top-level element: invalid VINT: %w", mkv.ErrCorruptSource),
	}

	// First run: the strict rewrite refuses it, as it did in prod.
	require.NoError(t, svc.StartRepairLibrary(ctx, "", appmaintenance.ReindexRunOptions{}))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, 1, st.Last.Failed, "the corrupt file failed: %+v", st.Last.Failures)

	res, _, _ := svc.CachedScan("")
	var got appmaintenance.SeekStatus
	for _, it := range res.Items {
		if it.Path == corrupt {
			got = it
		}
	}
	assert.True(t, got.ResyncRequired, "the refusal names its own remedy - throwing it away is what made the file fail for ever")
	assert.Zero(t, got.DamagedBytes, "how much a resync would drop is unknown here and must never be invented: the confirmation dialog puts that figure in front of the operator")
	assert.Equal(t, appmaintenance.VerdictRepairable, got.Verdict())

	// Second run, still strict: the file is no longer offered to the path that
	// is known to refuse it, so the batch stops reporting the same failure.
	reidx.reindexed = nil
	require.NoError(t, svc.StartRepairLibrary(ctx, "", appmaintenance.ReindexRunOptions{}))
	st = waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Zero(t, st.Last.Failed, "%+v", st.Last.Failures)
	assert.NotContains(t, reidx.reindexed, corrupt)

	// With the operator's resync consent it is finally repaired - and the flag
	// clears, so a repaired file does not stay pinned in the resync bucket.
	reidx.reindexed = nil
	require.NoError(t, svc.StartRepairLibrary(ctx, "", appmaintenance.ReindexRunOptions{ResyncDamaged: true, BackupWindow: 1}))
	waitIdle(t, svc)
	assert.Contains(t, reidx.reindexed, corrupt)
	res, _, _ = svc.CachedScan("")
	for _, it := range res.Items {
		if it.Path == corrupt {
			assert.False(t, it.ResyncRequired, "the rewrite went through: whatever refused it is not in the file any more")
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
	assert.Equal(t, appmaintenance.VerdictInformational, res.Items[0].Verdict(), "nothing to index: a remark, not a repair")
	assert.Equal(t, 1, res.Informational)
	assert.Zero(t, res.Repairable)

	require.NoError(t, svc.StartRepairLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)
	assert.Empty(t, reidx.reindexed, "no rewrite for a hole that has no picture to index")
}

// A declared audio start offset is a FACT about the file, never a defect to
// rewrite. A dubbed film (prod 2026-07-13) carried a deliberate 12.000 s
// alignment for its French dub; "repair all" retimed it to zero, the dub then
// played 12 s early in every player, and no backup existed. These two tests are
// that incident, frozen: the verdict must not be repairable, and the bulk job
// must not touch the file.
func TestSeekIndexService_AudioOffsetIsNeverRepaired(t *testing.T) {
	dir := t.TempDir()
	dub := filepath.Join(dir, "deliberate_dub_offset.mkv")
	require.NoError(t, os.WriteFile(dub, []byte("x"), 0o644))
	reidx := &fakeReindexer{hasIndex: map[string]bool{dub: true}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m-dub", Title: "Some Movie", Path: dub}}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).
		WithDiagnoser(fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{
			// 12 seconds: exactly the alignment the repair destroyed.
			dub: {Healthy: true, AudioDelaysMs: map[uint64]int64{2: 12_000}},
		}}).
		WithAudioDelayProber(fakeAudioDelayProber{delays: map[string]float64{dub: 12.0}})

	ctx := context.Background()
	require.NoError(t, svc.StartScan(ctx, ""))
	waitIdle(t, svc)

	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	require.Len(t, res.Items, 1)
	assert.InDelta(t, 12.0, res.Items[0].AudioDelaySec, 1e-9, "the offset is still measured and shown")
	assert.NotEqual(t, appmaintenance.VerdictRepairable, res.Items[0].Verdict(),
		"a start offset alone must never read as repairable")
	assert.Zero(t, res.Repairable, "and it must not be counted in what Repair all will act on")

	require.NoError(t, svc.StartRepairLibrary(ctx, "", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)
	assert.Empty(t, reidx.reindexed, "Repair all must not rewrite a file for a start offset - there is no longer any code path that could")
}

// The offset is only WORTH MENTIONING when the file's OWN audio tracks
// disagree - and even then nobody here can tell which track is misplaced, so
// it stays a remark and never becomes an automatic rewrite.
func TestSeekIndexService_IncoherentAudioTracksAreOnlyEverReported(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "tracks_disagree.mkv")
	require.NoError(t, os.WriteFile(broken, []byte("x"), 0o644))
	st := appmaintenance.SeekStatus{
		Path: broken, HasIndex: true,
		AudioDelaySec: 0, AudioTracksLagSec: 11.94, AudioTracksChecked: true,
	}
	assert.Equal(t, appmaintenance.VerdictInformational, st.Verdict(),
		"tracks that disagree are worth SAYING, and nothing more")
	assert.True(t, st.Ackable(), "and the operator can wave it through")

	coherent := appmaintenance.SeekStatus{
		Path: broken, HasIndex: true,
		AudioDelaySec: 12.0, AudioTracksLagSec: 0, AudioTracksChecked: true,
	}
	assert.Equal(t, appmaintenance.VerdictInformational, coherent.Verdict(),
		"a large offset PROVEN coherent is information, nothing more")
}

// A repair acts on the bytes on disk, not on a verdict about a file that no
// longer exists. Scan at 18:00 says "MP4 mislabelled .mkv"; at 19:00 an *arr
// upgrade drops a genuine MKV under the same name; "Repair all" at 20:00
// reuses the scan. The rename used to go through on the old verdict.
func TestSeekIndexService_RepairRefusesAStaleDiagnosis(t *testing.T) {
	_, paths, svc, _, reloc := triageLibrary(t)
	ctx := context.Background()

	// The file is replaced: new bytes, and the diagnoser now sees a sound MKV.
	require.NoError(t, os.WriteFile(paths["wrongct"], []byte("a real matroska now"), 0o644))
	diagOf(t).byPath[paths["wrongct"]] = &mkv.Diagnosis{Healthy: true}

	require.NoError(t, svc.StartFixContainerMedia(ctx, "m-wrongct"))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Zero(t, st.Last.Failed, "%+v", st.Last.Failures)
	assert.Equal(t, 1, st.Last.Skipped, "re-diagnosed sound: nothing to rename")
	_, err := os.Stat(paths["wrongct"])
	require.NoError(t, err, "the replaced file keeps its name")
	assert.Empty(t, reloc.moved, "the catalog is not repointed")
	res, _, _ := svc.CachedScan("")
	for _, it := range res.Items {
		if it.Path == paths["wrongct"] {
			assert.False(t, it.WrongContainer, "the cache carries the fresh verdict")
		}
	}
}

// Same invariant on the re-mux: a chunked file replaced by a sound one is
// not re-muxed on the strength of the old probe.
func TestSeekIndexService_ReinterleaveRefusesAStaleDiagnosis(t *testing.T) {
	_, paths, svc, _, _ := triageLibrary(t)
	ctx := context.Background()
	reint := reinterleaverOf(t)
	reint.probe.backsteps[paths["chunked"]] = 0 // the new file reads sane
	require.NoError(t, os.WriteFile(paths["chunked"], []byte("replaced by a sane remux"), 0o644))

	require.NoError(t, svc.StartReinterleaveMedia(ctx, "m-chunked"))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Zero(t, st.Last.Failed, "%+v", st.Last.Failures)
	assert.Equal(t, 1, st.Last.Skipped)
	assert.Empty(t, reint.done, "no re-mux of a file nobody re-diagnosed as chunked")
}
