package ffmpeg

// Audio start-delay cancellation INTEGRATION test: BuildFFmpegArgs against the
// real bundled ffmpeg, on a source whose audio track starts 0.9s after the
// video (the repack mux defect measured in prod: a dubbed title, constant
// A/V desync). Args-level unit tests prove the -itsoffset second input is
// EMITTED; only decoding the served segments proves the audio CONTENT lands on
// the scene the video shows - the seek/trim/zero-point interactions live
// entirely inside the ffmpeg binary.
//
// OPT-IN like the other transcode itests: run via `make transcode-itest` or
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/ffmpeg/ -run TestAudioDelayIntegration -v

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// genDelayedAudioSource writes clean.mkv (video and audio aligned at 0) and
// delayed.mkv (same content, audio track labels shifted +delaySec - the mux
// defect). Pink noise audio: aperiodic, so content correlation has one sharp
// peak. Keyframes every 6s, B-frames on (arms the CLI's DTS seek heuristic).
func genDelayedAudioSource(t *testing.T, ctx context.Context, dir string, delaySec float64) (clean, delayed string) {
	t.Helper()
	clean = filepath.Join(dir, "clean.mkv")
	delayed = filepath.Join(dir, "delayed.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "anoisesrc=color=pink:seed=42:sample_rate=48000",
		"-t", "60",
		"-c:v", "libx264", "-preset", "ultrafast", "-bf", "3",
		"-force_key_frames", "expr:gte(t,n_forced*6)",
		"-sc_threshold", "0", "-g", "9999", "-pix_fmt", "yuv420p",
		"-c:a", "ac3", "-ac", "2",
		clean)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("clean source generation: %v\n%s", err, out)
	}
	mux := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-i", clean,
		"-itsoffset", fmt.Sprintf("%.3f", delaySec), "-i", clean,
		"-map", "0:v:0", "-map", "1:a:0", "-c", "copy",
		delayed)
	if out, err := mux.CombinedOutput(); err != nil {
		t.Fatalf("delayed remux: %v\n%s", err, out)
	}
	return clean, delayed
}

func TestAudioDelayIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest` - needs the bundled ffmpeg)")
	}
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not in PATH", bin)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	dir := t.TempDir()

	const delay = 0.9
	clean, delayed := genDelayedAudioSource(t, ctx, dir, delay)

	// The probe must measure the injected defect from the real container.
	probed, err := probeAudioStartDelaySec(ctx, "ffprobe", delayed, 0)
	if err != nil {
		t.Fatalf("probeAudioStartDelaySec: %v", err)
	}
	if math.Abs(probed-delay) > 0.05 {
		t.Fatalf("probed delay %.3fs, injected %.3fs", probed, delay)
	}

	baseOpts := func() transcoding.HLSProcessOpts {
		return transcoding.HLSProcessOpts{
			FilePath:           delayed,
			VideoDecision:      "transcode",
			AudioDecision:      "transcode",
			AudioTrackIndex:    0,
			SubtitleTrackIndex: -1,
			UseFMP4:            true,
			AudioStartDelaySec: probed,
			Profiles: []transcoding.HLSProfile{
				{Name: "180p-itest", Width: 320, Height: 180, VideoBitrate: 800, AudioBitrate: 128},
			},
		}
	}

	// run executes the builder's args and returns (outDir, sorted variant-0
	// segment paths).
	run := func(t *testing.T, opts transcoding.HLSProcessOpts) (string, []string) {
		t.Helper()
		outDir := t.TempDir()
		opts.OutputDir = outDir
		args := BuildFFmpegArgs(opts)
		if out, err := exec.CommandContext(ctx, "ffmpeg", append([]string{"-hide_banner", "-v", "error", "-y"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("ffmpeg run: %v\n%s", err, out)
		}
		segs, err := filepath.Glob(filepath.Join(outDir, "stream_0_[0-9]*.m4s"))
		if err != nil || len(segs) == 0 {
			t.Fatalf("no segments produced in %s (%v)", outDir, err)
		}
		return outDir, segs
	}

	// firstPktDelta returns first_audio_pts − first_video_pts (s) of the
	// served variant-0 playlist: the label-level A/V start gap a player sees.
	firstPktDelta := func(t *testing.T, outDir string) float64 {
		t.Helper()
		pl := filepath.Join(outDir, "stream_0.m3u8")
		one := func(sel string) float64 {
			out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error",
				"-select_streams", sel, "-show_entries", "packet=pts_time",
				"-of", "csv=p=0", pl).Output()
			if err != nil {
				t.Fatalf("ffprobe %s packets: %v", sel, err)
			}
			var v float64
			if _, err := fmt.Sscanf(string(out), "%g", &v); err != nil {
				t.Fatalf("ffprobe %s first pts parse: %v (%q)", sel, err, string(out[:min(len(out), 40)]))
			}
			return v
		}
		return one("a") - one("v")
	}

	// correlate returns the offset (s) of the segment's audio content relative
	// to wantSec on the CLEAN timeline (labels == scene time there).
	correlate := func(t *testing.T, outDir, seg string, wantSec float64) float64 {
		t.Helper()
		return correlateAudio(t, ctx, outDir, filepath.Join(outDir, "stream_0_init.mp4"), seg, clean, wantSec)
	}

	// COLD sessions: the defect is a LABEL-level gap (content scene 0 carries
	// label 0.9, the player aligns by label → audio lags 0.9s). Two detectors:
	// firstPktDelta sees the label gap at the stream head; the content
	// correlation anchors on a mid-stream segment (t≈6), where an un-cancelled
	// delay shows as content belonging ~0.9s BEFORE the scene shown.

	// Sensitivity control: WITHOUT the cancel both detectors must show ~0.9.
	// Proves a regression cannot pass vacuously.
	t.Run("control_no_cancel_shows_defect", func(t *testing.T) {
		opts := baseOpts()
		opts.AudioStartDelaySec = 0
		outDir, segs := run(t, opts)
		if d := firstPktDelta(t, outDir); d < 0.6 {
			t.Errorf("control run should start audio ~0.9s after video, measured %+.3fs - the defect fixture is broken", d)
		}
		if off := correlate(t, outDir, segs[2], 6); off > -0.6 {
			t.Errorf("control run should show ~-0.9s content offset at t=6, measured %+.3fs", off)
		}
	})

	t.Run("single_transcode_cold", func(t *testing.T) {
		outDir, segs := run(t, baseOpts())
		if d := firstPktDelta(t, outDir); math.Abs(d) > 0.15 {
			t.Errorf("audio starts %+.3fs from video, want ~0", d)
		}
		if off := correlate(t, outDir, segs[2], 6); math.Abs(off) > 0.25 {
			t.Errorf("audio content offset %+.3fs, want ~0", off)
		}
	})

	t.Run("single_transcode_seek", func(t *testing.T) {
		opts := baseOpts()
		opts.StartSec = 15
		outDir, segs := run(t, opts)
		// start_number floor(15/3)=5 → first produced segment covers t=15.
		if off := correlate(t, outDir, segs[0], 15); math.Abs(off) > 0.25 {
			t.Errorf("audio content offset %+.3fs, want ~0", off)
		}
	})

	t.Run("multi_variant_cold_and_seek", func(t *testing.T) {
		opts := baseOpts()
		opts.VideoDecision = ""
		opts.Profiles = append(opts.Profiles, transcoding.HLSProfile{
			Name: "144p-itest", Width: 256, Height: 144, VideoBitrate: 400, AudioBitrate: 96,
		})
		outDir, segs := run(t, opts)
		if d := firstPktDelta(t, outDir); math.Abs(d) > 0.15 {
			t.Errorf("cold: audio starts %+.3fs from video, want ~0", d)
		}
		if off := correlate(t, outDir, segs[2], 6); math.Abs(off) > 0.25 {
			t.Errorf("cold: audio content offset %+.3fs, want ~0", off)
		}

		opts.StartSec = 15
		outDir, segs = run(t, opts)
		if off := correlate(t, outDir, segs[0], 15); math.Abs(off) > 0.25 {
			t.Errorf("seek: audio content offset %+.3fs, want ~0", off)
		}
	})

	t.Run("copy_video_cold", func(t *testing.T) {
		opts := baseOpts()
		opts.VideoDecision = "copy"
		opts.SourceVideoCodec = "h264"
		outDir, segs := run(t, opts)
		if d := firstPktDelta(t, outDir); math.Abs(d) > 0.15 {
			t.Errorf("audio starts %+.3fs from video, want ~0", d)
		}
		// hls_time 0 splits at every keyframe (6s GOPs): segs[1] covers t=6-12.
		if off := correlate(t, outDir, segs[1], 6); math.Abs(off) > 0.25 {
			t.Errorf("audio content offset %+.3fs, want ~0", off)
		}
	})

	// User resync: +300ms means the audio plays 300ms LATER than true sync,
	// so the content at any label belongs 300ms EARLIER in the source. Both a
	// bare offset (clean source) and an offset composed with the source-delay
	// cancel must land the same net result.
	t.Run("user_offset_composition", func(t *testing.T) {
		bare := baseOpts()
		bare.FilePath = clean // a healthy source: the offset is the ONLY shift
		bare.AudioStartDelaySec = 0
		bare.AudioUserOffsetSec = 0.3
		outDir, segs := run(t, bare)
		if off := correlate(t, outDir, segs[2], 6); math.Abs(off-(-0.3)) > 0.25 {
			t.Errorf("bare +300ms resync: content offset %+.3fs, want ~-0.300", off)
		}

		composed := baseOpts() // AudioStartDelaySec = probed 0.9
		composed.AudioUserOffsetSec = 0.3
		outDir, segs = run(t, composed)
		if off := correlate(t, outDir, segs[2], 6); math.Abs(off-(-0.3)) > 0.25 {
			t.Errorf("cancel 0.9 + user +0.3: content offset %+.3fs, want ~-0.300", off)
		}

		negative := baseOpts()
		negative.FilePath = clean
		negative.AudioStartDelaySec = 0
		negative.AudioUserOffsetSec = -0.3
		outDir, segs = run(t, negative)
		if off := correlate(t, outDir, segs[2], 6); math.Abs(off-0.3) > 0.25 {
			t.Errorf("bare -300ms resync: content offset %+.3fs, want ~+0.300", off)
		}
	})

	t.Run("copy_on_seek", func(t *testing.T) {
		// Target 12.5: past the 12.0 keyframe with margin over the CLI's ~130ms
		// DTS heuristic, so the session lands ON 12.0 like a real plan-driven
		// seek (the production path probes the landing; here it is known).
		opts := baseOpts()
		opts.VideoDecision = "copy"
		opts.SourceVideoCodec = "h264"
		opts.CopyOnSeek = true
		opts.StartSec = 12
		opts.SeekTargetSec = 12.5
		opts.StartNumber = 2
		outDir, segs := run(t, opts)
		seg := segs[0]

		// Zero-point integrity: make_zero must still zero on the VIDEO landing.
		// A hijack by early shifted audio shifts the video track by ~+0.9s and
		// would blow the copy timeline tripwire in production.
		scales := map[uint32]uint32{}
		starts := segmentTrackStartsMs(t, seg, scales)
		if len(starts) < 2 {
			t.Fatalf("want video+audio trafs in the first seek segment, got %d tracks", len(starts))
		}
		var videoStartMs int64 = -1
		for trackID, gotMs := range starts {
			if trackID == 1 { // hlsenc writes the video track first (track_ID 1)
				videoStartMs = gotMs
			}
		}
		if videoStartMs < 0 || videoStartMs > 400 {
			t.Errorf("video track starts at %dms post-make_zero, want ~0 (zero-point hijacked by early audio?)", videoStartMs)
		}

		// Content: the delay-cancelled audio starts at the seek TARGET (the
		// accurate-seek trim point), scene-aligned with the video.
		if off := correlate(t, outDir, seg, opts.SeekTargetSec); math.Abs(off) > 0.25 {
			t.Errorf("audio content offset %+.3fs from the seek target, want ~0", off)
		}
	})
}
