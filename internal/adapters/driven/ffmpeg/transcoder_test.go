package ffmpeg

import (
	"testing"

	"github.com/gravity-zero/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTranscoder() *Transcoder {
	return NewTranscoder("ffmpeg", "/tmp/evey/transcode")
}

func TestBuildTranscodeArgs_Basic(t *testing.T) {
	t.Parallel()
	tr := newTestTranscoder()
	job := &transcoding.TranscodeJob{
		ID:          "job-1",
		InputPath:   "/input/video.mkv",
		OutputCodec: "h264",
		Preset:      "fast",
		BitrateOut:  4000000,
		HWAccelType: "none",
	}

	args := tr.BuildTranscodeArgs(job)

	assert.Contains(t, args, "-y")
	assert.Contains(t, args, "-i")
	assert.Contains(t, args, "/input/video.mkv")
	assert.Contains(t, args, "-c:v")
	assert.Contains(t, args, "libx264")
	assert.Contains(t, args, "-preset")
	assert.Contains(t, args, "fast")
	assert.Contains(t, args, "-b:v")
	assert.Contains(t, args, "4000000")
	assert.Contains(t, args, "-f")
	assert.Contains(t, args, "hls")
	assert.Contains(t, args, "-hls_time")
	assert.Contains(t, args, "6")
}

func TestBuildTranscodeArgs_VAAPI(t *testing.T) {
	t.Parallel()
	tr := newTestTranscoder()
	job := &transcoding.TranscodeJob{
		ID:          "job-2",
		InputPath:   "/input/video.mkv",
		OutputCodec: "h264",
		HWAccelType: "vaapi",
	}

	args := tr.BuildTranscodeArgs(job)
	assert.Contains(t, args, "-hwaccel")
	assert.Contains(t, args, "vaapi")
	assert.Contains(t, args, "h264_vaapi")
}

func TestBuildTranscodeArgs_NVENC(t *testing.T) {
	t.Parallel()
	tr := newTestTranscoder()
	job := &transcoding.TranscodeJob{
		ID:          "job-3",
		InputPath:   "/input/video.mkv",
		OutputCodec: "hevc",
		HWAccelType: "nvenc",
	}

	args := tr.BuildTranscodeArgs(job)
	assert.Contains(t, args, "-hwaccel")
	assert.Contains(t, args, "cuda")
	assert.Contains(t, args, "hevc_nvenc")
}

func TestBuildTranscodeArgs_QSV(t *testing.T) {
	t.Parallel()
	tr := newTestTranscoder()
	job := &transcoding.TranscodeJob{
		ID:          "job-4",
		InputPath:   "/input/video.mkv",
		OutputCodec: "h264",
		HWAccelType: "qsv",
	}

	args := tr.BuildTranscodeArgs(job)
	assert.Contains(t, args, "-hwaccel")
	assert.Contains(t, args, "qsv")
	assert.Contains(t, args, "h264_qsv")
}

func TestBuildTranscodeArgs_Resolution(t *testing.T) {
	t.Parallel()
	tr := newTestTranscoder()
	job := &transcoding.TranscodeJob{
		ID:            "job-5",
		InputPath:     "/input/video.mkv",
		OutputCodec:   "h264",
		HWAccelType:   "none",
		ResolutionOut: "1280:720",
	}

	args := tr.BuildTranscodeArgs(job)
	assert.Contains(t, args, "-vf")
	assert.Contains(t, args, "scale=1280:720")
}

func TestBuildTranscodeArgs_HLSOutput(t *testing.T) {
	t.Parallel()
	tr := newTestTranscoder()
	job := &transcoding.TranscodeJob{
		ID:          "job-hls",
		InputPath:   "/input/video.mkv",
		OutputCodec: "h264",
		HWAccelType: "none",
	}

	args := tr.BuildTranscodeArgs(job)
	assert.Contains(t, args, "/tmp/evey/transcode/job-hls.m3u8")
	assert.Contains(t, args, "/tmp/evey/transcode/job-hls_%03d.ts")
}

func TestBuildTranscodeArgs_AudioCodecDefault(t *testing.T) {
	t.Parallel()
	tr := newTestTranscoder()
	job := &transcoding.TranscodeJob{
		ID:          "job-audio-default",
		InputPath:   "/input/video.mkv",
		OutputCodec: "h264",
		HWAccelType: "none",
	}

	args := tr.BuildTranscodeArgs(job)
	for i, a := range args {
		if a == "-c:a" {
			assert.Equal(t, transcoding.DefaultAudioCodec, args[i+1])
			return
		}
	}
	t.Fatal("-c:a flag not found in args")
}

func TestBuildTranscodeArgs_AudioCodecCustom(t *testing.T) {
	t.Parallel()
	tr := newTestTranscoder()
	job := &transcoding.TranscodeJob{
		ID:          "job-audio-custom",
		InputPath:   "/input/video.mkv",
		OutputCodec: "h264",
		AudioCodec:  "opus",
		HWAccelType: "none",
	}

	args := tr.BuildTranscodeArgs(job)
	for i, a := range args {
		if a == "-c:a" {
			assert.Equal(t, "opus", args[i+1])
			return
		}
	}
	t.Fatal("-c:a flag not found in args")
}

func TestParseProgress(t *testing.T) {
	t.Parallel()
	line := "frame=  120 fps= 30.0 q=28.0 size=    256kB time=00:00:04.00 bitrate= 524.3kbits/s speed= 1.5x"
	dur := int64(10000)
	p, ok := ParseProgress(line, "j1", &dur)
	require.True(t, ok)
	assert.Equal(t, "j1", p.JobID)
	assert.InDelta(t, 30.0, p.FPS, 0.1)
	assert.InDelta(t, 1.5, p.Speed, 0.1)
	assert.Equal(t, int64(524300), p.CurrentBitrate)
	assert.InDelta(t, 40.0, p.Percent, 1.0)
	assert.Positive(t, p.ETAMs)
}

func TestParseProgress_NilDuration(t *testing.T) {
	t.Parallel()
	line := "frame=  120 fps= 24.0 q=28.0 size=    256kB time=00:01:30.50 bitrate= 524.3kbits/s speed= 2.0x"
	p, ok := ParseProgress(line, "j2", nil)
	require.True(t, ok)
	assert.InDelta(t, float64(0), p.Percent, 0.001)
	assert.Equal(t, int64(0), p.ETAMs)
}

func TestParseProgress_NoMatch(t *testing.T) {
	t.Parallel()
	_, ok := ParseProgress("some random log line", "j3", nil)
	assert.False(t, ok)
}

func TestScanFFmpegLines(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		data   []byte
		atEOF  bool
		wantA  int
		wantT  string
		wantOK bool
	}{
		{"newline", []byte("hello\nworld"), false, 6, "hello", true},
		{"carriage return", []byte("progress\rmore"), false, 9, "progress", true},
		{"eof with data", []byte("final"), true, 5, "final", true},
		{"eof empty", []byte{}, true, 0, "", false},
		{"no delimiter not eof", []byte("partial"), false, 0, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			advance, token, err := scanFFmpegLines(tt.data, tt.atEOF)
			require.NoError(t, err)
			assert.Equal(t, tt.wantA, advance)
			if tt.wantOK {
				assert.Equal(t, tt.wantT, string(token))
			} else {
				assert.Nil(t, token)
			}
		})
	}
}

func TestNewTranscoder(t *testing.T) {
	t.Parallel()
	tr := NewTranscoder("/usr/bin/ffmpeg", "/tmp/out")
	assert.Equal(t, "/usr/bin/ffmpeg", tr.binaryPath)
	assert.Equal(t, "/tmp/out", tr.outputDir)
}

func TestTranscoder_Stop_NotFound(t *testing.T) {
	t.Parallel()
	tr := newTestTranscoder()
	err := tr.Stop(t.Context(), "nonexistent-job")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestTranscoder_Progress_NotFound(t *testing.T) {
	t.Parallel()
	tr := newTestTranscoder()
	_, err := tr.Progress(t.Context(), "nonexistent-job")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestBuildTranscodeArgs_ResolutionVAAPI(t *testing.T) {
	t.Parallel()
	tr := newTestTranscoder()
	job := &transcoding.TranscodeJob{
		ID:            "job-res-vaapi",
		InputPath:     "/input/video.mkv",
		OutputCodec:   "h264",
		HWAccelType:   "vaapi",
		ResolutionOut: "1280:720",
	}
	args := tr.BuildTranscodeArgs(job)
	found := false
	for _, a := range args {
		if a == "scale_vaapi=w=1280:h=720" {
			found = true
		}
	}
	assert.True(t, found, "expected scale_vaapi filter")
}

func TestBuildTranscodeArgs_ResolutionNVENC(t *testing.T) {
	t.Parallel()
	tr := newTestTranscoder()
	job := &transcoding.TranscodeJob{
		ID:            "job-res-nvenc",
		InputPath:     "/input/video.mkv",
		OutputCodec:   "h264",
		HWAccelType:   "nvenc",
		ResolutionOut: "1920:1080",
	}
	args := tr.BuildTranscodeArgs(job)
	found := false
	for _, a := range args {
		if a == "scale_cuda=w=1920:h=1080" {
			found = true
		}
	}
	assert.True(t, found, "expected scale_cuda filter")
}

func TestBuildTranscodeArgs_ResolutionQSV(t *testing.T) {
	t.Parallel()
	tr := newTestTranscoder()
	job := &transcoding.TranscodeJob{
		ID:            "job-res-qsv",
		InputPath:     "/input/video.mkv",
		OutputCodec:   "h264",
		HWAccelType:   "qsv",
		ResolutionOut: "1920:1080",
	}
	args := tr.BuildTranscodeArgs(job)
	found := false
	for _, a := range args {
		if a == "scale_qsv=w=1920:h=1080" {
			found = true
		}
	}
	assert.True(t, found, "expected scale_qsv filter")
}

func TestParseProgress_ExceedsPercent(t *testing.T) {
	t.Parallel()
	line := "frame= 9999 fps= 60.0 q=28.0 size=   4096kB time=00:10:00.00 bitrate= 524.3kbits/s speed= 2.0x"
	dur := int64(1000) // 1 second duration, but progress shows 10 minutes
	p, ok := ParseProgress(line, "j-cap", &dur)
	require.True(t, ok)
	assert.InDelta(t, 100.0, p.Percent, 0.1, "percent should be capped at 100")
}

func TestParseProgress_ZeroDuration(t *testing.T) {
	t.Parallel()
	line := "frame=  120 fps= 30.0 q=28.0 size=    256kB time=00:00:04.00 bitrate= 524.3kbits/s speed= 1.5x"
	dur := int64(0)
	p, ok := ParseProgress(line, "j-zero", &dur)
	require.True(t, ok)
	assert.InDelta(t, 0.0, p.Percent, 0.001, "zero duration = 0 percent")
}

func TestMapVideoEncoder_Passthrough(t *testing.T) {
	t.Parallel()
	got := mapVideoEncoder("custom_codec", "none")
	assert.Equal(t, "custom_codec", got, "unknown codec passes through")
}

func TestMapVideoEncoder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		codec  string
		hw     string
		expect string
	}{
		{"h264", "none", "libx264"},
		{"hevc", "none", "libx265"},
		{"h265", "none", "libx265"},
		{"vp9", "none", "libvpx-vp9"},
		{"av1", "none", "libaom-av1"},
		{"h264", "vaapi", "h264_vaapi"},
		{"hevc", "vaapi", "hevc_vaapi"},
		{"h264", "nvenc", "h264_nvenc"},
		{"hevc", "nvenc", "hevc_nvenc"},
		{"h264", "qsv", "h264_qsv"},
		{"hevc", "qsv", "hevc_qsv"},
	}

	for _, tt := range tests {
		t.Run(tt.codec+"_"+tt.hw, func(t *testing.T) {
			t.Parallel()
			got := mapVideoEncoder(tt.codec, tt.hw)
			assert.Equal(t, tt.expect, got)
		})
	}
}
