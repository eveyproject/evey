package http

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gravity-zero/evey/internal/adapters/driving/auth"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
)

const maxAvatarSize = 5 << 20 // 5 MB

func registerAvatarRoutes(
	mux *http.ServeMux,
	profileRepo domainuser.ProfileRepository,
	tokenGen *auth.JWTGenerator,
	dataDir string,
) {
	mux.HandleFunc("POST /api/v1/profiles/{profileId}/avatar", avatarUploadHandler(profileRepo, tokenGen, dataDir))
	mux.HandleFunc("GET /api/v1/profiles/{profileId}/avatar", avatarServeHandler(profileRepo, tokenGen, dataDir))
}

func avatarUploadHandler(
	profileRepo domainuser.ProfileRepository,
	tokenGen *auth.JWTGenerator,
	dataDir string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}

		profileID := r.PathValue("profileId")
		profile, err := profileRepo.GetByID(r.Context(), profileID)
		if err != nil || profile == nil {
			http.Error(w, `{"message":"profile not found"}`, http.StatusNotFound)
			return
		}
		if profile.UserID != claims.UserID {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxAvatarSize)
		if err := r.ParseMultipartForm(maxAvatarSize); err != nil {
			http.Error(w, `{"message":"file too large (max 5MB)"}`, http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("avatar")
		if err != nil {
			http.Error(w, `{"message":"missing avatar file"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()

		ct := header.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "image/") {
			http.Error(w, `{"message":"file must be an image"}`, http.StatusBadRequest)
			return
		}

		ext := ".jpg"
		switch {
		case strings.Contains(ct, "png"):
			ext = ".png"
		case strings.Contains(ct, "webp"):
			ext = ".webp"
		case strings.Contains(ct, "gif"):
			ext = ".gif"
		}

		avatarDir := filepath.Join(dataDir, "avatars")
		if err := os.MkdirAll(avatarDir, 0o755); err != nil {
			slog.Error("create avatar dir", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}

		filename := fmt.Sprintf("%s%s", profileID, ext)
		dest := filepath.Join(avatarDir, filename)

		out, err := os.Create(dest)
		if err != nil {
			slog.Error("create avatar file", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		defer out.Close()

		if _, err := io.Copy(out, file); err != nil {
			slog.Error("write avatar file", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}

		avatarPath := filepath.Join("avatars", filename)
		profile.AvatarPath = &avatarPath
		if err := profileRepo.Update(r.Context(), *profile); err != nil {
			slog.Error("update profile avatar", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"avatarPath":"%s"}`, avatarPath)
	}
}

func avatarServeHandler(
	profileRepo domainuser.ProfileRepository,
	tokenGen *auth.JWTGenerator,
	dataDir string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		if _, err := tokenGen.ValidateAccessToken(token); err != nil {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}

		profileID := r.PathValue("profileId")
		profile, err := profileRepo.GetByID(r.Context(), profileID)
		if err != nil || profile == nil || profile.AvatarPath == nil {
			http.NotFound(w, r)
			return
		}

		fullPath := filepath.Join(dataDir, *profile.AvatarPath)
		http.ServeFile(w, r, fullPath)
	}
}
