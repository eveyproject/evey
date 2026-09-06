package transcoding

// The reach decision is the one thing the 2026-09-06 investigation could not
// reconstruct: the logs showed a client hanging up on a segment (`context
// canceled` after ~10 s) but never said how far that segment was from the
// encoder's production point, so "the server made it wait too long" stayed a
// deduction instead of a fact. These tests lock the line that closes that gap.

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func capturingProducer(t *testing.T) (*SoftwareProducer, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	p := &SoftwareProducer{
		logger: slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
		opts:   swTestOpts().withDefaults(),
	}
	return p, &buf
}

func reachLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m))
		if m["msg"] == "software: reach decision" {
			out = append(out, m)
		}
	}
	return out
}

// Continuous playback misses the cache on nearly every segment while sitting
// 0-1 segments ahead of the frontier. Logging those would bury the file on a
// weak NAS and drown the rare lines that matter.
func TestLogReachDecision_SilentOnContinuousPlayback(t *testing.T) {
	p, buf := capturingProducer(t)
	for _, d := range []int{0, 1} {
		p.logReachDecision(context.Background(), "h", 0, 10+d, 10, true)
	}
	assert.Empty(t, reachLines(t, buf), "a reuse at the production edge is not an event")
}

// A reuse that puts the player behind a real wait IS an event - this is the
// case that has to be visible, because it is the one that can exceed the
// client's patience.
func TestLogReachDecision_ReportsAReuseThatMakesThePlayerWait(t *testing.T) {
	p, buf := capturingProducer(t)
	// At the derived window the far edge is the worst legal wait.
	w := p.opts.SeekReachWindowSegs
	p.logReachDecision(context.Background(), "h", 0, 100+w, 100, true)

	lines := reachLines(t, buf)
	require.Len(t, lines, 1)
	got := lines[0]
	assert.Equal(t, "reuse", got["decision"])
	assert.InDelta(t, float64(w), got["distance_segs"], 0.001)
	assert.InDelta(t, 100, got["frontier"], 0.001)
	assert.InDelta(t, float64(w), got["reach_window_segs"], 0.001)
	// The whole point of the derived window: the worst legal wait is inside
	// what a player will sit through.
	assert.Equal(t, false, got["over_client_patience"])
	assert.LessOrEqual(t, got["predicted_wait_s"], clientSegmentPatience.Seconds())
}

// A spawn is always worth a line: it is a whole ffmpeg, and on a weak NAS the
// reason one appeared is exactly what an operator needs.
func TestLogReachDecision_ReportsASpawnAndItsDistance(t *testing.T) {
	p, buf := capturingProducer(t)
	p.logReachDecision(context.Background(), "h", 1, 500, 100, false)

	lines := reachLines(t, buf)
	require.Len(t, lines, 1)
	got := lines[0]
	assert.Equal(t, "spawn", got["decision"])
	assert.InDelta(t, 400, got["distance_segs"], 0.001)
	assert.InDelta(t, 1, got["variant"], 0.001)
	assert.InDelta(t, 500, got["seg"], 0.001)
	// 400 segments at 1.5x would have been minutes of waiting: proof, in the
	// line itself, that spawning was the right call.
	assert.Equal(t, true, got["over_client_patience"])
}

// No session covers the position at all (a cold start, or a viewer below every
// live encoder). There is no distance to report and the line must say so rather
// than invent one.
func TestLogReachDecision_MarksTheNoCoveringSessionCase(t *testing.T) {
	p, buf := capturingProducer(t)
	p.logReachDecision(context.Background(), "h", 0, 42, -1, false)

	lines := reachLines(t, buf)
	require.Len(t, lines, 1)
	assert.Equal(t, "spawn", lines[0]["decision"])
	assert.InDelta(t, -1, lines[0]["frontier"], 0.001)
	assert.InDelta(t, -1, lines[0]["distance_segs"], 0.001)
	assert.InDelta(t, 0, lines[0]["predicted_wait_s"], 0.001)
}

// The predicted wait is the number the whole reach rule turns on, so it is
// derived from the pacing rather than guessed.
func TestPredictedReachWaitSec_FollowsThePacing(t *testing.T) {
	p, _ := capturingProducer(t)
	// 3 segments of 3 s = 9 s of content, crossed at 1.5x in 6 s.
	assert.InDelta(t, 6.0, p.predictedReachWaitSec(3), 0.001)
	assert.InDelta(t, 0, p.predictedReachWaitSec(0), 0.001)
	assert.InDelta(t, 0, p.predictedReachWaitSec(-1), 0.001)
}

// The -progress file name has to be self-describing: an investigation lines it
// up with the JSON log by hash, rung and start position, and the nanosecond
// suffix keeps a RESPAWN at the same position from overwriting the run before
// it - that repetition is itself the symptom worth reading (four encoders and
// two kills in 72 s, prod 2026-09-06).
func TestFFmpegProgressLogPath(t *testing.T) {
	assert.Empty(t, ffmpegProgressLogPath("", "abc", 0, 0),
		"no directory means the facility is off, not a file in the working dir")

	got := ffmpegProgressLogPath("/data/logs/ffmpeg", "49011580ac30abb0", 1, 45)
	assert.True(t, strings.HasPrefix(got, "/data/logs/ffmpeg/"))
	assert.Contains(t, got, "49011580ac30abb0")
	assert.Contains(t, got, "-v1-")
	assert.Contains(t, got, "-s45-")
	assert.True(t, strings.HasSuffix(got, ".log"))

	again := ffmpegProgressLogPath("/data/logs/ffmpeg", "49011580ac30abb0", 1, 45)
	assert.NotEqual(t, got, again, "a respawn at the same position must not clobber the previous run")
}
