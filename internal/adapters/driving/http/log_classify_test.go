package httpapi

import "testing"

// The admin log badge must name the engine that ACTUALLY ran. The
// keyframe_backfill_worker is mixed - pure mkvgo for the frame-rate walk,
// mkvgo-or-ffprobe for keyframes - so it used to be badged "ffmpeg" wholesale,
// which was simply false for most of its lines.
func TestClassifySource_EngineFieldOverridesTheGuess(t *testing.T) {
	cases := []struct {
		name, component, msg, engine, want string
	}{
		{"frame-rate is pure mkvgo", "keyframe_backfill_worker", "frame-rate reclassified", "mkvgo", "mkvgo"},
		{"keyframes served by the fast path", "keyframe_backfill_worker", "keyframe backfill: indexed", "mkvgo", "mkvgo"},
		{"keyframes fell back to ffprobe", "keyframe_backfill_worker", "keyframe backfill: indexed", "ffmpeg", "ffmpeg"},
		{"a real ffmpeg component stays ffmpeg", "software_producer", "started", "", "ffmpeg"},
		{"no engine, no guess for a mixed component", "keyframe_backfill_worker", "some line", "", "other"},
		{"an unknown engine value never wins", "software_producer", "started", "bogus", "ffmpeg"},
		{"the browser player is its own source, not other", "player_client", "player-client: playback error reported", "", "client"},
	}
	for _, c := range cases {
		if got := classifySource(c.component, c.msg, c.engine); got != c.want {
			t.Errorf("%s: classifySource(%q,%q,%q) = %q, want %q",
				c.name, c.component, c.msg, c.engine, got, c.want)
		}
	}
}
