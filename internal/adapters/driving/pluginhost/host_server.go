package pluginhost

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"

	pluginv1 "github.com/gravity-zero/evey/gen/plugin/v1"
	"github.com/gravity-zero/evey/internal/domain/library"
	plugindomain "github.com/gravity-zero/evey/internal/domain/plugin"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const hostSockPath = "/tmp/evey/plugins/evey-host.sock"

type MediaItemQuerier interface {
	GetByID(ctx context.Context, id string) (*library.MediaItem, error)
	List(ctx context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error)
	Count(ctx context.Context, filter library.ListMediaItemsFilter) (int, error)
}

type PluginConfigStore interface {
	GetByID(ctx context.Context, id string) (*plugindomain.Plugin, error)
	Update(ctx context.Context, p *plugindomain.Plugin) error
}

type HostServer struct {
	pluginv1.UnimplementedHostServiceServer
	mediaItems      MediaItemQuerier
	plugins         PluginConfigStore
	events          shared.EventBus
	logger          plugindomain.PluginLogger
	permInterceptor grpc.UnaryServerInterceptor
	grpcSrv         *grpc.Server
}

func NewHostServer(
	mediaItems MediaItemQuerier,
	plugins PluginConfigStore,
	events shared.EventBus,
	logger plugindomain.PluginLogger,
	permInterceptor grpc.UnaryServerInterceptor,
) *HostServer {
	return &HostServer{
		mediaItems:      mediaItems,
		plugins:         plugins,
		events:          events,
		logger:          logger,
		permInterceptor: permInterceptor,
	}
}

func (h *HostServer) Start() error {
	sockDir := filepath.Dir(hostSockPath)
	if err := os.MkdirAll(sockDir, 0o755); err != nil {
		return fmt.Errorf("create host sock dir: %w", err)
	}
	if err := os.Remove(hostSockPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("remove old host sock", slog.Any("err", err))
	}

	lis, err := net.Listen("unix", hostSockPath)
	if err != nil {
		return fmt.Errorf("listen host sock: %w", err)
	}

	if h.permInterceptor != nil {
		h.grpcSrv = grpc.NewServer(grpc.ChainUnaryInterceptor(h.pluginIDInterceptor(), h.permInterceptor))
	} else {
		h.grpcSrv = grpc.NewServer(grpc.UnaryInterceptor(h.pluginIDInterceptor()))
	}
	pluginv1.RegisterHostServiceServer(h.grpcSrv, h)

	slog.Info("host gRPC server started", slog.String("sock", hostSockPath))

	go func() {
		if err := h.grpcSrv.Serve(lis); err != nil {
			slog.Error("host gRPC serve", slog.Any("err", err))
		}
	}()
	return nil
}

func (h *HostServer) Stop() {
	if h.grpcSrv != nil {
		h.grpcSrv.GracefulStop()
	}
	if err := os.Remove(hostSockPath); err != nil {
		slog.Warn("remove host sock", slog.Any("err", err))
	}
}

func (h *HostServer) pluginIDInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}
		ids := md.Get("x-plugin-id")
		if len(ids) == 0 || ids[0] == "" {
			return nil, status.Error(codes.Unauthenticated, "missing x-plugin-id")
		}
		ctx = context.WithValue(ctx, pluginIDKey{}, ids[0])
		return handler(ctx, req)
	}
}

type pluginIDKey struct{}

func pluginIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(pluginIDKey{}).(string); ok {
		return v
	}
	return ""
}

func (h *HostServer) GetMediaItem(ctx context.Context, req *pluginv1.GetMediaItemRequest) (*pluginv1.MediaItem, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id required")
	}
	item, err := h.mediaItems.GetByID(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "media item %s not found", req.Id)
	}
	return mediaItemToProto(item), nil
}

func (h *HostServer) ListMediaItems(ctx context.Context, req *pluginv1.ListMediaItemsRequest) (*pluginv1.ListMediaItemsResponse, error) {
	filter := library.ListMediaItemsFilter{}
	if req.LibraryId != "" {
		filter.LibraryID = &req.LibraryId
	}
	if req.Type != "" {
		filter.Type = &req.Type
	}
	if req.ParentId != "" {
		filter.ParentID = &req.ParentId
	}
	if req.Limit > 0 {
		filter.Limit = int(req.Limit)
	}
	if req.Offset > 0 {
		filter.Offset = int(req.Offset)
	}

	items, err := h.mediaItems.List(ctx, filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list media: %v", err)
	}
	total, err := h.mediaItems.Count(ctx, filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "count media: %v", err)
	}

	pbItems := make([]*pluginv1.MediaItem, 0, len(items))
	for _, item := range items {
		pbItems = append(pbItems, mediaItemToProto(item))
	}
	return &pluginv1.ListMediaItemsResponse{Items: pbItems, Total: int32(total)}, nil
}

func (h *HostServer) UpdateMetadata(_ context.Context, _ *pluginv1.UpdateMetadataRequest) (*pluginv1.UpdateMetadataResponse, error) {
	return nil, status.Error(codes.Unimplemented, "UpdateMetadata not yet implemented")
}

func (h *HostServer) GetUserPreferences(_ context.Context, _ *pluginv1.GetUserPreferencesRequest) (*pluginv1.GetUserPreferencesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetUserPreferences not yet implemented")
}

func (h *HostServer) EmitEvent(ctx context.Context, req *pluginv1.EmitEventRequest) (*pluginv1.EmitEventResponse, error) {
	pluginID := pluginIDFromCtx(ctx)

	payload := make(map[string]any)
	if req.Payload != nil {
		payload = req.Payload.AsMap()
	}

	evt := pluginEmittedEvent{
		pluginID:  pluginID,
		eventType: req.EventType,
		payload:   payload,
	}
	if err := h.events.Publish(ctx, evt); err != nil {
		return nil, status.Errorf(codes.Internal, "publish event: %v", err)
	}
	return &pluginv1.EmitEventResponse{}, nil
}

func (h *HostServer) Log(ctx context.Context, req *pluginv1.LogRequest) (*pluginv1.LogResponse, error) {
	pluginID := pluginIDFromCtx(ctx)
	level := strings.ToUpper(req.Level.String())
	if level == "LOG_LEVEL_UNSPECIFIED" {
		level = "INFO"
	}
	level = strings.TrimPrefix(level, "LOG_LEVEL_")

	if h.logger != nil {
		h.logger.WriteLog(pluginID, level, req.Message, req.Fields)
	}
	return &pluginv1.LogResponse{}, nil
}

func (h *HostServer) GetConfig(ctx context.Context, req *pluginv1.GetConfigRequest) (*pluginv1.GetConfigResponse, error) {
	pluginID := pluginIDFromCtx(ctx)
	p, err := h.plugins.GetByID(ctx, pluginID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "plugin %s not found", pluginID)
	}
	val, ok := p.Config[req.Key]
	if !ok {
		return &pluginv1.GetConfigResponse{Found: false}, nil
	}
	return &pluginv1.GetConfigResponse{Value: fmt.Sprintf("%v", val), Found: true}, nil
}

func (h *HostServer) SetConfig(ctx context.Context, req *pluginv1.SetConfigRequest) (*pluginv1.SetConfigResponse, error) {
	pluginID := pluginIDFromCtx(ctx)
	p, err := h.plugins.GetByID(ctx, pluginID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "plugin %s not found", pluginID)
	}
	if p.Config == nil {
		p.Config = make(map[string]any)
	}
	p.Config[req.Key] = req.Value
	if err := h.plugins.Update(ctx, p); err != nil {
		return nil, status.Errorf(codes.Internal, "save config: %v", err)
	}
	return &pluginv1.SetConfigResponse{}, nil
}

func mediaItemToProto(item *library.MediaItem) *pluginv1.MediaItem {
	pb := &pluginv1.MediaItem{
		Id:        item.ID,
		LibraryId: item.LibraryID,
		ParentId:  item.ParentID,
		Type:      item.Type,
		Title:     item.Title,
		Overview:  item.Overview,
		FilePath:  item.FilePath,
		Genres:    item.Genres,
	}
	if item.Year != nil {
		pb.Year = int32(*item.Year)
	}
	if item.ProviderIDs != nil {
		pb.ProviderIds = item.ProviderIDs
	}
	return pb
}

type pluginEmittedEvent struct {
	pluginID  string
	eventType string
	payload   map[string]any
}

func (e pluginEmittedEvent) EventName() string {
	return "plugin.event." + e.eventType
}
