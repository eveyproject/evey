package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

// The compensation must stay OFF unless something positively establishes that
// the video is packaged by the ffmpeg copy fallback. Shifting audio against an
// mkvgo-packaged video would CREATE the desync it exists to remove, so every
// unknown has to resolve to 0 - the pre-fix behaviour.
func TestVideoCompositionDelaySec_DefaultsToNoShift(t *testing.T) {
	item := &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{Path: "/media/x.mkv"}}
	r := httptest.NewRequest("GET", "/hls/audio_0.m3u8", nil)

	cases := []struct {
		name string
		h    *hlsVODServe
		item *domainlibrary.MediaItem
	}{
		{
			name: "no probe wired",
			h:    &hlsVODServe{},
			item: item,
		},
		{
			name: "no on-demand packager wired: \"mkvgo did not take it\" would be an artefact, not an answer",
			h: &hlsVODServe{compositionDelayProbe: func(context.Context, string) float64 {
				t.Error("probe must not run without a packager to compare against")
				return 0.083
			}},
			item: item,
		},
		{
			name: "no media file",
			h: &hlsVODServe{compositionDelayProbe: func(context.Context, string) float64 {
				t.Error("probe must not run without a source path")
				return 0.083
			}},
			item: &domainlibrary.MediaItem{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.h.videoCompositionDelaySec(r, "m1", tc.item, nil); got != 0 {
				t.Errorf("videoCompositionDelaySec = %v, want 0 (no shift)", got)
			}
		})
	}
}

// sourceIdentity keys the plan-build memo. It must key on the file's identity,
// not just its path: a repaired or re-encoded source must re-prove whether it
// packages instead of inheriting the previous verdict.
func TestSourceIdentity_TracksFileIdentityNotJustPath(t *testing.T) {
	path := t.TempDir() + "/a.mkv"
	if err := os.WriteFile(path, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, ok := sourceIdentity(path)
	if !ok {
		t.Fatal("sourceIdentity failed on an existing file")
	}

	// Size differs, so the identity must differ even if the clock is coarse.
	time.Sleep(5 * time.Millisecond)
	if err := os.WriteFile(path, []byte("a longer body"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, ok := sourceIdentity(path)
	if !ok {
		t.Fatal("sourceIdentity failed after rewrite")
	}
	if first == second {
		t.Errorf("identity unchanged after the file changed (%q) - a repaired source would inherit a stale verdict", first)
	}

	if _, ok := sourceIdentity(path + ".missing"); ok {
		t.Error("a missing file must not yield an identity - it would poison the memo")
	}
}

// newAudioSyncItem returns a MediaItem backed by a real temp file, so
// sourceIdentity (which os.Stats the path) succeeds and planBuildsForSource
// actually exercises the memo.
func newAudioSyncItem(t *testing.T) *domainlibrary.MediaItem {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a.mkv")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{Path: path}}
}

// A cancellation says nothing about the FILE - see planBuildsForSource's own
// doc comment - so it must never be memoised: every call re-derives, and the
// verdict handed back for THIS request is still "packages fine" (no shift).
func TestPlanBuildsForSource_CancellationNeverMemoised(t *testing.T) {
	pkg := &fakeOnDemandPackager{err: context.Canceled}
	h := &hlsVODServe{onDemand: pkg}
	item := newAudioSyncItem(t)
	r := onDemandReq()

	for i := 0; i < 2; i++ {
		if !h.planBuildsForSource(r, "m1", item, nil) {
			t.Fatalf("call %d: a cancellation must resolve to true for this request", i)
		}
	}
	if pkg.calls != 2 {
		t.Fatalf("Plan calls = %d, want 2 - a context.Canceled verdict must be re-derived every time, never cached", pkg.calls)
	}
}

// A genuine build failure (not a cancellation) IS a property of the file and
// must be memoised - re-paying the whole PlanHLS walk on every sub-resource
// request of the same master would defeat the point of the memo.
func TestPlanBuildsForSource_RealFailureIsMemoised(t *testing.T) {
	pkg := &fakeOnDemandPackager{err: errors.New("no Cues index")}
	h := &hlsVODServe{onDemand: pkg}
	item := newAudioSyncItem(t)
	r := onDemandReq()

	for i := 0; i < 2; i++ {
		if h.planBuildsForSource(r, "m1", item, nil) {
			t.Fatalf("call %d: a real build failure must resolve to false", i)
		}
	}
	if pkg.calls != 1 {
		t.Fatalf("Plan calls = %d, want 1 - a real failure is a file property and must be memoised", pkg.calls)
	}
}

// The memo must stay BOUNDED: on a server running for weeks against a moving
// library, a bare unbounded map (one entry per file identity, kept forever)
// is a memory leak. A small cap makes the eviction observable fast: filling
// the cache past capacity must evict the oldest entry rather than grow it.
func TestPlanBuildsForSource_MemoStaysBounded(t *testing.T) {
	resetPlanBuildMemoCache(2)
	t.Cleanup(func() { resetPlanBuildMemoCache(0) })

	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h := &hlsVODServe{onDemand: pkg}
	r := onDemandReq()
	item1, item2, item3 := newAudioSyncItem(t), newAudioSyncItem(t), newAudioSyncItem(t)

	h.planBuildsForSource(r, "m1", item1, nil) // calls=1, memoised (cap 2: 1/2 full)
	h.planBuildsForSource(r, "m1", item2, nil) // calls=2, memoised (cap 2: full)
	h.planBuildsForSource(r, "m1", item3, nil) // calls=3, evicts item1's entry (LRU)

	if !h.planBuildsForSource(r, "m1", item1, nil) {
		t.Fatal("item1 must still package fine after re-deriving")
	}
	if pkg.calls != 4 {
		t.Fatalf("Plan calls = %d, want 4 - the cap must evict the oldest entry instead of growing forever", pkg.calls)
	}
}
