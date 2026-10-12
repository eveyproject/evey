package grpcapi

import (
	"testing"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/stretchr/testify/assert"
)

func TestPluginStateEnumMapping(t *testing.T) {
	t.Parallel()
	cases := map[plugin.PluginStatus]eveyv1.PluginState{
		plugin.Stopped:   eveyv1.PluginState_PLUGIN_STATE_STOPPED,
		plugin.Running:   eveyv1.PluginState_PLUGIN_STATE_RUNNING,
		plugin.Error:     eveyv1.PluginState_PLUGIN_STATE_ERROR,
		plugin.Installed: eveyv1.PluginState_PLUGIN_STATE_INSTALLED,
	}
	for k, v := range cases {
		assert.Equal(t, v, pluginStateToProto[k], "state %q", k)
	}
	assert.Equal(t, eveyv1.PluginState(5), pluginStateToProto[plugin.Initializing])
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
		Status:       plugin.Running,
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

// The dashboard renders a plugin's frontend slots and its analysis report
// from the proto; both are optional on the domain side and must come across
// complete when present, and stay absent (nil, not an empty report that reads
// as "safe: false") when the plugin was never analysed.
func TestPluginToProto_FrontendAndAnalysisReport(t *testing.T) {
	t.Parallel()
	bare := pluginToProto(&plugin.Plugin{ID: "p0", Status: plugin.Stopped})
	assert.Nil(t, bare.AnalysisReport)
	assert.Empty(t, bare.Frontend)

	p := &plugin.Plugin{
		ID:            "p1",
		Status:        plugin.Error,
		ErrorMessage:  "boom",
		Kind:          plugin.KindTheme,
		ExternalFetch: []string{"api.example.test"},
		WebRoot:       "web",
		Frontend:      []plugin.FrontModule{{Slot: "player.overlay", Src: "overlay.js"}, {Slot: "settings", Src: "settings.js"}},
		AnalysisReport: &plugin.AnalysisReport{
			Safe:      false,
			Issues:    []string{"[HIGH] main.js:3 - eval (dynamic-eval)"},
			FileCount: 7,
			URLs:      []plugin.ExtractedURL{{URL: "https://api.example.test", File: "main.js", Line: 12}},
		},
	}
	pb := pluginToProto(p)
	assert.Equal(t, eveyv1.PluginState_PLUGIN_STATE_ERROR, pb.State)
	assert.Equal(t, "boom", pb.ErrorMessage)
	assert.Equal(t, string(plugin.KindTheme), pb.Kind)
	assert.Equal(t, []string{"api.example.test"}, pb.ExternalFetch)
	assert.Equal(t, "web", pb.WebRoot)
	if assert.Len(t, pb.Frontend, 2) {
		assert.Equal(t, "player.overlay", pb.Frontend[0].Slot)
		assert.Equal(t, "settings.js", pb.Frontend[1].Src)
	}
	if assert.NotNil(t, pb.AnalysisReport) {
		assert.False(t, pb.AnalysisReport.Safe)
		assert.Equal(t, int32(7), pb.AnalysisReport.FileCount)
		assert.Equal(t, []string{"[HIGH] main.js:3 - eval (dynamic-eval)"}, pb.AnalysisReport.Issues)
		if assert.Len(t, pb.AnalysisReport.Urls, 1) {
			assert.Equal(t, "https://api.example.test", pb.AnalysisReport.Urls[0].Url)
			assert.Equal(t, "main.js", pb.AnalysisReport.Urls[0].File)
			assert.Equal(t, int32(12), pb.AnalysisReport.Urls[0].Line)
		}
	}
}
