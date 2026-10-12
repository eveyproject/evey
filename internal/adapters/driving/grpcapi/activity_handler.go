package grpcapi

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	appactivity "github.com/eveyproject/evey/internal/app/activity"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type activityHandler struct {
	eveyv1.UnimplementedActivityServiceServer
	svc        *appactivity.QueryService
	cmd        *appactivity.CommandService
	libQuery   *applibrary.QueryService
	userAccess *appuser.AccessService
	userQuery  *appuser.QueryService
	translator *eveyi18n.Translator
}

// RecordPageView logs a client navigation as a "page_view" audit entry, so
// admins can see which pages users visit. Best-effort: a failure or an empty/
// oversized path is swallowed (return OK) - page-view logging must never break
// navigation. UA/device/IP are captured from the request by the interceptors.
func (h *activityHandler) RecordPageView(ctx context.Context, req *eveyv1.RecordPageViewRequest) (*eveyv1.RecordPageViewResponse, error) {
	resp := &eveyv1.RecordPageViewResponse{}
	if h.cmd == nil {
		return resp, nil
	}
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("activityHandler.RecordPageView: %w", err)
	}
	path := strings.TrimSpace(req.Path)
	if path == "" || len(path) > 512 || !strings.HasPrefix(path, "/") {
		return resp, nil // ignore junk; not an error
	}
	e := &activity.AuditLogEntry{
		UserID:       shared.UserID(claims.UserID),
		ProfileID:    shared.ProfileID(claims.ProfileID),
		Action:       "page_view",
		ResourceType: "page",
		Details:      map[string]any{"path": path},
	}
	if ua := interceptors.UserAgentFromCtx(ctx); ua != "" {
		e.UserAgent = &ua
	}
	if dn := interceptors.DeviceNameFromCtx(ctx); dn != "" {
		e.DeviceName = &dn
	}
	if ip := interceptors.ClientIPFromCtx(ctx); ip != "" {
		e.ClientIP = &ip
	}
	if err := h.cmd.CreateAuditEntry(ctx, e); err != nil {
		slog.WarnContext(ctx, "record page view", slog.String("path", path), slog.Any("err", err))
	}
	return resp, nil
}

func (h *activityHandler) GetPlayHistory(ctx context.Context, req *eveyv1.GetPlayHistoryRequest) (*eveyv1.GetPlayHistoryResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.activity.GetPlayHistory")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("activityHandler.GetPlayHistory: %w", err)
	}
	// Two different questions share this RPC. A normal caller asks "what did I
	// watch?" and stays pinned to its own profile. An admin asks "who watched
	// what?" - any profile, any account, or (both ids empty) the whole server.
	// Only IsAdmin widens it; user_id from anyone else is dropped, not honored.
	profileID, userID := req.ProfileId, req.UserId
	if !claims.IsAdmin {
		own, resolveErr := resolveProfileID(claims, req.ProfileId)
		if resolveErr != nil {
			return nil, fmt.Errorf("activityHandler.GetPlayHistory: %w", resolveErr)
		}
		profileID, userID = own, ""
	}
	filter := activity.PlayHistoryFilter{
		ProfileID:   shared.ProfileID(profileID),
		UserID:      shared.UserID(userID),
		MediaItemID: shared.MediaItemID(req.MediaItemId),
		From:        fromTimestamppbPtr(req.From),
		To:          fromTimestamppbPtr(req.To),
	}
	if req.Pagination != nil {
		filter.Limit, filter.Offset = clampPagination(req.Pagination.PageSize, req.Pagination.Page, 50)
		if req.Pagination.Cursor != "" {
			c, err := decodeTimeCursor(req.Pagination.Cursor)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "invalid cursor: %v", err)
			}
			filter.Cursor = &activity.PlayHistoryCursor{StartedAt: c.t, SessionID: c.id}
			filter.Offset = 0
		}
	}
	entries, err := h.svc.GetPlayHistory(ctx, filter)
	if err != nil {
		return nil, internalError(ctx, "get play history", err)
	}
	// Real total across all matching sessions (a COUNT) - was never set, so the
	// UI always showed "0 entrées au total" despite rendering rows.
	total, err := h.svc.CountPlayHistory(ctx, filter)
	if err != nil {
		return nil, internalError(ctx, "count play history", err)
	}
	resp := &eveyv1.GetPlayHistoryResponse{Pagination: &eveyv1.PaginationResponse{TotalItems: int32(total)}} //nolint:gosec // count fits int32
	// The cursor above is taken from the unfiltered page, so a gated profile
	// may get a shorter page; total_items stays the unfiltered count, like the
	// neighbouring history RPCs.
	visible := h.filterPlayHistoryByAccess(ctx, entries)
	for _, e := range visible {
		resp.Entries = append(resp.Entries, playHistoryEntryToProto(e))
	}
	if len(entries) > 0 && filter.Limit > 0 && len(entries) >= filter.Limit {
		last := entries[len(entries)-1]
		resp.Pagination.NextCursor = encodeTimeCursor(last.StartedAt, last.ID)
	}
	return resp, nil
}

// filterPlayHistoryByAccess applies the same gate as GetWatchHistory: a gated
// profile (library grants or an enforced parental filter) only sees entries
// whose media resolves, sits in an allowed library and is not blocked by the
// parental rating. An unresolved (deleted) item stays hidden since access
// cannot be verified. An ungated caller gets the entries unchanged.
func (h *activityHandler) filterPlayHistoryByAccess(ctx context.Context, entries []*activity.PlayHistoryEntry) []*activity.PlayHistoryEntry {
	// Reuse the playback handler's grant lookup (admin bypass, deny when the
	// access port is unwired, deny on lookup error) instead of a second copy.
	allowedIDs := allowedLibraryIDsFor(ctx, h.userAccess)
	pf := resolveParentalFilterFromUserQuery(ctx, h.userQuery)
	if allowedIDs == nil && !pf.enforced() {
		return entries
	}
	byID := map[string]*library.MediaItem{}
	if h.libQuery != nil {
		ids := make([]string, 0, len(entries))
		for _, e := range entries {
			ids = append(ids, string(e.MediaItemID))
		}
		byID = fetchItemMap(ctx, h.libQuery.GetMediaItemsByIDs, ids)
	}
	out := make([]*activity.PlayHistoryEntry, 0, len(entries))
	for _, e := range entries {
		mi := byID[string(e.MediaItemID)]
		if mi == nil {
			continue
		}
		if allowedIDs != nil && !slices.Contains(allowedIDs, string(mi.LibraryID)) {
			continue
		}
		if mediaItemBlockedByParental(pf, mi) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func (h *activityHandler) GetUserStats(ctx context.Context, req *eveyv1.GetUserStatsRequest) (*eveyv1.UserStats, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.activity.GetUserStats")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("activityHandler.GetUserStats: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("activityHandler.GetUserStats: %w", err)
	}
	stats, err := h.svc.GetUserStats(ctx, profileID)
	if err != nil {
		return nil, internalError(ctx, "get user stats", err)
	}
	totalGenreMs := int64(0)
	for _, gs := range stats.GenreBreakdown {
		totalGenreMs += gs.TotalTimeMs
	}

	pb := &eveyv1.UserStats{
		ProfileId:          string(stats.ProfileID),
		TotalWatchTimeMs:   stats.TotalWatchTimeMs,
		TotalPlays:         int32(stats.TotalPlayCount),
		UniqueItemsWatched: int32(stats.UniqueItemsWatched),
	}
	for genre, gs := range stats.GenreBreakdown {
		var pct float64
		if totalGenreMs > 0 {
			pct = float64(gs.TotalTimeMs) / float64(totalGenreMs) * 100
		}
		pb.Genres = append(pb.Genres, &eveyv1.GenreBreakdown{
			Genre:       genre,
			TotalTimeMs: gs.TotalTimeMs,
			PlayCount:   int32(gs.PlayCount),
			Percent:     pct,
		})
	}
	for _, mp := range stats.MostPlayedItems {
		pb.MostPlayed = append(pb.MostPlayed, &eveyv1.MostPlayed{
			MediaItemId: string(mp.MediaItemID),
			Title:       mp.Title,
			PlayCount:   int32(mp.PlayCount),
			TotalTimeMs: mp.TotalTimeMs,
		})
	}
	for monthStr, m := range stats.PlayByMonth {
		var year, month int
		fmt.Sscanf(monthStr, "%d-%d", &year, &month)
		pb.ByMonth = append(pb.ByMonth, &eveyv1.MonthlyPlayCount{
			Year:        int32(year),
			Month:       int32(month),
			PlayCount:   int32(m.PlayCount),
			TotalTimeMs: m.TotalTimeMs,
		})
	}
	return pb, nil
}

func (h *activityHandler) GetServerStats(ctx context.Context, _ *eveyv1.GetServerStatsRequest) (*eveyv1.ServerStats, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.activity.GetServerStats")
	defer span.End()
	stats, err := h.svc.GetServerStats(ctx)
	if err != nil {
		return nil, internalError(ctx, "get server stats", err)
	}
	out := &eveyv1.ServerStats{
		TotalUsers:          int32(stats.TotalUsers),
		TotalMediaItems:     int32(stats.TotalItems),
		TotalPlays:          int32(stats.TotalPlays),
		TotalPlayTimeMs:     stats.TotalPlayTimeMs,
		TotalBandwidthBytes: stats.TotalBandwidthBytes,
		TotalStorageBytes:   stats.TotalStorageBytes,
	}
	for _, m := range stats.PopularMedia {
		out.PopularMedia = append(out.PopularMedia, &eveyv1.PopularMedia{
			MediaItemId:   m.MediaItemID,
			Title:         m.Title,
			Type:          mediaItemTypeToProto[m.Type],
			PlayCount:     int32(m.PlayCount),
			UniqueViewers: int32(m.UniqueViewers),
		})
	}
	for _, u := range stats.ActiveUsers {
		out.ActiveUsers = append(out.ActiveUsers, &eveyv1.ActiveUser{
			UserId:           u.UserID,
			Username:         u.Username,
			PlayCount:        int32(u.PlayCount),
			TotalWatchTimeMs: u.TotalWatchTimeMs,
			LastActiveAt:     timestamppb.New(u.LastActiveAt),
		})
	}
	return out, nil
}

func (h *activityHandler) GetLoginHistory(ctx context.Context, req *eveyv1.GetLoginHistoryRequest) (*eveyv1.GetLoginHistoryResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.activity.GetLoginHistory")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("activityHandler.GetLoginHistory: %w", err)
	}
	// Non-admin can only view their own login history
	userID := req.UserId
	if userID == "" {
		userID = claims.UserID
	} else if userID != claims.UserID && !claims.IsAdmin {
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.activity.cannot_access_other_history")
	}
	limit, offset := 50, 0
	var cursor *activity.LoginHistoryCursor
	if req.Pagination != nil {
		limit, offset = clampPagination(req.Pagination.PageSize, req.Pagination.Page, 50)
		if req.Pagination.Cursor != "" {
			c, err := decodeTimeCursor(req.Pagination.Cursor)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "invalid cursor: %v", err)
			}
			cursor = &activity.LoginHistoryCursor{StartedAt: c.t, EntryID: c.id}
		}
	}
	var entries []*activity.LoginEntry
	if cursor != nil || req.SuccessOnly || req.From != nil || req.To != nil {
		entries, err = h.svc.GetLoginHistoryCursor(ctx, userID, req.SuccessOnly,
			fromTimestamppbPtr(req.From), fromTimestamppbPtr(req.To), cursor, limit)
	} else {
		entries, err = h.svc.GetLoginHistory(ctx, userID, limit, offset)
	}
	if err != nil {
		return nil, internalError(ctx, "get login history", err)
	}
	resp := &eveyv1.GetLoginHistoryResponse{Pagination: &eveyv1.PaginationResponse{}}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, loginEntryToProto(e))
	}
	if len(entries) > 0 && len(entries) >= limit {
		last := entries[len(entries)-1]
		resp.Pagination.NextCursor = encodeTimeCursor(last.CreatedAt, last.ID)
	}
	return resp, nil
}

func (h *activityHandler) ListServerMetrics(ctx context.Context, req *eveyv1.ListServerMetricsRequest) (*eveyv1.ListServerMetricsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.activity.ListServerMetrics")
	defer span.End()
	from, fromOK := fromTimestamppb(req.From)
	to, toOK := fromTimestamppb(req.To)
	if !toOK {
		to = time.Now()
	}
	if !fromOK {
		from = to.Add(-24 * time.Hour)
	}
	metrics, err := h.svc.GetServerMetrics(ctx, from, to)
	if err != nil {
		return nil, internalError(ctx, "get metrics", err)
	}
	resp := &eveyv1.ListServerMetricsResponse{}
	for _, m := range metrics {
		resp.Metrics = append(resp.Metrics, serverMetricToProto(m))
	}
	return resp, nil
}

func (h *activityHandler) GetPopularMedia(ctx context.Context, req *eveyv1.GetPopularMediaRequest) (*eveyv1.GetPopularMediaResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.activity.GetPopularMedia")
	defer span.End()
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	} else if limit > maxPageSize {
		limit = maxPageSize
	}
	popular, err := h.svc.GetMostPopular(ctx, claims.ProfileID, limit)
	if err != nil {
		return nil, internalError(ctx, "get popular", err)
	}
	ids := make([]string, len(popular))
	for i, p := range popular {
		ids[i] = string(p.MediaItemID)
	}
	mediaByID := make(map[string]*library.MediaItem, len(ids))
	if len(ids) > 0 && h.libQuery != nil {
		mis, merr := h.libQuery.GetMediaItemsByIDs(ctx, ids)
		if merr != nil {
			slog.WarnContext(ctx, "popular: batch fetch media items", slog.Any("err", merr))
		}
		for _, mi := range mis {
			mediaByID[string(mi.ID)] = mi
		}
	}
	var allowedLibs map[string]bool
	if !claims.IsAdmin && h.userAccess != nil {
		libIDs, aErr := h.userAccess.GetLibraryAccessByUserID(ctx, claims.UserID)
		// Fail closed: swallowing this error left allowedLibs nil, which the
		// filter below reads as "unrestricted" → a non-admin would see popular
		// titles from libraries they can't access.
		if aErr != nil {
			return nil, internalError(ctx, "popular media: library access", aErr)
		}
		if len(libIDs) > 0 {
			allowedLibs = make(map[string]bool, len(libIDs))
			for _, id := range libIDs {
				allowedLibs[id] = true
			}
		}
	}
	pf := resolveParentalFilterFromUserQuery(ctx, h.userQuery)
	resp := &eveyv1.GetPopularMediaResponse{}
	for _, p := range popular {
		mi, exists := mediaByID[string(p.MediaItemID)]
		if !exists {
			continue
		}
		if allowedLibs != nil && !allowedLibs[string(mi.LibraryID)] {
			continue
		}
		if mediaItemBlockedByParental(pf, mi) {
			continue
		}
		resp.Items = append(resp.Items, &eveyv1.PopularMediaItem{
			MediaItemId: string(p.MediaItemID),
			Title:       p.Title,
			Type:        string(mi.Type),
			PlayCount:   int32(p.PlayCount),
		})
	}
	return resp, nil
}

func (h *activityHandler) GetAnalytics(ctx context.Context, _ *eveyv1.GetAnalyticsRequest) (*eveyv1.GetAnalyticsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.activity.GetAnalytics")
	defer span.End()
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || !claims.IsAdmin {
		return nil, status.Error(codes.PermissionDenied, "admin only")
	}
	data, err := h.svc.GetAnalytics(ctx)
	if err != nil {
		return nil, internalError(ctx, "get analytics", err)
	}
	resp := &eveyv1.GetAnalyticsResponse{
		TotalPlaysToday:    int32(data.TotalPlaysToday),
		TotalPlaysThisWeek: int32(data.TotalPlaysThisWeek),
		ActiveUsersToday:   int32(data.ActiveUsersToday),
	}
	for _, item := range data.TopMedia {
		resp.TopMedia = append(resp.TopMedia, &eveyv1.AnalyticsTopMedia{
			MediaItemId:          string(item.MediaItemID),
			Title:                item.Title,
			PlayCount:            int32(item.PlayCount),
			TotalDurationSeconds: item.TotalDurationSeconds,
		})
	}
	for _, entry := range data.RecentActivity {
		resp.RecentActivity = append(resp.RecentActivity, &eveyv1.AnalyticsRecentActivity{
			UserId:     string(entry.UserID),
			Username:   entry.Username,
			MediaTitle: entry.MediaTitle,
			PlayedAt:   timestamppb.New(entry.PlayedAt),
		})
	}
	for _, d := range data.DailyPlays {
		resp.DailyPlays = append(resp.DailyPlays, &eveyv1.AnalyticsDailyPlayCount{
			Date:             d.Date,
			PlayCount:        int32(d.PlayCount),
			WatchTimeSeconds: d.WatchTimeSeconds,
		})
	}
	for _, m := range data.PlayMethods {
		resp.PlayMethods = append(resp.PlayMethods, &eveyv1.AnalyticsPlayMethod{
			Method: m.Method,
			Count:  int32(m.Count),
		})
	}
	for _, h := range data.HourlyPlays {
		resp.HourlyPlays = append(resp.HourlyPlays, &eveyv1.AnalyticsHourlyPlayCount{
			Hour:  int32(h.Hour),
			Count: int32(h.Count),
		})
	}
	return resp, nil
}
