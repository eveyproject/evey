package grpcapi

import (
	"context"
	"errors"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appplaylist "github.com/gravity-zero/evey/internal/app/playlist"
	"github.com/gravity-zero/evey/internal/domain/playlist"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type playlistHandler struct {
	eveyv1.UnimplementedPlaylistServiceServer
	svc *appplaylist.Service
}

func (h *playlistHandler) ListPlaylists(ctx context.Context, req *eveyv1.ListPlaylistsRequest) (*eveyv1.ListPlaylistsResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	userID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, err
	}
	pls, err := h.svc.List(ctx, userID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list playlists: %v", err)
	}
	resp := &eveyv1.ListPlaylistsResponse{}
	for _, pl := range pls {
		resp.Playlists = append(resp.Playlists, playlistToProto(pl))
	}
	return resp, nil
}

func (h *playlistHandler) GetPlaylist(ctx context.Context, req *eveyv1.GetPlaylistRequest) (*eveyv1.PlaylistProto, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	userID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, err
	}
	pl, err := h.svc.GetByID(ctx, req.Id, userID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "playlist not found: %v", err)
	}
	return playlistToProto(pl), nil
}

func (h *playlistHandler) CreatePlaylist(ctx context.Context, req *eveyv1.CreatePlaylistRequest) (*eveyv1.PlaylistProto, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	userID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, err
	}
	pl := &playlist.Playlist{
		Name:        req.Name,
		Description: req.Description,
		UserID:      userID,
		Type:        playlistTypeFromProto(req.Type),
		Rules:       playlistRulesFromProto(req.Rules),
		SortBy:      req.SortBy,
		SortOrder:   req.SortOrder,
	}
	if err := h.svc.Create(ctx, pl); err != nil {
		return nil, status.Errorf(codes.Internal, "create playlist: %v", err)
	}
	return playlistToProto(pl), nil
}

func (h *playlistHandler) UpdatePlaylist(ctx context.Context, req *eveyv1.UpdatePlaylistRequest) (*eveyv1.PlaylistProto, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	userID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, err
	}
	pl := &playlist.Playlist{
		ID:          req.Id,
		Name:        req.Name,
		Description: req.Description,
		Rules:       playlistRulesFromProto(req.Rules),
		SortBy:      req.SortBy,
		SortOrder:   req.SortOrder,
	}
	if updateErr := h.svc.Update(ctx, pl, userID); updateErr != nil {
		if errors.Is(updateErr, appplaylist.ErrNotOwner) {
			return nil, status.Error(codes.PermissionDenied, "not playlist owner")
		}
		return nil, status.Errorf(codes.Internal, "update playlist: %v", updateErr)
	}
	updated, err := h.svc.GetByID(ctx, req.Id, userID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get updated playlist: %v", err)
	}
	return playlistToProto(updated), nil
}

func (h *playlistHandler) DeletePlaylist(ctx context.Context, req *eveyv1.DeletePlaylistRequest) (*eveyv1.DeletePlaylistResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	userID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, err
	}
	if err := h.svc.Delete(ctx, req.Id, userID); err != nil {
		if errors.Is(err, appplaylist.ErrNotOwner) {
			return nil, status.Error(codes.PermissionDenied, "not playlist owner")
		}
		return nil, status.Errorf(codes.Internal, "delete playlist: %v", err)
	}
	return &eveyv1.DeletePlaylistResponse{}, nil
}

func (h *playlistHandler) GetPlaylistItems(ctx context.Context, req *eveyv1.GetPlaylistItemsRequest) (*eveyv1.GetPlaylistItemsResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	userID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, err
	}
	mediaItems, plItems, err := h.svc.GetItems(ctx, req.Id, userID)
	if err != nil {
		if errors.Is(err, appplaylist.ErrNotOwner) {
			return nil, status.Error(codes.PermissionDenied, "not playlist owner")
		}
		return nil, status.Errorf(codes.Internal, "get playlist items: %v", err)
	}
	resp := &eveyv1.GetPlaylistItemsResponse{}
	for _, it := range plItems {
		resp.Items = append(resp.Items, &eveyv1.PlaylistItemProto{
			PlaylistId:  it.PlaylistID,
			MediaItemId: it.MediaItemID,
			SortOrder:   int32(it.SortOrder),
			AddedAt:     toTimestamppb(it.AddedAt),
		})
	}
	for i, item := range mediaItems {
		resp.Items = append(resp.Items, &eveyv1.PlaylistItemProto{
			PlaylistId:  req.Id,
			MediaItemId: item.ID,
			SortOrder:   int32(i),
		})
	}
	return resp, nil
}

func (h *playlistHandler) AddPlaylistItem(ctx context.Context, req *eveyv1.AddPlaylistItemRequest) (*eveyv1.AddPlaylistItemResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	userID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, err
	}
	if err := h.svc.AddItem(ctx, req.Id, req.MediaItemId, userID); err != nil {
		switch {
		case errors.Is(err, appplaylist.ErrNotOwner):
			return nil, status.Error(codes.PermissionDenied, "not playlist owner")
		case errors.Is(err, appplaylist.ErrSmartItems):
			return nil, status.Error(codes.FailedPrecondition, "cannot manually add items to smart playlist")
		case errors.Is(err, appplaylist.ErrMaxItems):
			return nil, status.Error(codes.ResourceExhausted, "playlist item limit reached")
		default:
			return nil, status.Errorf(codes.Internal, "add playlist item: %v", err)
		}
	}
	return &eveyv1.AddPlaylistItemResponse{}, nil
}

func (h *playlistHandler) RemovePlaylistItem(ctx context.Context, req *eveyv1.RemovePlaylistItemRequest) (*eveyv1.RemovePlaylistItemResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	userID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, err
	}
	if err := h.svc.RemoveItem(ctx, req.Id, req.MediaItemId, userID); err != nil {
		if errors.Is(err, appplaylist.ErrNotOwner) {
			return nil, status.Error(codes.PermissionDenied, "not playlist owner")
		} else if errors.Is(err, appplaylist.ErrSmartItems) {
			return nil, status.Error(codes.FailedPrecondition, "cannot remove items from smart playlist")
		}
		return nil, status.Errorf(codes.Internal, "remove playlist item: %v", err)
	}
	return &eveyv1.RemovePlaylistItemResponse{}, nil
}

// --- converters ---

var playlistTypeToProtoMap = map[playlist.PlaylistType]eveyv1.PlaylistType{
	playlist.Manual: eveyv1.PlaylistType_PLAYLIST_TYPE_MANUAL,
	playlist.Smart:  eveyv1.PlaylistType_PLAYLIST_TYPE_SMART,
}

var playlistTypeFromProtoMap = map[eveyv1.PlaylistType]playlist.PlaylistType{
	eveyv1.PlaylistType_PLAYLIST_TYPE_MANUAL: playlist.Manual,
	eveyv1.PlaylistType_PLAYLIST_TYPE_SMART:  playlist.Smart,
}

func playlistTypeFromProto(t eveyv1.PlaylistType) playlist.PlaylistType {
	if pt, ok := playlistTypeFromProtoMap[t]; ok {
		return pt
	}
	return playlist.Manual
}

func playlistToProto(pl *playlist.Playlist) *eveyv1.PlaylistProto {
	pb := &eveyv1.PlaylistProto{
		Id:          pl.ID,
		Name:        pl.Name,
		Description: pl.Description,
		UserId:      pl.UserID,
		Type:        playlistTypeToProtoMap[pl.Type],
		SortBy:      pl.SortBy,
		SortOrder:   pl.SortOrder,
		CreatedAt:   toTimestamppb(pl.CreatedAt),
		UpdatedAt:   toTimestamppb(pl.UpdatedAt),
	}
	for _, r := range pl.Rules {
		pb.Rules = append(pb.Rules, &eveyv1.PlaylistRule{
			Field:    r.Field,
			Operator: r.Operator,
			Value:    r.Value,
		})
	}
	return pb
}

func playlistRulesFromProto(rules []*eveyv1.PlaylistRule) []playlist.Rule {
	out := make([]playlist.Rule, 0, len(rules))
	for _, r := range rules {
		out = append(out, playlist.Rule{
			Field:    r.Field,
			Operator: r.Operator,
			Value:    r.Value,
		})
	}
	return out
}
