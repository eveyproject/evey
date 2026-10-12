package ffmpeg

// Real-ffmpeg proof that the server's stereo fold no longer clips. A 5.1 bed
// mastered at -1 dBFS per channel goes through the project's OWN argv - the
// audio-only HLS rendition and the compat download - and what comes out is
// decoded back to float and counted sample by sample. The same argv with the
// headroom option stripped is measured next to it, so the test carries its own
// before/after rather than a claim.
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/ffmpeg/ -run TestAudioDownmixHeadroomIntegration -v

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

type foldMeasure struct {
	samples int
	clipped int
	peakDB  float64
}

// measureFold decodes path to float and reports the true peak plus the number
// of samples whose absolute value reaches full scale - the ones a client's
// integer output stage hard-clips. astats' "Peak level" alone would not do:
// volumedetect buckets by dB and reports everything above -0.5 dBFS as "0 dB".
func measureFold(ctx context.Context, t *testing.T, path string) foldMeasure {
	t.Helper()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-nostdin",
		"-i", path, "-map", "0:a:0", "-c:a", "pcm_f32le", "-f", "f32le", "-")
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("decode %s: %v\n%s", path, err, errb.String())
	}
	raw := out.Bytes()
	m := foldMeasure{samples: len(raw) / 4, peakDB: math.Inf(-1)}
	peak := 0.0
	for i := 0; i+4 <= len(raw); i += 4 {
		v := math.Abs(float64(math.Float32frombits(binary.LittleEndian.Uint32(raw[i:]))))
		if v > peak {
			peak = v
		}
		if v >= 1.0 {
			m.clipped++
		}
	}
	if peak > 0 {
		m.peakDB = 20 * math.Log10(peak)
	}
	return m
}

// stripHeadroom removes the guard from an argv, reproducing the unguarded
// command line: swr's raw fold straight into the encoder.
func stripHeadroom(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "-af" || args[i] == "-rematrix_maxval" {
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}

// renderFold writes the fold guard's output as float PCM: the encoder input,
// before any codec overshoot.
func renderFold(ctx context.Context, t *testing.T, src, dst string) string {
	t.Helper()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-nostdin", "-y",
		"-i", src, "-map", "0:a:0", "-af", transcoding.AudioFoldFilter(0, 2), "-c:a", "pcm_f32le", dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("render fold: %v\n%s", err, out)
	}
	return dst
}

func TestAudioDownmixHeadroomIntegration(t *testing.T) {
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

	// A 5.1 bed the way films are mastered: every channel peaking at -1 dBFS,
	// music and effects strongly correlated between the fronts and the
	// surrounds, dialogue alone and 20 dB down in the centre. That last detail
	// is why the reported symptom is "voices fine, everything else tears": the
	// centre never sums with anything.
	src := filepath.Join(dir, "src.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "aevalsrc=0.62*sin(2*PI*220*t)+0.27*sin(2*PI*660*t)|"+
			"0.62*sin(2*PI*220*t)+0.27*sin(2*PI*659*t)|0.10*sin(2*PI*700*t)|"+
			"0.70*sin(2*PI*45*t)|0.85*sin(2*PI*221*t)|0.85*sin(2*PI*219*t):c=5.1:s=48000",
		"-t", "12",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "ac3", "-b:a", "640k",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("source generation: %v\n%s", err, out)
	}

	run := func(name string, args []string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
	}

	// concatenated fMP4: init segment + every media segment, in index order.
	concatRendition := func(outDir, dst string) string {
		t.Helper()
		segs, err := filepath.Glob(filepath.Join(outDir, "audio_0_*.m4s"))
		if err != nil || len(segs) == 0 {
			t.Fatalf("no audio segments in %s (%v)", outDir, err)
		}
		sort.Strings(segs)
		var buf bytes.Buffer
		init, err := os.ReadFile(filepath.Join(outDir, "audio_0_init.mp4"))
		if err != nil {
			t.Fatalf("init segment: %v", err)
		}
		buf.Write(init)
		for _, s := range segs {
			b, err := os.ReadFile(s)
			if err != nil {
				t.Fatalf("segment %s: %v", s, err)
			}
			buf.Write(b)
		}
		if err := os.WriteFile(dst, buf.Bytes(), 0o600); err != nil {
			t.Fatalf("write %s: %v", dst, err)
		}
		return dst
	}

	renditionOpts := func(outDir string, channels int) transcoding.AudioProcessOpts {
		return transcoding.AudioProcessOpts{
			FilePath: src, OutputDir: outDir, SourceTrackIdx: 0,
			AudioCodec: "aac", AudioBitrate: 128, MaxAudioChannels: channels,
		}
	}

	// --- audio-only HLS rendition, the streaming fold ---
	beforeDir := filepath.Join(dir, "rendition-before")
	afterDir := filepath.Join(dir, "rendition-after")
	for _, d := range []string{beforeDir, afterDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	afterArgs := BuildFFmpegAudioOnlyArgs(renditionOpts(afterDir, 2))
	run("rendition after", afterArgs)
	run("rendition before", stripHeadroom(BuildFFmpegAudioOnlyArgs(renditionOpts(beforeDir, 2))))

	before := measureFold(ctx, t, concatRendition(beforeDir, filepath.Join(dir, "before.mp4")))
	after := measureFold(ctx, t, concatRendition(afterDir, filepath.Join(dir, "after.mp4")))
	t.Logf("audio rendition 5.1->2.0  before: peak %+.2f dBFS, %d/%d clipped", before.peakDB, before.clipped, before.samples)
	t.Logf("audio rendition 5.1->2.0  after : peak %+.2f dBFS, %d/%d clipped", after.peakDB, after.clipped, after.samples)

	if before.clipped == 0 {
		t.Fatalf("the unprotected fold did not clip - the source no longer exercises the defect")
	}
	// What the guard controls is the encoder INPUT: the limiter holds the fold
	// under its ceiling whatever the bed sums to. The native aac encoder then
	// overshoots stacked pure tones by up to ~4 dB on decode (libfdk does not),
	// so the decoded streams are held to an order of magnitude, not to zero.
	pre := measureFold(ctx, t, renderFold(ctx, t, src, filepath.Join(dir, "pre.wav")))
	t.Logf("fold guard, encoder input       : peak %+.2f dBFS, %d/%d clipped", pre.peakDB, pre.clipped, pre.samples)
	if pre.clipped != 0 || pre.peakDB > 20*math.Log10(transcoding.DownmixLimiterCeiling)+0.05 {
		t.Errorf("the guarded fold exceeds its ceiling before the encoder: peak %+.2f dBFS, %d clipped", pre.peakDB, pre.clipped)
	}
	if after.clipped*10 >= before.clipped {
		t.Errorf("the guarded rendition still clips: %d samples at full scale vs %d unguarded", after.clipped, before.clipped)
	}

	// --- compat download, where the stereo fold is hardcoded and the result is
	// written to a file the user keeps ---
	dlBefore := filepath.Join(dir, "dl-before.mkv")
	dlAfter := filepath.Join(dir, "dl-after.mkv")
	run("compat after", BuildCompatArgs(src, dlAfter))
	run("compat before", stripHeadroom(BuildCompatArgs(src, dlBefore)))
	dlB := measureFold(ctx, t, dlBefore)
	dlA := measureFold(ctx, t, dlAfter)
	t.Logf("compat download 5.1->2.0  before: peak %+.2f dBFS, %d/%d clipped", dlB.peakDB, dlB.clipped, dlB.samples)
	t.Logf("compat download 5.1->2.0  after : peak %+.2f dBFS, %d/%d clipped", dlA.peakDB, dlA.clipped, dlA.samples)
	if dlB.clipped == 0 {
		t.Fatalf("the unprotected download fold did not clip - source no longer exercises the defect")
	}
	if dlA.clipped*10 >= dlB.clipped {
		t.Errorf("the guarded download fold still clips: %d samples at full scale vs %d unguarded", dlA.clipped, dlB.clipped)
	}

	// --- the nominal path: a 5.1 target keeps its layout, so nothing sums and
	// the rendition must come out untouched (and still 6 channels).
	sixDir := filepath.Join(dir, "rendition-5.1")
	if err := os.MkdirAll(sixDir, 0o750); err != nil {
		t.Fatal(err)
	}
	sixArgs := BuildFFmpegAudioOnlyArgs(renditionOpts(sixDir, 6))
	sixTrim := primingTrimFilter(primingOverflowSec(renditionOpts(sixDir, 6)))
	for i, a := range sixArgs {
		// The encoder-priming trim drops input, it does not touch the sound.
		if a == "-rematrix_maxval" || (a == "-af" && sixArgs[i+1] != sixTrim) {
			t.Fatalf("5.1 rendition must add neither a resampler option nor a filter: %v", sixArgs)
		}
	}
	run("rendition 5.1", sixArgs)
	six := concatRendition(sixDir, filepath.Join(dir, "six.mp4"))
	probe := exec.CommandContext(ctx, "ffprobe", "-hide_banner", "-v", "error",
		"-select_streams", "a:0", "-show_entries", "stream=channels", "-of", "csv=p=0", six)
	out, err := probe.Output()
	if err != nil {
		t.Fatalf("probe 5.1 rendition: %v", err)
	}
	if got := string(bytes.TrimSpace(out)); got != "6" {
		t.Errorf("5.1 rendition served %s channels, want 6", got)
	}
}
