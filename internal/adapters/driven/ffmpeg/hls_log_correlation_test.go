package ffmpeg

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestHLSTranscoder_LogsCarryMediaAndSession: the spawn line and the exit-error
// line (with the stderr tail, the only trace of why ffmpeg died) carried the
// source path and the variant but neither the media nor the session, so a
// failed encode could not be tied to the playback that asked for it.
func TestHLSTranscoder_LogsCarryMediaAndSession(t *testing.T) {
	bin := t.TempDir()
	// Writes the playlist and a first segment so the handle reaches the exit
	// it reports, then dies the way a mid-encode failure does.
	writeFakeBinary(t, filepath.Join(bin, "ffmpeg"), "#!/bin/sh\n"+
		"echo '#EXTM3U' > \"$FAKE_HLS_OUT/stream_0.m3u8\"\n"+
		"echo seg > \"$FAKE_HLS_OUT/stream_0_000.ts\"\n"+
		"echo 'fake encoder failure' >&2\nexit 1\n")
	writeFakeBinary(t, filepath.Join(bin, "ffprobe"), "#!/bin/sh\nexit 1\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	logs := &lockedBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	out := t.TempDir()
	t.Setenv("FAKE_HLS_OUT", out)
	h, err := NewHLSTranscoder().Start(context.Background(), transcoding.HLSProcessOpts{
		OutputDir:          out,
		FilePath:           filepath.Join(out, "source.mkv"),
		MediaID:            "media-1",
		SessionID:          "session-1",
		Profiles:           []transcoding.HLSProfile{{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2000}},
		VideoDecision:      "transcode",
		AudioDecision:      "transcode",
		SubtitleTrackIndex: -1,
	})
	require.NoError(t, err)
	select {
	case <-h.Done():
	case <-time.After(20 * time.Second):
		require.Fail(t, "the fake ffmpeg never exited")
	}

	var start, exit string
	for line := range strings.SplitSeq(logs.String(), "\n") {
		switch {
		case strings.Contains(line, `"hls: ffmpeg start"`):
			start = line
		case strings.Contains(line, `"hls: ffmpeg exited with error"`):
			exit = line
		}
	}
	require.NotEmpty(t, start, "spawn line logged")
	require.NotEmpty(t, exit, "exit-error line logged")
	for _, line := range []string{start, exit} {
		assert.Contains(t, line, `"media_id":"media-1"`)
		assert.Contains(t, line, `"session_id":"session-1"`)
	}
}
