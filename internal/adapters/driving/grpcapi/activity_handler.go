package grpcapi

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/gravity-zero/evey/internal/domain/activity"
	eveyi18n "github.com/gravity-zero/evey/internal/pkg/i18n"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type activityHandler struct {
	eveyv1.UnimplementedActivityServiceServer
	svc        *appactivity.QueryService
	libQuery   *applibrary.QueryService
	userAccess *appuser.AccessService
	translator *eveyi18n.Translator
}

func (h *activityHandler) GetPlayHistory(ctx context.Context, req *eveyv1.GetPlayHistoryRequest) (*eveyv1.GetPlayHistoryResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("activityHandler.GetPlayHistory: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("activityHandler.GetPlayHistory: %w", err)
	}
	filter := activity.PlayHistoryFilter{
		ProfileID:   profileID,
		MediaItemID: req.MediaItemId,
		From:        fromTimestamppbPtr(req.From),
		To:          fromTimestamppbPtr(req.To),
	}
	if req.Pagination != nil {
		filter.Limit, filter.Offset = clampPagination(req.Pagination.PageSize, req.Pagination.Page, 50)
	}
	entries, err := h.svc.GetPlayHistory(ctx, filter)
	if err != nil {
		return nil, internalError("get play history", err)
	}
	resp := &eveyv1.GetPlayHistoryResponse{}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, playHistoryEntryToProto(e))
	}
	return resp, nil
}

func (h *activityHandler) GetUserStats(ctx context.Context, req *eveyv1.GetUserStatsRequest) (*eveyv1.UserStats, error) {
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
		return nil, internalError("get user stats", err)
	}
	totalGenreMs := int64(0)
	for _, gs := range stats.GenreBreakdown {
		totalGenreMs += gs.TotalTimeMs
	}

	pb := &eveyv1.UserStats{
		ProfileId:          stats.ProfileID,
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
			MediaItemId: mp.MediaItemID,
			Title:       mp.Title,
			PlayCount:   int32(mp.PlayCount),
			TotalTimeMs: mp.TotalTimeMs,
		})
	}
	for monthStr, cnt := range stats.PlayCountByMonth {
		var year, month int
		fmt.Sscanf(monthStr, "%d-%d", &year, &month)
		pb.ByMonth = append(pb.ByMonth, &eveyv1.MonthlyPlayCount{
			Year:      int32(year),
			Month:     int32(month),
			PlayCount: int32(cnt),
		})
	}
	return pb, nil
}

func (h *activityHandler) GetServerStats(ctx context.Context, _ *eveyv1.GetServerStatsRequest) (*eveyv1.ServerStats, error) {
	stats, err := h.svc.GetServerStats(ctx)
	if err != nil {
		return nil, internalError("get server stats", err)
	}
	return &eveyv1.ServerStats{
		TotalUsers:      int32(stats.TotalUsers),
		TotalMediaItems: int32(stats.TotalItems),
	}, nil
}

func (h *activityHandler) GetLoginHistory(ctx context.Context, req *eveyv1.GetLoginHistoryRequest) (*eveyv1.GetLoginHistoryResponse, error) {
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
	if req.Pagination != nil {
		limit, offset = clampPagination(req.Pagination.PageSize, req.Pagination.Page, 50)
	}
	entries, err := h.svc.GetLoginHistory(ctx, userID, limit, offset)
	if err != nil {
		return nil, internalError("get login history", err)
	}
	resp := &eveyv1.GetLoginHistoryResponse{}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, loginEntryToProto(e))
	}
	return resp, nil
}

func (h *activityHandler) ListServerMetrics(ctx context.Context, req *eveyv1.ListServerMetricsRequest) (*eveyv1.ListServerMetricsResponse, error) {
	from := fromTimestamppb(req.From)
	to := fromTimestamppb(req.To)
	if to.IsZero() {
		to = time.Now()
	}
	if from.IsZero() {
		from = to.Add(-24 * time.Hour)
	}
	metrics, err := h.svc.GetServerMetrics(ctx, from, to)
	if err != nil {
		return nil, internalError("get metrics", err)
	}
	resp := &eveyv1.ListServerMetricsResponse{}
	for _, m := range metrics {
		resp.Metrics = append(resp.Metrics, serverMetricToProto(m))
	}
	return resp, nil
}

func (h *activityHandler) GetPopularMedia(ctx context.Context, req *eveyv1.GetPopularMediaRequest) (*eveyv1.GetPopularMediaResponse, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	popular, err := h.svc.GetMostPopular(ctx, limit)
	if err != nil {
		return nil, internalError("get popular", err)
	}
	ids := make([]string, len(popular))
	for i, p := range popular {
		ids[i] = p.MediaItemID
	}
	type miInfo struct {
		typ       string
		libraryID string
	}
	infoByID := make(map[string]miInfo, len(ids))
	if len(ids) > 0 && h.libQuery != nil {
		mis, merr := h.libQuery.GetMediaItemsByIDs(ctx, ids)
		if merr != nil {
			slog.Warn("popular: batch fetch media items", slog.Any("err", merr))
		}
		for _, mi := range mis {
			infoByID[mi.ID] = miInfo{typ: mi.Type, libraryID: mi.LibraryID}
		}
	}
	resp := &eveyv1.GetPopularMediaResponse{}
	for _, p := range popular {
		info, exists := infoByID[p.MediaItemID]
		if !exists {
			continue
		}
		if !claims.IsAdmin && h.userAccess != nil {
			hasAccess, aErr := h.userAccess.HasLibraryAccess(ctx, claims.UserID, info.libraryID)
			if aErr != nil || !hasAccess {
				continue
			}
		}
		resp.Items = append(resp.Items, &eveyv1.PopularMediaItem{
			MediaItemId: p.MediaItemID,
			Title:       p.Title,
			Type:        info.typ,
			PlayCount:   int32(p.PlayCount),
		})
	}
	return resp, nil
}

func (h *activityHandler) GetAnalytics(ctx context.Context, _ *eveyv1.GetAnalyticsRequest) (*eveyv1.GetAnalyticsResponse, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || !claims.IsAdmin {
		return nil, status.Error(codes.PermissionDenied, "admin only")
	}
	data, err := h.svc.GetAnalytics(ctx)
	if err != nil {
		return nil, internalError("get analytics", err)
	}
	resp := &eveyv1.GetAnalyticsResponse{
		TotalPlaysToday:    int32(data.TotalPlaysToday),
		TotalPlaysThisWeek: int32(data.TotalPlaysThisWeek),
		ActiveUsersToday:   int32(data.ActiveUsersToday),
	}
	for _, item := range data.TopMedia {
		resp.TopMedia = append(resp.TopMedia, &eveyv1.AnalyticsTopMedia{
			MediaItemId:          item.MediaItemID,
			Title:                item.Title,
			PlayCount:            int32(item.PlayCount),
			TotalDurationSeconds: item.TotalDurationSeconds,
		})
	}
	for _, entry := range data.RecentActivity {
		resp.RecentActivity = append(resp.RecentActivity, &eveyv1.AnalyticsRecentActivity{
			UserId:    entry.UserID,
			Username:  entry.Username,
			MediaTitle: entry.MediaTitle,
			PlayedAt:  timestamppb.New(entry.PlayedAt),
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
