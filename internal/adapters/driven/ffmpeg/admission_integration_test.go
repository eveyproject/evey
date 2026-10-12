package ffmpeg

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
)

// TestVideoAdmissionIntegration proves the admission chain's accounting
// against REAL ffmpeg session lifecycles - the part every unit test fakes.
// The audit established exactly-one-release by reading the code; this runs
// it: a live encode session must hold exactly its cost, a third stream must
// be refused at spawn with ErrVideoCapacityExhausted, teardown must drain
// both pools to zero, and a spawn that fails outright (unreadable source)
// must leak nothing. Any drift here rots into a permanent 503 in production.
func TestVideoAdmissionIntegration(t *testing.T) {
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

	// A short, cheap synthetic source: admission is what is under test, not
	// encode throughput, so 30s of 320x180 keeps the sessions alive long
	// enough to observe while staying quick to produce.
	src := filepath.Join(dir, "admission-src.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "anoisesrc=color=pink:seed=7:sample_rate=48000",
		"-t", "30",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "48", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-ac", "2",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("source generation: %v\n%s", err, out)
	}

	// One calibrated software rung at cost 0.4: two streams fit (0.8), a
	// third (1.2) must be refused. The rung height matches the fingerprints
	// below so CostFor resolves it directly.
	limiter := apptranscoding.NewVideoSessionLimiter(0).WithCostModel(
		apptranscoding.NewVideoCostModel([]domaintranscoding.BenchRungSample{
			{Ladder: "software", Height: 180, FPS: 60, RequiredFPS: 24, Passing: true}, // cost 0.4
		}))

	cache, err := apptranscoding.NewSegmentCache(filepath.Join(dir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	producer := apptranscoding.NewSoftwareProducer(
		cache,
		NewHLSTranscoder(),
		apptranscoding.SourceResolverFunc(func(context.Context, apptranscoding.DecisionFingerprint) (string, []apptranscoding.SourceStream, error) {
			return src, nil, nil
		}),
		apptranscoding.SoftwareProducerOptions{},
		nil,
	).WithVideoSessionLimiter(limiter)

	fpFor := func(mediaID string) apptranscoding.DecisionFingerprint {
		return apptranscoding.DecisionFingerprint{
			MediaID:             mediaID,
			SourcePath:          src,
			VideoCodec:          "h264",
			AudioCodec:          "aac",
			SubtitleBurnInTrack: -1,
			UseFMP4:             true,
			Profiles: []apptranscoding.ProfileFingerprint{
				{Name: "180p-admission-itest", Width: 320, Height: 180, VideoBitrate: 500, AudioBitrate: 128},
			},
		}
	}
	startStream := func(mediaID string) error {
		t.Helper()
		fp := fpFor(mediaID)
		if ensureErr := producer.EnsureSegmentRange(ctx, fp, 0, 0); ensureErr != nil {
			return ensureErr
		}
		key := apptranscoding.SegmentKey{DecisionHash: fp.Hash(), VariantIdx: 0, SegmentIdx: 0}
		if waitErr := cache.Wait(ctx, key, true, 60*time.Second); waitErr != nil {
			t.Fatalf("wait segment for %s: %v", mediaID, waitErr)
		}
		return nil
	}

	// Two real sessions: each must hold exactly its rung cost in the CPU pool.
	if startErr := startStream("admission-a"); startErr != nil {
		t.Fatalf("first stream must be admitted: %v", startErr)
	}
	if startErr := startStream("admission-b"); startErr != nil {
		t.Fatalf("second stream must be admitted (0.8 <= 1.0): %v", startErr)
	}
	if used := limiter.UsedFor(apptranscoding.PoolCPU); used < 0.79 || used > 0.81 {
		t.Fatalf("CPU pool must hold exactly 2 x 0.4 while both sessions encode, got %.3f", used)
	}
	if used := limiter.UsedFor(apptranscoding.PoolGPU); used != 0 {
		t.Fatalf("software sessions must never touch the GPU pool, got %.3f", used)
	}
	if active := limiter.Active(); active != 2 {
		t.Fatalf("two distinct streams must be active, got %d", active)
	}

	// The third stream does not fit (1.2 > 1.0): refused at spawn, and the
	// refusal must surface as the typed capacity error, not a generic one.
	err = producer.EnsureSegmentRange(ctx, fpFor("admission-c"), 0, 0)
	if !errors.Is(err, apptranscoding.ErrVideoCapacityExhausted) {
		t.Fatalf("third stream must be refused with ErrVideoCapacityExhausted, got %v", err)
	}
	if used := limiter.UsedFor(apptranscoding.PoolCPU); used < 0.79 || used > 0.81 {
		t.Fatalf("a refused spawn must reserve nothing, got %.3f", used)
	}

	// Teardown: killing every real ffmpeg session must give every last bit of
	// budget back. This is the exactly-one-release guarantee against real
	// process exits, not mocks. Close is synchronous per session kill but the
	// reap goroutines may lag a beat, hence the bounded poll.
	producer.Close()
	deadline := time.Now().Add(20 * time.Second)
	for limiter.Active() != 0 || limiter.UsedFor(apptranscoding.PoolCPU) != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("pools must drain to zero after Close: active=%d used=%.3f",
				limiter.Active(), limiter.UsedFor(apptranscoding.PoolCPU))
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Failure path: a spawn whose ffmpeg cannot even start (source vanished)
	// must not leak a reservation. A fresh producer, same limiter.
	gone := filepath.Join(dir, "does-not-exist.mkv")
	failing := apptranscoding.NewSoftwareProducer(
		cache,
		NewHLSTranscoder(),
		apptranscoding.SourceResolverFunc(func(context.Context, apptranscoding.DecisionFingerprint) (string, []apptranscoding.SourceStream, error) {
			return gone, nil, nil
		}),
		apptranscoding.SoftwareProducerOptions{},
		nil,
	).WithVideoSessionLimiter(limiter)
	defer failing.Close()

	fpGone := fpFor("admission-gone")
	fpGone.SourcePath = gone
	_ = failing.EnsureSegmentRange(ctx, fpGone, 0, 0)
	// Whatever error shape the failed spawn takes, the accounting must be
	// untouched once the dust settles.
	deadline = time.Now().Add(20 * time.Second)
	for limiter.Active() != 0 || limiter.UsedFor(apptranscoding.PoolCPU) != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("a failed spawn must leak nothing: active=%d used=%.3f",
				limiter.Active(), limiter.UsedFor(apptranscoding.PoolCPU))
		}
		time.Sleep(200 * time.Millisecond)
	}
}
