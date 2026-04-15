package grpcapi

import (
	"testing"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/stretchr/testify/assert"
)

func TestPluginStateEnumMapping(t *testing.T) {
	t.Parallel()
	cases := map[string]eveyv1.PluginState{
		"stopped":   eveyv1.PluginState_PLUGIN_STATE_STOPPED,
		"running":   eveyv1.PluginState_PLUGIN_STATE_RUNNING,
		"error":     eveyv1.PluginState_PLUGIN_STATE_ERROR,
		"installed": eveyv1.PluginState_PLUGIN_STATE_INSTALLED,
	}
	for k, v := range cases {
		assert.Equal(t, v, pluginStateToProto[k], "state %q", k)
	}
	// initializing maps to numeric value 5
	assert.Equal(t, eveyv1.PluginState(5), pluginStateToProto["initializing"])
}

func TestPluginToProto_FullFields(t *testing.T) {
	t.Parallel()
	installed := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	p := &plugin.Plugin{
		ID:           "plug-1",
		Name:         "My Plugin",
		Description:  "desc",
		Version:      "1.2.3",
		Author:       "me",
		Language:     "go",
		Enabled:      true,
		Status:       "running",
		InstalledAt:  installed,
		Permissions:  []string{"net"},
		Capabilities: []string{"scan"},
		SourceHash:   "abc",
	}
	pb := pluginToProto(p)
	assert.Equal(t, "plug-1", pb.Id)
	assert.Equal(t, "My Plugin", pb.Name)
	assert.Equal(t, "desc", pb.Description)
	assert.Equal(t, "1.2.3", pb.Version)
	assert.Equal(t, "me", pb.Author)
	assert.Equal(t, "go", pb.Language)
	assert.True(t, pb.Enabled)
	assert.Equal(t, eveyv1.PluginState_PLUGIN_STATE_RUNNING, pb.State)
	assert.Equal(t, []string{"net"}, pb.Permissions)
	assert.Equal(t, []string{"scan"}, pb.Capabilities)
	assert.Equal(t, "abc", pb.SourceHash)
	assert.Equal(t, installed.Unix(), pb.InstalledAt.AsTime().Unix())
}

