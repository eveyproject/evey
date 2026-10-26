package plugin_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appplugin "github.com/eveyproject/evey/internal/app/plugin"
)

// A manifest's min_evey_version is a promise the server must keep: a plugin
// built against newer host APIs is refused at install and at update instead
// of failing at runtime. An unknown server version ("" or a dev build) checks
// nothing.
func TestInstall_MinEveyVersion(t *testing.T) {
	for _, c := range []struct {
		server, min string
		refused     bool
	}{
		{"0.4.2", "0.5.0", true},
		{"v0.4.2", "0.4.3", true},
		{"0.4.2", "0.4.2", false},
		{"v0.4.2", "0.3.9", false},
		{"0.4.2", "", false},
		{"", "9.0.0", false},
		{"dev", "9.0.0", false},
	} {
		rig := newPublisherRig(t, "off", appplugin.InstallerWithServerVersion(c.server))
		m := goManifestVersioned("versioned-plugin", "1.0.0")
		m.MinEveyVersion = c.min
		rig.parser.Result = m
		_, err := rig.svc.InstallPlugin(context.Background(), pkg(t, nil, "v1"))
		if c.refused {
			require.ErrorIs(t, err, appplugin.ErrServerTooOld, "server %q, min %q", c.server, c.min)
		} else {
			assert.NoError(t, err, "server %q, min %q", c.server, c.min)
		}
	}
}

func TestUpdate_MinEveyVersion(t *testing.T) {
	rig := newPublisherRig(t, "off", appplugin.InstallerWithServerVersion("0.4.2"))
	p, err := rig.install(t, "1.0.0", pkg(t, nil, "v1"))
	require.NoError(t, err)

	m := goManifestVersioned("signed-plugin", "1.1.0")
	m.MinEveyVersion = "0.5.0"
	rig.parser.Result = m
	candidate := pkg(t, nil, "v2")
	preview, err := rig.svc.PreviewUpdate(context.Background(), p.ID, candidate)
	require.NoError(t, err)
	_, err = rig.svc.ApplyUpdate(context.Background(), p.ID, candidate, preview.SourceHash, false)
	require.ErrorIs(t, err, appplugin.ErrServerTooOld)
}
