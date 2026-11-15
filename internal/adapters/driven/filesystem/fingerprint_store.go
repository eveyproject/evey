package filesystem

// fingerprint_store.go - the intro-detection fingerprint caches, one file per
// media item under the data directory.
//
// These fingerprints are derived data: they are recomputed from the media
// files whenever they are missing or stale (the caller compares the file
// fingerprint and the algorithm version). Kept in the database they made up
// most of every backup while being the one thing a backup never needs, so they
// live beside the other derived per-media stores (trickplay, subtitle index)
// and a database restore leaves them in place.
//
// File layout, little-endian:
//
//	magic[4] | format u8 | algo i16 | window i32 | extracted_at unix-ms i64 |
//	file_fingerprint len u16 + bytes | value count u32 | values | crc32 u32
//
// The CRC covers everything before it. A file that is short, torn or fails its
// CRC reads as a miss, so the worst a damaged file costs is one recompute.

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

const (
	fingerprintExt    = ".fp"
	fingerprintFormat = 1
	audioMagic        = "EVFA"
	phashMagic        = "EVFP"
	// maxFingerprintFile bounds a read: the largest real entry is a few
	// hundred KB (pHash of a long film), so anything bigger is not ours.
	maxFingerprintFile = 32 << 20
)

// safeMediaID keeps a media item id usable as a file name on its own, which
// is what lets the orphan sweep map a file back to its id without an index.
var safeMediaID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

var errBadFingerprintFile = errors.New("fingerprint file: invalid")

type fingerprintDir struct {
	dir   string
	magic string
}

func (d fingerprintDir) path(mediaItemID string) (string, error) {
	if !safeMediaID.MatchString(mediaItemID) {
		return "", fmt.Errorf("fingerprint store: media item id %q is not a safe file name", mediaItemID)
	}
	return filepath.Join(d.dir, mediaItemID+fingerprintExt), nil
}

type fingerprintHeader struct {
	algo            int16
	window          int32
	extractedAt     time.Time
	fileFingerprint string
}

func (d fingerprintDir) encode(h fingerprintHeader, values []byte, count int) ([]byte, error) {
	if len(h.fileFingerprint) > math.MaxUint16 {
		return nil, fmt.Errorf("fingerprint store: file fingerprint too long (%d bytes)", len(h.fileFingerprint))
	}
	if count > math.MaxUint32 {
		return nil, fmt.Errorf("fingerprint store: too many values (%d)", count)
	}
	buf := make([]byte, 0, 4+1+2+4+8+2+len(h.fileFingerprint)+4+len(values)+4)
	buf = append(buf, d.magic...)
	buf = append(buf, fingerprintFormat)
	buf = binary.LittleEndian.AppendUint16(buf, uint16(h.algo))   //nolint:gosec // bit-preserving round trip of an int16
	buf = binary.LittleEndian.AppendUint32(buf, uint32(h.window)) //nolint:gosec // bit-preserving round trip of an int32
	buf = binary.LittleEndian.AppendUint64(buf, uint64(h.extractedAt.UnixMilli()))
	buf = binary.LittleEndian.AppendUint16(buf, uint16(len(h.fileFingerprint)))
	buf = append(buf, h.fileFingerprint...)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(count))
	buf = append(buf, values...)
	return binary.LittleEndian.AppendUint32(buf, crc32.ChecksumIEEE(buf)), nil
}

// decode returns the header, the value count and the raw value bytes.
func (d fingerprintDir) decode(buf []byte) (fingerprintHeader, int, []byte, error) {
	var h fingerprintHeader
	const fixed = 4 + 1 + 2 + 4 + 8 + 2
	if len(buf) < fixed+4+4 || string(buf[:4]) != d.magic || buf[4] != fingerprintFormat {
		return h, 0, nil, errBadFingerprintFile
	}
	body, sum := buf[:len(buf)-4], binary.LittleEndian.Uint32(buf[len(buf)-4:])
	if crc32.ChecksumIEEE(body) != sum {
		return h, 0, nil, errBadFingerprintFile
	}
	h.algo = int16(binary.LittleEndian.Uint16(body[5:]))                         //nolint:gosec // bit-preserving round trip of an int16
	h.window = int32(binary.LittleEndian.Uint32(body[7:]))                       //nolint:gosec // bit-preserving round trip of an int32
	h.extractedAt = time.UnixMilli(int64(binary.LittleEndian.Uint64(body[11:]))) //nolint:gosec // bit-preserving round trip of an int64
	n := int(binary.LittleEndian.Uint16(body[19:]))
	rest := body[fixed:]
	if len(rest) < n+4 {
		return h, 0, nil, errBadFingerprintFile
	}
	h.fileFingerprint = string(rest[:n])
	count := int(binary.LittleEndian.Uint32(rest[n:]))
	return h, count, rest[n+4:], nil
}

func (d fingerprintDir) read(mediaItemID string) ([]byte, error) {
	p, err := d.path(mediaItemID)
	if err != nil {
		return nil, shared.ErrNotFound
	}
	f, err := os.Open(p) //nolint:gosec // safe id under our own data dir
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() > maxFingerprintFile {
		return nil, errBadFingerprintFile
	}
	buf := make([]byte, st.Size())
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, errBadFingerprintFile
	}
	return buf, nil
}

// write publishes the file atomically (temp file + rename), so a reader never
// sees half an entry and a crash mid-write leaves the previous one.
func (d fingerprintDir) write(mediaItemID string, blob []byte) error {
	p, err := d.path(mediaItemID)
	if err != nil {
		return err
	}
	if mkErr := os.MkdirAll(d.dir, 0o750); mkErr != nil {
		return fmt.Errorf("fingerprint store: create dir: %w", mkErr)
	}
	tmp, err := os.CreateTemp(d.dir, mediaItemID+".*.part")
	if err != nil {
		return fmt.Errorf("fingerprint store: temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(blob); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName) //nolint:errcheck // best-effort cleanup
		return fmt.Errorf("fingerprint store: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName) //nolint:errcheck // best-effort cleanup
		return fmt.Errorf("fingerprint store: close: %w", err)
	}
	if err := os.Rename(tmpName, p); err != nil {
		_ = os.Remove(tmpName) //nolint:errcheck // best-effort cleanup
		return fmt.Errorf("fingerprint store: publish: %w", err)
	}
	return nil
}

func (d fingerprintDir) has(mediaItemID string) bool {
	p, err := d.path(mediaItemID)
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

func (d fingerprintDir) remove(mediaItemID string) error {
	p, err := d.path(mediaItemID)
	if err != nil {
		return nil //nolint:nilerr // an id that cannot be a file has no file to remove
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// ids lists the media item ids that have a file, and removes the temp files a
// crash left behind.
func (d fingerprintDir) ids(ctx context.Context) ([]string, error) {
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := e.Name()
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(name, ".part") {
			_ = os.Remove(filepath.Join(d.dir, name)) //nolint:errcheck // best-effort cleanup
			continue
		}
		if id, ok := strings.CutSuffix(name, fingerprintExt); ok && safeMediaID.MatchString(id) {
			out = append(out, id)
		}
	}
	return out, nil
}

// deleteAlgoBelow removes every file whose algorithm version is below version.
// A file that cannot be decoded is removed too: it would never be served.
func (d fingerprintDir) deleteAlgoBelow(ctx context.Context, version int16) (int64, error) {
	ids, err := d.ids(ctx)
	if err != nil {
		return 0, err
	}
	var n int64
	for _, id := range ids {
		buf, err := d.read(id)
		if errors.Is(err, shared.ErrNotFound) {
			continue
		}
		stale := err != nil
		if !stale {
			h, _, _, decErr := d.decode(buf)
			stale = decErr != nil || h.algo < version
		}
		if stale {
			if rmErr := d.remove(id); rmErr != nil {
				return n, rmErr
			}
			n++
		}
	}
	return n, nil
}

// AudioFingerprintStore keeps Chromaprint fingerprints on disk. It implements
// library.IntroAudioFingerprintCacheRepository.
type AudioFingerprintStore struct{ d fingerprintDir }

// NewAudioFingerprintStore stores under root/audio.
func NewAudioFingerprintStore(root string) *AudioFingerprintStore {
	return &AudioFingerprintStore{d: fingerprintDir{dir: filepath.Join(root, "audio"), magic: audioMagic}}
}

func (s *AudioFingerprintStore) Get(_ context.Context, mediaItemID string) (*library.IntroAudioFingerprintCacheEntry, error) {
	buf, err := s.d.read(mediaItemID)
	if err != nil {
		return nil, err
	}
	h, count, raw, err := s.d.decode(buf)
	if err != nil || len(raw) != 4*count {
		return nil, shared.ErrNotFound
	}
	fp := make([]uint32, count)
	for i := range fp {
		fp[i] = binary.LittleEndian.Uint32(raw[i*4:])
	}
	return &library.IntroAudioFingerprintCacheEntry{
		MediaItemID:     mediaItemID,
		FileFingerprint: h.fileFingerprint,
		WindowSec:       int(h.window),
		Fingerprint:     fp,
		AlgoVersion:     h.algo,
		ExtractedAt:     h.extractedAt,
	}, nil
}

// GetBatch reads each id's file; ids with no usable file are absent.
func (s *AudioFingerprintStore) GetBatch(ctx context.Context, mediaItemIDs []string) (map[string]*library.IntroAudioFingerprintCacheEntry, error) {
	out := make(map[string]*library.IntroAudioFingerprintCacheEntry, len(mediaItemIDs))
	for _, id := range mediaItemIDs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		e, err := s.Get(ctx, id)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				continue
			}
			return nil, err
		}
		out[id] = e
	}
	return out, nil
}

// Upsert stamps ExtractedAt with the current time when it is unset, like the
// database default did.
func (s *AudioFingerprintStore) Upsert(_ context.Context, entry *library.IntroAudioFingerprintCacheEntry) error {
	at := entry.ExtractedAt
	if at.IsZero() {
		at = time.Now()
	}
	if entry.WindowSec < math.MinInt32 || entry.WindowSec > math.MaxInt32 {
		return fmt.Errorf("fingerprint store: window %d out of range", entry.WindowSec)
	}
	raw := make([]byte, 4*len(entry.Fingerprint))
	for i, v := range entry.Fingerprint {
		binary.LittleEndian.PutUint32(raw[i*4:], v)
	}
	blob, err := s.d.encode(fingerprintHeader{
		algo: entry.AlgoVersion, window: int32(entry.WindowSec), extractedAt: at, fileFingerprint: entry.FileFingerprint,
	}, raw, len(entry.Fingerprint))
	if err != nil {
		return err
	}
	return s.d.write(entry.MediaItemID, blob)
}

func (s *AudioFingerprintStore) Delete(_ context.Context, mediaItemID string) error {
	return s.d.remove(mediaItemID)
}

func (s *AudioFingerprintStore) DeleteByAlgoVersionBelow(ctx context.Context, version int16) (int64, error) {
	return s.d.deleteAlgoBelow(ctx, version)
}

// Has reports whether an entry file exists, valid or not.
func (s *AudioFingerprintStore) Has(mediaItemID string) bool { return s.d.has(mediaItemID) }

// MediaItemIDs lists the ids that have an entry file.
func (s *AudioFingerprintStore) MediaItemIDs(ctx context.Context) ([]string, error) {
	return s.d.ids(ctx)
}

// PhashFingerprintStore keeps pHash tile fingerprints on disk. It implements
// library.IntroFingerprintCacheRepository.
type PhashFingerprintStore struct{ d fingerprintDir }

// NewPhashFingerprintStore stores under root/phash.
func NewPhashFingerprintStore(root string) *PhashFingerprintStore {
	return &PhashFingerprintStore{d: fingerprintDir{dir: filepath.Join(root, "phash"), magic: phashMagic}}
}

func (s *PhashFingerprintStore) Get(_ context.Context, mediaItemID string) (*library.IntroFingerprintCacheEntry, error) {
	buf, err := s.d.read(mediaItemID)
	if err != nil {
		return nil, err
	}
	h, count, raw, err := s.d.decode(buf)
	if err != nil || len(raw) != 16*count {
		return nil, shared.ErrNotFound
	}
	tiles := make([]uint64, count)
	ts := make([]int64, count)
	for i := range tiles {
		tiles[i] = binary.LittleEndian.Uint64(raw[i*8:])
		ts[i] = int64(binary.LittleEndian.Uint64(raw[(count+i)*8:])) //nolint:gosec // bit-preserving round trip of an int64
	}
	return &library.IntroFingerprintCacheEntry{
		MediaItemID:     mediaItemID,
		FileFingerprint: h.fileFingerprint,
		PhashTiles:      tiles,
		PhashTimestamps: ts,
		AlgoVersion:     h.algo,
		ExtractedAt:     h.extractedAt,
	}, nil
}

// Upsert stamps ExtractedAt with the current time when it is unset, like the
// database default did.
func (s *PhashFingerprintStore) Upsert(_ context.Context, entry *library.IntroFingerprintCacheEntry) error {
	if len(entry.PhashTiles) != len(entry.PhashTimestamps) {
		return fmt.Errorf("phash_tiles len %d != phash_timestamps len %d", len(entry.PhashTiles), len(entry.PhashTimestamps))
	}
	at := entry.ExtractedAt
	if at.IsZero() {
		at = time.Now()
	}
	n := len(entry.PhashTiles)
	raw := make([]byte, 16*n)
	for i := range n {
		binary.LittleEndian.PutUint64(raw[i*8:], entry.PhashTiles[i])
		binary.LittleEndian.PutUint64(raw[(n+i)*8:], uint64(entry.PhashTimestamps[i])) //nolint:gosec // bit-preserving round trip of an int64
	}
	blob, err := s.d.encode(fingerprintHeader{
		algo: entry.AlgoVersion, extractedAt: at, fileFingerprint: entry.FileFingerprint,
	}, raw, n)
	if err != nil {
		return err
	}
	return s.d.write(entry.MediaItemID, blob)
}

func (s *PhashFingerprintStore) Delete(_ context.Context, mediaItemID string) error {
	return s.d.remove(mediaItemID)
}

func (s *PhashFingerprintStore) DeleteByAlgoVersionBelow(ctx context.Context, version int16) (int64, error) {
	return s.d.deleteAlgoBelow(ctx, version)
}

// Has reports whether an entry file exists, valid or not.
func (s *PhashFingerprintStore) Has(mediaItemID string) bool { return s.d.has(mediaItemID) }

// MediaItemIDs lists the ids that have an entry file.
func (s *PhashFingerprintStore) MediaItemIDs(ctx context.Context) ([]string, error) {
	return s.d.ids(ctx)
}

var (
	_ library.IntroAudioFingerprintCacheRepository = (*AudioFingerprintStore)(nil)
	_ library.IntroFingerprintCacheRepository      = (*PhashFingerprintStore)(nil)
)
