package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/library"
)

// fakeKeyframeStreamer writes payload to the pipe and reports stats.
type fakeKeyframeStreamer struct {
	stats   KeyframeStreamStats
	err     error
	payload []byte
	calls   int
	writeEr error // the error the pipe write returned
	missing bool  // CuesMissKeyframes' answer
	holeErr error
}

func (f *fakeKeyframeStreamer) CuesMissKeyframes(context.Context, string) (bool, error) {
	return f.missing, f.holeErr
}

func (f *fakeKeyframeStreamer) WriteKeyframeStream(_ context.Context, _ string, _ int64, w io.Writer) (KeyframeStreamStats, error) {
	f.calls++
	if f.err != nil {
		return KeyframeStreamStats{}, f.err
	}
	_, f.writeEr = w.Write(f.payload)
	if f.writeEr != nil {
		return KeyframeStreamStats{}, f.writeEr
	}
	return f.stats, nil
}

// makeTwoPathFfmpeg fakes ffmpeg for both sprite paths and appends one line per
// call ("keyframes" or "scan") to the returned log. The keyframe call drains
// stdin (or exits 1 without reading when keyExit is non-zero) and shows keyMs;
// the scan call shows scanMs.
func makeTwoPathFfmpeg(t *testing.T, keyMs, scanMs []int64, keyExit int) (bin, calls string) {
	t.Helper()
	dir := t.TempDir()
	calls = filepath.Join(dir, "calls.log")
	show := func(ms []int64) string {
		var b strings.Builder
		for i, v := range ms {
			fmt.Fprintf(&b, ">&2 echo '[Parsed_showinfo_1 @ 0x1] n: %d pts: %d pts_time:%.3f'\n", i, v, float64(v)/1000)
		}
		return b.String()
	}
	body := fmt.Sprintf(`#!/bin/sh
mode=scan
case " $* " in *" pipe:0 "*) mode=keyframes ;; esac
echo "$mode" >> %q
for last; do :; done
if [ $mode = keyframes ]; then
  [ %d -ne 0 ] && exit %d
  cat > /dev/null
  mkdir -p "$(dirname "$last")"; touch "$(dirname "$last")/trickplay_001.webp"
%s  exit 0
fi
mkdir -p "$(dirname "$last")"; touch "$(dirname "$last")/trickplay_001.webp"
%sexit 0
`, calls, keyExit, keyExit, show(keyMs), show(scanMs))
	bin = filepath.Join(dir, "ffmpeg")
	require.NoError(t, os.WriteFile(bin, []byte(body), 0o755))
	return bin, calls
}

func readCalls(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.Fields(string(b))
}

func goodStats(ms ...int64) KeyframeStreamStats {
	return KeyframeStreamStats{Samples: len(ms), PtsMs: ms, MedianCueGapMs: 1200}
}

func TestTrickplayKeyframes_UsedForMatroska(t *testing.T) {
	t.Parallel()
	bin, calls := makeTwoPathFfmpeg(t, []int64{0, 7008}, []int64{0, 8000}, 0)
	kf := &fakeKeyframeStreamer{stats: goodStats(0, 7008), payload: []byte("mkv")}
	g := NewTrickplayGenerator(bin, makeFakeProbeScript(t, 25)).WithKeyframeStreamer(kf)

	res, err := g.Generate(context.Background(), "/fake/video.mkv", t.TempDir(), 7, 320, 5, 5, 1, 25000)

	require.NoError(t, err)
	assert.Equal(t, []string{"keyframes"}, readCalls(t, calls))
	// The times are mkvgo's (exact source PTS), checked against showinfo.
	assert.Equal(t, []int64{0, 7008}, res.FrameTimestampsMs)
	assert.Equal(t, 1, res.ImageCount)
}

func TestTrickplayKeyframes_FallsBackToFullScan(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		kf      *fakeKeyframeStreamer
		keyMs   []int64
		keyExit int
	}{
		{"stream error", &fakeKeyframeStreamer{err: errors.New("no keyframe index")}, nil, 0},
		{"ffmpeg dropped a frame", &fakeKeyframeStreamer{stats: goodStats(0, 7008)}, []int64{0}, 0},
		{"ffmpeg shifted a frame", &fakeKeyframeStreamer{stats: goodStats(0, 7008)}, []int64{0, 7050}, 0},
		{"sparse cues hiding keyframes", &fakeKeyframeStreamer{stats: KeyframeStreamStats{Samples: 1, PtsMs: []int64{0}, MedianCueGapMs: 30000}, missing: true}, []int64{0}, 0},
		{"sparse cues, holes not probed", &fakeKeyframeStreamer{stats: KeyframeStreamStats{Samples: 1, PtsMs: []int64{0}, MedianCueGapMs: 30000}, holeErr: errors.New("no index")}, []int64{0}, 0},
		{"ffmpeg failed", &fakeKeyframeStreamer{stats: goodStats(0)}, []int64{0}, 1},
		// ffmpeg exits without reading a payload larger than a pipe buffer: the
		// writer must be released, not left blocked.
		{"ffmpeg died before reading", &fakeKeyframeStreamer{stats: goodStats(0), payload: make([]byte, 4<<20)}, nil, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bin, calls := makeTwoPathFfmpeg(t, tc.keyMs, []int64{0, 8000}, tc.keyExit)
			g := NewTrickplayGenerator(bin, makeFakeProbeScript(t, 25)).WithKeyframeStreamer(tc.kf)

			res, err := g.Generate(context.Background(), "/fake/video.mkv", t.TempDir(), 7, 320, 5, 5, 1, 25000)

			require.NoError(t, err)
			assert.Equal(t, []int64{0, 8000}, res.FrameTimestampsMs, "the full scan's times")
			got := readCalls(t, calls)
			assert.Equal(t, "scan", got[len(got)-1])
		})
	}
}

// A file without Cues is refused before any byte: ffmpeg is not started, so
// no failed fork is logged and the stream's reason is the one returned.
func TestTrickplayKeyframes_RefusedStreamStartsNoFfmpeg(t *testing.T) {
	t.Parallel()
	bin, calls := makeTwoPathFfmpeg(t, nil, nil, 183)
	kf := &fakeKeyframeStreamer{err: fmt.Errorf("%w: no Cues", ErrKeyframesUnavailable)}
	g := NewTrickplayGenerator(bin, makeFakeProbeScript(t, 25)).WithKeyframeStreamer(kf)

	_, err := g.generateFromKeyframes(context.Background(), "/fake/video.mkv", t.TempDir(), 7, "showinfo", 1, 0)

	require.ErrorIs(t, err, ErrKeyframesUnavailable)
	_, statErr := os.Stat(calls)
	assert.True(t, os.IsNotExist(statErr), "ffmpeg must not run")
}

func TestTrickplayKeyframes_NotForOtherContainers(t *testing.T) {
	t.Parallel()
	bin, calls := makeTwoPathFfmpeg(t, nil, []int64{0, 8000}, 0)
	kf := &fakeKeyframeStreamer{stats: goodStats(0)}
	g := NewTrickplayGenerator(bin, makeFakeProbeScript(t, 25)).WithKeyframeStreamer(kf)

	_, err := g.Generate(context.Background(), "/fake/video.avi", t.TempDir(), 7, 320, 5, 5, 1, 25000)

	require.NoError(t, err)
	assert.Zero(t, kf.calls)
	assert.Equal(t, []string{"scan"}, readCalls(t, calls))
}

// A sprite left by an earlier, longer run is not counted as ours.
func TestTrickplayKeyframes_ClearsStaleSprites(t *testing.T) {
	t.Parallel()
	bin, _ := makeTwoPathFfmpeg(t, []int64{0}, nil, 0)
	out := t.TempDir()
	for _, n := range []string{"trickplay_002.webp", "trickplay_003.webp"} {
		require.NoError(t, os.WriteFile(filepath.Join(out, n), nil, 0o644))
	}
	g := NewTrickplayGenerator(bin, makeFakeProbeScript(t, 25)).
		WithKeyframeStreamer(&fakeKeyframeStreamer{stats: goodStats(0)})

	res, err := g.Generate(context.Background(), "/fake/video.mkv", out, 7, 320, 5, 5, 1, 25000)

	require.NoError(t, err)
	assert.Equal(t, 1, res.ImageCount)
	assert.NoFileExists(t, filepath.Join(out, "trickplay_002.webp"))
}

func TestSpriteFilterTail_SharedByBothPaths(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "scale=320:-1,showinfo,tile=5x5", spriteFilterTail(320, 5, 5, trickplayColour{}))
	hdr := spriteFilterTail(320, 5, 5, trickplayColour{Transfer: "smpte2084"})
	assert.True(t, strings.HasPrefix(hdr, "scale=320:-1,showinfo,tile=5x5,"), hdr)
}

// pts_time:8.008 is 8007.999... as a float: truncating it put every such
// thumbnail 1 ms early in trickplay.json.
func TestParseShowinfoTimestamps_Rounds(t *testing.T) {
	t.Parallel()
	got := parseShowinfoTimestamps("n: 0 pts: 8008 pts_time:8.008\nn: 1 pts: 16016 pts_time:16.016\nn: 2 pts: 0 pts_time:0")
	assert.Equal(t, []int64{8008, 16016, 0}, got)
}

// The keyframe path keeps the source times (-copyts): a file whose first
// keyframe is not at 0 must not see every thumbnail shifted.
func TestTrickplayKeyframes_KeepsSourceTimes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	args := filepath.Join(dir, "args")
	bin := filepath.Join(dir, "ffmpeg")
	body := "#!/bin/sh\necho \"$@\" > " + args + "\ncat > /dev/null\nfor last; do :; done\ntouch \"$(dirname \"$last\")/trickplay_001.webp\"\n>&2 echo 'n: 0 pts: 5 pts_time:0.005'\n"
	require.NoError(t, os.WriteFile(bin, []byte(body), 0o755))
	g := NewTrickplayGenerator(bin, makeFakeProbeScript(t, 25)).
		WithKeyframeStreamer(&fakeKeyframeStreamer{stats: goodStats(5)})

	res, err := g.Generate(context.Background(), "/fake/video.mkv", t.TempDir(), 7, 320, 5, 5, 1, 25000)

	require.NoError(t, err)
	assert.Equal(t, []int64{5}, res.FrameTimestampsMs)
	b, err := os.ReadFile(args)
	require.NoError(t, err)
	assert.Contains(t, string(b), "-copyts -f matroska -i pipe:0")
}

// While a playback is live the keyframe-path ffmpeg is suspended like the scan's;
// the stream writer just waits on the pipe, and the result is still the
// keyframe one once the playback ends.
func TestTrickplayKeyframes_SuspendedWhileBusy(t *testing.T) {
	old := detectionPausePoll
	detectionPausePoll = 10 * time.Millisecond
	defer func() { detectionPausePoll = old }()

	inner, calls := makeTwoPathFfmpeg(t, []int64{0, 7008}, []int64{0, 8000}, 0)
	slow := filepath.Join(t.TempDir(), "slow-ffmpeg.sh")
	require.NoError(t, os.WriteFile(slow, []byte("#!/bin/sh\nsleep 0.2\nexec "+inner+" \"$@\"\n"), 0o755))
	var busy atomic.Bool
	busy.Store(true)
	kf := &fakeKeyframeStreamer{stats: goodStats(0, 7008), payload: make([]byte, 1<<20)}
	g := NewTrickplayGenerator(slow, makeFakeProbeScript(t, 25)).WithBusyChecker(busy.Load).WithKeyframeStreamer(kf)

	done := make(chan error, 1)
	var res *library.TrickplayResult
	go func() {
		var err error
		res, err = g.Generate(context.Background(), "/fake/video.mkv", t.TempDir(), 7, 320, 5, 5, 1, 25000)
		done <- err
	}()
	select {
	case <-done:
		t.Fatal("keyframe-path ffmpeg finished while a playback was live")
	case <-time.After(500 * time.Millisecond):
	}
	busy.Store(false)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("keyframe-path ffmpeg did not resume after the playback ended")
	}
	assert.Equal(t, []int64{0, 7008}, res.FrameTimestampsMs)
	assert.Equal(t, []string{"keyframes"}, readCalls(t, calls))
}

// Cues as sparse as the keyframes themselves (a long-GOP encode): the scan
// would see the same keyframes, so the keyframe strip stands.
func TestTrickplayKeyframes_SparseKeyframesKeepTheStream(t *testing.T) {
	t.Parallel()
	bin, calls := makeTwoPathFfmpeg(t, []int64{0, 10400}, []int64{0, 10400}, 0)
	kf := &fakeKeyframeStreamer{stats: KeyframeStreamStats{Samples: 2, PtsMs: []int64{0, 10400}, MedianCueGapMs: 10400}}
	g := NewTrickplayGenerator(bin, makeFakeProbeScript(t, 25)).WithKeyframeStreamer(kf)

	_, err := g.Generate(context.Background(), "/fake/video.mkv", t.TempDir(), 7, 320, 5, 5, 1, 25000)

	require.NoError(t, err)
	assert.Equal(t, []string{"keyframes"}, readCalls(t, calls))
}

// A source starting at 83 ms (B-frame MP4, a Matroska at 5 ms): the stream
// carries source times, the sidecar the player's timeline, which starts at the
// container's start_time - the same times the scan writes.
func TestTrickplayKeyframes_RebasedOnStartTime(t *testing.T) {
	t.Parallel()
	probe := filepath.Join(t.TempDir(), "ffprobe")
	require.NoError(t, os.WriteFile(probe, []byte("#!/bin/sh\nprintf '{\"streams\":[{}],\"format\":{\"start_time\":\"0.083000\"}}'\n"), 0o755))
	bin, _ := makeTwoPathFfmpeg(t, []int64{83, 8091}, nil, 0)
	g := NewTrickplayGenerator(bin, probe).
		WithKeyframeStreamer(&fakeKeyframeStreamer{stats: goodStats(83, 8091)})

	res, err := g.Generate(context.Background(), "/fake/video.mp4", t.TempDir(), 7, 320, 5, 5, 1, 25000)

	require.NoError(t, err)
	assert.Equal(t, []int64{0, 8008}, res.FrameTimestampsMs)
}
