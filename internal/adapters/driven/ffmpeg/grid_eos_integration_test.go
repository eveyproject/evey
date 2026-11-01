package ffmpeg

// End of stream on the frame grid: run the real ffmpeg to EOF on sources
// whose audio outlasts the picture, and hold the count the plan declares
// (SegmentCount of the container duration) against the fragments that
// exist. A declared segment nobody produces is a request that never
// resolves at the end of the film - the HLS playlist and the DASH timeline
// both list SegmentCount of the same duration. Without the frame tail the
// video stopped on its own last frame: 288 frames produced for 289, 300 and
// 336 declared at 23.976 fps, a whole last segment missing each time.
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/ffmpeg/ -run TestGridEOSIntegration -v

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/eveyproject/evey/internal/pkg/cmafparity"
)

func TestGridEOSIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest` - needs the bundled ffmpeg)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	for _, tc := range []struct {
		rate  string
		fps   float64 // 0 with vfr: the legacy 3 s clock
		video float64 // seconds of picture: a whole number of segments, the edge case
		extra float64 // seconds of audio past the picture
	}{
		{"24000/1001", 23.976, 12.012, 0.01}, {"24000/1001", 23.976, 12.012, 0.5}, {"24000/1001", 23.976, 12.012, 2},
		{"25", 25, 12, 0.01}, {"25", 25, 12, 0.5}, {"25", 25, 12, 2},
		{"30000/1001", 29.97, 12.012, 0.01}, {"30000/1001", 29.97, 12.012, 0.5},
		// Matroska keeps 59.94 fps at the millisecond: probed 19001/317, which
		// put frame 180 a hair before 3.003 s and the first cut at 6.006.
		{"60000/1001", 59.94, 12.012, 0.5}, {"120000/1001", 119.88, 12.012, 0.5},
		{"30", 30, 12, 0.5}, {"50", 50, 12, 0.5}, {"60", 60, 12, 0.5},
		// A variable frame rate keeps the legacy clock and no frame count: the
		// audio outlasting the picture past a 3 s boundary declared a last
		// segment no encode produced (a 21 ms one, a 30 s wait, a 503).
		{"vfr", 0, 12, 0.5},
	} {
		t.Run(fmt.Sprintf("%s+%.2fs", tc.rate, tc.extra), func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "src.mkv")
			rate, vfr := tc.rate, []string{}
			if tc.rate == "vfr" {
				rate, vfr = "30000/1001", []string{"-vf", "select='not(eq(mod(n\\,5)\\,4))'", "-fps_mode", "vfr"}
			}
			gen := exec.CommandContext(ctx, "ffmpeg", append(append([]string{"-hide_banner", "-v", "error", "-y",
				"-f", "lavfi", "-t", fmt.Sprint(tc.video), "-i", "testsrc2=size=320x180:rate=" + rate,
				"-f", "lavfi", "-t", fmt.Sprint(tc.video + tc.extra), "-i", "anoisesrc=color=pink:seed=7:sample_rate=48000"}, vfr...),
				"-c:v", "libx264", "-preset", "ultrafast", "-bf", "2", "-pix_fmt", "yuv420p", "-c:a", "aac", "-ac", "2", src)...)
			if out, err := gen.CombinedOutput(); err != nil {
				t.Fatalf("source: %v\n%s", err, out)
			}
			grid := transcoding.GridForFrameRate(tc.fps)
			durationMs := probeDurationMs(t, ctx, src)
			declared := grid.SegmentCount(durationMs)

			// The video rung as DASH serves it: video alone, audio apart.
			vdir := t.TempDir()
			args := BuildFFmpegArgs(transcoding.HLSProcessOpts{
				FilePath: src, OutputDir: vdir, VideoDecision: "transcode", AudioDecision: "transcode",
				AudioTrackIndex: -1, SubtitleTrackIndex: -1, UseFMP4: true, Grid: grid, DeclaredDurationMs: durationMs,
				Profiles: []transcoding.HLSProfile{{Name: "180p-itest", Width: 320, Height: 180, VideoBitrate: 600, AudioBitrate: 96}},
			})
			if out, err := exec.CommandContext(ctx, "ffmpeg", append([]string{"-hide_banner", "-v", "error", "-y"}, args...)...).CombinedOutput(); err != nil {
				t.Fatalf("video rung: %v\n%s", err, out)
			}
			adir := t.TempDir()
			aargs := BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{FilePath: src, OutputDir: adir, SourceTrackIdx: 0, AudioCodec: "aac", AudioBitrate: 96, MaxAudioChannels: 2, Grid: grid})
			if out, err := exec.CommandContext(ctx, "ffmpeg", append([]string{"-hide_banner", "-v", "error", "-y"}, aargs...)...).CombinedOutput(); err != nil {
				t.Fatalf("audio rendition: %v\n%s", err, out)
			}
			video, _ := filepath.Glob(filepath.Join(vdir, "stream_0_[0-9]*.m4s"))
			audio, _ := filepath.Glob(filepath.Join(adir, "audio_0_[0-9]*.m4s"))
			if !grid.Exact() {
				// No frame count on the legacy clock: every declared segment exists,
				// the last one included.
				t.Logf("container %d ms: declared %d segments, video produced %d, audio %d", durationMs, declared, len(video), len(audio))
				if len(video) != declared || len(audio) < declared {
					t.Errorf("the plan declares %d segments, the video rung produced %d, the audio rendition %d", declared, len(video), len(audio))
				}
				return
			}
			frames := 0
			for _, v := range video {
				out, _ := exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("cat %s %s | ffprobe -v error -count_packets -show_entries stream=nb_read_packets -of csv=p=0 -", filepath.Join(vdir, "stream_0_init.mp4"), v)).Output()
				var n int
				fmt.Sscan(string(out), &n)
				frames += n
			}
			t.Logf("container %d ms: declared %d segments / %d frames, video produced %d segments / %d frames, audio produced %d", durationMs, declared, grid.FrameCount(durationMs), len(video), frames, len(audio))
			if len(video) != declared || int64(frames) != grid.FrameCount(durationMs) {
				t.Errorf("the plan declares %d segments / %d frames, the video rung produced %d / %d", declared, grid.FrameCount(durationMs), len(video), frames)
			}
			// Every span, the last one included, is the declared one.
			decl := cmafparity.Declared{
				Timescale: int64(grid.Timescale),
				Start:     grid.SegmentStartTicks,
				Dur:       func(n int) int64 { return grid.SegmentDurationTicks(n, durationMs) },
				Count:     declared,
			}
			if res, verr := cmafparity.Verify(ctx, vdir, "stream_0", false, decl, 0); verr != nil || !res.OK() || len(res.Extra) != 0 {
				t.Errorf("video rung off its declared timeline: %v %+v", verr, res)
			}

			// A seek session to the second-to-last segment and a two-rung ladder
			// end on the same declared frame.
			sdir := t.TempDir()
			sopts := transcoding.HLSProcessOpts{
				FilePath: src, OutputDir: sdir, VideoDecision: "transcode", AudioDecision: "transcode",
				AudioTrackIndex: -1, SubtitleTrackIndex: -1, UseFMP4: true, Grid: grid, DeclaredDurationMs: durationMs,
				StartSec: grid.SegmentStartSeconds(declared - 2), StartNumber: declared - 2,
				Profiles: []transcoding.HLSProfile{{Name: "180p-itest", Width: 320, Height: 180, VideoBitrate: 600, AudioBitrate: 96}},
			}
			h, startErr := NewHLSTranscoder().Start(ctx, sopts)
			if startErr != nil {
				t.Fatalf("seek session: %v", startErr)
			}
			awaitSegmentFile(t, ctx, filepath.Join(sdir, fmt.Sprintf("stream_0_%03d.m4s", declared-1)), 30*time.Second)
			<-h.Done()
			_ = h.Stop()
			if res, verr := cmafparity.Verify(ctx, sdir, "stream_0", false, decl, 0); verr != nil || !res.OK() || len(res.Extra) != 0 || len(res.Indices) != 2 {
				t.Errorf("seek session to segment %d off the declared end: %v %+v", declared-2, verr, res)
			}
			// A seek session to the last segment, which starts where the picture
			// ends: no source frame at or after its start, nothing for the tail to
			// clone (ops harness 2026-10-19: no fragment, a 55 s wait, a respawn).
			tdir := t.TempDir()
			topts := sopts
			topts.OutputDir, topts.StartSec, topts.StartNumber = tdir, grid.SegmentStartSeconds(declared-1), declared-1
			th, tailErr := NewHLSTranscoder().Start(ctx, topts)
			if tailErr != nil {
				t.Fatalf("tail seek session: %v", tailErr)
			}
			awaitSegmentFile(t, ctx, filepath.Join(tdir, fmt.Sprintf("stream_0_%03d.m4s", declared-1)), 30*time.Second)
			<-th.Done()
			_ = th.Stop()
			if res, verr := cmafparity.Verify(ctx, tdir, "stream_0", false, decl, 0); verr != nil || !res.OK() || len(res.Extra) != 0 {
				t.Errorf("seek session to the last segment %d off the declared end: %v %+v", declared-1, verr, res)
			}
			ldir := t.TempDir()
			largs := BuildFFmpegArgs(transcoding.HLSProcessOpts{
				FilePath: src, OutputDir: ldir, VideoDecision: "transcode", AudioDecision: "transcode",
				AudioTrackIndex: -1, SubtitleTrackIndex: -1, UseFMP4: true, Grid: grid, DeclaredDurationMs: durationMs,
				Profiles: []transcoding.HLSProfile{
					{Name: "180p", Width: 320, Height: 180, VideoBitrate: 600, AudioBitrate: 96},
					{Name: "90p", Width: 160, Height: 90, VideoBitrate: 300, AudioBitrate: 96},
				},
			})
			if out, lerr := exec.CommandContext(ctx, "ffmpeg", append([]string{"-hide_banner", "-v", "error", "-y"}, largs...)...).CombinedOutput(); lerr != nil {
				t.Fatalf("ladder: %v\n%s", lerr, out)
			}
			for i := range 2 {
				if res, verr := cmafparity.Verify(ctx, ldir, fmt.Sprintf("stream_%d", i), false, decl, 0); verr != nil || !res.OK() || len(res.Extra) != 0 || len(res.Indices) != declared {
					t.Errorf("ladder rung %d off the declared timeline: %v %+v", i, verr, res)
				}
			}

			if len(audio) < declared {
				t.Errorf("the plan declares %d segments, the audio rendition produced %d", declared, len(audio))
			}
		})
	}
}
