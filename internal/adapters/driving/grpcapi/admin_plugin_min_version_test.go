package grpcapi

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	pluginfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// A plugin that needs a newer server is the admin's problem to solve, not a
// server fault: the RPC must say so with the versions involved.
func TestInstallPlugin_ServerTooOldIsAPrecondition(t *testing.T) {
	parser := pluginfakes.NewFakeManifestParser()
	parser.Result = &plugindomain.PluginManifest{Name: "needy", Version: "1.0.0", Kind: plugindomain.KindTheme, MinEveyVersion: "9.0.0"}
	h := &adminPluginHandler{
		pluginInstaller: appplugin.NewInstallerService(pluginfakes.NewFakePluginRepository(),
			pluginfakes.NewFakePluginRunner(), parser, sharedfakes.NewFakeEventBus(),
			appplugin.InstallerWithServerVersion("0.1.0")),
		enabled: true,
	}
	zip := filepath.Join(t.TempDir(), "p.zip")
	require.NoError(t, os.WriteFile(zip, []byte("zip"), 0o644))

	_, err := h.InstallPlugin(context.Background(), &eveyv1.InstallPluginRequest{Source: zip})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "9.0.0")
}
