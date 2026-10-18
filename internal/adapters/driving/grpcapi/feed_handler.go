package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	appfeed "github.com/eveyproject/evey/internal/app/feed"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/feed"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"google.golang.org/grpc/codes"
)

type feedHandler struct {
	eveyv1.UnimplementedFeedServiceServer
	commands   *appfeed.CommandService
	queries    *appfeed.QueryService
	userAccess *appuser.AccessService
	libQuery   *applibrary.QueryService
	userQuery  *appuser.QueryService
	translator *eveyi18n.Translator
}

func (h *feedHandler) requireMediaAccess(ctx context.Context, mediaItemID string) error {
	if mediaItemID == "" {
		return h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.media.id_required")
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.unauthenticated")
	}
	if claims.IsAdmin {
		return nil
	}
	if h.userAccess == nil {
		return errUserAccessNotConfigured(ctx, "feedHandler.requireMediaAccess")
	}
	if h.libQuery == nil {
		return nil
	}
	item, err := h.libQuery.GetMediaItem(ctx, mediaItemID)
	if err != nil || item == nil {
		return h.translator.TranslatedError(ctx, codes.NotFound, "errors.general.not_found")
	}
	ok, accessErr := h.userAccess.HasLibraryAccess(ctx, shared.UserID(claims.UserID), item.LibraryID)
	if accessErr != nil || !ok {
		return h.translator.TranslatedError(ctx, codes.NotFound, "errors.general.not_found")
	}
	return nil
}

// filterAccessibleAndAllowed applies BOTH the library-access and the parental
// filter, resolving the underlying media items at most once. The previous code
// ran filterByLibraryAccess then filterByParental back-to-back, each issuing its
// own GetMediaItemsByIDs over the same ids - a doubled heavy resolve per feed
// section on every non-admin home load.
func (h *feedHandler) filterAccessibleAndAllowed(ctx context.Context, items []*feed.FeedItem) ([]*feed.FeedItem, error) {
	if len(items) == 0 || h.libQuery == nil {
		return items, nil
	}
	claims := interceptors.ClaimsFromContext(ctx)

	// Library access applies to non-admins with a defined access set.
	var allowedIDs []string
	libFilter := false
	if h.userAccess != nil && claims != nil && !claims.IsAdmin {
		ids, err := h.userAccess.GetLibraryAccessByUserID(ctx, claims.UserID)
		if err != nil {
			return nil, err
		}
		// Zero grants = unrestricted (opt-out default): only filter when the user
		// has an explicit whitelist. Matches getAllowedLibraryIDs / HasAccess.
		if len(ids) > 0 {
			allowedIDs = ids
			libFilter = true
		}
	}

	pf := resolveParentalFilterFromUserQuery(ctx, h.userQuery)
	parentalFilter := pf.maxAge >= 0 || len(pf.blockedTags) > 0

	if !libFilter && !parentalFilter {
		return items, nil
	}

	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, string(item.MediaItemID))
	}
	mediaItems, err := h.libQuery.GetMediaItemsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	itemByID := make(map[string]*library.MediaItem, len(mediaItems))
	for _, mi := range mediaItems {
		itemByID[string(mi.ID)] = mi
	}

	filtered := items[:0]
	for _, item := range items {
		mi := itemByID[string(item.MediaItemID)]
		if libFilter && (mi == nil || !slices.Contains(allowedIDs, string(mi.LibraryID))) {
			continue
		}
		if parentalFilter && mediaItemBlockedByParental(pf, mi) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}

func (h *feedHandler) GetFeedConfig(ctx context.Context, req *eveyv1.GetFeedConfigRequest) (*eveyv1.GetFeedConfigResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.feed.GetFeedConfig")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.GetFeedConfig: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.GetFeedConfig: %w", err)
	}
	configs, err := h.queries.GetMergedConfig(ctx, profileID)
	if err != nil {
		return nil, internalError(ctx, "get feed config", err)
	}
	resp := &eveyv1.GetFeedConfigResponse{}
	for _, c := range configs {
		resp.Configs = append(resp.Configs, mergedConfigToProto(c))
	}
	return resp, nil
}

func (h *feedHandler) ContinueWatching(ctx context.Context, req *eveyv1.ContinueWatchingRequest) (*eveyv1.FeedResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.feed.ContinueWatching")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.ContinueWatching: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.ContinueWatching: %w", err)
	}
	items, err := h.queries.ContinueWatching(ctx, profileID, int(req.Limit))
	if err != nil {
		return nil, internalError(ctx, "continue watching", err)
	}
	// Apply BOTH library-access and parental filtering, like every other feed
	// endpoint: a profile's own history can still hold items from a library
	// whose access was later revoked, and those must not resurface here.
	items, err = h.filterAccessibleAndAllowed(ctx, items)
	if err != nil {
		return nil, internalError(ctx, "continue watching: access filter", err)
	}
	return feedItemsToResponse(h.localizeTitles(ctx, items)), nil
}

func (h *feedHandler) RecentlyAdded(ctx context.Context, req *eveyv1.RecentlyAddedRequest) (*eveyv1.FeedResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.feed.RecentlyAdded")
	defer span.End()
	// Resolve the caller's profile ID so the querier can hide items the
	// profile has fully watched. Anonymous/admin contexts get an empty
	// profileID, which the querier treats as "no watched filter".
	profileID := ""
	if claims, err := requireClaims(ctx); err == nil {
		profileID = claims.ProfileID
	}
	items, err := h.queries.RecentlyAdded(ctx, profileID, int(req.DaysWindow), int(req.Limit))
	if err != nil {
		return nil, internalError(ctx, "recently added", err)
	}
	items, err = h.filterAccessibleAndAllowed(ctx, items)
	if err != nil {
		return nil, internalError(ctx, "recently added: access filter", err)
	}
	return feedItemsToResponse(h.localizeTitles(ctx, items)), nil
}

func (h *feedHandler) Recommended(ctx context.Context, req *eveyv1.RecommendedRequest) (*eveyv1.FeedResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.feed.Recommended")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.Recommended: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.Recommended: %w", err)
	}
	items, err := h.queries.Recommended(ctx, profileID, int(req.Limit))
	if err != nil {
		return nil, internalError(ctx, "recommended", err)
	}
	items, err = h.filterAccessibleAndAllowed(ctx, items)
	if err != nil {
		return nil, internalError(ctx, "recommended: access filter", err)
	}
	return feedItemsToResponse(h.localizeTitles(ctx, items)), nil
}

func (h *feedHandler) SimilarItems(ctx context.Context, req *eveyv1.SimilarItemsRequest) (*eveyv1.FeedResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.feed.SimilarItems")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.SimilarItems: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.SimilarItems: %w", err)
	}
	if err := h.requireMediaAccess(ctx, req.MediaItemId); err != nil {
		return nil, fmt.Errorf("feedHandler.SimilarItems: %w", err)
	}
	items, err := h.queries.SimilarItems(ctx, req.MediaItemId, profileID, int(req.Limit))
	if err != nil {
		return nil, internalError(ctx, "similar items", err)
	}
	items, err = h.filterAccessibleAndAllowed(ctx, items)
	if err != nil {
		return nil, internalError(ctx, "similar items: access filter", err)
	}
	return feedItemsToResponse(h.localizeTitles(ctx, items)), nil
}

func (h *feedHandler) GetAdminFeedConfigs(ctx context.Context, _ *eveyv1.GetAdminFeedConfigsRequest) (*eveyv1.GetAdminFeedConfigsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.feed.GetAdminFeedConfigs")
	defer span.End()
	configs, err := h.queries.GetAdminConfigs(ctx)
	if err != nil {
		return nil, internalError(ctx, "get admin feed configs", err)
	}
	resp := &eveyv1.GetAdminFeedConfigsResponse{}
	for _, c := range configs {
		resp.Configs = append(resp.Configs, adminConfigToProto(c))
	}
	return resp, nil
}

func (h *feedHandler) UpdateAdminFeedConfig(ctx context.Context, req *eveyv1.UpdateAdminFeedConfigRequest) (*eveyv1.AdminFeedConfig, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.feed.UpdateAdminFeedConfig")
	defer span.End()
	cfg := &feed.AdminConfig{
		FeedType:     feed.Type(req.FeedType),
		Enabled:      req.Enabled,
		DisplayOrder: int(req.DisplayOrder),
		MaxItems:     int(req.MaxItems),
	}
	if err := h.commands.UpdateAdminConfig(ctx, cfg); err != nil {
		return nil, internalError(ctx, "update admin feed config", err)
	}
	return adminConfigToProto(cfg), nil
}

func (h *feedHandler) UpdateProfileFeedConfig(ctx context.Context, req *eveyv1.UpdateProfileFeedConfigRequest) (*eveyv1.ProfileFeedConfig, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.feed.UpdateProfileFeedConfig")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.UpdateProfileFeedConfig: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.UpdateProfileFeedConfig: %w", err)
	}
	cfg := &feed.ProfileConfig{
		ProfileID:    shared.ProfileID(profileID),
		FeedType:     feed.Type(req.FeedType),
		Enabled:      req.Enabled,
		DisplayOrder: (*int)(nil),
		MaxItems:     (*int)(nil),
	}
	if req.DisplayOrder != nil {
		v := int(*req.DisplayOrder)
		cfg.DisplayOrder = &v
	}
	if req.MaxItems != nil {
		v := int(*req.MaxItems)
		cfg.MaxItems = &v
	}
	if err := h.commands.UpdateProfileConfig(ctx, cfg); err != nil {
		return nil, internalError(ctx, "update profile feed config", err)
	}
	return profileConfigToProto(cfg), nil
}

func (h *feedHandler) ResetProfileFeedConfig(ctx context.Context, req *eveyv1.ResetProfileFeedConfigRequest) (*eveyv1.ResetProfileFeedConfigResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.feed.ResetProfileFeedConfig")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.ResetProfileFeedConfig: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.ResetProfileFeedConfig: %w", err)
	}
	if err := h.commands.ResetProfileConfig(ctx, profileID, feed.Type(req.FeedType)); err != nil {
		return nil, internalError(ctx, "reset profile feed config", err)
	}
	return &eveyv1.ResetProfileFeedConfigResponse{}, nil
}

func (h *feedHandler) ResumePlayback(ctx context.Context, req *eveyv1.ResumePlaybackRequest) (*eveyv1.FeedResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.feed.ResumePlayback")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.ResumePlayback: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.ResumePlayback: %w", err)
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	items, err := h.queries.ResumePlayback(ctx, profileID, limit)
	if err != nil {
		return nil, internalError(ctx, "resume playback", err)
	}
	items, err = h.filterAccessibleAndAllowed(ctx, items)
	if err != nil {
		return nil, internalError(ctx, "resume playback: access filter", err)
	}
	return feedItemsToResponse(h.localizeTitles(ctx, items)), nil
}

func (h *feedHandler) NextEpisode(ctx context.Context, req *eveyv1.NextEpisodeRequest) (*eveyv1.FeedResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.feed.NextEpisode")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.NextEpisode: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.NextEpisode: %w", err)
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	items, err := h.queries.NextEpisode(ctx, profileID, limit)
	if err != nil {
		return nil, internalError(ctx, "next episode", err)
	}
	items, err = h.filterAccessibleAndAllowed(ctx, items)
	if err != nil {
		return nil, internalError(ctx, "next episode: access filter", err)
	}
	return feedItemsToResponse(h.localizeTitles(ctx, items)), nil
}

func (h *feedHandler) BatchUpdateAdminFeedConfigs(ctx context.Context, req *eveyv1.BatchUpdateAdminFeedConfigsRequest) (*eveyv1.BatchUpdateAdminFeedConfigsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.feed.BatchUpdateAdminFeedConfigs")
	defer span.End()
	cfgs := make([]*feed.AdminConfig, 0, len(req.Configs))
	for _, c := range req.Configs {
		cfgs = append(cfgs, &feed.AdminConfig{
			FeedType:     feed.Type(c.FeedType),
			Enabled:      c.Enabled,
			DisplayOrder: int(c.DisplayOrder),
			MaxItems:     int(c.MaxItems),
		})
	}
	if err := h.commands.BatchUpdateAdminConfigs(ctx, cfgs); err != nil {
		switch {
		case errors.Is(err, feed.ErrInvalidFeedType):
			return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.feed.invalid_type")
		case errors.Is(err, feed.ErrNegativeMaxItems):
			return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.feed.max_items_negative")
		}
		return nil, internalError(ctx, "batch update admin feed configs", err)
	}
	return &eveyv1.BatchUpdateAdminFeedConfigsResponse{}, nil
}

// feedItemTypeName is the MediaItemType enum name ("MEDIA_ITEM_TYPE_MOVIE") the
// clients read a media type as everywhere else; the feed used to send the
// domain's bare "movie", which no client label or badge recognised. A type the
// enum does not know is passed through as is.
func feedItemTypeName(domainType string) string {
	if t, ok := mediaItemTypeToProto[domainType]; ok {
		return t.String()
	}
	return domainType
}

func feedItemToProto(item *feed.FeedItem) *eveyv1.FeedItem {
	pb := &eveyv1.FeedItem{
		MediaItemId:   string(item.MediaItemID),
		ParentId:      item.ParentID,
		Title:         item.Title,
		Type:          feedItemTypeName(item.Type),
		PositionMs:    item.Position.Milliseconds(),
		AddedAt:       item.AddedAt,
		Score:         int32(item.Score),
		SeasonNumber:  int32(item.SeasonNumber),
		EpisodeNumber: int32(item.EpisodeNumber),
	}
	if item.PrimaryBlurhash != nil {
		pb.PrimaryBlurhash = *item.PrimaryBlurhash
	}
	return pb
}

// localizeTitles overlays each feed item's title with the caller-locale
// translation (media_item_translations), so the home feed matches the language
// the detail page already uses. Best-effort: a missing translation keeps the
// base title, and a fetch error is logged but never fails the feed.
func (h *feedHandler) localizeTitles(ctx context.Context, items []*feed.FeedItem) []*feed.FeedItem {
	locale := eveyi18n.LocaleFromCtx(ctx)
	if locale == "" || len(items) == 0 || h.libQuery == nil {
		return items
	}
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, string(it.MediaItemID))
	}
	trans, err := h.libQuery.GetMediaItemTranslations(ctx, ids, locale)
	if err != nil {
		slog.WarnContext(ctx, "feed: localize titles", slog.Any("err", err))
		return items
	}
	for _, it := range items {
		if t, ok := trans[string(it.MediaItemID)]; ok && t.Title != "" {
			it.Title = t.Title
		}
	}
	return items
}

func feedItemsToResponse(items []*feed.FeedItem) *eveyv1.FeedResponse {
	resp := &eveyv1.FeedResponse{}
	for _, item := range items {
		resp.Items = append(resp.Items, feedItemToProto(item))
	}
	return resp
}

func mergedConfigToProto(c *feed.MergedConfig) *eveyv1.MergedFeedConfig {
	return &eveyv1.MergedFeedConfig{
		FeedType:     string(c.FeedType),
		Enabled:      c.Enabled,
		DisplayOrder: int32(c.DisplayOrder),
		MaxItems:     int32(c.MaxItems),
	}
}

func adminConfigToProto(c *feed.AdminConfig) *eveyv1.AdminFeedConfig {
	return &eveyv1.AdminFeedConfig{
		FeedType:     string(c.FeedType),
		Enabled:      c.Enabled,
		DisplayOrder: int32(c.DisplayOrder),
		MaxItems:     int32(c.MaxItems),
	}
}

func profileConfigToProto(c *feed.ProfileConfig) *eveyv1.ProfileFeedConfig {
	pb := &eveyv1.ProfileFeedConfig{
		ProfileId: string(c.ProfileID),
		FeedType:  string(c.FeedType),
		Enabled:   c.Enabled,
	}
	if c.DisplayOrder != nil {
		v := int32(*c.DisplayOrder)
		pb.DisplayOrder = &v
	}
	if c.MaxItems != nil {
		v := int32(*c.MaxItems)
		pb.MaxItems = &v
	}
	return pb
}
