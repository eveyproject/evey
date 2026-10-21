package loadtest

import (
	"context"
	"io"
	"net/http"
	"sort"
	"strings"
)

// runner_stats.go holds pure stat helpers: percentile, key sorting, read-
// amplification ratios and the disk-starvation / endpoint-reachability checks.

// sortedKeys returns m's keys in a deterministic order, so the same run
// always logs its per-type attributes in the same order.
func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// percentile returns the nearest-rank p-th percentile of samples (p in
// [0,100]). Samples is not mutated. An empty input returns 0.
func percentile(samples []int, p int) int {
	n := len(samples)
	if n == 0 {
		return 0
	}
	sorted := make([]int, n)
	copy(sorted, samples)
	sort.Ints(sorted)
	idx := min(max((p*n+99)/100, 1), n)
	return sorted[idx-1]
}

// diskReadStarved reports that the server pushed real traffic while the disk
// counter barely moved. There are two very different reasons for that, and the
// report must not pretend to know which:
//
//   - the operating system served the bytes from its page cache, so the storage
//     never had to work (a library small enough to sit in RAM);
//   - or the media do not live on a block device at all - a network share, or a
//     9p/virtiofs mount under a VM - in which case the kernel's per-cgroup I/O
//     accounting simply cannot see those reads. It reports zero, and zero is NOT
//     the same as "no disk activity".
//
// Either way the viewer count still stands. What must not stand is a confident
// "0.0 MB/s read from disk" next to a gigabit of traffic, which reads as a fact
// and is at best a blind spot.
func diskReadStarved(s ResourceStats) bool {
	// No device could be tied to the library: the counter is not starved, it
	// is absent, and the report says that instead.
	if s.Samples == 0 || s.ServedBytesPerSecAvg <= 0 || !s.DiskReadMeasured {
		return false
	}
	// A tenth of what went out the door is a generous floor: below that, the
	// block layer plainly isn't what's feeding the viewers.
	return s.DiskReadBytesPerSecAvg < s.ServedBytesPerSecAvg/10
}

// endpointReachable probes an origin before pointing hundreds of viewers at it.
//
// The reverse proxy is the honest place to measure - it is what a household
// actually connects through - but from inside the container its public hostname
// may not resolve, or no proxy may be deployed at all. Rather than fail the run
// (or, worse, report a machine that "holds zero viewers" because DNS didn't
// answer), the run checks once and quietly falls back to the loopback.
func (r *Runner) endpointReachable(ctx context.Context, baseURL, token string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, endpointProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/healthz", nil)
	if err != nil {
		return false
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		r.logger.Info("loadtest: reverse proxy not reachable, measuring the server directly", "url", baseURL, "err", err)
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	// Any answer at all proves something is listening and routing to us; the
	// exact status doesn't matter (healthz may be 200, or 404 on an older build).
	return resp.StatusCode < 500
}

// readAmplification is how many bytes the server pulled off the disk for every
// byte it handed to a viewer.
//
// On a direct-play server this is the number that decides everything, and no
// gauge anywhere reports it. The ceiling of such a box IS its read bandwidth:
// if it reads four bytes to serve one, three quarters of that bandwidth is spent
// on data it throws away, and the audience it can hold is a quarter of what the
// hardware could give. Halving the amplification doubles the viewers, on exactly
// the same disks.
//
// Zero when the reads could not be accounted (see diskReadStarved) - a ratio
// computed from a blind counter would be worse than no ratio at all.
func readAmplification(s ResourceStats) float64 {
	if s.Samples == 0 || s.ServedBytesPerSecAvg <= 0 || s.DiskReadBytesPerSecAvg <= 0 || !s.DiskReadMeasured {
		return 0
	}
	return float64(s.DiskReadBytesPerSecAvg) / float64(s.ServedBytesPerSecAvg)
}

// appReadAmplification is how many bytes the server ASKED the filesystem for per
// byte it served. Set against readAmplification (what the disk delivered), it
// separates a greedy packager from a kernel reading ahead - two problems with
// nothing in common but their symptom.
//
// Both sides are the packager's: its reads over what IT served. Dividing by
// every viewer's bytes put ffmpeg viewers' output under reads that do not
// count theirs, so a mixed run read a low app figure beside a high disk one -
// the "kernel readahead" signature - for I/O that was ffmpeg's. The bytes all
// viewers pulled remain the denominator only when nothing counts the
// packager's own.
// appCopyReadAmplification is the packager's segment copies per byte it
// served, on the same denominator as appReadAmplification.
func appCopyReadAmplification(s ResourceStats) float64 {
	served := s.AppServedBytesPerSecAvg
	if served <= 0 {
		served = s.ServedBytesPerSecAvg
	}
	if s.Samples == 0 || served <= 0 || s.AppCopyBytesPerSecAvg <= 0 {
		return 0
	}
	return float64(s.AppCopyBytesPerSecAvg) / float64(served)
}

func appReadAmplification(s ResourceStats) float64 {
	served := s.AppServedBytesPerSecAvg
	if served <= 0 {
		served = s.ServedBytesPerSecAvg
	}
	if s.Samples == 0 || served <= 0 || s.AppReadBytesPerSecAvg <= 0 {
		return 0
	}
	return float64(s.AppReadBytesPerSecAvg) / float64(served)
}
