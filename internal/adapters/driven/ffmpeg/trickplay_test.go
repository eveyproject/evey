package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeFakeProbeScript(t *testing.T, duration float64) string {
	t.Helper()
	body := fmt.Sprintf("#!/bin/sh\nprintf '{\"format\":{\"duration\":\"%g\"}}'\n", duration)
	return writeFakeBinary(t, filepath.Join(t.TempDir(), "ffprobe"), body)
}

// makeFakeFfmpegScript creates a fake ffmpeg that emulates the
// single-pass trickplay build: find the -y output pattern (containing
// "%03d") and touch trickplay_001.webp in its parent directory so the
// sprite-counting loop in Generate finds at least one file.
//
// Also emits stderr lines mimicking the showinfo filter so the timestamp
// parser has something to consume - without these, Generate fails on
// "no timestamps captured from showinfo".
func makeFakeFfmpegScript(t *testing.T) string {
	t.Helper()
	return writeFakeBinary(t, filepath.Join(t.TempDir(), "ffmpeg"), `#!/bin/sh
prev=""
for arg; do
  if [ "$prev" = "-y" ]; then
    outdir=$(dirname "$arg")
    mkdir -p "$outdir"
    touch "$outdir/trickplay_001.webp"
    >&2 echo "[Parsed_showinfo_2 @ 0xdead] n: 0 pts: 0 pts_time:0.000 ..."
    >&2 echo "[Parsed_showinfo_2 @ 0xdead] n: 1 pts: 100000 pts_time:10.000 ..."
    exit 0
  fi
  prev="$arg"
done
exit 0
`)
}

func TestTrickplayGenerator_Generate_Success(t *testing.T) {
	t.Parallel()
	probe := makeFakeProbeScript(t, 25.0) // 25s → timestamps 0,10,20 → 3 thumbs
	ffmpeg := makeFakeFfmpegScript(t)
	outputDir := t.TempDir()

	g := NewTrickplayGenerator(ffmpeg, probe)
	result, err := g.Generate(context.Background(), "/fake/video.mkv", outputDir, 10, 160, 5, 5, 1, 0)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, outputDir, result.ImageDir)
	assert.Equal(t, 1, result.ImageCount)
}

// While a playback is live the sprite ffmpeg is SIGSTOP-suspended; Generate
// only completes once the busy flag clears.
func TestTrickplayGenerator_Generate_SuspendedWhileBusy(t *testing.T) {
	old := detectionPausePoll
	detectionPausePoll = 10 * time.Millisecond
	defer func() { detectionPausePoll = old }()

	body := strings.Replace("#!/bin/sh\nsleep 0.2\nexec REAL \"$@\"\n", "REAL", makeFakeFfmpegScript(t), 1)
	script := writeFakeBinary(t, filepath.Join(t.TempDir(), "slow-ffmpeg.sh"), body)

	var busy atomic.Bool
	busy.Store(true)
	g := NewTrickplayGenerator(script, makeFakeProbeScript(t, 25)).WithBusyChecker(busy.Load)

	done := make(chan error, 1)
	go func() {
		_, err := g.Generate(context.Background(), "/fake/video.mkv", t.TempDir(), 10, 160, 5, 5, 1, 0)
		done <- err
	}()
	select {
	case <-done:
		t.Fatal("trickplay ffmpeg finished while a playback was live")
	case <-time.After(500 * time.Millisecond):
	}
	busy.Store(false)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("trickplay ffmpeg did not resume after the playback ended")
	}
}

func TestTrickplayGenerator_Generate_InvalidBinary(t *testing.T) {
	t.Parallel()
	probe := makeFakeProbeScript(t, 10.0)
	g := NewTrickplayGenerator("/nonexistent/ffmpeg-trickplay", probe)
	outputDir := t.TempDir()

	_, err := g.Generate(context.Background(), "/fake/video.mkv", outputDir, 10, 160, 5, 5, 1, 0)

	require.Error(t, err)
}

func TestTrickplayGenerator_Generate_InvalidProbe(t *testing.T) {
	t.Parallel()
	g := NewTrickplayGenerator("/nonexistent/ffmpeg", "/nonexistent/ffprobe")
	outputDir := t.TempDir()

	_, err := g.Generate(context.Background(), "/fake/video.mkv", outputDir, 10, 160, 5, 5, 1, 0)

	require.Error(t, err)
}

func TestTrickplayGenerator_Generate_CreatesOutputDir(t *testing.T) {
	t.Parallel()
	probe := makeFakeProbeScript(t, 10.0)
	ffmpeg := makeFakeFfmpegScript(t)
	base := t.TempDir()
	outputDir := filepath.Join(base, "nested", "trickplay")

	g := NewTrickplayGenerator(ffmpeg, probe)
	_, err := g.Generate(context.Background(), "/fake/video.mkv", outputDir, 10, 160, 5, 5, 1, 0)

	require.NoError(t, err)
	info, statErr := os.Stat(outputDir)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

// Trickplay thumbnails of an HDR / Dolby Vision source must be tone-mapped.
// The filter graph was select,scale,showinfo,tile - no colour handling at all -
// so a PQ (or DoVi P5, whose base layer is PQ-ish) source was sampled as if it
// were SDR: washed-out, dull, green/purple-tinted tiles. The player already
// tone-maps these files correctly; the scrubbing strip did not, which is why the
// thumbnails don't match the picture. Reported on a real DoVi P5 title.
func TestTrickplayFilter_TonemapsHDRSources(t *testing.T) {
	base := "select='isnan(prev_selected_t)+gte(t-prev_selected_t\\,10)',scale=320:-1,showinfo,tile=5x5"

	// SDR: untouched - no needless colour pass on the vast majority of files.
	if got := trickplayFilter(base, trickplayColour{}); got != base {
		t.Errorf("SDR filter must be unchanged:\n got %q\nwant %q", got, base)
	}

	for _, tc := range []struct {
		name string
		col  trickplayColour
	}{
		{"HDR10 (PQ)", trickplayColour{Transfer: "smpte2084"}},
		{"HLG", trickplayColour{Transfer: "arib-std-b67"}},
		// DoVi P5 tags NOTHING in the container (color_transfer=unknown): the
		// only signal is the DOVI configuration record. Missed → washed out.
		{"Dolby Vision P5, untagged container", trickplayColour{DoVi: true}},
	} {
		got := trickplayFilter(base, tc.col)
		if !strings.Contains(got, "tonemap") {
			t.Errorf("%s: filter must tone-map, got %q", tc.name, got)
		}
		// setparams only exists on the tonemapx build (jellyfin-ffmpeg); the
		// portable zscale fallback tags colour differently. Only require it
		// when this host's ffmpeg actually has the filter.
		if tonemapxAvailable("ffmpeg") && !strings.Contains(got, "setparams") {
			t.Errorf("%s: filter must tag the colour metadata first (rips under-tag), got %q", tc.name, got)
		}
		if !strings.Contains(got, "tile=5x5") {
			t.Errorf("%s: the tiling must survive, got %q", tc.name, got)
		}
	}
}

// snapshotFakes are an ffmpeg that records its argv and an ffprobe that
// reports the given transfer curve for the first video stream.
func snapshotFakes(t *testing.T, transfer string) (bin, probe, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args")
	bin = writeFakeBinary(t, filepath.Join(dir, "ffmpeg"), "#!/bin/sh\ncat > /dev/null\nprintf '%s\\n' \"$@\" > "+argsFile+"\nprintf 'jpeg'\n")
	probe = writeFakeBinary(t, filepath.Join(dir, "ffprobe"),
		fmt.Sprintf("#!/bin/sh\nprintf '{\"streams\":[{\"color_transfer\":\"%s\"}],\"format\":{\"start_time\":\"0.000\"}}'\n", transfer))
	return bin, probe, argsFile
}

func recordedVF(t *testing.T, argsFile string) string {
	t.Helper()
	raw, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	args := strings.Split(strings.TrimSpace(string(raw)), "\n")
	for i, a := range args {
		if a == "-vf" && i+1 < len(args) {
			return args[i+1]
		}
	}
	t.Fatalf("no -vf in %v", args)
	return ""
}

// A snapshot of an HDR source is a small SDR picture like the trickplay strip
// and the player: sampled as if it were SDR it came out grey and washed out.
// The exact decode and the keyframe-packet decode both tone-map; an SDR
// source keeps the bare scale, byte for byte.
func TestSnapshot_TonemapsAnHDRSource(t *testing.T) {
	bin, probe, argsFile := snapshotFakes(t, "smpte2084")
	g := NewTrickplayGenerator(bin, probe)

	out, err := g.ExtractFrame(context.Background(), "/fake/hdr.mkv", 12.5, 640)
	require.NoError(t, err)
	assert.Equal(t, "jpeg", string(out))
	vf := recordedVF(t, argsFile)
	assert.True(t, strings.HasPrefix(vf, "scale=640:-1,"), "the scale leads: %q", vf)
	assert.Contains(t, vf, "tonemap", "the exact decode tone-maps: %q", vf)

	_, err = g.DecodePacketToJPEG(context.Background(), "/fake/hdr.mkv", []byte{0, 0, 0, 1}, ".hevc", 640)
	require.NoError(t, err)
	vf = recordedVF(t, argsFile)
	assert.Contains(t, vf, "tonemap", "the keyframe-packet decode tone-maps: %q", vf)

	sdrBin, sdrProbe, sdrArgs := snapshotFakes(t, "bt709")
	g = NewTrickplayGenerator(sdrBin, sdrProbe)
	_, err = g.ExtractFrame(context.Background(), "/fake/sdr.mkv", 12.5, 640)
	require.NoError(t, err)
	assert.Equal(t, "scale=640:-1", recordedVF(t, sdrArgs), "an SDR source keeps the bare scale")
}

// The colour of a source is read once per file, not once per snapshot: the
// scrubbing preview asks for many frames of the same file in a row, and a
// probe per frame would cost more than the frame on the cheap path.
func TestSnapshot_ProbesTheColourOncePerFile(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	countFile := filepath.Join(dir, "probes")
	bin := writeFakeBinary(t, filepath.Join(dir, "ffmpeg"), "#!/bin/sh\ncat > /dev/null\nprintf '%s\\n' \"$@\" > "+argsFile+"\nprintf 'jpeg'\n")
	probe := writeFakeBinary(t, filepath.Join(dir, "ffprobe"),
		"#!/bin/sh\necho probe >> "+countFile+"\nprintf '{\"streams\":[{\"color_transfer\":\"smpte2084\"}],\"format\":{\"start_time\":\"0.000\"}}'\n")
	probes := func() int {
		raw, _ := os.ReadFile(countFile)
		return strings.Count(string(raw), "probe")
	}
	src := filepath.Join(dir, "hdr.mkv")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o644))
	other := filepath.Join(dir, "other.mkv")
	require.NoError(t, os.WriteFile(other, []byte("y"), 0o644))
	g := NewTrickplayGenerator(bin, probe)
	ctx := context.Background()

	_, err := g.ExtractFrame(ctx, src, 1, 640)
	require.NoError(t, err)
	_, err = g.DecodePacketToJPEG(ctx, src, []byte{0, 0, 0, 1}, ".hevc", 640)
	require.NoError(t, err)
	_, err = g.ExtractFrame(ctx, src, 2, 640)
	require.NoError(t, err)
	assert.Equal(t, 1, probes(), "one probe for three snapshots of the same file")
	assert.Contains(t, recordedVF(t, argsFile), "tonemap", "the remembered colour still tone-maps")

	_, err = g.ExtractFrame(ctx, other, 1, 640)
	require.NoError(t, err)
	assert.Equal(t, 2, probes(), "another file is probed once")

	// A rewritten file (new modification time) is read again.
	later := time.Now().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(src, later, later))
	_, err = g.ExtractFrame(ctx, src, 3, 640)
	require.NoError(t, err)
	assert.Equal(t, 3, probes(), "a changed file is probed again")
}
