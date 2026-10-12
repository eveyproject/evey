package mkvgo_test

// Fast unit coverage for the unsupported-container gate lives here; the real
// per-track verdict against ffmpeg-generated files is an integration test
// (real mkvgo ops.Playability, real files), gated like the other real-file
// tests in this package.
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/mkvgo/ -run Integration -v

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	mkvgoadapter "github.com/eveyproject/evey/internal/adapters/driven/mkvgo"
	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

func TestPlayability_UnsupportedContainerIsEmpty(t *testing.T) {
	a := mkvgoadapter.New()
	report, err := a.Playability(context.Background(), "/media/movie.avi", apptranscoding.PlayabilityTarget{Name: "test"})
	if err != nil {
		t.Fatalf("Playability: %v", err)
	}
	if report != nil {
		t.Errorf("report = %+v, want nil for a container mkvgo cannot evaluate head-only", report)
	}
}

func TestPlayabilityIntegration_RemuxAndTranscode(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	dir := t.TempDir()
	a := mkvgoadapter.New()

	// An H.264 High@4.1 / AAC MKV: every mse-generic-shaped target field this
	// source satisfies except the container itself (mkv is not in
	// target.Containers) - so the overall verdict should be "remux", cheapest
	// container "mp4", not a transcode.
	h264 := filepath.Join(dir, "h264.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25",
		"-f", "lavfi", "-i", "sine=frequency=1000:sample_rate=48000",
		"-t", "2", "-c:v", "libx264", "-profile:v", "high", "-level", "4.1",
		"-c:a", "aac", "-shortest",
		h264)
	if out, genErr := gen.CombinedOutput(); genErr != nil {
		t.Fatalf("ffmpeg source generation failed: %v\n%s", genErr, out)
	}

	mp4Target := apptranscoding.PlayabilityTarget{
		Name:        "mse-generic",
		Containers:  []string{"mp4"},
		VideoCodecs: []string{"h264"},
		AudioCodecs: []string{"aac"},
	}
	report, err := a.Playability(ctx, h264, mp4Target)
	if err != nil {
		t.Fatalf("Playability: %v", err)
	}
	if report == nil {
		t.Fatal("report = nil, want a real report for an EBML container")
	}
	if report.OverallVerdict != "remux" {
		t.Errorf("overall verdict = %q, want %q (container-only mismatch)", report.OverallVerdict, "remux")
	}
	if report.RemuxContainer != "mp4" {
		t.Errorf("remux container = %q, want %q", report.RemuxContainer, "mp4")
	}

	// A target rejecting the video codec entirely: the video track (and so the
	// overall verdict) must be "transcode".
	strictTarget := apptranscoding.PlayabilityTarget{
		Name:        "no-h264",
		Containers:  []string{"mp4"},
		VideoCodecs: []string{"av1"},
		AudioCodecs: []string{"aac"},
	}
	strictReport, err := a.Playability(ctx, h264, strictTarget)
	if err != nil {
		t.Fatalf("Playability(strictTarget): %v", err)
	}
	if strictReport.OverallVerdict != "transcode" {
		t.Errorf("overall verdict = %q, want %q", strictReport.OverallVerdict, "transcode")
	}
}
