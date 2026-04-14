package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

func snapshotHandler(tokenGen TokenVerifier, frameExt FrameExtractor, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter, baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		claims := HTTPClaimsFromContext(r.Context())

		item, err := mediaGetter.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			return
		}

		posMs, _ := strconv.ParseInt(r.URL.Query().Get("position_ms"), 10, 64)
		posSec := float64(posMs) / 1000.0
		if posSec < 0 {
			posSec = 0
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		data, err := frameExt.ExtractFrame(ctx, item.FilePath, posSec, 640)
		if err != nil {
			http.Error(w, `{"message":"frame extraction failed"}`, http.StatusInternalServerError)
			return
		}

		setCORSHeaders(w, r, baseURL)
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "private, max-age=5")
		w.Write(data)
	}
}
