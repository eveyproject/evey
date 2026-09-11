package ffmpeg

import (
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// startNumberFor must derive the -start_number from StartSec for transcode
// (uniform segments) but take the absolute index verbatim for copy
// (keyframe-aligned variable-length segments) - otherwise a forward seek emits
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
	// With an explicit probed target, -ss must be exactly that value (not
	// derived from the landed StartSec, which would re-trigger the ffmpeg CLI
	// seek rules one keyframe earlier).
	probed := base
	probed.StartSec, probed.CopyOnSeek, probed.SeekTargetSec = 514.661, true, 520.050
	if pa := joined(buildCopyVideoArgs(probed)); !strings.Contains(pa, " -ss 520.050 ") {
		t.Fatalf("probed copy seek must emit -ss SeekTargetSec verbatim, got: %s", pa)
	}
	if !strings.Contains(a, " -c:v copy ") {
		t.Fatalf("copy-on-seek must -c:v copy, got: %s", a)
	}
	if strings.Contains(a, "libx264") {
		t.Fatalf("copy-on-seek must NOT re-encode, got libx264: %s", a)
	}
	// The shared fMP4 init (stream_N_init.mp4) must stay canonical across sessions:
	// -copyts pushes the seek offset into the init's edit list (elst media_time),
	// presentation-shifting EVERY segment served against that shared init. The seek
	// session must reset to tfdt=0 (make_zero) and leave the init byte-identical.
	if strings.Contains(a, "-copyts") {
		t.Fatalf("copy-on-seek must NOT use -copyts (poisons the shared init's edit list): %s", a)
	}
	if !strings.Contains(a, "-avoid_negative_ts make_zero") {
		t.Fatalf("copy-on-seek must reset timestamps with make_zero so the shared init stays canonical: %s", a)
	}
	// The session must keep the decoded audio from the seek LANDING: the default
	// accurate-seek discards audio between the landing and the -ss target while
	// the muxer labels the first kept frame at the landing → audio content plays
	// EARLY by (target − landing), the deep-seek A/V desync.
	if !strings.Contains(a, "-noaccurate_seek") {
		t.Fatalf("copy-on-seek must use -noaccurate_seek (audio must start at the landing, not the target): %s", a)
	}
	// Keyframe-seek nudge: -ss must sit just PAST StartSec (520 + 0.05 = 520.050),
	// not AT it. -ss exactly at/below the keyframe snaps to the PREVIOUS keyframe →
	// a two-GOP segment with the audio a full GOP behind the video (the "son décalé"
	// bug). The nudge lands the copy on the intended keyframe.
	if !strings.Contains(a, "-ss 520.050") {
		t.Fatalf("copy-on-seek must nudge -ss past the keyframe (expected -ss 520.050), got: %s", a)
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

// The copy rung MUST run `-hls_time 0` - split at EVERY keyframe - because that
// is the only ffmpeg split rule that is independent of where the session
// started. Any hls_time > 0 splits on an absolute n×hls_time grid relative to
// the SESSION start: on long-GOP sources the grid lags until ffmpeg cuts at
// every keyframe anyway while the plan coalesces, so the produced
// stream_N_NNN.m4s drift from the #EXTINF indices the player addresses
// (measured in prod: minutes of drift + the A/V desync). The every-keyframe
// plan (KeyframeSegments) is the exact mirror of -hls_time 0.
func TestHLSTimeFor_CopySplitsAtEveryKeyframe(t *testing.T) {
	transcode := transcoding.HLSProcessOpts{}
	if got := hlsTimeFor(transcode); got != hlsSegmentSeconds {
		t.Fatalf("transcode -hls_time = %q, want default %q", got, hlsSegmentSeconds)
	}
	if got := hlsTimeFor(transcoding.HLSProcessOpts{VideoDecision: "copy"}); got != "0" {
		t.Fatalf("copy -hls_time = %q, want \"0\" (split at every keyframe)", got)
	}

	copyArgs := joined(buildCopyVideoArgs(transcoding.HLSProcessOpts{
		FilePath: "/m.mkv", VideoDecision: "copy",
	}))
	if !strings.Contains(copyArgs, " -hls_time 0 ") {
		t.Fatalf("copy rung must emit -hls_time 0, got: %s", copyArgs)
	}
}

// Copy sessions are paced (-readrate) with a full-speed initial burst so video
// copy cannot race minutes ahead of the CPU-bound audio re-encode (the prod
// A/V interleave desync), while startup/seek stay instant. ReadRate=0 must
// emit no pacing flags at all.
func TestBuildCopyVideoArgs_PacingBurst(t *testing.T) {
	paced := joined(buildCopyVideoArgs(transcoding.HLSProcessOpts{
		FilePath: "/m.mkv", VideoDecision: "copy", ReadRate: 8,
	}))
	if !strings.Contains(paced, " -readrate 8") {
		t.Fatalf("paced copy must emit -readrate, got: %s", paced)
	}
	if !strings.Contains(paced, " -readrate_initial_burst 30 ") {
		t.Fatalf("paced copy must emit -readrate_initial_burst, got: %s", paced)
	}
	unpaced := joined(buildCopyVideoArgs(transcoding.HLSProcessOpts{
		FilePath: "/m.mkv", VideoDecision: "copy",
	}))
	if strings.Contains(unpaced, "readrate") {
		t.Fatalf("ReadRate=0 must emit no readrate flags, got: %s", unpaced)
	}
}

// The "-profile:v high -level:v 4.1" pair is h264-only. libx265 aborts on it
// ("x265 [error]: unknown profile <high>" → the whole session dies), so a
// software-HEVC transcode (client PreferredVideoCodec=hevc, no HW HEVC) must
// emit NO profile/level and let x265 derive it from the pixel format.
func TestSoftwareProfileLevelArgs_H264Only(t *testing.T) {
	if got := joined(softwareProfileLevelArgs("libx264", -1, 1080, 0)); !strings.Contains(got, "-profile:v high") {
		t.Fatalf("libx264 single must get -profile:v high, got %s", got)
	}
	if got := joined(softwareProfileLevelArgs("libx264", 2, 1080, 0)); !strings.Contains(got, "-profile:v:2 high") {
		t.Fatalf("libx264 multi must get -profile:v:2 high, got %s", got)
	}
	if got := softwareProfileLevelArgs("libx265", -1, 1080, 0); got != nil {
		t.Fatalf("libx265 must get NO profile/level (x265 rejects \"high\"), got %v", got)
	}
	if got := joined(softwareProfileLevelArgs("libx264", -1, 2160, 0)); !strings.Contains(got, "-level:v 5.1") {
		t.Fatalf("a 2160p rung must be signalled level 5.1, got %s", got)
	}
	if got := softwareProfileLevelArgs("libsvtav1", 1, 1080, 0); got != nil {
		t.Fatalf("av1 must get NO profile/level, got %v", got)
	}
	if got := joined(softwareProfileLevelArgs("libx264", -1, 1080, 40)); !strings.Contains(got, "-level:v 4.0") {
		t.Fatalf("a level-4.0 decoder must lower the signalled level, got %s", got)
	}
	if got := joined(softwareProfileLevelArgs("libx264", -1, 720, 51)); !strings.Contains(got, "-level:v 4.1") {
		t.Fatalf("a cap above the natural level must not raise it, got %s", got)
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

func TestSvtAV1PresetArgs(t *testing.T) {
	// libsvtav1 needs the numeric preset; the named ladder fails to open it.
	if got := svtAV1PresetArgs("libsvtav1"); len(got) != 2 || got[0] != "-preset" || got[1] != svtAV1Preset {
		t.Fatalf("libsvtav1 must get -preset %s, got %v", svtAV1Preset, got)
	}
	// x264/x265 keep the named preset.
	for _, enc := range []string{"libx264", "libx265"} {
		got := svtAV1PresetArgs(enc)
		if len(got) != 2 || got[0] != "-preset" || got[1] != "veryfast" {
			t.Fatalf("%s must keep -preset veryfast, got %v", enc, got)
		}
	}
}

// Audio bitrate scales with the channel target (ladder bitrates are sized for
// stereo): 5.1 at 192k would starve; ~64k/channel, floor=base, cap 512k.
func TestScaledAudioBitrateKbps(t *testing.T) {
	cases := []struct {
		base int64
		ch   int
		want int64
	}{
		{192, 0, 192}, {192, 2, 192}, {192, 6, 384}, {128, 6, 384},
		{192, 8, 512}, {448, 6, 448}, {600, 6, 600},
	}
	for _, c := range cases {
		if got := scaledAudioBitrateKbps(c.base, c.ch); got != c.want {
			t.Fatalf("scaledAudioBitrateKbps(%d,%d)=%d want %d", c.base, c.ch, got, c.want)
		}
	}
}

// The copy rung's re-encoded audio follows the same sizing and sample-rate
// rules as every other builder: per-channel bitrate, 48 kHz.
func TestBuildCopyVideoArgs_AudioFollowsChannelSizing(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		FilePath: "/m.mkv", OutputDir: "/o", VideoDecision: "copy", AudioDecision: "transcode",
		AudioTrackIndex: 0, MaxAudioChannels: 6, UseFMP4: true,
		Profiles: []transcoding.HLSProfile{{Name: "1080p-copy-kf4", Width: 1920, Height: 1080, VideoBitrate: 5000, AudioBitrate: 192, Copy: true}},
	}
	a := joined(buildCopyVideoArgs(opts))
	if !strings.Contains(a, "-ac 6 -ar 48000 -b:a 384k") {
		t.Fatalf("5.1 copy-rung audio must be 384k at 48 kHz: %s", a)
	}
	opts.MaxAudioChannels = 2
	if a := joined(buildCopyVideoArgs(opts)); !strings.Contains(a, "-ac 2 -ar 48000 -b:a 192k") {
		t.Fatalf("stereo keeps the rung base: %s", a)
	}
}
