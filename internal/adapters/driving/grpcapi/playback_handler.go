package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"

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
	"github.com/evey-project/evey/internal/pkg/region"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
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
	hlsManager     *apptranscoding.SegmentOrchestrator
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
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.GetStreamInfo")
	defer span.End()
	if err := h.requireMediaAccess(ctx, req.MediaItemId); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.GetStreamInfo: %w", err)
	}
	item, err := h.librarySvc.GetMediaItem(ctx, req.MediaItemId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.NotFound, "media item not found: %v", err)
	}
	containerFormat := ""
	if item.File != nil {
		containerFormat = item.File.ContainerFormat
	}
	resp := &eveyv1.StreamInfo{
		MediaItemId: req.MediaItemId,
		Container:   containerFormat,
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

	prefs := apptranscoding.DefaultTranscodingPreferences()
	prefs.NetworkType = normalizeNetworkType(req.NetworkType)
	if h.userQuery != nil {
		if claims := interceptors.ClaimsFromContext(ctx); claims != nil && claims.ProfileID != "" {
			if prof, err := h.userQuery.GetProfileByID(ctx, claims.ProfileID); err == nil && prof != nil {
				parsed := apptranscoding.ParseTranscodingPreferences(prof.Preferences)
				// Keep network type from request; everything else from profile.
				parsed.NetworkType = prefs.NetworkType
				prefs = parsed
				if prof.MaxBitrateKbps != nil && *prof.MaxBitrateKbps > 0 {
					if maxBitrateKbps == 0 || *prof.MaxBitrateKbps < maxBitrateKbps {
						maxBitrateKbps = *prof.MaxBitrateKbps
					}
				}
			}
		}
	}

	decision := apptranscoding.MakePlaybackDecisionWithPrefs(ctx, streams, containerFormat, client, maxBitrateKbps, prefs)
	if apptranscoding.TranscodeDisallowed(decision, prefs) {
		err := status.Errorf(codes.FailedPrecondition, "transcoding disabled by user preferences")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
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
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.StartStream")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.StartStream: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.StartStream: %w", err)
	}
	if err := h.requireMediaAccess(ctx, req.MediaItemId); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.StartStream: %w", err)
	}
	if h.userQuery != nil {
		if blocked, _ := h.isBlockedByParental(ctx, profileID, req.MediaItemId); blocked {
			err := h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.playback.parental_blocked")
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, err
		}
	}

	session, err := playback.NewPlaySession(
		id.New("ps-"),
		shared.ProfileID(profileID),
		shared.MediaItemID(req.MediaItemId),
		playback.WithPlaySessionMethod(playback.PlayMethod(playMethodFromProto[req.PlayMethod])),
		playback.WithPlaySessionDevice(req.DeviceName, req.ClientName),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := h.cmd.StartStream(ctx, session, claims.UserID); err != nil {
		span.RecordError(err)
		if errors.Is(err, appplayback.ErrProfileNotFound) {
			tErr := h.translator.TranslatedError(ctx, codes.NotFound, "errors.auth.profile_not_found")
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
		if errors.Is(err, appplayback.ErrMaxStreams) {
			tErr := h.translator.TranslatedError(ctx, codes.ResourceExhausted, "errors.playback.max_streams_reached")
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
		if errors.Is(err, appplayback.ErrMaxDevicesPerProfile) {
			tErr := h.translator.TranslatedError(ctx, codes.ResourceExhausted, "errors.playback.max_devices_reached")
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
		span.SetStatus(otelcodes.Error, err.Error())
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
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.StopStream")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.StopStream: %w", err)
	}
	sess, sessErr := h.cmd.GetSession(ctx, req.SessionId)
	if !claims.IsAdmin {
		if sessErr != nil {
			tErr := h.translator.TranslatedError(ctx, codes.NotFound, "errors.playback.session_not_found")
			span.RecordError(tErr)
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
		if string(sess.ProfileID) != claims.ProfileID {
			tErr := h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.playback.cannot_stop_other_session")
			span.RecordError(tErr)
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
	}
	if err := h.cmd.StopStream(ctx, req.SessionId); err != nil {
		span.RecordError(err)
		if errors.Is(err, appplayback.ErrSessionNotFound) {
			tErr := h.translator.TranslatedError(ctx, codes.NotFound, "errors.playback.session_not_found")
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
		span.SetStatus(otelcodes.Error, err.Error())
		slog.Error("stop stream", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	if sessErr == nil && sess != nil && h.hlsManager != nil {
		h.hlsManager.RemoveSession(string(sess.MediaItemID))
	}
	sid := req.SessionId
	h.audit(ctx, claims.UserID, "stop_stream", "session", &sid, nil)
	return &eveyv1.StopStreamResponse{}, nil
}

func (h *playbackHandler) ReportProgress(ctx context.Context, req *eveyv1.ReportProgressRequest) (*eveyv1.ReportProgressResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.ReportProgress")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.ReportProgress: %w", err)
	}
	if !claims.IsAdmin {
		session, err := h.cmd.GetSession(ctx, req.SessionId)
		if err != nil {
			tErr := h.translator.TranslatedError(ctx, codes.NotFound, "errors.playback.session_not_found")
			span.RecordError(tErr)
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
		if string(session.ProfileID) != claims.ProfileID {
			tErr := h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.playback.cannot_report_other_session")
			span.RecordError(tErr)
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
	}
	// Consume remote command BEFORE updating the session (Update overwrites the row)
	resp := &eveyv1.ReportProgressResponse{}
	if cmd, cmdErr := h.cmd.ConsumeRemoteCommand(ctx, req.SessionId); cmdErr == nil && cmd != "" {
		resp.RemoteCommand = cmd
	}
	if err := h.cmd.ReportProgressBySession(ctx, req.SessionId, req.PositionMs, req.IsPaused); err != nil {
		span.RecordError(err)
		if errors.Is(err, appplayback.ErrProfileNotFound) {
			tErr := h.translator.TranslatedError(ctx, codes.NotFound, "errors.auth.profile_not_found")
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
		span.SetStatus(otelcodes.Error, err.Error())
		slog.Error("report progress", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return resp, nil
}

func (h *playbackHandler) GetWatchProgress(ctx context.Context, req *eveyv1.GetWatchProgressRequest) (*eveyv1.WatchProgress, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.GetWatchProgress")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.GetWatchProgress: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
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
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		slog.Error("get watch progress", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return watchProgressToProto(wp), nil
}

func (h *playbackHandler) GetWatchHistory(ctx context.Context, req *eveyv1.GetWatchHistoryRequest) (*eveyv1.GetWatchHistoryResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.GetWatchHistory")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.GetWatchHistory: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.GetWatchHistory: %w", err)
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	} else if limit > maxPageSize {
		limit = maxPageSize
	}
	offset := 0
	var cursor *playback.WatchHistoryCursor
	if req.Pagination != nil {
		limit, offset = clampPagination(req.Pagination.PageSize, req.Pagination.Page, limit)
		if req.Pagination.Cursor != "" {
			c, err := decodeTimeCursor(req.Pagination.Cursor)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(otelcodes.Error, err.Error())
				return nil, status.Errorf(codes.InvalidArgument, "invalid cursor: %v", err)
			}
			cursor = &playback.WatchHistoryCursor{LastPlayedAt: c.t, MediaItemID: c.id}
		}
	}
	var items []*playback.WatchProgress
	if cursor != nil {
		items, err = h.query.GetWatchHistoryCursor(ctx, profileID, cursor, limit)
	} else {
		items, err = h.query.GetWatchHistory(ctx, profileID, limit, offset)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		slog.Error("get watch history", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	allowedIDs, _ := h.getAllowedLibraryIDs(ctx)
	resp := &eveyv1.GetWatchHistoryResponse{Pagination: &eveyv1.PaginationResponse{}}
	resp.Items = h.enrichWatchHistory(ctx, items, allowedIDs)
	if len(items) > 0 && len(items) >= limit {
		last := items[len(items)-1]
		resp.Pagination.NextCursor = encodeTimeCursor(last.LastPlayedAt, string(last.MediaItemID))
	}
	return resp, nil
}

// enrichWatchHistory turns raw progress rows into display-ready items: it
// batch-resolves each media item (title, type, episode number) plus, for
// episodes, the parent season (season number) and grandparent series (title),
// so clients render a real label instead of the bare media_item_id. It also
// enforces library access: a restricted user only sees items in an allowed
// library, and an item whose media no longer exists is hidden from them.
// Best-effort — a failed lookup just leaves the affected fields empty.
func (h *playbackHandler) enrichWatchHistory(ctx context.Context, items []*playback.WatchProgress, allowedIDs []string) []*eveyv1.WatchProgress {
	ids := make([]string, 0, len(items))
	for _, wp := range items {
		ids = append(ids, string(wp.MediaItemID))
	}
	byID := h.mediaItemMap(ctx, ids)

	// Episodes need their season (number) and series (title), one level up each.
	seasonIDs := make([]string, 0)
	for _, mi := range byID {
		if mi.Type == library.Episode && mi.ParentID != "" {
			seasonIDs = append(seasonIDs, mi.ParentID)
		}
	}
	seasonByID := h.mediaItemMap(ctx, seasonIDs)
	seriesIDs := make([]string, 0)
	for _, s := range seasonByID {
		if s.ParentID != "" {
			seriesIDs = append(seriesIDs, s.ParentID)
		}
	}
	seriesByID := h.mediaItemMap(ctx, seriesIDs)

	out := make([]*eveyv1.WatchProgress, 0, len(items))
	for _, wp := range items {
		mi := byID[string(wp.MediaItemID)]
		if allowedIDs != nil {
			// Restricted user: skip items outside an allowed library, and skip
			// unresolved (deleted) media since access can't be verified.
			if mi == nil || !slices.Contains(allowedIDs, string(mi.LibraryID)) {
				continue
			}
		}
		pb := watchProgressToProto(wp)
		if mi != nil {
			pb.Title = mi.Title
			pb.MediaType = string(mi.Type)
			if mi.IndexNumber != nil {
				pb.IndexNumber = int32(*mi.IndexNumber)
			}
			if mi.Type == library.Episode {
				if season := seasonByID[mi.ParentID]; season != nil {
					if season.IndexNumber != nil {
						pb.ParentIndexNumber = int32(*season.IndexNumber)
					}
					if series := seriesByID[season.ParentID]; series != nil {
						pb.SeriesTitle = series.Title
					}
				}
			}
		}
		out = append(out, pb)
	}
	return out
}

// mediaItemMap batch-resolves media items by ID into a lookup map. Best-effort:
// a fetch error logs and yields an empty map rather than failing the caller.
func (h *playbackHandler) mediaItemMap(ctx context.Context, ids []string) map[string]*library.MediaItem {
	m := map[string]*library.MediaItem{}
	if len(ids) == 0 {
		return m
	}
	mis, err := h.librarySvc.GetMediaItemsByIDs(ctx, ids)
	if err != nil {
		slog.Warn("watch history: batch fetch media items", slog.Any("err", err))
		return m
	}
	for _, mi := range mis {
		m[string(mi.ID)] = mi
	}
	return m
}

func (h *playbackHandler) GetResumeItems(ctx context.Context, req *eveyv1.GetResumeItemsRequest) (*eveyv1.GetResumeItemsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.GetResumeItems")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.GetResumeItems: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.GetResumeItems: %w", err)
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	items, err := h.query.GetResumeItems(ctx, profileID, limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		slog.Error("get resume items", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	allowedIDs, _ := h.getAllowedLibraryIDs(ctx)
	resp := &eveyv1.GetResumeItemsResponse{}
	if allowedIDs != nil {
		ids := make([]string, len(items))
		for i, wp := range items {
			ids[i] = string(wp.MediaItemID)
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
			if allowed[string(wp.MediaItemID)] {
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
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.GetNextUp")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.GetNextUp: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.GetNextUp: %w", err)
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	progresses, err := h.query.GetNextUp(ctx, profileID, limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		slog.Error("get next up", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	ids := make([]string, len(progresses))
	for i, wp := range progresses {
		ids[i] = string(wp.MediaItemID)
	}
	mediaItems, err := h.librarySvc.GetMediaItemsByIDs(ctx, ids)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
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
		if item, ok := byID[string(wp.MediaItemID)]; ok {
			resp.Items = append(resp.Items, mediaItemToProto(item))
		}
	}
	return resp, nil
}

func (h *playbackHandler) MarkAsWatched(ctx context.Context, req *eveyv1.MarkAsWatchedRequest) (*eveyv1.MarkAsWatchedResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.MarkAsWatched")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.MarkAsWatched: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.MarkAsWatched: %w", err)
	}
	if err := h.requireMediaAccess(ctx, req.MediaItemId); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.MarkAsWatched: %w", err)
	}
	if err := h.cmd.MarkAsWatched(ctx, profileID, req.MediaItemId); err != nil {
		span.RecordError(err)
		if errors.Is(err, appplayback.ErrProfileNotFound) {
			tErr := h.translator.TranslatedError(ctx, codes.NotFound, "errors.auth.profile_not_found")
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
		span.SetStatus(otelcodes.Error, err.Error())
		slog.Error("mark as watched", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.MarkAsWatchedResponse{}, nil
}

func (h *playbackHandler) MarkAsUnwatched(ctx context.Context, req *eveyv1.MarkAsUnwatchedRequest) (*eveyv1.MarkAsUnwatchedResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.MarkAsUnwatched")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.MarkAsUnwatched: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.MarkAsUnwatched: %w", err)
	}
	if err := h.requireMediaAccess(ctx, req.MediaItemId); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.MarkAsUnwatched: %w", err)
	}
	if err := h.cmd.MarkAsUnwatched(ctx, profileID, req.MediaItemId); err != nil {
		span.RecordError(err)
		if errors.Is(err, appplayback.ErrProfileNotFound) {
			tErr := h.translator.TranslatedError(ctx, codes.NotFound, "errors.auth.profile_not_found")
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
		span.SetStatus(otelcodes.Error, err.Error())
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

// normalizeNetworkType accepts "wifi" or "cellular" and defaults unknown/empty to "wifi".
func normalizeNetworkType(raw string) string {
	if raw == "cellular" {
		return "cellular"
	}
	return "wifi"
}

// cellularCapFromPreferences parses max_bitrate_cellular_kbps out of a profile's
// preferences map. Returns 0 when missing, empty, or invalid.
func cellularCapFromPreferences(prefs map[string]string) int {
	v, ok := prefs["max_bitrate_cellular_kbps"]
	if !ok || v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func (h *playbackHandler) isBlockedByParental(ctx context.Context, profileID, mediaItemID string) (bool, string) {
	profile, err := h.userQuery.GetProfileByID(ctx, profileID)
	if err != nil {
		return false, ""
	}
	ps := profile.EffectiveParental()
	if ps == nil {
		return false, ""
	}
	item, err := h.librarySvc.GetMediaItem(ctx, mediaItemID)
	if err != nil {
		return false, ""
	}
	if ps.MaxAge != nil {
		reg := region.Resolve(profile.Region, "", profile.Locale)
		rating := ""
		var certByCountry map[string]string
		if item.Classification != nil {
			rating = item.Classification.OfficialRating
			certByCountry = item.Classification.CertificationsByCountry
		}
		if reg != "" && certByCountry != nil {
			if r, ok := certByCountry[reg]; ok && r != "" {
				rating = r
			}
		}
		if !ratings.IsAllowedByAge(reg, rating, *ps.MaxAge) {
			return true, "content rating exceeds profile age limit"
		}
	}
	if ratings.HasBlockedTag(itemTags(item), ps.BlockedTags) {
		return true, "content contains blocked tags"
	}
	return false, ""
}
