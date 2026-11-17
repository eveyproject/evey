package httpapi

import (
	"bytes"
	"context"
	"errors"
	"image"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gravity-zero/mkvgo/matroska"
	"github.com/gravity-zero/mkvgo/mkv/reader"
	"github.com/gravity-zero/mkvgo/mkv/subtitle"
)

// fakeBitmapDecoder hands back the display sets queued for each block.
type fakeBitmapDecoder struct{ sets [][]subtitle.BitmapDisplaySet }

func (f *fakeBitmapDecoder) Decode([]byte) ([]subtitle.BitmapDisplaySet, error) {
	if len(f.sets) == 0 {
		return nil, nil
	}
	s := f.sets[0]
	f.sets = f.sets[1:]
	return s, nil
}

func shown(x, y int) subtitle.BitmapDisplaySet {
	return subtitle.BitmapDisplaySet{ScreenW: 720, ScreenH: 576, Objects: []subtitle.PGSObject{{
		X: x, Y: y, Image: image.NewNRGBA(image.Rect(0, 0, 10, 4)),
	}}}
}

func assemble(t *testing.T, blocks [][3]int64, sets ...[]subtitle.BitmapDisplaySet) []matroska.PGSCue {
	t.Helper()
	var cues []matroska.PGSCue
	a := &bitmapCueAssembler{dec: &fakeBitmapDecoder{sets: sets}, emit: func(c matroska.PGSCue) error {
		cues = append(cues, c)
		return nil
	}}
	for _, b := range blocks {
		if err := a.block(b[0], b[1], nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.finish(); err != nil {
		t.Fatal(err)
	}
	return cues
}

func spans(cues []matroska.PGSCue) [][2]int64 {
	out := make([][2]int64, len(cues))
	for i, c := range cues {
		out[i] = [2]int64{c.StartMs, c.EndMs}
	}
	return out
}

func wantSpans(t *testing.T, got []matroska.PGSCue, want ...[2]int64) {
	t.Helper()
	g := spans(got)
	if len(g) != len(want) {
		t.Fatalf("cues = %v, want %v", g, want)
	}
	for i := range want {
		if g[i] != want[i] {
			t.Fatalf("cues = %v, want %v", g, want)
		}
	}
}

// A VobSub unit times itself: the start command delays the picture past the
// block, the stop command ends it. Neither the block duration nor the next
// block moves those.
func TestBitmapCueAssembler_VobSubStartAndStopCommands(t *testing.T) {
	s := shown(5, 6)
	s.Delay, s.Duration = 200*time.Millisecond, 1500*time.Millisecond
	cues := assemble(t, [][3]int64{{1000, 4000}, {9000, 0}}, []subtitle.BitmapDisplaySet{s})
	wantSpans(t, cues, [2]int64{1200, 2700})
	if cues[0].X != 5 || cues[0].Y != 6 || cues[0].ScreenW != 720 || cues[0].ScreenH != 576 {
		t.Fatalf("cue placed at %d,%d on %dx%d", cues[0].X, cues[0].Y, cues[0].ScreenW, cues[0].ScreenH)
	}
}

// Without a stop command the block duration ends the cue.
func TestBitmapCueAssembler_BlockDurationWhenNoStopCommand(t *testing.T) {
	cues := assemble(t, [][3]int64{{1000, 2500}}, []subtitle.BitmapDisplaySet{shown(0, 0)})
	wantSpans(t, cues, [2]int64{1000, 3500})
}

// A DVB page stays until the next page replaces or clears it, bounded by its
// time-out.
func TestBitmapCueAssembler_DVBPageEndsAtNextSetOrTimeout(t *testing.T) {
	page := shown(0, 0)
	page.Timeout = 5 * time.Second
	clear := subtitle.BitmapDisplaySet{}
	cues := assemble(t, [][3]int64{{1000, 0}, {3000, 0}, {10000, 0}, {30000, 0}},
		[]subtitle.BitmapDisplaySet{page},  // shown at 1 s
		[]subtitle.BitmapDisplaySet{clear}, // cleared at 3 s
		[]subtitle.BitmapDisplaySet{page},  // shown at 10 s, nothing clears it before 30 s
		[]subtitle.BitmapDisplaySet{clear},
	)
	wantSpans(t, cues, [2]int64{1000, 3000}, [2]int64{10000, 15000})
}

// A last cue nothing closes gets the default duration rather than vanishing.
func TestBitmapCueAssembler_UnclosedLastCue(t *testing.T) {
	cues := assemble(t, [][3]int64{{1000, 0}}, []subtitle.BitmapDisplaySet{shown(0, 0)})
	wantSpans(t, cues, [2]int64{1000, 1000 + bitmapDefaultCueMs})
}

// Two objects in one set become one picture covering both, placed where the
// top-left one sits.
func TestFlattenBitmapDisplaySet_CompositesObjects(t *testing.T) {
	ds := subtitle.BitmapDisplaySet{Objects: []subtitle.PGSObject{
		{X: 100, Y: 50, Image: image.NewNRGBA(image.Rect(0, 0, 20, 10))},
		{X: 110, Y: 400, Image: image.NewNRGBA(image.Rect(0, 0, 30, 10)), Forced: true},
	}}
	cue, err := flattenBitmapDisplaySet(&ds, 0)
	if err != nil {
		t.Fatal(err)
	}
	if cue.X != 100 || cue.Y != 50 || cue.Image.Bounds().Dx() != 40 || cue.Image.Bounds().Dy() != 360 || !cue.Forced {
		t.Fatalf("cue at %d,%d size %v forced %v", cue.X, cue.Y, cue.Image.Bounds().Size(), cue.Forced)
	}
}

func TestFlattenBitmapDisplaySet_RefusesAForgedSpread(t *testing.T) {
	ds := subtitle.BitmapDisplaySet{Objects: []subtitle.PGSObject{
		{X: 0, Y: 0, Image: image.NewNRGBA(image.Rect(0, 0, 1, 1))},
		{X: 60000, Y: 0, Image: image.NewNRGBA(image.Rect(0, 0, 1, 1))},
	}}
	if _, err := flattenBitmapDisplaySet(&ds, 0); err == nil {
		t.Fatal("a 60000-pixel spread was composited")
	}
}

// mkvgoFixture returns a file of mkvgo's own test data, read from the module
// cache: its VobSub and DVB samples are the FFmpeg FATE streams the decoders
// were validated against, and ffmpeg cannot encode a bitmap subtitle from text
// to make one here.
func mkvgoFixture(t *testing.T, name string) string {
	t.Helper()
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/gravity-zero/mkvgo").Output()
	if err != nil {
		t.Skipf("mkvgo module directory unavailable: %v", err)
	}
	p := filepath.Join(strings.TrimSpace(string(out)), "internal", "testdata", name)
	if _, err := os.Stat(p); err != nil {
		t.Skipf("mkvgo fixture %s unavailable: %v", name, err)
	}
	return p
}

// End to end on real streams: a VobSub and a DVB track in Matroska come back
// as timed pictures in a well-formed blob, every cue inside the film, in
// order, on the track's subtitle plane.
func TestExtractBitmapBlob_VobSubAndDVB(t *testing.T) {
	for _, tc := range []struct{ file, codecID string }{
		{"vobsub.mkv", codecVobSub},
		{"dvbsub.mkv", codecDVBSub},
	} {
		t.Run(tc.file, func(t *testing.T) {
			mkv := mkvgoFixture(t, tc.file)
			ctx := context.Background()
			c, err := reader.OpenMeta(ctx, mkv)
			if err != nil {
				t.Fatal(err)
			}
			var track bitmapTrack
			for _, tr := range c.Tracks {
				if tr.Codec == tc.codecID {
					track = bitmapTrack{ID: tr.ID, Codec: tr.Codec}
				}
			}
			if track.ID == 0 {
				t.Fatalf("no %s track in the file", tc.codecID)
			}
			blob, err := extractBitmapBlob(ctx, mkv, track, nil)
			if err != nil {
				t.Fatal(err)
			}
			h, err := decodePGSBlobHeader(blob)
			if err != nil {
				t.Fatal(err)
			}
			if len(h.Cues) == 0 {
				t.Fatal("no cue decoded")
			}
			if h.ScreenW == 0 || h.ScreenH == 0 {
				t.Errorf("no subtitle plane: %dx%d", h.ScreenW, h.ScreenH)
			}
			var prev int64 = -1
			for i, c := range h.Cues {
				if c.EndMs <= c.StartMs || c.StartMs < prev {
					t.Errorf("cue %d spans %d-%d ms after one starting at %d", i, c.StartMs, c.EndMs, prev)
				}
				if c.W == 0 || c.H == 0 || c.Len == 0 || c.X+c.W > h.ScreenW || c.Y+c.H > h.ScreenH {
					t.Errorf("cue %d: %+v off a %dx%d plane", i, c, h.ScreenW, h.ScreenH)
				}
				prev = c.StartMs
			}
			t.Logf("%d cues on %dx%d, first %d-%d ms", len(h.Cues), h.ScreenW, h.ScreenH, h.Cues[0].StartMs, h.Cues[0].EndMs)
		})
	}
}

// A sidecar .sup, and a VobSub .idx with its .sub, are served as the same
// decoded-image blob as an embedded track, and stored in the cache.
func TestServeExternalSubtitle_PictureSidecars(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
	}{
		{"pgs_sub.sup", []string{"pgs_sub.sup"}},
		{"vobsub.idx", []string{"vobsub.idx", "vobsub.sub"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tc.files {
				b, err := os.ReadFile(mkvgoFixture(t, f))
				if err != nil {
					t.Fatal(err)
				}
				// Renamed after a film, as the scanner finds them.
				if err := os.WriteFile(filepath.Join(dir, "film"+filepath.Ext(f)), b, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cache := subtitleCache{dir: t.TempDir()}
			side := filepath.Join(dir, "film"+filepath.Ext(tc.name))
			rec := httptest.NewRecorder()
			serveExternalSubtitle(rec, httptest.NewRequest(http.MethodGet, "/x", nil), cache, side, dir, "")
			if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/vnd.evey.pgs" {
				t.Fatalf("status %d, type %q: %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
			}
			h, err := decodePGSBlobHeader(rec.Body.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			if len(h.Cues) == 0 || h.ScreenW == 0 {
				t.Fatalf("%d cues on %dx%d", len(h.Cues), h.ScreenW, h.ScreenH)
			}
			pair := ""
			if filepath.Ext(side) == ".idx" {
				pair = strings.TrimSuffix(side, ".idx") + ".sub"
			}
			if _, ok := cache.get(externalBitmapKey(cache, side, pair)); !ok {
				t.Error("the blob was not cached")
			}
			t.Logf("%d cues on %dx%d", len(h.Cues), h.ScreenW, h.ScreenH)
		})
	}
}

// An .idx whose .sub is missing answers the failure, it does not hang or 500.
func TestServeExternalSubtitle_IdxWithoutSub(t *testing.T) {
	dir := t.TempDir()
	b, err := os.ReadFile(mkvgoFixture(t, "vobsub.idx"))
	if err != nil {
		t.Fatal(err)
	}
	side := filepath.Join(dir, "film.idx")
	if err := os.WriteFile(side, b, 0o600); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	serveExternalSubtitle(rec, httptest.NewRequest(http.MethodGet, "/x", nil), subtitleCache{}, side, dir, "")
	if rec.Code == http.StatusOK || rec.Code == http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
}

// splitUnitDecoder holds a unit over two blocks: the first gives nothing, the
// second completes it.
type splitUnitDecoder struct{ pending bool }

func (d *splitUnitDecoder) Continues([]byte) bool { return d.pending }

func (d *splitUnitDecoder) Decode([]byte) ([]subtitle.BitmapDisplaySet, error) {
	if !d.pending {
		d.pending = true
		return nil, nil
	}
	d.pending = false
	s := shown(0, 0)
	s.Duration = time.Second
	return []subtitle.BitmapDisplaySet{s}, nil
}

// A VobSub unit a muxer spread over two blocks is timed by the first one, not
// by the block that completes it.
func TestBitmapCueAssembler_UnitSpreadOverBlocksTakesFirstBlockTime(t *testing.T) {
	var cues []matroska.PGSCue
	a := &bitmapCueAssembler{dec: &splitUnitDecoder{}, emit: func(c matroska.PGSCue) error {
		cues = append(cues, c)
		return nil
	}}
	for _, ms := range []int64{1000, 1040} {
		if err := a.block(ms, 0, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.finish(); err != nil {
		t.Fatal(err)
	}
	wantSpans(t, cues, [2]int64{1000, 2000})
}

// Read from the subtitle index, a VobSub or DVB track gives the same blob as
// the walk, byte for byte.
func TestExtractBitmapBlob_IndexMatchesWalk(t *testing.T) {
	for _, tc := range []struct{ file, codecID string }{
		{"vobsub.mkv", codecVobSub},
		{"dvbsub.mkv", codecDVBSub},
	} {
		t.Run(tc.file, func(t *testing.T) {
			mkv := mkvgoFixture(t, tc.file)
			ctx := context.Background()
			c, err := reader.OpenMeta(ctx, mkv)
			if err != nil {
				t.Fatal(err)
			}
			var track bitmapTrack
			for _, tr := range c.Tracks {
				if tr.Codec == tc.codecID {
					track = bitmapTrack{ID: tr.ID, Codec: tr.Codec}
				}
			}
			ix, err := matroska.BuildSubtitleIndex(ctx, mkv, []uint64{track.ID})
			if err != nil {
				t.Fatal(err)
			}
			if ix.Blocks(track.ID) == 0 {
				t.Fatal("the index holds no block of the track")
			}
			walked, err := extractBitmapBlob(ctx, mkv, track, nil)
			if err != nil {
				t.Fatal(err)
			}
			indexed, err := extractBitmapBlob(ctx, mkv, track, ix)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(walked, indexed) {
				t.Fatalf("indexed blob (%d bytes) differs from the walked one (%d bytes)", len(indexed), len(walked))
			}
		})
	}
}

// An index built for another file is refused and the track is walked instead.
func TestExtractBitmapBlob_StaleIndexFallsBackToWalk(t *testing.T) {
	vob, dvb := mkvgoFixture(t, "vobsub.mkv"), mkvgoFixture(t, "dvbsub.mkv")
	ctx := context.Background()
	c, err := reader.OpenMeta(ctx, vob)
	if err != nil {
		t.Fatal(err)
	}
	var track bitmapTrack
	for _, tr := range c.Tracks {
		if tr.Codec == codecVobSub {
			track = bitmapTrack{ID: tr.ID, Codec: tr.Codec}
		}
	}
	other, err := matroska.BuildSubtitleIndex(ctx, dvb, nil)
	if err != nil {
		t.Fatal(err)
	}
	walked, err := extractBitmapBlob(ctx, vob, track, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := extractBitmapBlob(ctx, vob, track, other)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(walked, got) {
		t.Fatal("a stale index changed the blob")
	}
}

// faultyDecoder fails the blocks it is told to, after handing back what it
// completed ahead of the fault, and counts its restarts.
type faultyDecoder struct {
	n      int
	fail   map[int]bool
	resets int
}

func (d *faultyDecoder) Reset() { d.resets++ }

func (d *faultyDecoder) Decode([]byte) ([]subtitle.BitmapDisplaySet, error) {
	d.n++
	s := shown(0, 0)
	s.Duration = time.Second
	if d.fail[d.n] {
		return []subtitle.BitmapDisplaySet{s}, errors.New("a segment that cannot be read")
	}
	return []subtitle.BitmapDisplaySet{s}, nil
}

func runFaulty(t *testing.T, dec *faultyDecoder, blocks ...int64) ([]matroska.PGSCue, error) {
	t.Helper()
	var cues []matroska.PGSCue
	a := &bitmapCueAssembler{dec: dec, emit: func(c matroska.PGSCue) error {
		cues = append(cues, c)
		return nil
	}}
	for _, ms := range blocks {
		if err := a.block(ms, 0, nil); err != nil {
			return cues, err
		}
	}
	return cues, a.finish()
}

// A block that does not decode loses nothing but itself: the set completed
// ahead of the fault and every later block still give their cues, and the
// decoder starts over after the fault.
func TestBitmapCueAssembler_ABadBlockDoesNotKillTheTrack(t *testing.T) {
	dec := &faultyDecoder{fail: map[int]bool{2: true}}
	cues, err := runFaulty(t, dec, 1000, 5000, 9000)
	if err != nil {
		t.Fatal(err)
	}
	wantSpans(t, cues, [2]int64{1000, 2000}, [2]int64{5000, 6000}, [2]int64{9000, 10000})
	if dec.resets != 1 {
		t.Fatalf("decoder restarted %d times, want 1", dec.resets)
	}
}

// A track where every block fails is reported, not served as an empty track.
func TestBitmapCueAssembler_EveryBlockFailingIsAnError(t *testing.T) {
	a := &bitmapCueAssembler{dec: &erroringDecoder{}, emit: func(matroska.PGSCue) error { return nil }}
	for _, ms := range []int64{1000, 2000} {
		if err := a.block(ms, 0, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.finish(); err == nil {
		t.Fatal("a track of undecodable blocks finished clean")
	}
}

type erroringDecoder struct{}

func (erroringDecoder) Decode([]byte) ([]subtitle.BitmapDisplaySet, error) {
	return nil, errors.New("not a subpicture")
}

// A file with a zeroed region in the middle still gives the cues outside it,
// as mkvgo's PGS read does, instead of failing the whole track.
func TestExtractBitmapBlob_PassesOverADamagedRegion(t *testing.T) {
	src := mkvgoFixture(t, "vobsub.mkv")
	ctx := context.Background()
	c, err := reader.OpenMeta(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	var track bitmapTrack
	for _, tr := range c.Tracks {
		if tr.Codec == codecVobSub {
			track = bitmapTrack{ID: tr.ID, Codec: tr.Codec}
		}
	}
	whole, err := extractBitmapBlob(ctx, src, track, nil)
	if err != nil {
		t.Fatal(err)
	}
	wh, err := decodePGSBlobHeader(whole)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	mid := len(b) / 2
	for i := mid; i < mid+4096 && i < len(b); i++ {
		b[i] = 0
	}
	damaged := filepath.Join(t.TempDir(), "damaged.mkv")
	if err = os.WriteFile(damaged, b, 0o600); err != nil {
		t.Fatal(err)
	}
	blob, err := extractBitmapBlob(ctx, damaged, track, nil)
	if err != nil {
		t.Fatalf("one damaged region failed the track: %v", err)
	}
	h, err := decodePGSBlobHeader(blob)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Cues) == 0 || len(h.Cues) > len(wh.Cues) {
		t.Fatalf("%d cues from the damaged file, %d from the whole one", len(h.Cues), len(wh.Cues))
	}
	t.Logf("%d of %d cues kept past the damage", len(h.Cues), len(wh.Cues))
}

// The .sub is found in any letter case, as the scanner pairs it.
func TestVobSubPairSub_AnyLetterCase(t *testing.T) {
	dir := t.TempDir()
	idx := filepath.Join(dir, "film.idx")
	for _, f := range []string{"film.idx", "film.sUb", "other.sub"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, ok := vobSubPairSub(idx)
	if !ok || got != filepath.Join(dir, "film.sUb") {
		t.Fatalf("paired %q, %v", got, ok)
	}
}

// Replacing the .sub alone changes the key: its pictures are what is served.
func TestExternalBitmapKey_FollowsTheSub(t *testing.T) {
	dir := t.TempDir()
	idx, sub := filepath.Join(dir, "film.idx"), filepath.Join(dir, "film.sub")
	if err := os.WriteFile(idx, []byte("idx"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sub, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	cache := subtitleCache{dir: t.TempDir()}
	before := externalBitmapKey(cache, idx, sub)
	if err := os.WriteFile(sub, []byte("other pictures"), 0o600); err != nil {
		t.Fatal(err)
	}
	after := externalBitmapKey(cache, idx, sub)
	if before == "" || before == after {
		t.Fatalf("key %q then %q", before, after)
	}
	if !strings.HasPrefix(after, string(subtitleKindPGS)+"-") {
		t.Fatalf("key %q does not name a blob", after)
	}
}

// A sidecar that does not decode is remembered: the next request answers from
// the memo instead of decoding it again.
func TestServeExternalSubtitle_FailureIsRemembered(t *testing.T) {
	dir := t.TempDir()
	side := filepath.Join(dir, "film.sup")
	if err := os.WriteFile(side, []byte("not a PGS stream at all, just text"), 0o600); err != nil {
		t.Fatal(err)
	}
	cache := subtitleCache{dir: t.TempDir()}
	rec := httptest.NewRecorder()
	serveExternalSubtitle(rec, httptest.NewRequest(http.MethodGet, "/x", nil), cache, side, dir, "")
	if rec.Code == http.StatusOK {
		t.Fatalf("garbage served: %d", rec.Code)
	}
	if _, ok := cache.getFailure(externalBitmapKey(cache, side, "")); !ok {
		t.Fatal("the failure was not remembered")
	}
}

// A damaged region closes the cue on screen where the damage starts.
func TestBitmapCueAssembler_DamageClosesTheOpenCue(t *testing.T) {
	var cues []matroska.PGSCue
	dec := &fakeBitmapDecoder{sets: [][]subtitle.BitmapDisplaySet{{shown(0, 0)}, {shown(0, 0)}}}
	a := &bitmapCueAssembler{dec: dec, emit: func(c matroska.PGSCue) error {
		cues = append(cues, c)
		return nil
	}}
	if err := a.block(1000, 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := a.skipDamage(4000); err != nil {
		t.Fatal(err)
	}
	if err := a.block(60000, 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := a.finish(); err != nil {
		t.Fatal(err)
	}
	wantSpans(t, cues, [2]int64{1000, 4000}, [2]int64{60000, 60000 + bitmapDefaultCueMs})
}

// A .sub reached through a symbolic link pairs like a file: the scanner lists
// the .idx on it.
func TestVobSubPairSub_FollowsASymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(t.TempDir(), "pictures.sub")
	if err := os.WriteFile(real, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "film.idx"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(dir, "film.sub")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	if got, ok := vobSubPairSub(filepath.Join(dir, "film.idx")); !ok || got != filepath.Join(dir, "film.sub") {
		t.Fatalf("paired %q, %v", got, ok)
	}
}

// bitmapServable is what the server decodes into the image blob; the client
// draws exactly the codecs of CLIENT_DRAWABLE_SUBTITLE_CODECS and asks for a
// burn-in of every other bitmap. A codec on one list and not the other is a
// track selected and invisible, or burnt for nothing.
func TestBitmapServable_AgreesWithTheClientDrawableList(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "frontend", "packages", "api", "src", "imageSubtitles.ts")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	set := func(marker string) []string {
		_, after, ok := strings.Cut(string(raw), marker)
		if !ok {
			t.Fatalf("%s not found in %s: renamed or reshaped? update this guard with it", marker, path)
		}
		body, _, ok := strings.Cut(after, "])")
		if !ok {
			t.Fatalf("unterminated %s", marker)
		}
		var out []string
		for _, m := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(body, -1) {
			out = append(out, m[1])
		}
		if len(out) == 0 {
			t.Fatalf("%s holds no codec", marker)
		}
		return out
	}
	drawable := set("CLIENT_DRAWABLE_SUBTITLE_CODECS: ReadonlySet<string> = new Set([")
	inDrawable := map[string]bool{}
	for _, c := range drawable {
		inDrawable[c] = true
		if !bitmapServable(c) {
			t.Errorf("the client draws %q, the server does not decode it", c)
		}
	}
	for _, c := range set("IMAGE_SUBTITLE_CODECS: ReadonlySet<string> = new Set([") {
		if !inDrawable[c] && bitmapServable(c) {
			t.Errorf("the server decodes %q, the client burns it in", c)
		}
	}
}
