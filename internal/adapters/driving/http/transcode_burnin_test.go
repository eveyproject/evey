package httpapi

import (
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/pkg/hdr"
)

func TestBuildFFmpegArgs_NoSubtitle_CopyVideo(t *testing.T) {
	h := &transcodeServe{}
	p := transcodeParams{subtitleTrack: -1}
	a := transcodeAnalysis{hdrInfo: &hdr.HDRInfo{}}

	args := h.buildFFmpegArgs("/tmp/video.mkv", p, a)
	joined := strings.Join(args, " ")

	if strings.Contains(joined, "overlay") {
		t.Error("should not contain overlay without subtitle")
	}
	if !strings.Contains(joined, "-c:v copy") {
		t.Errorf("expected video copy, got: %s", joined)
	}
	if !strings.Contains(joined, "-map 0:v:0") {
		t.Errorf("expected -map 0:v:0, got: %s", joined)
	}
}

func TestBuildFFmpegArgs_BurnIn_PGS(t *testing.T) {
	h := &transcodeServe{}
	p := transcodeParams{subtitleTrack: 1}
	a := transcodeAnalysis{
		burnInSubtitle:      true,
		needsVideoTranscode: true,
		hdrInfo:             &hdr.HDRInfo{},
	}

	args := h.buildFFmpegArgs("/tmp/video.mkv", p, a)
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "[0:v][0:s:1]overlay[vb]") {
		t.Errorf("expected overlay filter for subtitle track 1, got: %s", joined)
	}
	if !strings.Contains(joined, "-map [vb]") {
		t.Errorf("expected -map [vb], got: %s", joined)
	}
	if strings.Contains(joined, "-c:v copy") {
		t.Error("video should be transcoded when burning in subtitles")
	}
	if !strings.Contains(joined, "-c:v libx264") {
		t.Errorf("expected libx264, got: %s", joined)
	}
}

func TestBuildFFmpegArgs_BurnIn_WithScale(t *testing.T) {
	h := &transcodeServe{}
	p := transcodeParams{subtitleTrack: 0, videoScale: 720}
	a := transcodeAnalysis{
		burnInSubtitle:      true,
		needsVideoTranscode: true,
		hdrInfo:             &hdr.HDRInfo{},
	}

	args := h.buildFFmpegArgs("/tmp/video.mkv", p, a)
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "[0:v][0:s:0]overlay,scale=-2:720[vb]") {
		t.Errorf("expected overlay+scale filter, got: %s", joined)
	}
}

func TestBuildFFmpegArgs_BurnIn_WithToneMap(t *testing.T) {
	h := &transcodeServe{}
	p := transcodeParams{subtitleTrack: 2}
	a := transcodeAnalysis{
		burnInSubtitle:      true,
		needsVideoTranscode: true,
		needsToneMap:        true,
		hdrInfo:             &hdr.HDRInfo{IsHDR: true, Type: "hdr10", BitDepth: 10},
	}

	args := h.buildFFmpegArgs("/tmp/video.mkv", p, a)
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "[0:v]") {
		t.Errorf("expected tone map on [0:v], got: %s", joined)
	}
	if !strings.Contains(joined, "[vt][0:s:2]overlay[vb]") {
		t.Errorf("expected overlay after tone map, got: %s", joined)
	}
	if !strings.Contains(joined, "-map [vb]") {
		t.Errorf("expected -map [vb], got: %s", joined)
	}
}

func TestBuildFFmpegArgs_BurnIn_WithToneMapAndScale(t *testing.T) {
	h := &transcodeServe{}
	p := transcodeParams{subtitleTrack: 0, videoScale: 480}
	a := transcodeAnalysis{
		burnInSubtitle:      true,
		needsVideoTranscode: true,
		needsToneMap:        true,
		hdrInfo:             &hdr.HDRInfo{IsHDR: true, Type: "hdr10", BitDepth: 10},
	}

	args := h.buildFFmpegArgs("/tmp/video.mkv", p, a)
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "[vt][0:s:0]overlay,scale=-2:480[vb]") {
		t.Errorf("expected tone map + overlay + scale, got: %s", joined)
	}
}

func TestBuildFFmpegArgs_NoSubtitle_VideoTranscode(t *testing.T) {
	h := &transcodeServe{}
	p := transcodeParams{subtitleTrack: -1}
	a := transcodeAnalysis{
		needsVideoTranscode: true,
		hdrInfo:             &hdr.HDRInfo{},
	}

	args := h.buildFFmpegArgs("/tmp/video.mkv", p, a)
	joined := strings.Join(args, " ")

	if strings.Contains(joined, "overlay") {
		t.Error("should not contain overlay without subtitle")
	}
	if !strings.Contains(joined, "-c:v libx264") {
		t.Errorf("expected libx264 transcode, got: %s", joined)
	}
	if !strings.Contains(joined, "-map 0:v:0") {
		t.Errorf("expected standard map, got: %s", joined)
	}
}

func TestBuildFFmpegArgs_NoSubtitle_WithScale(t *testing.T) {
	h := &transcodeServe{}
	p := transcodeParams{subtitleTrack: -1, videoScale: 720}
	a := transcodeAnalysis{hdrInfo: &hdr.HDRInfo{}}

	args := h.buildFFmpegArgs("/tmp/video.mkv", p, a)
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "scale=-2:720") {
		t.Errorf("expected scale filter, got: %s", joined)
	}
	if strings.Contains(joined, "overlay") {
		t.Error("should not contain overlay")
	}
}

func TestBuildFFmpegArgs_StartTime(t *testing.T) {
	h := &transcodeServe{}
	p := transcodeParams{subtitleTrack: -1, startTimeSec: 10.5}
	a := transcodeAnalysis{hdrInfo: &hdr.HDRInfo{}}

	args := h.buildFFmpegArgs("/tmp/video.mkv", p, a)
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "-ss 10.500") {
		t.Errorf("expected -ss, got: %s", joined)
	}
}

func TestBuildFFmpegArgs_OutputFormat(t *testing.T) {
	h := &transcodeServe{}
	p := transcodeParams{subtitleTrack: -1}
	a := transcodeAnalysis{hdrInfo: &hdr.HDRInfo{}}

	args := h.buildFFmpegArgs("/tmp/video.mkv", p, a)
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "-f mp4 -") {
		t.Errorf("expected fMP4 output, got: %s", joined)
	}
	if !strings.Contains(joined, "frag_keyframe+empty_moov+default_base_moof") {
		t.Errorf("expected movflags, got: %s", joined)
	}
}
