package ffmpeg

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// The args golden file pins the exact ffmpeg command line every builder emits
// for a matrix of sessions. The ladder engine refactor moves where the rate
// control comes from (a Rung's quality target instead of constants in the
// builder); this file is the proof the bytes did not move. Regenerate on a
// DELIBERATE change with `-update` and read the diff.
var updateArgsGolden = flag.Bool("update", false, "rewrite the golden files")

func goldenOpts(mutate func(*transcoding.HLSProcessOpts)) transcoding.HLSProcessOpts {
	o := transcoding.HLSProcessOpts{
		OutputDir:          "/cache/h",
		FilePath:           "/media/film.mkv",
		Profiles:           []transcoding.HLSProfile{{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 5000, AudioBitrate: 192}},
		VideoDecision:      "transcode",
		AudioDecision:      "transcode",
		AudioTrackIndex:    0,
		SubtitleTrackIndex: -1,
		UseFMP4:            true,
		SourceVideoCodec:   "h264",
		VideoCodec:         "h264",
		AudioCodec:         "aac",
		AudioEncoder:       "libfdk_aac",
	}
	if mutate != nil {
		mutate(&o)
	}
	return o
}

var goldenLadder = []transcoding.HLSProfile{
	{Name: "480p", Width: 854, Height: 480, VideoBitrate: 1500, AudioBitrate: 192},
	{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 3500, AudioBitrate: 192},
	{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 8750, AudioBitrate: 192},
	{Name: "2160p", Width: 3840, Height: 2160, VideoBitrate: 70000, AudioBitrate: 256},
}

func goldenSessions() map[string]transcoding.HLSProcessOpts {
	vaapi := func(o *transcoding.HLSProcessOpts) {
		o.HWAccel = transcoding.VAAPI
		o.HWDevice = "/dev/dri/renderD128"
		o.HWFeatures = []string{"bframes_h264", "low_power_h264"}
		o.SourceVideoCodec = "hevc"
	}
	nvenc := func(o *transcoding.HLSProcessOpts) {
		o.HWAccel = transcoding.NVENC
		o.HWFeatures = []string{"rc_lookahead", "spatial_aq", "temporal_aq", "b_ref_mode", "bframes"}
		o.SourceVideoCodec = "hevc"
	}
	qsv := func(o *transcoding.HLSProcessOpts) {
		o.HWAccel = transcoding.QSV
		o.HWDevice = "/dev/dri/renderD128"
		o.HWFeatures = []string{"forced_idr", "low_power_h264"}
	}
	rkmpp := func(o *transcoding.HLSProcessOpts) {
		o.HWAccel = transcoding.RKMPP
		o.SourceVideoCodec = "hevc"
	}
	return map[string]transcoding.HLSProcessOpts{
		"copy-cold": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			o.VideoDecision = "copy"
			o.AudioDecision = "copy"
			o.ReadRate = 4
			o.Profiles[0].Copy = true
		}),
		"copy-seek-plan": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			o.VideoDecision = "copy"
			o.StartSec = 153.2
			o.StartNumber = 51
			o.CopyOnSeek = true
			o.SeekTargetSec = 153.25
			o.ReadRate = 4
			o.SourceVideoCodec = "hevc"
			o.MaxAudioChannels = 6
		}),
		"copy-seek-noplan": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			o.VideoDecision = "copy"
			o.StartSec = 30
			o.AudioDecision = "copy"
		}),
		"sw-single":         goldenOpts(func(o *transcoding.HLSProcessOpts) { o.ReadRate = 1.5 }),
		"sw-single-seek":    goldenOpts(func(o *transcoding.HLSProcessOpts) { o.ReadRate = 1.5; o.StartSec = 60 }),
		"sw-single-tonemap": goldenOpts(func(o *transcoding.HLSProcessOpts) { o.ToneMap = true; o.SourceVideoCodec = "hevc" }),
		"sw-single-burnin":  goldenOpts(func(o *transcoding.HLSProcessOpts) { o.SubtitleTrackIndex = 1; o.SubtitleCodec = "hdmv_pgs_subtitle" }),
		"sw-single-burnin-hdr": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			o.SubtitleTrackIndex = 0
			o.SubtitleCodec = "dvd_subtitle"
			o.ToneMap = true
		}),
		"sw-single-telecine": goldenOpts(func(o *transcoding.HLSProcessOpts) { o.InverseTelecine = true }),
		"sw-single-deint-all": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			o.DeinterlaceAll = true
			o.SourceSAR = "64:45"
			o.Profiles[0] = transcoding.HLSProfile{Name: "480p", Width: 854, Height: 480, VideoBitrate: 1400, AudioBitrate: 192}
		}),
		"sw-single-level31":  goldenOpts(func(o *transcoding.HLSProcessOpts) { o.MaxH264Level = 31 }),
		"sw-single-hevc":     goldenOpts(func(o *transcoding.HLSProcessOpts) { o.VideoCodec = "hevc" }),
		"sw-single-av1":      goldenOpts(func(o *transcoding.HLSProcessOpts) { o.VideoCodec = "av1" }),
		"sw-single-audio-51": goldenOpts(func(o *transcoding.HLSProcessOpts) { o.MaxAudioChannels = 6; o.AudioNormalize = true }),
		"sw-single-audio-delay": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			o.AudioStartDelaySec = 0.35
			o.AudioUserOffsetSec = -0.2
			o.StartSec = 12
			o.ReadRate = 1.5
		}),
		"sw-single-audio-copy": goldenOpts(func(o *transcoding.HLSProcessOpts) { o.AudioDecision = "copy" }),
		"sw-single-noaudio":    goldenOpts(func(o *transcoding.HLSProcessOpts) { o.AudioTrackIndex = -1 }),
		"sw-single-opus":       goldenOpts(func(o *transcoding.HLSProcessOpts) { o.AudioCodec = "opus" }),
		"sw-single-2160p":      goldenOpts(func(o *transcoding.HLSProcessOpts) { o.Profiles = goldenLadder[3:] }),
		"sw-multi": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			o.Profiles = goldenLadder[:3]
			o.VideoDecision = ""
			o.ReadRate = 1.5
		}),
		"sw-multi-tonemap-burnin": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			o.Profiles = goldenLadder[:2]
			o.ToneMap = true
			o.SubtitleTrackIndex = 0
			o.SubtitleCodec = "hdmv_pgs_subtitle"
		}),
		"hw-vaapi-single":      goldenOpts(vaapi),
		"hw-vaapi-single-seek": goldenOpts(func(o *transcoding.HLSProcessOpts) { vaapi(o); o.StartSec = 300; o.Profiles = goldenLadder[3:] }),
		"hw-vaapi-opencl-tonemap": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			vaapi(o)
			o.ToneMap = true
			o.HWTonemap = true
			o.HWFeatures = append(o.HWFeatures, transcoding.FeatureTonemapOpenCL)
		}),
		"hw-vaapi-vulkan-tonemap": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			vaapi(o)
			o.ToneMap = true
			o.HWTonemap = true
			o.HWFeatures = append(o.HWFeatures, transcoding.FeatureTonemapVulkan)
		}),
		"hw-vaapi-vulkan-tonemap-multi": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			vaapi(o)
			o.ToneMap = true
			o.HWTonemap = true
			o.HWFeatures = append(o.HWFeatures, transcoding.FeatureTonemapVulkan)
			o.Profiles = goldenLadder[:3]
			o.VideoDecision = ""
		}),
		"hw-vaapi-opencl-over-vulkan-tonemap": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			vaapi(o)
			o.ToneMap = true
			o.HWTonemap = true
			o.HWFeatures = append(o.HWFeatures, transcoding.FeatureTonemapVulkan, transcoding.FeatureTonemapOpenCL)
		}),
		"hw-vaapi-vpp-tonemap":    goldenOpts(func(o *transcoding.HLSProcessOpts) { vaapi(o); o.ToneMap = true; o.HWTonemap = true }),
		"hw-vaapi-bridge-tonemap": goldenOpts(func(o *transcoding.HLSProcessOpts) { vaapi(o); o.ToneMap = true }),
		"hw-vaapi-hdr-passthrough": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			vaapi(o)
			o.VideoCodec = "hevc"
			o.HDRPassthrough = true
			o.HDRTransfer = "pq"
			o.HDRSupported = true
			o.HWFeatures = []string{"bframes_hevc"}
		}),
		"hw-vaapi-hevc": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			vaapi(o)
			o.VideoCodec = "hevc"
			o.HWFeatures = []string{"bframes_hevc"}
		}),
		"hw-vaapi-decoupled": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			vaapi(o)
			o.SourceVideoCodec = "mpeg4"
			o.HWSoftwareDecode = true
			o.DeinterlaceAll = true
		}),
		"hw-vaapi-overlay": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			vaapi(o)
			o.HWOverlay = true
			o.SubtitleTrackIndex = 2
			o.SubtitleCodec = "hdmv_pgs_subtitle"
		}),
		"hw-vaapi-overlay-tonemap": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			vaapi(o)
			o.HWOverlay = true
			o.SubtitleTrackIndex = 2
			o.SubtitleCodec = "hdmv_pgs_subtitle"
			o.ToneMap = true
			o.HWTonemap = true
		}),
		"hw-vaapi-multi": goldenOpts(func(o *transcoding.HLSProcessOpts) { vaapi(o); o.Profiles = goldenLadder; o.VideoDecision = "" }),
		"hw-vaapi-multi-overlay": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			vaapi(o)
			o.Profiles = goldenLadder[:2]
			o.HWOverlay = true
			o.SubtitleTrackIndex = 0
			o.SubtitleCodec = "dvd_subtitle"
		}),
		"hw-nvenc-single":         goldenOpts(nvenc),
		"hw-nvenc-bridge-tonemap": goldenOpts(func(o *transcoding.HLSProcessOpts) { nvenc(o); o.ToneMap = true }),
		"hw-nvenc-cuda-tonemap": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			nvenc(o)
			o.ToneMap = true
			o.HWTonemap = true
			o.HWFeatures = append(o.HWFeatures, transcoding.FeatureTonemapCUDA)
		}),
		"hw-nvenc-cuda-tonemap-multi": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			nvenc(o)
			o.ToneMap = true
			o.HWTonemap = true
			o.HWFeatures = append(o.HWFeatures, transcoding.FeatureTonemapCUDA)
			o.Profiles = goldenLadder[:3]
			o.VideoDecision = ""
		}),
		"hw-nvenc-av1":   goldenOpts(func(o *transcoding.HLSProcessOpts) { nvenc(o); o.VideoCodec = "av1" }),
		"hw-nvenc-multi": goldenOpts(func(o *transcoding.HLSProcessOpts) { nvenc(o); o.Profiles = goldenLadder[:3]; o.VideoDecision = "" }),
		"hw-qsv-single":  goldenOpts(qsv),
		"hw-qsv-tonemap-bridge": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			qsv(o)
			o.ToneMap = true
			o.HWTonemap = true
			o.SourceVideoCodec = "hevc"
		}),
		"hw-qsv-deinterlace": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			qsv(o)
			o.SourceVideoCodec = "mpeg2video"
			o.HWDeinterlace = true
		}),
		"hw-qsv-deinterlace-multi": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			qsv(o)
			o.HWDeinterlace = true
			o.Profiles = goldenLadder[:2]
			o.VideoDecision = ""
		}),
		"hw-rkmpp-single": goldenOpts(rkmpp),
		"hw-rkmpp-multi":  goldenOpts(func(o *transcoding.HLSProcessOpts) { rkmpp(o); o.Profiles = goldenLadder[:3]; o.VideoDecision = "" }),
		"hw-rkmpp-hevc":   goldenOpts(func(o *transcoding.HLSProcessOpts) { rkmpp(o); o.VideoCodec = "hevc" }),
		"hw-rkmpp-opencl-tonemap": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			rkmpp(o)
			o.ToneMap = true
			o.HWTonemap = true
			o.HWFeatures = []string{transcoding.FeatureTonemapOpenCL}
		}),
		"hw-rkmpp-bridge-tonemap": goldenOpts(func(o *transcoding.HLSProcessOpts) { rkmpp(o); o.ToneMap = true }),
		"hw-rkmpp-decoupled": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			rkmpp(o)
			o.SourceVideoCodec = "vc1"
			o.HWSoftwareDecode = true
		}),
		"hw-rkmpp-overlay": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			rkmpp(o)
			o.HWOverlay = true
			o.SubtitleTrackIndex = 2
			o.SubtitleCodec = "hdmv_pgs_subtitle"
		}),
		"hw-qsv-decoupled": goldenOpts(func(o *transcoding.HLSProcessOpts) {
			qsv(o)
			o.SourceVideoCodec = "mpeg2video"
			o.HWSoftwareDecode = true
		}),
	}
}

func TestBuildFFmpegArgsGolden(t *testing.T) {
	// Two host-dependent inputs are pinned to their production values: the
	// bundled jellyfin-ffmpeg carries tonemapx, and the thread budget is a
	// cgroup fact. The golden describes the bytes on the box, not on this laptop.
	prevTonemapx := tonemapxAvailable("ffmpeg")
	prevThreads := softwareTranscodeThreads()
	tonemapxOK = true
	softwareThreadsValue = 8
	t.Cleanup(func() { tonemapxOK = prevTonemapx; softwareThreadsValue = prevThreads })

	sessions := goldenSessions()
	names := make([]string, 0, len(sessions))
	for n := range sessions {
		names = append(names, n)
	}
	sortStrings(names)
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "## %s\n%s\n\n", n, strings.Join(BuildFFmpegArgs(sessions[n]), " "))
	}
	got := b.String()

	path := filepath.Join("testdata", "args_golden.txt")
	if *updateArgsGolden {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		t.Logf("golden rewritten: %d sessions", len(names))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden file: run with -update once")
	if string(want) != got {
		wantBlocks := strings.Split(string(want), "\n\n")
		gotBlocks := strings.Split(got, "\n\n")
		var diff []string
		for i := range max(len(wantBlocks), len(gotBlocks)) {
			w, g := "", ""
			if i < len(wantBlocks) {
				w = wantBlocks[i]
			}
			if i < len(gotBlocks) {
				g = gotBlocks[i]
			}
			if w != g {
				diff = append(diff, "--- want\n"+w, "+++ got\n"+g)
			}
			if len(diff) >= 6 {
				break
			}
		}
		t.Fatalf("ffmpeg args golden drifted. First differences:\n%s", strings.Join(diff, "\n"))
	}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
