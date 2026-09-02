package ffmpeg

import (
	"math"
	"testing"
)

// silencedetect reports every stretch with a start, an end and a duration;
// the parser turns that into audible seconds and a stretch count.
func TestParseSilenceLog_CountsStretchesAndAudibleTime(t *testing.T) {
	log := `[silencedetect @ 0x1] silence_start: 10.5
[silencedetect @ 0x1] silence_end: 15.5 | silence_duration: 5
size=N/A time=00:01:40.00 bitrate=N/A speed= 300x
[silencedetect @ 0x1] silence_start: 60
[silencedetect @ 0x1] silence_end: 62.25 | silence_duration: 2.25
`
	audible, stretches := parseSilenceLog(log, 100)
	if stretches != 2 {
		t.Fatalf("stretches = %d, want 2", stretches)
	}
	if math.Abs(audible-92.75) > 1e-9 {
		t.Fatalf("audible = %.3f, want 92.75", audible)
	}
}

// A track that ends in silence leaves the last stretch open in the log; it
// runs to the end of the track and still counts - a hole at the tail is the
// shape a truncated rewrite leaves.
func TestParseSilenceLog_OpenStretchRunsToTheEnd(t *testing.T) {
	log := "[silencedetect @ 0x1] silence_start: 90\n"
	audible, stretches := parseSilenceLog(log, 100)
	if stretches != 1 || math.Abs(audible-90) > 1e-9 {
		t.Fatalf("got %d stretches, %.1f audible; want 1 and 90", stretches, audible)
	}
}

// Packet sizes are grouped per audio track in audio order, whatever the
// container's stream indexes are, and the set is what the guard compares: one
// size for a sound AC-3 track, two where the frame format changes.
func TestParsePacketSizes_GroupsByAudioOrder(t *testing.T) {
	csv := "1,1792\n2,2560\n1,1792\n2,2560\n1,1792\n2,1792\n"
	sets := parsePacketSizes(csv)
	if len(sets[0]) != 1 {
		t.Errorf("track 0 sizes = %d, want 1 (uniform)", len(sets[0]))
	}
	if len(sets[1]) != 2 {
		t.Errorf("track 1 sizes = %d, want 2 (format changes mid-track)", len(sets[1]))
	}
}
