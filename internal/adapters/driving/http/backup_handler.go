package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	appbackup "github.com/gravity-zero/evey/internal/app/backup"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
)

func registerBackupRoutes(mux *http.ServeMux, svc *appbackup.Service, tokenGen *auth.JWTGenerator, heavyRL *ratelimit.Limiter) {
	requireAdminLocal := func(r *http.Request) bool {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			return false
		}
		claims, err := tokenGen.ValidateAccessToken(strings.TrimPrefix(h, "Bearer "))
		return err == nil && claims.IsAdmin
	}

	mux.HandleFunc("POST /api/v1/admin/backups", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		m, err := svc.CreateBackup(r.Context())
		if err != nil {
			slog.Error("create backup", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"id":        m.ID,
			"createdAt": m.CreatedAt.Format(time.RFC3339),
			"version":   m.Version,
			"tables":    m.Tables,
			"size":      m.Size,
		}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	})

	mux.HandleFunc("GET /api/v1/admin/backups", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		list, err := svc.ListBackups(r.Context())
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		type item struct {
			ID        string `json:"id"`
			CreatedAt string `json:"createdAt"`
			Version   string `json:"version"`
			Size      int64  `json:"size"`
		}
		items := make([]item, 0, len(list))
		for _, m := range list {
			items = append(items, item{
				ID:        m.ID,
				CreatedAt: m.CreatedAt.Format(time.RFC3339),
				Version:   m.Version,
				Size:      m.Size,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"backups": items}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	})

	mux.HandleFunc("GET /api/v1/admin/backups/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		backupID := r.PathValue("id")
		m, rc, err := svc.GetBackup(r.Context(), backupID)
		if err != nil {
			http.Error(w, `{"message":"backup not found"}`, http.StatusNotFound)
			return
		}
		defer rc.Close()
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.gz"`, m.ID))
		if _, err := io.Copy(w, rc); err != nil {
			slog.Warn("backup download copy", slog.Any("err", err))
		}
	})

	mux.HandleFunc("POST /api/v1/admin/backups/restore", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		if !heavyRL.Allow(ratelimit.ClientIP(r)) {
			http.Error(w, `{"message":"too many requests"}`, http.StatusTooManyRequests)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2<<30) // 2GB max
		file, _, err := r.FormFile("backup")
		if err != nil {
			http.Error(w, `{"message":"backup file required"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()
		if err := svc.RestoreBackup(r.Context(), file); err != nil {
			slog.Error("restore backup", slog.Any("err", err))
			http.Error(w, `{"message":"restore failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
			slog.Warn("write failed", slog.Any("err", err))
		}
	})

	mux.HandleFunc("DELETE /api/v1/admin/backups/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		backupID := r.PathValue("id")
		if err := svc.DeleteBackup(r.Context(), backupID); err != nil {
			http.Error(w, `{"message":"backup not found"}`, http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
