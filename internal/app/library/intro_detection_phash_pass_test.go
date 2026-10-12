package library

import (
	"context"
	"testing"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// pHashPass glue: siblings come from the repo (episodes of the same parent,
// the target excluded), every episode's hashes are read through its sprite
// manifest, and a frame shared by the season becomes a candidate window in
// the TARGET's timeline. Without any sibling manifest there is nothing to
// compare to and the pass yields nothing.
func TestPHashPass(t *testing.T) {
	manifest := &domainlibrary.TrickplayManifest{MediaItemID: "e1", Interval: 7, Width: 320, Height: 180, TileWidth: 5, TileHeight: 3, ImageCount: 1}
	newEp := func(id string, n int) *domainlibrary.MediaItem {
		dur, _ := shared.NewDurationMs(40 * 60 * 1000)
		return &domainlibrary.MediaItem{ID: shared.MediaItemID(id), ParentID: "season-1", Type: domainlibrary.Episode, IndexNumber: &n,
			File: &domainlibrary.MediaFile{Path: "/media/" + id + ".mkv", Fingerprint: "fp-" + id, Duration: dur}}
	}
	eps := []*domainlibrary.MediaItem{newEp("e1", 1), newEp("e2", 2), newEp("e3", 3)}
	cacheDir := t.TempDir()
	for _, id := range []string{"e1", "e2", "e3"} {
		writeTrickplaySidecarFixture(t, cacheDir, id, manifest.Interval, manifest.TileWidth*manifest.TileHeight, 15)
	}
	// Every episode shows the same 15 tiles: the whole first sprite is "intro".
	det := &hashesDetector{tileHashes: []uint64{11, 22, 33, 44, 55, 66, 77, 88, 99, 111, 222, 333, 444, 555, 666}}
	svc := NewIntroDetectionService(&fakeSegmentRepo{}, &fakeTrickplayRepoWithManifest{manifest: manifest}, &fakeMediaItemRepo{siblings: eps}, &fakeConfigRepo{}, det, cacheDir)
	cfg := defaultIntroConfig()
	cfg.FrameCutoffPercent = 1 // keep every fixture frame in the search

	res, err := svc.pHashPass(context.Background(), eps[0], cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("a season sharing every frame must yield candidates")
	}
	if len(res.siblings) != 2 {
		t.Errorf("siblings = %d, want the 2 other episodes (target excluded)", len(res.siblings))
	}
	for _, s := range res.siblings {
		if s.ID == "e1" {
			t.Error("the target must not be its own sibling")
		}
	}
	if len(res.candidates) == 0 {
		t.Fatal("no candidate window")
	}
	c := res.candidates[0]
	if c.startMs != 0 || c.endMs <= c.startMs || c.matchCount == 0 {
		t.Errorf("candidate = %+v, want a window from the first frame with matches", c)
	}

	// Alone in its season: nothing to compare to.
	solo := NewIntroDetectionService(&fakeSegmentRepo{}, &fakeTrickplayRepoWithManifest{manifest: manifest}, &fakeMediaItemRepo{siblings: eps[:1]}, &fakeConfigRepo{}, det, cacheDir)
	res, err = solo.pHashPass(context.Background(), eps[0], cfg)
	if err != nil || res != nil {
		t.Errorf("solo episode: res=%v err=%v, want nothing", res, err)
	}

	// Siblings whose frames share nothing with the target: no candidate.
	nothingShared := &hashesDetector{tileHashes: nil}
	empty := NewIntroDetectionService(&fakeSegmentRepo{}, &fakeTrickplayRepoWithManifest{manifest: manifest}, &fakeMediaItemRepo{siblings: eps}, &fakeConfigRepo{}, nothingShared, cacheDir)
	res, err = empty.pHashPass(context.Background(), eps[0], cfg)
	if err != nil || res != nil {
		t.Errorf("no hashes: res=%v err=%v, want nothing", res, err)
	}
}
