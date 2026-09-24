package cmafparity

import (
	"testing"
)

// The timeline mkvgo emits (a SegmentList here: the fragments had a gap in
// their numbering) is read back with its repeats expanded and its explicit
// restarts honoured, so an index maps to the right span.
func TestParseTimeline_ExpandsRepeatsAndRestarts(t *testing.T) {
	t.Parallel()
	mpd := `<?xml version="1.0" encoding="UTF-8"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static">
  <Period>
    <AdaptationSet mimeType="video/mp4" contentType="video">
      <Representation id="v1" bandwidth="189874" width="854" height="480" codecs="avc1.4D401E">
        <SegmentList timescale="12800">
          <Initialization sourceURL="stream_0_init.mp4"/>
          <SegmentTimeline>
            <S t="0" d="38400" r="2"/>
            <S t="1497600" d="38400" r="1"/>
          </SegmentTimeline>
          <SegmentURL media="stream_0_000.m4s"/>
        </SegmentList>
      </Representation>
    </AdaptationSet>
  </Period>
</MPD>`
	ts, spans, err := parseTimeline([]byte(mpd), "video")
	if err != nil {
		t.Fatal(err)
	}
	if ts != 12800 {
		t.Fatalf("timescale %d", ts)
	}
	want := [][2]int64{{0, 38400}, {38400, 38400}, {76800, 38400}, {1497600, 38400}, {1536000, 38400}}
	if len(spans) != len(want) {
		t.Fatalf("spans %v", spans)
	}
	for i := range want {
		if spans[i] != want[i] {
			t.Fatalf("span %d: got %v want %v", i, spans[i], want[i])
		}
	}
}

// Exact comparison cross-multiplies: 72072 ticks at 24000 and 270270 at
// 90000 are the same instant; one tick off is not. A tolerance turns it into
// a seconds comparison.
func TestSame(t *testing.T) {
	t.Parallel()
	if !same(72072, 24000, 270270, 90000, 0) {
		t.Fatal("equal instants across timescales must match exactly")
	}
	if same(72072, 24000, 270271, 90000, 0) {
		t.Fatal("one tick off must not match exactly")
	}
	if !same(144000, 48000, 144384, 48000, 1024.0/48000) {
		t.Fatal("within one AAC frame must match under that tolerance")
	}
	if same(144000, 48000, 146000, 48000, 1024.0/48000) {
		t.Fatal("two frames off must not")
	}
}
