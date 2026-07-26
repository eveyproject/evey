package mkvgo

import (
	"testing"

	"github.com/stretchr/testify/assert"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
)

func TestDurationTagSeconds(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"00:00:18.000000000", 18.0, true},
		{"01:35:42.123000000", 5742.123, true},
		{"1:2:3.5", 3723.5, true},
		{"", 0, false},
		{"N/A", 0, false},
		{"12.5", 0, false}, // bare seconds is not the tag form
	}
	for _, c := range cases {
		got, ok := durationTagSeconds(c.in)
		assert.Equal(t, c.ok, ok, "input %q", c.in)
		if c.ok {
			assert.InDelta(t, c.want, got, 1e-6, "input %q", c.in)
		}
	}
}

// The DURATION tags are TagTrackUID-targeted: resolution must go through each
// track's UID, and a track without a usable tag must stay absent (that absence
// routes it to the tail walk).
func TestAudioTagEnds_ResolvesPerTrackUID(t *testing.T) {
	meta := &extmkv.Container{
		Tags: []extmkv.Tag{
			{TargetID: 71, SimpleTags: []extmkv.SimpleTag{{Name: "DURATION", Value: "00:01:35.700000000"}}},
			{TargetID: 72, SimpleTags: []extmkv.SimpleTag{{Name: "duration", Value: "00:01:39.851000000"}}},
			{TargetID: 73, SimpleTags: []extmkv.SimpleTag{{Name: "TITLE", Value: "no duration here"}}},
			// Global (untargeted) tags must never be read as a track duration.
			{TargetID: 0, SimpleTags: []extmkv.SimpleTag{{Name: "DURATION", Value: "09:59:59.000000000"}}},
		},
	}
	audio := []extmkv.Track{
		{ID: 1, UID: 71, Type: extmkv.AudioTrack},
		{ID: 2, UID: 72, Type: extmkv.AudioTrack},
		{ID: 3, UID: 73, Type: extmkv.AudioTrack},
		{ID: 4, UID: 0, Type: extmkv.AudioTrack}, // no UID -> cannot match a tag
	}
	ends := trackTagEnds(meta, audio)
	assert.InDelta(t, 95.700, ends[1], 1e-6)
	assert.InDelta(t, 99.851, ends[2], 1e-6, "lowercase tag name must parse the same")
	_, ok := ends[3]
	assert.False(t, ok, "a track without a DURATION tag stays unresolved")
	_, ok = ends[4]
	assert.False(t, ok, "a track without a UID stays unresolved")
	assert.Len(t, ends, 2)
}

// --- synthetic EBML helpers -------------------------------------------------

// vintSize encodes n as a 1-byte EBML data size (n < 0x7F).
func vintSize(n int) byte { return 0x80 | byte(n) }

// el builds a 1-byte-ID EBML element with a 1-byte size header.
func el(id byte, body ...byte) []byte {
	out := []byte{id, vintSize(len(body))}
	return append(out, body...)
}

// simpleBlock builds a SimpleBlock body: track VINT (data-size form) + int16
// relative timecode + flags + payload.
func simpleBlock(track byte, relTC int16, payload ...byte) []byte {
	body := []byte{0x80 | track, byte(uint16(relTC) >> 8), byte(uint16(relTC))}
	body = append(body, 0x00) // flags
	return append(body, payload...)
}

// The cluster scan must keep the LATEST block time per wanted track, read
// through BlockGroups, and ignore tracks it was not asked about.
func TestScanClusterLastTimes(t *testing.T) {
	var body []byte
	body = append(body, el(0xE7, 0x10)...)                              // Timestamp: cluster TS = 16 ticks
	body = append(body, el(0xA3, simpleBlock(1, 5, 0xAA)...)...)        // track 1 @ 21
	body = append(body, el(0xA3, simpleBlock(2, 7, 0xBB)...)...)        // track 2 (not wanted)
	body = append(body, el(0xA0, el(0xA1, simpleBlock(1, 9)...)...)...) // BlockGroup: track 1 @ 25
	body = append(body, el(0xA3, simpleBlock(1, 2, 0xCC)...)...)        // track 1 @ 18 (earlier - must not win)

	last := map[uint64]int64{}
	scanClusterLastTimes(body, map[uint64]bool{1: true}, last)
	assert.Equal(t, map[uint64]int64{1: 25}, last,
		"the LATEST timecode wins, BlockGroups included, unwanted tracks ignored")
}

// A negative relative timecode (block just before the cluster anchor) must not
// wrap: the 2 bytes are a SIGNED big-endian int16.
func TestScanClusterLastTimes_NegativeRelTC(t *testing.T) {
	var body []byte
	body = append(body, el(0xE7, 0x64)...)                         // TS = 100
	body = append(body, el(0xA3, simpleBlock(1, -30, 0xAA)...)...) // track 1 @ 70

	last := map[uint64]int64{}
	scanClusterLastTimes(body, map[uint64]bool{1: true}, last)
	assert.Equal(t, int64(70), last[1])
}

func TestCueOffsetAtOrBefore(t *testing.T) {
	meta := &extmkv.Container{
		SegmentStart: 1000,
		Cues: []extmkv.CuePoint{
			{TimeMs: 0, ClusterPos: 0},
			{TimeMs: 60_000, ClusterPos: 5_000},
			{TimeMs: 120_000, ClusterPos: 11_000},
			{TimeMs: 180_000, ClusterPos: 17_000},
		},
	}
	assert.Equal(t, int64(1000+11_000), cueOffsetAtOrBefore(meta, 150_000),
		"the last cue at or before the target wins")
	assert.Equal(t, int64(1000+17_000), cueOffsetAtOrBefore(meta, 500_000))
	assert.Equal(t, int64(1000+0), cueOffsetAtOrBefore(meta, 10),
		"a target before every cue falls back to the first")
}

// THE FALSE ACCUSATION (prod 2026-07-26). Every gap this probe reported was
// measured against the container's DECLARED duration - itself a number written
// by whatever muxed the file, and one that a bad remux routinely over-states.
// When the declaration is the liar, the VIDEO stops early too, and blaming the
// audio produces a badge like "Audio -154s" on a file whose tracks are
// perfectly aligned. The reference must be where the video actually ends.
func TestTailGap_ReferenceIsTheVideoEndNotTheDeclaredDuration(t *testing.T) {
	audio := []extmkv.Track{{ID: 1, Type: extmkv.AudioTrack}}
	video := []extmkv.Track{{ID: 2, Type: extmkv.VideoTrack}}

	// The container declares 3600s; both tracks stop together at 3446s. The
	// declaration is wrong, the mux is not: nothing to report.
	assert.Zero(t, tailGap(3600, audio, video, map[uint64]float64{1: 3446, 2: 3446}),
		"tracks that end together are aligned, whatever the container declares")

	// The real defect: the video runs to the end, the audio dies 154s early.
	assert.InDelta(t, 154.0, tailGap(3600, audio, video, map[uint64]float64{1: 3446, 2: 3600}), 1e-9)

	// A video that outlives the declared duration must not inflate the gap
	// beyond it - that is a different anomaly, and not this probe's to report.
	assert.InDelta(t, 154.0, tailGap(3600, audio, video, map[uint64]float64{1: 3446, 2: 4000}), 1e-9)

	// Audio-only container (.mka): no video to compare against, the declared
	// duration stays the only reference there is.
	assert.InDelta(t, 154.0, tailGap(3600, audio, nil, map[uint64]float64{1: 3446}), 1e-9)

	// Several audio tracks: the worst one is reported.
	audio2 := []extmkv.Track{{ID: 1, Type: extmkv.AudioTrack}, {ID: 3, Type: extmkv.AudioTrack}}
	assert.InDelta(t, 154.0, tailGap(3600, audio2, video, map[uint64]float64{1: 3446, 3: 3599, 2: 3600}), 1e-9)
}
