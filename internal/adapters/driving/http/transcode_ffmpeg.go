package httpapi

import (
	"fmt"
	"strings"

	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/hdr"
)

func (h *transcodeServe) buildFFmpegArgs(filePath string, p transcodeParams, a transcodeAnalysis) []string {
	var args []string
	if p.startTimeSec > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", p.startTimeSec))
	}
	args = append(args, "-i", filePath)

	if a.burnInSubtitle && p.subtitleTrack >= 0 {
		var filters []string
		switch {
		case a.needsToneMap && !p.remuxOnly:
			toneMapFilter := hdr.BuildToneMapFilter(a.hdrInfo, hdr.DefaultToneMapOptions())
			filters = append(filters, fmt.Sprintf("[0:v]%s[vt]", toneMapFilter))
			if p.videoScale > 0 {
				filters = append(filters, fmt.Sprintf("[vt][0:s:%d]overlay,scale=-2:%d[vb]", p.subtitleTrack, p.videoScale))
			} else {
				filters = append(filters, fmt.Sprintf("[vt][0:s:%d]overlay[vb]", p.subtitleTrack))
			}
		case p.videoScale > 0:
			filters = append(filters, fmt.Sprintf("[0:v][0:s:%d]overlay,scale=-2:%d[vb]", p.subtitleTrack, p.videoScale))
		default:
			filters = append(filters, fmt.Sprintf("[0:v][0:s:%d]overlay[vb]", p.subtitleTrack))
		}
		args = append(args, "-filter_complex", strings.Join(filters, ";"))
		args = append(args, "-map", "[vb]")
		args = append(args, "-map", fmt.Sprintf("0:a:%d", p.audioTrack))
		args = append(args, "-c:v", "libx264", "-crf", "23", "-preset", "veryfast", "-threads", "2")
	} else {
		args = append(args, "-map", "0:v:0")
		args = append(args, "-map", fmt.Sprintf("0:a:%d", p.audioTrack))

		switch {
		case a.needsToneMap && !p.remuxOnly:
			toneMapFilter := hdr.BuildToneMapFilter(a.hdrInfo, hdr.DefaultToneMapOptions())
			if p.videoScale > 0 {
				args = append(args, "-vf", fmt.Sprintf("scale=-2:%d,%s", p.videoScale, toneMapFilter))
			} else {
				args = append(args, "-vf", toneMapFilter)
			}
			args = append(args, "-c:v", "libx264", "-crf", "23", "-preset", "veryfast", "-threads", "2")
		case p.videoScale > 0:
			args = append(args, "-vf", fmt.Sprintf("scale=-2:%d", p.videoScale), "-c:v", "libx264", "-crf", "23", "-preset", "veryfast", "-threads", "2")
		case a.needsVideoTranscode:
			args = append(args, "-c:v", "libx264", "-crf", "23", "-preset", "veryfast", "-tune", "zerolatency", "-threads", "0")
		default:
			args = append(args, "-c:v", "copy")
		}
	}

	if a.needsAudioTranscode {
		args = append(args, "-c:a", "aac", "-ac", "2", "-b:a", "192k")
		// Headroom for the stereo fold - see domain/transcoding/audio_downmix.go.
		args = append(args, "-af", domaintranscoding.AudioFoldFilter(0, 2))
	} else {
		args = append(args, "-c:a", "copy")
	}
	args = append(args, "-movflags", "frag_keyframe+empty_moov+default_base_moof",
		"-frag_duration", "2000000",
		"-f", "mp4", "-")
	return args
}
