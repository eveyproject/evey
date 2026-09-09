package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/pkg/procfork"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// benchDecodeCodecs are the source codecs the bench measures DECODE throughput
// for. The cost model interpolates within a codec but never across, so each
// codec it prices must be measured. H.264 + HEVC are what a real library holds
// (63%/37% on the production NAS); AV1 falls back to output-rung pricing.
var benchDecodeCodecs = []struct {
	codec   string
	encoder string
}{
	{codec: "hevc", encoder: "libx265"},
	{codec: "h264", encoder: "libx264"},
}

// expectedSampleCodec is what generateSample produces and what the filename
// claims. Anything else must be recorded and surfaced, never assumed.
const expectedSampleCodec = "hevc"

// cachedSampleIsSound re-validates a sample already cached at the default path,
// which resolveBenchSample used to trust on existence alone. That is how prod
// spent two months calibrating on a hand-placed H.264 clip named .hevc.
//
// Only a clip we generated is regenerated: an operator-configured override or
// SHA-pinned URL is deliberate content, reported and kept.
func (r *BenchmarkRunner) cachedSampleIsSound(ctx context.Context, label string, src benchSampleSource, path string) bool {
	// An empty file needs no probe to be condemned, and MUST be condemned here,
	// because the probe fails on it - which the "no verdict is not a guilty
	// verdict" rule below then reads as innocence. Prod, 2026-09-09:
	// sample-hdr10.hevc was 0 bytes and kept on exactly that path, so
	// software_hdr_max_height was benched against nothing at all.
	if st, err := os.Stat(path); err == nil && st.Size() == 0 {
		r.logger.Warn("benchmark: cached "+label+" sample is empty, regenerating it",
			slog.String("path", path))
		return false
	}
	probed, ok := r.probeSourceClass(ctx, path)
	if !ok {
		// No verdict is not a guilty verdict: deleting a cached clip because the
		// probe itself failed (no ffprobe, a transient error) would be destroying
		// evidence on no evidence. Keep it and let the bench report what it can.
		return true
	}
	if strings.EqualFold(probed.Codec, expectedSampleCodec) {
		return true
	}
	if src.url != "" {
		r.logger.Warn("benchmark: the pinned "+label+" reference clip is not "+expectedSampleCodec+
			"; keeping it (operator-configured) and calibrating against what it is",
			slog.String("path", path),
			slog.String("actual_codec", probed.Codec))
		return true
	}
	r.logger.Warn("benchmark: cached "+label+" sample is the wrong codec, regenerating it",
		slog.String("path", path),
		slog.String("expected_codec", expectedSampleCodec),
		slog.String("actual_codec", probed.Codec),
		slog.Int("height", probed.Height))
	return false
}

// ffprobeBin derives the ffprobe path from the configured ffmpeg one - the two
// always ship together, in the same directory, in every image this runs in.
func (r *BenchmarkRunner) ffprobeBin() string {
	bin := r.ffmpegBin()
	dir, file := filepath.Split(bin)
	return filepath.Join(dir, strings.Replace(file, "ffmpeg", "ffprobe", 1))
}

// probeSourceClass reads the real codec and height of path's first video
// stream - the only place the bench looks at what a sample actually IS.
func (r *BenchmarkRunner) probeSourceClass(ctx context.Context, path string) (transcoding.SourceClass, bool) {
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := procfork.Output(probeCtx, procfork.Options{Class: procfork.Probe, Site: "benchmark.probe_sample"}, r.ffprobeBin(),
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name,height",
		"-of", "json",
		path,
	)
	if err != nil {
		r.logger.Warn("benchmark: could not probe the bench sample's codec",
			slog.String("path", path), slog.Any("err", err))
		return transcoding.SourceClass{}, false
	}
	var parsed struct {
		Streams []struct {
			CodecName string `json:"codec_name"`
			Height    int    `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil || len(parsed.Streams) == 0 {
		r.logger.Warn("benchmark: unreadable probe output for the bench sample",
			slog.String("path", path), slog.Any("err", err))
		return transcoding.SourceClass{}, false
	}
	s := parsed.Streams[0]
	if s.CodecName == "" || s.Height <= 0 {
		return transcoding.SourceClass{}, false
	}
	return transcoding.SourceClass{Codec: s.CodecName, Height: s.Height}, true
}

// benchDecodeFPS measures DECODE-only throughput: unpack frames, drop them,
// count per wall-clock second. No encoder, no scaler.
func (r *BenchmarkRunner) benchDecodeFPS(ctx context.Context, path string) (float64, bool) {
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(r.durationSec*3)*time.Second+10*time.Second)
	defer cancel()
	cmd, finish, err := procfork.Command(runCtx, procfork.Options{Class: procfork.Production, Site: "benchmark.decode", UnboundedThreads: true}, r.ffmpegBin(),
		"-hide_banner", "-nostats",
		"-t", strconv.Itoa(r.durationSec),
		"-i", path,
		"-an", "-sn",
		"-f", "null", "-",
	)
	if err != nil {
		return 0, false
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	startedAt := time.Now()
	err = finish(cmd.Run(), stderr.Bytes())
	wall := time.Since(startedAt).Seconds()
	if err != nil || wall <= 0 {
		r.logger.Debug("benchmark: decode probe failed",
			slog.String("path", path), slog.Any("err", err),
			slog.String("stderr_tail", tailString(stderr.String(), 300)))
		return 0, false
	}
	matches := frameRE.FindAllStringSubmatch(stderr.String(), -1)
	if len(matches) == 0 {
		return 0, false
	}
	last := matches[len(matches)-1]
	if len(last) < 2 {
		return 0, false
	}
	frames, err := strconv.Atoi(last[1])
	if err != nil || frames <= 0 {
		return 0, false
	}
	return float64(frames) / wall, true
}

// companionSamplePath sits next to the SDR sample, codec in the name.
func (r *BenchmarkRunner) companionSamplePath(codec string) string {
	return filepath.Join(filepath.Dir(r.samplePath), "sample-decode-"+codec+".mkv")
}

// ensureCompanionSample transcodes the SDR sample into codec once and caches it,
// so decode can be measured for a codec the sample is not in. Same content and
// resolution, so the measurements differ ONLY by codec. Encoded ultrafast: this
// clip is decoded, never judged on quality.
func (r *BenchmarkRunner) ensureCompanionSample(ctx context.Context, src, encoder, codec string) (string, bool) {
	dst := r.companionSamplePath(codec)
	if _, err := os.Stat(dst); err == nil {
		return dst, true
	}
	genCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	if out, err := procfork.CombinedOutput(genCtx, procfork.Options{Class: procfork.Production, Site: "benchmark.companion_sample", UnboundedThreads: true}, r.ffmpegBin(),
		"-hide_banner", "-nostats", "-y",
		"-t", strconv.Itoa(r.durationSec),
		"-i", src,
		"-c:v", encoder, "-preset", "ultrafast", "-b:v", "5M",
		"-an", "-sn",
		dst,
	); err != nil {
		r.logger.Warn("benchmark: could not build the decode companion sample",
			slog.String("codec", codec), slog.Any("err", err),
			slog.String("tail", tailString(string(out), 200)))
		return "", false
	}
	return dst, true
}

// decodeEvidence measures decode throughput per codec and reports which source
// class the ENCODE rungs were measured against - probed from the sample, never
// inferred from its name, since the cost model subtracts it to isolate each
// rung's encode term.
//
// Best-effort: less evidence simply means the cost model prices on the output
// rung as before. Skipped on the synthetic fallback, whose decode cost is not a
// real source's.
func (r *BenchmarkRunner) decodeEvidence(ctx context.Context, sample string, fallback bool) (transcoding.SourceClass, []transcoding.DecodeRungSample) {
	if fallback || sample == "" {
		return transcoding.SourceClass{}, nil
	}
	benchSource, ok := r.probeSourceClass(ctx, sample)
	if !ok {
		return transcoding.SourceClass{}, nil
	}
	if !strings.EqualFold(benchSource.Codec, expectedSampleCodec) {
		// Not fatal (an operator may point the bench at their own content) but it
		// decides how every stream here is priced, so it is stated out loud.
		r.logger.Warn("benchmark: the bench sample is not the codec its name implies; capacity is calibrated against what it ACTUALLY is",
			slog.String("path", sample),
			slog.String("expected_codec", expectedSampleCodec),
			slog.String("actual_codec", benchSource.Codec),
			slog.Int("height", benchSource.Height))
	}

	samples := make([]transcoding.DecodeRungSample, 0, len(benchDecodeCodecs))
	for _, want := range benchDecodeCodecs {
		path := sample
		if !strings.EqualFold(benchSource.Codec, want.codec) {
			companion, built := r.ensureCompanionSample(ctx, sample, want.encoder, want.codec)
			if !built {
				continue
			}
			path = companion
		}
		fps, measured := r.benchDecodeFPS(ctx, path)
		if !measured {
			continue
		}
		samples = append(samples, transcoding.DecodeRungSample{
			Codec:  want.codec,
			Height: benchSource.Height,
			FPS:    fps,
		})
		r.logger.Info("benchmark: decode probed",
			slog.String("codec", want.codec),
			slog.Int("height", benchSource.Height),
			slog.String("fps", fmt.Sprintf("%.1f", fps)))
	}
	return benchSource, samples
}
