package mkvgo

import (
	"math"

	"github.com/gravity-zero/mkvgo/mp4"
)

// budget.go - what the on-demand plans may hold at once, process-wide.
//
// A "walk" is the read of one source window (video + audio) that builds a
// segment; while it runs it holds that window, 6-10 MiB in 1080p and ~15 MiB on
// a UHD remux. Before mkvgo 0.42 nothing bounded how many ran together, so an
// audience starting at once on a weak host had its memory grow with the
// audience. The cap below is one walk per CPU - a walk is CPU-bound once the
// bytes are in - held down so that every walk in flight fits in a quarter of
// the heap limit; the rest queue (plan.Stats().SlotWaits counts them).

const (
	// walkWindowBytes is the memory one walk holds in the worst routine case,
	// a UHD remux (mkvgo's integration note: ~15 MiB at 20 Mb/s).
	walkWindowBytes = 15 << 20
	// walkMemoryShare is the divisor: walks in flight may hold at most
	// memLimit / walkMemoryShare together.
	walkMemoryShare = 4
	// tableCacheShare sizes the window tables every plan keeps for streaming
	// (sample positions + segment heads + small audio): memLimit / share,
	// clamped to [tableCacheMin, tableCacheMax].
	tableCacheShare = 16
	tableCacheMin   = 16 << 20
	tableCacheMax   = 128 << 20
)

// ProcessBudget is the result of BudgetFor, applied by ApplyProcessBudget.
type ProcessBudget struct {
	// MaxWalks caps the source windows read at once; always >= 1.
	MaxWalks int
	// TableCacheBytes bounds the window tables all plans keep for Open.
	TableCacheBytes int64
}

// BudgetFor sizes the budget from the CPUs the runtime will use
// (runtime.GOMAXPROCS(0)) and the heap limit in force (debug.SetMemoryLimit(-1);
// math.MaxInt64 means none). cpus <= 0 counts as 1.
func BudgetFor(cpus int, memLimit int64) ProcessBudget {
	if cpus <= 0 {
		cpus = 1
	}
	b := ProcessBudget{MaxWalks: cpus, TableCacheBytes: tableCacheMax}
	if memLimit <= 0 || memLimit == math.MaxInt64 {
		return b
	}
	if byMem := int(memLimit / walkMemoryShare / walkWindowBytes); byMem < b.MaxWalks {
		b.MaxWalks = max(byMem, 1)
	}
	b.TableCacheBytes = min(max(memLimit/tableCacheShare, tableCacheMin), tableCacheMax)
	return b
}

// ApplyProcessBudget sets the budget on mkvgo, for every plan of the process.
func ApplyProcessBudget(b ProcessBudget) {
	mp4.SetMaxConcurrentWalks(b.MaxWalks)
	mp4.SetTableCacheBytes(b.TableCacheBytes)
}
