package grpcapi

import (
	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/domain/plugin"
)

var pluginStateToProto = map[string]eveyv1.PluginState{
	"stopped":      eveyv1.PluginState_PLUGIN_STATE_STOPPED,
	"initializing": eveyv1.PluginState(5),
	"running":      eveyv1.PluginState_PLUGIN_STATE_RUNNING,
	"error":        eveyv1.PluginState_PLUGIN_STATE_ERROR,
	"installed":    eveyv1.PluginState_PLUGIN_STATE_INSTALLED,
}

func pluginToProto(p *plugin.Plugin) *eveyv1.Plugin {
	return &eveyv1.Plugin{
		Id:           p.ID,
		Name:         p.Name,
		Description:  p.Description,
		Version:      p.Version,
		Author:       p.Author,
		Language:     p.Language,
		Enabled:      p.Enabled,
		State:        pluginStateToProto[p.Status],
		InstalledAt:  toTimestamppb(p.InstalledAt),
		Permissions:  p.Permissions,
		Capabilities: p.Capabilities,
		SourceHash:   p.SourceHash,
	}
}

func pluginRuntimeToProto(rt *plugin.PluginRuntime) *eveyv1.PluginRuntime {
	return &eveyv1.PluginRuntime{
		Id:       rt.ID,
		Language: rt.Language,
		Version:  rt.Version,
	}
}
