package mkvgo

// On-demand packaging INTEGRATION test: the real OnDemandPackager (mkvgo
// mp4.PlanHLS) against a synthetic H.264 + AAC MKV produced by the bundled
// ffmpeg, asserting the plan serves a coherent HLS + DASH presentation with the
// URL prefix applied. This is the end-to-end proof that the zero-storage copy
// rung actually packages a real file - the unit tests only cover the handler
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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
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
	// No I-frame stream: no client uses it, and serving it walks keyframes.
	if bytes.Contains(master, []byte("EXT-X-I-FRAME-STREAM-INF")) {
		t.Errorf("master advertises an I-frame stream:\n%s", master)
	}
	if _, _, ierr := plan.Resource(ctx, "iframe.m3u8"); ierr == nil {
		t.Error("Resource(iframe.m3u8) served, want an error")
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
		AESKey: []byte("0123456789abcdef"), AESKeyURI: "key",
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

// TestOnDemandPackagerIntegration_AudioResync proves E2 end to end: a source
// with the classic repack mux defect (audio track starting late relative to
// the video, muxed via -itsoffset like the real dubbed titles
// cases) is served with its A/V desync CANCELLED IN PRESENTATION - no ffmpeg,
// nothing written to the source - because Plan wires the real
// matroska.AudioStartDelays measurement into mp4.Options.AudioPresentationShift.
func TestOnDemandPackagerIntegration_AudioResync(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dir := t.TempDir()

	// clean.mkv: video+audio aligned at 0. delayed.mkv: same content, audio
	// track re-muxed with a +900ms offset (repack defect), Cues kept so
	// PlanHLS packages it without a SynthesizeIndex walk in the way.
	clean := filepath.Join(dir, "clean.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "20",
		"-c:v", "libx264", "-g", "48", "-keyint_min", "48", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k",
		clean)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("clean source generation: %v\n%s", err, out)
	}
	delayed := filepath.Join(dir, "delayed.mkv")
	mux := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-i", clean,
		"-itsoffset", fmt.Sprintf("%.3f", 0.9), "-i", clean,
		"-map", "0:v:0", "-map", "1:a:0", "-c", "copy",
		delayed)
	if out, err := mux.CombinedOutput(); err != nil {
		t.Fatalf("delayed remux: %v\n%s", err, out)
	}

	pkg := NewOnDemandPackager()
	defer pkg.Close()
	defaultAudioDelays := pkg.audioDelays // the real matroska.AudioStartDelays wiring

	shifted, err := pkg.Plan(ctx, delayed, apptranscoding.OnDemandOptions{SegmentMs: 6000})
	if err != nil {
		t.Fatalf("Plan(delayed): %v", err)
	}
	shiftedInit, _, err := shifted.Resource(ctx, "init_a1.mp4")
	if err != nil {
		t.Fatalf("Resource(init_a1.mp4) shifted: %v", err)
	}

	// Control: the same delayed source, but with the audio-delay probe
	// disabled - the pre-E2 behaviour (desynced copy, no resync).
	pkg.audioDelays = func(context.Context, string) (map[uint64]int64, error) { return nil, nil }
	unshifted, err := pkg.Plan(ctx, delayed, apptranscoding.OnDemandOptions{SegmentMs: 6000, URLPrefix: "unshifted_"})
	if err != nil {
		t.Fatalf("Plan(delayed, no probe): %v", err)
	}
	unshiftedInit, _, err := unshifted.Resource(ctx, "init_a1.mp4")
	if err != nil {
		t.Fatalf("Resource(init_a1.mp4) unshifted: %v", err)
	}

	if bytes.Equal(shiftedInit, unshiftedInit) {
		t.Error("a real 900ms desync must change the audio init segment's edit list once resync is wired")
	}

	// Aligned source: the probe measures ~0 and the plan is byte-identical
	// with or without it wired - no regression on a healthy file.
	pkg.audioDelays = defaultAudioDelays
	alignedShifted, err := pkg.Plan(ctx, clean, apptranscoding.OnDemandOptions{SegmentMs: 6000, URLPrefix: "aligned_"})
	if err != nil {
		t.Fatalf("Plan(clean): %v", err)
	}
	alignedInit, _, err := alignedShifted.Resource(ctx, "init_a1.mp4")
	if err != nil {
		t.Fatalf("Resource(init_a1.mp4) aligned: %v", err)
	}
	pkg.audioDelays = func(context.Context, string) (map[uint64]int64, error) { return nil, nil }
	alignedNoProbe, err := pkg.Plan(ctx, clean, apptranscoding.OnDemandOptions{SegmentMs: 6000, URLPrefix: "aligned2_"})
	if err != nil {
		t.Fatalf("Plan(clean, no probe): %v", err)
	}
	alignedNoProbeInit, _, err := alignedNoProbe.Resource(ctx, "init_a1.mp4")
	if err != nil {
		t.Fatalf("Resource(init_a1.mp4) aligned no-probe: %v", err)
	}
	if !bytes.Equal(alignedInit, alignedNoProbeInit) {
		t.Error("an aligned source's audio init must not change when the resync is wired (below the defect floor)")
	}
}

// TestOnDemandPackagerIntegration_KeepAudioTrack proves the track-trim wiring
// end to end: a real 3-audio-track MKV (a "MULTI" release) trimmed to one
// track advertises exactly ONE audio EXT-X-MEDIA in its master, while the
// untrimmed plan for the same source advertises all three - and the video
// still plays (segment 1 is a real fMP4 fragment either way).
func TestOnDemandPackagerIntegration_KeepAudioTrack(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dir := t.TempDir()

	src := filepath.Join(dir, "multi.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-f", "lavfi", "-i", "sine=frequency=660:sample_rate=48000",
		"-f", "lavfi", "-i", "sine=frequency=880:sample_rate=48000",
		"-t", "20",
		"-map", "0:v:0", "-map", "1:a:0", "-map", "2:a:0", "-map", "3:a:0",
		"-c:v", "libx264", "-g", "48", "-keyint_min", "48", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k",
		"-metadata:s:a:0", "language=eng", "-metadata:s:a:1", "language=fre", "-metadata:s:a:2", "language=jpn",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("multi-audio source generation failed: %v\n%s", err, out)
	}

	pkg := NewOnDemandPackager()
	defer pkg.Close()

	full, err := pkg.Plan(ctx, src, apptranscoding.OnDemandOptions{SegmentMs: 6000, URLPrefix: "full_"})
	if err != nil {
		t.Fatalf("Plan (untrimmed): %v", err)
	}
	fullMaster, _, err := full.Resource(ctx, "master.m3u8")
	if err != nil {
		t.Fatalf("Resource(master.m3u8) untrimmed: %v", err)
	}
	if got := bytes.Count(fullMaster, []byte("TYPE=AUDIO")); got != 3 {
		t.Errorf("untrimmed master has %d AUDIO renditions, want 3:\n%s", got, fullMaster)
	}

	trackIdx := 1 // the French track
	trimmed, err := pkg.Plan(ctx, src, apptranscoding.OnDemandOptions{SegmentMs: 6000, URLPrefix: "trim_", KeepAudioTrack: &trackIdx})
	if err != nil {
		t.Fatalf("Plan (trimmed): %v", err)
	}
	trimmedMaster, _, err := trimmed.Resource(ctx, "master.m3u8")
	if err != nil {
		t.Fatalf("Resource(master.m3u8) trimmed: %v", err)
	}
	if got := bytes.Count(trimmedMaster, []byte("TYPE=AUDIO")); got != 1 {
		t.Errorf("trimmed master has %d AUDIO renditions, want 1:\n%s", got, trimmedMaster)
	}
	if !bytes.Contains(trimmedMaster, []byte(`LANGUAGE="fre"`)) {
		t.Errorf("trimmed master's single audio rendition should be the French track:\n%s", trimmedMaster)
	}

	// The video still packages fine under the trim - not just the master text.
	seg, _, err := trimmed.Resource(ctx, "seg00001.m4s")
	if err != nil {
		t.Fatalf("Resource(seg00001.m4s) trimmed: %v", err)
	}
	if len(seg) < 8 || !bytes.Equal(seg[4:8], []byte("styp")) {
		t.Errorf("trimmed segment 1 is not a CMAF fragment, got %d bytes", len(seg))
	}
}

// One un-portable audio track must NOT kill the whole plan.
//
// mp4.PlanHLS fails hard on a codec it cannot carry ("track N: codec %q cannot
// be remuxed to MP4 (set Options.SkipUnsupported to drop it)"), so a single
// TrueHD / LPCM / Vorbis track - routine in a Blu-ray remux - made the entire
// on-demand plan fail. The handler then logged "on-demand plan build failed,
// falling back to ffmpeg" and RE-ENCODED THE VIDEO, which was perfectly
// copy-eligible. Proven live on a real HEVC/bt709 + TrueHD title:
//
//	err=mkvgo PlanHLS ".../S01E06.mkv": mp4: track 3: codec "truehd" cannot be
//	remuxed to MP4 (set Options.SkipUnsupported to drop it)
//
// The admin remux path already passes SkipUnsupported (adapter.go); the serving
// path did not. Evey re-encodes the audio it can't carry anyway (hybrid rung),
// so mkvgo dropping the track is exactly right.
func TestOnDemandPackagerIntegration_UnportableAudioTrackIsDropped(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// H.264 video (carriable) + TrueHD audio (NOT carriable to MP4).
	dir := t.TempDir()
	src := filepath.Join(dir, "truehd.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "10",
		"-c:v", "libx264", "-g", "48", "-keyint_min", "48", "-pix_fmt", "yuv420p",
		"-strict", "-2", // truehd is flagged experimental in ffmpeg
		"-c:a", "truehd", "-ac", "2",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg truehd source generation failed: %v\n%s", err, out)
	}

	pkg := NewOnDemandPackager()
	defer pkg.Close()

	plan, err := pkg.Plan(ctx, src, apptranscoding.OnDemandOptions{SegmentMs: 6000, URLPrefix: "od_"})
	if err != nil {
		t.Fatalf("Plan must drop the un-portable audio track, not fail: %v", err)
	}
	if n := plan.NumSegments(); n <= 0 {
		t.Fatalf("NumSegments = %d, want > 0 (video must still be packaged)", n)
	}
}
