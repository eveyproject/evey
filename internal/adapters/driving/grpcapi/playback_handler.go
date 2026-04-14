package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appactivity "github.com/evey-project/evey/internal/app/activity"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appplayback "github.com/evey-project/evey/internal/app/playback"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/playback"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/transcoding"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/ratings"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type playbackHandler struct {
	eveyv1.UnimplementedPlaybackServiceServer
	cmd            *appplayback.CommandService
	query          *appplayback.QueryService
	librarySvc     *applibrary.QueryService
	activitySvc    *appactivity.CommandService
	userQuery      *appuser.QueryService
	userAccess     *appuser.AccessService
	translator     *eveyi18n.Translator
	hlsManager     *apptranscoding.HLSManager
}

func (h *playbackHandler) audit(ctx context.Context, userID, action, resourceType string, resourceID *string, details map[string]any) {
	if h.activitySvc == nil {
		return
	}
	e := &activity.AuditLogEntry{
		UserID:       shared.UserID(userID),
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      details,
	}
	if ip := interceptors.ClientIPFromCtx(ctx); ip != "" {
		e.ClientIP = &ip
	}
	if ua := interceptors.UserAgentFromCtx(ctx); ua != "" {
		e.UserAgent = &ua
	}
	if dn := interceptors.DeviceNameFromCtx(ctx); dn != "" {
		e.DeviceName = &dn
	}
	if err := h.activitySvc.CreateAuditEntry(ctx, e); err != nil {
		slog.Warn("audit log failed", slog.String("action", action), slog.Any("err", err))
	}
}

func (h *playbackHandler) GetStreamInfo(ctx context.Context, req *eveyv1.GetStreamInfoRequest) (*eveyv1.StreamInfo, error) {
	if err := h.requireMediaAccess(ctx, req.MediaItemId); err != nil {
		return nil, fmt.Errorf("playbackHandler.GetStreamInfo: %w", err)
	}
	item, err := h.librarySvc.GetMediaItem(ctx, req.MediaItemId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "media item not found: %v", err)
	}
	resp := &eveyv1.StreamInfo{
		MediaItemId: req.MediaItemId,
		Container:   item.ContainerFormat,
	}
	streams, err := h.librarySvc.GetMediaStreams(ctx, req.MediaItemId)
	if err != nil {
		slog.Warn("get media streams", slog.String("media_item_id", req.MediaItemId), slog.Any("err", err))
	}
	for _, s := range streams {
		resp.MediaStreams = append(resp.MediaStreams, mediaStreamToProto(s))
	}

	client := transcoding.DefaultWebProfile
	if len(req.SupportedCodecs) > 0 {
		client.VideoCodecs = req.SupportedCodecs
	}
	if len(req.SupportedAudioCodecs) > 0 {
		client.AudioCodecs = req.SupportedAudioCodecs
	}
	if len(req.SupportedContainers) > 0 {
		client.Containers = req.SupportedContainers
	}
	if req.MaxWidth > 0 {
		client.MaxWidth = int(req.MaxWidth)
	}
	if req.MaxHeight > 0 {
		client.MaxHeight = int(req.MaxHeight)
	}

	var maxBitrateKbps int
	if req.MaxBitrate > 0 {
		maxBitrateKbps = int(req.MaxBitrate / 1000)
	}

	decision := apptranscoding.MakePlaybackDecision(streams, item.ContainerFormat, client, maxBitrateKbps)
	resp.TranscodeReasons = decision.TranscodeReasons
	resp.VideoDecision = decision.VideoDecision
	resp.AudioDecision = decision.AudioDecision

	switch decision.PlayMethod {
	case "direct_play":
		resp.PlayMethod = eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY
		resp.DirectPlayUrl = "/api/v1/media/" + req.MediaItemId + "/stream"
	case "direct_stream":
		resp.PlayMethod = eveyv1.PlayMethod_PLAY_METHOD_DIRECT_STREAM
		resp.DirectPlayUrl = "/api/v1/media/" + req.MediaItemId + "/stream"
	default:
		resp.PlayMethod = eveyv1.PlayMethod_PLAY_METHOD_TRANSCODE
		resp.TranscodeUrl = "/api/v1/media/" + req.MediaItemId + "/hls/master.m3u8"
	}

	return resp, nil
}

func (h *playbackHandler) StartStream(ctx context.Context, req *eveyv1.StartStreamRequest) (*eveyv1.PlaySession, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playbackHandler.StartStream: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("playbackHandler.StartStream: %w", err)
	}
	if err := h.requireMediaAccess(ctx, req.MediaItemId); err != nil {
		return nil, fmt.Errorf("playbackHandler.StartStream: %w", err)
	}
	if h.userQuery != nil {
		if blocked, _ := h.isBlockedByParental(ctx, profileID, req.MediaItemId); blocked {
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.playback.parental_blocked")
		}
	}

	session := &playback.PlaySession{
		ID:          id.New("ps-"),
		ProfileID:   profileID,
		MediaItemID: req.MediaItemId,
		PlayMethod:  playMethodFromProto[req.PlayMethod],
		DeviceName:  req.DeviceName,
		DeviceType:  req.ClientName,
	}
	if err := h.cmd.StartStream(ctx, session, claims.UserID); err != nil {
		if errors.Is(err, appplayback.ErrProfileNotFound) {
			return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.auth.profile_not_found")
		}
		slog.Error("start stream", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	details := map[string]any{"media_id": req.MediaItemId, "method": req.PlayMethod.String()}
	if item, err := h.librarySvc.GetMediaItem(ctx, req.MediaItemId); err == nil {
		details["title"] = item.Title
	}
	h.audit(ctx, claims.UserID, "start_stream", "media", &req.MediaItemId, details)
	return playSessionToProto(session), nil
}

func (h *playbackHandler) StopStream(ctx context.Context, req *eveyv1.StopStreamRequest) (*eveyv1.StopStreamResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playbackHandler.StopStream: %w", err)
	}
	sess, sessErr := h.cmd.GetSession(ctx, req.SessionId)
	if !claims.IsAdmin {
		if sessErr != nil {
			return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.playback.session_not_found")
		}
		if sess.ProfileID != claims.ProfileID {
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.playback.cannot_stop_other_session")
		}
	}
	if err := h.cmd.StopStream(ctx, req.SessionId); err != nil {
		slog.Error("stop stream", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	if sessErr == nil && sess != nil && h.hlsManager != nil {
		h.hlsManager.RemoveSession(sess.MediaItemID)
	}
	sid := req.SessionId
	h.audit(ctx, claims.UserID, "stop_stream", "session", &sid, nil)
	return &eveyv1.StopStreamResponse{}, nil
}

func (h *playbackHandler) ReportProgress(ctx context.Context, req *eveyv1.ReportProgressRequest) (*eveyv1.ReportProgressResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playbackHandler.ReportProgress: %w", err)
	}
	if !claims.IsAdmin {
		session, err := h.cmd.GetSession(ctx, req.SessionId)
		if err != nil {
			return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.playback.session_not_found")
		}
		if session.ProfileID != claims.ProfileID {
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.playback.cannot_report_other_session")
		}
	}
	// Consume remote command BEFORE updating the session (Update overwrites the row)
	resp := &eveyv1.ReportProgressResponse{}
	if cmd, cmdErr := h.cmd.ConsumeRemoteCommand(ctx, req.SessionId); cmdErr == nil && cmd != "" {
		resp.RemoteCommand = cmd
	}
	if err := h.cmd.ReportProgressBySession(ctx, req.SessionId, req.PositionMs, req.IsPaused); err != nil {
		if errors.Is(err, appplayback.ErrProfileNotFound) {
			return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.auth.profile_not_found")
		}
		slog.Error("report progress", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return resp, nil
}

func (h *playbackHandler) GetWatchProgress(ctx context.Context, req *eveyv1.GetWatchProgressRequest) (*eveyv1.WatchProgress, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playbackHandler.GetWatchProgress: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("playbackHandler.GetWatchProgress: %w", err)
	}
	if err := h.requireMediaAccess(ctx, req.MediaItemId); err != nil {
		return &eveyv1.WatchProgress{}, nil
	}
	wp, err := h.query.GetWatchProgress(ctx, profileID, req.MediaItemId)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return &eveyv1.WatchProgress{}, nil
		}
		slog.Error("get watch progress", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return watchProgressToProto(wp), nil
}

func (h *playbackHandler) GetWatchHistory(ctx context.Context, req *eveyv1.GetWatchHistoryRequest) (*eveyv1.GetWatchHistoryResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playbackHandler.GetWatchHistory: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("playbackHandler.GetWatchHistory: %w", err)
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	} else if limit > maxPageSize {
		limit = maxPageSize
	}
	offset := 0
	if req.Pagination != nil {
		limit, offset = clampPagination(req.Pagination.PageSize, req.Pagination.Page, limit)
	}
	items, err := h.query.GetWatchHistory(ctx, profileID, limit, offset)
	if err != nil {
		slog.Error("get watch history", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	allowedIDs, _ := h.getAllowedLibraryIDs(ctx)
	resp := &eveyv1.GetWatchHistoryResponse{}
	if allowedIDs != nil {
		ids := make([]string, len(items))
		for i, wp := range items {
			ids[i] = wp.MediaItemID
		}
		mis, mErr := h.librarySvc.GetMediaItemsByIDs(ctx, ids)
		if mErr != nil {
			slog.Warn("watch history: batch fetch media items", slog.Any("err", mErr))
		}
		allowed := make(map[string]bool, len(mis))
		for _, mi := range mis {
			if slices.Contains(allowedIDs, string(mi.LibraryID)) {
				allowed[string(mi.ID)] = true
			}
		}
		for _, wp := range items {
			if allowed[wp.MediaItemID] {
				resp.Items = append(resp.Items, watchProgressToProto(wp))
			}
		}
	} else {
		for _, wp := range items {
			resp.Items = append(resp.Items, watchProgressToProto(wp))
		}
	}
	return resp, nil
}

func (h *playbackHandler) GetResumeItems(ctx context.Context, req *eveyv1.GetResumeItemsRequest) (*eveyv1.GetResumeItemsResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playbackHandler.GetResumeItems: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("playbackHandler.GetResumeItems: %w", err)
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	items, err := h.query.GetResumeItems(ctx, profileID, limit)
	if err != nil {
		slog.Error("get resume items", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	allowedIDs, _ := h.getAllowedLibraryIDs(ctx)
	resp := &eveyv1.GetResumeItemsResponse{}
	if allowedIDs != nil {
		ids := make([]string, len(items))
		for i, wp := range items {
			ids[i] = wp.MediaItemID
		}
		mis, mErr := h.librarySvc.GetMediaItemsByIDs(ctx, ids)
		if mErr != nil {
			slog.Warn("resume items: batch fetch media items", slog.Any("err", mErr))
		}
		allowed := make(map[string]bool, len(mis))
		for _, mi := range mis {
			if slices.Contains(allowedIDs, string(mi.LibraryID)) {
				allowed[string(mi.ID)] = true
			}
		}
		for _, wp := range items {
			if allowed[wp.MediaItemID] {
				resp.Progress = append(resp.Progress, watchProgressToProto(wp))
			}
		}
	} else {
		for _, wp := range items {
			resp.Progress = append(resp.Progress, watchProgressToProto(wp))
		}
	}
	return resp, nil
}

func (h *playbackHandler) GetNextUp(ctx context.Context, req *eveyv1.GetNextUpRequest) (*eveyv1.GetNextUpResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playbackHandler.GetNextUp: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("playbackHandler.GetNextUp: %w", err)
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	progresses, err := h.query.GetNextUp(ctx, profileID, limit)
	if err != nil {
		slog.Error("get next up", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	ids := make([]string, len(progresses))
	for i, wp := range progresses {
		ids[i] = wp.MediaItemID
	}
	mediaItems, err := h.librarySvc.GetMediaItemsByIDs(ctx, ids)
	if err != nil {
		slog.Error("get media items for next up", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	allowedIDs, _ := h.getAllowedLibraryIDs(ctx)
	byID := make(map[string]*library.MediaItem, len(mediaItems))
	for _, mi := range mediaItems {
		if allowedIDs != nil && !slices.Contains(allowedIDs, string(mi.LibraryID)) {
			continue
		}
		byID[string(mi.ID)] = mi
	}
	resp := &eveyv1.GetNextUpResponse{}
	for _, wp := range progresses {
		if item, ok := byID[wp.MediaItemID]; ok {
			resp.Items = append(resp.Items, mediaItemToProto(item))
		}
	}
	return resp, nil
}

func (h *playbackHandler) MarkAsWatched(ctx context.Context, req *eveyv1.MarkAsWatchedRequest) (*eveyv1.MarkAsWatchedResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playbackHandler.MarkAsWatched: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("playbackHandler.MarkAsWatched: %w", err)
	}
	if err := h.requireMediaAccess(ctx, req.MediaItemId); err != nil {
		return nil, fmt.Errorf("playbackHandler.MarkAsWatched: %w", err)
	}
	if err := h.cmd.MarkAsWatched(ctx, profileID, req.MediaItemId); err != nil {
		if errors.Is(err, appplayback.ErrProfileNotFound) {
			return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.auth.profile_not_found")
		}
		slog.Error("mark as watched", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.MarkAsWatchedResponse{}, nil
}

func (h *playbackHandler) MarkAsUnwatched(ctx context.Context, req *eveyv1.MarkAsUnwatchedRequest) (*eveyv1.MarkAsUnwatchedResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playbackHandler.MarkAsUnwatched: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("playbackHandler.MarkAsUnwatched: %w", err)
	}
	if err := h.requireMediaAccess(ctx, req.MediaItemId); err != nil {
		return nil, fmt.Errorf("playbackHandler.MarkAsUnwatched: %w", err)
	}
	if err := h.cmd.MarkAsUnwatched(ctx, profileID, req.MediaItemId); err != nil {
		if errors.Is(err, appplayback.ErrProfileNotFound) {
			return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.auth.profile_not_found")
		}
		slog.Error("mark as unwatched", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.MarkAsUnwatchedResponse{}, nil
}

func (h *playbackHandler) requireMediaAccess(ctx context.Context, mediaItemID string) error {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return status.Errorf(codes.Unauthenticated, "unauthenticated")
	}
	if claims.IsAdmin || h.userAccess == nil {
		return nil
	}
	item, err := h.librarySvc.GetMediaItem(ctx, mediaItemID)
	if err != nil {
		return h.translator.TranslatedError(ctx, codes.NotFound, "errors.media.not_found")
	}
	ok, _ := h.userAccess.HasLibraryAccess(ctx, claims.UserID, string(item.LibraryID))
	if !ok {
		return h.translator.TranslatedError(ctx, codes.NotFound, "errors.media.not_found")
	}
	return nil
}

func (h *playbackHandler) getAllowedLibraryIDs(ctx context.Context) ([]string, error) {
	if h.userAccess == nil {
		return nil, nil
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return nil, nil
	}
	return h.userAccess.GetLibraryAccessByUserID(ctx, claims.UserID)
}

func (h *playbackHandler) isBlockedByParental(ctx context.Context, profileID, mediaItemID string) (bool, string) {
	profile, err := h.userQuery.GetProfileByID(ctx, profileID)
	if err != nil || profile.Parental == nil || !profile.Parental.Enabled {
		return false, ""
	}
	ps := profile.Parental
	item, err := h.librarySvc.GetMediaItem(ctx, mediaItemID)
	if err != nil {
		return false, ""
	}
	if ps.MaxRating != "" && !ratings.IsAllowed(item.OfficialRating, ps.MaxRating) {
		return true, "content rating exceeds profile limit"
	}
	if ratings.HasBlockedTag(item.Tags, ps.BlockedTags) {
		return true, "content contains blocked tags"
	}
	return false, ""
}
