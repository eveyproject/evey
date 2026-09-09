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
	"time"

	"golang.org/x/sync/singleflight"
)

// subtitleCacheMaxBytes bounds what is stored. A dialogue track is a few KB; an
// ASS track with typesetting can reach a few hundred. A decoded BITMAP track is
// the outlier: ~1000 cues of paletted PNG, 6 to 9 MB measured on real discs, so
// the cap has to clear that or every PGS track would be extracted again on every
// request. Anything past it is not a subtitle any player will render.
const subtitleCacheMaxBytes = 32 << 20

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

// purge removes every stored subtitle. Wired to the admin cache wipe so one
// button clears everything the server has derived from the library.
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

// subtitleExtractionDeadline bounds one extraction of the given source. Never
// below subtitleExtractionTimeout; above it, what the file takes to read at
// the paced rate (subtitle_read_pacer.go) with half again for a busy disk. A
// 70 GB film paced behind a viewer would otherwise be cut off at 30 minutes,
// its full read wasted, and paid again on the next request.
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
