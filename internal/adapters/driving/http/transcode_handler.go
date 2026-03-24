package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	domaintranscoding "github.com/gravity-zero/evey/internal/domain/transcoding"
	"github.com/gravity-zero/evey/internal/pkg/hdr"
)

func chaptersHandler(tokenGen TokenVerifier, libSvc ChaptersGetter, baseURL string, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter) http.HandlerFunc {
	type chapterJSON struct {
		Title       string `json:"title"`
		StartTimeMs int64  `json:"startTimeMs"`
		EndTimeMs   int64  `json:"endTimeMs"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		token := r.Header.Get("Authorization")
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		} else if t := r.URL.Query().Get("token"); t != "" {
			token = t
		} else {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		item, err := mediaGetter.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			return
		}
		chapters, err := libSvc.GetChapters(r.Context(), mediaID)
		if err != nil {
			http.Error(w, `{"message":"internal"}`, http.StatusInternalServerError)
			return
		}
		out := make([]chapterJSON, 0, len(chapters))
		for _, ch := range chapters {
			out = append(out, chapterJSON{
				Title:       ch.Title,
				StartTimeMs: ch.StartTimeMs,
				EndTimeMs:   ch.EndTimeMs,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		setCORSHeaders(w, r, baseURL)
		if err := json.NewEncoder(w).Encode(map[string]any{"chapters": out}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}
}

func libraryStatsHandler(tokenGen TokenVerifier, libSvc LibraryLister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		} else if t := r.URL.Query().Get("token"); t != "" {
			token = t
		}
		if token == "" {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		if _, err := tokenGen.ValidateAccessToken(token); err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		libs, err := libSvc.ListLibraries(r.Context())
		if err != nil {
			http.Error(w, `{"message":"internal"}`, http.StatusInternalServerError)
			return
		}
		type libStat struct {
			ID            string  `json:"id"`
			LastScannedAt *string `json:"lastScannedAt"`
		}
		stats := make([]libStat, 0, len(libs))
		for _, lib := range libs {
			var ts *string
			if lib.LastScannedAt != nil {
				s := lib.LastScannedAt.UTC().Format(time.RFC3339)
				ts = &s
			}
			stats = append(stats, libStat{ID: lib.ID, LastScannedAt: ts})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"stats": stats}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}
}

// --- playbackInfoHandler ---

type playbackServe struct {
	tokens        TokenVerifier
	libSvc        MediaStreamsGetter
	libraryAccess LibraryAccessChecker
}

func playbackInfoHandler(tokenGen TokenVerifier, libSvc MediaStreamsGetter, libraryAccess LibraryAccessChecker) http.HandlerFunc {
	h := &playbackServe{tokens: tokenGen, libSvc: libSvc, libraryAccess: libraryAccess}
	return h.serveHTTP
}

func (h *playbackServe) serveHTTP(w http.ResponseWriter, r *http.Request) {
	mediaID := r.PathValue("id")
	token := r.Header.Get("Authorization")
	if strings.HasPrefix(token, "Bearer ") {
		token = token[7:]
	} else if t := r.URL.Query().Get("token"); t != "" {
		token = t
	} else {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	claims, err := h.tokens.ValidateAccessToken(token)
	if err != nil {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}

	item, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil || item.FilePath == "" {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		return
	}

	streams, err := h.libSvc.GetMediaStreams(r.Context(), mediaID)
	if err != nil {
		http.Error(w, `{"message":"internal"}`, http.StatusInternalServerError)
		return
	}

	client := h.parseClientProfile(r)

	var userMaxBitrate int
	if client.MaxBitrate > 0 {
		userMaxBitrate = int(client.MaxBitrate)
		client.MaxBitrate = 0
	}

	decision := apptranscoding.MakePlaybackDecision(streams, item.ContainerFormat, client, userMaxBitrate)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(decision); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *playbackServe) parseClientProfile(r *http.Request) domaintranscoding.ClientProfile {
	client := domaintranscoding.DefaultWebProfile
	if v := r.URL.Query().Get("maxWidth"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			client.MaxWidth = n
		}
	}
	if v := r.URL.Query().Get("maxHeight"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			client.MaxHeight = n
		}
	}
	if v := r.URL.Query().Get("maxBitrate"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			client.MaxBitrate = n
		}
	}
	if v := r.URL.Query().Get("videoCodecs"); v != "" {
		client.VideoCodecs = strings.Split(v, ",")
	}
	if v := r.URL.Query().Get("audioCodecs"); v != "" {
		client.AudioCodecs = strings.Split(v, ",")
	}
	if v := r.URL.Query().Get("containers"); v != "" {
		client.Containers = strings.Split(v, ",")
	}
	if r.URL.Query().Get("hdr") == "true" {
		client.SupportsHDR = true
	}
	return client
}

// --- hlsMasterHandler ---

type hlsMasterServe struct {
	tokens              TokenVerifier
	libSvc              MediaStreamsGetter
	hlsManager          *apptranscoding.HLSManager
	libraryAccess       LibraryAccessChecker
	userMaxStreams       UserMaxStreamsGetter
	audioNormalizeGetter func(ctx context.Context) bool
	bitrateGetter       UserBitrateGetter
}

func hlsMasterHandler(tokenGen TokenVerifier, libSvc MediaStreamsGetter, hlsManager *apptranscoding.HLSManager, libraryAccess LibraryAccessChecker, userMaxStreams UserMaxStreamsGetter, audioNormalizeGetter func(ctx context.Context) bool, bitrateGetter UserBitrateGetter) http.HandlerFunc {
	h := &hlsMasterServe{
		tokens: tokenGen, libSvc: libSvc, hlsManager: hlsManager,
		libraryAccess: libraryAccess, userMaxStreams: userMaxStreams,
		audioNormalizeGetter: audioNormalizeGetter, bitrateGetter: bitrateGetter,
	}
	return h.serveHTTP
}

func (h *hlsMasterServe) serveHTTP(w http.ResponseWriter, r *http.Request) {
	mediaID := r.PathValue("id")
	token := r.Header.Get("Authorization")
	if strings.HasPrefix(token, "Bearer ") {
		token = token[7:]
	} else if t := r.URL.Query().Get("token"); t != "" {
		token = t
	} else {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	claims, err := h.tokens.ValidateAccessToken(token)
	if err != nil {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	item, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil || item.FilePath == "" {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		return
	}

	maxStreams := 0
	if h.userMaxStreams != nil {
		if ms := h.userMaxStreams.GetMaxStreams(r.Context(), claims.UserID); ms != nil {
			maxStreams = *ms
		}
	}

	sourceWidth, sourceHeight, mediaStreams := h.resolveSourceDimensions(r.Context(), mediaID)
	subtitleTrackIndex, subtitleCodec := h.resolveSubtitle(r, mediaStreams)

	audioTrackIndex := 0
	if at := r.URL.Query().Get("audioTrack"); at != "" {
		if n, parseErr := strconv.Atoi(at); parseErr == nil && n >= 0 {
			audioTrackIndex = n
		}
	}

	hdrSupported := r.URL.Query().Get("hdr") == "1"
	audioNormalize := h.audioNormalizeGetter != nil && h.audioNormalizeGetter(r.Context())

	// Compute playback decision to determine video/audio copy vs transcode
	client := domaintranscoding.DefaultWebProfile
	if hdrSupported {
		client.SupportsHDR = true
	}
	decision := apptranscoding.MakePlaybackDecision(mediaStreams, item.ContainerFormat, client, 0)
	videoDecision := decision.VideoDecision
	audioDecision := decision.AudioDecision

	var startSec float64
	if ss := r.URL.Query().Get("startSec"); ss != "" {
		if f, parseErr := strconv.ParseFloat(ss, 64); parseErr == nil && f > 0 {
			startSec = f
		}
	}

	var session *apptranscoding.HLSSession
	if startSec > 0 {
		session, err = h.hlsManager.RestartFromPosition(claims.UserID, mediaID, item.FilePath, sourceWidth, sourceHeight, maxStreams, audioTrackIndex, subtitleTrackIndex, subtitleCodec, mediaStreams, hdrSupported, audioNormalize, startSec, videoDecision, audioDecision)
	} else {
		session, err = h.hlsManager.GetOrCreate(claims.UserID, mediaID, item.FilePath, sourceWidth, sourceHeight, maxStreams, audioTrackIndex, subtitleTrackIndex, subtitleCodec, mediaStreams, hdrSupported, audioNormalize, 0, videoDecision, audioDecision)
	}
	if err != nil {
		slog.Error("hls: get or create session", slog.Any("err", err))
		http.Error(w, `{"message":"transcode busy"}`, http.StatusServiceUnavailable)
		return
	}
	if err := session.WaitReady(r.Context()); err != nil {
		slog.Error("hls: wait ready", slog.Any("err", err))
		http.Error(w, `{"message":"transcode failed"}`, http.StatusInternalServerError)
		return
	}

	if session.StartSec > 0 {
		w.Header().Set("X-Start-Sec", fmt.Sprintf("%.3f", session.StartSec))
	}
	h.serveMaster(w, r, session.OutputDir, claims.UserID)
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

func (h *hlsMasterServe) serveMaster(w http.ResponseWriter, r *http.Request, outputDir, userID string) {
	setCORSHeaders(w, r, "")
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")

	masterPath := filepath.Join(outputDir, "master.m3u8")
	var maxBps int64
	if h.bitrateGetter != nil {
		if limitKbps, bitrateErr := h.bitrateGetter.GetUserBitrateLimit(r.Context(), userID); bitrateErr == nil && limitKbps > 0 {
			maxBps = int64(limitKbps) * 1000
		}
	}
	if maxBps <= 0 {
		http.ServeFile(w, r, masterPath)
		return
	}
	data, readErr := os.ReadFile(masterPath)
	if readErr != nil {
		slog.Error("hls: read master playlist", slog.Any("err", readErr))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	filtered := filterMasterPlaylist(string(data), maxBps)
	_, _ = w.Write([]byte(filtered))
}

func hlsSegmentHandler(tokenGen TokenVerifier, libSvc MediaItemGetter, hlsManager *apptranscoding.HLSManager, libraryAccess LibraryAccessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		file := r.PathValue("file")
		token := r.Header.Get("Authorization")
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		} else if t := r.URL.Query().Get("token"); t != "" {
			token = t
		} else {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}

		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			return
		}

		session, ok := hlsManager.GetSession(mediaID)
		if !ok {
			http.Error(w, `{"message":"no active session"}`, http.StatusNotFound)
			return
		}
		if !claims.IsAdmin && session.UserID != claims.UserID {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		if waitErr := session.WaitReady(r.Context()); waitErr != nil {
			http.Error(w, `{"message":"session not ready"}`, http.StatusServiceUnavailable)
			return
		}

		filePath, err := safeJoin(session.OutputDir, filepath.Base(file))
		if err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if _, err := os.Stat(filePath); err != nil {
			http.Error(w, `{"message":"segment not found"}`, http.StatusNotFound)
			return
		}

		switch {
		case strings.HasSuffix(file, ".m3u8"):
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Header().Set("Cache-Control", "no-cache")
		case strings.HasSuffix(file, ".ts"):
			w.Header().Set("Content-Type", "video/mp2t")
			w.Header().Set("Cache-Control", "no-cache")
		}
		session.MarkAccess()
		http.ServeFile(w, r, filePath)
	}
}

// --- transcodeHandler ---

type transcodeServe struct {
	tokens        TokenVerifier
	libSvc        MediaStreamsGetter
	baseURL       string
	libraryAccess LibraryAccessChecker
	throttle      *apptranscoding.TranscodeThrottle
	userMaxStreams UserMaxStreamsGetter
}

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

func transcodeHandler(tokenGen TokenVerifier, libSvc MediaStreamsGetter, baseURL string, libraryAccess LibraryAccessChecker, throttle *apptranscoding.TranscodeThrottle, userMaxStreams UserMaxStreamsGetter) http.HandlerFunc {
	h := &transcodeServe{
		tokens: tokenGen, libSvc: libSvc, baseURL: baseURL,
		libraryAccess: libraryAccess, throttle: throttle, userMaxStreams: userMaxStreams,
	}
	return h.serveHTTP
}

func (h *transcodeServe) serveHTTP(w http.ResponseWriter, r *http.Request) {
	mediaID := r.PathValue("id")
	token := r.Header.Get("Authorization")
	if strings.HasPrefix(token, "Bearer ") {
		token = token[7:]
	} else if t := r.URL.Query().Get("token"); t != "" {
		token = t
	} else {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	claims, err := h.tokens.ValidateAccessToken(token)
	if err != nil {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	item, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil || item.FilePath == "" {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		return
	}
	maxStreams := 0
	if h.userMaxStreams != nil {
		if ms := h.userMaxStreams.GetMaxStreams(r.Context(), claims.UserID); ms != nil {
			maxStreams = *ms
		}
	}
	if !h.throttle.Acquire(claims.UserID, maxStreams) {
		http.Error(w, `{"message":"max concurrent streams reached"}`, http.StatusTooManyRequests)
		return
	}
	defer h.throttle.Release(claims.UserID)

	params := h.parseParams(r)
	clientSupportsHDR := r.URL.Query().Get("hdr") == "true"
	analysis := h.analyzeStreams(r.Context(), mediaID, params.remuxOnly, clientSupportsHDR, params.subtitleTrack)

	slog.Debug("transcode: decision", slog.String("media_id", mediaID), slog.Bool("needsVideoTranscode", analysis.needsVideoTranscode), slog.Bool("needsAudioTranscode", analysis.needsAudioTranscode), slog.Bool("remuxOnly", params.remuxOnly), slog.Bool("needsToneMap", analysis.needsToneMap))

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	args := h.buildFFmpegArgs(item.FilePath, params, analysis)
	slog.Debug("transcode: ffmpeg", slog.String("media_id", mediaID), slog.String("args", strings.Join(args, " ")))
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		slog.Error("ffmpeg stdout pipe", slog.Any("err", err))
		http.Error(w, "transcode failed", http.StatusInternalServerError)
		return
	}
	if err := cmd.Start(); err != nil {
		slog.Error("ffmpeg start", slog.Any("err", err))
		http.Error(w, "transcode failed", http.StatusInternalServerError)
		return
	}
	// Disable write deadline — transcode streams last minutes/hours, global WriteTimeout would cut them.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		slog.Debug("set write deadline unsupported", slog.Any("err", err))
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Accept-Ranges", "none")
	setCORSHeaders(w, r, h.baseURL)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in transcode ffmpeg cancel", "error", r, "stack", debug.Stack())
			}
		}()
		<-r.Context().Done()
		cancel()
	}()
	if _, err := io.Copy(w, stdout); err != nil && ctx.Err() == nil {
		slog.Warn("transcode copy", slog.Any("err", err))
	}
	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		slog.Warn("ffmpeg transcode", slog.String("media_id", mediaID), slog.Any("err", err))
	}
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

func (h *transcodeServe) analyzeStreams(ctx context.Context, mediaID string, remuxOnly, clientSupportsHDR bool, subtitleTrack int) transcodeAnalysis {
	a := transcodeAnalysis{needsAudioTranscode: true}

	streams, err := h.libSvc.GetMediaStreams(ctx, mediaID)
	if err != nil {
		slog.Error("transcode: GetMediaStreams failed", slog.String("media_id", mediaID), slog.Any("err", err))
		return a
	}
	a.streams = streams
	for _, s := range streams {
		slog.Debug("transcode: stream", slog.String("media_id", mediaID), slog.String("type", s.StreamType), slog.String("codec", s.Codec), slog.Bool("default", s.IsDefault))
		if s.StreamType == "video" && s.Codec != "h264" && !remuxOnly {
			a.needsVideoTranscode = true
		}
		if s.StreamType == "audio" && s.IsDefault {
			switch s.Codec {
			case "aac", "mp3", "opus":
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
	a.needsToneMap = a.hdrInfo.IsHDR && !clientSupportsHDR
	if a.needsToneMap {
		a.needsVideoTranscode = true
	}
	return a
}

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
	} else {
		args = append(args, "-c:a", "copy")
	}
	args = append(args, "-movflags", "frag_keyframe+empty_moov+default_base_moof",
		"-frag_duration", "2000000",
		"-f", "mp4", "-")
	return args
}
