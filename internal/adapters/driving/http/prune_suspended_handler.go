package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"

	applibrary "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// PruneController is the slice of the library service the prune-hold routes use.
type PruneController interface {
	PruneSuspensions() []applibrary.PruneSuspension
	ConfirmPrune(libraryID string) bool
	ScanLibrary(ctx context.Context, libraryID string) (added, updated int, err error)
}

type pruneSuspensionDTO struct {
	LibraryID string `json:"libraryId"`
	Root      string `json:"root"`
	Missing   int    `json:"missing"`
	Total     int    `json:"total"`
}

// pruneSuspensionsHandler handles GET /api/v1/admin/libraries/prune-suspended
// (admin-gated at registration): the roots whose deletions are held back
// because too many of their files vanished in one scan.
func pruneSuspensionsHandler(ctrl PruneController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.admin.PruneSuspensions")
		defer span.End()
		held := ctrl.PruneSuspensions()
		out := make([]pruneSuspensionDTO, 0, len(held))
		for _, s := range held {
			out = append(out, pruneSuspensionDTO{LibraryID: s.LibraryID, Root: s.Root, Missing: s.Missing, Total: s.Total})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"suspensions": out}); err != nil {
			slog.WarnContext(r.Context(), "prune suspensions: write response", slog.Any("err", err))
		}
	}
}

// pruneConfirmHandler handles POST /api/v1/admin/libraries/{id}/prune/confirm
// (admin-gated at registration): arms the one-shot override so the next pass
// deletes under the held roots, then scans the library to run that pass. 409
// when nothing is held, so a stale page cannot arm a deletion for later.
func pruneConfirmHandler(ctrl PruneController, serverCtx context.Context, libraryAccess LibraryAccessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.admin.PruneConfirm")
		defer span.End()
		r = r.WithContext(ctx)
		libraryID := r.PathValue("id")
		if libraryID == "" {
			http.Error(w, `{"message":"missing library id"}`, http.StatusBadRequest)
			return
		}
		if !checkLibraryAccess(ctx, w, HTTPClaimsFromContext(ctx), shared.LibraryID(libraryID), libraryAccess) {
			return
		}
		if !ctrl.ConfirmPrune(libraryID) {
			http.Error(w, `{"message":"no deletion is suspended for this library"}`, http.StatusConflict)
			return
		}
		slog.WarnContext(ctx, "admin confirmed a suspended orphan prune", slog.String("library_id", libraryID))
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					slog.ErrorContext(serverCtx, "panic in confirmed prune scan",
						slog.String("library_id", libraryID), slog.Any("recover", rec), slog.String("stack", string(debug.Stack())))
				}
			}()
			if _, _, err := ctrl.ScanLibrary(serverCtx, libraryID); err != nil {
				slog.WarnContext(serverCtx, "confirmed prune scan failed", slog.String("library_id", libraryID), slog.Any("err", err))
			}
		}()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"started"}`))
	}
}
