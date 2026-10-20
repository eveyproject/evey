package httpapi

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gravity-zero/mkvgo/matroska"
	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/writer"
)

// cuedFixture writes a Matroska the way muxers do - every subtitle block cued
// with its position inside its cluster - with two SRT tracks (2 and 3) of six
// cues each, and a trusted NUMBER_OF_FRAMES statistic for the tracks named in
// verifiedTracks only.
func cuedFixture(t *testing.T, dir, name string, verifiedTracks ...uint64) string {
	t.Helper()
	w, h := uint32(320), uint32(240)
	var video []extmkv.Block
	for i := range 100 {
		video = append(video, extmkv.Block{TrackNumber: 1, Timecode: int64(i) * 40, Keyframe: i%25 == 0, Data: append([]byte{0, 0, 1, 0x2D, 0x65}, make([]byte, 300)...)})
	}
	subsOf := func(track uint64) []extmkv.Block {
		var subs []extmkv.Block
		for i := range 6 {
			subs = append(subs, extmkv.Block{TrackNumber: track, Timecode: int64(i)*600 + 100, Keyframe: true, Duration: 500, Data: []byte("track " + string(rune('0'+track)) + " cue " + string(rune('A'+i)))})
		}
		return subs
	}
	subs := append(subsOf(2), subsOf(3)...)
	var sets [][]extmkv.Block
	for c := range 4 {
		var set []extmkv.Block
		for _, b := range video[c*25 : (c+1)*25] {
			set = append(set, b)
			for _, s := range subs {
				if s.Timecode >= b.Timecode && s.Timecode < b.Timecode+40 {
					set = append(set, s)
				}
			}
		}
		sets = append(sets, set)
	}
	var tags []extmkv.Tag
	for _, id := range verifiedTracks {
		tags = append(tags, extmkv.Tag{TargetID: id * 11, SimpleTags: []extmkv.SimpleTag{
			{Name: "_STATISTICS_WRITING_APP", Value: "test"},
			{Name: "DURATION", Value: "00:00:03.700000000"},
			{Name: "NUMBER_OF_FRAMES", Value: "6"},
		}})
	}
	tracks := []extmkv.Track{
		{ID: 1, UID: 11, Type: extmkv.VideoTrack, Codec: "h264", Width: &w, Height: &h},
		{ID: 2, UID: 22, Type: extmkv.SubtitleTrack, Codec: "srt", Language: "eng"},
		{ID: 3, UID: 33, Type: extmkv.SubtitleTrack, Codec: "srt", Language: "fre"},
	}

	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	mw := writer.NewMKVWriter(f)
	if err := mw.WriteStart(); err != nil {
		t.Fatal(err)
	}
	c := &extmkv.Container{Info: extmkv.SegmentInfo{TimecodeScale: 1000000, MuxingApp: "test", WritingApp: "test"}, Tags: tags}
	if err := mw.WriteMetadata(c, tracks, 4000); err != nil {
		t.Fatal(err)
	}
	for _, blocks := range sets {
		clusterPos := mw.RelPos()
		offsets, err := writer.WriteClusterOffsets(f, blocks[0].Timecode, 1000000, blocks)
		if err != nil {
			t.Fatal(err)
		}
		for i, b := range blocks {
			if b.Keyframe || b.TrackNumber >= 2 {
				mw.Cues = append(mw.Cues, extmkv.CuePoint{TimeMs: b.Timecode, Track: b.TrackNumber, ClusterPos: clusterPos, RelativePos: offsets[i]})
			}
		}
	}
	if err := mw.Finalize(); err != nil {
		t.Fatal(err)
	}
	return path
}

// Only the tracks the Cues are verified complete on make it into the derived
// index; a file with none gives no index at all.
func TestSubtitleIndexFromCues_KeepsOnlyVerifiedTracks(t *testing.T) {
	dir := t.TempDir()
	c := newSubtitleIndexCache(dir)
	ctx := context.Background()

	both := cuedFixture(t, dir, "both.mkv", 2, 3)
	ix, verified, err := c.fromCues(ctx, both)
	if err != nil || ix == nil {
		t.Fatalf("both verified: ix=%v err=%v", ix, err)
	}
	if !verified[2] || !verified[3] || ix.Blocks(2) != 6 || ix.Blocks(3) != 6 {
		t.Fatalf("verified=%v blocks=%d/%d, want both tracks with 6 blocks", verified, ix.Blocks(2), ix.Blocks(3))
	}
	if !ix.Unresolved() {
		t.Fatal("fromCues must hand back an unresolved index")
	}

	one := cuedFixture(t, dir, "one.mkv", 3)
	ix, verified, err = c.fromCues(ctx, one)
	if err != nil || ix == nil {
		t.Fatalf("one verified: ix=%v err=%v", ix, err)
	}
	if verified[2] || !verified[3] {
		t.Fatalf("verified=%v, want track 3 only", verified)
	}
	if got := ix.Tracks(); len(got) != 1 || got[0] != 3 || ix.Blocks(2) != 0 {
		t.Fatalf("index covers %v (blocks(2)=%d), want track 3 alone", got, ix.Blocks(2))
	}

	none := cuedFixture(t, dir, "none.mkv")
	ix, _, err = c.fromCues(ctx, none)
	if err != nil || ix != nil {
		t.Fatalf("no verified track: ix=%v err=%v, want nil, nil", ix, err)
	}
}

// Resolving stores the index under the file's key; the stored one reads the
// same cues the walk reads, and the second caller gets the stored one.
func TestSubtitleIndexFromCues_ResolveStoresAUsableIndex(t *testing.T) {
	dir := t.TempDir()
	c := newSubtitleIndexCache(dir)
	ctx := context.Background()
	film := cuedFixture(t, dir, "film.mkv", 2, 3)

	ix, _, err := c.fromCues(ctx, film)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := c.resolveAndStore(ctx, film, ix)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Unresolved() {
		t.Fatal("resolveAndStore returned an unresolved index")
	}
	stored, ok := c.get(film)
	if !ok {
		t.Fatal("the resolved index must be stored")
	}
	var fromIndex, fromWalk bytes.Buffer
	if xerr := matroska.ExtractSubtitleWebVTTFrom(ctx, film, 3, stored, &fromIndex); xerr != nil {
		t.Fatalf("extract from the stored index: %v", xerr)
	}
	if werr := matroska.ExtractSubtitleWebVTT(ctx, film, 3, &fromWalk); werr != nil {
		t.Fatalf("extract by walk: %v", werr)
	}
	if fromIndex.String() != fromWalk.String() || !strings.Contains(fromIndex.String(), "track 3 cue F") {
		t.Fatalf("index and walk disagree:\n%s\n---\n%s", fromIndex.String(), fromWalk.String())
	}

	// A second, unresolved copy resolves to the stored one without re-reading.
	again, _, _ := c.fromCues(ctx, film)
	got, err := c.resolveAndStore(ctx, film, again)
	if err != nil || got.Unresolved() {
		t.Fatalf("second resolve: %v (unresolved=%v)", err, got != nil && got.Unresolved())
	}
}

// Without a stored index the WebVTT extraction takes the Cues path and leaves
// the index behind for the next request and the packager.
func TestExtractMkvgoSubtitleWebVTT_UsesTheCuesAndStoresTheIndex(t *testing.T) {
	dir := t.TempDir()
	prev := subtitleIndexes
	subtitleIndexes = newSubtitleIndexCache(dir)
	t.Cleanup(func() { subtitleIndexes = prev })
	film := cuedFixture(t, dir, "film.mkv", 2, 3)

	var out bytes.Buffer
	if err := extractMkvgoSubtitleWebVTT(context.Background(), ".mkv", film, 2, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "track 2 cue A") || !strings.Contains(out.String(), "track 2 cue F") {
		t.Fatalf("unexpected cues:\n%s", out.String())
	}
	if _, ok := subtitleIndexes.get(film); !ok {
		t.Fatal("the Cues-derived index must be stored after the extraction")
	}
}

// The estimate for the cued path counts the resolution (bounded by every
// indexed block) plus the track's own blocks.
func TestEstimateCuedSubtitleExtraction(t *testing.T) {
	dir := t.TempDir()
	c := newSubtitleIndexCache(dir)
	ix, _, err := c.fromCues(context.Background(), cuedFixture(t, dir, "f.mkv", 2, 3))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := estimateCuedSubtitleExtraction(ix, 2), 18*subtitleIndexedBlockCost; got != want {
		t.Fatalf("estimate = %v, want %v (12 cued blocks + 6 of the track)", got, want)
	}
	if estimateCuedSubtitleExtraction(nil, 2) != 0 {
		t.Fatal("nil index must estimate 0")
	}
}

// A source cut before its declared end (a download still running) yields the
// cues it holds, once, flagged incomplete so nothing caches them; no attempt
// falls through to a second read of the same cut file.
func TestExtractSubtitleVTT_TruncatedSourceServesItsCuesUncached(t *testing.T) {
	dir := t.TempDir()
	prev := subtitleIndexes
	subtitleIndexes = newSubtitleIndexCache(dir)
	t.Cleanup(func() { subtitleIndexes = prev })
	film := cuedFixture(t, dir, "film.mkv", 2, 3)
	st, err := os.Stat(film)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Truncate(film, st.Size()*6/10); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err = extractMkvgoSubtitleWebVTT(context.Background(), ".mkv", film, 2, &out)
	if !errors.Is(err, matroska.ErrSourceTruncated) {
		t.Fatalf("want ErrSourceTruncated, got %v", err)
	}
	if got := strings.Count(out.String(), "track 2 cue A"); got != 1 {
		t.Fatalf("the first cue must be there exactly once, found %d times:\n%s", got, out.String())
	}
	if strings.Contains(out.String(), "track 2 cue F") {
		t.Fatalf("a cue past the cut cannot be there:\n%s", out.String())
	}

	data, err := extractSubtitleVTT(context.Background(), film, 0, "subrip", "")
	if !errors.Is(err, errSubtitleIncomplete) || !strings.Contains(string(data), "track 2 cue A") {
		t.Fatalf("want the held cues with errSubtitleIncomplete, got err=%v:\n%s", err, data)
	}
}
