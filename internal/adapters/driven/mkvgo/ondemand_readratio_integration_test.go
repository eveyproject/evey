package mkvgo

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

// A real plan counts as served exactly the bytes its segments wrote, whether
// streamed or handed out whole, and WindowStats sums them beside the source
// read.
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/mkvgo/ -run TestReadRatio -v
func TestReadRatio_RealPlanCountsWhatItServed(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1")
	}
	ctx := context.Background()
	src := filepath.Join(t.TempDir(), "src.mkv")
	out, err := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=300",
		"-t", "30", "-c:v", "libx264", "-preset", "ultrafast", "-g", "48",
		"-c:a", "aac", src).CombinedOutput()
	require.NoError(t, err, string(out))

	p := NewOnDemandPackager()
	defer p.Close()
	plan, err := p.Plan(ctx, src, apptranscoding.OnDemandOptions{})
	require.NoError(t, err)
	opener := plan.(apptranscoding.OnDemandOpener)
	var want int64
	for _, name := range plan.Resources() {
		if strings.HasSuffix(name, ".m3u8") || strings.HasSuffix(name, ".mpd") {
			continue
		}
		res, err := opener.Open(ctx, name)
		require.NoError(t, err, name)
		if b := res.Bytes(); b != nil {
			want += int64(len(b))
		} else {
			n, err := res.WriteRange(ctx, io.Discard, 0, res.Size())
			require.NoError(t, err, name)
			want += n
		}
		require.NoError(t, res.Close())
	}
	require.Positive(t, want)
	st := p.WindowStats()
	require.Equal(t, want, st["served_to_clients_bytes"], "%v", st)
	require.Positive(t, st["source_read_bytes"], "%v", st)
	require.Zero(t, st["plans_read_over_ratio"], "%v", st)
	t.Logf("served %d, read %d (%.2fx), stats %v", want, st["source_read_bytes"],
		float64(st["source_read_bytes"])/float64(want), st)
}
