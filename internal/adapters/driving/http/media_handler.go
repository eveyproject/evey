package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	appsubtitle "github.com/gravity-zero/evey/internal/app/subtitle"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	domainmeta "github.com/gravity-zero/evey/internal/domain/metadata"
	domaintranscoding "github.com/gravity-zero/evey/internal/domain/transcoding"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/hdr"
	"github.com/gravity-zero/evey/internal/pkg/urlvalidator"
)

// filterMasterPlaylist removes #EXT-X-STREAM-INF variants exceeding maxBps (bps).
// If all variants exceed the limit, keeps the one with the lowest BANDWIDTH.
func filterMasterPlaylist(content string, maxBps int64) string {
	if maxBps <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	type variant struct {
		inf string
		uri string
		bw  int64
	}
	var header []string
	var variants []variant
	i := 0
	for i < len(lines) {
		line := lines[i]
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			bw := int64(0)
			attrs := strings.TrimPrefix(line, "#EXT-X-STREAM-INF:")
			if _, after, ok := strings.Cut(attrs, "BANDWIDTH="); ok {
				if _, err := fmt.Sscanf(after, "%d", &bw); err != nil {
					slog.Warn("parse BANDWIDTH", slog.Any("err", err))
				}
			}
			uri := ""
			if i+1 < len(lines) {
				uri = lines[i+1]
				i++
			}
			variants = append(variants, variant{inf: line, uri: uri, bw: bw})
		} else {
			header = append(header, line)
		}
		i++
	}

	var allowed []variant
	for _, v := range variants {
		if v.bw <= maxBps {
			allowed = append(allowed, v)
		}
	}
	if len(allowed) == 0 && len(variants) > 0 {
		lowest := variants[0]
		for _, v := range variants[1:] {
			if v.bw < lowest.bw {
				lowest = v
			}
		}
		allowed = []variant{lowest}
	}

	var out []string
	out = append(out, header...)
	for _, v := range allowed {
		out = append(out, v.inf, v.uri)
	}
	return strings.Join(out, "\n")
}

// --- mediaHandler: stream + artwork ---

type mediaServe struct {
	tokens        TokenVerifier
	libSvc        MediaItemGetter
	artworkRepo   ArtworkReader
	baseURL       string
	libraryAccess LibraryAccessChecker
}

func mediaHandler(tokens TokenVerifier, libSvc MediaItemGetter, artworkRepo ArtworkReader, baseURL string, libraryAccess LibraryAccessChecker) http.HandlerFunc {
	h := &mediaServe{tokens: tokens, libSvc: libSvc, artworkRepo: artworkRepo, baseURL: baseURL, libraryAccess: libraryAccess}
	return func(w http.ResponseWriter, r *http.Request) {
		artType := r.PathValue("artType")
		if artType != "" {
			h.handleArtwork(w, r)
		} else {
			h.handleStream(w, r)
		}
	}
}

func (h *mediaServe) handleStream(w http.ResponseWriter, r *http.Request) {
	mediaID := r.PathValue("id")
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) < 8 {
		if t := r.URL.Query().Get("token"); t != "" {
			authHeader = "Bearer " + t
		}
	}
	if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	claims, err := h.tokens.ValidateAccessToken(authHeader[7:])
	if err != nil {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	item, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		return
	}
	setCORSHeaders(w, r, h.baseURL)
	http.ServeFile(w, r, item.FilePath)
}

func (h *mediaServe) handleArtwork(w http.ResponseWriter, r *http.Request) {
	mediaID := r.PathValue("id")
	artType := r.PathValue("artType")
	artAuthHeader := r.Header.Get("Authorization")
	if len(artAuthHeader) < 8 {
		if t := r.URL.Query().Get("token"); t != "" {
			artAuthHeader = "Bearer " + t
		}
	}
	if len(artAuthHeader) < 8 || artAuthHeader[:7] != "Bearer " {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	artClaims, err := h.tokens.ValidateAccessToken(artAuthHeader[7:])
	if err != nil {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	artItem, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !checkLibraryAccess(r.Context(), w, artClaims, artItem.LibraryID, h.libraryAccess) {
		return
	}

	records := h.findArtworkRecords(r.Context(), mediaID, artType, artItem.ParentID)
	if records == nil {
		http.NotFound(w, r)
		return
	}

	for _, rec := range records {
		if artType == "" || rec.Type == artType {
			if rec.LocalPath != nil && *rec.LocalPath != "" {
				serveFileWithCache(w, r, *rec.LocalPath)
				return
			}
		}
	}
	var artURL string
	for _, rec := range records {
		if artType == "" || rec.Type == artType {
			if rec.SourceURL != nil {
				artURL = *rec.SourceURL
				break
			}
		}
	}
	if artURL == "" {
		http.NotFound(w, r)
		return
	}
	if err := urlvalidator.ValidateRedirectURL(artURL); err != nil {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.Redirect(w, r, artURL, http.StatusFound)
}

func (h *mediaServe) findArtworkRecords(ctx context.Context, mediaID, artType, parentID string) []*domainmeta.ArtworkRecord {
	hasMatchingType := func(recs []*domainmeta.ArtworkRecord) bool {
		for _, rec := range recs {
			if artType == "" || rec.Type == artType {
				return true
			}
		}
		return false
	}
	lookupID := mediaID
	nextParentID := parentID
	for {
		records, err := h.artworkRepo.GetByMediaItem(ctx, lookupID)
		if err != nil {
			return nil
		}
		if hasMatchingType(records) {
			return records
		}
		if nextParentID == "" {
			return nil
		}
		lookupID = nextParentID
		parent, err := h.libSvc.GetMediaItem(ctx, lookupID)
		if err != nil {
			return nil
		}
		nextParentID = parent.ParentID
	}
}

func selectArtworkHandler(tokens TokenVerifier, artworkRepo ArtworkRepo, libSvc MediaItemGetter, libraryAccess LibraryAccessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		claims, err := tokens.ValidateAccessToken(authHeader[7:])
		if err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		if !claims.IsAdmin {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		mediaID := r.PathValue("id")
		artworkID := r.PathValue("artworkId")
		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			return
		}
		records, err := artworkRepo.GetByMediaItem(r.Context(), mediaID)
		if err != nil || len(records) == 0 {
			http.NotFound(w, r)
			return
		}
		var selected *domainmeta.ArtworkRecord
		for _, rec := range records {
			if rec.ID == artworkID {
				selected = rec
				break
			}
		}
		if selected == nil {
			http.NotFound(w, r)
			return
		}
		for _, rec := range records {
			if rec.Type == selected.Type && rec.ID != selected.ID && rec.SortOrder < selected.SortOrder {
				if err := artworkRepo.UpdateSortOrder(r.Context(), rec.ID, selected.SortOrder); err != nil {
					slog.Warn("artwork sort update", slog.Any("err", err))
				}
				break
			}
		}
		if err := artworkRepo.UpdateSortOrder(r.Context(), selected.ID, -1); err != nil {
			slog.Warn("artwork sort update", slog.Any("err", err))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}

func downloadHandler(tokens TokenVerifier, libSvc MediaItemGetter, baseURL string, libraryAccess LibraryAccessChecker) http.HandlerFunc {
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
		claims, err := tokens.ValidateAccessToken(token)
		if err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil || item.FilePath == "" {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			return
		}
		fi, err := os.Stat(item.FilePath)
		if err != nil {
			http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
			return
		}
		filename := sanitizeFilename(filepath.Base(item.FilePath))
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
		setCORSHeaders(w, r, baseURL)
		http.ServeFile(w, r, item.FilePath)
	}
}

func subtitleHandler(tokenGen TokenVerifier, libSvc MediaStreamsGetter, baseURL string, libraryAccess LibraryAccessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		trackStr := r.PathValue("track")
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
		trackIndex, _ := strconv.Atoi(trackStr)
		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil || item.FilePath == "" {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			return
		}

		if streams, serr := libSvc.GetMediaStreams(r.Context(), mediaID); serr == nil {
			subIdx := 0
			for _, s := range streams {
				if s.StreamType != "subtitle" {
					continue
				}
				if subIdx == trackIndex && s.IsExternal && s.ExternalPath != nil {
					serveExternalSubtitle(w, r, *s.ExternalPath, filepath.Dir(item.FilePath), baseURL)
					return
				}
				subIdx++
			}
		}

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		cmd := exec.CommandContext(ctx, "ffmpeg",
			"-i", item.FilePath,
			"-map", fmt.Sprintf("0:s:%d", trackIndex),
			"-f", "webvtt",
			"-",
		)
		w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		setCORSHeaders(w, r, baseURL)
		cmd.Stdout = w
		if err := cmd.Start(); err != nil {
			slog.Error("ffmpeg subtitle start", slog.Any("err", err))
			return
		}
		go func() { <-r.Context().Done(); cancel() }()
		if err := cmd.Wait(); err != nil && ctx.Err() == nil {
			slog.Warn("ffmpeg subtitle", slog.String("media_id", mediaID), slog.Any("err", err))
		}
	}
}

func serveExternalSubtitle(w http.ResponseWriter, r *http.Request, path, baseDir, baseURL string) {
	safePath, err := safeJoin(baseDir, filepath.Base(path))
	if err != nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	ext := strings.ToLower(filepath.Ext(safePath))
	setCORSHeaders(w, r, baseURL)
	w.Header().Set("Cache-Control", "no-cache")
	if ext == ".vtt" {
		w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
		http.ServeFile(w, r, safePath)
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-i", safePath, "-f", "webvtt", "-")
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	cmd.Stdout = w
	if err := cmd.Start(); err != nil {
		slog.Error("ffmpeg external subtitle", slog.Any("err", err))
		return
	}
	go func() { <-r.Context().Done(); cancel() }()
	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		slog.Warn("ffmpeg external subtitle convert", slog.String("path", safePath), slog.Any("err", err))
	}
}

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

	hdrSupported := r.URL.Query().Get("hdr") == "1"
	audioNormalize := h.audioNormalizeGetter != nil && h.audioNormalizeGetter(r.Context())
	session, err := h.hlsManager.GetOrCreate(claims.UserID, mediaID, item.FilePath, sourceWidth, sourceHeight, maxStreams, subtitleTrackIndex, subtitleCodec, mediaStreams, hdrSupported, audioNormalize)
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
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		session.MarkAccess()
		http.ServeFile(w, r, filePath)
	}
}

// --- registerSubtitleRoutes ---

type subtitleRoutes struct {
	subtitleSvc   *appsubtitle.Service
	tokens        TokenVerifier
	mediaGetter   MediaItemGetter
	libraryAccess LibraryAccessChecker
}

func registerSubtitleRoutes(mux *http.ServeMux, subtitleSvc *appsubtitle.Service, tokenGen TokenVerifier, mediaGetter MediaItemGetter, libraryAccess LibraryAccessChecker) {
	h := &subtitleRoutes{subtitleSvc: subtitleSvc, tokens: tokenGen, mediaGetter: mediaGetter, libraryAccess: libraryAccess}
	mux.HandleFunc("GET /api/v1/media/{id}/subtitles/search", h.search)
	mux.HandleFunc("POST /api/v1/media/{id}/subtitles/download", h.download)
	mux.HandleFunc("GET /api/v1/media/{id}/subtitles", h.list)
	mux.HandleFunc("DELETE /api/v1/media/{id}/subtitles/{subtitleId}", h.delete)
}

func (h *subtitleRoutes) requireAuth(w http.ResponseWriter, r *http.Request) (*domainuser.Claims, bool) {
	tok := r.Header.Get("Authorization")
	if strings.HasPrefix(tok, "Bearer ") {
		tok = tok[7:]
	} else if t := r.URL.Query().Get("token"); t != "" {
		tok = t
	} else {
		tok = ""
	}
	if tok == "" {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return nil, false
	}
	claims, err := h.tokens.ValidateAccessToken(tok)
	if err != nil {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return nil, false
	}
	return claims, true
}

func (h *subtitleRoutes) requireMediaAccess(w http.ResponseWriter, r *http.Request, mediaID string) (*domainuser.Claims, bool) {
	claims, ok := h.requireAuth(w, r)
	if !ok {
		return nil, false
	}
	item, err := h.mediaGetter.GetMediaItem(r.Context(), mediaID)
	if err != nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return nil, false
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		return nil, false
	}
	return claims, true
}

func (h *subtitleRoutes) search(w http.ResponseWriter, r *http.Request) {
	mediaID := r.PathValue("id")
	if _, ok := h.requireMediaAccess(w, r, mediaID); !ok {
		return
	}
	language := r.URL.Query().Get("language")
	results, err := h.subtitleSvc.Search(r.Context(), mediaID, language)
	if err != nil {
		slog.Error("subtitle search", slog.Any("err", err))
		http.Error(w, `{"message":"search failed"}`, http.StatusInternalServerError)
		return
	}
	type resultJSON struct {
		FileID   int     `json:"fileId"`
		FileName string  `json:"fileName"`
		Language string  `json:"language"`
		Format   string  `json:"format"`
		Rating   float64 `json:"rating"`
	}
	out := make([]resultJSON, 0, len(results))
	for _, res := range results {
		out = append(out, resultJSON{
			FileID:   res.FileID,
			FileName: res.FileName,
			Language: res.Language,
			Format:   res.Format,
			Rating:   res.Rating,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"results": out}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *subtitleRoutes) download(w http.ResponseWriter, r *http.Request) {
	mediaID := r.PathValue("id")
	if _, ok := h.requireMediaAccess(w, r, mediaID); !ok {
		return
	}
	var body struct {
		FileID   int    `json:"fileId"`
		Language string `json:"language"`
		Format   string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return
	}
	if body.FileID == 0 {
		http.Error(w, `{"message":"fileId required"}`, http.StatusBadRequest)
		return
	}
	sub, err := h.subtitleSvc.Download(r.Context(), mediaID, body.FileID, body.Language, body.Format)
	if err != nil {
		slog.Error("subtitle download", slog.Any("err", err))
		http.Error(w, `{"message":"download failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id":           sub.ID,
		"mediaItemId":  sub.MediaItemID,
		"language":     sub.Language,
		"format":       sub.Format,
		"filePath":     sub.FilePath,
		"source":       sub.Source,
		"downloadedAt": sub.DownloadedAt.UTC().Format(time.RFC3339),
	}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *subtitleRoutes) list(w http.ResponseWriter, r *http.Request) {
	mediaID := r.PathValue("id")
	if _, ok := h.requireMediaAccess(w, r, mediaID); !ok {
		return
	}
	subs, err := h.subtitleSvc.List(r.Context(), mediaID)
	if err != nil {
		slog.Error("subtitle list", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	type subJSON struct {
		ID           string `json:"id"`
		MediaItemID  string `json:"mediaItemId"`
		Language     string `json:"language"`
		Format       string `json:"format"`
		FilePath     string `json:"filePath"`
		Source       string `json:"source"`
		DownloadedAt string `json:"downloadedAt"`
	}
	out := make([]subJSON, 0, len(subs))
	for _, s := range subs {
		out = append(out, subJSON{
			ID:           s.ID,
			MediaItemID:  s.MediaItemID,
			Language:     s.Language,
			Format:       s.Format,
			FilePath:     s.FilePath,
			Source:       s.Source,
			DownloadedAt: s.DownloadedAt.UTC().Format(time.RFC3339),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"subtitles": out}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *subtitleRoutes) delete(w http.ResponseWriter, r *http.Request) {
	mediaID := r.PathValue("id")
	subtitleID := r.PathValue("subtitleId")
	if _, ok := h.requireMediaAccess(w, r, mediaID); !ok {
		return
	}
	subs, err := h.subtitleSvc.List(r.Context(), mediaID)
	if err != nil {
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	found := false
	for _, s := range subs {
		if s.ID == subtitleID {
			found = true
			break
		}
	}
	if !found {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if err := h.subtitleSvc.Delete(r.Context(), subtitleID); err != nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	audioTrack   int
	videoScale   int
	remuxOnly    bool
	startTimeSec float64
}

type transcodeAnalysis struct {
	needsVideoTranscode bool
	needsAudioTranscode bool
	needsToneMap        bool
	hdrInfo             *hdr.HDRInfo
	streams             []*domainlibrary.MediaStream
}

// transcodeHandler is included here for completeness; it was part of handlers.go.
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
	analysis := h.analyzeStreams(r.Context(), mediaID, params.remuxOnly, clientSupportsHDR)

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
	go func() { <-r.Context().Done(); cancel() }()
	if _, err := io.Copy(w, stdout); err != nil && ctx.Err() == nil {
		slog.Warn("transcode copy", slog.Any("err", err))
	}
	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		slog.Warn("ffmpeg transcode", slog.String("media_id", mediaID), slog.Any("err", err))
	}
}

func (h *transcodeServe) parseParams(r *http.Request) transcodeParams {
	p := transcodeParams{}
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
	p.remuxOnly = r.URL.Query().Get("remux") == "1"
	if st := r.URL.Query().Get("startTime"); st != "" {
		if f, err := strconv.ParseFloat(st, 64); err == nil && f > 0 {
			p.startTimeSec = f
		}
	}
	return p
}

func (h *transcodeServe) analyzeStreams(ctx context.Context, mediaID string, remuxOnly, clientSupportsHDR bool) transcodeAnalysis {
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

	a.hdrInfo = hdr.DetectHDR(streams)
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
	args = append(args,
		"-i", filePath,
		"-map", "0:v:0",
		"-map", fmt.Sprintf("0:a:%d", p.audioTrack),
	)

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
