package pluginapi

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/gravity-zero/evey/internal/adapters/driven/pluginrunner"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/plugin"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/user"
)

type LibraryPort interface {
	GetMediaItem(ctx context.Context, id string) (*library.MediaItem, error)
	ListMediaItems(ctx context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error)
}

type MetadataPort interface {
	SetOverride(ctx context.Context, override interface{}) error
}

type UserPort interface {
	ListProfiles(ctx context.Context, userID string) ([]*user.Profile, error)
}

type PluginConfigPort interface {
	GetPluginConfig(ctx context.Context, id string) (map[string]interface{}, error)
	UpdatePluginConfig(ctx context.Context, id string, config map[string]interface{}) error
}

type PluginAPIServer struct {
	library      LibraryPort
	metadata     MetadataPort
	userSvc      UserPort
	pluginConfig PluginConfigPort
	events       shared.EventBus
	sockPath     string
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

func (s *PluginAPIServer) GetMediaItem(ctx context.Context, pluginID, mediaItemID string) (*library.MediaItem, error) {
	p, err := s.getPlugin(pluginID)
	if err != nil {
		return nil, err
	}
	if !pluginrunner.CheckCapability(p, "media.read") || !pluginrunner.CheckPermission(p, "media.read") {
		return nil, fmt.Errorf("plugin %s lacks media.read capability", pluginID)
	}
	return s.library.GetMediaItem(ctx, mediaItemID)
}

func (s *PluginAPIServer) ListMediaItems(ctx context.Context, pluginID string, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	p, err := s.getPlugin(pluginID)
	if err != nil {
		return nil, err
	}
	if !pluginrunner.CheckCapability(p, "media.read") || !pluginrunner.CheckPermission(p, "media.read") {
		return nil, fmt.Errorf("plugin %s lacks media.read capability", pluginID)
	}
	return s.library.ListMediaItems(ctx, filter)
}

func (s *PluginAPIServer) UpdateMetadata(ctx context.Context, pluginID string, override interface{}) error {
	p, err := s.getPlugin(pluginID)
	if err != nil {
		return err
	}
	if !pluginrunner.CheckCapability(p, "media.write") || !pluginrunner.CheckPermission(p, "media.write") {
		return fmt.Errorf("plugin %s lacks media.write capability", pluginID)
	}
	return s.metadata.SetOverride(ctx, override)
}

func (s *PluginAPIServer) GetUserPreferences(ctx context.Context, pluginID, userID, profileID string) (map[string]string, error) {
	p, err := s.getPlugin(pluginID)
	if err != nil {
		return nil, err
	}
	if !pluginrunner.CheckPermission(p, "api.user") {
		return nil, fmt.Errorf("plugin %s lacks api.user permission", pluginID)
	}
	profiles, err := s.userSvc.ListProfiles(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, prof := range profiles {
		if prof.ID == profileID {
			return prof.Preferences, nil
		}
	}
	return nil, fmt.Errorf("profile %s not found for user %s", profileID, userID)
}

func (s *PluginAPIServer) EmitEvent(ctx context.Context, pluginID string, event shared.Event) error {
	p, err := s.getPlugin(pluginID)
	if err != nil {
		return err
	}
	if !pluginrunner.CheckCapability(p, "notification.sender") {
		return fmt.Errorf("plugin %s lacks notification.sender capability", pluginID)
	}
	return s.events.Publish(ctx, event)
}

func (s *PluginAPIServer) Log(pluginID string, level, message string, fields map[string]string) {
	log.Printf("[plugin:%s] [%s] %s %v", pluginID, level, message, fields)
}

func (s *PluginAPIServer) GetConfig(ctx context.Context, pluginID, key string) (string, bool, error) {
	config, err := s.pluginConfig.GetPluginConfig(ctx, pluginID)
	if err != nil {
		return "", false, err
	}
	val, ok := config[key]
	if !ok {
		return "", false, nil
	}
	return fmt.Sprintf("%v", val), true, nil
}

func (s *PluginAPIServer) SetConfig(ctx context.Context, pluginID, key, value string) error {
	config, err := s.pluginConfig.GetPluginConfig(ctx, pluginID)
	if err != nil {
		return err
	}
	if config == nil {
		config = make(map[string]interface{})
	}
	config[key] = value
	return s.pluginConfig.UpdatePluginConfig(ctx, pluginID, config)
}

func (s *PluginAPIServer) Start() error {
	os.Remove(s.sockPath)
	lis, err := net.Listen("unix", s.sockPath)
	if err != nil {
		return err
	}
	s.listener = lis
	log.Printf("plugin API server listening on %s", s.sockPath)
	return nil
}

func (s *PluginAPIServer) Stop() {
	if s.listener != nil {
		s.listener.Close()
	}
	os.Remove(s.sockPath)
}
