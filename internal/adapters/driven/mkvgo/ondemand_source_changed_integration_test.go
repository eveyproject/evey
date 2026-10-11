package mkvgo

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
)

// TestOnDemandPackagerIntegration_SourceReplacedMidPlayback replaces a media's
// file while a plan built on it is in use: the plan refuses its next segment
// with the port's ErrSourceChanged, Replan drops it from the cache and builds
// the new file's plan, whose SourceID differs and whose segment is the new
// file's. Same opt-in as the other integration tests.
func TestOnDemandPackagerIntegration_SourceReplacedMidPlayback(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	dir := t.TempDir()
	src := filepath.Join(dir, "src.mkv")
	replacement := filepath.Join(dir, "replacement.mkv")
	gen := func(path, pattern, seconds string) {
		cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
			"-f", "lavfi", "-i", pattern+"=size=320x180:rate=24",
			"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
			"-t", seconds,
			"-c:v", "libx264", "-g", "48", "-keyint_min", "48", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-b:a", "128k",
			path)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("ffmpeg source generation failed: %v\n%s", err, out)
		}
	}
	gen(src, "testsrc2", "30")
	gen(replacement, "testsrc", "24")

	// Reference: what a plan over the replacement serves, built on its own.
	ref := NewOnDemandPackager()
	defer ref.Close()
	opts := apptranscoding.OnDemandOptions{SegmentMs: 6000, URLPrefix: "od_"}
	refPlan, err := ref.Plan(ctx, replacement, opts)
	if err != nil {
		t.Fatalf("reference Plan: %v", err)
	}
	wantSeg, _, err := refPlan.Resource(ctx, "seg00001.m4s")
	if err != nil {
		t.Fatalf("reference Resource(seg00001.m4s): %v", err)
	}

	pkg := NewOnDemandPackager()
	defer pkg.Close()
	plan, err := pkg.Plan(ctx, src, opts)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	staleKey := onDemandCacheKey(src, opts)
	before, _, err := plan.Resource(ctx, "seg00001.m4s")
	if err != nil {
		t.Fatalf("Resource(seg00001.m4s) before the replacement: %v", err)
	}
	if bytes.Equal(before, wantSeg) {
		t.Fatal("the two sources serve the same segment: the test proves nothing")
	}

	// The file is replaced while the plan is held (a rename over it, as a
	// library manager or an rsync does).
	if err = os.Rename(replacement, src); err != nil {
		t.Fatalf("replace the source: %v", err)
	}

	if _, _, err = plan.Resource(ctx, "seg00001.m4s"); !errors.Is(err, apptranscoding.ErrSourceChanged) {
		t.Fatalf("Resource after the replacement: err = %v, want ErrSourceChanged", err)
	}
	if _, err = plan.(apptranscoding.OnDemandOpener).Open(ctx, "seg00001.m4s"); !errors.Is(err, apptranscoding.ErrSourceChanged) {
		t.Fatalf("Open after the replacement: err = %v, want ErrSourceChanged", err)
	}
	fresh, err := pkg.Replan(ctx, plan)
	if err != nil {
		t.Fatalf("Replan: %v", err)
	}
	if _, ok := pkg.plans.Get(staleKey); ok {
		t.Error("the stale plan is still cached under the old file's key")
	}
	if fresh.SourceID() == "" || fresh.SourceID() == plan.SourceID() {
		t.Errorf("SourceID: old %q, new %q, want two different stamps", plan.SourceID(), fresh.SourceID())
	}
	if fresh.SourceID() != refPlan.SourceID() {
		t.Errorf("SourceID of the rebuilt plan %q, want the replacement's %q", fresh.SourceID(), refPlan.SourceID())
	}
	got, _, err := fresh.Resource(ctx, "seg00001.m4s")
	if err != nil {
		t.Fatalf("rebuilt plan Resource(seg00001.m4s): %v", err)
	}
	if !bytes.Equal(got, wantSeg) {
		t.Errorf("rebuilt plan: %d bytes, not the new file's segment (%d bytes)", len(got), len(wantSeg))
	}

	// The next playback asks the packager again and starts without an error.
	next, err := pkg.Plan(ctx, src, opts)
	if err != nil {
		t.Fatalf("Plan after the replacement: %v", err)
	}
	for _, name := range []string{"master.m3u8", "playlist.m3u8", "init.mp4", "seg00001.m4s"} {
		if _, _, err := next.Resource(ctx, name); err != nil {
			t.Errorf("next playback Resource(%s): %v", name, err)
		}
	}
}
