package ffmpeg

import (
	"testing"

	"github.com/gravity-zero/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
)

func TestResolveHWEncoder(t *testing.T) {
	tests := []struct {
		device string
		codec  string
		want   string
	}{
		{string(transcoding.VAAPI), "h264", "h264_vaapi"},
		{string(transcoding.VAAPI), "hevc", "hevc_vaapi"},
		{string(transcoding.NVENC), "h264", "h264_nvenc"},
		{string(transcoding.NVENC), "hevc", "hevc_nvenc"},
		{string(transcoding.QSV), "h264", "h264_qsv"},
		{string(transcoding.QSV), "hevc", "hevc_qsv"},
		{"unknown", "h264", ""},
		{string(transcoding.VAAPI), "H264", "h264_vaapi"},
	}

	for _, tt := range tests {
		t.Run(tt.device+"_"+tt.codec, func(t *testing.T) {
			got := resolveHWEncoder(tt.device, tt.codec)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCheckEncoderAvailable_InvalidPath(t *testing.T) {
	ok := checkEncoderAvailable(t.Context(), "/nonexistent/ffmpeg", "libx264")
	assert.False(t, ok)
}

func TestNewHWDetector(t *testing.T) {
	d := NewHWDetector("/usr/bin/ffmpeg")
	assert.NotNil(t, d)
	assert.Equal(t, "/usr/bin/ffmpeg", d.ffmpegPath)
}

func TestDetectDevices_ReturnsNoError(t *testing.T) {
	d := NewHWDetector("/nonexistent/ffmpeg")
	devices, err := d.DetectDevices(t.Context())
	assert.NoError(t, err)
	// may or may not find devices, but should not error
	_ = devices
}

func TestSupportsCodec_UnknownDevice(t *testing.T) {
	d := NewHWDetector("/nonexistent/ffmpeg")
	supported, err := d.SupportsCodec(t.Context(), "unknown_device", "h264")
	assert.NoError(t, err)
	assert.False(t, supported)
}

func TestSupportsCodec_ValidDevice_InvalidPath(t *testing.T) {
	d := NewHWDetector("/nonexistent/ffmpeg")
	supported, err := d.SupportsCodec(t.Context(), string(transcoding.VAAPI), "h264")
	assert.NoError(t, err)
	assert.False(t, supported, "invalid ffmpeg path means encoder not available")
}

func TestProbeHWCodecs_UnknownType(t *testing.T) {
	codecs := probeHWCodecs(t.Context(), "/nonexistent/ffmpeg", "unknown")
	assert.Nil(t, codecs)
}

func TestProbeHWCodecs_VAAPI_InvalidPath(t *testing.T) {
	codecs := probeHWCodecs(t.Context(), "/nonexistent/ffmpeg", "vaapi")
	assert.Empty(t, codecs)
}

func TestProbeHWCodecs_NVENC_InvalidPath(t *testing.T) {
	codecs := probeHWCodecs(t.Context(), "/nonexistent/ffmpeg", "nvenc")
	assert.Empty(t, codecs)
}

func TestProbeHWCodecs_QSV_InvalidPath(t *testing.T) {
	codecs := probeHWCodecs(t.Context(), "/nonexistent/ffmpeg", "qsv")
	assert.Empty(t, codecs)
}
