package ffmpeg

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/hdr"
	"github.com/evey-project/evey/internal/pkg/procfork"
)

// A GPU tone-mapper can exit 0 at full speed and still draw the wrong picture:
// the QSV VPP curve washes the highlights out and halves the saturation (UHD
// 630, 2026-09-09), and nothing in the filter probe or the route proof looks
// at a pixel. This gate does, for every GPU tone-map implementation the
// calibration probes (VAAPI VPP, OpenCL, Vulkan/libplacebo, CUDA): it runs the
// production chain on a small HDR10 mire, pulls frames back, runs the SOFTWARE
// reference (the tonemapx/zscale chain the CPU route serves) on the same
// frames, and compares means and structure - the same decode-back-and-compare
// shape as the encoder gate in hw_output_validation.go, with the colour stats
// that gate does not need.
//
// Thresholds come from measurements on an RTX 4070 Ti (2026-09-13, tonemap_cuda
// vs tonemapx), not from taste:
//   - on REAL HDR10 titles the two agree to PSNR 41-47 dB / SSIM >= 0.993;
//   - on a full-range synthetic testsrc2 (10-bit codes up to 1023 = 10 000 nits)
//     they DISAGREE by 15 dB (luma ratio 1.49) - correct implementations
//     diverge on signal no film carries, so that mire is useless as a gate;
//   - on a testsrc2 whose codes are scaled into the range real content lives
//     in (tonemapGateLevel) the correct CUDA sits at luma ratio 1.03-1.08 and
//     saturation ratio 0.99-1.02, while the failure shapes sit far away:
//     no tone-map at all 0.71-0.81 / 0.63-0.71, a washed half-saturated output
//     (the VPP symptom) 1.15-1.22 / 0.50, a 25 % desaturation 1.00 / 0.75.
//
// Hence the bounds below. They separate the populations with margin on this
// rig; every verdict is logged with its numbers so a host that lands between
// them is visible, and - as everywhere in the calibration - only a MEASURED
// divergence refuses an implementation: a gate that cannot conclude keeps it.
const (
	tonemapGateLumaMin = 0.85
	tonemapGateLumaMax = 1.15
	tonemapGateSatMin  = 0.80
	tonemapGateSatMax  = 1.25
	// tonemapGateMinSat is the reference mean saturation under which the ratio
	// is noise (a dark scene measured 2.8 vs 3.4 on real content, a 1.21 ratio
	// that means nothing) and the saturation check is skipped.
	tonemapGateMinSat = 8.0
	// tonemapGateMinLuma: below this mean the output is black, not tone-mapped.
	tonemapGateMinLuma = 16.0
	// tonemapGateLevel scales the mire's 10-bit codes into the band real HDR
	// content lives in (a 0.40 factor measured; 0.45 keeps the picture bright
	// enough for the saturation check to be meaningful).
	tonemapGateLevel = 0.45
)

// tonemapGateOffsets are the seconds sampled from the 2 s mire.
var tonemapGateOffsets = []float64{0.5, 1.5}

// tonemapVerdict is one implementation's measured agreement with the reference.
type tonemapVerdict struct {
	OK bool
	// Worst-case ratios over the sampled frames (implementation / reference).
	LumaRatio, SatRatio float64
	WorstHamming        int
	// SatChecked is false when the reference is too desaturated to judge.
	SatChecked bool
}

func (v tonemapVerdict) String() string {
	return fmt.Sprintf("luma_ratio=%.3f sat_ratio=%.3f sat_checked=%v hamming=%d", v.LumaRatio, v.SatRatio, v.SatChecked, v.WorstHamming)
}

// tonemapGateMirePath sits next to the bench samples.
func (r *BenchmarkRunner) tonemapGateMirePath() string {
	return filepath.Join(filepath.Dir(r.samplePath), "tonemap-gate-hdr10.hevc")
}

// ensureTonemapGateMire encodes the gate's clip once: 1280x720, 24 fps, 2 s,
// the bench's testsrc2+blur+noise picture, its 10-bit codes scaled by
// tonemapGateLevel (limited range: luma from 64, chroma from 512), HEVC Main10
// tagged bt2020/PQ like the HDR bench sample. Fail-closed: no mire, no gate.
func (r *BenchmarkRunner) ensureTonemapGateMire(ctx context.Context) (string, bool) {
	dst := r.tonemapGateMirePath()
	if st, err := os.Stat(dst); err == nil && st.Size() > 0 {
		return dst, true
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", false
	}
	genCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	lut := fmt.Sprintf("lutyuv=y=(val-64)*%.2f+64:u=(val-512)*%.2f+512:v=(val-512)*%.2f+512", tonemapGateLevel, tonemapGateLevel+0.1, tonemapGateLevel+0.1)
	args := []string{"-hide_banner", "-v", "error", "-nostdin", "-y",
		"-f", "lavfi", "-i", "testsrc2=duration=2:size=1280x720:rate=24,gblur=sigma=1.5,noise=alls=4:allf=t+u,format=yuv420p10le," + lut,
		"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le", "-tag:v", "hvc1", "-an",
		"-x265-params", "hdr10=1:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc",
		"-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc",
		dst}
	if out, err := procfork.CombinedOutput(genCtx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.tonemap_gate_mire"}, r.ffmpegBin(), args...); err != nil {
		r.logger.Warn("benchmark: tone-map gate mire generation failed", slog.Any("err", err), slog.String("tail", tailString(string(out), 200)))
		return "", false
	}
	return dst, true
}

// validateTonemapImpl measures one GPU tone-map chain against the software
// reference on the gate mire. chain is the exact filter the builder emits for
// the implementation (hwTonemapFilter's output for a given feature set),
// initArgs the backend's production hwaccel prologue on the device. An error
// means the gate could not conclude (mire, extraction, decode); a verdict with
// OK=false is a measured divergence.
func (r *BenchmarkRunner) validateTonemapImpl(ctx context.Context, backend, device, chain string) (tonemapVerdict, error) {
	mire, ok := r.ensureTonemapGateMire(ctx)
	if !ok {
		return tonemapVerdict{}, fmt.Errorf("tonemap gate: no mire")
	}
	runCtx, cancel := context.WithTimeout(ctx, 4*hwProbeTimeout)
	defer cancel()
	tmpDir, err := os.MkdirTemp("", "evey-tonemap-gate-*")
	if err != nil {
		return tonemapVerdict{}, fmt.Errorf("tonemap gate: temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Reference: the CPU route's own chain (tonemapx when the build has it, the
	// zscale chain otherwise), at the mire's resolution.
	refOpts := hdr.DefaultToneMapOptions()
	refOpts.UseTonemapx = tonemapxAvailable(r.ffmpegPath)
	refFilter := hdr.BuildToneMapFilter(&hdr.HDRInfo{IsHDR: true}, refOpts)
	gpuFilter := chain + ",hwdownload,format=nv12"
	initArgs := hwInitArgs(transcoding.HWAccelType(backend), device)

	v := tonemapVerdict{OK: true, LumaRatio: 1, SatRatio: 1}
	for i, off := range tonemapGateOffsets {
		ref, err := r.extractFrameImage(runCtx, []string{"-i", mire}, refFilter, off, filepath.Join(tmpDir, fmt.Sprintf("ref_%d.png", i)))
		if err != nil {
			return tonemapVerdict{}, fmt.Errorf("tonemap gate: reference frame at %.1fs: %w", off, err)
		}
		got, err := r.extractFrameImage(runCtx, append(append([]string{}, initArgs...), "-i", mire), gpuFilter, off, filepath.Join(tmpDir, fmt.Sprintf("gpu_%d.png", i)))
		if err != nil {
			return tonemapVerdict{}, fmt.Errorf("tonemap gate: GPU frame at %.1fs: %w", off, err)
		}
		v = foldTonemapVerdict(v, frameStatsOf(ref), frameStatsOf(got), pHashOf(ref), pHashOf(got))
	}
	return v, nil
}

// frameStats are the per-frame means the gate compares.
type frameStats struct{ meanY, meanSat float64 }

// frameStatsOf: mean luma and mean chroma magnitude (BT.601 Y/Cb/Cr from the
// PNG's RGB, 8-bit units - the same quantities ffmpeg's signalstats YAVG and
// SATAVG report, which the thresholds were measured with).
func frameStatsOf(img image.Image) frameStats {
	b := img.Bounds()
	var sumY, sumSat float64
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			rf, gf, bf := float64(cr>>8), float64(cg>>8), float64(cb>>8)
			yy := 0.299*rf + 0.587*gf + 0.114*bf
			cbv := -0.168736*rf - 0.331264*gf + 0.5*bf
			crv := 0.5*rf - 0.418688*gf - 0.081312*bf
			sumY += yy
			sumSat += math.Hypot(cbv, crv)
			n++
		}
	}
	if n == 0 {
		return frameStats{}
	}
	return frameStats{meanY: sumY / float64(n), meanSat: sumSat / float64(n)}
}

func pHashOf(img image.Image) uint64 {
	b := img.Bounds()
	return pHashDCT(img, b.Min.X, b.Min.Y, b.Max.X, b.Max.Y)
}

// foldTonemapVerdict applies the gate's bounds to one frame pair and keeps the
// worst case across frames. Pure, so the thresholds are unit-tested against the
// measured populations.
func foldTonemapVerdict(v tonemapVerdict, ref, got frameStats, refHash, gotHash uint64) tonemapVerdict {
	if ref.meanY < tonemapGateMinLuma || got.meanY < tonemapGateMinLuma {
		v.OK = false // black (or the mire itself is): not a tone-mapped picture
	}
	if ref.meanY > 0 {
		lr := got.meanY / ref.meanY
		if math.Abs(lr-1) > math.Abs(v.LumaRatio-1) {
			v.LumaRatio = lr
		}
		if lr < tonemapGateLumaMin || lr > tonemapGateLumaMax {
			v.OK = false
		}
	}
	if ref.meanSat >= tonemapGateMinSat {
		v.SatChecked = true
		sr := got.meanSat / ref.meanSat
		if math.Abs(sr-1) > math.Abs(v.SatRatio-1) {
			v.SatRatio = sr
		}
		if sr < tonemapGateSatMin || sr > tonemapGateSatMax {
			v.OK = false
		}
	}
	if d := hammingDistance(refHash, gotHash); d > v.WorstHamming {
		v.WorstHamming = d
	}
	if v.WorstHamming >= hwCorruptHammingThreshold {
		v.OK = false
	}
	return v
}

// extractFrameImage decodes one frame at offset through filter and returns it
// (PNG on disk for the log, image in memory for the stats). Output-side -ss,
// like extractFrameHash, so both sides land on the same picture.
func (r *BenchmarkRunner) extractFrameImage(ctx context.Context, inputArgs []string, filter string, offset float64, dst string) (image.Image, error) {
	args := []string{"-hide_banner", "-nostats", "-v", "error", "-nostdin"}
	args = append(args, inputArgs...)
	args = append(args, "-vf", filter+",format=rgb24", "-ss", fmt.Sprintf("%.3f", offset), "-frames:v", "1", "-f", "image2", "-y", dst)
	if out, err := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Probe, Site: "hw_validate.tonemap_frame"}, r.ffmpegPath, args...); err != nil {
		return nil, fmt.Errorf("frame extract: %w (%s)", err, tailString(strings.TrimSpace(string(out)), 200))
	}
	f, err := os.Open(dst)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// tonemapImplsToGate lists, for a backend's probed tone-map facts, each
// implementation the builder may emit with the feature set that selects it
// (hwTonemapFilter's own precedence), so every certified curve is measured, not
// only the one the rule currently prefers.
func tonemapImplsToGate(backend string, nativeOK, openclOK, vulkanOK bool) []struct {
	name     string
	features []string
} {
	var out []struct {
		name     string
		features []string
	}
	add := func(name string, features []string) {
		out = append(out, struct {
			name     string
			features []string
		}{name, features})
	}
	switch backend {
	case "vaapi":
		if openclOK {
			add(transcoding.FeatureTonemapOpenCL, []string{transcoding.FeatureTonemapOpenCL})
		}
		if vulkanOK {
			add(transcoding.FeatureTonemapVulkan, []string{transcoding.FeatureTonemapVulkan})
		}
		if nativeOK {
			add("tonemap_vaapi", nil)
		}
	case "nvenc":
		if nativeOK {
			add(transcoding.FeatureTonemapCUDA, []string{transcoding.FeatureTonemapCUDA})
		}
	case "rkmpp":
		if openclOK {
			add(transcoding.FeatureTonemapOpenCL, []string{transcoding.FeatureTonemapOpenCL})
		}
	}
	return out
}

// tonemapGateEnforced says whether a measured divergence REFUSES the
// implementation (true) or is only logged (false). Enforced for the
// implementations whose agreement with the reference was measured on real
// hardware while these bounds were set (CUDA on the RTX 4070 Ti) and for the
// new Vulkan/libplacebo route, which ships with no other proof; the OpenCL and
// VPP curves that production already validated by eye run in shadow until a
// host has logged their numbers - refusing the route prod-host serves HDR with,
// on a threshold it never measured, is the one regression this gate must not
// cause. Flip an entry to true once its numbers are on record.
var tonemapGateEnforced = map[string]bool{
	transcoding.FeatureTonemapVulkan: true,
	transcoding.FeatureTonemapCUDA:   true,
	transcoding.FeatureTonemapOpenCL: false,
	"tonemap_vaapi":                  false,
}

// gateTonemapImpls runs the gate on every probed implementation of a backend
// and returns the probe booleans with the measured failures removed (when
// enforced). Inconclusive gates keep the implementation, logged.
func (r *BenchmarkRunner) gateTonemapImpls(ctx context.Context, backend, device string, nativeOK, openclOK, vulkanOK bool) (bool, bool, bool) {
	for _, impl := range tonemapImplsToGate(backend, nativeOK, openclOK, vulkanOK) {
		chain := hwTonemapFilter(transcoding.HWAccelType(backend), impl.features)
		if chain == "" {
			continue
		}
		v, err := r.validateTonemapImpl(ctx, backend, device, chain)
		attrs := []any{slog.String("backend", backend), slog.String("device", device), slog.String("impl", impl.name)}
		if err != nil {
			r.logger.Warn("benchmark: tone-map correctness gate inconclusive - implementation kept on its probe alone", append(attrs, slog.Any("err", err))...)
			continue
		}
		attrs = append(attrs, slog.String("measured", v.String()), slog.Bool("enforced", tonemapGateEnforced[impl.name]))
		if v.OK {
			r.logger.Info("benchmark: tone-map correctness gate passed", attrs...)
			continue
		}
		if !tonemapGateEnforced[impl.name] {
			r.logger.Warn("benchmark: tone-map correctness gate DIVERGES (shadow: implementation kept, numbers recorded)", attrs...)
			continue
		}
		r.logger.Warn("benchmark: tone-map correctness gate DIVERGES - implementation NOT certified", attrs...)
		switch impl.name {
		case transcoding.FeatureTonemapOpenCL:
			openclOK = false
		case transcoding.FeatureTonemapVulkan:
			vulkanOK = false
		default:
			nativeOK = false
		}
	}
	return nativeOK, openclOK, vulkanOK
}
