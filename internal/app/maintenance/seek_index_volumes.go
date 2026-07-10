package maintenance

import (
	"path/filepath"
	"sort"
	"syscall"
)

// SeekVolume aggregates the un-indexed repair candidates of one filesystem so
// the UI can advise for or against backups BEFORE a batch starts: a rewrite
// needs the largest candidate free as a temporary copy, and keeping backups
// persists the SUM of the candidates on top of the current usage.
type SeekVolume struct {
	// Root is the shortest directory observed on this filesystem - a human
	// label, not necessarily the mount point.
	Root string
	// FreeBytes is the space available to an unprivileged user right now.
	FreeBytes int64
	// MissingBytes sums the sizes of the un-indexed candidates.
	MissingBytes int64
	// LargestMissingBytes is the biggest single candidate (the floor a
	// rewrite's temporary copy needs).
	LargestMissingBytes int64
	// MissingFiles counts the candidates.
	MissingFiles int
}

// VolumeSummary groups the un-indexed items by the filesystem they live on
// (device id) and reads each filesystem's live free space. Files that
// vanished since the scan are skipped - the summary is advisory, the repair
// itself re-checks everything.
func VolumeSummary(items []SeekStatus) []SeekVolume {
	type agg struct {
		root    string
		missing int64
		largest int64
		files   int
	}
	byDev := map[uint64]*agg{}
	for _, it := range items {
		if it.HasIndex || it.Path == "" {
			continue
		}
		var st syscall.Stat_t
		if syscall.Stat(it.Path, &st) != nil {
			continue
		}
		a := byDev[st.Dev]
		if a == nil {
			a = &agg{root: filepath.Dir(it.Path)}
			byDev[st.Dev] = a
		}
		if d := filepath.Dir(it.Path); len(d) < len(a.root) {
			a.root = d
		}
		a.missing += it.SizeBytes
		if it.SizeBytes > a.largest {
			a.largest = it.SizeBytes
		}
		a.files++
	}
	out := make([]SeekVolume, 0, len(byDev))
	for _, a := range byDev {
		var fs syscall.Statfs_t
		var free int64
		if syscall.Statfs(a.root, &fs) == nil {
			free = int64(fs.Bavail) * fs.Bsize //nolint:gosec // block counts fit int64
		}
		out = append(out, SeekVolume{
			Root:                a.root,
			FreeBytes:           free,
			MissingBytes:        a.missing,
			LargestMissingBytes: a.largest,
			MissingFiles:        a.files,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Root < out[j].Root })
	return out
}
