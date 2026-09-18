package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

func cellSet(cells []transcoding.ExpressibilityCell) map[string]bool {
	out := map[string]bool{}
	for _, c := range cells {
		out[c.Backend+"/"+c.Topology+"/"+c.Needs] = true
	}
	return out
}

// The expected cells are what the RESOLVER resolves, never a product of
// proofs: on the two-backend host QSV wins SDR/HEVC and VAAPI's OpenCL wins
// the tone-map, so VAAPI's zero-copy plain is reached only by the source QSV
// cannot take, no bridge is ever resolved (a GPU tone-mapper exists), and the
// decoupled route - proven but unmeasured - is never fielded.
func TestExpectedExpressibilityCells_FollowTheResolver(t *testing.T) {
	got := cellSet(ExpectedExpressibilityCells(twoBackendResolver()))
	for _, want := range []string{"qsv/zero-copy/plain", "vaapi/gpu-tonemap/plain", "qsv/passthrough/plain"} {
		assert.True(t, got[want], "%s missing from %v", want, got)
	}
	for _, no := range []string{"qsv/bridge/plain", "vaapi/bridge/plain", "qsv/decoupled/plain", "vaapi/decoupled/plain", "qsv/zero-copy/burn-in+overlay"} {
		assert.False(t, got[no], "%s must not be expected: the resolver never resolves it here", no)
	}

	// A QSV without a GPU tone-mapper bridges; one that proved its
	// deinterlacer and a compositor reaches deint and burn-in+overlay; no
	// HDR10 encoder, no passthrough.
	cap := qsvDeintCapability()
	pr, _ := routeGoldenResolver(&cap, true)
	got = cellSet(ExpectedExpressibilityCells(pr))
	for _, want := range []string{"qsv/zero-copy/plain", "qsv/bridge/plain", "qsv/decoupled/plain", "qsv/zero-copy/deint", "qsv/zero-copy/burn-in+overlay"} {
		assert.True(t, got[want], "%s missing from %v", want, got)
	}
	assert.False(t, got["qsv/passthrough/plain"], "no HDR10 encoder: never resolved")

	// The policy is part of "reachable": GPU tone-mapping off removes the
	// VAAPI tone-map cell - and on this host, whose bridge was never measured,
	// no bridge cell replaces it (an unmeasured tuple is not fielded): HDR
	// simply leaves the GPU.
	pr = twoBackendResolver()
	pr.HWTonemapEnabled = func() bool { return false }
	got = cellSet(ExpectedExpressibilityCells(pr))
	assert.False(t, got["vaapi/gpu-tonemap/plain"])
	assert.False(t, got["vaapi/bridge/plain"] || got["qsv/bridge/plain"], "%v", got)

	assert.Empty(t, ExpectedExpressibilityCells(&PlanResolver{}), "no capability: no GPU cell")
	assert.Nil(t, ExpectedExpressibilityCells(nil))
}
