package loadtest

import (
	"bytes"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"

	pprofprofile "github.com/google/pprof/profile"
)

// heapprofile.go - WHO holds the memory the run ends with.
//
// The report says how big the server's reachable heap was; it cannot say what
// is in it. A heap that climbs while the audience shrinks is retention, not
// load, and the only way to name the culprit is the allocation site of the
// bytes still held. The server exposes no pprof endpoint (and should not), so
// the run takes its own heap profile when it finishes and logs the biggest
// sites: no debug port, no tool, readable through the admin log API.

const moduleMarker = "github.com/eveyproject/evey/"

// HeapSite is one allocation site and the bytes it still holds.
type HeapSite struct {
	Func string // the application function that allocated (or the leaf when none is ours)
	File string
	Line int
	// Via is the innermost function outside the Go runtime: the library that
	// performed the allocation on Func's behalf (a packager, bufio, bytes). It
	// is what tells two very different retentions apart when both enter through
	// the same application function.
	Via string
	// Chain is the path from Via up to Func, innermost first: two allocations
	// entering through the same application function by different library
	// paths are told apart only here.
	Chain string
	Bytes int64
}

// heapInUseTop collects once, then returns the n biggest allocation sites by
// bytes still in use. A site is attributed to the first frame, walking outward
// from the allocation, that belongs to this module: a bufio or bytes frame
// names the library, the Evey function that called it names the culprit.
func heapInUseTop(n int) (sites []HeapSite, total int64) {
	runtime.GC() // the profile reflects the last completed collection: make it fresh
	var buf bytes.Buffer
	if err := pprof.Lookup("heap").WriteTo(&buf, 0); err != nil {
		return nil, 0
	}
	prof, err := pprofprofile.Parse(&buf)
	if err != nil {
		return nil, 0
	}
	return topInUseSites(prof, n)
}

func topInUseSites(prof *pprofprofile.Profile, n int) ([]HeapSite, int64) {
	idx := -1
	for i, st := range prof.SampleType {
		if st.Type == "inuse_space" {
			idx = i
		}
	}
	if idx < 0 {
		return nil, 0
	}
	agg := map[siteKey]int64{}
	var total int64
	for _, s := range prof.Sample {
		v := s.Value[idx]
		if v <= 0 {
			continue
		}
		total += v
		k, ok := siteOf(s)
		if !ok {
			continue
		}
		agg[k] += v
	}
	out := make([]HeapSite, 0, len(agg))
	for k, v := range agg {
		out = append(out, HeapSite{Func: k.fn, File: k.file, Line: k.line, Via: k.via, Chain: k.chain, Bytes: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Bytes != out[j].Bytes {
			return out[i].Bytes > out[j].Bytes
		}
		return out[i].Func < out[j].Func
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out, total
}

type siteKey = struct {
	fn, file string
	line     int
	via      string
	chain    string
}

// chainFrames caps the frames a chain names: enough to cross a library's
// internals, short enough for one log line.
const chainFrames = 8

// siteOf picks the frames a sample is charged to: the innermost frame of this
// module (else the leaf), and the innermost frame outside the Go runtime.
// Location.Line holds inlined frames innermost first.
func siteOf(s *pprofprofile.Sample) (siteKey, bool) {
	var leaf siteKey
	haveLeaf := false
	via := ""
	var chain []string
	for _, loc := range s.Location {
		for _, ln := range loc.Line {
			if ln.Function == nil {
				continue
			}
			k := siteKey{fn: ln.Function.Name, file: ln.Function.Filename, line: int(ln.Line)}
			if !haveLeaf {
				leaf, haveLeaf = k, true
			}
			if via == "" && !runtimeFrame(k.fn) {
				via = k.fn
			}
			if fn, ours := strings.CutPrefix(k.fn, moduleMarker); ours {
				k.fn = fn
				k.file = trimToModule(k.file)
				k.via = strings.TrimPrefix(via, moduleMarker)
				k.chain = strings.Join(chain, " < ")
				return k, true
			}
			if via != "" && len(chain) < chainFrames {
				chain = append(chain, shortFunc(k.fn))
			}
		}
	}
	leaf.via = via
	leaf.chain = strings.Join(chain, " < ")
	return leaf, haveLeaf
}

// runtimeFrame reports whether a function belongs to the Go runtime's own
// allocation machinery rather than to code that asked for memory.
func runtimeFrame(fn string) bool {
	return strings.HasPrefix(fn, "runtime.") || strings.HasPrefix(fn, "internal/")
}

// goroutinesThrough counts the live goroutines whose stack passes through a
// function whose name contains marker, and the total. Requests still being
// served after their viewer left hold their response in memory: a heap that
// is in-flight work rather than a cache shows up here.
func goroutinesThrough(marker string) (matching, total int) {
	var buf bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&buf, 0); err != nil {
		return 0, runtime.NumGoroutine()
	}
	prof, err := pprofprofile.Parse(&buf)
	if err != nil || len(prof.SampleType) == 0 {
		return 0, runtime.NumGoroutine()
	}
	for _, s := range prof.Sample {
		n := int(s.Value[0])
		total += n
		if sampleThrough(s, marker) {
			matching += n
		}
	}
	return matching, total
}

// waitSitesThrough groups the goroutines whose stack passes through marker by
// the innermost frame outside the Go runtime: where each one is parked (a disk
// read, a lock, a channel). Biggest group first, at most n.
func waitSitesThrough(marker string, n int) []WaitSite {
	var buf bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&buf, 0); err != nil {
		return nil
	}
	prof, err := pprofprofile.Parse(&buf)
	if err != nil || len(prof.SampleType) == 0 {
		return nil
	}
	return waitSites(prof, marker, n)
}

// WaitSite is one place goroutines are parked and how many are there.
type WaitSite struct {
	At    string
	Count int
}

func waitSites(prof *pprofprofile.Profile, marker string, n int) []WaitSite {
	counts := map[string]int{}
	for _, s := range prof.Sample {
		if !sampleThrough(s, marker) {
			continue
		}
		var at []string
		for _, loc := range s.Location {
			for _, ln := range loc.Line {
				if ln.Function != nil && !runtimeFrame(ln.Function.Name) && len(at) < 3 {
					at = append(at, shortFunc(ln.Function.Name))
				}
			}
		}
		counts[strings.Join(at, " < ")] += int(s.Value[0])
	}
	out := make([]WaitSite, 0, len(counts))
	for at, c := range counts {
		out = append(out, WaitSite{At: at, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].At < out[j].At
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// shortFunc drops the import path: "github.com/x/mkvgo/mp4.(*Plan).read"
// becomes "mp4.(*Plan).read".
func shortFunc(fn string) string {
	if i := strings.LastIndex(fn, "/"); i >= 0 {
		return fn[i+1:]
	}
	return fn
}

func sampleThrough(s *pprofprofile.Sample, marker string) bool {
	for _, loc := range s.Location {
		for _, ln := range loc.Line {
			if ln.Function != nil && strings.Contains(ln.Function.Name, marker) {
				return true
			}
		}
	}
	return false
}

func trimToModule(file string) string {
	if i := strings.Index(file, "/internal/"); i >= 0 {
		return file[i+1:]
	}
	if i := strings.Index(file, "/cmd/"); i >= 0 {
		return file[i+1:]
	}
	return file
}
