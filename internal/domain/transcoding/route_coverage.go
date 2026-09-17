package transcoding

import (
	"sort"
	"strings"
	"sync"
)

// route_coverage.go - TEMPORARY instrumentation for taking RouteExpressibility
// out of shadow (ADR-0030, 6). Expressibility is deterministic, so the
// question is not "how long has it run" but "has every route cell this host
// really serves been checked at least once, by both checkers, without a
// disagreement". This table answers it; it goes away with the shadow.
//
// A CELL is {backend, topology, needs}: the codec plays no part (the builder's
// gates never read it), the height neither. A SITE is the checker: the plan
// resolver (asks the port before resolving) or the ffmpeg builder (its own
// hwUsable at build time). A cell is COVERED when both sites saw it.

// ExpressibilitySite names the checker that recorded a cell.
type ExpressibilitySite string

const (
	ExpressibilitySiteResolver ExpressibilitySite = "resolver"
	ExpressibilitySiteBuilder  ExpressibilitySite = "builder"
)

// Route needs, as recorded on a cell (RouteNeedsLabel).
const (
	NeedsPlain          = "plain"
	NeedsBurnIn         = "burn-in"         // bitmap subtitle composited on the CPU (no GPU route expresses it)
	NeedsBurnInOverlay  = "burn-in+overlay" // bitmap subtitle composited on the GPU
	NeedsDeinterlace    = "deint"           // the session-level GPU deinterlacer (QSV)
	needsLabelSeparator = ","
)

// RouteNeedsLabel names what a session composites beyond its topology.
func RouteNeedsLabel(burnIn, gpuOverlay, deinterlace bool) string {
	var parts []string
	switch {
	case burnIn && gpuOverlay:
		parts = append(parts, NeedsBurnInOverlay)
	case burnIn:
		parts = append(parts, NeedsBurnIn)
	}
	if deinterlace {
		parts = append(parts, NeedsDeinterlace)
	}
	if len(parts) == 0 {
		return NeedsPlain
	}
	return strings.Join(parts, needsLabelSeparator)
}

// Name is the short label of a topology for metrics and reports.
func (t Topology) Name() string {
	switch t {
	case TopologySoftware:
		return "software"
	case TopologySoftwareHDR:
		return "software-hdr"
	case TopologyZeroCopy:
		return "zero-copy"
	case TopologyGPUToneMap:
		return "gpu-tonemap"
	case TopologyBridge:
		return "bridge"
	case TopologyDecoupled:
		return "decoupled"
	case TopologyHDRPassthrough:
		return "passthrough"
	}
	return t.String()
}

// ExpressibilityCell is one {backend, topology, needs} the checkers can meet.
type ExpressibilityCell struct {
	Backend  string `json:"backend"`
	Topology string `json:"topology"`
	Needs    string `json:"needs"`
}

// ExpressibilityCellCounts is what the table knows about one cell.
type ExpressibilityCellCounts struct {
	ExpressibilityCell
	ResolverChecks        int `json:"resolver_checks"`
	BuilderChecks         int `json:"builder_checks"`
	ResolverDisagreements int `json:"resolver_disagreements"`
	BuilderDisagreements  int `json:"builder_disagreements"`
}

// Covered reports whether both checkers saw the cell at least once.
func (c ExpressibilityCellCounts) Covered() bool { return c.ResolverChecks > 0 && c.BuilderChecks > 0 }

// ExpressibilityReport is the exit criterion of the shadow, in one read:
// every expected cell covered by both sites and zero disagreements anywhere.
type ExpressibilityReport struct {
	// Expected are the cells this host can really reach (a conservative
	// enumeration, never a cartesian product - see the planner).
	Expected []ExpressibilityCell `json:"expected"`
	// Missing are the expected cells at least one site has not seen.
	Missing []ExpressibilityCell `json:"missing"`
	// Cells are every cell recorded so far, expected or not, with its counts.
	Cells []ExpressibilityCellCounts `json:"cells"`
	// Disagreements is the total over every cell and site.
	Disagreements int `json:"disagreements"`
	// ReadyToLeaveShadow: Missing is empty and Disagreements is zero.
	ReadyToLeaveShadow bool `json:"ready_to_leave_shadow"`
}

// ExpressibilityCoverage is the in-memory table both checkers record into.
// Process-wide (the builder is a pure function with no injection point) and
// deliberately not persisted: a restart clears it, and a recalibration plus
// one corpus run refill it in minutes.
type ExpressibilityCoverage struct {
	mu     sync.Mutex
	counts map[ExpressibilityCell]*ExpressibilityCellCounts
}

// ExpressibilityCoverageTable is the process-wide table.
var ExpressibilityCoverageTable = &ExpressibilityCoverage{}

// Record notes one check of cell by site, and whether it disagreed.
func (c *ExpressibilityCoverage) Record(site ExpressibilitySite, cell ExpressibilityCell, disagreed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counts == nil {
		c.counts = map[ExpressibilityCell]*ExpressibilityCellCounts{}
	}
	cc, ok := c.counts[cell]
	if !ok {
		cc = &ExpressibilityCellCounts{ExpressibilityCell: cell}
		c.counts[cell] = cc
	}
	switch site {
	case ExpressibilitySiteResolver:
		cc.ResolverChecks++
		if disagreed {
			cc.ResolverDisagreements++
		}
	case ExpressibilitySiteBuilder:
		cc.BuilderChecks++
		if disagreed {
			cc.BuilderDisagreements++
		}
	}
}

// Reset empties the table (tests, or a deliberate restart of the window).
func (c *ExpressibilityCoverage) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts = nil
}

// Report compares what was recorded with the expected cells.
func (c *ExpressibilityCoverage) Report(expected []ExpressibilityCell) ExpressibilityReport {
	c.mu.Lock()
	defer c.mu.Unlock()
	rep := ExpressibilityReport{Expected: expected, Missing: []ExpressibilityCell{}, Cells: []ExpressibilityCellCounts{}}
	for _, cell := range expected {
		if cc, ok := c.counts[cell]; !ok || !cc.Covered() {
			rep.Missing = append(rep.Missing, cell)
		}
	}
	for _, cc := range c.counts {
		rep.Cells = append(rep.Cells, *cc)
		rep.Disagreements += cc.ResolverDisagreements + cc.BuilderDisagreements
	}
	sort.Slice(rep.Cells, func(i, j int) bool { return cellLess(rep.Cells[i].ExpressibilityCell, rep.Cells[j].ExpressibilityCell) })
	sort.Slice(rep.Missing, func(i, j int) bool { return cellLess(rep.Missing[i], rep.Missing[j]) })
	rep.ReadyToLeaveShadow = len(rep.Missing) == 0 && rep.Disagreements == 0 && len(expected) > 0
	return rep
}

func cellLess(a, b ExpressibilityCell) bool {
	if a.Backend != b.Backend {
		return a.Backend < b.Backend
	}
	if a.Topology != b.Topology {
		return a.Topology < b.Topology
	}
	return a.Needs < b.Needs
}

// ExpressibilityPlanner enumerates the cells a host can really reach. It is
// the builder's knowledge (which filters exist per backend), so the ffmpeg
// adapter implements it next to RouteExpressibility.
type ExpressibilityPlanner interface {
	ExpectedCells(cap EncodeCapability) []ExpressibilityCell
}
