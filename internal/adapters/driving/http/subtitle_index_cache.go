package httpapi

// subtitle_index_cache.go - the block index that turns a subtitle extraction
// from a full read of the film into a handful of seeks.
//
// Producing one subtitle track means walking the whole container, because the
// track's blocks are interleaved with the video from end to end. That is why an
// extraction costs the same as reading the film (measured: 71 s on prod for
// 7 KB of text). An MP4 does not have this problem - its sample table already
// says where every sample is, which is why the MP4 path answers in 19 ms. The
// index is that table, built for Matroska.
//
// One walk builds it for every text track at once, and it is then worth ~46 KB
// on disk. After that, serving a track is 19 ms for a forced subtitle and
// 310 ms for a full dialogue one (mkvgo measurements on SSD, 15.8 GB source).
//
// Stored beside the extracted WebVTT and keyed the same way, because it is
// exactly as stale-able: an index built for one file must never be applied to
// another. mkvgo carries its own guard as well (source size, Segment UID and
// timecode scale, checked before a single cue is read, then per block), so a
// key that slips through cannot make it emit cues read from the wrong bytes.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gravity-zero/mkvgo/matroska"
)

// subtitleIndexes is the process-wide store, set once at wiring time next to
// the WebVTT cache. A package-level value because extractMkvgoSubtitleWebVTT is
// reached from both the route and the prewarmer, and threading it through every
// call would only move the same global one layer down.
var subtitleIndexes subtitleIndexCache

// subtitleIndexCache stores serialized SubtitleIndex blobs. Same zero-value
// contract as subtitleCache: an empty dir is a working no-op.
type subtitleIndexCache struct{ dir string }

func newSubtitleIndexCache(dataDir string) subtitleIndexCache {
	if dataDir == "" {
		return subtitleIndexCache{}
	}
	return subtitleIndexCache{dir: filepath.Join(dataDir, "subtitle-index")}
}

// key identifies the index of one FILE - not of one track, unlike the WebVTT
// cache: a single index covers every text track, which is the whole point of
// paying for one walk. -1 stands in for "the file, all tracks" in the shared
// key shape.
func (c subtitleIndexCache) key(filePath string) string {
	return subtitleCache(c).keyFor(filePath, -1)
}

func (c subtitleIndexCache) path(key string) string {
	return filepath.Join(c.dir, key+".idx")
}

// get loads the index for a file, or false. A blob that fails to decode is a
// miss: the caller walks the file, which is slow but correct.
func (c subtitleIndexCache) get(filePath string) (*matroska.SubtitleIndex, bool) {
	if c.dir == "" {
		return nil, false
	}
	key := c.key(filePath)
	if key == "" {
		return nil, false
	}
	blob, err := os.ReadFile(c.path(key)) //nolint:gosec // path is a hex digest under our own data dir
	if err != nil || len(blob) == 0 {
		return nil, false
	}
	var ix matroska.SubtitleIndex
	if err := ix.UnmarshalBinary(blob); err != nil {
		return nil, false
	}
	return &ix, true
}

// put stores the index. Best-effort, like the WebVTT cache: failing to keep it
// costs the next viewer a walk, never their playback.
func (c subtitleIndexCache) put(filePath string, ix *matroska.SubtitleIndex) error {
	if c.dir == "" || ix == nil {
		return nil
	}
	key := c.key(filePath)
	if key == "" {
		return nil
	}
	blob, err := ix.MarshalBinary()
	if err != nil {
		return fmt.Errorf("subtitle index: marshal: %w", err)
	}
	if mkErr := os.MkdirAll(c.dir, 0o755); mkErr != nil {
		return fmt.Errorf("subtitle index: create dir: %w", mkErr)
	}
	tmp, err := os.CreateTemp(c.dir, key+".*.part")
	if err != nil {
		return fmt.Errorf("subtitle index: temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(blob); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName) //nolint:errcheck // best-effort cleanup
		return fmt.Errorf("subtitle index: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName) //nolint:errcheck // best-effort cleanup
		return fmt.Errorf("subtitle index: close: %w", err)
	}
	if err := os.Rename(tmpName, c.path(key)); err != nil {
		_ = os.Remove(tmpName) //nolint:errcheck // best-effort cleanup
		return fmt.Errorf("subtitle index: publish: %w", err)
	}
	return nil
}

// buildAndStore walks the file once and keeps the index for the given text
// tracks. Returns the index so the caller can use it immediately - the walk it
// just paid for is the expensive part, not the object.
func (c subtitleIndexCache) buildAndStore(ctx context.Context, filePath string, trackIDs []uint64) (*matroska.SubtitleIndex, error) {
	if len(trackIDs) == 0 {
		return nil, nil //nolint:nilnil // "nothing to index" is not a failure
	}
	// Paced while a stream is being served - see subtitle_read_pacer.go.
	ix, err := matroska.BuildSubtitleIndex(ctx, filePath, trackIDs, subtitleReadOptions(ctx)...)
	if err != nil {
		return nil, fmt.Errorf("subtitle index: build: %w", err)
	}
	return ix, c.put(filePath, ix)
}
