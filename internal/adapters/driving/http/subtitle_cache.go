package httpapi

// subtitle_cache.go - an extracted subtitle is kept, because producing one
// costs a full read of the film and the result never changes.
//
// Measured on a production server (2026-09-05): 71 seconds to turn a 7 KB
// French track into WebVTT, and 71 seconds again on the very next identical
// request. A viewer therefore never got subtitles at all - no player waits that
// long, and every retry started from zero.
//
// The cost is not a bug in the extractor. On the same file, mkvgo took 89s and
// ffmpeg 79s: a subtitle track is interleaved with the video through the WHOLE
// container, so producing one means reading the film, whatever the tool. That
// is also why Jellyfin is not faster at it - it extracts EARLIER and once, and
// serves the copy afterwards. The only answer is to pay the cost once.
//
// On disk rather than in memory: the value survives a restart, which is when a
// viewer is most likely to press play again, and a subtitle file is a few KB
// where a library's worth of them would not be worth an eviction policy.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/eveyproject/evey/internal/pkg/sysinfo"
)

// subtitleCacheMaxBytes bounds what is stored. A dialogue track is a few KB; an
// ASS track with typesetting can reach a few hundred. A decoded BITMAP track is
// the outlier: ~1000 cues of paletted PNG, 6 to 9 MB measured on real discs, so
// the cap has to clear that or every PGS track would be extracted again on every
// request. Anything past it is not a subtitle any player will render.
const subtitleCacheMaxBytes = 32 << 20

// subtitleCacheWriteFloor is the free space the cache filesystem must keep
// after a store, in bytes. The directory lives under /data - the same disk as
// the keys, the logs, the backups and, on a compose install, Postgres - and
// nothing else bounds it: there is no total cap and no eviction yet (measured
// 2026-10-10: one decoded PGS track is 15-17 MB; a library's worth is 4-12 GB).
// Shared with the transcode cache's MinFreeBytes so both agree on "full"; the
// wiring sets it, and 0 disables the check.
var subtitleCacheWriteFloor atomic.Int64

func init() { subtitleCacheWriteFloor.Store(2 << 30) }

// SetSubtitleCacheWriteFloor sets the free-space floor for the subtitle
// cache. Called by the wiring with the same value the transcode cache uses
// (EVEY_TRANSCODE_CACHE_MIN_FREE_MB); 0 disables the floor.
func SetSubtitleCacheWriteFloor(bytes int64) {
	if bytes < 0 {
		bytes = 0
	}
	subtitleCacheWriteFloor.Store(bytes)
}

// subtitleCacheFreeBytes reports the free space of the filesystem holding dir.
// A variable so tests can stand in for statfs.
var subtitleCacheFreeBytes = func(dir string) (int64, error) {
	stats := sysinfo.GetStorageStats([]string{dir})
	if len(stats) == 0 {
		return 0, fmt.Errorf("statfs %q failed", dir)
	}
	return stats[0].FreeBytes, nil
}

// errSubtitleCacheFull is returned by put when the store would take the
// filesystem under the floor. Callers log it and serve the bytes anyway.
var errSubtitleCacheFull = errors.New("subtitle cache: free space below the floor")

// subtitleCacheFloorHit is told, at most once an hour, that a store was
// refused for lack of space - the wiring turns it into an admin notification
// (transcoding.DerivedCacheFull). nil = nobody listens.
var (
	subtitleCacheFloorHit     func(dir string, free, floor int64)
	subtitleCacheFloorWarnedA atomic.Int64 // unix seconds of the last warning
)

const subtitleCacheFloorWarnEvery = time.Hour

// OnSubtitleCacheFloorHit registers the listener for refused stores.
func OnSubtitleCacheFloorHit(fn func(dir string, free, floor int64)) { subtitleCacheFloorHit = fn }

// warnFloorHit calls the listener unless one was called within the hour: a
// full disk refuses every store, and one page per refused track would be
// hundreds of notifications for one fact.
func warnFloorHit(dir string, free, floor int64) {
	if subtitleCacheFloorHit == nil {
		return
	}
	now := time.Now().Unix()
	last := subtitleCacheFloorWarnedA.Load()
	if now-last < int64(subtitleCacheFloorWarnEvery/time.Second) {
		return
	}
	if subtitleCacheFloorWarnedA.CompareAndSwap(last, now) {
		subtitleCacheFloorHit(dir, free, floor)
	}
}

// subtitleKind separates the two things this cache holds under one (file, track)
// pair: the WebVTT of a text track, and the decoded-image blob of a bitmap one.
// They are different bytes for the same ordinal, so they need different keys.
type subtitleKind string

const (
	subtitleKindVTT subtitleKind = "vtt"
	subtitleKindPGS subtitleKind = "pgs"
)

// subtitleExtractions collapses concurrent requests for the SAME track onto one
// extraction. Without it, a player that retries a request it thinks has hung -
// which is precisely what a 71-second wait provokes - starts a second full read
// of the film, and the two compete for the same disk.
var subtitleExtractions singleflight.Group

// subtitleCache stores extracted WebVTT under the server's data directory.
// A zero value (empty dir) is a working no-op: every lookup misses and every
// store is dropped, so a deployment without a data dir behaves exactly as
// before rather than failing.
type subtitleCache struct{ dir string }

func newSubtitleCache(dataDir string) subtitleCache {
	if dataDir == "" {
		return subtitleCache{}
	}
	return subtitleCache{dir: filepath.Join(dataDir, "subtitles")}
}

// key identifies one extraction: the source file AS IT IS NOW, and which track.
//
// Size and modification time are part of it on purpose. A file replaced in
// place - a re-download, a repaired remux, a re-encode over the same path -
// keeps its name, and serving the old subtitle for the new film would be worse
// than a slow extraction. Changing either invalidates the entry by construction,
// with nothing to invalidate by hand.
func (c subtitleCache) key(filePath string, trackIndex int, size int64, modUnix int64) string {
	h := sha256.New()
	_, _ = h.Write([]byte(filePath))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strconv.FormatInt(size, 10)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strconv.FormatInt(modUnix, 10)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strconv.Itoa(trackIndex)))
	return hex.EncodeToString(h.Sum(nil))
}

// keyFor builds the key by stat-ing the source. An unreadable source is not
// cacheable: return "" and let the caller extract without caching rather than
// key an entry on facts we do not have.
func (c subtitleCache) keyFor(filePath string, trackIndex int) string {
	if c.dir == "" {
		return ""
	}
	st, err := os.Stat(filePath)
	if err != nil {
		return ""
	}
	return c.key(filePath, trackIndex, st.Size(), st.ModTime().Unix())
}

// keyForKind is keyFor with the kind folded in. The VTT kind returns the key
// UNCHANGED on purpose: every entry already on disk was written under it, and
// giving text tracks a new key would throw away a cache whose entries each cost
// a full read of a film to rebuild.
func (c subtitleCache) keyForKind(filePath string, trackIndex int, kind subtitleKind) string {
	k := c.keyFor(filePath, trackIndex)
	if k == "" || kind == subtitleKindVTT {
		return k
	}
	return string(kind) + "-" + k
}

// path names the file. The extension follows the kind so an operator looking in
// the directory can tell a WebVTT from a binary blob without opening it.
func (c subtitleCache) path(key string) string {
	if strings.HasPrefix(key, string(subtitleKindPGS)+"-") {
		return filepath.Join(c.dir, key+".bin")
	}
	return filepath.Join(c.dir, key+".vtt")
}

// get returns the cached WebVTT for key, or false. Any read error is a miss:
// the extraction that follows is slow but correct, which is the right trade for
// a cache.
func (c subtitleCache) get(key string) ([]byte, bool) {
	if c.dir == "" || key == "" {
		return nil, false
	}
	data, err := os.ReadFile(c.path(key)) //nolint:gosec // path is a hex digest under our own data dir
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

// put stores data under key. Best-effort by contract: a full disk or a
// read-only volume must cost a viewer their subtitle, not their playback, so
// every failure is returned for logging and never propagated to the response.
//
// Written to a temp file and renamed, because a half-written cache entry served
// to the next viewer is a truncated subtitle with nothing to say it is one.
func (c subtitleCache) put(key string, data []byte) error {
	if c.dir == "" || key == "" || len(data) == 0 {
		return nil
	}
	if len(data) > subtitleCacheMaxBytes {
		return fmt.Errorf("subtitle cache: %d bytes exceeds the %d cap", len(data), subtitleCacheMaxBytes)
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return fmt.Errorf("subtitle cache: create dir: %w", err)
	}
	// The floor is checked against the space left AFTER this entry: a 17 MB
	// blob on a disk 10 MB above the floor would otherwise be the one that
	// crosses it. A statfs failure does not block the store - the floor is a
	// guard, not a dependency.
	if floor := subtitleCacheWriteFloor.Load(); floor > 0 {
		if free, ferr := subtitleCacheFreeBytes(c.dir); ferr == nil && free-int64(len(data)) < floor {
			warnFloorHit(c.dir, free, floor)
			return fmt.Errorf("%w: %d free, %d needed, floor %d", errSubtitleCacheFull, free, len(data), floor)
		}
	}
	tmp, err := os.CreateTemp(c.dir, key+".*.part")
	if err != nil {
		return fmt.Errorf("subtitle cache: temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName) //nolint:errcheck // best-effort cleanup
		return fmt.Errorf("subtitle cache: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName) //nolint:errcheck // best-effort cleanup
		return fmt.Errorf("subtitle cache: close: %w", err)
	}
	if err := os.Rename(tmpName, c.path(key)); err != nil {
		_ = os.Remove(tmpName) //nolint:errcheck // best-effort cleanup
		return fmt.Errorf("subtitle cache: publish: %w", err)
	}
	return nil
}

// failureKey names the memo of a failed extraction. The mkvgo version is part
// of it on purpose: a track that one release cannot decode (2026-10-10: eleven
// PGS tracks of the library, zlib-compressed without the ContentEncodings that
// declares it) may decode under the next, so an upgrade must replay the
// failure rather than inherit it. Without the memo every request for such a
// track paid the full walk again before failing again.
func (c subtitleCache) failureKey(key string) string {
	return "fail-" + packagerVersion() + "-" + key
}

func (c subtitleCache) failurePath(key string) string {
	return filepath.Join(c.dir, c.failureKey(key)+".fail")
}

// getFailure returns the recorded reason an extraction failed under THIS mkvgo
// version, or false. The reason is what the viewer-facing error names.
func (c subtitleCache) getFailure(key string) (string, bool) {
	if c.dir == "" || key == "" {
		return "", false
	}
	data, err := os.ReadFile(c.failurePath(key)) //nolint:gosec // path is a hex digest under our own data dir
	if err != nil {
		return "", false
	}
	return string(data), true
}

// putFailure memorises a failed extraction. A cancelled or timed-out
// extraction is not a failure of the track and is never recorded. Best-effort
// like put: a lost memo costs a walk, not a playback.
func (c subtitleCache) putFailure(key string, cause error) error {
	if c.dir == "" || key == "" || cause == nil {
		return nil
	}
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return nil
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return fmt.Errorf("subtitle cache: create dir: %w", err)
	}
	msg := cause.Error()
	if len(msg) > 4096 {
		msg = msg[:4096]
	}
	if err := os.WriteFile(c.failurePath(key), []byte(msg), 0o644); err != nil { //nolint:gosec // our own data dir
		return fmt.Errorf("subtitle cache: write failure memo: %w", err)
	}
	return nil
}

// SubtitleCachePurger returns the purge of the subtitle store under dataDir,
// for the admin cache wipe (transcoding.Service.ClearCache). Extracted tracks,
// decoded bitmap blobs and failure memos go; the block indexes next door stay,
// because they cost a pass of the film each and never go stale on their own
// (keyed on size and mtime, verified block by block by mkvgo).
func SubtitleCachePurger(dataDir string) func(context.Context) (int, error) {
	c := newSubtitleCache(dataDir)
	return func(context.Context) (int, error) { return c.purge() }
}

// purge removes every stored subtitle, blob and failure memo. Reached from
// the admin cache wipe through SubtitleCachePurger - it was not, for a while:
// this comment promised the wiring and nothing called it.
func (c subtitleCache) purge() (int, error) {
	if c.dir == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("subtitle cache: read dir: %w", err)
	}
	removed := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := os.Remove(filepath.Join(c.dir, e.Name())); err == nil {
			removed++
		}
	}
	return removed, nil
}

// subtitleFlightKey is the singleflight key for one track's extraction. THE ONE
// PLACE it is built, because the whole point of the flight is that the on-demand
// handler and the prewarmer land on the SAME key: they both cost a full read of
// the container, and two of those at once on a network mount is the worst thing
// this server can do to itself.
//
// They diverged once. When the PGS path was added it spelled its key
// `cacheKey + "|pgs|" + filePath` while the prewarmer kept `cacheKey + "|" +
// filePath` - so a viewer asking for a bitmap track the prewarmer was already
// extracting started a SECOND full pass instead of waiting, and the prewarmer's
// comment promising otherwise was simply false. The cache key already carries
// the kind, so the extra marker bought nothing and cost that.
func subtitleFlightKey(cacheKey, filePath string) string {
	return cacheKey + "|" + filePath
}

// subtitleExtractionTimeout is the floor of one shared extraction's deadline.
// Generously sized: the work is a full sequential read of the container, and a
// 4K remux on a throttled network mount has been measured past 15 minutes.
const subtitleExtractionTimeout = 30 * time.Minute

// subtitleExtractionDeadline: at least the floor, else the paced read time of
// the file plus margin - a 70 GB film paced behind a viewer outlives 30 min.
func subtitleExtractionDeadline(filePath string) time.Duration {
	st, err := os.Stat(filePath)
	if err != nil || st.Size() <= 0 {
		return subtitleExtractionTimeout
	}
	paced := time.Duration(float64(st.Size())/float64(subtitleReadRateWhilePlaying)*1.5) * time.Second
	return max(subtitleExtractionTimeout, paced+5*time.Minute)
}

// runSharedSubtitleExtraction collapses concurrent requests for the same track
// onto ONE extraction, and - the part singleflight.Do alone gets wrong - runs
// that extraction on a context nobody's request can cancel.
//
// With Do(), the shared work inherits whichever caller happened to win the
// flight. When THAT caller goes away - a player retrying what it thinks is a
// hung request, an overlay remounting, a viewer closing the tab - its context is
// cancelled and every waiter fails with "context canceled", including the ones
// still perfectly connected. Prod, 2026-09-09: five requests for one PGS track
// all failed in the same millisecond that way, and since the extraction costs a
// full read of the film, nothing was cached either - so the next attempt paid
// the whole price again.
//
// The work therefore runs detached with its own deadline (it fills a cache that
// outlives every request), while each caller still gives up on its OWN context.
func runSharedSubtitleExtraction(ctx context.Context, key, filePath string, work func(context.Context) ([]byte, error)) ([]byte, error) {
	ch := subtitleExtractions.DoChan(key, func() (any, error) {
		workCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), subtitleExtractionDeadline(filePath))
		defer cancel()
		return work(workCtx)
	})
	select {
	case <-ctx.Done():
		// This caller is gone; the extraction keeps running and caches its
		// result for whoever asks next.
		return nil, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		data, _ := res.Val.([]byte)
		return data, nil
	}
}
