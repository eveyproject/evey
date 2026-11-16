package plugin_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appplugin "github.com/eveyproject/evey/internal/app/plugin"
)

// The gate for a source that came over the API reads the scheme whatever its
// case and ignores surrounding spaces; the download reads it the same way, or
// a link the gate let through would be opened as a path on the server.
func TestInstall_ALinkIsALinkWhateverItsCase(t *testing.T) {
	for name, spell := range map[string]func(string) string{
		"upper scheme":  func(u string) string { return "HTTPS" + strings.TrimPrefix(u, "https") },
		"mixed scheme":  func(u string) string { return "HttpS" + strings.TrimPrefix(u, "https") },
		"leading space": func(u string) string { return "  " + u },
	} {
		t.Run(name, func(t *testing.T) {
			rig := newAutoRig(t, "warn")
			rig.publish(t, goManifestVersioned("thing", "1.0.0"), rig.signed(t, "v1"))
			rig.releases.mu.Lock()
			link := spell(rig.releases.rel.ZipURL)
			rig.releases.mu.Unlock()
			require.NoError(t, appplugin.CheckRequestedSource(link))
			before := rig.zipGets.Load()

			p, err := rig.svc.InstallPlugin(context.Background(), link)
			require.NoError(t, err)
			assert.Equal(t, "1.0.0", rig.stored(t, p.ID).Version)
			assert.Greater(t, rig.zipGets.Load(), before, "the link was not downloaded")
		})
	}
}

func TestInstall_PlainHTTPIsRefusedWhateverItsCase(t *testing.T) {
	rig := newAutoRig(t, "warn")
	for _, link := range []string{"http://example.invalid/p.zip", "HTTP://example.invalid/p.zip", " hTTp://example.invalid/p.zip"} {
		_, err := rig.svc.InstallPlugin(context.Background(), link)
		require.ErrorIs(t, err, appplugin.ErrInsecureSource, link)
	}
}
