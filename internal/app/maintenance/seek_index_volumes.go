package maintenance

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/eveyproject/evey/internal/domain/mkv"
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

// summarizeVolumes is swappable so service tests can simulate a full volume
// without materializing terabytes on the test filesystem.
var summarizeVolumes = VolumeSummary

// volumeHeadroomBytes mirrors the rewrite's per-file space-preflight margin
// (rewriteHeadroomBytes in the mkvgo adapter): the batch guard should predict
// what the repairs themselves will accept.
const volumeHeadroomBytes = 256 << 20 // 256 MiB

// backupShortfalls returns the volumes that cannot hold the SUM of their
// repair candidates. A volume whose free space could not be probed reports
// FreeBytes 0 and is skipped: the guard is advisory, the per-file preflight
// inside the rewrite keeps the authority.
func backupShortfalls(vols []SeekVolume) []SeekVolume {
	var short []SeekVolume
	for _, v := range vols {
		if v.MissingFiles == 0 || v.FreeBytes == 0 {
			continue
		}
		if v.MissingBytes+volumeHeadroomBytes > v.FreeBytes {
			short = append(short, v)
		}
	}
	return short
}

// BackupSpaceError reports the volumes that cannot hold the .mkvgobak
// accumulation of a backup-keeping batch. It wraps mkv.ErrInsufficientSpace
// (for errors.Is routing) and keeps the per-volume numbers so the API layer
// can build a translated, parameterized refusal instead of a raw English
// string.
type BackupSpaceError struct {
	Volumes []SeekVolume
}

func (e *BackupSpaceError) Unwrap() error { return mkv.ErrInsufficientSpace }

// Detail renders the per-volume shortfall numbers. Locale-neutral on purpose
// (paths, byte sizes, counts) so translations can embed it verbatim.
func (e *BackupSpaceError) Detail() string {
	parts := make([]string, 0, len(e.Volumes))
	for _, v := range e.Volumes {
		parts = append(parts, fmt.Sprintf("%s: %d × ~%s > %s",
			v.Root, v.MissingFiles, fmtBytes(v.MissingBytes+volumeHeadroomBytes), fmtBytes(v.FreeBytes)))
	}
	return strings.Join(parts, "; ")
}

// Error keeps the raw-English form for logs and the async job summary (the
// job runner has no request locale); user-facing surfaces go through the
// API layer's translation of the same data.
func (e *BackupSpaceError) Error() string {
	return fmt.Sprintf("reindex library: %v: keeping one backup per repaired file would overfill the volume (%s) - free up space or repair without backups",
		mkv.ErrInsufficientSpace, e.Detail())
}

// backupSpacePreflight refuses a backup-keeping batch upfront when any volume
// cannot hold the sum of its candidates: keeping one .mkvgobak per repaired
// file persists that sum on top of the current usage, and the per-file guard
// cannot see the accumulation coming (it reads the LIVE free space, which
// only shrinks as the batch progresses) - so without this check an
// over-committed batch burns hours before its tail starts failing.
func (s *SeekIndexService) backupSpacePreflight(items []SeekStatus) error {
	short := backupShortfalls(summarizeVolumes(items))
	if len(short) == 0 {
		return nil
	}
	return &BackupSpaceError{Volumes: short}
}

// fmtBytes renders a byte count for operator-facing errors. Mirrors the mkvgo
// adapter's formatter (the app layer cannot import the adapter).
func fmtBytes(n int64) string {
	const unit = int64(1024)
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := unit, 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
