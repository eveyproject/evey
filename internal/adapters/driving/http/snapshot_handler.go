package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	domainuser "github.com/gravity-zero/evey/internal/domain/user"
)

func snapshotHandler(tokenGen TokenVerifier, frameExt FrameExtractor, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter, baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")

		var jwtClaims *domainuser.Claims
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
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		jwtClaims = claims

		item, err := mediaGetter.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, jwtClaims, item.LibraryID, libraryAccess) {
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
