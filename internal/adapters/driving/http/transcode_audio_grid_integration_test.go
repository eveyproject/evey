package httpapi

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	ffmpegadapter "github.com/eveyproject/evey/internal/adapters/driven/ffmpeg"
	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// TestAudioGridIntegration plays an audio rendition of a 23.976 fps source
// through the real handler, orchestrator and ffmpeg, and holds every segment
// the encoder wrote against the playlist the player was served: each tfdt
// must sit where the EXTINF sum places the segment, within one AAC frame.
// A player in sequence mode (Shaka) trusts that sum, not the tfdt. Played
// cold from 0 and from a seek, whose session is rebased to absolute time.
// Opt-in:
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driving/http/ -run AudioGridIntegration -v
func TestAudioGridIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// One minute: 20 segments, 60 ms of drift by the end on a 3.000 playlist,
	// three AAC frames - well past the tolerance.
	const durSec = 60
	src := filepath.Join(t.TempDir(), "movie.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=160x90:rate=24000/1001",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", fmt.Sprint(durSec),
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "ac3", "-b:a", "192k",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg source generation failed: %v\n%s", err, out)
	}

	rate := 24000.0 / 1001
	dur, _ := shared.NewDurationMs(durSec * 1000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: src, Duration: dur, Fingerprint: "grid-itest"},
	}
	streams := []*domainlibrary.MediaStream{
		{StreamType: "video", Codec: "h264", Framerate: &rate},
		{StreamType: "audio", Codec: "ac3", Language: "en"},
	}

	for _, first := range []int{0, 12} {
		t.Run(fmt.Sprintf("from segment %d", first), func(t *testing.T) {
			libSvc := &testFakeStreamsGetter{item: item, streams: streams}
			h := buildVODHandlerWithAudio(t, libSvc, allowAll(), testFakeParentalChecker{}, nil)
			orch := apptranscoding.NewAudioSegmentOrchestrator(h.audio.cache, ffmpegadapter.NewHLSTranscoder())
			t.Cleanup(func() { _ = orch.Close() })
			h.audio.audioOrch = orch
			h.audio.segmentWaitTimeout = time.Minute

			w := doAudioDispatchRequest(h, "m1", "audio_0.m3u8")
			require.Equal(t, http.StatusOK, w.Code)
			starts, _ := playlistSegmentStarts(t, w.Body.String())
			require.NotEmpty(t, starts)

			w = doAudioDispatchRequest(h, "m1", "audio_0_init.mp4")
			require.Equal(t, http.StatusOK, w.Code)
			timescale := mdhdTimescale(t, w.Body.Bytes())

			// One AAC frame at the rendition's 48 kHz, plus rounding: the muxer
			// closes a segment on the first frame past the boundary.
			tol := 1024.0/48000 + 0.001
			for n := first; n < len(starts)-1; n++ {
				w := doAudioDispatchRequest(h, "m1", fmt.Sprintf("audio_0_%03d.m4s", n))
				require.Equal(t, http.StatusOK, w.Code, "segment %d", n)
				got := float64(firstTfdt(t, w.Body.Bytes())) / float64(timescale)
				if math.Abs(got-starts[n]) > tol {
					t.Fatalf("segment %d: the playlist places it at %.3f s, its samples start at %.3f s",
						n, starts[n], got)
				}
			}
		})
	}
}

// mdhdTimescale reads the media timescale of an fMP4 init segment.
func mdhdTimescale(t *testing.T, init []byte) uint32 {
	t.Helper()
	i := bytes.Index(init, []byte("mdhd"))
	require.Positive(t, i, "init segment without mdhd")
	off := i + 4 + 12 // version/flags, creation and modification (v0)
	if init[i+4] == 1 {
		off = i + 4 + 20
	}
	return binary.BigEndian.Uint32(init[off : off+4])
}

// firstTfdt reads the baseMediaDecodeTime of a media segment's first fragment.
func firstTfdt(t *testing.T, seg []byte) uint64 {
	t.Helper()
	i := bytes.Index(seg, []byte("tfdt"))
	require.Positive(t, i, "segment without tfdt")
	if m := bytes.Index(seg, []byte("mdat")); m >= 0 {
		require.Less(t, i, m, "tfdt must come from the moof, not the samples")
	}
	if seg[i+4] == 1 {
		return binary.BigEndian.Uint64(seg[i+8 : i+16])
	}
	return uint64(binary.BigEndian.Uint32(seg[i+8 : i+12]))
}
