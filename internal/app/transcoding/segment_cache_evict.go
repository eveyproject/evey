package transcoding

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// EvictionPolicy configures the CacheBudget sweeper that bounds the on-disk
// transcode cache: segment hashes, fast-start remuxes and compat downloads
// together.
type EvictionPolicy struct {
	// MaxBytes is the soft cap on the whole transcode cache on disk, the
	// admin's "transcode cache budget". Above it the sweeper deletes the least
	// recently used entries until the total falls under MaxBytes - MarginBytes.
	// 0 disables the cap; the age and free-space passes still run.
	MaxBytes int64

	// MarginBytes is the headroom below MaxBytes after an eviction pass - gives
	// the next encoder room to land segments before the next sweep hits
	// MaxBytes again. Default = MaxBytes / 10.
	MarginBytes int64

	// MinFreeBytes is the floor of free space on the cache filesystem. Under
	// it the sweeper evicts whatever the cap says: on a small box the cache
	// shares the system disk with everything else, and a full disk breaks
	// playback for everyone. Default 2 GiB; 0 disables the pass.
	MinFreeBytes int64

	// FreeBytes reports the free space of the cache filesystem. nil: the
	// segment cache's own statfs.
	FreeBytes func() (int64, error)

	// DownloadTTL is how long a download file (compat, remux) survives after
	// its last access, whatever the cap. Default 7 days; 0 disables the pass.
	DownloadTTL time.Duration

	// ProtectWindow shields an entry (segment hash or file) accessed within it
	// from the cap pass: a viewer just pulled from it. It replaces the old "N most recent
	// hashes" rule, which kept a finished film's segments above the cap for as
	// long as nothing newer came along. Default 10 min.
	ProtectWindow time.Duration

	// SweepInterval is how often the sweeper wakes up to enforce the policy.
	// Default 30 s. Lower interval = tighter bound + more disk reads.
	SweepInterval time.Duration

	// ActivePlayback reports how many media are being read right now, across
	// every serving path (see ProducerSelector.ActivePlaybackCount). When it is
	// > 0 the sweeper backs off to PlaybackSweepInterval.
	//
	// Backing off rather than pausing outright is deliberate: eviction is the
	// only thing holding the cache under its cap, and a session WRITES segments
	// as it plays, so a sweeper that stopped for a whole film could let the disk
	// fill. Optional; nil keeps the fixed cadence.
	ActivePlayback func() int

	// PlaybackSweepInterval is the cadence used while playback is active.
	// Default 5 min, against 30 s idle.
	//
	// What this avoids is the sweep's FULL WALK of the cache root - one lstat
	// per file - which runs on every tick whatever the fill level, because the
	// total is only known once the walk is done. On a weak NAS whose cache
	// shares a spindle with the media, that is the same disk the viewer is
	// reading from.
	PlaybackSweepInterval time.Duration
}

// DefaultMinFreeBytes is the free-space floor when the policy sets none.
const DefaultMinFreeBytes = 2 << 30

// DefaultDownloadTTL is how long a download file is kept after its last use.
const DefaultDownloadTTL = 7 * 24 * time.Hour

// DefaultProtectWindow is how recently a hash must have been accessed to be
// spared by the cap pass.
const DefaultProtectWindow = 10 * time.Minute

// shouldSweep decides whether this tick pays for a full cache walk.
//
// While playback is active the sweeper drops to PlaybackSweepInterval, because
// the walk lstats every file under the cache root on EVERY tick regardless of
// fill (the total is only known once the walk finishes) - background disk work
// in contention with the viewer's own reads.
//
// The first tick always sweeps (lastSweep zero): a server that boots already
// over its cap must not stay unbounded just because someone is watching.
func (p EvictionPolicy) shouldSweep(now, lastSweep time.Time) bool {
	if lastSweep.IsZero() {
		return true
	}
	if p.ActivePlayback == nil || p.ActivePlayback() <= 0 {
		return true
	}
	return now.Sub(lastSweep) >= p.PlaybackSweepInterval
}

func (p EvictionPolicy) withDefaults() EvictionPolicy {
	out := p
	if out.MaxBytes > 0 && out.MarginBytes <= 0 {
		out.MarginBytes = out.MaxBytes / 10
	}
	if out.MinFreeBytes < 0 {
		out.MinFreeBytes = 0
	} else if out.MinFreeBytes == 0 {
		out.MinFreeBytes = DefaultMinFreeBytes
	}
	if out.DownloadTTL < 0 {
		out.DownloadTTL = 0
	} else if out.DownloadTTL == 0 {
		out.DownloadTTL = DefaultDownloadTTL
	}
	if out.ProtectWindow <= 0 {
		out.ProtectWindow = DefaultProtectWindow
	}
	if out.SweepInterval <= 0 {
		out.SweepInterval = 30 * time.Second
	}
	if out.PlaybackSweepInterval <= 0 {
		out.PlaybackSweepInterval = 5 * time.Minute
	}
	return out
}

// HashUsage is a single decision-hash bucket's disk footprint + last-access
// signal. The sweeper sorts by LastAccess to drive LRU eviction.
type HashUsage struct {
	Hash       string
	Bytes      int64
	LastAccess time.Time
}

// listHashUsage walks the cache root one level deep, totalling each hash
// directory's segment bytes and recording the freshest mtime across its
// files as LastAccess. LRU on directory atime would be cheaper (one stat per
// dir) but Linux mounted with `noatime` (common for media servers) would
// freeze it; mtime is updated on every segment write by ffmpeg, so it's a
// safer proxy for "this hash is still being produced into".
func (c *SegmentCache) listHashUsage() ([]HashUsage, error) {
	entries, err := os.ReadDir(c.baseDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("listHashUsage: %w", err)
	}
	var out []HashUsage
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(c.baseDir, e.Name())
		size, mt, err := dirSizeAndMtime(dir)
		if err != nil {
			continue
		}
		out = append(out, HashUsage{Hash: e.Name(), Bytes: size, LastAccess: mt})
	}
	// Merge in the access-time registry: a hash that is being READ (segments
	// served) but no longer WRITTEN keeps a fresh LastAccess. max(mtime, access)
	// is strictly >= the mtime-only value, so eviction is never MORE aggressive
	// than before - an actively-watched copy title (frozen mtime) stays warm.
	// Done after the directory I/O so the lock is held only for the map merge.
	// defer: this sweep runs under bgloop.Tick, which recovers panics. A plain
	// RUnlock would not be reached and the read lock would stay held, blocking
	// every writer of the cache.
	func() {
		c.mu.RLock()
		defer c.mu.RUnlock()
		for i := range out {
			if at, ok := c.accessTimes[out[i].Hash]; ok && at.After(out[i].LastAccess) {
				out[i].LastAccess = at
			}
		}
	}()
	return out, nil
}

func dirSizeAndMtime(dir string) (int64, time.Time, error) {
	var total int64
	var newest time.Time
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil //nolint:nilerr // vanished-mid-walk entries are skipped by design
		}
		total += info.Size()
		if mt := info.ModTime(); mt.After(newest) {
			newest = mt
		}
		return nil
	})
	return total, newest, err
}

// TotalBytes returns the current cumulative size of every segment under the
// cache root. Cheap diagnostic (walked once per call); not meant for hot paths.
func (c *SegmentCache) TotalBytes() (int64, error) {
	hashes, err := c.listHashUsage()
	if err != nil {
		return 0, err
	}
	var total int64
	for _, h := range hashes {
		total += h.Bytes
	}
	return total, nil
}

// In-flight protection (formerly the reserved "Phase 3" guard) is now live:
// sweepOnce evicts via cleanupHashIfIdle, which skips any hash with a non-zero
// pendingRefs so a mid-rafale session is never evicted even when it falls
// outside ProtectRecent.
