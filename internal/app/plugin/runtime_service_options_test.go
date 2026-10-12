package plugin_test

import (
	"context"
	"testing"

	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	plgfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/require"
)

// RuntimeWithShutdownCtx wires a shutdown context; cancelling it should not
// break service construction or basic ops.
func TestRuntimeWithShutdownCtx_Option(t *testing.T) {
	t.Parallel()
	pluginRepo := plgfakes.NewFakePluginRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc := appplugin.NewRuntimeService(
		pluginRepo,
		plgfakes.NewFakePluginRuntimeRepository(),
		runner,
		parser,
		bus,
		appplugin.RuntimeWithPluginDir("/tmp/test-plugins"),
		appplugin.RuntimeWithShutdownCtx(ctx),
	)

	require.NotNil(t, svc)
	// Cancelling the shutdown ctx should not panic or affect construction.
	cancel()
}
