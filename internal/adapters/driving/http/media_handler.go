package httpapi

import (
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// --- mediaHandler: stream + download ---

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
