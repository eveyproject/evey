package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	appmetadata "github.com/eveyproject/evey/internal/app/metadata"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/eveyproject/evey/internal/pkg/id"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type libraryHandler struct {
	eveyv1.UnimplementedLibraryServiceServer
	cmd         *applibrary.CommandService
	query       *applibrary.QueryService
	metadataSvc *appmetadata.QueryService
	userAccess  *appuser.AccessService
	userQuery   *appuser.QueryService
	translator  *eveyi18n.Translator
	scheduler   *applibrary.Scheduler
}

// scanNewLibrary reads the folder of a library that was just created without
// making the caller wait for it. The request context dies with the response,
// so the scan keeps only its values; a failure is logged, the periodic pass
// still catches up.
func (h *libraryHandler) scanNewLibrary(ctx context.Context, libraryID string) {
	scanCtx := context.WithoutCancel(ctx)
	go func() {
		if _, err := h.cmd.ScanLibraryReport(scanCtx, libraryID); err != nil {
			slog.WarnContext(scanCtx, "initial scan of new library failed", slog.String("library_id", libraryID), slog.Any("err", err))
		}
	}()
}

func (h *libraryHandler) CreateLibrary(ctx context.Context, req *eveyv1.CreateLibraryRequest) (*eveyv1.Library, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.CreateLibrary")
	defer span.End()
	lib, err := library.NewLibrary(
		shared.LibraryID(id.New("lib-")),
		req.Name,
		library.LibraryType(libraryTypeFromProto[req.Type]),
		library.WithLibraryPaths(req.Paths),
		library.WithLibraryScanInterval(360),
		library.WithLibraryScanOnStartup(req.ScanOnStartup),
		library.WithLibraryRealtimeMonitoring(req.RealtimeMonitoring),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		switch {
		case req.Name == "":
			return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.library.name_required")
		case !library.LibraryType(libraryTypeFromProto[req.Type]).IsValid():
			return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.library.type_invalid")
		}
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := h.cmd.CreateLibrary(ctx, lib); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "create library", err)
	}
	// The answer is built before the scan starts: the scan updates the
	// library record it was handed, and the response must read the one the
	// caller created.
	resp := libraryToProto(lib)
	h.scanNewLibrary(ctx, string(lib.ID))
	return resp, nil
}

func (h *libraryHandler) GetLibrary(ctx context.Context, req *eveyv1.GetLibraryRequest) (*eveyv1.Library, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.GetLibrary")
	defer span.End()
	lib, err := h.query.GetLibrary(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "library not found")
		slog.WarnContext(ctx, "grpc GetLibrary: query failed", slog.Any("err", err))
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.library.not_found")
	}
	if ok, accessErr := h.checkLibraryAccess(ctx, lib.ID); !ok {
		if accessErr != nil {
			span.RecordError(accessErr)
			slog.WarnContext(ctx, "check library access", slog.Any("err", accessErr))
		}
		span.SetStatus(otelcodes.Error, "library access denied")
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.library.not_found")
	}
	return libraryToProto(lib), nil
}

func (h *libraryHandler) ListLibraries(ctx context.Context, _ *eveyv1.ListLibrariesRequest) (*eveyv1.ListLibrariesResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.ListLibraries")
	defer span.End()
	libs, err := h.query.ListLibraries(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "list libraries", err)
	}
	libs, err = h.filterLibrariesByAccess(ctx, libs)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "filter libraries", err)
	}
	counts := h.libraryItemCounts(ctx, libs)
	resp := &eveyv1.ListLibrariesResponse{}
	for _, lib := range libs {
		pb := libraryToProto(lib)
		if counts != nil {
			n := int32(counts[lib.ID])
			pb.ItemCount = &n
		}
		resp.Libraries = append(resp.Libraries, pb)
	}
	return resp, nil
}

// libraryItemCounts counts every listed library's media items in one grouped
// query, the number ListMediaItems' total_items answers for that library_id
// alone: every item type, the profile's official_rating pre-filter applied
// (that total never included the strict per-item pass, which runs on a page).
// The phone used to send that request once per library card. nil when the
// count could not be read: the cards then show no counter rather than a wrong
// one, and the list itself still answers.
func (h *libraryHandler) libraryItemCounts(ctx context.Context, libs []*library.Library) map[shared.LibraryID]int {
	if len(libs) == 0 {
		return nil
	}
	ids := make([]shared.LibraryID, len(libs))
	for i, lib := range libs {
		ids[i] = lib.ID
	}
	pf := h.resolveParentalFilter(ctx)
	counts, err := h.query.CountMediaItemsPerLibrary(ctx, library.ListMediaItemsFilter{
		LibraryIDs:     ids,
		AllowedRatings: pf.allowedRatings,
	})
	if err != nil {
		slog.WarnContext(ctx, "count media items per library", slog.Any("err", err))
		return nil
	}
	return counts
}

func (h *libraryHandler) UpdateLibrary(ctx context.Context, req *eveyv1.UpdateLibraryRequest) (*eveyv1.Library, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.UpdateLibrary")
	defer span.End()
	if req.Id == "" {
		span.SetStatus(otelcodes.Error, "id is required")
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.library.id_required")
	}
	lib, err := h.query.GetLibrary(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "library not found")
		slog.WarnContext(ctx, "grpc UpdateLibrary: query failed", slog.Any("err", err))
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.library.not_found")
	}
	// Defense-in-depth IDOR check: RBAC already restricts this RPC to admins,
	// but we re-check here so a future RBAC misconfiguration does not silently
	// expose write access to other libraries.
	if ok, accessErr := h.checkLibraryAccess(ctx, lib.ID); !ok {
		if accessErr != nil {
			span.RecordError(accessErr)
			slog.WarnContext(ctx, "update library access check", slog.Any("err", accessErr))
		}
		span.SetStatus(otelcodes.Error, "library access denied")
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.library.not_found")
	}
	if req.Name != nil {
		lib.Name = *req.Name
	}
	if len(req.Paths) > 0 {
		for _, p := range req.Paths {
			if !filepath.IsAbs(p) || !strings.HasPrefix(filepath.Clean(p), "/media") {
				span.SetStatus(otelcodes.Error, "invalid path")
				return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.library.invalid_path")
			}
		}
		lib.Paths = req.Paths
	}
	if req.RealtimeMonitoring != nil {
		lib.RealtimeMonitoring = *req.RealtimeMonitoring
	}
	if err := h.cmd.UpdateLibrary(ctx, lib); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "update library", err)
	}
	return libraryToProto(lib), nil
}

func (h *libraryHandler) DeleteLibrary(ctx context.Context, req *eveyv1.DeleteLibraryRequest) (*eveyv1.DeleteLibraryResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.DeleteLibrary")
	defer span.End()
	if req.Id == "" {
		span.SetStatus(otelcodes.Error, "id is required")
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.library.id_required")
	}
	// Defense-in-depth IDOR check: RBAC already restricts this RPC to admins,
	// but explicitly re-verify access for non-admin claims so a future RBAC
	// misconfiguration cannot silently expose deletion of other libraries.
	// Admins short-circuit; an unwired userAccess port denies instead of
	// allowing (see checkLibraryAccess).
	if ok, accessErr := h.checkLibraryAccess(ctx, shared.LibraryID(req.Id)); !ok {
		if accessErr != nil {
			span.RecordError(accessErr)
			slog.WarnContext(ctx, "delete library access check", slog.Any("err", accessErr))
		}
		span.SetStatus(otelcodes.Error, "library access denied")
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.library.not_found")
	}
	if err := h.cmd.DeleteLibrary(ctx, req.Id); err != nil {
		// Deleting an already-absent library is not a server error: the desired
		// state (library gone) is achieved. Return idempotent success so a stale
		// admin list that still shows it simply refreshes it away, instead of the
		// 500 the bare repo "not found" used to surface.
		if errors.Is(err, shared.ErrNotFound) {
			slog.Info("delete library: already absent", slog.String("library_id", req.Id))
			return &eveyv1.DeleteLibraryResponse{}, nil
		}
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "delete library", err)
	}
	return &eveyv1.DeleteLibraryResponse{}, nil
}

func (h *libraryHandler) ScanLibrary(req *eveyv1.ScanLibraryRequest, stream eveyv1.LibraryService_ScanLibraryServer) error {
	ctx, span := tracing.StartSpan(stream.Context(), "grpc.library.ScanLibrary")
	defer span.End()
	report, err := h.cmd.ScanLibraryReport(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, applibrary.ErrScanInProgress) {
			span.SetStatus(otelcodes.Error, "scan in progress")
			return h.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.library.scan_in_progress")
		}
		span.SetStatus(otelcodes.Error, err.Error())
		return internalError(ctx, "scan library", err)
	}
	if err := stream.Send(&eveyv1.ScanProgress{
		LibraryId:      req.Id,
		Phase:          eveyv1.ScanPhase_SCAN_PHASE_COMPLETE,
		Percent:        100,
		TotalItems:     int32(report.Added + report.Updated),
		ProcessedItems: int32(report.Added + report.Updated),
		FailedPaths:    report.FailedPaths,
	}); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}
	return nil
}

func (h *libraryHandler) filterLibrariesByAccess(ctx context.Context, libs []*library.Library) ([]*library.Library, error) {
	allowedIDs, err := h.getAllowedLibraryIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("libraryHandler.filterLibrariesByAccess: %w", err)
	}
	if allowedIDs == nil {
		return libs, nil
	}
	allowed := make(map[string]bool, len(allowedIDs))
	for _, id := range allowedIDs {
		allowed[id] = true
	}
	filtered := make([]*library.Library, 0, len(libs))
	for _, lib := range libs {
		if allowed[string(lib.ID)] {
			filtered = append(filtered, lib)
		}
	}
	return filtered, nil
}

func (h *libraryHandler) getAllowedLibraryIDs(ctx context.Context) ([]string, error) {
	// Admin bypass is a claims check, independent of whether userAccess is
	// wired, so it is resolved before the wiring check below - an unwired
	// port must never turn an admin request into a denial.
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return nil, nil
	}
	if h.userAccess == nil {
		// nil reads as "unrestricted" to filterLibrariesByAccess below; a
		// wiring regression must not silently show every library to every
		// non-admin, so return the same non-nil empty set a zero-access user
		// would get.
		return []string{}, nil
	}
	ids, err := h.userAccess.GetLibraryAccessByUserID(ctx, claims.UserID)
	if err != nil {
		// Deny-shaped, NOT nil (mirrors playbackHandler.getAllowedLibraryIDs):
		// nil reads as "unrestricted" everywhere downstream, so returning it on
		// a grant-store outage would WIDEN what the caller sees. Callers that
		// discard this error must still end up denying.
		slog.ErrorContext(ctx, "library: grant lookup failed, denying",
			slog.String("user_id", claims.UserID), slog.Any("err", err))
		return []string{}, fmt.Errorf("libraryHandler.getAllowedLibraryIDs: %w", err)
	}
	// A non-admin with zero grants is UNRESTRICTED (sees every library): the admin
	// panel treats "no library checked" as the default-all opt-out, matching
	// HasAccess (zero restrictions = full access). Grants act as a whitelist only
	// when present; nil here means "unrestricted" to filterLibrariesByAccess.
	if len(ids) == 0 {
		return nil, nil
	}
	return ids, nil
}

// allowedLibraryFilter resolves the caller's allowed set for query-side
// narrowing. An EMPTY filter.LibraryIDs slice reads as "no filter" in the
// query layer (media_item_queries.go guards on len > 0), so a deny-shaped
// empty set (grant lookup failure, unwired port) cannot be expressed through
// the filter: those callers get ok=false and must short-circuit with an
// empty result instead of skipping the narrowing.
func (h *libraryHandler) allowedLibraryFilter(ctx context.Context) (ids []string, restricted, ok bool) {
	allowed, err := h.getAllowedLibraryIDs(ctx)
	if err != nil || (allowed != nil && len(allowed) == 0) {
		return nil, false, false
	}
	if allowed == nil {
		return nil, false, true
	}
	return allowed, true, true
}

func toLibraryIDs(ids []string) []shared.LibraryID {
	out := make([]shared.LibraryID, len(ids))
	for i, id := range ids {
		out[i] = shared.LibraryID(id)
	}
	return out
}

func (h *libraryHandler) checkLibraryAccess(ctx context.Context, libraryID shared.LibraryID) (bool, error) {
	// Callers treat false as "deny" and log the error - see libraryAccessDecision
	// for why a missing port denies rather than allows.
	return libraryAccessDecision(ctx, h.userAccess, string(libraryID), "libraryHandler.checkLibraryAccess")
}

func (h *libraryHandler) GetLibrarySchedule(ctx context.Context, req *eveyv1.GetLibraryScheduleRequest) (*eveyv1.GetLibraryScheduleResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.GetLibrarySchedule")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if !claims.IsAdmin {
		span.SetStatus(otelcodes.Error, "admin only")
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.auth.admin_access_required")
	}
	if h.scheduler == nil {
		return &eveyv1.GetLibraryScheduleResponse{}, nil
	}
	sched, err := h.scheduler.GetSchedule(ctx, req.Id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return &eveyv1.GetLibraryScheduleResponse{}, nil
		}
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "get library schedule", err)
	}
	return &eveyv1.GetLibraryScheduleResponse{
		Schedule: scheduleToProto(sched),
	}, nil
}

func (h *libraryHandler) SetLibrarySchedule(ctx context.Context, req *eveyv1.SetLibraryScheduleRequest) (*eveyv1.LibrarySchedule, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.SetLibrarySchedule")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if !claims.IsAdmin {
		span.SetStatus(otelcodes.Error, "admin only")
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.auth.admin_access_required")
	}
	if h.scheduler == nil {
		span.SetStatus(otelcodes.Error, "scheduler not available")
		return nil, h.translator.TranslatedError(ctx, codes.Unimplemented, "errors.admin.scheduler_not_available")
	}
	if req.Cron == "" {
		span.SetStatus(otelcodes.Error, "cron expression required")
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.library.cron_required")
	}
	sched, err := h.scheduler.SetSchedule(ctx, req.Id, req.Cron, req.Enabled)
	if err != nil {
		span.RecordError(err)
		if strings.HasPrefix(err.Error(), "invalid cron") {
			span.SetStatus(otelcodes.Error, "invalid cron expression")
			slog.WarnContext(ctx, "grpc SetLibrarySchedule: invalid cron expression", slog.Any("err", err))
			return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.library.cron_invalid")
		}
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "set schedule", err)
	}
	return scheduleToProto(sched), nil
}

func (h *libraryHandler) DeleteLibrarySchedule(ctx context.Context, req *eveyv1.DeleteLibraryScheduleRequest) (*eveyv1.DeleteLibraryScheduleResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.DeleteLibrarySchedule")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if !claims.IsAdmin {
		span.SetStatus(otelcodes.Error, "admin only")
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.auth.admin_access_required")
	}
	if h.scheduler != nil {
		if delErr := h.scheduler.DeleteSchedule(ctx, req.Id); delErr != nil {
			span.RecordError(delErr)
			slog.WarnContext(ctx, "delete schedule", slog.Any("err", delErr))
		}
	}
	return &eveyv1.DeleteLibraryScheduleResponse{}, nil
}

func scheduleToProto(s *library.ScanSchedule) *eveyv1.LibrarySchedule {
	pb := &eveyv1.LibrarySchedule{
		LibraryId: string(s.LibraryID),
		Cron:      s.CronExpression,
		Enabled:   s.Enabled,
	}
	if s.LastRunAt != nil {
		pb.LastRunAt = toTimestamppbPtr(s.LastRunAt)
	}
	if s.NextRunAt != nil {
		pb.NextRunAt = toTimestamppbPtr(s.NextRunAt)
	}
	return pb
}
