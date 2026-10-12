package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	appplaylist "github.com/eveyproject/evey/internal/app/playlist"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/playlist"
	"github.com/eveyproject/evey/internal/domain/shared"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type playlistHandler struct {
	eveyv1.UnimplementedPlaylistServiceServer
	svc        *appplaylist.Service
	libQuery   *applibrary.QueryService
	userAccess *appuser.AccessService
	// userQuery resolves the caller's parental filter, the same way
	// collectionHandler and feedHandler do. Without it this handler could only
	// gate on library access, and a gated profile still read back the titles it
	// is not allowed to see.
	userQuery  *appuser.QueryService
	translator *eveyi18n.Translator
}

// allowedLibraryIDs returns the library IDs the caller may see, or nil for full
// access (admin / no restrictions configured). An unwired userAccess port
// fails closed instead (non-nil empty slice). Mirrors
// libraryHandler/playbackHandler.getAllowedLibraryIDs so GetPlaylistItems hides
// metadata for items whose library the caller lost (or never had) access to.
func (h *playlistHandler) allowedLibraryIDs(ctx context.Context) ([]string, error) {
	// Admin bypass is a claims check, independent of whether userAccess is
	// wired, so it is resolved before the wiring check below - an unwired
	// port must never turn an admin request into a denial.
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return nil, nil
	}
	if h.userAccess == nil {
		// nil reads as "full access" below (GetPlaylistItems); a wiring
		// regression must deny instead, like a real zero-access user would get.
		return []string{}, nil
	}
	ids, err := h.userAccess.GetLibraryAccessByUserID(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.allowedLibraryIDs: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return ids, nil
}

func (h *playlistHandler) ListPlaylists(ctx context.Context, req *eveyv1.ListPlaylistsRequest) (*eveyv1.ListPlaylistsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playlist.ListPlaylists")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.ListPlaylists: %w", err)
	}
	pls, err := h.svc.List(ctx, shared.ProfileID(claims.ProfileID))
	if err != nil {
		return nil, internalError(ctx, "list playlists", err)
	}
	resp := &eveyv1.ListPlaylistsResponse{}
	for _, pl := range pls {
		resp.Playlists = append(resp.Playlists, playlistToProto(pl))
	}
	return resp, nil
}

func (h *playlistHandler) GetPlaylist(ctx context.Context, req *eveyv1.GetPlaylistRequest) (*eveyv1.PlaylistProto, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playlist.GetPlaylist")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.GetPlaylist: %w", err)
	}
	pl, err := h.svc.GetByID(ctx, req.Id, shared.ProfileID(claims.ProfileID))
	if err != nil {
		slog.WarnContext(ctx, "grpc GetPlaylist: query failed", slog.Any("err", err))
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.playlist.not_found")
	}
	return playlistToProto(pl), nil
}

func (h *playlistHandler) CreatePlaylist(ctx context.Context, req *eveyv1.CreatePlaylistRequest) (*eveyv1.PlaylistProto, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playlist.CreatePlaylist")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.CreatePlaylist: %w", err)
	}
	pl, err := playlist.NewPlaylist(req.Name, claims.ProfileID, playlistTypeFromProto(req.Type))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	pl.Description = req.Description
	pl.Rules = playlistRulesFromProto(req.Rules)
	pl.SortBy = req.SortBy
	pl.SortOrder = req.SortOrder
	if err := h.svc.Create(ctx, pl); err != nil {
		if errors.Is(err, playlist.ErrDuplicateName) {
			// The double-tap that used to mint two identical playlists now
			// surfaces as a clean conflict instead of a 500.
			return nil, h.translator.TranslatedError(ctx, codes.AlreadyExists, "errors.playlist.duplicate_name")
		}
		return nil, internalError(ctx, "create playlist", err)
	}
	return playlistToProto(pl), nil
}

func (h *playlistHandler) UpdatePlaylist(ctx context.Context, req *eveyv1.UpdatePlaylistRequest) (*eveyv1.PlaylistProto, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playlist.UpdatePlaylist")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.UpdatePlaylist: %w", err)
	}
	profileID := shared.ProfileID(claims.ProfileID)
	pl := &playlist.Playlist{
		ID:          req.Id,
		Name:        req.Name,
		Description: req.Description,
		Rules:       playlistRulesFromProto(req.Rules),
		SortBy:      req.SortBy,
		SortOrder:   req.SortOrder,
	}
	if updateErr := h.svc.Update(ctx, pl, profileID); updateErr != nil {
		if errors.Is(updateErr, appplaylist.ErrNotOwner) || errors.Is(updateErr, shared.ErrNotFound) {
			// NotFound, not PermissionDenied: a playlist owned by another profile
			// must be indistinguishable from a non-existent one (matches GetPlaylist).
			return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.playlist.not_found")
		}
		if errors.Is(updateErr, playlist.ErrDuplicateName) {
			return nil, h.translator.TranslatedError(ctx, codes.AlreadyExists, "errors.playlist.duplicate_name")
		}
		return nil, internalError(ctx, "update playlist", updateErr)
	}
	updated, err := h.svc.GetByID(ctx, req.Id, profileID)
	if err != nil {
		return nil, internalError(ctx, "get updated playlist", err)
	}
	return playlistToProto(updated), nil
}

func (h *playlistHandler) DeletePlaylist(ctx context.Context, req *eveyv1.DeletePlaylistRequest) (*eveyv1.DeletePlaylistResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playlist.DeletePlaylist")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.DeletePlaylist: %w", err)
	}
	if err := h.svc.Delete(ctx, req.Id, shared.ProfileID(claims.ProfileID)); err != nil {
		if errors.Is(err, appplaylist.ErrNotOwner) || errors.Is(err, shared.ErrNotFound) {
			// NotFound, not PermissionDenied: a playlist owned by another profile
			// must be indistinguishable from a non-existent one (matches GetPlaylist).
			return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.playlist.not_found")
		}
		return nil, internalError(ctx, "delete playlist", err)
	}
	return &eveyv1.DeletePlaylistResponse{}, nil
}

func (h *playlistHandler) GetPlaylistItems(ctx context.Context, req *eveyv1.GetPlaylistItemsRequest) (*eveyv1.GetPlaylistItemsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playlist.GetPlaylistItems")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.GetPlaylistItems: %w", err)
	}
	mediaItems, plItems, err := h.svc.GetItems(ctx, req.Id, shared.ProfileID(claims.ProfileID))
	if err != nil {
		if errors.Is(err, appplaylist.ErrNotOwner) || errors.Is(err, shared.ErrNotFound) {
			// NotFound, not PermissionDenied: a playlist owned by another profile
			// must be indistinguishable from a non-existent one (matches GetPlaylist).
			return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.playlist.not_found")
		}
		return nil, internalError(ctx, "get playlist items", err)
	}
	mediaByID := make(map[string]*library.MediaItem, len(mediaItems))
	for _, m := range mediaItems {
		mediaByID[string(m.ID)] = m
	}
	// For manual playlists we need to fetch metadata separately
	if h.libQuery != nil && len(plItems) > 0 {
		ids := make([]string, 0, len(plItems))
		for _, it := range plItems {
			if _, ok := mediaByID[string(it.MediaItemID)]; !ok {
				ids = append(ids, string(it.MediaItemID))
			}
		}
		if len(ids) > 0 {
			// Fail closed. accessible() below deliberately keeps a row whose
			// media is nil ("carries no leakable fields"), which is right for
			// media that legitimately does not resolve - and wrong for a
			// database that cannot be read: swallowing this error made EVERY
			// row nil-media, so the library-access and parental checks both
			// turned into a pass.
			extra, eErr := h.libQuery.GetMediaItemsByIDs(ctx, ids)
			if eErr != nil {
				return nil, internalError(ctx, "get playlist items: resolve metadata", eErr)
			}
			for _, m := range extra {
				mediaByID[string(m.ID)] = m
			}
		}
	}
	// A profile may have added an item before losing access to its library (or
	// before AddItem started gating access). Hide the item's metadata when the
	// caller can no longer access its library, so GetPlaylistItems is not a
	// read-back leak. allowed==nil means full access (admin / no restriction).
	allowedIDs, err := h.allowedLibraryIDs(ctx)
	if err != nil {
		return nil, internalError(ctx, "resolve library access", err)
	}
	// Library access alone is not the whole gate. A parentally restricted
	// profile could still read back the title, year and type of a title it is
	// blocked from - the playlist may predate the restriction, or have been
	// built by another profile on the same account. Every sibling listing
	// (collections, feeds, favourites, resume/next-up) filters on this; this
	// one did not, which is the twin-guard gap that keeps recurring here.
	pf := resolveParentalFilterFromUserQuery(ctx, h.userQuery)
	accessible := func(m *library.MediaItem) bool {
		// Unknown media (metadata not resolved) carries no leakable fields, so
		// keep the bare id rather than dropping a row the owner expects to see.
		if m == nil {
			return true
		}
		if mediaItemBlockedByParental(pf, m) {
			return false
		}
		if allowedIDs == nil {
			return true
		}
		return slices.Contains(allowedIDs, string(m.LibraryID))
	}
	resp := &eveyv1.GetPlaylistItemsResponse{}
	for _, it := range plItems {
		m := mediaByID[string(it.MediaItemID)]
		if m != nil && !accessible(m) {
			continue
		}
		mt := it.MediaType
		if mt == "" {
			mt = playlist.MediaTypeVideo
		}
		pi := &eveyv1.PlaylistItemProto{
			PlaylistId:  it.PlaylistID,
			MediaItemId: string(it.MediaItemID),
			SortOrder:   int32(it.SortOrder),
			AddedAt:     toTimestamppb(it.AddedAt),
			MediaType:   mt,
		}
		if m != nil {
			pi.Title = m.Title
			pi.Type = string(m.Type)
			if m.Catalog != nil && m.Catalog.Year != nil {
				pi.Year = int32(*m.Catalog.Year)
			}
		}
		resp.Items = append(resp.Items, pi)
	}
	for i, item := range mediaItems {
		if !accessible(item) {
			continue
		}
		pi := &eveyv1.PlaylistItemProto{
			PlaylistId:  req.Id,
			MediaItemId: string(item.ID),
			SortOrder:   int32(i),
			Title:       item.Title,
			Type:        string(item.Type),
		}
		if item.Catalog != nil && item.Catalog.Year != nil {
			pi.Year = int32(*item.Catalog.Year)
		}
		resp.Items = append(resp.Items, pi)
	}
	return resp, nil
}

func (h *playlistHandler) AddPlaylistItem(ctx context.Context, req *eveyv1.AddPlaylistItemRequest) (*eveyv1.AddPlaylistItemResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playlist.AddPlaylistItem")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.AddPlaylistItem: %w", err)
	}
	// Same question AddFavorite asks before writing: may this caller act on
	// this media_item_id? The read side already filters inaccessible rows back
	// out, but the row still landed - and playlist_items.media_item_id carries
	// an FK to media_items, so "add it and see" answered whether an arbitrary
	// id exists on the server.
	if accessErr := requireMediaLibraryAccess(ctx, req.MediaItemId, h.libQuery, h.userAccess, "playlistHandler.AddPlaylistItem"); accessErr != nil {
		return nil, fmt.Errorf("playlistHandler.AddPlaylistItem: %w", accessErr)
	}
	if err := h.svc.AddItemWithType(ctx, req.Id, req.MediaItemId, shared.ProfileID(claims.ProfileID), req.MediaType); err != nil {
		switch {
		case errors.Is(err, appplaylist.ErrNotOwner), errors.Is(err, shared.ErrNotFound):
			// NotFound, not PermissionDenied: a playlist owned by another profile
			// must be indistinguishable from a non-existent one (matches GetPlaylist).
			return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.playlist.not_found")
		case errors.Is(err, appplaylist.ErrSmartItems):
			return nil, h.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.playlist.cannot_add_smart")
		case errors.Is(err, appplaylist.ErrMaxItems):
			return nil, h.translator.TranslatedError(ctx, codes.ResourceExhausted, "errors.playlist.limit_reached")
		default:
			return nil, internalError(ctx, "add playlist item", err)
		}
	}
	return &eveyv1.AddPlaylistItemResponse{}, nil
}

func (h *playlistHandler) RemovePlaylistItem(ctx context.Context, req *eveyv1.RemovePlaylistItemRequest) (*eveyv1.RemovePlaylistItemResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playlist.RemovePlaylistItem")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("playlistHandler.RemovePlaylistItem: %w", err)
	}
	if err := h.svc.RemoveItem(ctx, req.Id, req.MediaItemId, shared.ProfileID(claims.ProfileID)); err != nil {
		if errors.Is(err, appplaylist.ErrNotOwner) || errors.Is(err, shared.ErrNotFound) {
			// NotFound, not PermissionDenied: a playlist owned by another profile
			// must be indistinguishable from a non-existent one (matches GetPlaylist).
			return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.playlist.not_found")
		} else if errors.Is(err, appplaylist.ErrSmartItems) {
			return nil, h.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.playlist.cannot_remove_smart")
		}
		return nil, internalError(ctx, "remove playlist item", err)
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
		// Proto field is still named user_id (wire-compat, frontend-unused); it
		// now carries the owning profile id since playlists are profile-scoped.
		UserId:    string(pl.ProfileID),
		Type:      playlistTypeToProtoMap[pl.Type],
		SortBy:    pl.SortBy,
		SortOrder: pl.SortOrder,
		CreatedAt: toTimestamppb(pl.CreatedAt),
		UpdatedAt: toTimestamppb(pl.UpdatedAt),
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
