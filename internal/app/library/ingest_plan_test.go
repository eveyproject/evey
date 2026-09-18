package library

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/mkv"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
)

// --- local fakes: IngestPlanner / mkv.Reindexer ---

type fakeIngestPlanner struct {
	mu       sync.Mutex
	calls    int
	plans    []*library.ServingPlan // consumed in order, last one repeats once exhausted
	lastPath string
	err      error
}

func (f *fakeIngestPlanner) Plan(_ context.Context, path, _ string) (*library.ServingPlan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastPath = path
	if f.err != nil {
		return nil, f.err
	}
	idx := f.calls - 1
	if idx >= len(f.plans) {
		idx = len(f.plans) - 1
	}
	p := *f.plans[idx]
	return &p, nil
}

// ingestFakeReindexer is a minimal mkv.Reindexer stub: only ReindexInPlace's
// call count/error matters to the "plan+repair" mode under test.
type ingestFakeReindexer struct {
	mu       sync.Mutex
	calls    int
	lastPath string
	err      error
}

func (r *ingestFakeReindexer) HasSeekIndex(context.Context, string) (bool, error) { return false, nil }
func (r *ingestFakeReindexer) ReindexInPlace(_ context.Context, path string, _ mkv.ReindexOptions) (mkv.ReindexResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.lastPath = path
	if r.err != nil {
		return mkv.ReindexResult{}, r.err
	}
	return mkv.ReindexResult{Method: mkv.ReindexMethodInPlace}, nil
}
func (r *ingestFakeReindexer) HasBackup(context.Context, string) (bool, error) { return false, nil }
func (r *ingestFakeReindexer) RestoreBackup(context.Context, string) error     { return nil }
func (r *ingestFakeReindexer) DeleteBackup(context.Context, string) error      { return nil }
func (r *ingestFakeReindexer) SupportsInPlace() bool                           { return true }

// --- helpers ---

func newIngestTestScanService() (*ScanService, *libfakes.FakeMediaItemCommandRepo, *libfakes.FakeFileScanner, *libfakes.FakeLibraryQueryRepo) {
	libCmdRepo := libfakes.NewFakeLibraryCommandRepo()
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	mediaRepo := libfakes.NewFakeMediaItemCommandRepo()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	personRepo := libfakes.NewFakePersonRepository()
	mediaPersonRepo := libfakes.NewFakeMediaPersonRepository()
	scanner := libfakes.NewFakeFileScanner()
	prober := libfakes.NewFakeStreamProber()
	search := libfakes.NewFakeSearchIndex()
	bus := sharedfakes.NewFakeEventBus()

	svc := NewScanService(libCmdRepo, libQueryRepo, mediaRepo, streamRepo, personRepo, mediaPersonRepo,
		scanner, prober, search, bus)
	return svc, mediaRepo, scanner, libQueryRepo
}

func mkPlan(target string, strategy library.ServingStrategy, needsReindex bool) *library.ServingPlan {
	return &library.ServingPlan{
		Target: target, Strategy: strategy, NeedsReindex: needsReindex,
	}
}

// spyWarmer records the one keyframe warm-up applyIngestPlan may issue - the
// verdict's only observable consumer besides the reindexer.
type spyWarmer struct{ id, path string }

func (w *spyWarmer) install(svc *ScanService) {
	svc.WithKeyframeWarmer(func(_ context.Context, id, path string) bool {
		w.id, w.path = id, path
		return true
	})
}

// --- loadIngestServingPlanMode ---

func TestLoadIngestServingPlanMode_NilRepo_Off(t *testing.T) {
	svc, _, _, _ := newIngestTestScanService()
	if got := svc.loadIngestServingPlanMode(context.Background()); got != IngestServingPlanOff {
		t.Errorf("mode = %q, want off", got)
	}
}

func TestLoadIngestServingPlanMode_MissingKey_Off(t *testing.T) {
	svc, _, _, _ := newIngestTestScanService()
	svc.ingestCfg = sharedfakes.NewFakeServerConfigRepository()
	if got := svc.loadIngestServingPlanMode(context.Background()); got != IngestServingPlanOff {
		t.Errorf("mode = %q, want off", got)
	}
}

func TestLoadIngestServingPlanMode_InvalidValue_FallsBackOff(t *testing.T) {
	svc, _, _, _ := newIngestTestScanService()
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cfg.Set(context.Background(), IngestServingPlanConfigKey, "bogus")
	svc.ingestCfg = cfg
	if got := svc.loadIngestServingPlanMode(context.Background()); got != IngestServingPlanOff {
		t.Errorf("invalid value: mode = %q, want off", got)
	}
}

func TestLoadIngestServingPlanMode_ReadsFreshEachCall(t *testing.T) {
	svc, _, _, _ := newIngestTestScanService()
	cfg := sharedfakes.NewFakeServerConfigRepository()
	svc.ingestCfg = cfg

	if got := svc.loadIngestServingPlanMode(context.Background()); got != IngestServingPlanOff {
		t.Fatalf("initial mode = %q, want off", got)
	}
	// Admin flips it mid-process, no restart: the very next read must see it,
	// proving there is no long-lived cache.
	cfg.Set(context.Background(), IngestServingPlanConfigKey, IngestServingPlanPlan)
	if got := svc.loadIngestServingPlanMode(context.Background()); got != IngestServingPlanPlan {
		t.Errorf("mode after flip = %q, want plan", got)
	}
}

// --- ingestPause ---

func TestIngestPause_DefaultVsActivePlayback(t *testing.T) {
	svc, _, _, _ := newIngestTestScanService()
	if got := svc.ingestPause(); got != ingestItemPause {
		t.Errorf("no activePlayback wired: pause = %v, want %v", got, ingestItemPause)
	}
	svc.WithIngestActivePlayback(func() int { return 0 })
	if got := svc.ingestPause(); got != ingestItemPause {
		t.Errorf("activePlayback=0: pause = %v, want %v", got, ingestItemPause)
	}
	svc.WithIngestActivePlayback(func() int { return 1 })
	if got := svc.ingestPause(); got != ingestPoliteItemPause {
		t.Errorf("activePlayback=1: pause = %v, want %v", got, ingestPoliteItemPause)
	}
}

// --- applyIngestPlan ---

func TestApplyIngestPlan_ModeOff_NoIngestCall(t *testing.T) {
	svc, _, _, _ := newIngestTestScanService()
	planner := &fakeIngestPlanner{plans: []*library.ServingPlan{mkPlan("mse-generic", library.ServingStrategyDirectPlay, false)}}
	cfg := sharedfakes.NewFakeServerConfigRepository() // no key set -> off
	svc.WithIngestPlanning(cfg, planner, nil)

	svc.applyIngestPlan(context.Background(), "mi-1", "/movies/a.mkv")

	if planner.calls != 0 {
		t.Errorf("planner.calls = %d, want 0 when mode is off", planner.calls)
	}
}

func TestApplyIngestPlan_PlannerNil_NoOp(t *testing.T) {
	svc, _, _, _ := newIngestTestScanService()
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cfg.Set(context.Background(), IngestServingPlanConfigKey, IngestServingPlanPlan)
	svc.WithIngestPlanning(cfg, nil, nil) // no planner wired
	w := &spyWarmer{}
	w.install(svc)

	svc.applyIngestPlan(context.Background(), "mi-1", "/movies/a.mkv") // must not panic

	if w.id != "" {
		t.Errorf("no verdict, no warm-up: warmer called for %q", w.id)
	}
}

func TestApplyIngestPlan_ModePlan_ComputesAndWarms(t *testing.T) {
	svc, _, _, _ := newIngestTestScanService()
	planner := &fakeIngestPlanner{plans: []*library.ServingPlan{mkPlan("mse-generic", library.ServingStrategyRemuxHLS, true)}}
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cfg.Set(context.Background(), IngestServingPlanConfigKey, IngestServingPlanPlan)
	svc.WithIngestPlanning(cfg, planner, nil)
	w := &spyWarmer{}
	w.install(svc)

	svc.applyIngestPlan(context.Background(), "mi-1", "/movies/a.mkv")

	if planner.calls != 1 {
		t.Fatalf("planner.calls = %d, want 1", planner.calls)
	}
	if planner.lastPath != "/movies/a.mkv" {
		t.Errorf("planner path = %q, want /movies/a.mkv", planner.lastPath)
	}
	if w.id != "mi-1" || w.path != "/movies/a.mkv" {
		t.Errorf("a remux-hls verdict warms the keyframes of the item (got %q %q)", w.id, w.path)
	}
}

// plan mode (not plan+repair) must never touch the reindexer, even when the
// computed plan flags NeedsReindex.
func TestApplyIngestPlan_ModePlan_NeverRepairs(t *testing.T) {
	svc, _, _, _ := newIngestTestScanService()
	planner := &fakeIngestPlanner{plans: []*library.ServingPlan{mkPlan("mse-generic", library.ServingStrategyRemuxHLS, true)}}
	reindexer := &ingestFakeReindexer{}
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cfg.Set(context.Background(), IngestServingPlanConfigKey, IngestServingPlanPlan)
	svc.WithIngestPlanning(cfg, planner, reindexer)

	svc.applyIngestPlan(context.Background(), "mi-1", "/movies/a.mkv")

	if reindexer.calls != 0 {
		t.Errorf("reindexer.calls = %d, want 0 in plain 'plan' mode", reindexer.calls)
	}
}

func TestApplyIngestPlan_ModePlanRepair_NeedsReindex_RepairsAndReplans(t *testing.T) {
	svc, _, _, _ := newIngestTestScanService()
	planner := &fakeIngestPlanner{plans: []*library.ServingPlan{
		mkPlan("mse-generic", library.ServingStrategyRemuxHLS, true),  // pre-repair
		mkPlan("mse-generic", library.ServingStrategyRemuxHLS, false), // post-repair
	}}
	reindexer := &ingestFakeReindexer{}
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cfg.Set(context.Background(), IngestServingPlanConfigKey, IngestServingPlanPlanRepair)
	svc.WithIngestPlanning(cfg, planner, reindexer)

	svc.applyIngestPlan(context.Background(), "mi-1", "/movies/a.mkv")

	if reindexer.calls != 1 {
		t.Fatalf("reindexer.calls = %d, want 1", reindexer.calls)
	}
	if reindexer.lastPath != "/movies/a.mkv" {
		t.Errorf("reindexer path = %q, want /movies/a.mkv", reindexer.lastPath)
	}
	if planner.calls != 2 {
		t.Fatalf("planner.calls = %d, want 2 (pre + post repair)", planner.calls)
	}
}

func TestApplyIngestPlan_ModePlanRepair_NoReindexerWired_NoRepair(t *testing.T) {
	svc, _, _, _ := newIngestTestScanService()
	planner := &fakeIngestPlanner{plans: []*library.ServingPlan{mkPlan("mse-generic", library.ServingStrategyRemuxHLS, true)}}
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cfg.Set(context.Background(), IngestServingPlanConfigKey, IngestServingPlanPlanRepair)
	svc.WithIngestPlanning(cfg, planner, nil) // no reindexer wired

	svc.applyIngestPlan(context.Background(), "mi-1", "/movies/a.mkv")

	if planner.calls != 1 {
		t.Errorf("planner.calls = %d, want 1 (no repair attempted without a reindexer)", planner.calls)
	}
}

func TestApplyIngestPlan_ModePlanRepair_RepairFails_NoReplan(t *testing.T) {
	svc, _, _, _ := newIngestTestScanService()
	planner := &fakeIngestPlanner{plans: []*library.ServingPlan{mkPlan("mse-generic", library.ServingStrategyRemuxHLS, true)}}
	reindexer := &ingestFakeReindexer{err: assertError("disk full")}
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cfg.Set(context.Background(), IngestServingPlanConfigKey, IngestServingPlanPlanRepair)
	svc.WithIngestPlanning(cfg, planner, reindexer)

	svc.applyIngestPlan(context.Background(), "mi-1", "/movies/a.mkv")

	if reindexer.calls != 1 {
		t.Fatalf("reindexer.calls = %d, want 1", reindexer.calls)
	}
	if planner.calls != 1 {
		t.Errorf("planner.calls = %d, want 1 (no re-plan after a failed repair)", planner.calls)
	}
}

func TestApplyIngestPlan_PlannerError_NoWarmUp(t *testing.T) {
	svc, _, _, _ := newIngestTestScanService()
	planner := &fakeIngestPlanner{err: assertError("mkvgo: parse panic")}
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cfg.Set(context.Background(), IngestServingPlanConfigKey, IngestServingPlanPlan)
	svc.WithIngestPlanning(cfg, planner, nil)
	w := &spyWarmer{}
	w.install(svc)

	svc.applyIngestPlan(context.Background(), "mi-1", "/movies/a.mkv")

	if w.id != "" {
		t.Errorf("Plan errored: nothing to consume, warmer called for %q", w.id)
	}
}

// --- scan-time wiring: new files only ---

func TestScanLibrary_NewFile_ComputesIngestPlan(t *testing.T) {
	svc, mediaRepo, scanner, libQueryRepo := newIngestTestScanService()
	planner := &fakeIngestPlanner{plans: []*library.ServingPlan{mkPlan("mse-generic", library.ServingStrategyDirectPlay, false)}}
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cfg.Set(context.Background(), IngestServingPlanConfigKey, IngestServingPlanPlan)
	svc.WithIngestPlanning(cfg, planner, nil)

	libQueryRepo.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}})
	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/new.mkv", Size: 1024, Title: "New Movie", Type: library.MovieItem},
	}

	added, _, err := svc.ScanLibrary(context.Background(), "lib-1")
	if err != nil {
		t.Fatalf("ScanLibrary: %v", err)
	}
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}
	items := mediaRepo.All()
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if planner.calls != 1 {
		t.Errorf("planner.calls = %d, want 1 for the one new file", planner.calls)
	}
	if planner.lastPath != "/movies/new.mkv" {
		t.Errorf("planner path = %q, want the new file", planner.lastPath)
	}
}

// A re-scan of an UNCHANGED file must never call the planner again - only
// brand-new files trigger the ingest-plan precompute.
func TestScanLibrary_UnchangedFile_NoIngestPlanCall(t *testing.T) {
	svc, _, scanner, libQueryRepo := newIngestTestScanService()
	planner := &fakeIngestPlanner{plans: []*library.ServingPlan{mkPlan("mse-generic", library.ServingStrategyDirectPlay, false)}}
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cfg.Set(context.Background(), IngestServingPlanConfigKey, IngestServingPlanPlan)
	svc.WithIngestPlanning(cfg, planner, nil)

	libQueryRepo.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}})
	now := time.Now().Truncate(time.Second)
	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/a.mkv", Size: 1024, ModTime: now, Fingerprint: "fp-a", Title: "A", Type: library.MovieItem},
	}

	if _, _, err := svc.ScanLibrary(context.Background(), "lib-1"); err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if planner.calls != 1 {
		t.Fatalf("after first scan: planner.calls = %d, want 1", planner.calls)
	}

	// Second scan, same file, unchanged (detectChanges short-circuits it
	// before createMediaFile is ever reached again).
	if _, _, err := svc.ScanLibrary(context.Background(), "lib-1"); err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if planner.calls != 1 {
		t.Errorf("after unchanged re-scan: planner.calls = %d, want still 1", planner.calls)
	}
}

// assertError is a tiny error-with-message helper (avoids pulling in errors
// just for one string).
type assertError string

func (e assertError) Error() string { return string(e) }

// --- keyframe warmer (first-play fast path) ---

func TestApplyIngestPlan_WarmsKeyframesForNonTranscodeStrategies(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		strategy library.ServingStrategy
		want     bool
	}{
		{library.ServingStrategyDirectPlay, true},
		{library.ServingStrategyRemuxHLS, true},
		{library.ServingStrategyTranscode, false},
	} {
		svc, _, _, _ := newIngestTestScanService()
		planner := &fakeIngestPlanner{plans: []*library.ServingPlan{mkPlan("mse-generic", tc.strategy, false)}}
		cfg := sharedfakes.NewFakeServerConfigRepository()
		if err := cfg.Set(ctx, IngestServingPlanConfigKey, IngestServingPlanPlan); err != nil {
			t.Fatal(err)
		}
		svc.WithIngestPlanning(cfg, planner, nil)

		var warmedID, warmedPath string
		svc.WithKeyframeWarmer(func(_ context.Context, id, path string) bool {
			warmedID, warmedPath = id, path
			return true
		})

		svc.applyIngestPlan(ctx, "mi-warm", "/movies/warm.mkv")

		if tc.want {
			if warmedID != "mi-warm" || warmedPath != "/movies/warm.mkv" {
				t.Errorf("strategy %q: warmer not called with item/path (got %q %q)", tc.strategy, warmedID, warmedPath)
			}
		} else if warmedID != "" {
			t.Errorf("strategy %q: warmer must NOT be called for a transcode plan", tc.strategy)
		}
	}
}

func TestApplyIngestPlan_NilWarmerIsSafe(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := newIngestTestScanService()
	planner := &fakeIngestPlanner{plans: []*library.ServingPlan{mkPlan("mse-generic", library.ServingStrategyDirectPlay, false)}}
	cfg := sharedfakes.NewFakeServerConfigRepository()
	if err := cfg.Set(ctx, IngestServingPlanConfigKey, IngestServingPlanPlan); err != nil {
		t.Fatal(err)
	}
	svc.WithIngestPlanning(cfg, planner, nil)

	svc.applyIngestPlan(ctx, "mi-1", "/movies/a.mkv") // must not panic

	if planner.calls != 1 {
		t.Errorf("planner.calls = %d, want 1 without a warmer", planner.calls)
	}
}
