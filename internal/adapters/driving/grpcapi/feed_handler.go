package grpcapi

import (
	"context"
	"fmt"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appfeed "github.com/gravity-zero/evey/internal/app/feed"
	"github.com/gravity-zero/evey/internal/domain/feed"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type feedHandler struct {
	eveyv1.UnimplementedFeedServiceServer
	commands *appfeed.CommandService
	queries  *appfeed.QueryService
}

func (h *feedHandler) GetFeedConfig(ctx context.Context, req *eveyv1.GetFeedConfigRequest) (*eveyv1.GetFeedConfigResponse, error) {
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
		return nil, status.Errorf(codes.Internal, "get feed config: %v", err)
	}
	resp := &eveyv1.GetFeedConfigResponse{}
	for _, c := range configs {
		resp.Configs = append(resp.Configs, mergedConfigToProto(c))
	}
	return resp, nil
}

func (h *feedHandler) ContinueWatching(ctx context.Context, req *eveyv1.ContinueWatchingRequest) (*eveyv1.FeedResponse, error) {
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
		return nil, status.Errorf(codes.Internal, "continue watching: %v", err)
	}
	return feedItemsToResponse(items), nil
}

func (h *feedHandler) RecentlyAdded(ctx context.Context, req *eveyv1.RecentlyAddedRequest) (*eveyv1.FeedResponse, error) {
	items, err := h.queries.RecentlyAdded(ctx, int(req.DaysWindow), int(req.Limit))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "recently added: %v", err)
	}
	return feedItemsToResponse(items), nil
}

func (h *feedHandler) Recommended(ctx context.Context, req *eveyv1.RecommendedRequest) (*eveyv1.FeedResponse, error) {
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
		return nil, status.Errorf(codes.Internal, "recommended: %v", err)
	}
	return feedItemsToResponse(items), nil
}

func (h *feedHandler) SimilarItems(ctx context.Context, req *eveyv1.SimilarItemsRequest) (*eveyv1.FeedResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.SimilarItems: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.SimilarItems: %w", err)
	}
	items, err := h.queries.SimilarItems(ctx, req.MediaItemId, profileID, int(req.Limit))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "similar items: %v", err)
	}
	return feedItemsToResponse(items), nil
}

func (h *feedHandler) GetAdminFeedConfigs(ctx context.Context, _ *eveyv1.GetAdminFeedConfigsRequest) (*eveyv1.GetAdminFeedConfigsResponse, error) {
	configs, err := h.queries.GetAdminConfigs(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get admin feed configs: %v", err)
	}
	resp := &eveyv1.GetAdminFeedConfigsResponse{}
	for _, c := range configs {
		resp.Configs = append(resp.Configs, adminConfigToProto(c))
	}
	return resp, nil
}

func (h *feedHandler) UpdateAdminFeedConfig(ctx context.Context, req *eveyv1.UpdateAdminFeedConfigRequest) (*eveyv1.AdminFeedConfig, error) {
	cfg := &feed.AdminConfig{
		FeedType:     feed.Type(req.FeedType),
		Enabled:      req.Enabled,
		DisplayOrder: int(req.DisplayOrder),
		MaxItems:     int(req.MaxItems),
	}
	if err := h.commands.UpdateAdminConfig(ctx, cfg); err != nil {
		return nil, status.Errorf(codes.Internal, "update admin feed config: %v", err)
	}
	return adminConfigToProto(cfg), nil
}

func (h *feedHandler) UpdateProfileFeedConfig(ctx context.Context, req *eveyv1.UpdateProfileFeedConfigRequest) (*eveyv1.ProfileFeedConfig, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.UpdateProfileFeedConfig: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.UpdateProfileFeedConfig: %w", err)
	}
	cfg := &feed.ProfileConfig{
		ProfileID:    profileID,
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
		return nil, status.Errorf(codes.Internal, "update profile feed config: %v", err)
	}
	return profileConfigToProto(cfg), nil
}

func (h *feedHandler) ResetProfileFeedConfig(ctx context.Context, req *eveyv1.ResetProfileFeedConfigRequest) (*eveyv1.ResetProfileFeedConfigResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.ResetProfileFeedConfig: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("feedHandler.ResetProfileFeedConfig: %w", err)
	}
	if err := h.commands.ResetProfileConfig(ctx, profileID, feed.Type(req.FeedType)); err != nil {
		return nil, status.Errorf(codes.Internal, "reset profile feed config: %v", err)
	}
	return &eveyv1.ResetProfileFeedConfigResponse{}, nil
}

func (h *feedHandler) ResumePlayback(ctx context.Context, req *eveyv1.ResumePlaybackRequest) (*eveyv1.FeedResponse, error) {
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
		return nil, status.Errorf(codes.Internal, "resume playback: %v", err)
	}
	return feedItemsToResponse(items), nil
}

func (h *feedHandler) NextEpisode(ctx context.Context, req *eveyv1.NextEpisodeRequest) (*eveyv1.FeedResponse, error) {
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
		return nil, status.Errorf(codes.Internal, "next episode: %v", err)
	}
	return feedItemsToResponse(items), nil
}

func (h *feedHandler) BatchUpdateAdminFeedConfigs(ctx context.Context, req *eveyv1.BatchUpdateAdminFeedConfigsRequest) (*eveyv1.BatchUpdateAdminFeedConfigsResponse, error) {
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
		return nil, status.Errorf(codes.InvalidArgument, "batch update: %v", err)
	}
	return &eveyv1.BatchUpdateAdminFeedConfigsResponse{}, nil
}

func feedItemToProto(item *feed.FeedItem) *eveyv1.FeedItem {
	return &eveyv1.FeedItem{
		MediaItemId: item.MediaItemID,
		ParentId:    item.ParentID,
		Title:       item.Title,
		Type:        item.Type,
		PositionMs:  item.PositionMs,
		AddedAt:     item.AddedAt,
		Score:       int32(item.Score),
	}
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
		ProfileId: c.ProfileID,
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
