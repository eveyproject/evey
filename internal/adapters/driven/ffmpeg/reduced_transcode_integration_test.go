package ffmpeg

// Real-ffmpeg + real-mkvgo proof of the whole reduced-download architecture,
// end to end: a too-big source goes in, slices come out of ONE encoder, and
// mkvgo's ops.Join - the very call the phone will make - puts them back
// together into a file that is the right length, the right size, and still in
// sync.
//
// This is the test the design lives or dies by. The alternative architecture
// (one ffmpeg run per slice) also produces slices; what it cannot produce is a
// join that ops.Join accepts (identical CodecPrivate) and audio that has not
// drifted from the picture by a whole AAC frame per boundary. Both are checked
// below, so a future "simplification" back to per-slice runs fails here rather
// than in the field on a two-hour film.
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/ffmpeg/ -run TestReducedSlicesIntegration -v

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	mkvops "github.com/gravity-zero/mkvgo/mkv/ops"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/reduceddl"
)

func TestReducedSlicesIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest` - needs the bundled ffmpeg)")
	}
	for _, bin := range []string{"ffmpeg", "ffprobe", "nice"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not in PATH", bin)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	dir := t.TempDir()

	// Source: 25 s of 1280x800 (taller than the 720 target, so a reduced
	// variant is warranted) with stereo audio.
	const srcDurationSec = 25
	src := filepath.Join(dir, "src.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=1280x800:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", strconv.Itoa(srcDurationSec),
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-ac", "2",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("source generation: %v\n%s", err, out)
	}

	// 10 s slices: 25 s of film means three of them, the last one short. A
	// short tail slice is the normal case, not an edge case.
	spec := reduceddl.Spec{Width: reduceddl.WidthFor(1280, 800), SliceSec: 10}
	if spec.Width != 1152 {
		t.Fatalf("delivery width = %d, want 1152 (1280x800 at height 720)", spec.Width)
	}
	jobDir := filepath.Join(dir, "job")

	var lastPermille int
	if err := NewReducedTranscoder("ffmpeg").TranscodeReducedSlices(
		ctx, src, jobDir, spec, transcoding.PicturePlan{}, srcDurationSec*1000, 0, func(p int) { lastPermille = p },
	); err != nil {
		t.Fatalf("TranscodeReducedSlices: %v", err)
	}
	// A production that takes an hour and reports a static 0 reads as a hang -
	// that exact failure already happened once on the compat path.
	if lastPermille < 500 {
		t.Errorf("final progress permille = %d, want >= 500", lastPermille)
	}

	// The list file is the readiness signal the handler serves on, so it has
	// to agree with what is actually on disk.
	listPath := filepath.Join(jobDir, reduceddl.ListName(0))
	list, err := os.Open(listPath)
	if err != nil {
		t.Fatalf("open the slice list: %v", err)
	}
	closed, endSec := reduceddl.ClosedSlices(list)
	_ = list.Close()
	// 25 s cut every 10 s: two full slices and a short tail. Fewer than three
	// means the keyframe forcing is not landing on the grid and the muxer is
	// swallowing the rest of the film into one slice.
	if closed != 3 {
		t.Fatalf("the list reports %d closed slices, want 3 (10 s + 10 s + 5 s)", closed)
	}
	// The recorded end of the last slice is what a resume seeks to; a value
	// that does not reach the end of the film would silently truncate it.
	if math.Abs(endSec-srcDurationSec) > 1.0 {
		t.Errorf("the list ends at %.3f s, want ~%d s", endSec, srcDurationSec)
	}

	var slices []string
	for i := range closed {
		p := filepath.Join(jobDir, reduceddl.SliceName(i))
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("slice %d is in the list but not on disk: %v", i, err)
		}
		slices = append(slices, p)
	}
	t.Logf("produced %d slices", len(slices))

	// THE test: mkvgo joins them, exactly as the phone will. ops.Join refuses
	// a CodecPrivate mismatch outright, so this failing is the single-encoder
	// invariant breaking.
	joined := filepath.Join(dir, "joined.mkv")
	if err := mkvops.Join(ctx, slices, joined); err != nil {
		t.Fatalf("mkvgo ops.Join over the slices: %v", err)
	}

	// Length, measured on the CONTENT rather than the header: the last packet
	// of each track must land at the end of the film. This is the assertion
	// that says the join reconstructed all 25 s and not one slice of it.
	vEnd := lastPacketTime(t, ctx, joined, "v:0")
	aEnd := lastPacketTime(t, ctx, joined, "a:0")
	if math.Abs(vEnd-srcDurationSec) > 1.0 {
		t.Errorf("joined video ends at %.3f s, want ~%d s", vEnd, srcDurationSec)
	}

	// A/V sync across the joins. ops.Join appends each track against its OWN
	// end, so per-slice rounding would show up here as the audio and the
	// picture ending at different times - the exact failure that rules out one
	// ffmpeg run per slice. One continuous encoder makes the packets
	// contiguous across a cut, so this must stay tight.
	if skew := math.Abs(vEnd - aEnd); skew > 0.1 {
		t.Errorf("audio ends %.3f s away from the picture after %d joins - A/V drift across slice boundaries (video %.3f, audio %.3f)",
			skew, len(slices)-1, vEnd, aEnd)
	}

	// The DECLARED duration: a v0.25.1 Join announced the length of slice 0
	// (12 s for this 25 s film - players believe the header and refuse to
	// seek past it), which the app repaired with an EditInPlace stamp. Since
	// mkvgo v0.26.0 the duration each op computes wins over the one copied
	// from the source, so the joined header must simply be right.
	if declared := probeDuration(t, ctx, joined); math.Abs(declared-srcDurationSec) > 1.0 {
		t.Errorf("joined file declares %.3f s, want ~%d s - mkvgo's Join lost the total again", declared, srcDurationSec)
	}

	report := probeReduced(t, ctx, joined)
	var video, audio *probedStream
	for i := range report.Streams {
		switch report.Streams[i].CodecType {
		case "video":
			video = &report.Streams[i]
		case "audio":
			audio = &report.Streams[i]
		}
	}
	if video == nil || audio == nil {
		t.Fatalf("joined file is missing streams: %+v", report.Streams)
	}
	if video.Width != 1152 || video.Height != reduceddl.TargetHeight {
		t.Errorf("joined video is %dx%d, want 1152x%d", video.Width, video.Height, reduceddl.TargetHeight)
	}
	if video.CodecName != "h264" {
		t.Errorf("joined video codec = %q, want h264", video.CodecName)
	}
	if audio.CodecName != "aac" || audio.Channels != 2 {
		t.Errorf("joined audio = %s/%dch, want aac/2ch", audio.CodecName, audio.Channels)
	}
}

type probedStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Channels  int    `json:"channels"`
	Duration  string `json:"duration"`
}

type probedFile struct {
	Streams []probedStream `json:"streams"`
	Format  struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func probeReduced(t *testing.T, ctx context.Context, path string) probedFile {
	t.Helper()
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-show_entries", "stream=codec_type,codec_name,width,height,channels,duration:format=duration",
		"-of", "json", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	var report probedFile
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("parse ffprobe: %v\n%s", err, out)
	}
	return report
}

// lastPacketTime is when the final packet of a track lands. The CONTENT
// answer to "how long is this file", as opposed to what its header claims -
// and the two genuinely disagree after a mkvgo v0.25.1 join.
func lastPacketTime(t *testing.T, ctx context.Context, path, stream string) float64 {
	t.Helper()
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-select_streams", stream, "-show_entries", "packet=pts_time",
		"-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("ffprobe packets of %s in %s: %v", stream, path, err)
	}
	last := 0.0
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSuffix(strings.TrimSpace(line), ",")
		if line == "" {
			continue
		}
		if v, err := strconv.ParseFloat(line, 64); err == nil {
			last = v
		}
	}
	if last == 0 {
		t.Fatalf("no packet timestamps for %s in %s", stream, path)
	}
	return last
}

// probeDuration is what the container DECLARES, which is what a player trusts.
func probeDuration(t *testing.T, ctx context.Context, path string) float64 {
	t.Helper()
	d, err := strconv.ParseFloat(strings.TrimSpace(probeReduced(t, ctx, path).Format.Duration), 64)
	if err != nil {
		t.Fatalf("parse the declared duration of %s: %v", path, err)
	}
	return d
}
