package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

const maxAvatarSize = 5 << 20 // 5 MB

func registerAvatarRoutes(
	mux *http.ServeMux,
	profileRepo domainuser.ProfileRepository,
	tokenGen domainuser.TokenGenerator,
	dataDir string,
) {
	authMw := httpAuthMiddleware(tokenGen)
	mux.HandleFunc("POST /api/v1/profiles/{profileId}/avatar", authMw(avatarUploadHandler(profileRepo, dataDir)))
	// Avatars are public (non-sensitive): no auth required for serving.
	mux.HandleFunc("GET /api/v1/profiles/{profileId}/avatar", avatarServeHandler(profileRepo, dataDir))
}

func avatarUploadHandler(
	profileRepo domainuser.ProfileRepository,
	dataDir string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.avatar.Upload")
		defer span.End()
		r = r.WithContext(ctx)
		claims := HTTPClaimsFromContext(r.Context())

		profileID := r.PathValue("profileId")
		span.SetAttributes(attribute.String("profile_id", profileID))
		profile, err := profileRepo.GetByID(r.Context(), shared.ProfileID(profileID))
		if err != nil || profile == nil {
			if err != nil {
				span.RecordError(err)
			}
			span.SetStatus(otelcodes.Error, "profile not found")
			http.Error(w, `{"message":"profile not found"}`, http.StatusNotFound)
			return
		}
		if string(profile.UserID) != claims.UserID {
			span.SetStatus(otelcodes.Error, "forbidden")
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}

		// An avatar upload on a slow mobile link may exceed the body read deadline;
		// opt out (BodyReadTimeoutMiddleware) so it is not cut off.
		ClearBodyReadDeadline(w)
		r.Body = http.MaxBytesReader(w, r.Body, maxAvatarSize)
		if err := r.ParseMultipartForm(maxAvatarSize); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "file too large")
			http.Error(w, `{"message":"file too large (max 5MB)"}`, http.StatusBadRequest)
			return
		}

		file, _, err := r.FormFile("avatar")
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "missing avatar file")
			http.Error(w, `{"message":"missing avatar file"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Detect real content type from file magic bytes
		buf := make([]byte, 512)
		n, _ := file.Read(buf)
		ct := http.DetectContentType(buf[:n])
		if _, err := file.Seek(0, 0); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "seek failed")
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}

		if !strings.HasPrefix(ct, "image/") {
			span.SetStatus(otelcodes.Error, "not an image")
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
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "mkdir avatar dir failed")
			slog.Error("create avatar dir", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}

		// Remove old avatar if exists (different extension)
		if profile.AvatarPath != nil {
			oldPath := filepath.Join(dataDir, *profile.AvatarPath)
			if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
				slog.Warn("remove old avatar", slog.String("path", oldPath), slog.Any("err", err))
			}
		}

		filename := filepath.Base(profileID) + ext
		dest := filepath.Join(avatarDir, filename)

		out, err := os.Create(dest)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "create avatar file failed")
			slog.Error("create avatar file", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		outClosed := false
		defer func() {
			if !outClosed {
				out.Close()
			}
		}()

		if _, err := io.Copy(out, file); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "write avatar file failed")
			slog.Error("write avatar file", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}

		outClosed = true
		if err := out.Close(); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "close avatar file failed")
			slog.Error("close avatar file", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}

		avatarPath := filepath.Join("avatars", filename)
		profile.AvatarPath = &avatarPath
		if err := profileRepo.Update(r.Context(), profile); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "update profile failed")
			slog.Error("update profile avatar", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"avatarPath": avatarPath}); err != nil {
			slog.Error("encode avatar response", slog.Any("err", err))
		}
	}
}

func avatarServeHandler(
	profileRepo domainuser.ProfileRepository,
	dataDir string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.avatar.Serve")
		defer span.End()
		r = r.WithContext(ctx)
		profileID := r.PathValue("profileId")
		span.SetAttributes(attribute.String("profile_id", profileID))
		profile, err := profileRepo.GetByID(r.Context(), shared.ProfileID(profileID))
		if err != nil || profile == nil || profile.AvatarPath == nil {
			if err != nil {
				span.RecordError(err)
			}
			span.SetStatus(otelcodes.Error, "profile or avatar not found")
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Cache-Control", "public, max-age=3600")
		fullPath := filepath.Join(dataDir, *profile.AvatarPath)
		absData, _ := filepath.Abs(dataDir)
		absPath, _ := filepath.Abs(fullPath)
		if !strings.HasPrefix(absPath, absData+string(filepath.Separator)) {
			span.SetStatus(otelcodes.Error, "path traversal blocked")
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		http.ServeFile(w, r, fullPath)
	}
}
