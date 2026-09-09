package transcoding

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/sysinfo"
)

// SegmentKind tags a SegmentKey as belonging to the muxed/video pipeline
// (the default empty string = "v") or the audio-only rendition pipeline
// (KindAudio). Audio segments live in their own DecisionHash directory
// (computed from AudioFingerprint) and use a distinct filename pattern
// so they never collide with video segments even when the cache dir
// happens to be shared.
type SegmentKind string

const (
	KindVideo SegmentKind = ""
	KindAudio SegmentKind = "audio"
)

// SegmentKey uniquely identifies a single HLS segment on disk. Two playback
// sessions with the same key MUST share the same segment file (whole point of
// the cache). The key is content-addressed by the DecisionHash - anything that
// would make ffmpeg produce different bytes (codec, audio track, sub burn-in,
// HDR tonemap, …) is hashed in, so cache collisions are impossible.
type SegmentKey struct {
	DecisionHash string // see DecisionFingerprint / AudioFingerprint
	VariantIdx   int    // video: index into ManifestInput.Profiles; audio: source audio track index
	SegmentIdx   int    // 0-based segment index in the variant playlist
	// IsInit marks the fMP4 initialization segment (`stream_N_init.mp4`).
	// Init segments have no segment index - they carry the moov box only.
	IsInit bool
	// Kind picks the URL/filename pattern. Empty (default) = video,
	// matches the pre-feature behaviour exactly. KindAudio uses the
	// audio_T_NNN.m4s pattern and ignores useFMP4 (audio is fMP4-only).
	Kind SegmentKind
}

// fileName returns the on-disk name for the segment (without directory). Mirrors
// the URLs the manifest generator emits, so the orchestrator + http handler agree.
func (k SegmentKey) fileName(useFMP4 bool) string {
	if k.Kind == KindAudio {
		if k.IsInit {
			return fmt.Sprintf("audio_%d_init.mp4", k.VariantIdx)
		}
		return fmt.Sprintf("audio_%d_%03d.m4s", k.VariantIdx, k.SegmentIdx)
	}
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
// segments - the same ffmpeg invocation against the same source. Any new
// parameter that influences the encoded bytes belongs here.
//
// Note: stream_token is NOT part of the fingerprint - it changes per session
// but doesn't change the bytes ffmpeg emits.
type DecisionFingerprint struct {
	// AudioEncoder is the concrete ffmpeg AAC implementation these bytes were
	// made with - see HLSProcessOpts.AudioEncoder. Hashed only when it is not
	// the native encoder (nonNativeAudioEncoder), so a box that never had
	// libfdk keeps every segment it already cached.
	AudioEncoder        string
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
	// AudioCopy passes the source audio through (`-c:a copy`) instead of
	// re-encoding to AAC. Copied and transcoded audio are DIFFERENT bytes for
	// the same media, so this is a Hash() dimension - but emitted CONDITIONALLY
	// (only when true) so enabling the feature does not orphan every existing
	// cache entry whose behavior is unchanged. DecisionFingerprint has no
	// reflexive every-field-affects-hash guard (unlike ProfileFingerprint), so
	// the dimension is covered by an explicit test: Hash_DistinguishesAudioCopy.
	AudioCopy bool

	// AudioOffsetMs is the user's audio resync (positive = audio later),
	// applied by re-encoding the audio from a time-shifted input. A different
	// offset = different bytes, so it is a Hash() dimension - emitted
	// CONDITIONALLY (only when non-zero) so the zero-offset majority keeps its
	// existing cache entries.
	AudioOffsetMs int

	// SourceVideoCodec is the codec of the SOURCE video stream ("h264",
	// "hevc"…), set by the handler from the probed streams. The copy builder
	// needs it to tag the fMP4 sample entry (`-tag:v hvc1` - Safari refuses
	// ffmpeg's hev1 default). Deliberately EXCLUDED from Hash(): it is fully
	// determined by (MediaID, SourcePath) which are already hashed, so it can
	// never alias two different byte streams.
	SourceVideoCodec string

	// SourceVideoHeight is the SOURCE video height. With SourceVideoCodec it names
	// the DECODE workload admission prices. EXCLUDED from Hash() for the same
	// reason as SourceVideoCodec: determined by (MediaID, SourcePath).
	SourceVideoHeight int

	// SubtitleBurnInCodec is the codec of the burn-in subtitle track
	// (SubtitleBurnInTrack), set by the handler from the probed streams. The
	// ffmpeg builder re-derives burn-in eligibility from it (IsImageSubtitle),
	// so a bitmap subtitle the decision forced a transcode to burn is silently
	// dropped without it. Deliberately EXCLUDED from Hash(): it is fully
	// determined by (MediaID, SourcePath, SubtitleBurnInTrack), all already
	// hashed, so it can never alias two different byte streams.
	SubtitleBurnInCodec string

	// DeinterlaceAll switches the software builder to bwdif deint=all (vs the
	// default flag-gated deint=interlaced) for legacy sources the idet content
	// detection flagged interlaced despite their container flags. Different
	// deinterlace mode ⇒ different output frames, so it is a Hash() dimension -
	// emitted CONDITIONALLY (only when true) so enabling it never orphans the
	// caches of progressive titles whose bytes are unchanged.
	DeinterlaceAll bool
	// SourceSAR is the source sample aspect ratio ("64:45" anamorphic, ""/"1:1"
	// square). Anamorphic correction changes the scaled height, so the served
	// bytes differ ⇒ Hash() dimension, emitted CONDITIONALLY (only when it
	// denotes a non-square ratio) to preserve every square-pixel cache entry.
	SourceSAR string

	// HWAccel / HWDevice select the hardware encode backend ("nvenc"/"vaapi"/
	// "qsv" + DRM render node) when the handler resolves one. Deliberately
	// EXCLUDED from Hash(): a hardware-encoded segment is byte-compatible with
	// the software one (same codec/bitrate/resolution/container/naming), so the
	// addressable cache stays encoder-agnostic - toggling HW at runtime must
	// not fork the cache. Empty = software path.
	HWAccel  string
	HWDevice string
	// HWFeatures is the probed tuning set forwarded to the HW encoder. Like
	// HWAccel/HWDevice it is EXCLUDED from Hash(): tuning options affect the
	// encoder's internal RC, not the segment's codec/bitrate/resolution/
	// container/naming, so a segment encoded with or without them stays
	// interchangeable in the addressable cache (and a level-2 demotion that
	// drops tuning must not fork it).
	HWFeatures []string
	// HWSoftwareDecode marks the DECOUPLED hardware route: software decode +
	// hwupload + HW scale/encode, for a source this GPU cannot decode (see
	// HLSProcessOpts.HWSoftwareDecode). EXCLUDED from Hash() for the same reason
	// as HWAccel: what changes is which silicon decodes, not the codec, bitrate,
	// resolution, container or naming of the segment produced - a decoupled
	// segment stays interchangeable with a zero-copy or software one.
	HWSoftwareDecode bool

	// HWTonemap requests GPU HDR->SDR tone-mapping for this transcode. EXCLUDED
	// from Hash() like HWAccel/HWFeatures: a HW- vs software-tonemapped segment is
	// the same codec/bitrate/resolution/container/naming, so the addressable cache
	// stays encoder-agnostic (toggling HW tone-mapping must not fork the cache).
	HWTonemap bool

	// HWOverlay requests a GPU burn-in of the bitmap subtitle. EXCLUDED from
	// Hash() for the same reason as HWTonemap: the composited picture is the same
	// subtitle on the same frames, so a GPU- and a software-burnt segment stay
	// interchangeable in the addressable cache.
	HWOverlay bool

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
	// Copy marks a remux (video-copy) rung. Copy bytes != transcode bytes at the
	// same WxH, so it is folded into Hash(). Whenever a byte-affecting field is
	// added here, TestProfileFingerprint_EveryFieldAffectsHash fails until it is
	// also added to Hash() - the structural guard against the next omission.
	Copy bool
}

// ProfileFingerprintOf projects a domain HLSProfile onto its cache fingerprint.
// The ONLY place HLSProfile → ProfileFingerprint is performed, so the projection
// can be reflection-guarded in one spot: TestProfileFingerprintOf_CopiesEveryField
// reddens if a ProfileFingerprint field is not copied from its same-named
// HLSProfile field. Together with the Hash() reflection guard this closes the
// HLSProfile → ProfileFingerprint → Hash chain - a forgotten byte-affecting field
// fails a test rather than silently aliasing the cache.
func ProfileFingerprintOf(p transcoding.HLSProfile) ProfileFingerprint {
	return ProfileFingerprint{
		Name:         p.Name,
		Width:        p.Width,
		Height:       p.Height,
		VideoBitrate: p.VideoBitrate,
		AudioBitrate: p.AudioBitrate,
		Copy:         p.Copy,
	}
}

// AudioFingerprint is the content-address of an audio-only HLS rendition
// session. Lives in its own namespace (separate Hash space from
// DecisionFingerprint) so the audio pipeline can be swapped/rebuilt
// independently of the video one. Every parameter that influences the
// audio bytes ffmpeg emits belongs here.
//
// SourceTrackIdx is the 0-indexed audio stream in the source media - the
// rendition the player picked. AudioCodec is the encoder (aac / ac3 /
// eac3 / opus). AudioNormalize and MaxAudioChannels mirror the muxed
// pipeline's existing knobs so audio settings stay consistent across
// the two pipelines.
type AudioFingerprint struct {
	MediaID        string
	SourcePath     string
	SourceTrackIdx int
	AudioCodec     string
	// AudioEncoder is the concrete ffmpeg AAC implementation these bytes were
	// made with - see HLSProcessOpts.AudioEncoder. Hashed only when it is not
	// the native encoder, so existing caches stay valid where nothing changed.
	AudioEncoder     string
	AudioBitrate     int64 // kbps; 0 = encoder default
	AudioNormalize   bool
	MaxAudioChannels int

	// SourceDurationSec is the container-declared media duration, in seconds.
	// TRANSPORT ONLY - deliberately excluded from Hash(): it is not an encode
	// input (same bytes either way), it merely lets the orchestrator BOUND the
	// silence generator it spawns when the audio track proves to end before
	// this duration (see AudioSegmentOrchestrator's stream-end handling).
	SourceDurationSec float64

	// VideoCompositionDelay is the seconds the VIDEO's first frame is presented
	// late for the packaging THIS session's video actually uses: 0 when mkvgo
	// packages it (it presents at 0), has_b_frames/fps when the video falls back
	// to ffmpeg `-c:v copy` in fMP4 (MP4 forbids a negative DTS). The audio
	// rendition is its own stream starting at 0, so without this shift it runs
	// EARLY by exactly that much - audible past ~45 ms.
	//
	// HASHED, unlike SourceDurationSec above: the shift moves the OUTPUT
	// timestamps, so shifted and unshifted segments are different bytes and must
	// never alias in the cache.
	VideoCompositionDelay float64
}

// Hash returns the canonical 64-bit hex fingerprint for an audio rendition.
// Namespaced with the literal "audio:" prefix in the digest input so the
// hash space can NEVER collide with DecisionFingerprint, even by accident
// (sha256 collisions aside).
func (a AudioFingerprint) Hash() string {
	return a.hashWithDownmixPolicy(a.downmixPolicy())
}

// downmixPolicy is the fold-down policy this rendition encodes under, or empty
// when it folds nothing (a layout-preserving target) or re-encodes nothing (the
// wasm passthrough). Those two keep their pre-policy hash on purpose: their
// bytes did not change, so their cached segments stay valid.
func (a AudioFingerprint) downmixPolicy() string {
	if a.AudioCodec == "copy" {
		return ""
	}
	return transcoding.AudioDownmixPolicy(a.MaxAudioChannels)
}

// hashWithDownmixPolicy is Hash with the policy injected, so a test can prove
// that changing the matrix policy really forks the cache. AudioFingerprint has
// no field that carries it - MaxAudioChannels is 2 before AND after the fix -
// so without this line the orchestrator would keep serving the clipped
// segments it already produced and the fix would be invisible.
func (a AudioFingerprint) hashWithDownmixPolicy(downmixPolicy string) string {
	var b strings.Builder
	b.WriteString("audio:\n")
	fmt.Fprintf(&b, "media=%s\n", a.MediaID)
	fmt.Fprintf(&b, "src=%s\n", a.SourcePath)
	fmt.Fprintf(&b, "atrack=%d\n", a.SourceTrackIdx)
	fmt.Fprintf(&b, "acodec=%s\n", a.AudioCodec)
	if aenc := nonNativeAudioEncoder(a.AudioEncoder); aenc != "" {
		fmt.Fprintf(&b, "aenc=%s\n", aenc)
	}
	fmt.Fprintf(&b, "abitrate=%d\n", a.AudioBitrate)
	fmt.Fprintf(&b, "anorm=%t\n", a.AudioNormalize)
	fmt.Fprintf(&b, "ach=%d\n", a.MaxAudioChannels)
	// Milliseconds, not the raw float: the probe is quantised to the frame
	// anyway, and a %f of a float64 would fork the cache on rounding noise.
	fmt.Fprintf(&b, "vcd=%d\n", int64(a.VideoCompositionDelay*1000+0.5))
	if downmixPolicy != "" {
		fmt.Fprintf(&b, "dmx=%s\n", downmixPolicy)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])[:16]
}

// AudioDecision returns the HLSProcessOpts.AudioDecision value for this
// fingerprint: "copy" only when the handler established AudioCopy eligibility
// (selected source track decodable by the client, no loudnorm, single-rung
// ladder), else "transcode" - the historical behavior. Both producers derive
// the ffmpeg audio args from THIS so a given hash always yields the same
// bytes regardless of which producer lands the segment.
func (f DecisionFingerprint) AudioDecision() string {
	if f.AudioCopy {
		return "copy"
	}
	return "transcode"
}

// Hash returns the canonical, stable SHA-256 fingerprint as a hex string. The
// canonical form sorts/normalises fields so logically-equivalent decisions
// (e.g. same profiles in a different append order) collide on the same hash.
func (f DecisionFingerprint) Hash() string {
	return f.hashWithDownmixPolicy(f.downmixPolicy())
}

// downmixPolicy mirrors AudioFingerprint.downmixPolicy for the muxed pipeline:
// empty when the session copies audio or keeps the source layout.
func (f DecisionFingerprint) downmixPolicy() string {
	if f.AudioCopy {
		return ""
	}
	return transcoding.AudioDownmixPolicy(f.MaxAudioChannels)
}

// hashWithDownmixPolicy is Hash with the policy injected - see the twin on
// AudioFingerprint for why the fingerprint cannot derive it from its fields.
func (f DecisionFingerprint) hashWithDownmixPolicy(downmixPolicy string) string {
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
	// The CONCRETE AAC implementation, and only when it is not the historical
	// native encoder: libfdk and native produce different bytes at one bitrate,
	// so they must not share a cache entry. Emitting nothing for the native case
	// keeps every already-cached segment on a box without libfdk valid.
	if aenc := nonNativeAudioEncoder(f.AudioEncoder); aenc != "" {
		fmt.Fprintf(&b, "aenc=%s\n", aenc)
	}
	fmt.Fprintf(&b, "atrack=%d\n", f.AudioTrack)
	fmt.Fprintf(&b, "sub=%d\n", f.SubtitleBurnInTrack)
	fmt.Fprintf(&b, "hdr=%t\n", f.HDRTonemap)
	fmt.Fprintf(&b, "anorm=%t\n", f.AudioNormalize)
	fmt.Fprintf(&b, "ach=%d\n", f.MaxAudioChannels)
	fmt.Fprintf(&b, "sidecar=%t\n", f.ForceSidecarSubs)
	fmt.Fprintf(&b, "fmp4=%t\n", f.UseFMP4)
	if f.AudioCopy {
		// Conditional on purpose: see the AudioCopy field comment.
		b.WriteString("acopy=true\n")
	}
	if f.AudioOffsetMs != 0 {
		// Conditional: only a resynced session forks its cache.
		fmt.Fprintf(&b, "aoffset=%d\n", f.AudioOffsetMs)
	}
	if f.DeinterlaceAll {
		// Conditional: only an idet-flagged interlaced title forks its cache.
		b.WriteString("deintall=true\n")
	}
	if num, den, ok := parseSARRatio(f.SourceSAR); ok && num != den {
		// Conditional: only an anamorphic source forks its cache (square pixels
		// keep the pre-feature hash). Normalised to the reduced ratio so "64:45"
		// and "128:90" hash identically.
		fmt.Fprintf(&b, "sar=%d:%d\n", num, den)
	}
	if downmixPolicy != "" {
		fmt.Fprintf(&b, "dmx=%s\n", downmixPolicy)
	}
	for _, p := range profiles {
		fmt.Fprintf(&b, "p=%s|%dx%d|%d|%d|copy=%t\n", p.Name, p.Width, p.Height, p.VideoBitrate, p.AudioBitrate, p.Copy)
	}
	sum := sha256.Sum256([]byte(b.String()))
	// 16 hex chars = 64 bits, low enough collision risk for our cache scale.
	return hex.EncodeToString(sum[:])[:16]
}

// parseSARRatio parses "N:D" into reduced integer parts (gcd-normalised so
// equivalent ratios hash identically). ok=false for empty/malformed/zero
// values - treated as square pixels by the caller.
func parseSARRatio(s string) (num, den int, ok bool) {
	n, d, found := strings.Cut(strings.TrimSpace(s), ":")
	if !found {
		return 0, 0, false
	}
	num, err1 := strconv.Atoi(n)
	den, err2 := strconv.Atoi(d)
	if err1 != nil || err2 != nil || num <= 0 || den <= 0 {
		return 0, 0, false
	}
	g := gcdInt(num, den)
	return num / g, den / g, true
}

func gcdInt(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}

// SegmentCache is the on-disk store for transcoded HLS segments, addressed by
// SegmentKey. Phase 1: mono-session, no eviction - files live until
// CleanupHash is called explicitly. Phase 2 will add LRU eviction sized by
// total bytes on disk.
//
// Concurrency: all methods are safe for concurrent use. The in-memory index
// tracks which segments have been finalised (written + closed by ffmpeg). The
// orchestrator uses Wait to block until the requested segment is ready.
type SegmentCache struct {
	baseDir string // <transcodeCacheDir>/segments

	mu sync.RWMutex
	// pending[hash] = chan closed when the LAST live session for that
	// hash is torn down (orRefCount drops to 0), or when CleanupHash
	// is called explicitly. Wait subscribes to this channel so it
	// aborts early on session teardown instead of timing out.
	//
	// Refcounted because the multi-rafale orchestrator may have
	// several sessions sharing the same hash directory (two clients
	// on different positions of the same media). Closing the channel
	// when ONE rafale teardowns would break Wait() for the others.
	pending     map[string]chan struct{}
	pendingRefs map[string]int

	// accessTimes records the last time each hash was Touched (a segment was
	// served to a player). The LRU sweeper merges it with the directory mtime so
	// an actively-READ hash stays warm even when nothing is being WRITTEN to it.
	// This matters for the copy rung: its ReadRate=0 mux finishes in minutes,
	// then the dir mtime freezes, so without this an actively-watched 2 h title
	// looks cold to the sweeper and gets evicted out from under the player.
	// Guarded by mu. Entries are dropped in CleanupHash so the map tracks the
	// live cache, not history.
	accessTimes map[string]time.Time
}

// NewSegmentCache returns a cache rooted at baseDir/segments. The directory is
// created if missing. Caller owns baseDir's lifecycle.
func NewSegmentCache(baseDir string) (*SegmentCache, error) {
	root := filepath.Join(baseDir, "segments")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("SegmentCache: mkdir %q: %w", root, err)
	}
	return &SegmentCache{
		baseDir:     root,
		pending:     make(map[string]chan struct{}),
		pendingRefs: make(map[string]int),
		accessTimes: make(map[string]time.Time),
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
	fi, err := os.Stat(final)
	if err != nil {
		return false
	}
	// A 0-byte file is a segment mid-creation, NOT a ready one. ffmpeg writes
	// the fMP4 init segment (stream_N_init.mp4) DIRECTLY - it does not get the
	// `.tmp`→rename atomicity that `-hls_flags temp_file` gives media segments -
	// so there is a window where the init exists at 0 bytes. Serving it empty
	// makes the player's SourceBuffer init fail → endless buffering with no
	// error. Treat any 0-byte segment as not-ready so Wait blocks until ffmpeg
	// has actually written it (and a truncated init left by a killed rafale
	// re-spawns instead of being served forever). Latency-amplified by HW
	// rc-lookahead, which delays the first encoded frames.
	if fi.Size() == 0 {
		return false
	}
	// Defence in depth: if a `.tmp` exists alongside, treat as not-ready even
	// though the rename normally beats the stat - costs one extra syscall.
	if _, err := os.Stat(final + ".tmp"); err == nil {
		return false
	}
	// fMP4 segments must carry actual content. A non-empty file holding only a
	// `styp`/`ftyp` box and no moov/moof/mdat is a malformed segment an aborted
	// ffmpeg mux left at the FINAL name (observed in prod under load: a 24-byte
	// styp-only `.m4s` - `size>0` so it passed the check above). Serving it wedges
	// the player's SourceBuffer with no error, which also breaks the ABR fallback
	// to a lower rung. Treat it as not-ready so Wait re-spawns the rafale. Cheap:
	// reads only top-level box headers (≤16 bytes each, a handful of boxes).
	if useFMP4 && !fmp4HasContentBox(final, fi.Size()) {
		return false
	}
	return true
}

// fmp4HasContentBox reports whether an ISO-BMFF file holds at least one
// fully-written content box - `moov` (init segment) or `moof`/`mdat` (media
// segment). It walks only top-level box headers and requires the content box to
// be entirely on disk, so it rejects both an aborted styp-only segment and a
// content box ffmpeg is still mid-writing (no torn read served). total is the
// stat size already taken by the caller.
func fmp4HasContentBox(path string, total int64) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	var hdr [16]byte
	var off int64
	for range 16 { // a sane segment reaches a content box in its first few boxes
		if off+8 > total {
			return false // EOF before any content box (e.g. styp-only)
		}
		if _, err := f.ReadAt(hdr[:8], off); err != nil {
			return false
		}
		size := int64(binary.BigEndian.Uint32(hdr[0:4]))
		typ := string(hdr[4:8])
		hdrLen := int64(8)
		switch size {
		case 1: // 64-bit largesize follows the 8-byte header
			if off+16 > total {
				return false
			}
			if _, err := f.ReadAt(hdr[8:16], off+8); err != nil {
				return false
			}
			size = int64(binary.BigEndian.Uint64(hdr[8:16]))
			hdrLen = 16
		case 0: // box runs to EOF
			size = total - off
		}
		if size < hdrLen {
			return false // malformed box size - bail rather than loop forever
		}
		switch typ {
		case "moov", "moof", "mdat":
			return off+size <= total // require the content box fully written
		}
		off += size
	}
	return false
}

// Wait blocks until the segment is ready, or ctx is cancelled, or the session
// is torn down via CleanupHash. Returns nil when ready, an error otherwise.
// Implementation is a poll loop with exponential backoff - inotify would be
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
	delete(c.accessTimes, decisionHash)
	c.mu.Unlock()

	dir := filepath.Join(c.baseDir, decisionHash)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("SegmentCache.CleanupHash %q: %w", dir, err)
	}
	return nil
}

// cleanupHashIfIdle is the eviction-safe variant used by the LRU sweeper: it
// deletes the hash directory only when no live session is serving it
// (pendingRefs == 0). The refs check and the in-memory teardown happen under the
// same lock, so a session streaming an older-cached title (low atime, outside
// ProtectRecent) is not evicted out from under it - the sweeper used to protect
// by recency only, so a slow client on an old hash could be cut to a
// stall/503/re-encode. Returns whether the hash was evicted. A session that
// registers in the tiny window after the lock is released just re-encodes the
// missing segments (rare, and no worse than the prior unconditional eviction).
func (c *SegmentCache) cleanupHashIfIdle(decisionHash string) (bool, error) {
	if decisionHash == "" {
		return false, errors.New("SegmentCache.cleanupHashIfIdle: empty decisionHash")
	}
	c.mu.Lock()
	if c.pendingRefs[decisionHash] > 0 {
		c.mu.Unlock()
		return false, nil // a live session is serving this hash - keep it
	}
	if ch, ok := c.pending[decisionHash]; ok {
		close(ch)
		delete(c.pending, decisionHash)
	}
	delete(c.accessTimes, decisionHash)
	c.mu.Unlock()

	dir := filepath.Join(c.baseDir, decisionHash)
	if err := os.RemoveAll(dir); err != nil {
		return false, fmt.Errorf("SegmentCache.cleanupHashIfIdle %q: %w", dir, err)
	}
	return true, nil
}

// Touch records that hash was just accessed (a segment was served to a player).
// The LRU sweeper treats a recently-touched hash as recently-used even though
// reads never update file mtime. Essential for the copy rung whose ReadRate=0
// mux freezes the dir mtime once it finishes - without Touch an actively-watched
// title looks cold and is evicted out from under the player. One cheap map write.
func (c *SegmentCache) Touch(hash string) {
	if hash == "" {
		return
	}
	c.mu.Lock()
	c.accessTimes[hash] = time.Now()
	c.mu.Unlock()
}

// FreeBytes reports the free space on the filesystem backing the segment cache.
// The copy-rung disk guard uses it to refuse a whole-file remux that would not
// fit, so a small box falls back to the (segment-bounded) transcode ladder
// instead of filling the disk to ENOSPC. Reuses the same statfs path as the
// admin storage stats.
func (c *SegmentCache) FreeBytes() (int64, error) {
	stats := sysinfo.GetStorageStats([]string{c.baseDir})
	if len(stats) == 0 {
		return 0, fmt.Errorf("SegmentCache.FreeBytes: statfs %q failed", c.baseDir)
	}
	return stats[0].FreeBytes, nil
}

// LastAccess returns the last Touch time for hash and whether one was recorded.
// Used by the whole-file strategy's idle-cancel: a hash that has not been
// touched for a while has no player pulling segments, so its background mux can
// be stopped. Falls back to "not found" for hashes never served (cold start).
func (c *SegmentCache) LastAccess(hash string) (time.Time, bool) {
	c.mu.RLock()
	t, ok := c.accessTimes[hash]
	c.mu.RUnlock()
	return t, ok
}

// RegisterSession marks a hash as live so Wait calls bind to its lifetime.
// Returns a release function the caller must invoke when the session ends -
// that aborts in-flight Waits but does NOT delete files. Disk cleanup happens
// either via the LRU sweeper or by an explicit CleanupHash on shutdown. This
// separation lets cancel-on-seek tear down a rafale without throwing away the
// segments it has already produced (the user may rewind into that zone next).
// Idempotent on the same hash.
func (c *SegmentCache) RegisterSession(decisionHash string) func() {
	c.mu.Lock()
	if _, ok := c.pending[decisionHash]; !ok {
		c.pending[decisionHash] = make(chan struct{})
	}
	c.pendingRefs[decisionHash]++
	c.mu.Unlock()
	released := false
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		// Make the teardown idempotent - the orchestrator's killSession +
		// reap goroutine could both fire on a normal exit. Only the
		// first call decrements the refcount.
		if released {
			return
		}
		released = true
		c.pendingRefs[decisionHash]--
		if c.pendingRefs[decisionHash] > 0 {
			// Other rafales still serving this hash - don't close the
			// shared channel yet, their Wait() callers depend on it.
			return
		}
		delete(c.pendingRefs, decisionHash)
		if ch, ok := c.pending[decisionHash]; ok {
			close(ch)
			delete(c.pending, decisionHash)
		}
	}
}

// ListReady returns the segment indices currently materialised under hash for a
// given variant. Used by the orchestrator to decide whether a fresh ffmpeg
// rafale is needed or the requested segment is already on disk. Cheap O(N)
// directory scan - no in-memory index to keep coherent.
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
		// Names look like "stream_0_042.m4s" - extract the 042.
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

// HighestReadyFrom returns the highest segment index in [from, until] that is
// ready AND reachable from `from` without a gap, or from-1 when `from` itself
// is not ready. Callers feed the previous result back as the next `from`, which
// is what makes it cheap: it stats only the segments produced since the last
// call (normally 0-2) instead of listing the whole hash directory.
//
// Why not ListReady: the throttle loop asked for every ready index once per
// second per session purely to read the last one, so each tick paid an
// os.ReadDir plus a parse of every entry in a directory that holds a full
// title's segments. That cost grows with the cache, not with the work, and it
// lands on the disk the player is streaming from - the exact I/O the weak-NAS
// target cannot spare.
//
// Stopping at the first gap is deliberate and is what the throttle actually
// wants to know: production is sequential, so a hole means the client stalls
// there, and counting a later island would overstate the buffer and SIGSTOP
// ffmpeg while the client is starved.
func (c *SegmentCache) HighestReadyFrom(decisionHash string, variantIdx int, useFMP4 bool, from, until int) int {
	highest := from - 1
	for idx := from; idx <= until; idx++ {
		k := SegmentKey{DecisionHash: decisionHash, VariantIdx: variantIdx, SegmentIdx: idx}
		if !c.IsReady(k, useFMP4) {
			break
		}
		highest = idx
	}
	return highest
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

// nonNativeAudioEncoder returns the encoder name only when it differs from
// ffmpeg's built-in AAC. The native encoder is the historical default, so it
// must hash to NOTHING: adding a field for it would invalidate every segment
// already on disk on every installation, for bytes that did not change.
func nonNativeAudioEncoder(enc string) string {
	if enc == "" || enc == "aac" {
		return ""
	}
	return enc
}
