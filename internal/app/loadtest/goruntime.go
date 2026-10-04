package loadtest

import "runtime/metrics"

// goruntime.go - what the Go runtime of the server itself was doing.
//
// The load test samples the machine (cgroup CPU, memory, disks) but, until now,
// not the process it is measuring. GOMEMLIMIT is a SOFT limit: as the live heap
// approaches it the collector runs back to back, which slows the whole server
// and looks exactly like a storage or CPU ceiling from the outside. Whether a
// run's latency was the disk or the garbage collector is a question only these
// four numbers can answer.

// goRuntime is one reading of the runtime's own accounting.
type goRuntime struct {
	heapLiveBytes int64   // heap the last collection found reachable
	memLimitBytes int64   // GOMEMLIMIT, 0 when unset (math.MaxInt64 reads as unset)
	gcCPUSeconds  float64 // cumulative CPU time spent collecting
	totalCPUSec   float64 // cumulative CPU time of every Go-runtime class
	ok            bool
}

var goRuntimeNames = []string{
	"/gc/heap/live:bytes",
	"/gc/gomemlimit:bytes",
	"/cpu/classes/gc/total:cpu-seconds",
	"/cpu/classes/total:cpu-seconds",
}

// readGoRuntime reads the counters. A metric this toolchain does not know reads
// as zero and clears ok: the report then simply omits the figures.
func readGoRuntime() goRuntime {
	samples := make([]metrics.Sample, len(goRuntimeNames))
	for i, n := range goRuntimeNames {
		samples[i].Name = n
	}
	metrics.Read(samples)

	var out goRuntime
	out.ok = true
	for i, s := range samples {
		switch s.Value.Kind() {
		case metrics.KindUint64:
			switch i {
			case 0:
				out.heapLiveBytes = int64(min(s.Value.Uint64(), 1<<62))
			case 1:
				// "no limit" is reported as the largest int64.
				if v := s.Value.Uint64(); v < 1<<62 {
					out.memLimitBytes = int64(v)
				}
			}
		case metrics.KindFloat64:
			switch i {
			case 2:
				out.gcCPUSeconds = s.Value.Float64()
			case 3:
				out.totalCPUSec = s.Value.Float64()
			}
		default:
			out.ok = false
		}
	}
	return out
}

// gcCPUPercent is the share of the Go runtime's CPU time spent collecting
// garbage between two readings.
func gcCPUPercent(prev, cur goRuntime) float64 {
	dTotal := cur.totalCPUSec - prev.totalCPUSec
	dGC := cur.gcCPUSeconds - prev.gcCPUSeconds
	if !prev.ok || !cur.ok || dTotal <= 0 || dGC < 0 {
		return 0
	}
	return min(100, dGC/dTotal*100)
}
