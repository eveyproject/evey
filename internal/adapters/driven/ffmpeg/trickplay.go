package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/evey-project/evey/internal/pkg/procfork"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/pkg/hdr"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// keyframeInputFormat maps a mkvgo keyframe-packet extension to the ffmpeg
// demuxer that reads it: Annex-B elementary streams for H.264/HEVC, an IVF
// container for VP8/VP9/AV1.
var keyframeInputFormat = map[string]string{
	".h264": "h264",
	".hevc": "hevc",
	".ivf":  "ivf",
}

type TrickplayGenerator struct {
	binary      string
	probeBinary string
}

func NewTrickplayGenerator(binary, probeBinary string) *TrickplayGenerator {
	return &TrickplayGenerator{binary: binary, probeBinary: probeBinary}
}

func (g *TrickplayGenerator) probeDuration(ctx context.Context, filePath string) (float64, error) {
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "trickplay.duration"}, g.probeBinary,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		filePath,
	)
	if err != nil {
		return 0, fmt.Errorf("ffprobe: %w", err)
	}
	var result struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if unmarshalErr := json.Unmarshal(out, &result); unmarshalErr != nil {
		return 0, fmt.Errorf("ffprobe parse: %w", unmarshalErr)
	}
	d, err := strconv.ParseFloat(result.Format.Duration, 64)
	if err != nil {
		return 0, fmt.Errorf("ffprobe duration %q: %w", result.Format.Duration, err)
	}
	return d, nil
}

// Generate produces a trickplay sprite sheet (tiled webp images) for the
// given media file. Previous implementation launched one ffmpeg process per
// thumbnail (N seeks into the same file), which for a 2160p HEVC episode
// turned ~420 thumbs into ~25 minutes of CPU - the server spent entire
// weekends catching up on a modest library. This single-pass version
// streams the decoder once with `fps=1/interval,scale,tile` baked into
// the filter graph: the file is opened once, frames are downsampled on
// the fly, and tiled sprites land straight into the output dir as webp.
// Typical speedup: 5-15× on 4K content vs the N-call approach.
//
// maxWorkers now caps ffmpeg's internal decode/encode thread count
// (0 → use the CPU-budget default; adaptive stays gentle at ~25% of budget).
func (g *TrickplayGenerator) Generate(ctx context.Context, filePath, outputDir string, interval, width, tileWidth, tileHeight, maxWorkers int, knownDurationMs int64) (*library.TrickplayResult, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.trickplay.Generate")
	defer span.End()
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("create trickplay dir: %w", err)
	}

	// Readability pre-flight, to short-circuit unprobeable inputs before paying
	// ffmpeg's startup cost. The RESULT was always discarded - this only ever
	// asked "does this file open?" - so a duration the scan already established
	// answers it just as well, without a fork. Only a file we know nothing about
	// still pays for ffprobe here.
	if knownDurationMs <= 0 {
		if _, err := g.probeDuration(ctx, filePath); err != nil {
			return nil, err
		}
	}

	if interval <= 0 {
		interval = 10
	}
	if width <= 0 {
		width = 320
	}
	if tileWidth <= 0 {
		tileWidth = 10
	}
	if tileHeight <= 0 {
		tileHeight = 10
	}

	// The caller sizes the decode on the task's CPU share (cpubudget); the
	// fork below reserves that many slots. Unsized decodes on one thread.
	threads := max(maxWorkers, 1)

	outputPattern := filepath.Join(outputDir, "trickplay_%03d.webp")
	// -skip_frame nokey instructs the decoder to drop non-keyframe packets
	// so HEVC/H.264 4K decode only processes I-frames (typically every
	// 2-5 seconds). On a 1080p HEVC episode this turns ~9 min of full
	// decode into ~30s of keyframe-only work - without this flag the
	// single-pass version still pegged 1 core at 100% for minutes per item.
	//
	// -vsync vfr stops ffmpeg from trying to duplicate frames to match
	// a constant framerate when the input stream has gaps (keyframe
	// skipping creates gaps by design).
	//
	// We use the `select` filter (NOT `fps=1/N`): `fps` rewrites every
	// output frame's PTS to a clean multiple of N, which is a lie when
	// keyframes are sparse - players that trust the PTS would scrub to
	// frames whose true source position differs by ±2.5s. `select` keeps
	// each frame's original PTS, so the `showinfo` filter downstream logs
	// the real source-video time for every kept thumbnail.
	//
	// Expression: pick the first frame, then any frame whose time is at
	// least N seconds after the previously selected - yields one thumb
	// per ~N seconds of source while honoring keyframe placement.
	vf := trickplayFilter(fmt.Sprintf(
		"select='isnan(prev_selected_t)+gte(t-prev_selected_t\\,%d)',scale=%d:-1,showinfo,tile=%dx%d",
		interval, width, tileWidth, tileHeight,
	), g.probeColour(ctx, filePath))
	cmd, finish, err := procfork.Command(ctx, procfork.Options{Class: procfork.Scan, Site: "trickplay.sprites"}, g.binary,
		"-nostdin",
		"-threads", strconv.Itoa(threads),
		"-skip_frame", "nokey",
		"-i", filePath,
		"-an",
		"-vsync", "vfr",
		"-vf", vf,
		"-c:v", "libwebp",
		"-quality", "75",
		"-y", outputPattern,
	)
	if err != nil {
		return nil, err
	}
	out, runErr := cmd.CombinedOutput()
	if runErr = finish(runErr, out); runErr != nil {
		return nil, fmt.Errorf("ffmpeg trickplay single-pass: %w\n%s", runErr, string(out))
	}

	timestampsMs := parseShowinfoTimestamps(string(out))
	if len(timestampsMs) == 0 {
		return nil, errors.New("trickplay: no timestamps captured from showinfo")
	}

	imageCount := 0
	for i := 1; ; i++ {
		p := filepath.Join(outputDir, fmt.Sprintf("trickplay_%03d.webp", i))
		if _, statErr := os.Stat(p); statErr != nil {
			break
		}
		imageCount++
	}
	if imageCount == 0 {
		return nil, errors.New("no trickplay sprites produced")
	}

	return &library.TrickplayResult{
		ImageDir:          outputDir,
		ImageCount:        imageCount,
		FrameTimestampsMs: timestampsMs,
	}, nil
}

// reShowinfoPts matches the pts_time field in showinfo lines.
// Example: "[Parsed_showinfo_2 @ 0x...] n: 1 pts: 7672500 pts_time:7.672 ..."
var reShowinfoPts = regexp.MustCompile(`pts_time:(\d+(?:\.\d+)?)`)

// parseShowinfoTimestamps pulls every pts_time value from ffmpeg's stderr
// (showinfo output) and returns them as milliseconds, in extraction order.
func parseShowinfoTimestamps(stderr string) []int64 {
	matches := reShowinfoPts.FindAllStringSubmatch(stderr, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]int64, 0, len(matches))
	for _, m := range matches {
		v, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			continue
		}
		out = append(out, int64(v*1000))
	}
	return out
}

// ExtractFrame extracts a single JPEG frame at the given position (seconds).
// Returns the raw JPEG bytes. Uses -ss before -i for fast seeking.
func (g *TrickplayGenerator) ExtractFrame(ctx context.Context, filePath string, positionSec float64, width int) ([]byte, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.trickplay.ExtractFrame")
	defer span.End()
	ss := fmt.Sprintf("%.3f", positionSec)
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Interactive, Site: "trickplay.extract_frame"}, g.binary,
		"-ss", ss,
		"-i", filePath,
		"-frames:v", "1",
		"-vf", fmt.Sprintf("scale=%d:-1", width),
		"-f", "image2",
		"-c:v", "mjpeg",
		"-q:v", "3",
		"-y", "pipe:1",
	)
	if err != nil {
		return nil, fmt.Errorf("ffmpeg extract frame at %ss: %w", ss, err)
	}
	return out, nil
}

// DecodePacketToJPEG decodes a single encoded keyframe packet (from mkvgo's
// ExtractKeyframeSample - Annex-B or IVF) into a scaled JPEG. It decodes just
// that one frame, so on a weak NAS it avoids the seek + GOP decode ExtractFrame
// pays. ext selects the input demuxer (.h264/.hevc/.ivf); an unknown ext errors
// so the caller falls back to the exact ffmpeg -ss path.
func (g *TrickplayGenerator) DecodePacketToJPEG(ctx context.Context, packet []byte, ext string, width int) ([]byte, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.trickplay.DecodePacketToJPEG")
	defer span.End()

	inFmt := keyframeInputFormat[strings.ToLower(ext)]
	if inFmt == "" {
		return nil, fmt.Errorf("decode keyframe packet: unsupported ext %q", ext)
	}
	cmd, finish, err := procfork.Command(ctx, procfork.Options{Class: procfork.Interactive, Site: "trickplay.decode_packet"}, g.binary,
		"-nostdin",
		"-f", inFmt,
		"-i", "pipe:0",
		"-frames:v", "1",
		"-vf", fmt.Sprintf("scale=%d:-1", width),
		"-f", "image2",
		"-c:v", "mjpeg",
		"-q:v", "3",
		"-y", "pipe:1",
	)
	if err != nil {
		return nil, err
	}
	cmd.Stdin = bytes.NewReader(packet)
	out, err := cmd.Output()
	err = finish(err, nil)
	if err != nil {
		return nil, fmt.Errorf("ffmpeg decode keyframe packet (%s): %w", inFmt, err)
	}
	return out, nil
}

// trickplayColour is what the thumbnail pass needs to know about the source's
// colour. Zero value = SDR.
type trickplayColour struct {
	Transfer string // color_transfer as ffprobe reports it ("smpte2084", "arib-std-b67", ...)
	DoVi     bool   // a DOVI configuration record is present
}

func (c trickplayColour) isHDR() bool {
	switch strings.ToLower(strings.TrimSpace(c.Transfer)) {
	case "smpte2084", "arib-std-b67":
		return true
	}
	// Dolby Vision P5 tags NOTHING in the container (color_transfer=unknown):
	// its base layer is still PQ-ish, so sampling it as SDR washes the picture
	// out. The DOVI configuration record is the only signal.
	return c.DoVi
}

// trickplayFilter tone-maps the thumbnail pass when the source is HDR/DoVi.
//
// The graph used to be select,scale,showinfo,tile - no colour handling - so an
// HDR source was sampled as if it were SDR and the strip came out washed out and
// tinted, while the player (which DOES tone-map) showed the right picture.
// Delegates to the player's own tone-map builder (buildToneMapFilter, which
// handles the tonemapx/zscale portability the private copy used to hardcode)
// so the strip and the player never drift on the curve. The strip already
// scaled in base, so dstWidth=0 folds no further downscale; the tone-map runs
// AFTER base, on the small tiles, not the source frame.
func trickplayFilter(base string, col trickplayColour) string {
	if !col.isHDR() {
		return base // the vast majority of files: no needless colour pass
	}
	info := &hdr.HDRInfo{IsHDR: true, Type: trickplayHDRType(col)}
	return base + "," + buildToneMapFilter(info, 0)
}

// trickplayHDRType maps the thumbnail probe's colour signal onto the Type
// hdr.setParamsFor reads to tag the frame before tonemapx: HLG needs its own
// transfer tag, everything else (PQ, and an untagged DoVi base layer) is PQ-ish.
func trickplayHDRType(col trickplayColour) string {
	switch strings.ToLower(strings.TrimSpace(col.Transfer)) {
	case "smpte2084":
		return "hdr10"
	case "arib-std-b67":
		return "hlg"
	}
	return "dolby_vision" // untagged DoVi P5 container: no transfer, only the RPU
}

// probeColour reads the source's transfer curve and DoVi presence. Best-effort:
// on any failure it returns the zero value (SDR), i.e. today's behaviour - a
// thumbnail pass must never fail because a probe did.
func (g *TrickplayGenerator) probeColour(ctx context.Context, filePath string) trickplayColour {
	return probeSourceColour(ctx, g.probeBinary, "trickplay.colour", filePath)
}

// probeSourceColour is the probe behind probeColour, shared with the sync
// preview: both render a small SDR picture of a source that may be HDR.
func probeSourceColour(ctx context.Context, probeBinary, site, filePath string) trickplayColour {
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: site}, probeBinary,
		"-v", "quiet",
		"-select_streams", "v:0",
		"-print_format", "json",
		"-show_entries", "stream=color_transfer:stream_side_data=side_data_type",
		filePath,
	)
	if err != nil {
		return trickplayColour{}
	}
	var parsed struct {
		Streams []struct {
			ColorTransfer string `json:"color_transfer"`
			SideDataList  []struct {
				SideDataType string `json:"side_data_type"`
			} `json:"side_data_list"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil || len(parsed.Streams) == 0 {
		return trickplayColour{}
	}
	s := parsed.Streams[0]
	col := trickplayColour{Transfer: s.ColorTransfer}
	for _, sd := range s.SideDataList {
		if strings.Contains(strings.ToLower(sd.SideDataType), "dovi") ||
			strings.Contains(strings.ToLower(sd.SideDataType), "dolby vision") {
			col.DoVi = true
		}
	}
	return col
}
