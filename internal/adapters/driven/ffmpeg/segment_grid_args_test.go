package ffmpeg

import (
	"slices"
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// On a frame-exact grid the encoder is told the grid three ways: a keyframe
// every FramesPerSegment frames (with the GOP and scene-cut pinned to it),
// -hls_time equal to the exact segment length, and the video track timescale
// the manifest declares in. The legacy grid keeps the former arguments, byte
// for byte.
func TestGridArgs_FrameExactVersusLegacy(t *testing.T) {
	t.Parallel()
	exact := transcoding.HLSProcessOpts{VideoDecision: "transcode", Grid: transcoding.GridForFrameRate(23.976)}
	if got := forceKeyframesArgsFor(exact); !slices.Equal(got, []string{"-force_key_frames", "expr:eq(mod(n,72),0)", "-g", "72", "-keyint_min", "72", "-sc_threshold", "0"}) {
		t.Fatalf("frame-exact keyframes: %v", got)
	}
	if got := hlsTimeFor(exact); got != "3.003000" {
		t.Fatalf("hls_time must be the exact segment length: %q", got)
	}
	if got := videoTimescaleArgs(exact); !slices.Equal(got, []string{"-video_track_timescale", "24000"}) {
		t.Fatalf("timescale: %v", got)
	}

	legacy := transcoding.HLSProcessOpts{VideoDecision: "transcode"}
	if got := forceKeyframesArgsFor(legacy); !slices.Equal(got, hlsForceKeyframesArgs) {
		t.Fatalf("legacy keyframes must be unchanged: %v", got)
	}
	if got := hlsTimeFor(legacy); got != "3" {
		t.Fatalf("legacy hls_time: %q", got)
	}
	if got := videoTimescaleArgs(legacy); got != nil {
		t.Fatalf("legacy grid pins no timescale: %v", got)
	}

	// The copy rung splits at every keyframe and keeps the source's own
	// timescale, grid or not.
	copyRung := transcoding.HLSProcessOpts{VideoDecision: "copy", Grid: transcoding.GridForFrameRate(25)}
	if got := hlsTimeFor(copyRung); got != "0" {
		t.Fatalf("copy hls_time: %q", got)
	}
	if got := videoTimescaleArgs(copyRung); got != nil {
		t.Fatalf("copy keeps the source timescale: %v", got)
	}
}

// A seek session's -start_number is the nearest grid index of its start, not
// a truncation: 39 x 3.003 s must read as 39 whatever the float says.
func TestStartNumberFor_RoundsOntoTheGrid(t *testing.T) {
	t.Parallel()
	g := transcoding.GridForFrameRate(23.976)
	for _, n := range []int{1, 39, 400, 1199} {
		opts := transcoding.HLSProcessOpts{VideoDecision: "transcode", Grid: g, StartSec: g.SegmentStartSeconds(n)}
		if got := startNumberFor(opts); got != n {
			t.Fatalf("segment %d: start number %d", n, got)
		}
	}
	legacy := transcoding.HLSProcessOpts{VideoDecision: "transcode", StartSec: 153}
	if got := startNumberFor(legacy); got != 51 {
		t.Fatalf("legacy 153 s: %d", got)
	}
}

// The whole argv of a frame-exact single-rung session carries the grid, and
// the audio-only rendition cuts on the same cadence.
func TestBuildFFmpegArgs_CarriesTheGrid(t *testing.T) {
	t.Parallel()
	g := transcoding.GridForFrameRate(29.97)
	opts := transcoding.HLSProcessOpts{
		FilePath: "/f.mkv", OutputDir: "/out", VideoDecision: "transcode", AudioDecision: "transcode",
		SubtitleTrackIndex: -1, UseFMP4: true, Grid: g,
		Profiles: []transcoding.HLSProfile{{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2800, AudioBitrate: 192}},
	}
	argv := strings.Join(BuildFFmpegArgs(opts), " ")
	for _, want := range []string{"-force_key_frames expr:eq(mod(n,90),0)", "-g 90 -keyint_min 90 -sc_threshold 0", "-video_track_timescale 30000", "-hls_time 3.003000"} {
		if !strings.Contains(argv, want) {
			t.Fatalf("argv lacks %q:\n%s", want, argv)
		}
	}
	audio := strings.Join(BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{
		FilePath: "/f.mkv", OutputDir: "/out", AudioCodec: "aac", Grid: g,
	}), " ")
	if !strings.Contains(audio, "-hls_time 3.003000") {
		t.Fatalf("audio rendition must cut on the video's cadence:\n%s", audio)
	}
}

// With a declared duration on an exact grid every video output holds its last
// frame up to it and stops on its last declared frame - counted from the
// session's first segment. A copy, the legacy grid or no declared duration
// keep the former arguments.
func TestGridArgs_EndOfStreamTail(t *testing.T) {
	t.Parallel()
	grid := transcoding.GridForFrameRate(23.976)
	base := transcoding.HLSProcessOpts{
		VideoDecision: "transcode", AudioTrackIndex: -1, SubtitleTrackIndex: -1, UseFMP4: true, Grid: grid,
		DeclaredDurationMs: 120_641, // 2892 frames on the grid
		FilePath:           "/f.mkv", OutputDir: "/tmp/o",
		Profiles: []transcoding.HLSProfile{{Name: "360p", Width: 640, Height: 360, VideoBitrate: 800, AudioBitrate: 128}},
	}
	cold := strings.Join(BuildFFmpegArgs(base), " ")
	if !strings.Contains(cold, ",tpad=stop_mode=clone:stop=-1") || !strings.Contains(cold, "-frames:v 2892") {
		t.Fatalf("cold session: %s", cold)
	}
	seek := base
	seek.StartSec, seek.StartNumber = grid.SegmentStartSeconds(40), 40
	if got := strings.Join(BuildFFmpegArgs(seek), " "); !strings.Contains(got, "-frames:v 12") {
		t.Fatalf("a session from segment 40 has 2892 - 40x72 frames left: %s", got)
	}
	ladder := base
	ladder.Profiles = append(ladder.Profiles, transcoding.HLSProfile{Name: "180p", Width: 320, Height: 180, VideoBitrate: 300, AudioBitrate: 96})
	if got := strings.Join(BuildFFmpegArgs(ladder), " "); strings.Count(got, "tpad=") != 2 || !strings.Contains(got, "-frames:v 2892") {
		t.Fatalf("every rung of a ladder: %s", got)
	}
	for name, o := range map[string]transcoding.HLSProcessOpts{
		"legacy grid":          func() transcoding.HLSProcessOpts { o := base; o.Grid = transcoding.SegmentGrid{}; return o }(),
		"no declared duration": func() transcoding.HLSProcessOpts { o := base; o.DeclaredDurationMs = 0; return o }(),
	} {
		if got := strings.Join(BuildFFmpegArgs(o), " "); strings.Contains(got, "tpad") || strings.Contains(got, "-frames:v") {
			t.Fatalf("%s: %s", name, got)
		}
	}
	if eosFrameArgs(transcoding.HLSProcessOpts{VideoDecision: "copy", Grid: grid, DeclaredDurationMs: 120_641}) != nil {
		t.Fatal("a copy is never padded")
	}
}
