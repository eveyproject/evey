package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	appbackup "github.com/evey-project/evey/internal/app/backup"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

func registerBackupRoutes(mux *http.ServeMux, svc *appbackup.Service, tokenGen *auth.JWTGenerator, heavyRL, writeRL *ratelimit.Limiter) {
	admin := httpAdminMiddleware(tokenGen)

	mux.HandleFunc("POST /api/v1/admin/backups", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		m, err := svc.CreateBackup(r.Context())
		if err != nil {
			slog.Error("create backup failed", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		slog.Info("backup created", slog.String("id", m.ID), slog.Int64("size", m.Size))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if encErr := json.NewEncoder(w).Encode(map[string]any{
			"id":        m.ID,
			"createdAt": m.CreatedAt.Format(time.RFC3339),
			"version":   m.Version,
			"tables":    m.Tables,
			"size":      m.Size,
		}); encErr != nil {
			slog.Warn("json encode failed", slog.Any("err", encErr))
		}
	})))

	mux.HandleFunc("GET /api/v1/admin/backups", admin(func(w http.ResponseWriter, r *http.Request) {
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
		if encErr := json.NewEncoder(w).Encode(map[string]any{"backups": items}); encErr != nil {
			slog.Warn("json encode failed", slog.Any("err", encErr))
		}
	}))

	mux.HandleFunc("GET /api/v1/admin/backups/{id}", admin(func(w http.ResponseWriter, r *http.Request) {
		backupID := r.PathValue("id")
		m, rc, err := svc.GetBackup(r.Context(), backupID)
		if err != nil {
			http.Error(w, `{"message":"backup not found"}`, http.StatusNotFound)
			return
		}
		defer rc.Close()
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.tar.gz"`, m.ID))
		if _, cpErr := io.Copy(w, rc); cpErr != nil {
			slog.Warn("backup download copy", slog.Any("err", cpErr))
		}
	}))

	mux.HandleFunc("POST /api/v1/admin/backups/restore", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
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
			slog.Error("restore backup failed", slog.Any("err", err))
			http.Error(w, `{"message":"restore failed"}`, http.StatusInternalServerError)
			return
		}
		slog.Info("backup restored successfully")
		w.Header().Set("Content-Type", "application/json")
		if _, writeErr := w.Write([]byte(`{"ok":true}`)); writeErr != nil {
			slog.Warn("write failed", slog.Any("err", writeErr))
		}
	})))

	mux.HandleFunc("DELETE /api/v1/admin/backups/{id}", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		backupID := r.PathValue("id")
		if err := svc.DeleteBackup(r.Context(), backupID); err != nil {
			http.Error(w, `{"message":"backup not found"}`, http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))
}
