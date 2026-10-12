package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// disabled handler exercises the EVEY_PLUGINS_ENABLED=false guard. We
// don't wire any of the underlying services because every gated method
// must short-circuit before reaching them.
func newDisabledPluginHandler() *adminPluginHandler {
	return &adminPluginHandler{enabled: false}
}

func TestAdminPlugin_DisabledFlag_BlocksMutations(t *testing.T) {
	h := newDisabledPluginHandler()
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
	}{
		{"InstallPlugin", func() error { _, err := h.InstallPlugin(ctx, &eveyv1.InstallPluginRequest{Source: "any"}); return err }},
		{"UninstallPlugin", func() error { _, err := h.UninstallPlugin(ctx, &eveyv1.UninstallPluginRequest{Id: "x"}); return err }},
		{"EnablePlugin", func() error { _, err := h.EnablePlugin(ctx, &eveyv1.EnablePluginRequest{Id: "x"}); return err }},
		{"DisablePlugin", func() error { _, err := h.DisablePlugin(ctx, &eveyv1.DisablePluginRequest{Id: "x"}); return err }},
		{"AddPluginRuntime", func() error {
			_, err := h.AddPluginRuntime(ctx, &eveyv1.AddPluginRuntimeRequest{Name: "x"})
			return err
		}},
		{"RemovePluginRuntime", func() error {
			_, err := h.RemovePluginRuntime(ctx, &eveyv1.RemovePluginRuntimeRequest{Id: "x"})
			return err
		}},
		{"UpdatePluginConfig", func() error {
			_, err := h.UpdatePluginConfig(ctx, &eveyv1.UpdatePluginConfigRequest{PluginId: "x"})
			return err
		}},
		{"IssuePluginToken", func() error {
			_, err := h.IssuePluginToken(ctx, &eveyv1.IssuePluginTokenRequest{PluginId: "x"})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatalf("%s should fail when plugins disabled", tc.name)
			}
			st, ok := status.FromError(err)
			if !ok || st.Code() != codes.FailedPrecondition {
				t.Errorf("%s code = %v, want FailedPrecondition", tc.name, st.Code())
			}
		})
	}
}
