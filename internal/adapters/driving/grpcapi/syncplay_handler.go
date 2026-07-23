package grpcapi

import (
	"context"
	"fmt"
	"log/slog"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmusic "github.com/evey-project/evey/internal/app/music"
	appplaylist "github.com/evey-project/evey/internal/app/playlist"
	appsyncplay "github.com/evey-project/evey/internal/app/syncplay"
	appuser "github.com/evey-project/evey/internal/app/user"
	domainsyncplay "github.com/evey-project/evey/internal/domain/syncplay"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type syncplayHandler struct {
	eveyv1.UnimplementedSyncPlayServiceServer
	hub        *appsyncplay.Hub
	libQuery   *applibrary.QueryService
	userAccess *appuser.AccessService
	// userQuery resolves the caller profile's parental settings. Without it a
	// restricted profile could open a room on media it may not watch, and the
	// room's public join code then launders that media into a profile-less
	// guest token the parental gate cannot see.
	userQuery   *appuser.QueryService
	musicSvc    *appmusic.Service
	playlistSvc *appplaylist.Service
}

func (h *syncplayHandler) requireMediaAccess(ctx context.Context, mediaID, mediaType string) error {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if mediaType == "music" {
		if claims.IsAdmin {
			return nil
		}
		if h.musicSvc == nil {
			return status.Error(codes.NotFound, "media not found")
		}
		t, err := h.musicSvc.GetTrack(ctx, mediaID)
		if err != nil || t == nil {
			return status.Error(codes.NotFound, "media not found")
		}
		if h.userAccess == nil {
			return nil
		}
		ok, _ := h.userAccess.HasLibraryAccess(ctx, claims.UserID, string(t.LibraryID))
		if !ok {
			return status.Error(codes.NotFound, "media not found")
		}
		return nil
	}
	if claims.IsAdmin || h.userAccess == nil {
		return nil
	}
	item, err := h.libQuery.GetMediaItem(ctx, mediaID)
	if err != nil || item == nil {
		return status.Error(codes.NotFound, "media not found")
	}
	ok, _ := h.userAccess.HasLibraryAccess(ctx, claims.UserID, string(item.LibraryID))
	if !ok {
		return status.Error(codes.NotFound, "media not found")
	}
	// Parental gate, same opaque NotFound as the access denial above so the
	// caller can't tell "forbidden" from "absent".
	//
	// Library access is per USER, parental is per PROFILE - so a kids profile on
	// a household account clears the check above for the whole library. Skipping
	// this made a room a laundering step: the join-code route is public and
	// mints a media-scoped guest token carrying NO profile, and the parental
	// gate no-ops on an empty profile, so the blocked media became streamable.
	if mediaItemBlockedByParental(resolveParentalFilterFromUserQuery(ctx, h.userQuery), item) {
		return status.Error(codes.NotFound, "media not found")
	}
	return nil
}

func (h *syncplayHandler) CreateRoom(ctx context.Context, req *eveyv1.CreateSyncPlayRoomRequest) (*eveyv1.CreateSyncPlayRoomResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.syncplay.CreateRoom")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("syncplayHandler.CreateRoom: %w", err)
	}
	hostID := claims.ProfileID
	if hostID == "" {
		hostID = claims.UserID
	}

	// Playlist-bound room (music queue)
	if req.PlaylistId != "" {
		if h.playlistSvc == nil {
			span.SetStatus(otelcodes.Error, "playlist service unavailable")
			return nil, status.Error(codes.FailedPrecondition, "playlist service unavailable")
		}
		pl, plErr := h.playlistSvc.GetByID(ctx, req.PlaylistId, claims.ProfileID)
		if plErr != nil || pl == nil {
			if plErr != nil {
				span.RecordError(plErr)
			}
			span.SetStatus(otelcodes.Error, "playlist not found")
			return nil, status.Error(codes.NotFound, "playlist not found")
		}
		mediaItems, plItems, itemsErr := h.playlistSvc.GetItems(ctx, req.PlaylistId, claims.ProfileID)
		if itemsErr != nil {
			span.RecordError(itemsErr)
			span.SetStatus(otelcodes.Error, "playlist not found")
			return nil, status.Error(codes.NotFound, "playlist not found")
		}
		trackIDs := make([]string, 0)
		if pl.IsSmart() {
			for _, mi := range mediaItems {
				trackIDs = append(trackIDs, string(mi.ID))
			}
		} else {
			for _, it := range plItems {
				trackIDs = append(trackIDs, string(it.MediaItemID))
			}
		}
		if len(trackIDs) == 0 {
			span.SetStatus(otelcodes.Error, "playlist is empty")
			return nil, status.Error(codes.FailedPrecondition, "playlist is empty")
		}
		// Validate first-track access (music). If user has no access, fail.
		if err := h.requireMediaAccess(ctx, trackIDs[0], "music"); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, err
		}
		name := req.Name
		if len(name) > 100 {
			return nil, status.Error(codes.InvalidArgument, "room name must be 100 characters or fewer")
		}
		if name == "" {
			name = pl.Name
			if name == "" {
				name = "Listen Together"
			}
		}
		room, err := h.hub.CreateRoomFromPlaylist(ctx, hostID, req.PlaylistId, name, trackIDs)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, internalError(ctx, "create room", err)
		}
		return &eveyv1.CreateSyncPlayRoomResponse{
			Id:        room.ID,
			Code:      room.Code,
			Name:      room.Name,
			MediaId:   string(room.MediaID),
			HostId:    string(room.HostID),
			MediaType: room.MediaType,
		}, nil
	}

	// Explicit client-built queue: a mix of albums, individual tracks and
	// playlists assembled on the client into one ordered list of track IDs.
	if len(req.Queue) > 0 {
		const maxQueue = 500
		seen := make(map[string]bool, len(req.Queue))
		trackIDs := make([]string, 0, len(req.Queue))
		for _, t := range req.Queue {
			if t == "" || seen[t] {
				continue
			}
			seen[t] = true
			trackIDs = append(trackIDs, t)
			if len(trackIDs) >= maxQueue {
				break
			}
		}
		if len(trackIDs) == 0 {
			span.SetStatus(otelcodes.Error, "queue is empty")
			return nil, status.Error(codes.InvalidArgument, "queue is empty")
		}
		// Gate on the first track's library access (the rest are enforced per
		// track at playback time, same as the playlist path).
		if accessErr := h.requireMediaAccess(ctx, trackIDs[0], "music"); accessErr != nil {
			span.RecordError(accessErr)
			span.SetStatus(otelcodes.Error, accessErr.Error())
			return nil, accessErr
		}
		name := req.Name
		if len(name) > 100 {
			return nil, status.Error(codes.InvalidArgument, "room name must be 100 characters or fewer")
		}
		if name == "" {
			name = "Listen Together"
		}
		room, createErr := h.hub.CreateRoomFromQueue(ctx, hostID, name, trackIDs)
		if createErr != nil {
			span.RecordError(createErr)
			span.SetStatus(otelcodes.Error, createErr.Error())
			return nil, internalError(ctx, "create room", createErr)
		}
		return &eveyv1.CreateSyncPlayRoomResponse{
			Id:        room.ID,
			Code:      room.Code,
			Name:      room.Name,
			MediaId:   string(room.MediaID),
			HostId:    string(room.HostID),
			MediaType: room.MediaType,
		}, nil
	}

	if req.MediaId == "" {
		span.SetStatus(otelcodes.Error, "media_id required")
		return nil, status.Error(codes.InvalidArgument, "media_id required")
	}
	mediaType := req.MediaType
	if mediaType == "" {
		mediaType = "video"
	}
	if err := h.requireMediaAccess(ctx, req.MediaId, mediaType); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	name := req.Name
	if len(name) > 100 {
		return nil, status.Error(codes.InvalidArgument, "room name must be 100 characters or fewer")
	}
	if name == "" {
		if mediaType == "music" {
			name = "Listen Together"
		} else {
			name = "Watch Together"
		}
	}
	room, err := h.hub.CreateRoomTyped(ctx, hostID, req.MediaId, name, mediaType)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "create room", err)
	}
	return &eveyv1.CreateSyncPlayRoomResponse{
		Id:        room.ID,
		Code:      room.Code,
		Name:      room.Name,
		MediaId:   string(room.MediaID),
		HostId:    string(room.HostID),
		MediaType: room.MediaType,
	}, nil
}

func (h *syncplayHandler) JoinByCode(ctx context.Context, req *eveyv1.JoinSyncPlayByCodeRequest) (*eveyv1.JoinSyncPlayByCodeResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.syncplay.JoinByCode")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("syncplayHandler.JoinByCode: %w", err)
	}
	if req.Code == "" {
		span.SetStatus(otelcodes.Error, "code required")
		return nil, status.Error(codes.InvalidArgument, "code required")
	}
	room, err := h.hub.GetRoomByCode(ctx, req.Code)
	if err != nil || room == nil {
		if err != nil {
			span.RecordError(err)
		}
		span.SetStatus(otelcodes.Error, "room not found")
		return nil, status.Error(codes.NotFound, "room not found")
	}
	if err := h.requireMediaAccess(ctx, string(room.MediaID), room.MediaType); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if err := h.hub.JoinRoom(ctx, room.ID, claims.UserID, claims.Username); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "room not found")
		return nil, status.Error(codes.NotFound, "room not found")
	}
	return &eveyv1.JoinSyncPlayByCodeResponse{
		RoomId:    room.ID,
		MediaId:   string(room.MediaID),
		WsUrl:     "/api/v1/syncplay/rooms/" + room.ID + "/ws",
		MediaType: room.MediaType,
	}, nil
}

func (h *syncplayHandler) GetRoom(ctx context.Context, req *eveyv1.GetSyncPlayRoomRequest) (*eveyv1.GetSyncPlayRoomResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.syncplay.GetRoom")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("syncplayHandler.GetRoom: %w", err)
	}
	room, err := h.hub.GetRoom(ctx, req.RoomId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "room not found")
		return nil, status.Error(codes.NotFound, "room not found")
	}
	if err := h.requireMediaAccess(ctx, string(room.MediaID), room.MediaType); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	isMember := string(room.HostID) == claims.ProfileID || string(room.HostID) == claims.UserID
	for _, m := range room.Members {
		if string(m.UserID) == claims.UserID {
			isMember = true
			break
		}
	}
	if !isMember && !claims.IsAdmin {
		// NotFound, not PermissionDenied: a non-member must not be able to tell an
		// existing room they're not in from a non-existent one (matches the
		// missing-room response above). Span keeps the real reason.
		span.SetStatus(otelcodes.Error, "not a room member")
		return nil, status.Error(codes.NotFound, "room not found")
	}
	members := make([]*eveyv1.SyncPlayMember, 0, len(room.Members))
	for _, m := range room.Members {
		members = append(members, &eveyv1.SyncPlayMember{
			UserId:   string(m.UserID),
			Username: domainsyncplay.MaskUsername(m.Username),
		})
	}
	queue := make([]string, 0, len(room.Queue))
	for _, q := range room.Queue {
		queue = append(queue, string(q))
	}
	resp := &eveyv1.GetSyncPlayRoomResponse{
		Id:         room.ID,
		Name:       room.Name,
		MediaId:    string(room.MediaID),
		HostId:     string(room.HostID),
		MediaType:  room.MediaType,
		Members:    members,
		Queue:      queue,
		TrackIndex: int32(room.TrackIndex),
		State: &eveyv1.SyncPlayState{
			Playing:    room.State.Playing,
			PositionMs: room.State.PositionMs,
		},
	}
	if room.PlaylistID != nil {
		resp.PlaylistId = *room.PlaylistID
	}
	if string(room.HostID) == claims.ProfileID || string(room.HostID) == claims.UserID || claims.IsAdmin {
		resp.Code = room.Code
	}
	return resp, nil
}

func (h *syncplayHandler) JoinRoom(ctx context.Context, req *eveyv1.JoinSyncPlayRoomRequest) (*eveyv1.JoinSyncPlayRoomResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.syncplay.JoinRoom")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("syncplayHandler.JoinRoom: %w", err)
	}
	room, err := h.hub.GetRoom(ctx, req.RoomId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "room not found")
		return nil, status.Error(codes.NotFound, "room not found")
	}
	if err := h.requireMediaAccess(ctx, string(room.MediaID), room.MediaType); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if err := h.hub.JoinRoom(ctx, req.RoomId, claims.UserID, claims.Username); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "room not found")
		return nil, status.Error(codes.NotFound, "room not found")
	}
	return &eveyv1.JoinSyncPlayRoomResponse{
		RoomId: req.RoomId,
		WsUrl:  "/api/v1/syncplay/rooms/" + req.RoomId + "/ws",
	}, nil
}

func (h *syncplayHandler) LeaveRoom(ctx context.Context, req *eveyv1.LeaveSyncPlayRoomRequest) (*eveyv1.LeaveSyncPlayRoomResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.syncplay.LeaveRoom")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("syncplayHandler.LeaveRoom: %w", err)
	}
	if err := h.hub.LeaveRoom(ctx, req.RoomId, claims.UserID); err != nil {
		slog.WarnContext(ctx, "syncplay leave room failed", slog.String("room_id", req.RoomId), slog.Any("err", err))
		span.RecordError(err)
	}
	return &eveyv1.LeaveSyncPlayRoomResponse{}, nil
}
