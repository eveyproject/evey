package pluginapi

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"

	pluginv1 "github.com/gravity-zero/evey/gen/plugin/v1"
	"github.com/gravity-zero/evey/internal/adapters/driven/pluginrunner"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/plugin"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type LibraryPort interface {
	GetMediaItem(ctx context.Context, id string) (*library.MediaItem, error)
	ListMediaItems(ctx context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error)
}

type MetadataPort interface {
	SetOverride(ctx context.Context, override any) error
}

type UserPort interface {
	ListProfiles(ctx context.Context, userID string) ([]*user.Profile, error)
}

type PluginConfigPort interface {
	GetPluginConfig(ctx context.Context, id string) (map[string]any, error)
	UpdatePluginConfig(ctx context.Context, id string, config map[string]any) error
}

type PluginAPIServer struct {
	pluginv1.UnimplementedHostServiceServer

	library      LibraryPort
	metadata     MetadataPort
	userSvc      UserPort
	pluginConfig PluginConfigPort
	events       shared.EventBus
	sockPath     string
	grpcServer   *grpc.Server
	listener     net.Listener

	mu      sync.RWMutex
	plugins map[string]*plugin.Plugin
}

func NewPluginAPIServer(
	library LibraryPort,
	metadata MetadataPort,
	userSvc UserPort,
	pluginConfig PluginConfigPort,
	events shared.EventBus,
	sockDir string,
) *PluginAPIServer {
	return &PluginAPIServer{
		library:      library,
		metadata:     metadata,
		userSvc:      userSvc,
		pluginConfig: pluginConfig,
		events:       events,
		sockPath:     filepath.Join(sockDir, "evey-host.sock"),
		plugins:      make(map[string]*plugin.Plugin),
	}
}

func (s *PluginAPIServer) RegisterPlugin(p *plugin.Plugin) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.plugins[p.ID] = p
}

func (s *PluginAPIServer) UnregisterPlugin(pluginID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.plugins, pluginID)
}

func (s *PluginAPIServer) getPlugin(pluginID string) (*plugin.Plugin, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.plugins[pluginID]
	if !ok {
		return nil, fmt.Errorf("plugin %s not registered", pluginID)
	}
	return p, nil
}

// pluginIDFromCtx extracts x-plugin-id from gRPC incoming metadata.
func pluginIDFromCtx(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get("x-plugin-id")
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// --- HostServiceServer implementation ---

func (s *PluginAPIServer) GetMediaItem(ctx context.Context, req *pluginv1.GetMediaItemRequest) (*pluginv1.MediaItem, error) {
	pluginID := pluginIDFromCtx(ctx)
	p, err := s.getPlugin(pluginID)
	if err != nil {
		return nil, status.Errorf(codes.PermissionDenied, "%v", err)
	}
	if !pluginrunner.CheckCapability(p, "media.read") || !pluginrunner.CheckPermission(p, "media.read") {
		return nil, status.Errorf(codes.PermissionDenied, "plugin %s lacks media.read", pluginID)
	}
	item, err := s.library.GetMediaItem(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return domainItemToProto(item), nil
}

func (s *PluginAPIServer) ListMediaItems(ctx context.Context, req *pluginv1.ListMediaItemsRequest) (*pluginv1.ListMediaItemsResponse, error) {
	pluginID := pluginIDFromCtx(ctx)
	p, err := s.getPlugin(pluginID)
	if err != nil {
		return nil, status.Errorf(codes.PermissionDenied, "%v", err)
	}
	if !pluginrunner.CheckCapability(p, "media.read") || !pluginrunner.CheckPermission(p, "media.read") {
		return nil, status.Errorf(codes.PermissionDenied, "plugin %s lacks media.read", pluginID)
	}

	filter := library.ListMediaItemsFilter{
		Limit:  int(req.Limit),
		Offset: int(req.Offset),
	}
	if req.LibraryId != "" {
		filter.LibraryID = &req.LibraryId
	}
	if req.ParentId != "" {
		filter.ParentID = &req.ParentId
	}
	if req.Type != "" {
		filter.Type = &req.Type
	}

	items, err := s.library.ListMediaItems(ctx, filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	resp := &pluginv1.ListMediaItemsResponse{Total: int32(len(items))}
	for _, item := range items {
		resp.Items = append(resp.Items, domainItemToProto(item))
	}
	return resp, nil
}

func (s *PluginAPIServer) UpdateMetadata(ctx context.Context, req *pluginv1.UpdateMetadataRequest) (*pluginv1.UpdateMetadataResponse, error) {
	pluginID := pluginIDFromCtx(ctx)
	p, err := s.getPlugin(pluginID)
	if err != nil {
		return nil, status.Errorf(codes.PermissionDenied, "%v", err)
	}
	if !pluginrunner.CheckCapability(p, "media.write") || !pluginrunner.CheckPermission(p, "media.write") {
		return nil, status.Errorf(codes.PermissionDenied, "plugin %s lacks media.write", pluginID)
	}
	override := map[string]any{
		"media_item_id": req.MediaItemId,
		"fields":        req.Fields,
		"provider_ids":  req.ProviderIds,
	}
	if err := s.metadata.SetOverride(ctx, override); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &pluginv1.UpdateMetadataResponse{Success: true}, nil
}

func (s *PluginAPIServer) GetUserPreferences(ctx context.Context, req *pluginv1.GetUserPreferencesRequest) (*pluginv1.GetUserPreferencesResponse, error) {
	pluginID := pluginIDFromCtx(ctx)
	p, err := s.getPlugin(pluginID)
	if err != nil {
		return nil, status.Errorf(codes.PermissionDenied, "%v", err)
	}
	if !pluginrunner.CheckPermission(p, "api.user") {
		return nil, status.Errorf(codes.PermissionDenied, "plugin %s lacks api.user permission", pluginID)
	}
	profiles, err := s.userSvc.ListProfiles(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	for _, prof := range profiles {
		if prof.ID == req.ProfileId {
			return &pluginv1.GetUserPreferencesResponse{Preferences: prof.Preferences}, nil
		}
	}
	return nil, status.Errorf(codes.NotFound, "profile %s not found for user %s", req.ProfileId, req.UserId)
}

func (s *PluginAPIServer) EmitEvent(ctx context.Context, req *pluginv1.EmitEventRequest) (*pluginv1.EmitEventResponse, error) {
	pluginID := pluginIDFromCtx(ctx)
	p, err := s.getPlugin(pluginID)
	if err != nil {
		return nil, status.Errorf(codes.PermissionDenied, "%v", err)
	}
	if !pluginrunner.CheckCapability(p, "notification.sender") {
		return nil, status.Errorf(codes.PermissionDenied, "plugin %s lacks notification.sender capability", pluginID)
	}
	_ = s.events.Publish(ctx, pluginEvent{eventType: req.EventType})
	return &pluginv1.EmitEventResponse{}, nil
}

func (s *PluginAPIServer) Log(_ context.Context, req *pluginv1.LogRequest) (*pluginv1.LogResponse, error) {
	log.Printf("[plugin] [%s] %s %v", req.Level, req.Message, req.Fields)
	return &pluginv1.LogResponse{}, nil
}

func (s *PluginAPIServer) GetConfig(ctx context.Context, req *pluginv1.GetConfigRequest) (*pluginv1.GetConfigResponse, error) {
	pluginID := pluginIDFromCtx(ctx)
	config, err := s.pluginConfig.GetPluginConfig(ctx, pluginID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	val, ok := config[req.Key]
	if !ok {
		return &pluginv1.GetConfigResponse{Found: false}, nil
	}
	return &pluginv1.GetConfigResponse{Value: fmt.Sprintf("%v", val), Found: true}, nil
}

func (s *PluginAPIServer) SetConfig(ctx context.Context, req *pluginv1.SetConfigRequest) (*pluginv1.SetConfigResponse, error) {
	pluginID := pluginIDFromCtx(ctx)
	config, err := s.pluginConfig.GetPluginConfig(ctx, pluginID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	if config == nil {
		config = make(map[string]any)
	}
	config[req.Key] = req.Value
	if err := s.pluginConfig.UpdatePluginConfig(ctx, pluginID, config); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &pluginv1.SetConfigResponse{}, nil
}

func (s *PluginAPIServer) Start() error {
	os.Remove(s.sockPath)
	lis, err := net.Listen("unix", s.sockPath)
	if err != nil {
		return err
	}
	s.listener = lis
	s.grpcServer = grpc.NewServer()
	pluginv1.RegisterHostServiceServer(s.grpcServer, s)
	log.Printf("plugin host service listening on %s", s.sockPath)
	go s.grpcServer.Serve(lis)
	return nil
}

func (s *PluginAPIServer) Stop() {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
	os.Remove(s.sockPath)
}

type pluginEvent struct{ eventType string }

func (e pluginEvent) EventName() string { return e.eventType }

func domainItemToProto(item *library.MediaItem) *pluginv1.MediaItem {
	pb := &pluginv1.MediaItem{
		Id:        item.ID,
		LibraryId: item.LibraryID,
		Type:      item.Type,
		Title:     item.Title,
		Overview:  item.Overview,
		FilePath:  item.FilePath,
	}
	pb.ParentId = item.ParentID
	if item.Year != nil {
		pb.Year = int32(*item.Year)
	}
	if item.ProviderIDs != nil {
		pb.ProviderIds = item.ProviderIDs
	}
	pb.Genres = item.Genres
	return pb
}
