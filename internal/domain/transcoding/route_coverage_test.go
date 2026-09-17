package transcoding_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// A cell is covered only when BOTH checkers saw it; the report is the go/no-go
// of the shadow: no missing cell, no disagreement, at least one expected cell.
func TestExpressibilityCoverage_ReportIsTheExitCriterion(t *testing.T) {
	c := &transcoding.ExpressibilityCoverage{}
	zero := transcoding.ExpressibilityCell{Backend: "vaapi", Topology: "zero-copy", Needs: transcoding.NeedsPlain}
	bridge := transcoding.ExpressibilityCell{Backend: "vaapi", Topology: "bridge", Needs: transcoding.NeedsPlain}
	extra := transcoding.ExpressibilityCell{Backend: "vaapi", Topology: "decoupled", Needs: transcoding.NeedsPlain}
	expected := []transcoding.ExpressibilityCell{zero, bridge}

	rep := c.Report(expected)
	assert.Len(t, rep.Missing, 2, "nothing recorded: everything is missing")
	assert.False(t, rep.ReadyToLeaveShadow)

	c.Record(transcoding.ExpressibilitySiteBuilder, zero, false) // the calibration saw it at the builder...
	rep = c.Report(expected)
	assert.Contains(t, rep.Missing, zero, "...but the resolver has not: still missing")

	c.Record(transcoding.ExpressibilitySiteResolver, zero, false) // ...a real request saw it at the resolver
	c.Record(transcoding.ExpressibilitySiteResolver, bridge, false)
	c.Record(transcoding.ExpressibilitySiteBuilder, bridge, false)
	c.Record(transcoding.ExpressibilitySiteBuilder, extra, false) // an unexpected cell is reported, never required
	rep = c.Report(expected)
	assert.Empty(t, rep.Missing)
	assert.Equal(t, 0, rep.Disagreements)
	assert.True(t, rep.ReadyToLeaveShadow)
	assert.Len(t, rep.Cells, 3)

	c.Record(transcoding.ExpressibilitySiteBuilder, bridge, true) // one disagreement anywhere: no-go
	rep = c.Report(expected)
	assert.Equal(t, 1, rep.Disagreements)
	assert.False(t, rep.ReadyToLeaveShadow)

	assert.False(t, c.Report(nil).ReadyToLeaveShadow, "no expected cell = nothing proven")
}

func TestRouteNeedsLabel(t *testing.T) {
	assert.Equal(t, "plain", transcoding.RouteNeedsLabel(false, false, false))
	assert.Equal(t, "burn-in", transcoding.RouteNeedsLabel(true, false, false))
	assert.Equal(t, "burn-in+overlay", transcoding.RouteNeedsLabel(true, true, false))
	assert.Equal(t, "deint", transcoding.RouteNeedsLabel(false, false, true))
	assert.Equal(t, "burn-in+overlay,deint", transcoding.RouteNeedsLabel(true, true, true))
	assert.Equal(t, "plain", transcoding.RouteNeedsLabel(false, true, false), "an overlay with nothing to burn is not a need")
}
