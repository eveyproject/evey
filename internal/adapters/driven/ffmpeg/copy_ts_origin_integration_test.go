package ffmpeg

// Copy rung over an MPEG-TS, whose first keyframe sits ~1.4 s into the
// source: the plan counts from that keyframe, and a seek session must land on
// the requested keyframe and label it where the cold session would. Before
// the plan origin, -ss was read from the container's start_time (segment 3
// asked, segment 4's picture served under its name) and the seek rebase used
// the raw source time (1.48 s late against the cold fragments).
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/ffmpeg/ -run TestCopyTSOriginIntegration -v

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
)

func TestCopyTSOriginIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest` - needs the bundled ffmpeg)")
	}
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not in PATH", bin)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	dir := t.TempDir()

	// 300 s so a deep seek is beyond the cold session's reach; 1 s GOPs, the
	// cadence where the start_time offset crossed a whole keyframe.
	src := filepath.Join(dir, "itest-src.ts")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25",
		"-f", "lavfi", "-i", "anoisesrc=color=pink:seed=42:sample_rate=48000",
		"-t", "300", "-c:v", "libx264", "-preset", "ultrafast", "-bf", "2",
		"-g", "25", "-keyint_min", "25", "-sc_threshold", "0", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-ac", "2", "-f", "mpegts", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("source generation: %v\n%s", err, out)
	}
	prober := NewProber("ffprobe")
	kf, err := prober.ProbeKeyframes(ctx, src)
	if err != nil || len(kf) < 100 {
		t.Fatalf("probe keyframes: %v (%d found)", err, len(kf))
	}
	if kf[0] < 1000 {
		t.Fatalf("the source must start late like a real MPEG-TS, first keyframe at %d ms", kf[0])
	}
	durMs := probeDurationMs(t, ctx, src)
	planner := apptranscoding.NewCopyPlanner(itestStreams{kf: kf}, itestItems{durMs: durMs})
	plan, origin, err := planner.PlanWithOrigin(ctx, "itest-media")
	if err != nil {
		t.Fatal(err)
	}

	cache, err := apptranscoding.NewSegmentCache(filepath.Join(dir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	producer := apptranscoding.NewSoftwareProducer(cache, NewHLSTranscoder(),
		apptranscoding.SourceResolverFunc(func(context.Context, apptranscoding.DecisionFingerprint) (string, []apptranscoding.SourceStream, error) {
			return src, nil, nil
		}), apptranscoding.SoftwareProducerOptions{}, nil)
	defer producer.Close()
	producer.SetCopyMode(planner, prober)

	fp := apptranscoding.DecisionFingerprint{
		MediaID: "itest-media", SourcePath: src, VideoCodec: "h264", AudioCodec: "aac",
		SubtitleBurnInTrack: -1, UseFMP4: true,
		Profiles: []apptranscoding.ProfileFingerprint{{Name: "180p-copy-itest", Width: 320, Height: 180, VideoBitrate: 1000, AudioBitrate: 192, Copy: true}},
	}
	hash := fp.Hash()
	fetch := func(segIdx int) string {
		t.Helper()
		if ensureErr := producer.EnsureSegmentRange(ctx, fp, 0, segIdx); ensureErr != nil {
			t.Fatalf("EnsureSegmentRange(%d): %v", segIdx, ensureErr)
		}
		key := apptranscoding.SegmentKey{DecisionHash: hash, VariantIdx: 0, SegmentIdx: segIdx}
		if waitErr := cache.Wait(ctx, key, true, 90*time.Second); waitErr != nil {
			t.Fatalf("wait segment %d: %v", segIdx, waitErr)
		}
		d, _ := cache.DirFor(hash)
		return filepath.Join(d, fmt.Sprintf("stream_0_%03d.m4s", segIdx))
	}
	seg0 := fetch(0)
	deep := (len(plan) * 3) / 4
	segDeep := fetch(deep)
	if n := producer.ActiveSessionCount(); n < 2 {
		t.Fatalf("deep seek did not spawn a seek session (%d active)", n)
	}

	initPath := filepath.Join(filepath.Dir(segDeep), "stream_0_init.mp4")
	scales, err := parseInitTimescales(initPath)
	if err != nil {
		t.Fatal(err)
	}
	videoStart := func(path string) int64 {
		t.Helper()
		starts := segmentTrackStartsMs(t, path, scales)
		v, ok := starts[1]
		if !ok {
			t.Fatalf("%s: no video traf", filepath.Base(path))
		}
		return v
	}
	// The cold session and the seek session on ONE timeline: the plan's.
	if got := videoStart(seg0); got != plan[0].StartMs {
		t.Errorf("cold segment 0 video starts at %d ms, plan says %d", got, plan[0].StartMs)
	}
	if got, want := videoStart(segDeep), plan[deep].StartMs; got < want-5 || got > want+5 {
		t.Errorf("seek segment %d video starts at %d ms, plan says %d (origin %d ms)", deep, got, want, origin)
	}

	// The picture: the deep segment opens on the source frame at its plan
	// keyframe, in the source's own time.
	joined := filepath.Join(dir, "deep.mp4")
	initB, _ := os.ReadFile(initPath)
	segB, _ := os.ReadFile(segDeep)
	if writeErr := os.WriteFile(joined, append(initB, segB...), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}
	firstFrame := func(args ...string) string {
		t.Helper()
		out, runErr := exec.CommandContext(ctx, "ffmpeg", append(append([]string{"-v", "error"}, args...),
			"-map", "0:v:0", "-frames:v", "1", "-f", "framemd5", "-")...).Output()
		if runErr != nil {
			t.Fatalf("framemd5 %v: %v", args, runErr)
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		f := strings.Split(lines[len(lines)-1], ",")
		return strings.TrimSpace(f[len(f)-1])
	}
	got := firstFrame("-i", joined)
	want := firstFrame("-seek_timestamp", "1", "-ss", fmt.Sprintf("%.3f", float64(plan[deep].StartMs+origin)/1000), "-i", src)
	if got != want {
		t.Errorf("seek segment %d opens on another picture than the source keyframe at %d ms", deep, plan[deep].StartMs+origin)
	}
}
