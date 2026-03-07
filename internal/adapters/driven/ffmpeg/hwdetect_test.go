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
