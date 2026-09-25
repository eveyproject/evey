package transcoding

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RemuxCacheJanitor lists a flat-file cache directory for the CacheBudget: the
// fast-start MP4s the /remux.mp4 handler writes under <CacheDir>/remux and the
// compat-download MKVs under <CacheDir>/compat, one whole file per entry. It
// has no sweep of its own: the budget evicts across every kind of entry with
// one clock, and these files count in the same bytes as the segments.
//
// LastAccess is the file mtime, which the remux and compat handlers touch on
// every serve: a remux a player ranges through, or a download a phone keeps
// fetching, stays warm. Source-fingerprint keying means
// every changed source orphans its old file here, which the budget reclaims.
type RemuxCacheJanitor struct {
	dir    string
	leases *FileLeases
}

// WithLeases makes the budget spare every file leases holds, in every pass.
func (j *RemuxCacheJanitor) WithLeases(leases *FileLeases) *RemuxCacheJanitor {
	j.leases = leases
	return j
}

// NewRemuxCacheJanitor lists dir (typically <CacheDir>/remux or /compat).
func NewRemuxCacheJanitor(dir string) *RemuxCacheJanitor {
	return &RemuxCacheJanitor{dir: dir}
}

type remuxFileUsage struct {
	path  string
	bytes int64
	mtime time.Time
}

// listFiles enumerates completed outputs one level deep. In-flight "*.tmp"
// files are skipped so a sweep racing a producer never deletes a half-written
// file out from under it.
func (j *RemuxCacheJanitor) listFiles() ([]remuxFileUsage, error) {
	entries, err := os.ReadDir(j.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("remux listFiles: %w", err)
	}
	var out []remuxFileUsage
	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, remuxFileUsage{
			path:  filepath.Join(j.dir, e.Name()),
			bytes: info.Size(),
			mtime: info.ModTime(),
		})
	}
	return out, nil
}
