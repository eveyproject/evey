package plugin

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/eveyproject/evey/internal/domain/plugin"
)

// ErrNotMetadataPlugin refuses metadata settings on a plugin without the
// metadata.provider capability.
var ErrNotMetadataPlugin = errors.New("plugin is not a metadata provider")

// ErrUnknownMetadataField refuses a field a metadata plugin cannot replace.
var ErrUnknownMetadataField = errors.New("unknown metadata field")

// MetadataSettingsService reads and sets the fields each metadata plugin may
// replace on an item.
type MetadataSettingsService struct {
	plugins  plugin.PluginRepository
	settings plugin.MetadataSettingsRepository
}

func NewMetadataSettingsService(plugins plugin.PluginRepository, settings plugin.MetadataSettingsRepository) *MetadataSettingsService {
	return &MetadataSettingsService{plugins: plugins, settings: settings}
}

// metadataPlugin returns the plugin when it is a metadata provider.
func (s *MetadataSettingsService) metadataPlugin(ctx context.Context, pluginID string) error {
	p, err := s.plugins.GetByID(ctx, pluginID)
	if err != nil || p == nil {
		return ErrPluginNotFound
	}
	if !slices.Contains(p.Capabilities, plugin.CapabilityMetadataProvider) {
		return ErrNotMetadataPlugin
	}
	return nil
}

// Fields is the plugin's choice, the default when none was made.
func (s *MetadataSettingsService) Fields(ctx context.Context, pluginID string) ([]string, error) {
	if err := s.metadataPlugin(ctx, pluginID); err != nil {
		return nil, err
	}
	return s.settings.MetadataFields(ctx, pluginID)
}

// SetFields stores the plugin's choice, checked and in display order.
func (s *MetadataSettingsService) SetFields(ctx context.Context, pluginID string, fields []string) ([]string, error) {
	if err := s.metadataPlugin(ctx, pluginID); err != nil {
		return nil, err
	}
	normalized, err := plugin.NormalizeMetadataFields(fields)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnknownMetadataField, err)
	}
	if err := s.settings.SetMetadataFields(ctx, pluginID, normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}
