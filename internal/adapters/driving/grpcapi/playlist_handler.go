package grpcapi

import (
	"context"
	"errors"
	"fmt"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appplaylist "github.com/gravity-zero/evey/internal/app/playlist"
	"github.com/gravity-zero/evey/internal/domain/playlist"
	eveyi18n "github.com/gravity-zero/evey/internal/pkg/i18n"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type playlistHandler struct {
	eveyv1.UnimplementedPlaylistServiceServer
	svc        *appplaylist.Service
	translator *eveyi18n.Translator
}

func (h *playlistHandler) ListPlaylists(ctx context.Context, req *eveyv1.ListPlaylistsRequest) (*eveyv1.ListPlaylistsResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.ListPlaylists: %w", err)
	}
	userID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.ListPlaylists: %w", err)
	}
	pls, err := h.svc.List(ctx, userID)
	if err != nil {
		return nil, internalError("list playlists", err)
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
		return nil, fmt.Errorf("playlistHandler.GetPlaylist: %w", err)
	}
	userID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.GetPlaylist: %w", err)
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
		return nil, fmt.Errorf("playlistHandler.CreatePlaylist: %w", err)
	}
	userID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.CreatePlaylist: %w", err)
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
		return nil, internalError("create playlist", err)
	}
	return playlistToProto(pl), nil
}

func (h *playlistHandler) UpdatePlaylist(ctx context.Context, req *eveyv1.UpdatePlaylistRequest) (*eveyv1.PlaylistProto, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.UpdatePlaylist: %w", err)
	}
	userID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.UpdatePlaylist: %w", err)
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
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.playlist.not_owner")
		}
		return nil, internalError("update playlist", updateErr)
	}
	updated, err := h.svc.GetByID(ctx, req.Id, userID)
	if err != nil {
		return nil, internalError("get updated playlist", err)
	}
	return playlistToProto(updated), nil
}

func (h *playlistHandler) DeletePlaylist(ctx context.Context, req *eveyv1.DeletePlaylistRequest) (*eveyv1.DeletePlaylistResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.DeletePlaylist: %w", err)
	}
	userID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.DeletePlaylist: %w", err)
	}
	if err := h.svc.Delete(ctx, req.Id, userID); err != nil {
		if errors.Is(err, appplaylist.ErrNotOwner) {
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.playlist.not_owner")
		}
		return nil, internalError("delete playlist", err)
	}
	return &eveyv1.DeletePlaylistResponse{}, nil
}

func (h *playlistHandler) GetPlaylistItems(ctx context.Context, req *eveyv1.GetPlaylistItemsRequest) (*eveyv1.GetPlaylistItemsResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.GetPlaylistItems: %w", err)
	}
	userID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.GetPlaylistItems: %w", err)
	}
	mediaItems, plItems, err := h.svc.GetItems(ctx, req.Id, userID)
	if err != nil {
		if errors.Is(err, appplaylist.ErrNotOwner) {
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.playlist.not_owner")
		}
		return nil, internalError("get playlist items", err)
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
		return nil, fmt.Errorf("playlistHandler.AddPlaylistItem: %w", err)
	}
	userID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.AddPlaylistItem: %w", err)
	}
	if err := h.svc.AddItem(ctx, req.Id, req.MediaItemId, userID); err != nil {
		switch {
		case errors.Is(err, appplaylist.ErrNotOwner):
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.playlist.not_owner")
		case errors.Is(err, appplaylist.ErrSmartItems):
			return nil, h.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.playlist.cannot_add_smart")
		case errors.Is(err, appplaylist.ErrMaxItems):
			return nil, h.translator.TranslatedError(ctx, codes.ResourceExhausted, "errors.playlist.limit_reached")
		default:
			return nil, internalError("add playlist item", err)
		}
	}
	return &eveyv1.AddPlaylistItemResponse{}, nil
}

func (h *playlistHandler) RemovePlaylistItem(ctx context.Context, req *eveyv1.RemovePlaylistItemRequest) (*eveyv1.RemovePlaylistItemResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.RemovePlaylistItem: %w", err)
	}
	userID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.RemovePlaylistItem: %w", err)
	}
	if err := h.svc.RemoveItem(ctx, req.Id, req.MediaItemId, userID); err != nil {
		if errors.Is(err, appplaylist.ErrNotOwner) {
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.playlist.not_owner")
		} else if errors.Is(err, appplaylist.ErrSmartItems) {
			return nil, h.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.playlist.cannot_remove_smart")
		}
		return nil, internalError("remove playlist item", err)
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
