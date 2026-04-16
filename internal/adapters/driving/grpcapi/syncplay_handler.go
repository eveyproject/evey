package grpcapi

import (
	"context"
	"fmt"
	"log/slog"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmusic "github.com/evey-project/evey/internal/app/music"
	appplaylist "github.com/evey-project/evey/internal/app/playlist"
	appsyncplay "github.com/evey-project/evey/internal/app/syncplay"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	domainsyncplay "github.com/evey-project/evey/internal/domain/syncplay"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type syncplayHandler struct {
	eveyv1.UnimplementedSyncPlayServiceServer
	hub         *appsyncplay.Hub
	libQuery    *applibrary.QueryService
	userAccess  *appuser.AccessService
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
		// TODO: wire proper LibraryAccessChecker for music tracks
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
	return nil
}

func (h *syncplayHandler) CreateRoom(ctx context.Context, req *eveyv1.CreateSyncPlayRoomRequest) (*eveyv1.CreateSyncPlayRoomResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("syncplayHandler.CreateRoom: %w", err)
	}
	hostID := claims.ProfileID
	if hostID == "" {
		hostID = claims.UserID
	}

	// Playlist-bound room (music queue)
	if req.PlaylistId != "" {
		if h.playlistSvc == nil {
			return nil, status.Error(codes.FailedPrecondition, "playlist service unavailable")
		}
		pl, err := h.playlistSvc.GetByID(ctx, req.PlaylistId, claims.UserID)
		if err != nil || pl == nil {
			return nil, status.Error(codes.NotFound, "playlist not found")
		}
		mediaItems, plItems, err := h.playlistSvc.GetItems(ctx, req.PlaylistId, claims.UserID)
		if err != nil {
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
			return nil, status.Error(codes.FailedPrecondition, "playlist is empty")
		}
		// Validate first-track access (music). If user has no access, fail.
		if err := h.requireMediaAccess(ctx, trackIDs[0], "music"); err != nil {
			return nil, err
		}
		name := req.Name
		if name == "" {
			name = pl.Name
			if name == "" {
				name = "Listen Together"
			}
		}
		room, err := h.hub.CreateRoomFromPlaylist(ctx, hostID, req.PlaylistId, name, trackIDs)
		if err != nil {
			return nil, internalError("create room", err)
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
		return nil, status.Error(codes.InvalidArgument, "media_id required")
	}
	mediaType := req.MediaType
	if mediaType == "" {
		mediaType = "video"
	}
	if err := h.requireMediaAccess(ctx, req.MediaId, mediaType); err != nil {
		return nil, err
	}
	name := req.Name
	if name == "" {
		if mediaType == "music" {
			name = "Listen Together"
		} else {
			name = "Watch Together"
		}
	}
	room, err := h.hub.CreateRoomTyped(ctx, hostID, req.MediaId, name, mediaType)
	if err != nil {
		return nil, internalError("create room", err)
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
	if err := h.requireMediaAccess(ctx, string(room.MediaID), room.MediaType); err != nil {
		return nil, err
	}
	if err := h.hub.JoinRoom(ctx, room.ID, claims.UserID, claims.Username); err != nil {
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
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("syncplayHandler.GetRoom: %w", err)
	}
	room, err := h.hub.GetRoom(ctx, req.RoomId)
	if err != nil {
		return nil, status.Error(codes.NotFound, "room not found")
	}
	if err := h.requireMediaAccess(ctx, string(room.MediaID), room.MediaType); err != nil {
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
		return nil, status.Error(codes.PermissionDenied, "forbidden")
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
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("syncplayHandler.JoinRoom: %w", err)
	}
	room, err := h.hub.GetRoom(ctx, req.RoomId)
	if err != nil {
		return nil, status.Error(codes.NotFound, "room not found")
	}
	if err := h.requireMediaAccess(ctx, string(room.MediaID), room.MediaType); err != nil {
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
