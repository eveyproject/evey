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

	// A chapterless source emits no EXT-X-DATERANGE lines: ChapterMarkers is
	// hardcoded on in the adapter, but it must stay a no-op for a file with no
	// chapters (byte-identical to the pre-ChapterMarkers output).
	videoPlaylist, _, err := plan.Resource(ctx, "playlist.m3u8")
	if err != nil {
		t.Fatalf("Resource(playlist.m3u8): %v", err)
	}
	if bytes.Contains(videoPlaylist, []byte("EXT-X-DATERANGE")) {
		t.Errorf("chapterless source's playlist carries EXT-X-DATERANGE:\n%s", videoPlaylist)
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

	// mkvgo v0.18.0 fix: EXT-X-MAP (the clear init segment) must precede
	// EXT-X-KEY, else a player tries to decrypt the clear init and retry-loops
	// (hls.js fragDecryptError). Lock the ordering from Evey's side.
	if mapIdx, keyIdx := bytes.Index(pl, []byte("EXT-X-MAP")), bytes.Index(pl, []byte("EXT-X-KEY")); mapIdx < 0 || keyIdx < 0 || mapIdx > keyIdx {
		t.Errorf("EXT-X-MAP must appear before EXT-X-KEY (map=%d key=%d):\n%s", mapIdx, keyIdx, pl)
	}

	// Rotated plan (forward secrecy): a fresh EXT-X-KEY per period, each at its
	// own key?i=<n> URI. Rotate every segment so even a short clip exercises it.
	rotPlan, err := pkg.Plan(ctx, src, apptranscoding.OnDemandOptions{
		SegmentMs: 6000, URLPrefix: "od_",
		AESRotateSegments: 1,
		AESKeys: []apptranscoding.OnDemandAESKey{
			{Key: []byte("0000000000000000"), URI: "key?i=0"},
			{Key: []byte("1111111111111111"), URI: "key?i=1"},
			{Key: []byte("2222222222222222"), URI: "key?i=2"},
		},
	})
	if err != nil {
		t.Fatalf("rotated Plan: %v", err)
	}
	rpl, _, err := rotPlan.Resource(ctx, "playlist.m3u8")
	if err != nil {
		t.Fatalf("Resource(playlist.m3u8) rotated: %v", err)
	}
	if !bytes.Contains(rpl, []byte(`URI="key?i=0"`)) {
		t.Errorf("rotated playlist missing the per-period key URI scheme:\n%s", rpl)
	}
	if nKeys := bytes.Count(rpl, []byte("EXT-X-KEY:METHOD=AES-128")); rotPlan.NumSegments() >= 2 && nKeys < 2 {
		t.Errorf("rotated playlist has %d EXT-X-KEY for %d segments, want >= 2:\n%s", nKeys, rotPlan.NumSegments(), rpl)
	}
}

// TestOnDemandPackagerIntegration_ChapterMarkers proves the mkvgo v0.17.0
// ChapterMarkers wiring: a source with chapters gets EXT-X-DATERANGE lines in
// its media playlist, one per chapter, carrying the chapter title.
func TestOnDemandPackagerIntegration_ChapterMarkers(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	dir := t.TempDir()
	metaPath := filepath.Join(dir, "chapters.txt")
	meta := ";FFMETADATA1\n\n" +
		"[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=15000\ntitle=Chapter One\n\n" +
		"[CHAPTER]\nTIMEBASE=1/1000\nSTART=15000\nEND=30000\ntitle=Chapter Two\n"
	if err := os.WriteFile(metaPath, []byte(meta), 0o644); err != nil {
		t.Fatalf("write chapters metadata: %v", err)
	}

	// Same synthetic source as the base test, plus a chapters metadata input
	// mapped in via -map_metadata so ffmpeg's matroska muxer writes real
	// Matroska ChapterAtom entries.
	src := filepath.Join(dir, "src_chapters.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-i", metaPath,
		"-map_metadata", "2",
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

	videoPlaylist, _, err := plan.Resource(ctx, "playlist.m3u8")
	if err != nil {
		t.Fatalf("Resource(playlist.m3u8): %v", err)
	}
	if got := bytes.Count(videoPlaylist, []byte("EXT-X-DATERANGE")); got != 2 {
		t.Errorf("EXT-X-DATERANGE count = %d, want 2 (one per chapter):\n%s", got, videoPlaylist)
	}
	if !bytes.Contains(videoPlaylist, []byte("Chapter One")) {
		t.Errorf("playlist missing first chapter title:\n%s", videoPlaylist)
	}
	if !bytes.Contains(videoPlaylist, []byte("Chapter Two")) {
		t.Errorf("playlist missing second chapter title:\n%s", videoPlaylist)
	}
}
