// Package procfork starts the media tools evey forks (ffmpeg, ffprobe, fpcalc)
// under one policy: a decode thread count sized on CPU and RAM, a hard memory
// cap, admission against the free memory, and one log line per exit. Four
// ffmpeg processes OOM-killed the host on 2026-09-01 (7 GiB RSS each, thread
// count = nproc, no cap anywhere); this is the structural answer. Scan forks
// additionally reserve their threads in the admin's background CPU budget
// (SetCPUBudget) before they start.
package procfork

import "fmt"

const (
	mib = int64(1) << 20
	gib = int64(1) << 30

	// assumedBudgetBytes stands in when neither cgroup nor /proc/meminfo is readable.
	assumedBudgetBytes = 4 * gib

	// addressSpaceSlackBytes is added to the RSS cap to form RLIMIT_AS. Measured
	// on ffmpeg 7.1: ~600 MiB of mapped libraries plus ~80 MiB of glibc arena
	// and stack per thread, 0.7-1.5 GiB in the 2026-08-24 OOM forensics.
	addressSpaceSlackBytes = 2 * gib

	// Decoder memory model, measured on HEVC 10-bit (1080p: 80 MiB at one
	// thread, +11 MiB per extra thread, i.e. under two frames per thread; four
	// leaves a x2 margin). The DPB is shared across threads.
	decoderBaseBytes = 80 * mib
	dpbFrames        = 8
	framesPerThread  = 4
)

// Class is what the process is for; it selects priority, caps and admission.
type Class int

const (
	// Probe reads headers or capabilities: logged, otherwise untouched.
	Probe Class = iota + 1
	// Scan is a background pass over the library: lowest priority, one thread.
	Scan
	// Interactive is a short job a user is waiting on (snapshot, preview clip).
	Interactive
	// Production is a long encode a user is waiting on (downloads, legacy transcode).
	Production
)

func (c Class) String() string {
	switch c {
	case Probe:
		return "probe"
	case Scan:
		return "scan"
	case Interactive:
		return "interactive"
	case Production:
		return "production"
	}
	return fmt.Sprintf("class(%d)", int(c))
}

// Source describes the picture being decoded, when the caller knows it. The
// zero value is sized as 4K 10-bit, the worst case a library holds.
type Source struct {
	Width, Height int
	TenBit        bool
}

func (s Source) frameBytes() int64 {
	w, h, tenBit := s.Width, s.Height, s.TenBit
	if w <= 0 || h <= 0 {
		w, h, tenBit = 3840, 2160, true
	}
	bytes := int64(w) * int64(h) * 3 / 2
	if tenBit {
		bytes *= 2
	}
	return bytes
}

// RSSCap is the memory one process of the class may claim, from the budget
// (cgroup limit, else host RAM). Probe has no cap.
func RSSCap(c Class, budgetBytes int64) int64 {
	if budgetBytes <= 0 {
		budgetBytes = assumedBudgetBytes
	}
	switch c {
	case Scan:
		return clamp64(budgetBytes/8, 512*mib, gib)
	case Interactive:
		return clamp64(budgetBytes/4, 512*mib, gib)
	case Production:
		return clamp64(budgetBytes/4, 3*gib/2, 4*gib)
	}
	return 0
}

// reserveBytes is what must stay free after every in-flight process reached
// its cap: the server, Meilisearch and the page cache need it.
func reserveBytes(budgetBytes int64) int64 {
	if budgetBytes <= 0 {
		budgetBytes = assumedBudgetBytes
	}
	return clamp64(budgetBytes/8, 512*mib, gib)
}

func addressSpaceCap(rssCap int64) int64 {
	return rssCap + addressSpaceSlackBytes
}

// Threads is the decoder thread count for the class: the CPU share the class
// is allowed, then no more than the RSS cap can hold for this source.
func Threads(c Class, src Source, cpuBudget int, rssCap int64) int {
	cpuCap := 1
	switch c {
	case Interactive:
		cpuCap = clampInt(cpuBudget/2, 1, 4)
	case Production:
		cpuCap = clampInt(cpuBudget/2, 1, 8)
	}
	if rssCap <= 0 {
		return cpuCap
	}
	frame := src.frameBytes()
	ramCap := (rssCap - decoderBaseBytes - dpbFrames*frame) / (framesPerThread * frame)
	return clampInt(int(ramCap), 1, cpuCap)
}

func clamp64(v, lo, hi int64) int64 {
	return min(max(v, lo), hi)
}

func clampInt(v, lo, hi int) int {
	return min(max(v, lo), hi)
}
