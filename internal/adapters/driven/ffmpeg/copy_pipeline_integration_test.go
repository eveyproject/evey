package ffmpeg

// Copy-rung INTEGRATION test: the real SoftwareProducer + real ffmpeg/ffprobe
// against a synthetic B-frame MKV, asserting the served bytes match the
// keyframe plan — the end-to-end property no unit test can capture because it
// depends on the actual ffmpeg binary's behavior (session-relative split
// grids, cue/DTS-heuristic seek landings, accurate-seek audio trimming: each
// shipped a desync in 2026-06 that the unit suites couldn't see).
//
// OPT-IN by design: it must run against the BUNDLED ffmpeg build
// (jellyfin-ffmpeg), not whatever a CI distro ships — a divergent binary
// makes both false reds and false greens. Run it via `make transcode-itest`
// (executes inside the evey-server container) or:
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/ffmpeg/ -run TestCopyPipelineIntegration -v
//
// Run it before shipping ANY change to the copy pipeline (planner, builder
// args, producer session logic, rebase pump) or bumping the ffmpeg image.

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

type itestStreams struct{ kf []int64 }

func (s itestStreams) GetByMediaItemID(context.Context, shared.MediaItemID) ([]*domainlibrary.MediaStream, error) {
	return []*domainlibrary.MediaStream{{StreamType: domainlibrary.StreamTypeVideo, Codec: "h264", Keyframes: s.kf}}, nil
}

type itestItems struct{ durMs int64 }

func (s itestItems) GetByID(context.Context, shared.MediaItemID) (*domainlibrary.MediaItem, error) {
	d, err := shared.NewDurationMs(s.durMs)
	if err != nil {
		return nil, err
	}
	return &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{Duration: d}}, nil
}

func TestCopyPipelineIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest` — needs the bundled ffmpeg)")
	}
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not in PATH", bin)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	dir := t.TempDir()

	// --- Source: 300 s H.264 WITH B-frames (-bf 3 arms the ffmpeg CLI's
	// ~130 ms DTS seek heuristic) and irregular keyframes (10 s / 3 s
	// alternation — the long-GOP pattern that broke the n×hls_time grid).
	// Audio = seeded pink noise: APERIODIC, so the content correlation has a
	// single sharp peak (a sine correlates at every period — useless), AC-3
	// so the decode + AAC re-encode chain (the trim-labeling victim) runs.
	// 300 s keeps the deep-seek target beyond the cold session's burst+pacing
	// reach at request time, forcing a REAL seek session (landing probe and
	// all) instead of the cold session covering everything first.
	src := filepath.Join(dir, "itest-src.mkv")
	const srcDurSec = 300
	var kfTimes []string
	for base := 0.0; base < srcDurSec; base += 13 {
		kfTimes = append(kfTimes, fmt.Sprintf("%.3f", base), fmt.Sprintf("%.3f", base+10))
	}
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "anoisesrc=color=pink:seed=42:sample_rate=48000",
		"-t", fmt.Sprint(srcDurSec),
		"-c:v", "libx264", "-preset", "ultrafast", "-bf", "3",
		"-force_key_frames", strings.Join(kfTimes, ","),
		"-sc_threshold", "0", "-g", "9999", "-pix_fmt", "yuv420p",
		"-c:a", "ac3", "-ac", "2",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("source generation: %v\n%s", err, out)
	}

	// --- Plan from the REAL prober, exactly like the keyframe backfill.
	prober := NewProber("ffprobe")
	kf, err := prober.ProbeKeyframes(ctx, src)
	if err != nil || len(kf) < 10 {
		t.Fatalf("probe keyframes: %v (%d found)", err, len(kf))
	}
	const durMs = srcDurSec * 1000
	plan := apptranscoding.KeyframeSegments(kf, durMs)
	if len(plan) < 10 {
		t.Fatalf("plan too small: %d segments", len(plan))
	}

	// --- Real producer in copy mode over a real cache + real transcoder.
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
	)
	defer producer.Close()
	producer.SetCopyMode(
		apptranscoding.NewCopyPlanner(itestStreams{kf: kf}, itestItems{durMs: durMs}),
		prober,
	)

	fp := apptranscoding.DecisionFingerprint{
		MediaID:             "itest-media",
		SourcePath:          src,
		VideoCodec:          "h264",
		AudioCodec:          "aac",
		SubtitleBurnInTrack: -1,
		UseFMP4:             true,
		Profiles:            []apptranscoding.ProfileFingerprint{{Name: "180p-copy-itest", Width: 320, Height: 180, VideoBitrate: 1000, AudioBitrate: 192, Copy: true}},
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
		d, dirErr := cache.DirFor(hash)
		if dirErr != nil {
			t.Fatal(dirErr)
		}
		return filepath.Join(d, fmt.Sprintf("stream_0_%03d.m4s", segIdx))
	}

	// Cold start covers segment 0; a deep seek (beyond the cold session's
	// reach window) spawns the landing-probed seek session.
	seg0 := fetch(0)
	deep := (len(plan) * 3) / 4
	segDeep := fetch(deep)
	if n := producer.ActiveSessionCount(); n < 2 {
		t.Fatalf("deep seek did not spawn a seek session (%d active) — the landing-probe path was NOT exercised; deepen the target or shorten the burst", n)
	}

	// --- Timeline assertions: first tfdt of EVERY track == plan position.
	initPath := filepath.Join(filepath.Dir(segDeep), "stream_0_init.mp4")
	scales, err := parseInitTimescales(initPath)
	if err != nil {
		t.Fatalf("init timescales: %v", err)
	}
	checkSegment := func(path string, wantMs int64, tolMs int64) {
		t.Helper()
		starts := segmentTrackStartsMs(t, path, scales)
		if len(starts) < 2 {
			t.Fatalf("%s: want video+audio trafs in the SAME segment (audio missing = the accurate-seek trim bug), got %d tracks", filepath.Base(path), len(starts))
		}
		for trackID, gotMs := range starts {
			if diff := gotMs - wantMs; diff > tolMs || diff < -tolMs {
				t.Errorf("%s track %d starts at %dms, plan says %dms (drift %+dms)", filepath.Base(path), trackID, gotMs, wantMs, diff)
			}
		}
	}
	checkSegment(seg0, plan[0].StartMs, 150)
	checkSegment(segDeep, plan[deep].StartMs, 300) // seek path: nudge + priming headroom

	// --- Audio CONTENT correlation: the decisive check the desyncs needed.
	// Decode the deep segment's audio and a source window around the plan
	// position; the best alignment must sit at the plan position (a trimmed-
	// but-mislabeled session shows up as a full-GOP offset here).
	offset := correlateAudio(t, ctx, dir, initPath, segDeep, src, float64(plan[deep].StartMs)/1000.0)
	if math.Abs(offset) > 0.25 {
		t.Errorf("deep-seek audio CONTENT is offset by %+.3fs from the plan position (trim/labeling regression)", offset)
	}
}

// segmentTrackStartsMs returns trackID → first tfdt (ms) for each traf in the
// fMP4 segment at path.
func segmentTrackStartsMs(t *testing.T, path string, scales map[uint32]uint32) map[uint32]int64 {
	t.Helper()
	buf, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[uint32]int64, 2)
	childBoxes(buf, 0, len(buf), func(typ string, off, size int) bool {
		if typ == "sidx" && size >= 28 {
			refID := binary.BigEndian.Uint32(buf[off+12 : off+16])
			if ts := binary.BigEndian.Uint32(buf[off+16 : off+20]); ts != 0 {
				scales[refID] = ts
			}
		}
		if typ != "moof" {
			return true
		}
		childBoxes(buf, off+8, off+size, func(t2 string, o2, s2 int) bool {
			if t2 != "traf" {
				return true
			}
			var trackID uint32
			var tfdt uint64
			seen := false
			childBoxes(buf, o2+8, o2+s2, func(t3 string, o3, s3 int) bool {
				switch t3 {
				case "tfhd":
					if s3 >= 16 {
						trackID = binary.BigEndian.Uint32(buf[o3+12 : o3+16])
					}
				case "tfdt":
					if buf[o3+8] == 1 && s3 >= 20 {
						tfdt, seen = binary.BigEndian.Uint64(buf[o3+12:o3+20]), true
					} else if s3 >= 16 {
						tfdt, seen = uint64(binary.BigEndian.Uint32(buf[o3+12:o3+16])), true
					}
				}
				return true
			})
			if seen {
				if ts := scales[trackID]; ts != 0 {
					if _, dup := out[trackID]; !dup {
						out[trackID] = int64(tfdt * 1000 / uint64(ts)) //nolint:gosec // bounded media timestamps
					}
				}
			}
			return true
		})
		return true
	})
	return out
}

// correlateAudio decodes the segment's audio (init+segment concatenated) and a
// source window around wantSec, cross-correlates, and returns the offset (s)
// of the segment's audio content relative to the plan position (0 = aligned).
func correlateAudio(t *testing.T, ctx context.Context, dir, initPath, segPath, src string, wantSec float64) float64 {
	t.Helper()
	const rate = 8000
	clip := filepath.Join(dir, "clip.mp4")
	initB, err := os.ReadFile(initPath)
	if err != nil {
		t.Fatal(err)
	}
	segB, err := os.ReadFile(segPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(clip, append(initB, segB...), 0o644); err != nil {
		t.Fatal(err)
	}
	probe := decodeAudio(t, ctx, clip, 0, 0, rate)
	if len(probe) > rate*2 {
		probe = probe[:rate*2]
	}
	winStart := wantSec - 4
	ref := decodeAudio(t, ctx, src, winStart, 10, rate)
	if len(probe) < rate/2 || len(ref) <= len(probe) {
		t.Fatalf("audio decode too short (probe %d, ref %d samples)", len(probe), len(ref))
	}

	var pMean float64
	for _, v := range probe {
		pMean += float64(v)
	}
	pMean /= float64(len(probe))
	var pNorm float64
	for _, v := range probe {
		d := float64(v) - pMean
		pNorm += d * d
	}
	pNorm = math.Sqrt(pNorm)

	bestLag, bestR := 0, math.Inf(-1)
	for lag := 0; lag+len(probe) <= len(ref); lag += 8 {
		var dot, wMean, wNorm float64
		for i := range probe {
			wMean += float64(ref[lag+i])
		}
		wMean /= float64(len(probe))
		for i := range probe {
			dw := float64(ref[lag+i]) - wMean
			dot += dw * (float64(probe[i]) - pMean)
			wNorm += dw * dw
		}
		if wNorm > 0 && pNorm > 0 {
			if r := dot / (math.Sqrt(wNorm) * pNorm); r > bestR {
				bestR, bestLag = r, lag
			}
		}
	}
	foundSec := winStart + float64(bestLag)/rate
	t.Logf("audio correlation: content at source t=%.3fs (plan %.3fs, r=%.3f)", foundSec, wantSec, bestR)
	return foundSec - wantSec
}

// decodeAudio renders mono float32 PCM at rate Hz; start/dur 0 = whole input.
func decodeAudio(t *testing.T, ctx context.Context, path string, startSec, durSec float64, rate int) []float32 {
	t.Helper()
	args := []string{"-hide_banner", "-v", "error", "-y"}
	if startSec > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", startSec))
	}
	if durSec > 0 {
		args = append(args, "-t", fmt.Sprintf("%.3f", durSec))
	}
	args = append(args, "-i", path, "-map", "0:a:0", "-ac", "1", "-ar", fmt.Sprint(rate), "-f", "f32le", "-")
	out, err := exec.CommandContext(ctx, "ffmpeg", args...).Output()
	if err != nil {
		t.Fatalf("decode audio %s: %v", path, err)
	}
	n := len(out) / 4
	pcm := make([]float32, n)
	for i := range n {
		pcm[i] = math.Float32frombits(binary.LittleEndian.Uint32(out[i*4 : i*4+4]))
	}
	return pcm
}

// TestAudioCopyIntegration: with an AAC source and AudioCopy set (the decision
// the handler makes for a client that decodes AAC), the served audio packets
// must be BIT-IDENTICAL to the source — `ffprobe -show_packets` data hashes
// prove a true passthrough, where "codec==aac" alone would also pass for a
// re-encode. Guards phase A0 of the audio chantier.
func TestAudioCopyIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not in PATH", bin)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	dir := t.TempDir()

	// AAC source (pink noise), B-frames + irregular keyframes like the main test.
	src := filepath.Join(dir, "itest-aac.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "anoisesrc=color=pink:seed=7:sample_rate=48000",
		"-t", "60",
		"-c:v", "libx264", "-preset", "ultrafast", "-bf", "3",
		"-force_key_frames", "0,10,13,23,26,36,39,49,52",
		"-sc_threshold", "0", "-g", "9999", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k", "-ac", "2",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("source generation: %v\n%s", err, out)
	}

	prober := NewProber("ffprobe")
	kf, err := prober.ProbeKeyframes(ctx, src)
	if err != nil || len(kf) < 5 {
		t.Fatalf("probe keyframes: %v (%d)", err, len(kf))
	}
	plan := apptranscoding.KeyframeSegments(kf, 60_000)

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
	)
	defer producer.Close()
	producer.SetCopyMode(apptranscoding.NewCopyPlanner(itestStreams{kf: kf}, itestItems{durMs: 60_000}), prober)

	fp := apptranscoding.DecisionFingerprint{
		MediaID:             "itest-aac-media",
		SourcePath:          src,
		VideoCodec:          "h264",
		AudioCodec:          "aac",
		SubtitleBurnInTrack: -1,
		UseFMP4:             true,
		// What the handler sets for a client whose audioCodecs include the
		// source codec: true audio passthrough.
		AudioCopy: true,
		Profiles:  []apptranscoding.ProfileFingerprint{{Name: "180p-copy-acopy", Width: 320, Height: 180, VideoBitrate: 1000, AudioBitrate: 192, Copy: true}},
	}
	hash := fp.Hash()
	if err := producer.EnsureSegmentRange(ctx, fp, 0, 1); err != nil {
		t.Fatal(err)
	}
	key := apptranscoding.SegmentKey{DecisionHash: hash, VariantIdx: 0, SegmentIdx: 1}
	if err := cache.Wait(ctx, key, true, 60*time.Second); err != nil {
		t.Fatal(err)
	}
	cacheDir, err := cache.DirFor(hash)
	if err != nil {
		t.Fatal(err)
	}

	// Served audio packet hashes (init+segment concatenated for a valid fMP4).
	clip := filepath.Join(dir, "served.mp4")
	initB, _ := os.ReadFile(filepath.Join(cacheDir, "stream_0_init.mp4"))
	segB, _ := os.ReadFile(filepath.Join(cacheDir, "stream_0_001.m4s"))
	if err := os.WriteFile(clip, append(initB, segB...), 0o644); err != nil {
		t.Fatal(err)
	}
	served := audioPacketHashes(t, ctx, clip)
	source := audioPacketHashes(t, ctx, src)
	if len(served) < 50 {
		t.Fatalf("too few served audio packets (%d)", len(served))
	}
	sourceSet := make(map[string]bool, len(source))
	for _, h := range source {
		sourceSet[h] = true
	}
	miss := 0
	for _, h := range served {
		if !sourceSet[h] {
			miss++
		}
	}
	// Passthrough ⇒ every served packet exists verbatim in the source. A
	// re-encode produces entirely different packet bytes (≈100% misses).
	if miss > 0 {
		t.Fatalf("%d/%d served audio packets are NOT source bytes — audio was re-encoded despite AudioCopy", miss, len(served))
	}
	_ = plan
}

// audioPacketHashes returns the md5 of every audio packet's data payload.
func audioPacketHashes(t *testing.T, ctx context.Context, path string) []string {
	t.Helper()
	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "packet=data_hash",
		"-show_data_hash", "md5",
		"-of", "csv=p=0",
		path,
	).Output()
	if err != nil {
		t.Fatalf("ffprobe packet hashes %s: %v", path, err)
	}
	var hashes []string
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ","))
		if line != "" {
			hashes = append(hashes, line)
		}
	}
	return hashes
}

// TestMultichannelAudioIntegration: a 5.1 source that must transcode (the
// decision's channel target = 6) is served as AAC 5.1 — not the historical
// stereo downmix. Channel COUNT is asserted on the served segment, and channel
// CONTENT via per-channel sines: FL carries 300 Hz and FR 500 Hz in the
// source; a downmix-upmix would bleed them together. Guards phase A1.
func TestMultichannelAudioIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not in PATH", bin)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	dir := t.TempDir()

	// 5.1 AC-3 source: one pure tone per channel (FL FR FC LFE BL BR).
	src := filepath.Join(dir, "itest-51.mkv")
	expr := "sin(2*PI*300*t)|sin(2*PI*500*t)|sin(2*PI*700*t)|sin(2*PI*60*t)|sin(2*PI*900*t)|sin(2*PI*1100*t)"
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "aevalsrc="+expr+":channel_layout=5.1:sample_rate=48000",
		"-t", "45",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "48", "-pix_fmt", "yuv420p",
		"-c:a", "ac3", "-b:a", "448k",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("source generation: %v\n%s", err, out)
	}

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
	)
	defer producer.Close()

	// What the decision produces for a 5.1 DTS/AC-3 source on a web client:
	// transcode to AAC with a 6-channel target (applyAudioChannelTarget).
	fp := apptranscoding.DecisionFingerprint{
		MediaID:             "itest-51-media",
		SourcePath:          src,
		VideoCodec:          "h264",
		AudioCodec:          "aac",
		MaxAudioChannels:    6,
		SubtitleBurnInTrack: -1,
		UseFMP4:             true,
		Profiles:            []apptranscoding.ProfileFingerprint{{Name: "180p-51", Width: 320, Height: 180, VideoBitrate: 800, AudioBitrate: 192}},
	}
	hash := fp.Hash()
	if err := producer.EnsureSegmentRange(ctx, fp, 0, 1); err != nil {
		t.Fatal(err)
	}
	key := apptranscoding.SegmentKey{DecisionHash: hash, VariantIdx: 0, SegmentIdx: 1}
	if err := cache.Wait(ctx, key, true, 90*time.Second); err != nil {
		t.Fatal(err)
	}
	cacheDir, err := cache.DirFor(hash)
	if err != nil {
		t.Fatal(err)
	}
	clip := filepath.Join(dir, "served51.mp4")
	initB, _ := os.ReadFile(filepath.Join(cacheDir, "stream_0_init.mp4"))
	segB, _ := os.ReadFile(filepath.Join(cacheDir, "stream_0_001.m4s"))
	if err := os.WriteFile(clip, append(initB, segB...), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Channel count: must be 6.
	chOut, err := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-select_streams", "a:0", "-show_entries", "stream=channels",
		"-of", "csv=p=0", clip).Output()
	if err != nil {
		t.Fatalf("ffprobe channels: %v", err)
	}
	if got := strings.TrimSpace(string(chOut)); got != "6" {
		t.Fatalf("served audio has %s channels, want 6 (stereo downmix regression)", got)
	}

	// 2. Channel content: FL must be the 300 Hz tone, distinct from FR (500 Hz).
	chanPCM := func(spec string) []float32 {
		out, err := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
			"-i", clip, "-map", "0:a:0",
			"-af", "pan=mono|c0="+spec,
			"-ar", "8000", "-f", "f32le", "-").Output()
		if err != nil {
			t.Fatalf("decode channel %s: %v", spec, err)
		}
		n := len(out) / 4
		pcm := make([]float32, n)
		for i := range n {
			pcm[i] = math.Float32frombits(binary.LittleEndian.Uint32(out[i*4 : i*4+4]))
		}
		return pcm
	}
	dominant := func(pcm []float32, freq float64) float64 {
		// Goertzel-style projection at freq, normalized by signal energy.
		var re, im, energy float64
		for i, v := range pcm {
			ph := 2 * math.Pi * freq * float64(i) / 8000
			re += float64(v) * math.Cos(ph)
			im += float64(v) * math.Sin(ph)
			energy += float64(v) * float64(v)
		}
		if energy == 0 {
			return 0
		}
		return (re*re + im*im) / (energy * float64(len(pcm)))
	}
	fl := chanPCM("FL")
	if len(fl) < 8000 {
		t.Fatalf("FL decode too short: %d samples", len(fl))
	}
	at300 := dominant(fl, 300)
	at500 := dominant(fl, 500)
	// A PURE tone projects to exactly 0.5 under this normalization
	// (|Σ e^{-jωn}·sin(ωn)|² = (N/2)², energy = N/2 → 0.5), so ≥0.4 means
	// "FL is essentially the 300 Hz tone" and ≤0.1 at 500 Hz means no FR bleed.
	if at300 < 0.4 || at500 > 0.1 {
		t.Fatalf("FL channel content wrong (proj 300Hz=%.2f, 500Hz=%.2f) — channels were mixed (downmix/upmix regression)", at300, at500)
	}
}
