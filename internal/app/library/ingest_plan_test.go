package library

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/mkv"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
)

// --- local fakes: IngestPlanner / ServingPlanRepository / mkv.Reindexer ---

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
	p := *f.plans[idx] // copy: applyIngestPlan mutates MediaItemID on the returned pointer
	return &p, nil
}

type fakeServingPlanStore struct {
	mu    sync.Mutex
	plans map[shared.MediaItemID]*library.ServingPlan
	err   error
}

func newFakeServingPlanStore() *fakeServingPlanStore {
	return &fakeServingPlanStore{plans: map[shared.MediaItemID]*library.ServingPlan{}}
}

func (f *fakeServingPlanStore) Upsert(_ context.Context, plan *library.ServingPlan) error {
	if f.err != nil {
		return f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.plans[plan.MediaItemID] = plan
	return nil
}

func (f *fakeServingPlanStore) GetByMediaItemID(_ context.Context, id shared.MediaItemID) (*library.ServingPlan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.plans[id]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return p, nil
}

func (f *fakeServingPlanStore) DeleteByMediaItemID(_ context.Context, id shared.MediaItemID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.plans, id)
	return nil
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
		PlanJSON: []byte(`{}`),
	}
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
	store := newFakeServingPlanStore()
	cfg := sharedfakes.NewFakeServerConfigRepository() // no key set -> off
	svc.WithIngestPlanning(cfg, planner, store, nil)

	svc.applyIngestPlan(context.Background(), "mi-1", "/movies/a.mkv")

	if planner.calls != 0 {
		t.Errorf("planner.calls = %d, want 0 when mode is off", planner.calls)
	}
	if _, err := store.GetByMediaItemID(context.Background(), "mi-1"); err == nil {
		t.Errorf("no plan should have been stored")
	}
}

func TestApplyIngestPlan_PlannerNil_NoOp(t *testing.T) {
	svc, _, _, _ := newIngestTestScanService()
	store := newFakeServingPlanStore()
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cfg.Set(context.Background(), IngestServingPlanConfigKey, IngestServingPlanPlan)
	svc.WithIngestPlanning(cfg, nil, store, nil) // no planner wired

	svc.applyIngestPlan(context.Background(), "mi-1", "/movies/a.mkv")

	if _, err := store.GetByMediaItemID(context.Background(), "mi-1"); err == nil {
		t.Errorf("no plan should have been stored with a nil planner")
	}
}

func TestApplyIngestPlan_ModePlan_ComputesAndStores(t *testing.T) {
	svc, _, _, _ := newIngestTestScanService()
	planner := &fakeIngestPlanner{plans: []*library.ServingPlan{mkPlan("mse-generic", library.ServingStrategyRemuxHLS, true)}}
	store := newFakeServingPlanStore()
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cfg.Set(context.Background(), IngestServingPlanConfigKey, IngestServingPlanPlan)
	svc.WithIngestPlanning(cfg, planner, store, nil)

	svc.applyIngestPlan(context.Background(), "mi-1", "/movies/a.mkv")

	if planner.calls != 1 {
		t.Fatalf("planner.calls = %d, want 1", planner.calls)
	}
	got, err := store.GetByMediaItemID(context.Background(), "mi-1")
	if err != nil {
		t.Fatalf("GetByMediaItemID: %v", err)
	}
	if got.Strategy != library.ServingStrategyRemuxHLS {
		t.Errorf("Strategy = %q, want remux-hls", got.Strategy)
	}
	if got.MediaItemID != "mi-1" {
		t.Errorf("MediaItemID = %q, want mi-1", got.MediaItemID)
	}
}

// plan mode (not plan+repair) must never touch the reindexer, even when the
// computed plan flags NeedsReindex.
func TestApplyIngestPlan_ModePlan_NeverRepairs(t *testing.T) {
	svc, _, _, _ := newIngestTestScanService()
	planner := &fakeIngestPlanner{plans: []*library.ServingPlan{mkPlan("mse-generic", library.ServingStrategyRemuxHLS, true)}}
	store := newFakeServingPlanStore()
	reindexer := &ingestFakeReindexer{}
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cfg.Set(context.Background(), IngestServingPlanConfigKey, IngestServingPlanPlan)
	svc.WithIngestPlanning(cfg, planner, store, reindexer)

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
	store := newFakeServingPlanStore()
	reindexer := &ingestFakeReindexer{}
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cfg.Set(context.Background(), IngestServingPlanConfigKey, IngestServingPlanPlanRepair)
	svc.WithIngestPlanning(cfg, planner, store, reindexer)

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
	got, err := store.GetByMediaItemID(context.Background(), "mi-1")
	if err != nil {
		t.Fatalf("GetByMediaItemID: %v", err)
	}
	if got.NeedsReindex {
		t.Errorf("stored plan must be the POST-repair plan (NeedsReindex=false)")
	}
}

func TestApplyIngestPlan_ModePlanRepair_NoReindexerWired_StoresPreRepairPlan(t *testing.T) {
	svc, _, _, _ := newIngestTestScanService()
	planner := &fakeIngestPlanner{plans: []*library.ServingPlan{mkPlan("mse-generic", library.ServingStrategyRemuxHLS, true)}}
	store := newFakeServingPlanStore()
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cfg.Set(context.Background(), IngestServingPlanConfigKey, IngestServingPlanPlanRepair)
	svc.WithIngestPlanning(cfg, planner, store, nil) // no reindexer wired

	svc.applyIngestPlan(context.Background(), "mi-1", "/movies/a.mkv")

	if planner.calls != 1 {
		t.Errorf("planner.calls = %d, want 1 (no repair attempted without a reindexer)", planner.calls)
	}
	got, err := store.GetByMediaItemID(context.Background(), "mi-1")
	if err != nil {
		t.Fatalf("GetByMediaItemID: %v", err)
	}
	if !got.NeedsReindex {
		t.Errorf("stored plan should still be the (unrepaired) computed plan")
	}
}

func TestApplyIngestPlan_ModePlanRepair_RepairFails_StoresPreRepairPlan(t *testing.T) {
	svc, _, _, _ := newIngestTestScanService()
	planner := &fakeIngestPlanner{plans: []*library.ServingPlan{mkPlan("mse-generic", library.ServingStrategyRemuxHLS, true)}}
	store := newFakeServingPlanStore()
	reindexer := &ingestFakeReindexer{err: assertError("disk full")}
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cfg.Set(context.Background(), IngestServingPlanConfigKey, IngestServingPlanPlanRepair)
	svc.WithIngestPlanning(cfg, planner, store, reindexer)

	svc.applyIngestPlan(context.Background(), "mi-1", "/movies/a.mkv")

	if reindexer.calls != 1 {
		t.Fatalf("reindexer.calls = %d, want 1", reindexer.calls)
	}
	if planner.calls != 1 {
		t.Errorf("planner.calls = %d, want 1 (no re-plan after a failed repair)", planner.calls)
	}
	got, err := store.GetByMediaItemID(context.Background(), "mi-1")
	if err != nil {
		t.Fatalf("GetByMediaItemID: %v", err)
	}
	if !got.NeedsReindex {
		t.Errorf("stored plan must be the pre-repair plan when the repair itself failed")
	}
}

func TestApplyIngestPlan_PlannerError_NoStore(t *testing.T) {
	svc, _, _, _ := newIngestTestScanService()
	planner := &fakeIngestPlanner{err: assertError("mkvgo: parse panic")}
	store := newFakeServingPlanStore()
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cfg.Set(context.Background(), IngestServingPlanConfigKey, IngestServingPlanPlan)
	svc.WithIngestPlanning(cfg, planner, store, nil)

	svc.applyIngestPlan(context.Background(), "mi-1", "/movies/a.mkv")

	if _, err := store.GetByMediaItemID(context.Background(), "mi-1"); err == nil {
		t.Errorf("no plan should be stored when Plan itself errors")
	}
}

// --- scan-time wiring: new files only ---

func TestScanLibrary_NewFile_ComputesIngestPlan(t *testing.T) {
	svc, mediaRepo, scanner, libQueryRepo := newIngestTestScanService()
	planner := &fakeIngestPlanner{plans: []*library.ServingPlan{mkPlan("mse-generic", library.ServingStrategyDirectPlay, false)}}
	store := newFakeServingPlanStore()
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cfg.Set(context.Background(), IngestServingPlanConfigKey, IngestServingPlanPlan)
	svc.WithIngestPlanning(cfg, planner, store, nil)

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
	if _, err := store.GetByMediaItemID(context.Background(), items[0].ID); err != nil {
		t.Errorf("plan not stored for the new item: %v", err)
	}
}

// A re-scan of an UNCHANGED file must never call the planner again - only
// brand-new files trigger the ingest-plan precompute.
func TestScanLibrary_UnchangedFile_NoIngestPlanCall(t *testing.T) {
	svc, _, scanner, libQueryRepo := newIngestTestScanService()
	planner := &fakeIngestPlanner{plans: []*library.ServingPlan{mkPlan("mse-generic", library.ServingStrategyDirectPlay, false)}}
	store := newFakeServingPlanStore()
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cfg.Set(context.Background(), IngestServingPlanConfigKey, IngestServingPlanPlan)
	svc.WithIngestPlanning(cfg, planner, store, nil)

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
		store := newFakeServingPlanStore()
		cfg := sharedfakes.NewFakeServerConfigRepository()
		if err := cfg.Set(ctx, IngestServingPlanConfigKey, IngestServingPlanPlan); err != nil {
			t.Fatal(err)
		}
		svc.WithIngestPlanning(cfg, planner, store, nil)

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
	store := newFakeServingPlanStore()
	cfg := sharedfakes.NewFakeServerConfigRepository()
	if err := cfg.Set(ctx, IngestServingPlanConfigKey, IngestServingPlanPlan); err != nil {
		t.Fatal(err)
	}
	svc.WithIngestPlanning(cfg, planner, store, nil)

	svc.applyIngestPlan(ctx, "mi-1", "/movies/a.mkv") // must not panic

	if _, err := store.GetByMediaItemID(ctx, "mi-1"); err != nil {
		t.Errorf("plan should still be stored without a warmer: %v", err)
	}
}
