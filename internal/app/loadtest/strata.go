package loadtest

import (
	"context"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// strata.go - who the audience is, kept honest at every size.

// eligibilityProbeConcurrency bounds the pre-run eligibility check: a master
// playlist per candidate, a few at a time, so the check never becomes a load
// test of its own.
const eligibilityProbeConcurrency = 6

// scenarioUsesMkvgo reports whether any rank of the scenario plays as an mkvgo
// viewer - the only type whose titles must be checked before the draw.
func scenarioUsesMkvgo(scenario string) bool {
	for rank := 1; rank <= 20; rank++ {
		if scenarioViewerType(scenario, rank) == viewerTypeMkvgo {
			return true
		}
	}
	return false
}

// mkvgoEligible keeps the candidates the server packages through mkvgo for an
// mkvgo viewer, by asking for each one's master playlist exactly as the viewer
// will. It runs before anything is drawn, so a title the server will not
// package never takes a place in a stratum. The plans those requests built,
// and the playback activity they raised, are dropped afterwards: the run must
// start as cold as a real first play.
func (r *Runner) mkvgoEligible(ctx context.Context, cfg Config, candidates []MediaCandidate) []MediaCandidate {
	keep := make([]bool, len(candidates))
	sem := make(chan struct{}, eligibilityProbeConcurrency)
	var wg sync.WaitGroup
	for i, m := range candidates {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			keep[i] = r.servedByMkvgo(ctx, cfg, m)
		}()
	}
	wg.Wait()
	out := make([]MediaCandidate, 0, len(candidates))
	paths := make([]string, 0, len(candidates))
	ids := make([]string, 0, len(candidates))
	for i, m := range candidates {
		ids = append(ids, m.ID)
		if keep[i] {
			out = append(out, m)
			paths = append(paths, m.Path)
		}
	}
	if r.dropPlans != nil {
		r.dropPlans(paths)
	}
	if r.forgetPlayback != nil {
		r.forgetPlayback(ids)
	}
	r.logger.Info("loadtest: mkvgo eligibility checked before the draw", "candidates", len(candidates), "eligible", len(out))
	return out
}

// logStrataShares logs the draw's target split: every stratum's share of the
// eligible titles, which every audience size is held to.
func (r *Runner) logStrataShares(classes []string) {
	counts := map[string]int{}
	for _, c := range classes {
		counts[c]++
	}
	args := []any{"titles", len(classes)}
	for _, k := range sortedKeys(counts) {
		args = append(args, k, counts[k])
	}
	r.logger.Info("loadtest: strata of the draw", args...)
}

// logStrata logs the live audience's split by stratum, and the same split
// summed by disk and by bitrate quarter.
func (r *Runner) logStrata(ramp *rampState, pool *candidatePool) {
	shares := ramp.liveStrata()
	if len(shares) == 0 {
		return
	}
	byDisk, byBand := map[string]int{}, map[string]int{}
	args := []any{"viewers", ramp.tier}
	for _, s := range shares {
		args = append(args, "s:"+s.Stratum, s.Viewers)
		disk, band := splitStratum(s.Stratum)
		byDisk[disk] += s.Viewers
		byBand[band] += s.Viewers
	}
	for _, k := range sortedKeys(byDisk) {
		args = append(args, "disk:"+k, byDisk[k])
	}
	for _, k := range sortedKeys(byBand) {
		args = append(args, "band:"+k, byBand[k])
	}
	r.logger.Info("loadtest: strata", args...)
}

// splitStratum separates "<disk>|q3" into its disk and band; a stratum
// without a disk (one disk, or an encoded viewer's cost class) is all band.
func splitStratum(s string) (disk, band string) {
	if i := strings.LastIndex(s, "|"); i >= 0 {
		return s[:i], s[i+1:]
	}
	return "", s
}

// strataSummary folds a split by stratum into its disk and band totals, each
// sorted by name - what the report and the logs show.
func strataSummary(shares []StratumShare) (byDisk, byBand []StratumShare) {
	d, b := map[string]int{}, map[string]int{}
	for _, s := range shares {
		disk, band := splitStratum(s.Stratum)
		d[disk] += s.Viewers
		b[band] += s.Viewers
	}
	for k, n := range d {
		byDisk = append(byDisk, StratumShare{Stratum: k, Viewers: n})
	}
	for k, n := range b {
		byBand = append(byBand, StratumShare{Stratum: k, Viewers: n})
	}
	sort.Slice(byDisk, func(i, j int) bool { return byDisk[i].Stratum < byDisk[j].Stratum })
	sort.Slice(byBand, func(i, j int) bool { return byBand[i].Stratum < byBand[j].Stratum })
	return byDisk, byBand
}

// loadTestEligibilityProbe is the X-Evey-Load-Test value of an eligibility
// request: the server treats it like any load-test request, and it marks the
// request as the run's pre-draw check rather than a viewer.
const loadTestEligibilityProbe = "eligibility"

// servedByMkvgo asks the server for m's master playlist as an mkvgo viewer
// would and reports whether mkvgo packaged it.
func (r *Runner) servedByMkvgo(ctx context.Context, cfg Config, m MediaCandidate) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildMasterURL(cfg, viewerTypeMkvgo, m, false, 0), nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set(headerLoadTest, loadTestEligibilityProbe)
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK && resp.Header.Get(headerServedBy) == servedByMkvgo
}
