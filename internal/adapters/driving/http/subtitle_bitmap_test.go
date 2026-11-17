package httpapi

import (
	"context"
	"image"
	"os"
	"os/exec"
	"path/filepath"
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
