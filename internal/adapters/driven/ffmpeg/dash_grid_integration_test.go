package ffmpeg

// Frame-exact grid INTEGRATION test: the fragments the real ffmpeg cuts under
// the grid's arguments must hold, tick for tick, the timeline the plan
// declares for them (the DASH manifest is written before they exist). The
// reader is mkvgo (DASHFromCMAF over the produced files): its spans come from
// each fragment's own moof, so a match here is a match with what a DASH
// player will see. Covered: 23.976, 25 and 29.97 fps cold sessions, a seek
// session rebased onto the absolute timeline, a two-rung switch set, and a
// separate audio rendition within one AAC frame of its boundary.
//
// OPT-IN like the other transcode itests: run via `make transcode-itest` or
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/ffmpeg/ -run TestDASHGridIntegration -v

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/cmafparity"
	"github.com/gravity-zero/mkvgo/mp4"
)

// genGridSource writes a 12 s CFR test source at rate (an ffmpeg rational
// such as "24000/1001") with pink-noise AAC audio.
func genGridSource(t *testing.T, ctx context.Context, dir, rate string) string {
	t.Helper()
	src := filepath.Join(dir, "src.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate="+rate,
		"-f", "lavfi", "-i", "anoisesrc=color=pink:seed=11:sample_rate=48000",
		"-t", "12",
		"-c:v", "libx264", "-preset", "ultrafast", "-bf", "2", "-g", "48", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-ac", "2",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("source generation (%s): %v\n%s", rate, err, out)
	}
	return src
}

func TestDASHGridIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest` - needs the bundled ffmpeg)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	for _, tc := range []struct {
		rate string
		fps  float64
	}{
		{"24000/1001", 23.976}, {"25", 25}, {"30000/1001", 29.97},
	} {
		t.Run(tc.rate, func(t *testing.T) {
			grid := transcoding.GridForFrameRate(tc.fps)
			if !grid.Exact() {
				t.Fatalf("no exact grid for %v", tc.fps)
			}
			dir := t.TempDir()
			src := genGridSource(t, ctx, dir, tc.rate)
			// The declaration is made from the probed container duration, as the
			// plan's is. The lavfi source's audio outlasts its last frame by a few
			// milliseconds, and ffmpeg then pads the picture to the container's
			// end by AT MOST one frame (it did at 25 and 29.97 fps here, not at
			// 23.976): a one-frame fragment past the declared count, which neither
			// manifest lists and no player fetches (the presentation ends at the
			// declared duration). Every fragment up to the count must be exact;
			// that single trailing one is tolerated, see tailOnly.
			durationMs := probeDurationMs(t, ctx, src)
			declared := cmafparity.Declared{
				Timescale: int64(grid.Timescale),
				Start:     grid.SegmentStartTicks,
				Dur:       func(n int) int64 { return grid.SegmentDurationTicks(n, durationMs) },
				Count:     grid.SegmentCount(durationMs),
			}
			base := func() transcoding.HLSProcessOpts {
				return transcoding.HLSProcessOpts{
					FilePath:           src,
					VideoDecision:      "transcode",
					AudioDecision:      "transcode",
					AudioTrackIndex:    0,
					SubtitleTrackIndex: -1,
					UseFMP4:            true,
					Grid:               grid,
					Profiles: []transcoding.HLSProfile{
						{Name: "180p-itest", Width: 320, Height: 180, VideoBitrate: 600, AudioBitrate: 96},
					},
				}
			}

			// Cold session, muxed audio: every fragment exact, the last one
			// holding the leftover frames.
			cold := t.TempDir()
			opts := base()
			opts.OutputDir = cold
			args := BuildFFmpegArgs(opts)
			if out, err := exec.CommandContext(ctx, "ffmpeg", append([]string{"-hide_banner", "-v", "error", "-y"}, args...)...).CombinedOutput(); err != nil {
				t.Fatalf("cold session: %v\n%s", err, out)
			}
			res, err := cmafparity.Verify(ctx, cold, "stream_0", false, declared, 0)
			if err != nil {
				t.Fatalf("cold parity: %v", err)
			}
			if want := grid.SegmentCount(durationMs); len(res.Indices)-len(res.Extra) != want || !tailOnly(res.Extra, want) {
				t.Fatalf("cold session cut %d segments, the plan declares %d (%v, past the count %v)", len(res.Indices), want, res.Indices, res.Extra)
			}
			if res.FileTimescale != int64(grid.Timescale) {
				t.Fatalf("fragments written at timescale %d, the grid declares %d", res.FileTimescale, grid.Timescale)
			}
			if !res.OK() {
				t.Fatalf("cold session off the declared timeline: %v", res.Mismatches)
			}

			// Seek session at segment 2, rebased by the publish pump: the
			// fragments land at 2 x SegmentTicks, exactly, under the same
			// declaration as the cold ones.
			seek := t.TempDir()
			sopts := base()
			sopts.OutputDir = seek
			sopts.StartSec = grid.SegmentStartSeconds(2)
			sopts.StartNumber = 2
			h, startErr := NewHLSTranscoder().Start(ctx, sopts)
			if startErr != nil {
				t.Fatalf("seek session start: %v", startErr)
			}
			awaitSegmentFile(t, ctx, filepath.Join(seek, "stream_0_003.m4s"), 90*time.Second)
			<-h.Done()
			_ = h.Stop()
			res, err = cmafparity.Verify(ctx, seek, "stream_0", false, declared, 0)
			if err != nil {
				t.Fatalf("seek parity: %v", err)
			}
			if len(res.Indices) == 0 || res.Indices[0] != 2 {
				t.Fatalf("seek session must start at segment 2, got %v", res.Indices)
			}
			if !res.OK() {
				t.Fatalf("seek session off the declared timeline: %v", res.Mismatches)
			}

			// Two rungs from one grid: mkvgo's switch-set validation is the
			// judge - a DASH player switching mid-stream needs identical spans.
			abr := t.TempDir()
			aopts := base()
			aopts.OutputDir = abr
			aopts.Profiles = []transcoding.HLSProfile{
				{Name: "180p", Width: 320, Height: 180, VideoBitrate: 600, AudioBitrate: 96},
				{Name: "90p", Width: 160, Height: 90, VideoBitrate: 300, AudioBitrate: 96},
			}
			args = BuildFFmpegArgs(aopts)
			if out, abrErr := exec.CommandContext(ctx, "ffmpeg", append([]string{"-hide_banner", "-v", "error", "-y"}, args...)...).CombinedOutput(); abrErr != nil {
				t.Fatalf("abr session: %v\n%s", abrErr, out)
			}
			var pres mp4.CMAFPresentation
			for i := range aopts.Profiles {
				segs, _ := filepath.Glob(filepath.Join(abr, fmt.Sprintf("stream_%d_[0-9]*.m4s", i)))
				names := make([]string, len(segs))
				for k, s := range segs {
					names[k] = filepath.Base(s)
				}
				pres.Video = append(pres.Video, mp4.CMAFRepresentation{Dir: abr, Init: fmt.Sprintf("stream_%d_init.mp4", i), Segments: names})
			}
			if _, switchErr := mp4.DASHFromCMAF(ctx, pres); switchErr != nil {
				t.Fatalf("two rungs of one grid must form a valid switch set: %v", switchErr)
			}
			for i := range aopts.Profiles {
				res, err = cmafparity.Verify(ctx, abr, fmt.Sprintf("stream_%d", i), false, declared, 0)
				if err != nil || !res.OK() {
					t.Fatalf("rung %d off the declared timeline: %v %v", i, err, res.Mismatches)
				}
			}

			// A separate audio rendition on the same grid: within one AAC
			// frame of its declared boundary, one segment per video segment.
			audio := t.TempDir()
			aud := transcoding.AudioProcessOpts{
				FilePath: src, OutputDir: audio, SourceTrackIdx: 0, AudioCodec: "aac", AudioBitrate: 96, MaxAudioChannels: 2, Grid: grid,
			}
			aargs := BuildFFmpegAudioOnlyArgs(aud)
			if out, err := exec.CommandContext(ctx, "ffmpeg", append([]string{"-hide_banner", "-v", "error", "-y"}, aargs...)...).CombinedOutput(); err != nil {
				t.Fatalf("audio rendition: %v\n%s", err, out)
			}
			const audioTimescale = 48000
			adeclared := cmafparity.Declared{
				Timescale: audioTimescale,
				Start:     func(n int) int64 { return int64(grid.SegmentStartSeconds(n) * audioTimescale) },
				Dur: func(n int) int64 {
					return int64(float64(grid.SegmentDurationTicks(n, durationMs)) / float64(grid.Timescale) * audioTimescale)
				},
				Count: grid.SegmentCount(durationMs),
			}
			ares, audioErr := cmafparity.Verify(ctx, audio, "audio_0", true, adeclared, 1024.0/audioTimescale)
			if audioErr != nil {
				t.Fatalf("audio parity: %v", audioErr)
			}
			// The rendition cuts the audio that outlasts the picture into one
			// more, tiny fragment (9 ms here) past the count: unlisted, unfetched.
			if want := grid.SegmentCount(durationMs); len(ares.Indices)-len(ares.Extra) != want || !tailOnly(ares.Extra, want) {
				t.Fatalf("audio rendition cut %d segments for %d video segments (%v, past the count %v)", len(ares.Indices), want, ares.Indices, ares.Extra)
			}
			if !ares.OK() {
				t.Fatalf("audio rendition further than one AAC frame from its boundary: %v", ares.Mismatches)
			}
		})
	}
}

func awaitSegmentFile(t *testing.T, ctx context.Context, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for %s: %v", path, ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
	t.Fatalf("%s did not appear within %s", path, timeout)
}

// probeDurationMs is the container's declared duration, what the plan
// declares its segments from.
func probeDurationMs(t *testing.T, ctx context.Context, path string) int64 {
	t.Helper()
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("ffprobe duration: %v", err)
	}
	var sec float64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%g", &sec); err != nil {
		t.Fatalf("ffprobe duration parse %q: %v", out, err)
	}
	return int64(sec*1000 + 0.5)
}

// tailOnly reports whether extra holds nothing, or exactly the one fragment
// right past the declared count.
func tailOnly(extra []int, count int) bool {
	return len(extra) == 0 || (len(extra) == 1 && extra[0] == count)
}

// A copied AAC rendition keeps the source's 44.1 kHz, in the fragments and in
// the track timescale: the rate (and the SegmentTemplate timescale) the DASH
// manifest declares for a copy is the source's, never the re-encode's 48 kHz.
func TestDASHCopiedAACRateIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest` - needs the bundled ffmpeg)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25",
		"-f", "lavfi", "-i", "anoisesrc=color=pink:seed=11:sample_rate=44100",
		"-t", "8", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-ac", "2", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("source generation: %v\n%s", err, out)
	}
	out := t.TempDir()
	args := BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{
		FilePath: src, OutputDir: out, SourceTrackIdx: 0, AudioCodec: "copy", SourceAudioCodec: "aac",
		Grid: transcoding.GridForFrameRate(25),
	})
	if b, err := exec.CommandContext(ctx, "ffmpeg", append([]string{"-hide_banner", "-v", "error", "-y"}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("copied rendition: %v\n%s", err, b)
	}
	joined := filepath.Join(dir, "joined.mp4")
	init, err := os.ReadFile(filepath.Join(out, "audio_0_init.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	seg, err := os.ReadFile(filepath.Join(out, "audio_0_000.m4s"))
	if err != nil {
		t.Fatal(err)
	}
	if writeErr := os.WriteFile(joined, append(init, seg...), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}
	probe, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "a:0",
		"-show_entries", "stream=sample_rate,time_base", "-of", "csv=p=0", joined).Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	if got := strings.TrimSpace(string(probe)); got != "44100,1/44100" {
		t.Fatalf("copied AAC fragments carry %q, want 44100 Hz on a 1/44100 timescale", got)
	}
}
