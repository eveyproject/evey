package cpuutil

import "runtime"

// TrickplayWorkers returns the number of parallel workers for trickplay.
// adaptiveCPU=true -> 25% of cores (gentle), false -> 50% of cores (faster).
// Minimum 1, maximum 4.
func TrickplayWorkers(adaptiveCPU bool) int {
	divisor := 4
	if !adaptiveCPU {
		divisor = 2
	}
	n := runtime.NumCPU() / divisor
	if n < 1 {
		return 1
	}
	if n > 4 {
		return 4
	}
	return n
}
