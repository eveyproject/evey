package ffmpeg

import (
	"fmt"
	"slices"
	"strings"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/hdr"
)

// hls_args_hw.go groups the hardware-accelerated ffmpeg argument builders
// (tone-mapping, scale/deinterlace filters, HW encoder args and init).

// resolveToneMap returns the HDR->SDR tonemap filter for this transcode, or "".
// It honours two signals: the explicit ToneMap decision baked into the opts by
// the orchestrated VOD path (which does not thread real Streams), and the
// stream-based detection used when real Streams ARE present (legacy/direct path).
// A client that renders HDR natively (HDRSupported) is never tonemapped. The
// downscale to dstWidth is folded into the tonemap's linearizing zscale so the
// heavy float pipeline runs at delivery resolution (see hdr.ToneMapOptions.
// DstWidth); dstWidth<=0 keeps a full-resolution tonemap (the caller scales
// separately, e.g. the subtitle burn-in path).
func resolveToneMap(opts transcoding.HLSProcessOpts, dstWidth int) string {
	if opts.HDRSupported {
		return ""
	}
	if opts.ToneMap {
		// Generic HDR10/HLG -> SDR (bt709) tonemap. The filter (zscale linearize
		// -> tonemap -> bt709) reads the input transfer/primaries from the decoded
		// frame's colour metadata, so it needs no per-stream params here.
		return buildToneMapFilter(&hdr.HDRInfo{IsHDR: true}, dstWidth)
	}
	return detectToneMap(opts.HDRSupported, opts.Streams, dstWidth)
}

func detectToneMap(hdrSupported bool, streams []transcoding.SourceStream, dstWidth int) string {
	if hdrSupported {
		return ""
	}
	hdrStreams := make([]hdr.VideoStream, len(streams))
	for i, st := range streams {
		hdrStreams[i] = st
	}
	hdrInfo := hdr.DetectHDR(hdrStreams)
	if hdrInfo.IsHDR {
		return buildToneMapFilter(hdrInfo, dstWidth)
	}
	return ""
}

// buildToneMapFilter builds the tonemap filter string for the given HDR info,
// scaled to dstWidth and using tonemapx when the ffmpeg build provides it.
func buildToneMapFilter(info *hdr.HDRInfo, dstWidth int) string {
	tmOpts := hdr.DefaultToneMapOptions()
	tmOpts.DstWidth = dstWidth
	tmOpts.UseTonemapx = tonemapxAvailable("ffmpeg")
	return hdr.BuildToneMapFilter(info, tmOpts)
}

// --- hardware-accelerated transcode path ---
//
// v1 handles the "simple" filtergraph: a zero-copy decode → GPU deinterlace
// (no-op on progressive, NVENC/VAAPI only) → GPU scale → GPU encode pipeline.
// HDR tonemapping and bitmap-subtitle burn-in still take the software path
// (their GPU filtergraphs are backend-specific and depend on optional ffmpeg
// filters that minimal/NAS builds may lack). The orchestrated VOD path
// currently always hits this simple case (Streams=nil, SubtitleCodec unset ⇒ no
// tonemap/burn-in), so v1 covers 100% of live VOD transcodes.
//
// The arg shapes here are validated end-to-end on real hardware (NVENC on an
// RTX 4070 Ti, VAAPI on a Ryzen iGPU): scale_cuda/scale_vaapi both accept a
// `-2` auto-height, split fans out GPU surfaces, and the *_nvenc/*_vaapi
// encoders consume them with no hwdownload round-trip.

// hwUsable reports whether the hardware encode path applies to this job. The
// wiring layer only sets HWAccel to a non-None value after confirming the flag
// is on and a detected device supports the codec; this gate additionally keeps
// the complex-filtergraph cases (tonemap, burn-in) on the proven software path.
func hwUsable(opts transcoding.HLSProcessOpts, burnIn bool, toneMap string) bool {
	if opts.HWAccel == "" || opts.HWAccel == transcoding.None || !opts.HWAccel.IsValid() {
		return false
	}
	// A burn-in normally kept the software path - and that single rule sent a 4K
	// HDR session's decode, tone-map and every encode to the CPU because the
	// container flagged a forced subtitle DEFAULT (prod, 2026-09-09). Same
	// exception shape as the tone-map below: the host probed a working GPU
	// compositor (opts.HWOverlay, set only when the calibration confirmed it) AND
	// the backend has one wired. Otherwise software, exactly as before.
	if burnIn && (!opts.HWOverlay || hwOverlayFilter(opts.HWAccel) == "") {
		return false
	}
	// A decoupled session needs an upload chain for its backend, and it never
	// tone-maps on the GPU: its upload folds to 8-bit nv12 on the CPU, which is
	// precisely the depth the HW tone-mapper needs to still have. Both are
	// resolver-side gates already; they are repeated here for the reason the
	// tone-map one is - the builder must never emit a pipeline it cannot
	// express, whatever the caller believed.
	// The device check is not redundant with the upload one: VAAPI without a
	// render node binds nothing, and hwupload would then have no device to
	// target - a graph that fails at spawn rather than falling back.
	if opts.HWSoftwareDecode && (hwUploadFilter(opts.HWAccel) == "" ||
		len(hwDeviceOnlyArgs(opts.HWAccel, opts.HWDevice)) == 0 || toneMap != "") {
		return false
	}
	// A tone-map normally keeps the software path. The exception: the host probed
	// GPU tone-mapping AND it's wired into this request (opts.HWTonemap) AND the
	// backend has a HW tone-map filter - then the GPU folds HDR->SDR in-pipeline.
	if toneMap != "" {
		if opts.HWTonemap && hwTonemapFilter(opts.HWAccel, opts.HWFeatures) != "" {
			return true
		}
		return hwTonemapBridge(opts.HWAccel, toneMap, "") != ""
	}
	return true
}

// hwTonemapBridge tone-maps on the CPU between a GPU decode and a GPU encode,
// for backends without a GPU tone-mapper (AMD VAAPI, an NVENC on a stock
// ffmpeg without tonemap_cuda, an Intel without its ICD): download the decoded
// surface, run the software chain, upload the SDR frames. Measured 4K at 3.1x
// on a Ryzen 9900X + RTX 4070 Ti.
//
// deint is the SOFTWARE deinterlacer this session would run (deinterlaceFilterFor),
// placed right after the download: the GPU deinterlacer must NOT sit in front of
// hwdownload. yadif_cuda / deinterlace_vaapi=auto pass a PROGRESSIVE frame
// through untouched - still owned by the decoder's frame pool - and hwdownload,
// configured against the deinterlacer's own pool, refuses it ("Input frame is
// not the in the configured hwframe context", exit 234). Measured 2026-09-12 on
// an RTX 4070 Ti: every progressive HDR title failed on this route; an
// interlaced-flagged clip or deint=all passed. bwdif deint=interlaced on the
// CPU is the same no-op on progressive content the software path relies on.
func hwTonemapBridge(accel transcoding.HWAccelType, toneMap, deint string) string {
	if toneMap == "" {
		return ""
	}
	// One format, not a list: a p010le|nv12 alternative broke the CUDA negotiation
	// (verified on an RTX 4070 Ti); an 8-bit HDR source converts on the way down.
	pre := "hwdownload,format=p010le,"
	if deint != "" {
		pre += deint + ","
	}
	if !strings.Contains(toneMap, "tonemapx") {
		pre += "format=yuv420p10le|yuv420p," // the zscale chain is planar-only
	}
	switch accel {
	case transcoding.VAAPI:
		return pre + toneMap + ",hwupload"
	case transcoding.QSV:
		return pre + toneMap + ",hwupload=extra_hw_frames=16,format=qsv"
	case transcoding.NVENC:
		return pre + toneMap + ",hwupload_cuda"
	}
	return ""
}

// hwBridges reports whether this job tone-maps on the CPU between the GPU
// decode and encode (no GPU tone-mapper probed/wired for it).
func hwBridges(opts transcoding.HLSProcessOpts) bool {
	if resolveToneMap(opts, 0) == "" {
		return false
	}
	return !opts.HWTonemap || hwTonemapFilter(opts.HWAccel, opts.HWFeatures) == ""
}

// hwToneMapStage is what runs before the GPU scaler(s) on an HDR->SDR job: the
// GPU tone-mapper when probed and wanted, else the CPU bridge; "" when none.
func hwToneMapStage(opts transcoding.HLSProcessOpts) string {
	toneMap := resolveToneMap(opts, 0)
	if toneMap == "" {
		return ""
	}
	if opts.HWTonemap {
		if tm := hwTonemapFilter(opts.HWAccel, opts.HWFeatures); tm != "" {
			return tm
		}
	}
	return hwTonemapBridge(opts.HWAccel, toneMap, deinterlaceFilterFor(opts))
}

// hwTonemapFilter returns the GPU HDR->SDR tone-map filter for the backend, or
// "" when no HW tone-mapper is wired. Inserted before the scaler so the GPU
// folds HDR (and 10-bit) in one pass. The caller gates on opts.HWTonemap, set
// only when the calibration probed the filter AND proved the builder route.
func hwTonemapFilter(accel transcoding.HWAccelType, features []string) string {
	switch accel {
	case transcoding.VAAPI:
		// OpenCL when probed: bt2390 like the software path, RPU/HLG aware. The
		// fixed-function VPP curve is the fallback.
		if slices.Contains(features, transcoding.FeatureTonemapOpenCL) {
			return hwOpenCLTonemapChain
		}
		return "tonemap_vaapi=format=nv12:matrix=bt709:primaries=bt709:transfer=bt709"
	case transcoding.NVENC:
		// tonemap_cuda on the NVDEC frames (jellyfin-ffmpeg): same curve as
		// OpenCL, RPU/HLG aware, zero-copy into scale_cuda. Only when probed -
		// a stock ffmpeg lacks the filter and keeps the CPU bridge.
		if slices.Contains(features, transcoding.FeatureTonemapCUDA) {
			return hwCUDATonemapChain
		}
		return ""
	case transcoding.QSV:
		// vpp_qsv=tonemap washes out the highlights and halves saturation (exit 0,
		// wrong pixels - proven on an UHD 630, 2026-09-09), and no visual gate
		// certifies it. It stays INELIGIBLE: a QSV host tone-maps on the CPU
		// bridge (correct, slower) or in software until a gate passes. Returning
		// "" here also keeps the tonemap route out of the calibration's HWRoutes
		// (routeProbesFor gates on this), so HWTonemapWanted never selects it.
		return ""
	}
	return ""
}

// burnInRequested reports whether this job composites a bitmap subtitle onto the
// video. Single definition on purpose: "should we burn" and "can the GPU burn"
// must never be able to disagree, or a job would take the HW path and then be
// built without its subtitle.
func burnInRequested(opts transcoding.HLSProcessOpts) bool {
	return opts.SubtitleTrackIndex >= 0 && IsImageSubtitle(opts.SubtitleCodec) && !opts.ForceSidecarSubs
}

// hwOverlayFilter returns the GPU compositor that blends a decoded bitmap
// subtitle onto the video surface, or "" when the backend has none wired.
//
// No explicit w/h: the overlay runs at the SOURCE resolution, where the PGS/
// VobSub bitmap is already authored to fit, and the rung scaler runs after it -
// the same order the software path uses. Sizing only has to be stated when the
// two surfaces differ, which is exactly what made a scale-before-overlay graph
// fail with "Failed to render parameter buffer: 18" on Intel VAAPI.
//
// eof_action=pass:repeatlast=0 is what makes a SPARSE track work: a forced
// subtitle carries a handful of cues in a whole film, so its branch hits EOF
// long before the video and must not stall or freeze the last bitmap on screen.
func hwOverlayFilter(accel transcoding.HWAccelType) string {
	switch accel {
	case transcoding.VAAPI:
		return "overlay_vaapi=eof_action=pass:repeatlast=0"
	case transcoding.QSV:
		return "overlay_qsv=eof_action=pass:repeatlast=0"
	case transcoding.NVENC:
		return "overlay_cuda=eof_action=pass:repeatlast=0"
	}
	return ""
}

// hwOverlayUploadFilter returns the filters that put the DECODED subtitle bitmap
// on the GPU in the pixel format its compositor blends. ffmpeg turns the
// subtitle stream into video frames itself (sub2video), so this branch only has
// to convert and upload. CUDA blends in planar YUV with an alpha plane; the VPP
// backends take packed BGRA.
func hwOverlayUploadFilter(accel transcoding.HWAccelType) string {
	switch accel {
	case transcoding.VAAPI:
		return "format=bgra,hwupload"
	case transcoding.QSV:
		return "format=bgra,hwupload=extra_hw_frames=64"
	case transcoding.NVENC:
		return "format=yuva420p,hwupload_cuda"
	}
	return ""
}

// hwBurnInBranch renders the "[0:s:N]<upload>[s]" branch and the blend that
// consumes it, given the label carrying the main surface. Shared by both HW
// builders so a single-rung and a multi-rung burn-in can never drift apart.
func hwBurnInBranch(opts transcoding.HLSProcessOpts, mainLabel string) (subBranch, blend string) {
	subBranch = fmt.Sprintf("[0:s:%d]%s[subpic]", opts.SubtitleTrackIndex, hwOverlayUploadFilter(opts.HWAccel))
	blend = fmt.Sprintf("%s[subpic]%s", mainLabel, hwOverlayFilter(opts.HWAccel))
	return subBranch, blend
}

// hwInitArgs returns the pre-`-i` flags that put decode + filtering on the GPU
// for a zero-copy transcode. NVENC selects the device through CUDA; VAAPI/QSV
// take an explicit DRM render node when one is provided (empty → ffmpeg
// default device).
func hwInitArgs(accel transcoding.HWAccelType, device string) []string {
	switch accel {
	case transcoding.NVENC:
		return []string{"-hwaccel", "cuda", "-hwaccel_output_format", "cuda"}
	case transcoding.VAAPI:
		a := []string{"-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi"}
		if device != "" {
			a = append(a, "-hwaccel_device", device)
		}
		return a
	case transcoding.QSV:
		a := []string{"-hwaccel", "qsv", "-hwaccel_output_format", "qsv"}
		if device != "" {
			a = append(a, "-qsv_device", device)
		}
		return a
	}
	return nil
}

// hwDeviceOnlyArgs returns the pre-`-i` flags that bind a GPU for FILTERING
// only, without `-hwaccel`: ffmpeg decodes in software and an explicit
// hwupload (hwUploadFilter) moves the frames onto the device. This is the
// decoupled counterpart of hwInitArgs, and deliberately the same device setup
// the calibration's probeHWEncode / probeHW10BitTranscode already validate on
// the host - so a backend that reports usable has run this exact prologue.
func hwDeviceOnlyArgs(accel transcoding.HWAccelType, device string) []string {
	switch accel {
	case transcoding.VAAPI:
		if device == "" {
			// No render node to bind: hwupload would have no device to target.
			// The caller's hwUsable gate keeps such a session on software.
			return nil
		}
		return []string{"-vaapi_device", device}
	case transcoding.QSV:
		return []string{"-init_hw_device", "qsv=hw", "-filter_hw_device", "hw"}
	}
	return nil
}

// hwUploadFilter returns the filter that moves software-decoded frames onto the
// GPU for the decoupled path, or "" for a backend with no wired upload chain
// (NVENC - see HLSProcessOpts.HWSoftwareDecode). format=nv12 does the 8-bit
// fold on the CPU before the upload, so the surface reaching scale_<backend>
// is already what the 8-bit HW encoders take - the p010-into-scale_vaapi
// hazard hwScaleFilter documents cannot arise on this path.
//
// Mirrors benchmark.go's hwUploadChain, which is the shape the calibration
// measures; the two must stay identical or the bench would certify a pipeline
// the runtime does not emit.
func hwUploadFilter(accel transcoding.HWAccelType) string {
	switch accel {
	case transcoding.VAAPI:
		return "format=nv12,hwupload"
	case transcoding.QSV:
		return "hwupload=extra_hw_frames=16,format=qsv"
	}
	return ""
}

// hwSourceFilterPrefix returns the filters that run BEFORE the GPU scaler(s):
// on the zero-copy path just the GPU deinterlacer (the decoder already put the
// frames on the device), on the decoupled path the SOFTWARE deinterlacer
// followed by the upload.
//
// The deinterlacer stays in software there on purpose. Both deinterlacers act
// only on frames the decoder FLAGGED as interlaced, and that flag is a property
// of the decoded frame: keeping bwdif on the host side means the decoupled path
// deinterlaces exactly what the all-software path deinterlaces today, on the
// very sources that need it (PAL/NTSC DVD rips). Its cost at SD is small next
// to the encode being offloaded.
func hwSourceFilterPrefix(opts transcoding.HLSProcessOpts) string {
	if !opts.HWSoftwareDecode {
		// A bridged session deinterlaces on the CPU inside the bridge, after
		// hwdownload (see hwTonemapBridge): a GPU deinterlacer in front of the
		// download breaks the download on progressive content.
		if hwBridges(opts) {
			return ""
		}
		if d := hwDeinterlaceFilter(opts.HWAccel, opts.DeinterlaceAll, opts.HWDeinterlace); d != "" {
			return d + ","
		}
		return ""
	}
	prefix := ""
	if d := deinterlaceFilterFor(opts); d != "" {
		prefix = d + ","
	}
	return prefix + hwUploadFilter(opts.HWAccel) + ","
}

// hwScaleFilter returns the GPU scaler for the backend. Width pins the output
// width; `-2` keeps the source aspect ratio rounded to an even height (which
// every HW encoder requires). format=nv12 forces 8-bit 4:2:0 output: 10-bit
// sources (HEVC Main10 / yuv420p10le, ubiquitous for 2160p) otherwise reach the
// 8-bit-only h264 HW encoders as 10-bit and fail with "10 bit encode not
// supported" → "device doesn't support required NVENC features", producing no
// segment. This mirrors the software path's `-pix_fmt yuv420p`. (No HDR tonemap
// here, matching the software path's current behaviour.)
func hwScaleFilter(accel transcoding.HWAccelType, width int) string {
	return hwScaleFilterFormat(accel, width, "nv12")
}

// hwScaleFilterFormat is hwScaleFilter with the output surface format (nv12, or
// p010 for the 10-bit HDR passthrough).
func hwScaleFilterFormat(accel transcoding.HWAccelType, width int, format string) string {
	switch accel {
	case transcoding.NVENC:
		return fmt.Sprintf("scale_cuda=%d:-2:format=%s", width, format)
	case transcoding.VAAPI:
		return fmt.Sprintf("scale_vaapi=%d:-2:format=%s", width, format)
	case transcoding.QSV:
		return fmt.Sprintf("scale_qsv=w=%d:h=%s:format=%s", width, qsvAutoHeight(width), format)
	}
	return fmt.Sprintf("scale=%d:-2", width)
}

// qsvAutoHeight is the "keep the aspect, round to even" height for scale_qsv,
// which - unlike scale_cuda / scale_vaapi - refuses the -2 shorthand ("Size
// values less than -1 are not acceptable"; measured on an UHD 630, 2026-09-13:
// every QSV session and every QSV probe failed on it). The expression form is
// what the generic scale's -2 means.
func qsvAutoHeight(width int) string {
	return fmt.Sprintf("trunc(%d*ih/iw/2)*2", width)
}

// hwScaleToHeight renders a GPU (or software) scaler pinning the HEIGHT with
// the width following the aspect - the bench ladders' form. scale_qsv gets the
// expression scale's -2 stands for.
func hwScaleToHeight(scaler string, height int) string {
	if scaler == "scale_qsv" {
		return fmt.Sprintf("scale_qsv=w=trunc(%d*iw/ih/2)*2:h=%d", height, height)
	}
	if scaler == "" {
		scaler = "scale"
	}
	return fmt.Sprintf("%s=-2:%d", scaler, height)
}

// hwRungScaleFilter is the per-rung scaler: p010 surfaces on an HDR passthrough, nv12 otherwise.
func hwRungScaleFilter(opts transcoding.HLSProcessOpts, width int) string {
	if opts.HDRPassthrough {
		return hwScaleFilterFormat(opts.HWAccel, width, "p010")
	}
	return hwScaleFilter(opts.HWAccel, width)
}

// hwHDRPassthroughArgs: HEVC Main10 + the source's HDR signalling, so the encoder
// writes the VUI/SEI an HDR display needs. variant < 0 = no stream specifier.
func hwHDRPassthroughArgs(accel transcoding.HWAccelType, transfer string, variant int) []string {
	spec := ""
	if variant >= 0 {
		spec = fmt.Sprintf(":%d", variant)
	}
	trc := "smpte2084"
	if strings.EqualFold(transfer, "hlg") {
		trc = "arib-std-b67"
	}
	args := []string{
		"-profile:v" + spec, "main10",
		"-color_primaries:v" + spec, "bt2020",
		"-color_trc:v" + spec, trc,
		"-colorspace:v" + spec, "bt2020nc",
	}
	if accel == transcoding.VAAPI {
		args = append(args, "-sei:v"+spec, "hdr")
	}
	return args
}

// hevcTagArgs muxes an HEVC output as hvc1 (Safari refuses ffmpeg's hev1 default).
func hevcTagArgs(codec string, variant int) []string {
	if normalizeVideoCodec(codec) != "hevc" {
		return nil
	}
	if variant >= 0 {
		return []string{fmt.Sprintf("-tag:v:%d", variant), "hvc1"}
	}
	return []string{"-tag:v", "hvc1"}
}

// hwDeinterlaceFilter returns a GPU deinterlacer that, like the software
// deinterlaceFilter ("bwdif=deint=interlaced"), only touches frames the decoder
// flags as interlaced and passes progressive content through untouched - so it
// is always safe to prepend, never doubles the frame rate, and keeps the work
// on the GPU surfaces (no hwdownload round-trip). Returns "" for backends with
// no field-flag-respecting HW deinterlacer:
//   - NVENC/CUDA: yadif_cuda with deint=interlaced (mode 0 ⇒ one frame per frame)
//   - VAAPI: deinterlace_vaapi=auto=1 - auto defaults to 0 (verified with
//     `ffmpeg -h filter=deinterlace_vaapi`), which deinterlaces EVERY frame,
//     progressive films included (softened picture, doubled VPP work; prod
//     2026-09-04). auto=1 passes frames the decoder did not flag through
//     unchanged. Covers AMD iGPUs and Intel iGPUs driven through VAAPI.
//   - QSV: deinterlace_qsv has no "only-if-flagged" mode and would wrongly
//     deinterlace progressive content, so it is emitted ONLY on hwDeint - the
//     resolver's verdict that the source is known interlaced (field_order flag
//     or idet) AND the calibration proved the filter correct
//     (HWBackendCap.HWDeinterlace + the deint route). mode=advanced is the
//     motion-adaptive one; the default rate=frame keeps one frame per field
//     pair, like bwdif send_frame.
func hwDeinterlaceFilter(accel transcoding.HWAccelType, all, hwDeint bool) string {
	// all = the idet verdict said interlaced despite progressive flags: every
	// frame goes through, like the software deint=all.
	switch accel {
	case transcoding.QSV:
		if hwDeint {
			return hwQSVDeinterlaceFilter
		}
		return ""
	case transcoding.NVENC:
		if all {
			return "yadif_cuda=deint=all"
		}
		return "yadif_cuda=deint=interlaced"
	case transcoding.VAAPI:
		if all {
			return "deinterlace_vaapi"
		}
		return "deinterlace_vaapi=auto=1"
	}
	return ""
}

// hwQSVDeinterlaceFilter is the QSV session-level deinterlacer the probe
// certifies and the builder emits (identical strings: what was proven is what runs).
const hwQSVDeinterlaceFilter = "deinterlace_qsv=mode=advanced"

// hwEncoderQualityArgs returns the encoder-specific quality flags applied to
// the output stream currently being defined (no stream specifier - ffmpeg
// binds them to the next `-c:v`). The BASELINE (preset, plus NVENC's required
// -forced-idr) is always emitted and is byte-for-byte the pre-probe behaviour.
// The optional streaming tuning (b-frames, rc-lookahead, spatial/temporal AQ,
// b_ref_mode) is gated on `features` - the per-option set the calibration
// probe confirmed this host's encoder ACCEPTS (capability.HWFeatures forwarded
// via HLSProcessOpts.HWFeatures). An option absent from `features` is never
// passed, so an older NVENC/QSV/VAAPI chip that rejects it is never handed it,
// and the level-2 safety net catches anything the probe missed. Empty features
// ⇒ the proven conservative baseline. No `-pix_fmt yuv420p` - GPU surfaces.
func hwEncoderQualityArgs(accel transcoding.HWAccelType, codec string, features []string) []string {
	has := func(f string) bool {
		return slices.Contains(features, f)
	}
	lowPower := has(transcoding.LowPowerFeature(normalizeVideoCodec(codec)))
	switch accel {
	case transcoding.NVENC:
		// -forced-idr 1 is REQUIRED and unconditional: without it NVENC honours
		// -force_key_frames with plain (non-IDR) I-frames the HLS segmenter
		// won't cut on, so segment lengths drift off the precomputed VOD
		// cadence (validated on an RTX 4070 Ti). It is supported on every NVENC
		// generation, so it is not a probed option.
		args := []string{"-preset", "p4", "-forced-idr", "1"}
		if has("rc_lookahead") {
			args = append(args, "-rc-lookahead", "20")
		}
		if has("spatial_aq") {
			args = append(args, "-spatial-aq", "1")
		}
		if has("temporal_aq") {
			args = append(args, "-temporal-aq", "1")
		}
		if has("b_ref_mode") {
			args = append(args, "-b_ref_mode", "middle")
		}
		if has("bframes") {
			args = append(args, "-bf", "3")
		}
		return args
	case transcoding.QSV:
		// QSV baseline: veryfast keeps cold starts low on weak iGPUs. -forced_idr
		// is gated because an older pre-Skylake/Apollo-Lake QSV rejects unknown
		// options and would fail the whole encode - only emit it once probed.
		args := []string{"-preset", "veryfast"}
		if lowPower {
			args = append(args, "-low_power", "1")
		}
		if has("forced_idr") {
			args = append(args, "-forced_idr", "1")
		}
		return args
	case transcoding.VAAPI:
		// VAAPI rate control is the driver default; only add probed extras.
		var args []string
		if lowPower {
			args = append(args, "-low_power", "1")
		}
		if has(transcoding.BFramesFeature(normalizeVideoCodec(codec), lowPower)) {
			args = append(args, "-bf", "3")
		}
		return args
	}
	return nil
}

// appendHWTranscodeInputArgs emits the GPU-decode input prologue shared by the
// single- and multi-variant HW builders: the seek, the HW init flags (which
// bind to the video -i that immediately follows), the video input, and the
// software-decoded audio-delay input.
// hwRateControlArgs renders a target for a GPU encoder. Bitrate-targeted
// modes are VBR whose peak is the ceiling (bufsize the window): the 160 %
// headroom of the CPU path let a 70 Mb/s 2160p rung ship 3 s segments at
// 80-130 Mb/s (VAAPI, prod 2026-09-11), and a bare -b:v left VAAPI in AVBR.
// The GPU constant-quality modes (QVBR, ICQ, cq) are not calibrated yet, so a
// constant-quality target is served as VBR at its ceiling until they are.
func hwRateControlArgs(accel transcoding.HWAccelType, qt transcoding.QualityTarget, variant int) []string {
	spec := ""
	if variant >= 0 {
		spec = fmt.Sprintf(":v:%d", variant)
	}
	target := qt.TargetKbps
	if qt.Mode != transcoding.RateControlVBR && qt.Mode != transcoding.RateControlCBR {
		target = qt.MaxKbps
	}
	var args []string
	if accel == transcoding.VAAPI {
		mode := "VBR"
		if qt.Mode == transcoding.RateControlCBR {
			mode = "CBR"
		}
		args = append(args, "-rc_mode"+spec, mode)
	}
	bflag := "-b:v"
	if variant >= 0 {
		bflag = fmt.Sprintf("-b:v:%d", variant) // -b:v:N, not -b:v + :v:N
	}
	args = append(args, bflag, fmt.Sprintf("%dk", target))
	if qt.MaxKbps > 0 {
		args = append(args, "-maxrate"+spec, fmt.Sprintf("%dk", qt.MaxKbps))
	}
	if qt.BufferKbps > 0 {
		args = append(args, "-bufsize"+spec, fmt.Sprintf("%dk", qt.BufferKbps))
	}
	return args
}

func appendHWTranscodeInputArgs(args []string, opts transcoding.HLSProcessOpts) []string {
	if opts.StartSec > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", opts.StartSec))
	}
	if opts.HWSoftwareDecode {
		// Decoupled: bind the device for the filters only. No -hwaccel, so the
		// video input decodes in software exactly like the all-software path.
		args = append(args, hwDeviceOnlyArgs(opts.HWAccel, opts.HWDevice)...)
	} else {
		args = append(args, hwInitArgs(opts.HWAccel, opts.HWDevice)...)
	}
	args = appendInputModifiers(args, opts.ReadRate)
	args = append(args, "-i", opts.FilePath)
	// The audio input decodes in software: the hwaccel flags above bind to the
	// VIDEO input only (they precede its -i).
	return appendAudioDelayInput(args, opts, opts.StartSec, func(a []string) []string {
		return appendInputModifiers(a, opts.ReadRate)
	})
}

// buildSingleTranscodeHWArgs is the GPU single-rung equivalent of
// buildSingleTranscodeArgs. Mirrors that builder's container choice (.ts) so a
// SW↔HW fallback never changes on-disk naming for the same job.
func buildSingleTranscodeHWArgs(opts transcoding.HLSProcessOpts) []string {
	p := opts.Profiles[len(opts.Profiles)-1]

	args := appendHWTranscodeInputArgs(nil, opts)

	vf := hwRungScaleFilter(opts, p.Width)
	if tm := hwToneMapStage(opts); tm != "" {
		vf = tm + "," + vf
	}
	vf = hwSourceFilterPrefix(opts) + vf
	if burnInRequested(opts) {
		// A second input (the subtitle) means -filter_complex, not -vf. The blend
		// runs at source resolution and the rung scaler after it, so drop the
		// scaler from the main branch and re-add it past the compositor.
		main := hwSourceFilterPrefix(opts)
		if tm := hwToneMapStage(opts); tm != "" {
			main += tm + ","
		}
		main = strings.TrimSuffix(main, ",")
		if main == "" {
			main = "null" // a filter chain cannot be empty
		}
		subBranch, blend := hwBurnInBranch(opts, "[main]")
		args = append(args, "-filter_complex", fmt.Sprintf("[0:v]%s[main];%s;%s,%s[out]",
			main, subBranch, blend, hwRungScaleFilter(opts, p.Width)))
		args = append(args, "-map", "[out]")
	} else {
		args = append(args, "-map", "0:v:0", "-vf", vf)
	}
	args = append(args, "-c:v", videoEncoderFor(opts.VideoCodec, opts.HWAccel))
	args = append(args, hwEncoderQualityArgs(opts.HWAccel, opts.VideoCodec, opts.HWFeatures)...)
	if opts.HDRPassthrough {
		args = append(args, hwHDRPassthroughArgs(opts.HWAccel, opts.HDRTransfer, -1)...)
	}
	args = append(args, hevcTagArgs(opts.VideoCodec, -1)...)
	args = append(args, hwRateControlArgs(opts.HWAccel, qualityTargetFor(opts, len(opts.Profiles)-1, p, true), -1)...)
	args = append(args, hlsForceKeyframesArgs...)
	args = append(args, audioOutputArgsSingle(opts, p.AudioBitrate)...)

	return appendHLSOutputArgs(args, opts.OutputDir, opts.UseFMP4, hlsTimeFor(opts), startNumberFor(opts), opts.VariantIndex)
}

// buildMultiVariantHWArgs is the GPU equivalent of buildMultiVariantArgs: a
// single decode fed into a GPU split, one GPU scaler per rung, one HW encoder
// per rung - zero host-memory round-trip. Audio stays software-encoded.
func buildMultiVariantHWArgs(opts transcoding.HLSProcessOpts) []string {
	n := len(opts.Profiles)
	vcodec := videoEncoderFor(opts.VideoCodec, opts.HWAccel)

	args := appendHWTranscodeInputArgs(nil, opts)
	// No -copyts, same reason as buildMultiVariantArgs (edit-list poisoning).

	splitOutputs := make([]string, n)
	for i := range n {
		splitOutputs[i] = fmt.Sprintf("[v%d]", i)
	}
	// Deinterlace once before the GPU split, so every rung scales from
	// progressive frames (no-op on progressive sources), and on the decoupled
	// path upload once too - the split then fans out GPU surfaces exactly as it
	// does for a zero-copy decode.
	src := "[0:v]" + hwSourceFilterPrefix(opts)
	// Tone-map once on the source surface before the GPU split (not per-rung).
	if tm := hwToneMapStage(opts); tm != "" {
		src += tm + ","
	}
	// Burn the subtitle in ONCE, before the split, so every rung inherits it from
	// a single GPU blend instead of compositing per rung.
	var filterParts []string
	if burnInRequested(opts) {
		main := strings.TrimSuffix(strings.TrimPrefix(src, "[0:v]"), ",")
		if main == "" {
			main = "null"
		}
		subBranch, blend := hwBurnInBranch(opts, "[main]")
		filterParts = append(filterParts,
			fmt.Sprintf("[0:v]%s[main]", main),
			subBranch,
			fmt.Sprintf("%s,split=%d%s", blend, n, strings.Join(splitOutputs, "")),
		)
	} else {
		filterParts = append(filterParts, fmt.Sprintf("%ssplit=%d%s", src, n, strings.Join(splitOutputs, "")))
	}
	for i, p := range opts.Profiles {
		filterParts = append(filterParts, fmt.Sprintf("[v%d]%s[o%d]", i, hwRungScaleFilter(opts, p.Width), i))
	}
	args = append(args, "-filter_complex", strings.Join(filterParts, ";"))

	for i, p := range opts.Profiles {
		args = append(args, "-map", fmt.Sprintf("[o%d]", i), fmt.Sprintf("-c:v:%d", i), vcodec)
		args = append(args, hwEncoderQualityArgs(opts.HWAccel, opts.VideoCodec, opts.HWFeatures)...)
		if opts.HDRPassthrough {
			args = append(args, hwHDRPassthroughArgs(opts.HWAccel, opts.HDRTransfer, i)...)
		}
		args = append(args, hevcTagArgs(opts.VideoCodec, i)...)
		args = append(args, hwRateControlArgs(opts.HWAccel, qualityTargetFor(opts, i, p, true), i)...)
		args = append(args, audioVariantArgs(opts, i, p.AudioBitrate)...)
	}
	args = append(args, hlsForceKeyframesArgs...)

	return appendHLSMultiVariantTail(args, opts, n)
}
