package mkvgo

// On-demand packaging INTEGRATION test: the real OnDemandPackager (mkvgo
// mp4.PlanHLS) against a synthetic H.264 + AAC MKV produced by the bundled
// ffmpeg, asserting the plan serves a coherent HLS + DASH presentation with the
// URL prefix applied. This is the end-to-end proof that the zero-storage copy
// rung actually packages a real file — the unit tests only cover the handler
// routing against a fake packager.
//
// OPT-IN (needs a real ffmpeg to synthesize a Cues-indexed source):
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/mkvgo/ -run Integration -v
//
// or `make transcode-itest` (runs inside the evey-server container).

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
)

func TestOnDemandPackagerIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Source: 30 s H.264 (2 s GOP → keyframes every 2 s so 6 s segments split
	// cleanly) + AAC audio, muxed to MKV. ffmpeg's matroska muxer writes a Cues
	// index, which PlanHLS seeks through.
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "30",
		"-c:v", "libx264", "-g", "48", "-keyint_min", "48", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg source generation failed: %v\n%s", err, out)
	}

	pkg := NewOnDemandPackager()
	defer pkg.Close()

	plan, err := pkg.Plan(ctx, src, apptranscoding.OnDemandOptions{
		SegmentMs: 6000,
		URLPrefix: "od_",
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if n := plan.NumSegments(); n <= 0 {
		t.Fatalf("NumSegments = %d, want > 0", n)
	}
	if !plan.HasDASH() {
		t.Error("HasDASH = false, want a DASH manifest for an unencrypted plan")
	}

	// Master playlist: references its sub-resources through the od_ prefix.
	master, ct, err := plan.Resource(ctx, "master.m3u8")
	if err != nil {
		t.Fatalf("Resource(master.m3u8): %v", err)
	}
	if !bytes.HasPrefix(master, []byte("#EXTM3U")) {
		t.Errorf("master playlist does not start with #EXTM3U:\n%s", master)
	}
	if !bytes.Contains(master, []byte("od_")) {
		t.Errorf("master playlist does not carry the od_ URL prefix:\n%s", master)
	}
	if !strings.Contains(ct, "mpegurl") {
		t.Errorf("master content-type = %q, want an HLS type", ct)
	}

	// First media segment: a real fMP4 CMAF fragment (styp box).
	seg, _, err := plan.Resource(ctx, "seg00001.m4s")
	if err != nil {
		t.Fatalf("Resource(seg00001.m4s): %v", err)
	}
	if len(seg) < 8 || !bytes.Equal(seg[4:8], []byte("styp")) {
		t.Errorf("segment 1 is not a CMAF fragment (no styp box), got %d bytes", len(seg))
	}

	// DASH manifest.
	mpd, _, err := plan.Resource(ctx, "manifest.mpd")
	if err != nil {
		t.Fatalf("Resource(manifest.mpd): %v", err)
	}
	if !bytes.Contains(mpd, []byte("<MPD")) {
		t.Errorf("manifest.mpd is not an MPD document:\n%s", mpd)
	}

	// A second Plan for the same source hits the cache (no rebuild).
	if _, replanErr := pkg.Plan(ctx, src, apptranscoding.OnDemandOptions{SegmentMs: 6000, URLPrefix: "od_"}); replanErr != nil {
		t.Fatalf("second Plan (cache hit): %v", replanErr)
	}

	// Encrypted plan: the media playlist advertises EXT-X-KEY and no DASH
	// manifest is emitted (AES-128 is HLS-only).
	encPlan, err := pkg.Plan(ctx, src, apptranscoding.OnDemandOptions{
		SegmentMs: 6000, URLPrefix: "od_",
		AESKey:    []byte("0123456789abcdef"), AESKeyURI: "key",
	})
	if err != nil {
		t.Fatalf("encrypted Plan: %v", err)
	}
	pl, _, err := encPlan.Resource(ctx, "playlist.m3u8")
	if err != nil {
		t.Fatalf("Resource(playlist.m3u8) encrypted: %v", err)
	}
	if !bytes.Contains(pl, []byte("EXT-X-KEY:METHOD=AES-128")) {
		t.Errorf("encrypted playlist missing EXT-X-KEY:\n%s", pl)
	}
	if !bytes.Contains(pl, []byte(`URI="key"`)) {
		t.Errorf("EXT-X-KEY does not point at the key URI:\n%s", pl)
	}
	if encPlan.HasDASH() {
		t.Error("HasDASH = true for an encrypted plan, want false (HLS only)")
	}
}
