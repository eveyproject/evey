package httpapi

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/hdr"
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
		// ParseFloat accepts "Inf" and "inf" with no error at all, and +Inf
		// satisfies a bare `f > 0`, so the value used to reach the ffmpeg
		// argument builder and come out as `-ss +Inf`. ffmpeg rejects that, so
		// a nonsense query parameter produced a failed transcode instead of a
		// seek to the start. ("1e400" was already handled: ParseFloat returns
		// +Inf there too, but with an out-of-range error.)
		if f, err := strconv.ParseFloat(st, 64); err == nil && f > 0 && !math.IsInf(f, 1) {
			p.startTimeSec = f
		}
	}
	return p
}

// clampAudioTrack bounds a requested audio track index to the audio streams
// actually present in the media. The index is emitted verbatim as ffmpeg
// "-map 0:a:N"; an out-of-range value (from a buggy or malicious client) would
// make ffmpeg fail with "matches no streams" and abort the whole transcode with
// a 500. We fall back to the first audio track instead. In-range indices are
// returned unchanged, so legitimate track selection (indices derived from the
// client's streamInfo) is never affected.
func clampAudioTrack(idx int, streams []*domainlibrary.MediaStream) int {
	if idx <= 0 {
		return 0
	}
	audioCount := 0
	for _, s := range streams {
		if s.StreamType == "audio" {
			audioCount++
		}
	}
	if idx >= audioCount {
		return 0
	}
	return idx
}

func (h *transcodeServe) analyzeStreams(ctx context.Context, mediaID string, remuxOnly bool, client domaintranscoding.ClientProfile, subtitleTrack int) transcodeAnalysis {
	a := transcodeAnalysis{needsAudioTranscode: true}

	streams, err := h.libSvc.GetMediaStreams(ctx, mediaID)
	if err != nil {
		slog.ErrorContext(ctx, "transcode: GetMediaStreams failed", slog.String("media_id", mediaID), slog.Any("err", err))
		return a
	}
	a.streams = streams
	for _, s := range streams {
		slog.Debug("transcode: stream", slog.String("media_id", mediaID), slog.String("type", string(s.StreamType)), slog.String("codec", s.Codec), slog.Bool("default", s.IsDefault))
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

// hdrQuerySupported reports whether the client advertised an HDR-capable
// display. Web and mobile send hdr=1; hdr=true is the older spelling some
// callers used - accepting only one of the two silently strips the capability
// and forces a tonemap for clients that decode HDR natively.
func hdrQuerySupported(r *http.Request) bool {
	v := r.URL.Query().Get("hdr")
	return v == "1" || v == "true"
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
	// Audio codecs this client accepts as a RAW passthrough inside our fMP4
	// segments - deliberately not `audioCodecs`, which every web and iOS client
	// fills with mp3 whether or not it can take it in that container. This one
	// rides the segment URLs like the codec lists do, so a segment request
	// re-derives the SAME decision the master was built from.
	if v := r.URL.Query().Get("fmp4AudioPassthrough"); v != "" {
		client.FMP4AudioPassthrough = strings.Split(v, ",")
	}
	// Channel ceiling of the playback DEVICE (a 2.0 TV, a stereo-only cast
	// target...). Web players normally omit it: browsers decode multichannel
	// AAC and downmix to the output device themselves, so the profile default
	// applies. Clamped to a sane layout range; the decision additionally caps
	// AAC targets at 5.1 (aacMaxTargetChannels).
	if n := maxAudioChannelsQuery(r); n > 0 {
		client.MaxAudioChannels = n
	}
	if n := parseIntQuery(r, "maxH264Level", 0); n > 0 {
		client.MaxH264Level = n
	}
	if hdrSupported {
		client.SupportsHDR = true
	}
	return client
}

// maxAudioChannelsQuery is the ONE reading of the client's channel ceiling.
// The master's decision, the URIs the manifest bakes and the audio rendition's
// own fingerprint all derive from it: two readings that disagree serve a
// rendition the master never negotiated - exactly the 5.1 a cast receiver kept
// getting after asking for stereo (2026-08-08). Returns 0 when nothing
// usable was declared.
func maxAudioChannelsQuery(r *http.Request) int {
	if r == nil {
		return 0
	}
	if v := r.URL.Query().Get("maxAudioChannels"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 8 {
			return n
		}
	}
	return 0
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
