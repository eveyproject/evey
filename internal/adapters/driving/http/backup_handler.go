package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	appbackup "github.com/eveyproject/evey/internal/app/backup"
	appmedia "github.com/eveyproject/evey/internal/app/media"
	backupdomain "github.com/eveyproject/evey/internal/domain/backup"
	"github.com/eveyproject/evey/internal/pkg/metrics"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// backupDownloadTokenTTL is how long a minted download link stays usable:
// long enough for the browser or phone to start the transfer, no more.
const backupDownloadTokenTTL = 2 * time.Minute

func registerBackupRoutes(mux *http.ServeMux, svc *appbackup.Service, tokenGen domainuser.TokenGenerator, tokens *appmedia.StreamTokenStore, heavyRL, writeRL *ratelimit.Limiter) {
	admin := httpAdminMiddleware(tokenGen)

	// An anchor or a native download cannot send the Authorization header, and
	// the account JWT is no longer accepted in an admin URL: the admin mints a
	// short token bound to THIS backup (header-authenticated), and the URL
	// carries that instead.
	mux.HandleFunc("POST /api/v1/admin/backups/{id}/download-token", admin(func(w http.ResponseWriter, r *http.Request) {
		claims := HTTPClaimsFromContext(r.Context())
		backupID := r.PathValue("id")
		if tokens == nil || claims == nil || backupID == "" || len(backupID) > 128 {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		token := tokens.CreateBackupDownload(shared.UserID(claims.UserID), backupID, backupDownloadTokenTTL)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"token": token}); err != nil {
			slog.WarnContext(r.Context(), "backup download token encode failed", slog.Any("err", err))
		}
	}))

	mux.HandleFunc("POST /api/v1/admin/backups", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.backup.Create")
		defer span.End()
		r = r.WithContext(ctx)
		m, err := svc.CreateBackup(r.Context())
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "create backup failed")
			if errors.Is(err, appbackup.ErrOperationInProgress) {
				// A restore holds the database: nothing failed, try again later.
				http.Error(w, `{"message":"a backup or restore is already running"}`, http.StatusConflict)
				return
			}
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

	mux.HandleFunc("GET /api/v1/admin/backups/{id}", backupDownloadAuth(admin, func(w http.ResponseWriter, r *http.Request) {
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
			} else if errors.Is(err, appbackup.ErrOperationInProgress) {
				http.Error(w, `{"message":"a backup or restore is already running"}`, http.StatusConflict)
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

// backupDownloadAuth lets a backup download through on the admin JWT, or on a
// backup-download token bound to this very backup id; any other token is
// handed to the admin gate, which refuses it.
func backupDownloadAuth(admin func(http.HandlerFunc) http.HandlerFunc, next http.HandlerFunc) http.HandlerFunc {
	viaAdmin := admin(next)
	return func(w http.ResponseWriter, r *http.Request) {
		if st, ok := StreamTokenFromContext(r.Context()); ok && st.Scope == shared.ScopeBackupDownload {
			if st.MediaItemID != shared.MediaItemID(r.PathValue("id")) {
				metrics.AuthzDeniedTotal.WithLabelValues("http", "backup_token_scope_mismatch").Inc()
				http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
				return
			}
			next(w, r)
			return
		}
		viaAdmin(w, r)
	}
}
