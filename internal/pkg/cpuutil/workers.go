package cpuutil

import "github.com/evey-project/evey/internal/pkg/sysinfo"

// TrickplayWorkers returns the number of parallel workers for trickplay,
// sized off sysinfo.CPUBudget() (the cgroup CPU quota when one is set, else
// runtime.NumCPU()) so a container with a CPU limit doesn't oversubscribe.
// adaptiveCPU=true -> 25% of the budget (gentle), false -> 50% (faster).
// Minimum 1, maximum 4.
func TrickplayWorkers(adaptiveCPU bool) int {
	divisor := 4
	if !adaptiveCPU {
		divisor = 2
	}
	n := sysinfo.CPUBudget() / divisor
	if n < 1 {
		return 1
	}
	if n > 4 {
		return 4
	}
	return n
}
