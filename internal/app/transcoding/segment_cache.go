package transcoding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// SegmentKey uniquely identifies a single HLS segment on disk. Two playback
// sessions with the same key MUST share the same segment file (whole point of
// the cache). The key is content-addressed by the DecisionHash — anything that
// would make ffmpeg produce different bytes (codec, audio track, sub burn-in,
// HDR tonemap, …) is hashed in, so cache collisions are impossible.
type SegmentKey struct {
	DecisionHash string // see DecisionFingerprint
	VariantIdx   int    // index into ManifestInput.Profiles (0 = lowest quality)
	SegmentIdx   int    // 0-based segment index in the variant playlist
	// IsInit marks the fMP4 initialization segment (`stream_N_init.mp4`).
	// Init segments have no segment index — they carry the moov box only.
	IsInit bool
}

// fileName returns the on-disk name for the segment (without directory). Mirrors
// the URLs the manifest generator emits, so the orchestrator + http handler agree.
func (k SegmentKey) fileName(useFMP4 bool) string {
	if k.IsInit {
		return fmt.Sprintf("stream_%d_init.mp4", k.VariantIdx)
	}
	ext := "ts"
	if useFMP4 {
		ext = "m4s"
	}
	return fmt.Sprintf("stream_%d_%03d.%s", k.VariantIdx, k.SegmentIdx, ext)
}

// DecisionFingerprint is the canonical input that produces a DecisionHash. Two
// playback decisions that fingerprint identically MUST yield byte-identical
// segments — the same ffmpeg invocation against the same source. Any new
// parameter that influences the encoded bytes belongs here.
//
// Note: stream_token is NOT part of the fingerprint — it changes per session
// but doesn't change the bytes ffmpeg emits.
type DecisionFingerprint struct {
	MediaID             string
	SourcePath          string // absolute path; same media at a different path = different cache
	VideoCodec          string // domain codec: "h264", "hevc", "av1"
	AudioCodec          string
	AudioTrack          int
	SubtitleBurnInTrack int // -1 = none
	HDRTonemap          bool
	AudioNormalize      bool
	MaxAudioChannels    int
	ForceSidecarSubs    bool
	UseFMP4             bool

	// Profiles must be the EXACT ABR ladder ffmpeg will emit. Different ladders
	// produce different multi-variant runs (variant indices and bitrates differ),
	// so they belong in the hash. Encoded as `name@WxH@vKbps@aKbps;…` ordered.
	Profiles []ProfileFingerprint
}

// ProfileFingerprint is the subset of HLSProfile that affects the encoded bytes
// (the human-readable Name is included for cache-dir diagnosability).
type ProfileFingerprint struct {
	Name         string
	Width        int
	Height       int
	VideoBitrate int64
	AudioBitrate int64
}

// Hash returns the canonical, stable SHA-256 fingerprint as a hex string. The
// canonical form sorts/normalises fields so logically-equivalent decisions
// (e.g. same profiles in a different append order) collide on the same hash.
func (f DecisionFingerprint) Hash() string {
	profiles := append([]ProfileFingerprint(nil), f.Profiles...)
	// Stable order: by width then by name. The orchestrator already pushes
	// profiles in ascending width but we don't trust callers.
	sort.SliceStable(profiles, func(i, j int) bool {
		if profiles[i].Width != profiles[j].Width {
			return profiles[i].Width < profiles[j].Width
		}
		return profiles[i].Name < profiles[j].Name
	})

	var b strings.Builder
	fmt.Fprintf(&b, "media=%s\n", f.MediaID)
	fmt.Fprintf(&b, "src=%s\n", f.SourcePath)
	fmt.Fprintf(&b, "vcodec=%s\n", f.VideoCodec)
	fmt.Fprintf(&b, "acodec=%s\n", f.AudioCodec)
	fmt.Fprintf(&b, "atrack=%d\n", f.AudioTrack)
	fmt.Fprintf(&b, "sub=%d\n", f.SubtitleBurnInTrack)
	fmt.Fprintf(&b, "hdr=%t\n", f.HDRTonemap)
	fmt.Fprintf(&b, "anorm=%t\n", f.AudioNormalize)
	fmt.Fprintf(&b, "ach=%d\n", f.MaxAudioChannels)
	fmt.Fprintf(&b, "sidecar=%t\n", f.ForceSidecarSubs)
	fmt.Fprintf(&b, "fmp4=%t\n", f.UseFMP4)
	for _, p := range profiles {
		fmt.Fprintf(&b, "p=%s|%dx%d|%d|%d\n", p.Name, p.Width, p.Height, p.VideoBitrate, p.AudioBitrate)
	}
	sum := sha256.Sum256([]byte(b.String()))
	// 16 hex chars = 64 bits, low enough collision risk for our cache scale.
	return hex.EncodeToString(sum[:])[:16]
}

// SegmentCache is the on-disk store for transcoded HLS segments, addressed by
// SegmentKey. Phase 1: mono-session, no eviction — files live until
// CleanupHash is called explicitly. Phase 2 will add LRU eviction sized by
// total bytes on disk.
//
// Concurrency: all methods are safe for concurrent use. The in-memory index
// tracks which segments have been finalised (written + closed by ffmpeg). The
// orchestrator uses Wait to block until the requested segment is ready.
type SegmentCache struct {
	baseDir string // <transcodeCacheDir>/segments

	mu     sync.RWMutex
	// pending[hash] = chan closed when CleanupHash is called for that hash, so
	// Wait can abort early on session teardown instead of timing out.
	pending map[string]chan struct{}
}

// NewSegmentCache returns a cache rooted at baseDir/segments. The directory is
// created if missing. Caller owns baseDir's lifecycle.
func NewSegmentCache(baseDir string) (*SegmentCache, error) {
	root := filepath.Join(baseDir, "segments")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("SegmentCache: mkdir %q: %w", root, err)
	}
	return &SegmentCache{
		baseDir: root,
		pending: make(map[string]chan struct{}),
	}, nil
}

// Path returns the absolute path the segment WILL live at (whether or not it
// currently exists). Use IsReady to check existence, Wait to block.
func (c *SegmentCache) Path(k SegmentKey, useFMP4 bool) string {
	return filepath.Join(c.baseDir, k.DecisionHash, k.fileName(useFMP4))
}

// DirFor returns the absolute output directory for a given DecisionHash. The
// orchestrator hands this to ffmpeg as `-hls_segment_filename` parent.
func (c *SegmentCache) DirFor(decisionHash string) (string, error) {
	if decisionHash == "" {
		return "", errors.New("SegmentCache.DirFor: empty decisionHash")
	}
	dir := filepath.Join(c.baseDir, decisionHash)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("SegmentCache.DirFor: mkdir %q: %w", dir, err)
	}
	return dir, nil
}

// IsReady returns true iff the segment file exists on disk AND no `.tmp` sibling
// is still being written to it. ffmpeg with `-hls_flags temp_file` writes
// `<name>.tmp` first then renames atomically on segment close, so the absence
// of `.tmp` AND the presence of the final name together prove a complete segment.
func (c *SegmentCache) IsReady(k SegmentKey, useFMP4 bool) bool {
	final := c.Path(k, useFMP4)
	if _, err := os.Stat(final); err != nil {
		return false
	}
	// Defence in depth: if a `.tmp` exists alongside, treat as not-ready even
	// though the rename normally beats the stat — costs one extra syscall.
	if _, err := os.Stat(final + ".tmp"); err == nil {
		return false
	}
	return true
}

// Wait blocks until the segment is ready, or ctx is cancelled, or the session
// is torn down via CleanupHash. Returns nil when ready, an error otherwise.
// Implementation is a poll loop with exponential backoff — inotify would be
// nicer but is Linux-only and adds inotify_init1 to the syscall surface for
// the seccomp profile, which we deliberately keep narrow.
func (c *SegmentCache) Wait(ctx context.Context, k SegmentKey, useFMP4 bool, timeout time.Duration) error {
	if c.IsReady(k, useFMP4) {
		return nil
	}

	c.mu.RLock()
	cancelCh, hasCancel := c.pending[k.DecisionHash]
	c.mu.RUnlock()

	deadline := time.Now().Add(timeout)
	delay := 20 * time.Millisecond
	maxDelay := 250 * time.Millisecond

	for {
		// Re-check after each sleep cycle so a segment that lands just before
		// the timer fires is still observed.
		if c.IsReady(k, useFMP4) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("SegmentCache.Wait: timeout after %s waiting for %s/%s", timeout, k.DecisionHash, k.fileName(useFMP4))
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-cancelChIfNotNil(cancelCh, hasCancel):
			timer.Stop()
			return fmt.Errorf("SegmentCache.Wait: session %s torn down", k.DecisionHash)
		case <-timer.C:
		}
		if delay < maxDelay {
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}
}

// CleanupHash removes every file under <baseDir>/segments/<decisionHash> and
// signals every concurrent Wait() to abort. Safe to call multiple times.
func (c *SegmentCache) CleanupHash(decisionHash string) error {
	if decisionHash == "" {
		return errors.New("SegmentCache.CleanupHash: empty decisionHash")
	}
	c.mu.Lock()
	if ch, ok := c.pending[decisionHash]; ok {
		close(ch)
		delete(c.pending, decisionHash)
	}
	c.mu.Unlock()

	dir := filepath.Join(c.baseDir, decisionHash)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("SegmentCache.CleanupHash %q: %w", dir, err)
	}
	return nil
}

// RegisterSession marks a hash as live so Wait calls bind to its lifetime.
// Returns a teardown function the caller must invoke (typically via defer) when
// the session ends — that closes the channel, aborting in-flight Waits.
// Idempotent on the same hash: a second RegisterSession returns the existing
// teardown without recreating the channel.
func (c *SegmentCache) RegisterSession(decisionHash string) func() {
	c.mu.Lock()
	if _, ok := c.pending[decisionHash]; !ok {
		c.pending[decisionHash] = make(chan struct{})
	}
	c.mu.Unlock()
	return func() {
		_ = c.CleanupHash(decisionHash)
	}
}

// ListReady returns the segment indices currently materialised under hash for a
// given variant. Used by the orchestrator to decide whether a fresh ffmpeg
// rafale is needed or the requested segment is already on disk. Cheap O(N)
// directory scan — no in-memory index to keep coherent.
func (c *SegmentCache) ListReady(decisionHash string, variantIdx int, useFMP4 bool) ([]int, error) {
	dir := filepath.Join(c.baseDir, decisionHash)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("SegmentCache.ListReady: %w", err)
	}
	prefix := fmt.Sprintf("stream_%d_", variantIdx)
	ext := ".ts"
	if useFMP4 {
		ext = ".m4s"
	}
	var idxs []int
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ext) {
			continue
		}
		// Skip the init segment (no segment index in its name).
		if strings.Contains(name, "_init.") {
			continue
		}
		// Names look like "stream_0_042.m4s" — extract the 042.
		body := strings.TrimPrefix(name, prefix)
		body = strings.TrimSuffix(body, ext)
		var n int
		if _, err := fmt.Sscanf(body, "%d", &n); err != nil {
			continue
		}
		idxs = append(idxs, n)
	}
	sort.Ints(idxs)
	return idxs, nil
}

// cancelChIfNotNil yields a never-fires channel when has=false so the select in
// Wait doesn't have to special-case the no-session-registered path. A nil
// receive in select blocks forever, which is exactly what we want here.
func cancelChIfNotNil(ch chan struct{}, has bool) <-chan struct{} {
	if has {
		return ch
	}
	return nil
}
