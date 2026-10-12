package transcoding

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/require"
)

// One Resolve reads the host capability and the route costs ONCE: a
// calibration published mid-request (a demotion, a re-bench) used to be read
// by the backend pick and again by the pricing, seven reads apart.
func TestResolve_ReadsTheCapabilityOnce(t *testing.T) {
	srcs := routeGoldenSources()
	var web goldenClient
	for _, c := range goldenClients() {
		if c.name == "web-hevc-4m" {
			web = c
		}
	}
	if web.name == "" {
		for _, c := range goldenClients() {
			if c.name == "web-hevc" {
				web = c
				web.capKb = 4000
			}
		}
	}
	require.NotEmpty(t, web.name)
	vaapi := vaapiCapable()
	pr, _ := routeGoldenResolver(&vaapi, true)

	capReads, costReads := 0, 0
	inner, innerCosts := pr.Capability, pr.RouteCosts
	pr.Capability = func() transcoding.HostCapabilitySnapshot { capReads++; return inner() }
	pr.RouteCosts = func() *RouteCostModel { costReads++; return innerCosts() }

	req := routeGoldenRequest(srcs["2160p-hevc-hdr10-70m"], web)
	plan, err := pr.Resolve(context.Background(), req)
	require.NoError(t, err)
	require.True(t, plan.Route.HW(), "the scenario must exercise the GPU pick and pricing")
	require.Equal(t, 1, capReads, "the capability is read once per Resolve")
	require.Equal(t, 1, costReads, "the route costs are read once per Resolve")
}
