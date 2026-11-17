package httpapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gravity-zero/mkvgo/matroska"
	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/reader"
)

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

// A track where every picture failed to decode is an error, so the failure is
// remembered; one bad picture among good ones is not.
func TestBitmapSkipTally_EveryPictureFailingIsAnError(t *testing.T) {
	bad := extmkv.DamagedRange{StartOffset: -1, EndOffset: -1, Err: errors.New("not a subpicture")}
	var none bitmapSkipTally
	none.wrap(nil)[0].OnSkip(bad)
	if none.err() == nil {
		t.Fatal("no picture and a failure gave no error")
	}
	var some bitmapSkipTally
	some.wrap(nil)[0].OnSkip(bad)
	if err := some.count(func(matroska.BitmapCue) error { return nil })(matroska.BitmapCue{}); err != nil {
		t.Fatal(err)
	}
	if err := some.err(); err != nil {
		t.Fatalf("one bad picture among good ones failed the track: %v", err)
	}
	var damaged bitmapSkipTally
	damaged.wrap(nil)[0].OnSkip(extmkv.DamagedRange{StartOffset: 10, EndOffset: 20})
	if err := damaged.err(); err != nil {
		t.Fatalf("a damaged region with no decode error failed the track: %v", err)
	}
}

// A pair whose .sub is named in another case than its .idx decodes like any
// pair: mkvgo is handed the .sub vobSubPairSub found.
func TestExtractExternalBitmapBlob_SubInAnotherCase(t *testing.T) {
	dir := t.TempDir()
	for src, dst := range map[string]string{"vobsub.idx": "film.idx", "vobsub.sub": "film.SUB"} {
		b, err := os.ReadFile(mkvgoFixture(t, src))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, dst), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	idx := filepath.Join(dir, "film.idx")
	sub, ok := vobSubPairSub(idx)
	if !ok {
		t.Fatal("the .SUB was not paired")
	}
	blob, err := extractExternalBitmapBlob(context.Background(), idx, sub)
	if err != nil {
		t.Fatal(err)
	}
	h, err := decodePGSBlobHeader(blob)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Cues) != 43 {
		t.Fatalf("%d cues, want 43", len(h.Cues))
	}
}
