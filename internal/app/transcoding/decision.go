package transcoding

import (
	"strings"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/transcoding"
)

var bitmapSubCodecs = map[string]bool{
	"hdmv_pgs_subtitle": true,
	"dvd_subtitle":      true,
	"dvb_subtitle":      true,
	"pgssub":            true,
	"xsub":              true,
}

func MakePlaybackDecision(
	streams []*library.MediaStream,
	containerFormat string,
	client transcoding.ClientProfile,
	maxBitrateKbps int,
) *transcoding.PlaybackDecision {
	d := &transcoding.PlaybackDecision{
		PlayMethod:       "direct_play",
		VideoDecision:    "copy",
		AudioDecision:    "copy",
		SubtitleDecision: "",
		TranscodeReasons: []string{},
	}

	effectiveMaxBitrate := client.MaxBitrate
	if maxBitrateKbps > 0 && (effectiveMaxBitrate == 0 || int64(maxBitrateKbps) < effectiveMaxBitrate) {
		effectiveMaxBitrate = int64(maxBitrateKbps)
	}

	var totalBitrate int64
	for _, s := range streams {
		if s.Bitrate != nil {
			totalBitrate += *s.Bitrate
		}
	}

	for _, s := range streams {
		switch s.StreamType {
		case "video":
			if !contains(client.VideoCodecs, strings.ToLower(s.Codec)) {
				d.TranscodeReasons = append(d.TranscodeReasons, "video_codec_unsupported")
				d.VideoDecision = "transcode"
				d.VideoCodec = "h264"
			}

			if s.Width != nil && client.MaxWidth > 0 && *s.Width > client.MaxWidth {
				d.TranscodeReasons = append(d.TranscodeReasons, "resolution_exceeded")
				d.VideoDecision = "transcode"
				d.MaxWidth = client.MaxWidth
			}
			if s.Height != nil && client.MaxHeight > 0 && *s.Height > client.MaxHeight {
				if !containsReason(d.TranscodeReasons, "resolution_exceeded") {
					d.TranscodeReasons = append(d.TranscodeReasons, "resolution_exceeded")
				}
				d.VideoDecision = "transcode"
				d.MaxHeight = client.MaxHeight
			}

			if s.HDRType != nil && *s.HDRType != "" && !client.SupportsHDR {
				d.TranscodeReasons = append(d.TranscodeReasons, "hdr_tonemapping_needed")
				d.VideoDecision = "transcode"
			}

			if d.VideoDecision == "transcode" && d.VideoCodec == "" {
				d.VideoCodec = "h264"
			}

		case "audio":
			if !s.IsDefault {
				continue
			}
			if !contains(client.AudioCodecs, strings.ToLower(s.Codec)) {
				d.TranscodeReasons = append(d.TranscodeReasons, "audio_codec_unsupported")
				d.AudioDecision = "transcode"
				d.AudioCodec = "aac"
			}
			if s.Channels != nil && client.MaxAudioChannels > 0 && *s.Channels > client.MaxAudioChannels {
				d.TranscodeReasons = append(d.TranscodeReasons, "audio_channels_exceeded")
				d.AudioDecision = "transcode"
				if d.AudioCodec == "" {
					d.AudioCodec = "aac"
				}
			}

		case "subtitle":
			if bitmapSubCodecs[strings.ToLower(s.Codec)] {
				d.SubtitleDecision = "burn"
			} else if s.IsExternal {
				if d.SubtitleDecision == "" {
					d.SubtitleDecision = "external"
				}
			} else {
				if d.SubtitleDecision == "" {
					d.SubtitleDecision = "external"
				}
			}
		}
	}

	// Bitrate check (uses total file bitrate)
	totalBitrateKbps := totalBitrate / 1000
	if effectiveMaxBitrate > 0 && totalBitrateKbps > effectiveMaxBitrate {
		d.TranscodeReasons = append(d.TranscodeReasons, "bitrate_exceeded")
		d.VideoDecision = "transcode"
		d.VideoBitrate = effectiveMaxBitrate
		if d.VideoCodec == "" {
			d.VideoCodec = "h264"
		}
	}

	// Container check
	containerLower := strings.ToLower(containerFormat)
	if containerLower == "matroska" {
		containerLower = "mkv"
	}
	if !contains(client.Containers, containerLower) {
		d.TranscodeReasons = append(d.TranscodeReasons, "container_incompatible")
	}

	// Determine play method
	switch {
	case len(d.TranscodeReasons) == 0:
		d.PlayMethod = "direct_play"
	case onlyContainerIssue(d.TranscodeReasons) && d.VideoDecision == "copy" && d.AudioDecision == "copy":
		d.PlayMethod = "direct_stream"
		d.Container = preferredContainer(client.Containers)
	default:
		d.PlayMethod = "transcode"
		if d.Container == "" {
			d.Container = preferredContainer(client.Containers)
		}
	}

	// Default transcode bitrates
	if d.VideoDecision == "transcode" && d.VideoBitrate == 0 {
		d.VideoBitrate = 4000 // 4 Mbps default
	}
	if d.AudioDecision == "transcode" && d.AudioBitrate == 0 {
		d.AudioBitrate = 192 // 192 kbps default
	}

	return d
}

func contains(list []string, val string) bool {
	for _, v := range list {
		if strings.EqualFold(v, val) {
			return true
		}
	}
	return false
}

func containsReason(reasons []string, reason string) bool {
	for _, r := range reasons {
		if r == reason {
			return true
		}
	}
	return false
}

func onlyContainerIssue(reasons []string) bool {
	for _, r := range reasons {
		if r != "container_incompatible" {
			return false
		}
	}
	return len(reasons) > 0
}

func preferredContainer(containers []string) string {
	if len(containers) > 0 {
		return containers[0]
	}
	return "mp4"
}
