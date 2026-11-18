package plugin

import (
	"slices"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/pkg/cache"
)

// The static analysis reads every file of a plugin, its bundled dependencies
// included, and an install is always preceded by a judgment of the same zip:
// the admin's preview, or the automatic update's own check. The report is
// kept for a short while under the zip's SHA-256, so the install reuses the
// verdict reached on those exact bytes instead of reading them a second time.
// In memory only: a restart, and with it any change of the rules, forgets it.
const (
	analysisMemoTTL  = 15 * time.Minute
	analysisMemoSize = 16
)

type analysisMemo = cache.TTLCache[*plugin.AnalysisReport]

func newAnalysisMemo() *analysisMemo {
	c, _ := cache.NewWithMaxSize[*plugin.AnalysisReport](0, analysisMemoSize)
	return c
}

func analysisMemoKey(sourceHash, webRoot string) string {
	return sourceHash + "\x00" + webRoot
}

func cloneAnalysisReport(r *plugin.AnalysisReport) *plugin.AnalysisReport {
	c := *r
	c.Issues = slices.Clone(r.Issues)
	c.Categories = slices.Clone(r.Categories)
	c.RawFindings = slices.Clone(r.RawFindings)
	c.URLs = slices.Clone(r.URLs)
	return &c
}

// rememberAnalysis keeps the report reached on the zip with this hash.
func (s *InstallerService) rememberAnalysis(sourceHash, webRoot string, r *plugin.AnalysisReport) {
	if s.analyses == nil || sourceHash == "" || r == nil {
		return
	}
	s.analyses.Set(analysisMemoKey(sourceHash, webRoot), cloneAnalysisReport(r), analysisMemoTTL)
}

// recalledAnalysis returns a copy of the report kept for this zip, nil when
// it was never judged, or too long ago.
func (s *InstallerService) recalledAnalysis(sourceHash, webRoot string) *plugin.AnalysisReport {
	if s.analyses == nil || sourceHash == "" {
		return nil
	}
	r, ok := s.analyses.Get(analysisMemoKey(sourceHash, webRoot))
	if !ok {
		return nil
	}
	return cloneAnalysisReport(r)
}
