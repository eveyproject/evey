package ffmpeg

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

type HWDetector struct {
	ffmpegPath string
}

func NewHWDetector(ffmpegPath string) *HWDetector {
	return &HWDetector{ffmpegPath: ffmpegPath}
}

func (d *HWDetector) DetectDevices(ctx context.Context) ([]transcoding.HWDevice, error) {
	var devices []transcoding.HWDevice

	if dev := detectVAAPI(ctx, d.ffmpegPath); dev != nil {
		devices = append(devices, *dev)
	}
	if dev := detectNVENC(ctx, d.ffmpegPath); dev != nil {
		devices = append(devices, *dev)
	}
	if dev := detectQSV(ctx, d.ffmpegPath); dev != nil {
		devices = append(devices, *dev)
	}

	return devices, nil
}

func (d *HWDetector) SupportsCodec(ctx context.Context, deviceType, codec string) (bool, error) {
	encoder := resolveHWEncoder(deviceType, codec)
	if encoder == "" {
		return false, nil
	}
	return checkEncoderAvailable(ctx, d.ffmpegPath, encoder), nil
}

func detectVAAPI(ctx context.Context, ffmpegPath string) *transcoding.HWDevice {
	driPath := "/dev/dri"
	entries, err := os.ReadDir(driPath)
	if err != nil {
		return nil
	}

	hasRender := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "renderD") {
			hasRender = true
			break
		}
	}
	if !hasRender {
		return nil
	}

	codecs := probeHWCodecs(ctx, ffmpegPath, "vaapi")
	return &transcoding.HWDevice{
		Type:            string(transcoding.VAAPI),
		Name:            filepath.Join(driPath, "renderD128"),
		SupportedCodecs: codecs,
	}
}

func detectNVENC(ctx context.Context, ffmpegPath string) *transcoding.HWDevice {
	// Check nvidia-smi
	smiPath, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return nil
	}

	cmd := exec.CommandContext(ctx, smiPath, "--query-gpu=name", "--format=csv,noheader")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	gpuName := strings.TrimSpace(string(out))
	if gpuName == "" {
		return nil
	}

	codecs := probeHWCodecs(ctx, ffmpegPath, "nvenc")
	return &transcoding.HWDevice{
		Type:            string(transcoding.NVENC),
		Name:            gpuName,
		SupportedCodecs: codecs,
	}
}

func detectQSV(ctx context.Context, ffmpegPath string) *transcoding.HWDevice {
	// Check for /dev/dri/renderD128 with Intel GPU
	if _, err := os.Stat("/dev/dri/renderD128"); err != nil {
		return nil
	}

	cmd := exec.CommandContext(ctx, "ls", "/sys/class/drm/card0/device/driver/module/drivers/")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	if !strings.Contains(string(out), "i915") {
		return nil
	}

	codecs := probeHWCodecs(ctx, ffmpegPath, "qsv")
	return &transcoding.HWDevice{
		Type:            string(transcoding.QSV),
		Name:            "Intel Quick Sync",
		SupportedCodecs: codecs,
	}
}

func probeHWCodecs(ctx context.Context, ffmpegPath, hwType string) []string {
	var encoderSuffixes []string
	switch hwType {
	case "vaapi":
		encoderSuffixes = []string{"h264_vaapi", "hevc_vaapi"}
	case "nvenc":
		encoderSuffixes = []string{"h264_nvenc", "hevc_nvenc"}
	case "qsv":
		encoderSuffixes = []string{"h264_qsv", "hevc_qsv"}
	default:
		return nil
	}

	var codecs []string
	for _, enc := range encoderSuffixes {
		if checkEncoderAvailable(ctx, ffmpegPath, enc) {
			codecs = append(codecs, enc)
		}
	}
	return codecs
}

func checkEncoderAvailable(ctx context.Context, ffmpegPath, encoder string) bool {
	cmd := exec.CommandContext(ctx, ffmpegPath, "-encoders")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == encoder {
			return true
		}
	}
	return false
}

func resolveHWEncoder(deviceType, codec string) string {
	codec = strings.ToLower(codec)
	switch deviceType {
	case string(transcoding.VAAPI):
		return fmt.Sprintf("%s_vaapi", codec)
	case string(transcoding.NVENC):
		return fmt.Sprintf("%s_nvenc", codec)
	case string(transcoding.QSV):
		return fmt.Sprintf("%s_qsv", codec)
	}
	return ""
}
