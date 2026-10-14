package httpapi

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// An MP4 timed-text (tx3g) subtitle is stored "mov_text" (ffprobe, and the
// mkvgo fast path since mkvgo 0.51) or "subrip" (the fast path before). Both
// name the same track: the mkvgo extraction serves it either way, with the same
// bytes. Before, "mov_text" failed the codec guard and went to ffmpeg, which
// cuts the cues differently (a Stranger Things episode: 504 cues by mkvgo, 505
// by ffmpeg, 2026-10-15).
func TestTryMKVGoSubtitle_MP4TimedTextUnderEitherCodecName(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	dir := t.TempDir()
	srt := filepath.Join(dir, "in.srt")
	if err := os.WriteFile(srt, []byte("1\n00:00:00,500 --> 00:00:01,500\nhello\n\n2\n00:00:02,000 --> 00:00:03,000\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mp4 := filepath.Join(dir, "tx3g.mp4")
	gen := exec.Command("ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "color=size=64x64:rate=10:duration=4", "-i", srt,
		"-c:v", "libx264", "-c:s", "mov_text", mp4)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v\n%s", err, out)
	}
	ctx := context.Background()

	legacy, ok := tryMKVGoSubtitleBytes(ctx, ".mp4", mp4, 0, "subrip", "")
	if !ok {
		t.Fatal(`stored "subrip": the mkvgo extraction refused the track`)
	}
	current, ok := tryMKVGoSubtitleBytes(ctx, ".mp4", mp4, 0, "mov_text", "")
	if !ok {
		t.Fatal(`stored "mov_text": the mkvgo extraction refused the track`)
	}
	if !bytes.Equal(legacy, current) || bytes.Count(current, []byte("-->")) != 2 {
		t.Fatalf("the two names served different subtitles:\n%s\n---\n%s", legacy, current)
	}
	// Another codec on record is a drifted ordinal: not served from mkvgo.
	if _, ok := tryMKVGoSubtitleBytes(ctx, ".mp4", mp4, 0, "ass", ""); ok {
		t.Fatal(`stored "ass": a mismatched codec was served`)
	}
}
