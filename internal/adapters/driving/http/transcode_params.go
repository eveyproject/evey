package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	domaintranscoding "github.com/gravity-zero/evey/internal/domain/transcoding"
	"github.com/gravity-zero/evey/internal/pkg/hdr"
)

type transcodeParams struct {
	audioTrack    int
	videoScale    int
	subtitleTrack int // -1 = no subtitle burn-in
	remuxOnly     bool
	startTimeSec  float64
}

type transcodeAnalysis struct {
	needsVideoTranscode bool
	needsAudioTranscode bool
	needsToneMap        bool
	burnInSubtitle      bool   // true when subtitle track requires burn-in (PGS/VobSub)
	subtitleCodec       string // codec of the resolved subtitle stream
	hdrInfo             *hdr.HDRInfo
	streams             []*domainlibrary.MediaStream
}

func (h *transcodeServe) parseParams(r *http.Request) transcodeParams {
	p := transcodeParams{subtitleTrack: -1}
	if at := r.URL.Query().Get("audioTrack"); at != "" {
		if n, err := strconv.Atoi(at); err == nil && n >= 0 {
			p.audioTrack = n
		}
	}
	if vs := r.URL.Query().Get("videoScale"); vs != "" {
		if n, err := strconv.Atoi(vs); err == nil && n > 0 && n <= 4320 {
			p.videoScale = n
		}
	}
	if st := r.URL.Query().Get("subtitleTrack"); st != "" {
		if n, err := strconv.Atoi(st); err == nil && n >= 0 {
			p.subtitleTrack = n
		}
	}
	p.remuxOnly = r.URL.Query().Get("remux") == "1"
	if st := r.URL.Query().Get("startTime"); st != "" {
		if f, err := strconv.ParseFloat(st, 64); err == nil && f > 0 {
			p.startTimeSec = f
		}
	}
	return p
}

func (h *transcodeServe) analyzeStreams(ctx context.Context, mediaID string, remuxOnly bool, client domaintranscoding.ClientProfile, subtitleTrack int) transcodeAnalysis {
	a := transcodeAnalysis{needsAudioTranscode: true}

	streams, err := h.libSvc.GetMediaStreams(ctx, mediaID)
	if err != nil {
		slog.Error("transcode: GetMediaStreams failed", slog.String("media_id", mediaID), slog.Any("err", err))
		return a
	}
	a.streams = streams
	for _, s := range streams {
		slog.Debug("transcode: stream", slog.String("media_id", mediaID), slog.String("type", s.StreamType), slog.String("codec", s.Codec), slog.Bool("default", s.IsDefault))
		if s.StreamType == "video" && !remuxOnly && !containsCodec(client.VideoCodecs, s.Codec) {
			a.needsVideoTranscode = true
		}
		if s.StreamType == "audio" && s.IsDefault {
			if containsCodec(client.AudioCodecs, s.Codec) {
				a.needsAudioTranscode = false
			}
		}
	}

	if subtitleTrack >= 0 {
		subIdx := 0
		for _, s := range streams {
			if s.StreamType != "subtitle" {
				continue
			}
			if subIdx == subtitleTrack {
				a.subtitleCodec = s.Codec
				if apptranscoding.IsImageSubtitle(s.Codec) {
					a.burnInSubtitle = true
					a.needsVideoTranscode = true
				}
				break
			}
			subIdx++
		}
	}

	hdrStreams := make([]hdr.VideoStream, len(streams))
	for i, s := range streams {
		hdrStreams[i] = s
	}
	a.hdrInfo = hdr.DetectHDR(hdrStreams)
	a.needsToneMap = a.hdrInfo.IsHDR && !client.SupportsHDR
	if a.needsToneMap {
		a.needsVideoTranscode = true
	}
	return a
}

func (h *hlsMasterServe) resolveSourceDimensions(ctx context.Context, mediaID string) (int, int, []*domainlibrary.MediaStream) {
	sourceWidth, sourceHeight := 1920, 1080
	streams, err := h.libSvc.GetMediaStreams(ctx, mediaID)
	if err != nil {
		return sourceWidth, sourceHeight, nil
	}
	for _, s := range streams {
		if s.StreamType == "video" && s.Width != nil && s.Height != nil {
			sourceWidth = *s.Width
			sourceHeight = *s.Height
			break
		}
	}
	return sourceWidth, sourceHeight, streams
}

func (h *hlsMasterServe) resolveSubtitle(r *http.Request, streams []*domainlibrary.MediaStream) (int, string) {
	st := r.URL.Query().Get("subtitleTrack")
	if st == "" || streams == nil {
		return -1, ""
	}
	n, err := strconv.Atoi(st)
	if err != nil || n < 0 {
		return -1, ""
	}
	subIdx := 0
	for _, s := range streams {
		if s.StreamType != "subtitle" {
			continue
		}
		if subIdx == n {
			return n, s.Codec
		}
		subIdx++
	}
	return -1, ""
}

func parseClientCapabilities(r *http.Request, hdrSupported bool) domaintranscoding.ClientProfile {
	client := domaintranscoding.DefaultWebProfile
	if v := r.URL.Query().Get("videoCodecs"); v != "" {
		client.VideoCodecs = strings.Split(v, ",")
	}
	if v := r.URL.Query().Get("audioCodecs"); v != "" {
		client.AudioCodecs = strings.Split(v, ",")
	}
	if v := r.URL.Query().Get("containers"); v != "" {
		client.Containers = strings.Split(v, ",")
	}
	if hdrSupported {
		client.SupportsHDR = true
	}
	return client
}

func containsCodec(list []string, codec string) bool {
	codec = strings.ToLower(codec)
	for _, v := range list {
		if strings.EqualFold(v, codec) {
			return true
		}
	}
	return false
}
