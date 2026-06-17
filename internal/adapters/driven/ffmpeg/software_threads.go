package ffmpeg

import (
	"os"
	"strconv"
	"sync"
)

// softwareTranscodeThreads caps the thread count of each software video
// encoder on the HLS transcode path (`-threads N`), bounded by
// EVEY_TRANSCODING_SOFTWARE_THREADS. Default 0 = unset, which leaves ffmpeg's
// own auto-detection in place (libx264/libx265 grab every core) — the historic
// behaviour, best when transcoding is the box's only job.
//
// The knob exists for the weak-NAS target where one transcode must not starve
// the rest of the server (scanning, metadata, serving the UI): pinning each
// rung's encoder to e.g. 2 threads keeps a predictable CPU envelope at the cost
// of slower encodes. On the software multi-variant path (one shared decode + N
// per-rung encoders) the cap is applied per encoder, so the process budget is
// roughly N_rungs x value + the decode. Hardware encoders ignore -threads, so
// the cap is software-only by construction.
//
// Out-of-range or non-numeric values fall back to 0 (no cap).
var (
	softwareThreadsOnce  sync.Once
	softwareThreadsValue int
)

func softwareTranscodeThreads() int {
	softwareThreadsOnce.Do(func() {
		softwareThreadsValue = parseSoftwareThreads(os.Getenv("EVEY_TRANSCODING_SOFTWARE_THREADS"))
	})
	return softwareThreadsValue
}

// parseSoftwareThreads validates the raw env value: a positive integer in
// [1, 64] enables the cap, anything else (empty, non-numeric, <=0, >64)
// returns 0 = no cap. Pure so it can be unit-tested without touching the
// process environment.
func parseSoftwareThreads(raw string) int {
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 64 {
		return 0
	}
	return n
}

// softwareThreadsArgsN returns the `-threads[spec] N` pair for a software
// encoder, or nil when n<=0 (leave ffmpeg auto). streamSpec is "" for a single
// output ("-threads") or ":v:<i>" to scope the cap to one rung's encoder on the
// multi-variant graph ("-threads:v:0").
func softwareThreadsArgsN(n int, streamSpec string) []string {
	if n <= 0 {
		return nil
	}
	return []string{"-threads" + streamSpec, strconv.Itoa(n)}
}

// softwareThreadsArgs is softwareThreadsArgsN bound to the configured cap.
func softwareThreadsArgs(streamSpec string) []string {
	return softwareThreadsArgsN(softwareTranscodeThreads(), streamSpec)
}
