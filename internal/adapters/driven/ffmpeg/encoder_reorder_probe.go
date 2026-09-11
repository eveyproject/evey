package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/procfork"
)

// EncoderSpec names one encoder configuration the transcode builders emit.
type EncoderSpec struct {
	Codec    string
	Accel    transcoding.HWAccelType
	Device   string
	Features []string
}

func (s EncoderSpec) key() string {
	f := slices.Clone(s.Features)
	slices.Sort(f)
	return strings.Join([]string{s.Codec, string(s.Accel), s.Device, strings.Join(f, ",")}, "|")
}

// encoderReorderCache memoises the probe per configuration: the answer is a
// property of the encoder, not of any source.
var encoderReorderCache sync.Map

const encoderReorderProbeTimeout = 20 * time.Second

// BundledEncoderReorderFrames reports how many frames this encoder configuration
// delays presentation by (has_b_frames of its own output). A separate audio
// rendition must be shifted by that many frames or it runs early (libx264
// veryfast: 2 frames, 83 ms at 24 fps). ok=false when the probe failed.
func BundledEncoderReorderFrames(ctx context.Context, spec EncoderSpec) (int, bool) {
	key := spec.key()
	if v, ok := encoderReorderCache.Load(key); ok {
		return v.(int), true
	}
	n, err := probeEncoderReorderFrames(ctx, "ffmpeg", "ffprobe", spec)
	if err != nil {
		return 0, false
	}
	encoderReorderCache.Store(key, n)
	return n, true
}

// encoderReorderProbeArgs builds the probe encode: a synthetic 24 fps source
// through the production encoder args, into a fragmented MP4 at outPath.
func encoderReorderProbeArgs(spec EncoderSpec, outPath string) []string {
	args := []string{"-hide_banner", "-v", "error", "-nostdin", "-y"}
	var vf string
	switch spec.Accel {
	case transcoding.VAAPI:
		args = append(args, "-vaapi_device", spec.Device)
		vf = "format=nv12,hwupload"
	case transcoding.QSV:
		args = append(args, "-init_hw_device", "qsv=hw", "-filter_hw_device", "hw")
		vf = "hwupload=extra_hw_frames=16,format=qsv"
	case transcoding.NVENC:
		args = append(args, "-init_hw_device", "cuda=cu", "-filter_hw_device", "cu")
		vf = "format=nv12,hwupload_cuda"
	}
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=256x144:rate=24:duration=1")
	if vf != "" {
		args = append(args, "-vf", vf)
	}
	enc := videoEncoderFor(spec.Codec, spec.Accel)
	args = append(args, "-c:v", enc)
	if spec.Accel == "" || spec.Accel == transcoding.None {
		args = append(args, softwareProfileLevelArgs(enc, -1, 144, 0)...)
		args = append(args, softwareX264TuningArgs(enc, -1)...)
		args = append(args, svtAV1PresetArgs(enc)...)
		args = append(args, softwareRateControlArgs(transcoding.StaticQualityPolicy{}.TargetFor(transcoding.HLSProfile{VideoBitrate: 1000}, transcoding.EncodeRoute{}), "")...)
		args = append(args, "-pix_fmt", "yuv420p")
	} else {
		args = append(args, hwEncoderQualityArgs(spec.Accel, spec.Codec, spec.Features)...)
		args = append(args, hwRateControlArgs(spec.Accel, transcoding.StaticQualityPolicy{}.TargetFor(transcoding.HLSProfile{VideoBitrate: 1000}, transcoding.EncodeRoute{HWUsable: true}), -1)...)
	}
	args = append(args, hlsForceKeyframesArgs...)
	return append(args, "-f", "mp4", "-movflags", "+frag_keyframe+empty_moov", outPath)
}

func probeEncoderReorderFrames(ctx context.Context, ffmpegPath, ffprobePath string, spec EncoderSpec) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, encoderReorderProbeTimeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "evey-reorder-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "probe.mp4")
	if runErr := procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "encoder_reorder.encode"}, ffmpegPath, encoderReorderProbeArgs(spec, out)...); runErr != nil {
		return 0, fmt.Errorf("encoder reorder probe encode: %w", runErr)
	}
	raw, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "encoder_reorder.probe"}, ffprobePath,
		"-v", "quiet", "-select_streams", "v:0", "-print_format", "json", "-show_entries", "stream=has_b_frames", out)
	if err != nil {
		return 0, fmt.Errorf("encoder reorder probe read: %w", err)
	}
	var parsed struct {
		Streams []struct {
			HasBFrames int `json:"has_b_frames"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed.Streams) == 0 {
		return 0, fmt.Errorf("encoder reorder probe: no video stream in output")
	}
	return parsed.Streams[0].HasBFrames, nil
}
