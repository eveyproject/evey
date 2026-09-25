package ffmpeg

// The codecs a manifest declares for a transcoded rung are the ones its
// init segment holds, at the level its parameters need and its stream meets: the HLS master's CODECS and the DASH manifest's codecs
// are held, byte for byte, against the RFC 6381 string read back from the
// avcC / hvcC the real encoder wrote - software and, where the host has one,
// NVENC. Before the shared contract every one of these differed (x264 High
// 4.1 and NVENC Main 3.0 under a declared High 4.0; HEVC 3.0/4.0 under a
// declared 3.1/4.1).
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/ffmpeg/ -run TestCodecStringIntegration -v

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"math/bits"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// initCodecString is the RFC 6381 codecs value of the video sample entry in
// an fMP4 init: avc1.PPCCLL from the avcC, hvc1.P.C.TL.CC from the hvcC.
func initCodecString(t *testing.T, init []byte) string {
	t.Helper()
	if i := bytes.Index(init, []byte("avcC")); i > 0 {
		return fmt.Sprintf("avc1.%02x%02x%02x", init[i+5], init[i+6], init[i+7])
	}
	i := bytes.Index(init, []byte("hvcC"))
	if i < 0 {
		t.Fatal("no avcC or hvcC in the init")
	}
	c := init[i+4:]
	profileIdc := c[1] & 0x1f
	tier := "L"
	if c[1]&0x20 != 0 {
		tier = "H"
	}
	compat := bits.Reverse32(uint32(c[2])<<24 | uint32(c[3])<<16 | uint32(c[4])<<8 | uint32(c[5]))
	cons := c[6:12]
	n := len(cons)
	for n > 0 && cons[n-1] == 0 {
		n--
	}
	var cs []string
	for _, b := range cons[:n] {
		cs = append(cs, fmt.Sprintf("%X", b))
	}
	s := fmt.Sprintf("hvc1.%d.%X.%s%d", profileIdc, compat, tier, c[12])
	if len(cs) > 0 {
		s += "." + strings.Join(cs, ".")
	}
	return s
}

func TestCodecStringIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest` - needs the bundled ffmpeg)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	dir := t.TempDir()
	// Noisy sources: a rung only meets its rate ceiling (and the level's
	// MaxBR only matters) when the picture costs bits.
	source := func(w, h int, rate string) string {
		t.Helper()
		src := filepath.Join(dir, fmt.Sprintf("src-%dx%d-%s.mkv", w, h, strings.ReplaceAll(rate, "/", "_")))
		if _, err := os.Stat(src); err == nil {
			return src
		}
		if out, err := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y", "-f", "lavfi", "-t", "2",
			"-i", fmt.Sprintf("testsrc2=size=%dx%d:rate=%s,noise=alls=60:allf=t", w, h, rate),
			"-c:v", "libx264", "-preset", "ultrafast", "-qp", "0", "-pix_fmt", "yuv420p", src).CombinedOutput(); err != nil {
			t.Fatalf("source: %v\n%s", err, out)
		}
		return src
	}
	accels := []transcoding.HWAccelType{""}
	if exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-f", "lavfi", "-i", "color=size=256x256:duration=0.1", "-c:v", "h264_nvenc", "-f", "null", "-").Run() == nil {
		accels = append(accels, transcoding.NVENC)
	} else {
		t.Log("no NVENC on this host: software only")
	}
	masterCodecs := regexp.MustCompile(`CODECS="([^",]+)`)
	mpdCodecs := regexp.MustCompile(`contentType="video"[\s\S]*?codecs="([^"]+)"`)
	for _, accel := range accels {
		for _, tc := range []struct {
			name   string
			codec  string
			w, h   int
			rate   string
			fps    float64
			kbps   int64
			wantLv int // the level the tables give for this rung on this route
		}{
			{"720p60", "h264", 1280, 720, "60", 60, 2800, 32},
			{"1080p60", "h264", 1920, 1080, "60", 60, 5000, 42},
			{"4K24", "h264", 3840, 2160, "24000/1001", 23.976, 16000, 51},
			{"1080p24-boosted-15M", "h264", 1920, 1080, "24000/1001", 23.976, 15000, 0},
			{"720p60", "hevc", 1280, 720, "60", 60, 1867, 120},
			{"1080p60", "hevc", 1920, 1080, "60", 60, 3333, 123},
			{"4K24", "hevc", 3840, 2160, "24000/1001", 23.976, 10667, 153},
			{"1080p24-boosted-15M", "hevc", 1920, 1080, "24000/1001", 23.976, 15000, 0},
		} {
			route := map[bool]string{true: "software", false: string(accel)}[accel == ""]
			t.Run(route+"/"+tc.codec+"/"+tc.name, func(t *testing.T) {
				grid := transcoding.GridForFrameRate(tc.fps)
				hw := accel != ""
				qt := transcoding.StaticQualityPolicy{}.TargetFor(transcoding.HLSProfile{VideoBitrate: tc.kbps}, transcoding.EncodeRoute{HWUsable: hw})
				p := transcoding.HLSProfile{Name: "r", Width: tc.w, Height: tc.h, VideoBitrate: tc.kbps, AudioBitrate: 128}
				p.CodecLevel = transcoding.CodecLevel(tc.codec, p, tc.fps, qt)
				if tc.wantLv > 0 && !hw && p.CodecLevel != tc.wantLv {
					t.Fatalf("the tables give level %d, want %d", p.CodecLevel, tc.wantLv)
				}
				o := transcoding.HLSProcessOpts{FilePath: source(tc.w, tc.h, tc.rate), OutputDir: t.TempDir(), VideoDecision: "transcode",
					AudioTrackIndex: -1, SubtitleTrackIndex: -1, UseFMP4: true, VideoCodec: tc.codec, HWAccel: accel, Grid: grid,
					Profiles: []transcoding.HLSProfile{p}}
				args := BuildFFmpegArgs(o)
				if out, err := exec.CommandContext(ctx, "ffmpeg", append([]string{"-hide_banner", "-v", "error", "-y"}, args...)...).CombinedOutput(); err != nil {
					t.Fatalf("encode: %v\n%s\n%s", err, out, strings.Join(args, " "))
				}
				init, err := os.ReadFile(filepath.Join(o.OutputDir, "stream_0_init.mp4"))
				if err != nil {
					t.Fatal(err)
				}
				got := initCodecString(t, init)

				// 1. The init, the HLS master and the DASH manifest: one level.
				in := apptranscoding.ManifestInput{
					MediaID: "m", DurationSec: 2, Grid: grid, Profiles: []transcoding.HLSProfile{p}, StreamToken: "t", UseFMP4: true,
					ChosenVideoCodec: tc.codec, ChosenAudioCodec: "aac", SubtitleBurnInTrack: -1,
					AudioRenditions: []apptranscoding.AudioRendition{{SourceTrackIdx: 0, Codec: "aac", SampleRate: 48000, Channels: 2}},
				}
				gen := apptranscoding.NewManifestGenerator()
				master, err := gen.Master(in)
				if err != nil {
					t.Fatal(err)
				}
				mpd, err := gen.MPD(in)
				if err != nil {
					t.Fatal(err)
				}
				hls, dash := masterCodecs.FindStringSubmatch(master), mpdCodecs.FindStringSubmatch(mpd)
				if hls == nil || dash == nil || !strings.EqualFold(hls[1], got) || !strings.EqualFold(dash[1], got) {
					t.Fatalf("the init holds %s, the HLS master declares %v, the MPD %v", got, hls, dash)
				}

				// 2. The stream conforms to that level: reference frames within its
				// DPB, and no second of it above its MaxBR.
				joined := filepath.Join(o.OutputDir, "joined.mp4")
				var all []byte
				all = append(all, init...)
				segs, _ := filepath.Glob(filepath.Join(o.OutputDir, "stream_0_[0-9]*.m4s"))
				for _, sg := range segs {
					b, _ := os.ReadFile(sg)
					all = append(all, b...)
				}
				if werr := os.WriteFile(joined, all, 0o644); werr != nil {
					t.Fatal(werr)
				}
				var maxKbps int64
				if tc.codec == "h264" {
					maxKbps, _ = transcoding.H264LevelLimits(p.CodecLevel)
					refs, _ := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=refs", "-of", "csv=p=0", joined).Output()
					var n int
					fmt.Sscan(strings.TrimSpace(string(refs)), &n)
					if dpb := transcoding.H264MaxDpbFrames(p.CodecLevel, tc.w, tc.h); n > dpb {
						t.Errorf("%d reference frames, level %d holds %d at %dx%d", n, p.CodecLevel, dpb, tc.w, tc.h)
					}
				} else {
					maxKbps, _ = transcoding.HEVCLevelLimits(p.CodecLevel)
				}
				sizes, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", "packet=size", "-of", "csv=p=0", joined).Output()
				if err != nil {
					t.Fatal(err)
				}
				var frames []int64
				for l := range strings.FieldsSeq(string(sizes)) {
					var n int64
					fmt.Sscan(l, &n)
					frames = append(frames, n)
				}
				// The level's hypothetical reference decoder: a buffer of MaxCPB
				// filled at MaxBR - over any run of frames, the bits may not
				// exceed MaxBR x its duration plus the buffer (the HRD bound, not
				// a per-second average).
				var cpbKbit int64
				if tc.codec == "h264" {
					_, cpbKbit = transcoding.H264LevelLimits(p.CodecLevel)
				} else {
					_, cpbKbit = transcoding.HEVCLevelLimits(p.CodecLevel)
				}
				worst := 0.0
				for i := range frames {
					var sum int64
					for j := i; j < len(frames); j++ {
						sum += frames[j]
						allowed := float64(maxKbps)*float64(j-i+1)/tc.fps + float64(cpbKbit)
						worst = math.Max(worst, float64(sum*8)/1000/allowed)
					}
				}
				if worst > 1 {
					t.Errorf("a run of frames uses %.0f%% of what level %d's decoder buffer holds", worst*100, p.CodecLevel)
				}
				window := int(math.Round(tc.fps))
				peak := int64(0)
				for i := 0; i+window <= len(frames); i++ {
					var sum int64
					for _, n := range frames[i : i+window] {
						sum += n
					}
					peak = max(peak, sum*8/1000)
				}
				t.Logf("%s level %d: 1 s peak %d kbit (MaxBR %d, CPB %d), worst HRD run %.0f%%", got, p.CodecLevel, peak, maxKbps, cpbKbit, worst*100)
			})
		}
	}
}
