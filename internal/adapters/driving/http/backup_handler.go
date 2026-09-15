package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"io"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	appbackup "github.com/evey-project/evey/internal/app/backup"
	backupdomain "github.com/evey-project/evey/internal/domain/backup"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

func registerBackupRoutes(mux *http.ServeMux, svc *appbackup.Service, tokenGen domainuser.TokenGenerator, heavyRL, writeRL *ratelimit.Limiter) {
	admin := httpAdminMiddleware(tokenGen)

	mux.HandleFunc("POST /api/v1/admin/backups", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.backup.Create")
		defer span.End()
		r = r.WithContext(ctx)
		m, err := svc.CreateBackup(r.Context())
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "create backup failed")
			internalError(ctx, w, "create backup failed", err)
			return
		}
		span.SetAttributes(attribute.String("backup_id", m.ID))
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
			slog.WarnContext(ctx, "json encode failed", slog.Any("err", encErr))
		}
	})))

	mux.HandleFunc("GET /api/v1/admin/backups", admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.backup.List")
		defer span.End()
		r = r.WithContext(ctx)
		list, err := svc.ListBackups(r.Context())
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "list backups failed")
			logInternal(r.Context(), "backups: list", err)
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
			slog.WarnContext(ctx, "json encode failed", slog.Any("err", encErr))
		}
	}))

	mux.HandleFunc("GET /api/v1/admin/backups/{id}", admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.backup.Download")
		defer span.End()
		r = r.WithContext(ctx)
		backupID := r.PathValue("id")
		span.SetAttributes(attribute.String("backup_id", backupID))
		m, rc, err := svc.GetBackup(r.Context(), backupID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "backup not found")
			http.Error(w, `{"message":"backup not found"}`, http.StatusNotFound)
			return
		}
		defer rc.Close()
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.tar.gz"`, m.ID))
		if _, cpErr := io.Copy(w, rc); cpErr != nil {
			slog.WarnContext(ctx, "backup download copy", slog.Any("err", cpErr))
		}
	}))

	mux.HandleFunc("POST /api/v1/admin/backups/restore", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.backup.Restore")
		defer span.End()
		r = r.WithContext(ctx)
		if !heavyRL.Allow(ratelimit.ClientIP(r)) {
			span.SetStatus(otelcodes.Error, "rate limited")
			http.Error(w, `{"message":"too many requests"}`, http.StatusTooManyRequests)
			return
		}
		// A 2 GB restore on a slow link legitimately takes minutes - opt out of the
		// body read deadline so BodyReadTimeoutMiddleware does not cut it off.
		ClearBodyReadDeadline(w)
		r.Body = http.MaxBytesReader(w, r.Body, 2<<30) // 2GB max
		file, _, err := r.FormFile("backup")
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "backup file required")
			http.Error(w, `{"message":"backup file required"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()
		if err := svc.RestoreBackup(r.Context(), file); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "restore failed")
			if errors.Is(err, appbackup.ErrInvalidBackup) {
				slog.WarnContext(ctx, "restore backup rejected: invalid or tampered archive", slog.Any("err", err))
				http.Error(w, `{"message":"backup rejected: invalid or tampered archive"}`, http.StatusUnprocessableEntity)
			} else {
				slog.ErrorContext(ctx, "restore backup failed", slog.Any("err", err))
				http.Error(w, `{"message":"restore failed"}`, http.StatusInternalServerError)
			}
			return
		}
		slog.Info("backup restored successfully")
		w.Header().Set("Content-Type", "application/json")
		if _, writeErr := w.Write([]byte(`{"ok":true}`)); writeErr != nil {
			slog.WarnContext(ctx, "write failed", slog.Any("err", writeErr))
		}
	})))

	mux.HandleFunc("DELETE /api/v1/admin/backups/{id}", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.backup.Delete")
		defer span.End()
		r = r.WithContext(ctx)
		backupID := r.PathValue("id")
		span.SetAttributes(attribute.String("backup_id", backupID))
		if err := svc.DeleteBackup(r.Context(), backupID); err != nil {
			span.RecordError(err)
			if errors.Is(err, backupdomain.ErrNotFound) {
				span.SetStatus(otelcodes.Error, "backup not found")
				http.Error(w, `{"message":"backup not found"}`, http.StatusNotFound)
			} else {
				span.SetStatus(otelcodes.Error, "backup delete failed")
				slog.ErrorContext(ctx, "backup delete failed", slog.String("backup_id", backupID), slog.Any("err", err))
				http.Error(w, `{"message":"backup delete failed"}`, http.StatusInternalServerError)
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))
}
