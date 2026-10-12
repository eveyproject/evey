package transcoding

import "github.com/eveyproject/evey/internal/pkg/sysinfo"

// cpuSigForTest is a thin wrapper that tests use to grab the host CPU
// signature without importing sysinfo directly (keeps test imports
// minimal and gives us a single point to mock if needed later).
func cpuSigForTest() string {
	return sysinfo.CPUSignature()
}
