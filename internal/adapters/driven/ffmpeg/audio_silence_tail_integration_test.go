package ffmpeg

// Silence-tail INTEGRATION test against the real bundled ffmpeg, reproducing
// the prod 2026-07-26 failure in miniature: an MKV whose AUDIO track ends
// before the container's declared duration ("Scary Movie": audio dies at
// 5742 s of 5991 s - 4 minutes of credits with no audio packets). The VOD
// playlist promises segments across the whole declared duration, so:
//
//  1. a real session seeked past the audio's end must hit the wall (exit
//     without producing its start segment) - the premise the orchestrator's
//     stream-end marking rests on;
//  2. the SILENCE session spawned in its place must keep the playlist's
//     promise with real, decodable, silent AAC segments.
//
// OPT-IN like the other transcode itests: run via `make transcode-itest` or
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/ffmpeg/ -run TestAudioSilenceTailIntegration -v

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// genShortAudioSource writes an MKV with 30 s of video but only 18 s of audio -
// the truncated-tail mux defect. The container's declared duration is the
// video's 30 s.
func genShortAudioSource(t *testing.T, ctx context.Context, dir string) string {
	t.Helper()
	src := filepath.Join(dir, "short_audio.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-t", "30", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-t", "18", "-i", "anoisesrc=color=pink:seed=42:sample_rate=48000",
		"-map", "0:v:0", "-map", "1:a:0",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "ac3", "-ac", "2",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("short-audio source generation: %v\n%s", err, out)
	}
	return src
}

// waitForFiles polls until every name exists in dir, or the deadline passes.
func waitForFiles(t *testing.T, dir string, deadline time.Duration, names ...string) {
	t.Helper()
	end := time.Now().Add(deadline)
	for {
		missing := ""
		for _, n := range names {
			if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
				missing = n
				break
			}
		}
		if missing == "" {
			return
		}
		if time.Now().After(end) {
			entries, _ := os.ReadDir(dir)
			var have []string
			for _, e := range entries {
				have = append(have, e.Name())
			}
			t.Fatalf("timed out waiting for %s in %s (have: %v)", missing, dir, have)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestAudioSilenceTailIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest` - needs the bundled ffmpeg)")
	}
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not in PATH", bin)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	dir := t.TempDir()
	src := genShortAudioSource(t, ctx, dir)
	tr := NewHLSTranscoder()

	// --- 1. The wall: a real session seeked past the audio's end produces
	// nothing (the orchestrator reads exactly this as proof of stream end).
	wallDir := filepath.Join(dir, "wall")
	if err := os.MkdirAll(wallDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wall, err := tr.StartAudioOnly(ctx, transcoding.AudioProcessOpts{
		FilePath:         src,
		OutputDir:        wallDir,
		SourceTrackIdx:   0,
		AudioCodec:       "aac",
		AudioBitrate:     128,
		MaxAudioChannels: 2,
		StartSec:         24.0, // past the audio's 18 s end, inside the 30 s video
		StartNumber:      8,
	})
	if err != nil {
		t.Fatalf("wall session start: %v", err)
	}
	select {
	case <-wall.Done():
	case <-time.After(60 * time.Second):
		_ = wall.Stop()
		t.Fatal("wall session did not exit - expected a near-immediate EOF past the audio's end")
	}
	if _, statErr := os.Stat(filepath.Join(wallDir, "audio_0_008.m4s")); statErr == nil {
		t.Fatal("a session seeked past the audio's end must NOT produce its start segment")
	}

	// --- 2. The silence tail keeps the playlist's promise: segments 8 and 9
	// (6 s bound), numbered/rebased on the absolute timeline, decodable, silent.
	silDir := filepath.Join(dir, "silence")
	if mkErr := os.MkdirAll(silDir, 0o755); mkErr != nil {
		t.Fatal(mkErr)
	}
	sil, err := tr.StartAudioOnly(ctx, transcoding.AudioProcessOpts{
		FilePath:           src, // must be ignored in silence mode
		OutputDir:          silDir,
		SourceTrackIdx:     0,
		AudioCodec:         "aac",
		AudioBitrate:       128,
		MaxAudioChannels:   2,
		StartSec:           24.0,
		StartNumber:        8,
		SilenceDurationSec: 6.0,
	})
	if err != nil {
		t.Fatalf("silence session start: %v", err)
	}
	defer func() { _ = sil.Stop() }()

	waitForFiles(t, silDir, 30*time.Second, "audio_0_init.mp4", "audio_0_008.m4s", "audio_0_009.m4s")
	select {
	case <-sil.Done():
	case <-time.After(30 * time.Second):
		t.Fatal("bounded silence session must exit once its -t span is encoded")
	}
	// The AAC encoder's priming/padding can spill ONE boundary segment past the
	// -t span (observed: seg 10 for a 6 s bound) - harmless, the playlist never
	// references it. What must not happen is the unbounded case.
	if _, statErr := os.Stat(filepath.Join(silDir, "audio_0_011.m4s")); statErr == nil {
		t.Fatal("silence must stop at its bound - segment 11 lies well past the 6 s span")
	}

	// Decodability + silence: concat init+segment, decode through volumedetect.
	joined := filepath.Join(dir, "joined.mp4")
	initBytes, err := os.ReadFile(filepath.Join(silDir, "audio_0_init.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	segBytes, err := os.ReadFile(filepath.Join(silDir, "audio_0_008.m4s"))
	if err != nil {
		t.Fatal(err)
	}
	if wErr := os.WriteFile(joined, append(initBytes, segBytes...), 0o644); wErr != nil {
		t.Fatal(wErr)
	}
	probe := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-show_entries", "format=duration:stream=codec_name",
		"-of", "default=noprint_wrappers=1", joined)
	probeOut, err := probe.CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe joined init+segment: %v\n%s", err, probeOut)
	}
	if !strings.Contains(string(probeOut), "codec_name=aac") {
		t.Fatalf("silence segment must be AAC, got:\n%s", probeOut)
	}
	// Format duration = the segment's END timestamp: ~27 s proves BOTH the 3 s
	// cadence AND that the rebase pump landed the fragment on the ABSOLUTE
	// timeline (24 s start), not at the lavfi graph's t=0 - a t=0 fragment
	// would make the player play the tail at the head of the film.
	if m := regexp.MustCompile(`duration=([0-9.]+)`).FindStringSubmatch(string(probeOut)); m != nil {
		d, _ := strconv.ParseFloat(m[1], 64)
		if d < 26.5 || d > 28.0 {
			t.Fatalf("silence segment end timestamp = %.3f s, want ~27 s (24 s absolute start + 3 s cadence)", d)
		}
	} else {
		t.Fatalf("no duration in ffprobe output:\n%s", probeOut)
	}

	vol := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-i", joined,
		"-af", "volumedetect", "-f", "null", "-")
	volOut, err := vol.CombinedOutput()
	if err != nil {
		t.Fatalf("volumedetect decode: %v\n%s", err, volOut)
	}
	m := regexp.MustCompile(`mean_volume:\s*(-?[0-9.]+|-inf)`).FindStringSubmatch(string(volOut))
	if m == nil {
		t.Fatalf("no mean_volume in volumedetect output:\n%s", volOut)
	}
	if m[1] != "-inf" {
		mean, _ := strconv.ParseFloat(m[1], 64)
		if mean > -60 {
			t.Fatalf("silence segment mean_volume = %s dB - that is not silence", m[1])
		}
	}
}
