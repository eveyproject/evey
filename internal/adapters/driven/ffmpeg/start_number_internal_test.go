package ffmpeg

import (
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// startNumberFor must derive the -start_number from StartSec for transcode
// (uniform segments) but take the absolute index verbatim for copy
// (keyframe-aligned variable-length segments) — otherwise a forward seek emits
// the wrong stream_N_<idx>.m4s and the player loads forever.
func TestStartNumberFor(t *testing.T) {
	cases := []struct {
		name string
		opts transcoding.HLSProcessOpts
		want int
	}{
		{"transcode cold start", transcoding.HLSProcessOpts{VideoDecision: "transcode", StartSec: 0}, 0},
		{"transcode mid-stream derives floor(StartSec/3)", transcoding.HLSProcessOpts{VideoDecision: "transcode", StartSec: 153}, 51},
		{"copy cold start", transcoding.HLSProcessOpts{VideoDecision: "copy", StartNumber: 0, StartSec: 0}, 0},
		{
			// The seek bug: a copy seek to plan-segment 80 lands at a keyframe time
			// (520s) whose floor(520/3)=173 ≠ 80. start_number must be the absolute
			// plan index (80), not the time-derived value.
			"copy seek uses absolute index, not StartSec/segLen",
			transcoding.HLSProcessOpts{VideoDecision: "copy", StartNumber: 80, StartSec: 520},
			80,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := startNumberFor(tc.opts); got != tc.want {
				t.Fatalf("startNumberFor(%+v) = %d, want %d", tc.opts, got, tc.want)
			}
		})
	}
}

func joined(a []string) string { return " " + strings.Join(a, " ") + " " }

// The copy rung must keep -c:v copy on a mid-stream (seek) start: re-encoding
// there emits a libx264 (Baseline) init that overwrites the shared source-High
// init → scrambled frames. CopyOnSeek gates copy vs the legacy libx264 seek.
func TestBuildCopyVideoArgs_CopyOnSeek(t *testing.T) {
	base := transcoding.HLSProcessOpts{
		FilePath: "/m.mkv", VideoDecision: "copy", AudioDecision: "copy", AudioTrackIndex: 0,
	}

	seekCopy := base
	seekCopy.StartSec, seekCopy.CopyOnSeek = 520, true
	a := joined(buildCopyVideoArgs(seekCopy))
	if !strings.Contains(a, " -c:v copy ") {
		t.Fatalf("copy-on-seek must -c:v copy, got: %s", a)
	}
	if strings.Contains(a, "libx264") {
		t.Fatalf("copy-on-seek must NOT re-encode, got libx264: %s", a)
	}

	seekReencode := base
	seekReencode.StartSec, seekReencode.CopyOnSeek = 520, false
	b := joined(buildCopyVideoArgs(seekReencode))
	if !strings.Contains(b, "libx264") {
		t.Fatalf("legacy seek (CopyOnSeek=false) should keep the libx264 path, got: %s", b)
	}

	cold := base
	cold.StartSec, cold.CopyOnSeek = 0, true
	c := joined(buildCopyVideoArgs(cold))
	if !strings.Contains(c, " -c:v copy ") {
		t.Fatalf("cold copy start must -c:v copy, got: %s", c)
	}
}

// The copy rung MUST run -hls_time at the plan's keyframe target (SegmentSeconds),
// not the default "3": with -c:v copy ffmpeg splits at the first keyframe >=
// hls_time, so hls_time must match the manifest's KeyframeSegments target or the
// produced segments drift from the #EXTINF indices the player addresses.
func TestHLSTimeFor_CopyMatchesPlanTarget(t *testing.T) {
	transcode := transcoding.HLSProcessOpts{} // SegmentSeconds 0
	if got := hlsTimeFor(transcode); got != hlsSegmentSeconds {
		t.Fatalf("transcode -hls_time = %q, want default %q", got, hlsSegmentSeconds)
	}
	if got := hlsTimeFor(transcoding.HLSProcessOpts{SegmentSeconds: 6}); got != "6" {
		t.Fatalf("copy -hls_time = %q, want \"6\"", got)
	}

	copyArgs := joined(buildCopyVideoArgs(transcoding.HLSProcessOpts{
		FilePath: "/m.mkv", VideoDecision: "copy", SegmentSeconds: 6,
	}))
	if !strings.Contains(copyArgs, " -hls_time 6 ") {
		t.Fatalf("copy rung must emit -hls_time 6, got: %s", copyArgs)
	}
	if strings.Contains(copyArgs, " -hls_time 3 ") {
		t.Fatalf("copy rung must NOT use the default -hls_time 3, got: %s", copyArgs)
	}
}

// The "-profile:v high -level:v 4.1" pair is h264-only. libx265 aborts on it
// ("x265 [error]: unknown profile <high>" → the whole session dies), so a
// software-HEVC transcode (client PreferredVideoCodec=hevc, no HW HEVC) must
// emit NO profile/level and let x265 derive it from the pixel format.
func TestSoftwareProfileLevelArgs_H264Only(t *testing.T) {
	if got := joined(softwareProfileLevelArgs("libx264", -1)); !strings.Contains(got, "-profile:v high") {
		t.Fatalf("libx264 single must get -profile:v high, got %s", got)
	}
	if got := joined(softwareProfileLevelArgs("libx264", 2)); !strings.Contains(got, "-profile:v:2 high") {
		t.Fatalf("libx264 multi must get -profile:v:2 high, got %s", got)
	}
	if got := softwareProfileLevelArgs("libx265", -1); got != nil {
		t.Fatalf("libx265 must get NO profile/level (x265 rejects \"high\"), got %v", got)
	}
	if got := softwareProfileLevelArgs("libaom-av1", 1); got != nil {
		t.Fatalf("av1 must get NO profile/level, got %v", got)
	}

	hevc := joined(BuildFFmpegArgs(transcoding.HLSProcessOpts{
		FilePath: "/m.mkv", VideoDecision: "transcode", AudioDecision: "copy",
		VideoCodec: "hevc", Profiles: transcoding.DefaultHLSProfiles[:1],
	}))
	if !strings.Contains(hevc, "libx265") {
		t.Fatalf("hevc must select libx265: %s", hevc)
	}
	if strings.Contains(hevc, "-profile:v high") || strings.Contains(hevc, "-profile:v:0 high") {
		t.Fatalf("hevc/libx265 must NOT carry the h264 'high' profile (x265 aborts): %s", hevc)
	}
}
