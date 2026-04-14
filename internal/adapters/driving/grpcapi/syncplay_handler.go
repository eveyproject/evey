package grpcapi

import (
	"context"
	"fmt"
	"log/slog"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appsyncplay "github.com/gravity-zero/evey/internal/app/syncplay"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	domainsyncplay "github.com/gravity-zero/evey/internal/domain/syncplay"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type syncplayHandler struct {
	eveyv1.UnimplementedSyncPlayServiceServer
	hub        *appsyncplay.Hub
	libQuery   *applibrary.QueryService
	userAccess *appuser.AccessService
}

func (h *syncplayHandler) requireMediaAccess(ctx context.Context, mediaID string) error {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return status.Error(codes.Unauthenticated, "unauthenticated")
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
	return nil
}

func (h *syncplayHandler) CreateRoom(ctx context.Context, req *eveyv1.CreateSyncPlayRoomRequest) (*eveyv1.CreateSyncPlayRoomResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("syncplayHandler.CreateRoom: %w", err)
	}
	if req.MediaId == "" {
		return nil, status.Error(codes.InvalidArgument, "media_id required")
	}
	if err := h.requireMediaAccess(ctx, req.MediaId); err != nil {
		return nil, err
	}
	name := req.Name
	if name == "" {
		name = "Watch Together"
	}
	hostID := claims.ProfileID
	if hostID == "" {
		hostID = claims.UserID
	}
	room, err := h.hub.CreateRoom(ctx, hostID, req.MediaId, name)
	if err != nil {
		return nil, internalError("create room", err)
	}
	return &eveyv1.CreateSyncPlayRoomResponse{
		Id:      room.ID,
		Code:    room.Code,
		Name:    room.Name,
		MediaId: room.MediaID,
		HostId:  room.HostID,
	}, nil
}

func (h *syncplayHandler) JoinByCode(ctx context.Context, req *eveyv1.JoinSyncPlayByCodeRequest) (*eveyv1.JoinSyncPlayByCodeResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("syncplayHandler.JoinByCode: %w", err)
	}
	if req.Code == "" {
		return nil, status.Error(codes.InvalidArgument, "code required")
	}
	room, err := h.hub.GetRoomByCode(ctx, req.Code)
	if err != nil || room == nil {
		return nil, status.Error(codes.NotFound, "room not found")
	}
	if err := h.requireMediaAccess(ctx, room.MediaID); err != nil {
		return nil, err
	}
	if err := h.hub.JoinRoom(ctx, room.ID, claims.UserID, claims.Username); err != nil {
		return nil, status.Error(codes.NotFound, "room not found")
	}
	return &eveyv1.JoinSyncPlayByCodeResponse{
		RoomId:  room.ID,
		MediaId: room.MediaID,
		WsUrl:   "/api/v1/syncplay/rooms/" + room.ID + "/ws",
	}, nil
}

func (h *syncplayHandler) GetRoom(ctx context.Context, req *eveyv1.GetSyncPlayRoomRequest) (*eveyv1.GetSyncPlayRoomResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("syncplayHandler.GetRoom: %w", err)
	}
	room, err := h.hub.GetRoom(ctx, req.RoomId)
	if err != nil {
		return nil, status.Error(codes.NotFound, "room not found")
	}
	if err := h.requireMediaAccess(ctx, room.MediaID); err != nil {
		return nil, err
	}
	isMember := room.HostID == claims.ProfileID || room.HostID == claims.UserID
	for _, m := range room.Members {
		if m.UserID == claims.UserID {
			isMember = true
			break
		}
	}
	if !isMember && !claims.IsAdmin {
		return nil, status.Error(codes.PermissionDenied, "forbidden")
	}
	members := make([]*eveyv1.SyncPlayMember, 0, len(room.Members))
	for _, m := range room.Members {
		members = append(members, &eveyv1.SyncPlayMember{
			UserId:   m.UserID,
			Username: domainsyncplay.MaskUsername(m.Username),
		})
	}
	resp := &eveyv1.GetSyncPlayRoomResponse{
		Id:      room.ID,
		Name:    room.Name,
		MediaId: room.MediaID,
		HostId:  room.HostID,
		Members: members,
		State: &eveyv1.SyncPlayState{
			Playing:    room.State.Playing,
			PositionMs: room.State.PositionMs,
		},
	}
	if room.HostID == claims.ProfileID || room.HostID == claims.UserID || claims.IsAdmin {
		resp.Code = room.Code
	}
	return resp, nil
}

func (h *syncplayHandler) JoinRoom(ctx context.Context, req *eveyv1.JoinSyncPlayRoomRequest) (*eveyv1.JoinSyncPlayRoomResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("syncplayHandler.JoinRoom: %w", err)
	}
	room, err := h.hub.GetRoom(ctx, req.RoomId)
	if err != nil {
		return nil, status.Error(codes.NotFound, "room not found")
	}
	if err := h.requireMediaAccess(ctx, room.MediaID); err != nil {
		return nil, err
	}
	if err := h.hub.JoinRoom(ctx, req.RoomId, claims.UserID, claims.Username); err != nil {
		return nil, status.Error(codes.NotFound, "room not found")
	}
	return &eveyv1.JoinSyncPlayRoomResponse{
		RoomId: req.RoomId,
		WsUrl:  "/api/v1/syncplay/rooms/" + req.RoomId + "/ws",
	}, nil
}

func (h *syncplayHandler) LeaveRoom(ctx context.Context, req *eveyv1.LeaveSyncPlayRoomRequest) (*eveyv1.LeaveSyncPlayRoomResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("syncplayHandler.LeaveRoom: %w", err)
	}
	if err := h.hub.LeaveRoom(ctx, req.RoomId, claims.UserID); err != nil {
		slog.Warn("syncplay leave room failed", slog.String("room_id", req.RoomId), slog.Any("err", err))
	}
	return &eveyv1.LeaveSyncPlayRoomResponse{}, nil
}
