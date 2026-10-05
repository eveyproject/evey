package httpapi

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// A download ZIP laid out before a single byte of video is read: stored (no
// compression), ZIP64 on every entry, each CRC in a data descriptor after the
// data. Every offset and the total length are then known from file sizes and
// names alone, so the archive answers Content-Length and Range like a file on
// disk - a browser shows progress and a cut transfer resumes - while nothing
// is ever written to disk. CRCs are computed as the data goes out; a request
// that starts past a file reads only the part it skipped to finish its CRC.

const (
	zipLocalHeaderLen   = 30
	zipLocalExtraLen    = 20 // ZIP64 extra: id, size, uncompressed, compressed
	zipDescriptorLen    = 24 // signature, crc, 8-byte sizes
	zipCentralHeaderLen = 46
	zipCentralExtraLen  = 28 // ZIP64 extra: id, size, uncompressed, compressed, offset
	zipEndLen           = 56 + 20 + 22
	zipFlags            = 0x0808 // data descriptor + UTF-8 names
	zipVersion          = 45     // ZIP64
	zipMax32            = 0xFFFFFFFF
)

type zipStreamFile struct {
	path    string
	name    string
	size    int64
	modTime time.Time
	header  int64 // offset of the local header
	data    int64 // offset of the first data byte
}

type zipLayout struct {
	files   []zipStreamFile
	central int64 // offset of the central directory
	size    int64
	etag    string
}

// newZipLayout stats every entry and places it. An entry whose file cannot be
// stat'ed is left out, as the streaming writer used to skip one it could not open.
func newZipLayout(entries []archiveEntry) (*zipLayout, []error) {
	l := &zipLayout{}
	var skipped []error
	h := sha256.New()
	var off int64
	for _, e := range entries {
		fi, err := os.Stat(e.path)
		if err != nil || !fi.Mode().IsRegular() {
			if err == nil {
				err = fmt.Errorf("%s: not a regular file", e.path)
			}
			skipped = append(skipped, err)
			continue
		}
		f := zipStreamFile{path: e.path, name: e.name, size: fi.Size(), modTime: fi.ModTime(), header: off}
		f.data = off + zipLocalHeaderLen + int64(len(f.name)) + zipLocalExtraLen
		off = f.data + f.size + zipDescriptorLen
		l.files = append(l.files, f)
		fmt.Fprintf(h, "%s\x00%d\x00%d\x00", f.name, f.size, f.modTime.UnixNano())
	}
	l.central = off
	for _, f := range l.files {
		off += zipCentralHeaderLen + int64(len(f.name)) + zipCentralExtraLen
	}
	l.size = off + zipEndLen
	l.etag = `"z-` + hex.EncodeToString(h.Sum(nil)[:12]) + `"`
	return l, skipped
}

func (f *zipStreamFile) dosTime() (date, clock uint16) {
	t := f.modTime.UTC()
	if t.Year() < 1980 {
		t = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	date = uint16(t.Day() + int(t.Month())<<5 + (t.Year()-1980)<<9)
	clock = uint16(t.Second()/2 + t.Minute()<<5 + t.Hour()<<11)
	return date, clock
}

func (f *zipStreamFile) localHeader() []byte {
	b := make([]byte, 0, zipLocalHeaderLen+len(f.name)+zipLocalExtraLen)
	date, clock := f.dosTime()
	b = binary.LittleEndian.AppendUint32(b, 0x04034b50)
	b = binary.LittleEndian.AppendUint16(b, zipVersion)
	b = binary.LittleEndian.AppendUint16(b, zipFlags)
	b = binary.LittleEndian.AppendUint16(b, 0) // stored
	b = binary.LittleEndian.AppendUint16(b, clock)
	b = binary.LittleEndian.AppendUint16(b, date)
	b = binary.LittleEndian.AppendUint32(b, 0) // crc: in the descriptor
	b = binary.LittleEndian.AppendUint32(b, zipMax32)
	b = binary.LittleEndian.AppendUint32(b, zipMax32)
	b = binary.LittleEndian.AppendUint16(b, uint16(len(f.name)))
	b = binary.LittleEndian.AppendUint16(b, zipLocalExtraLen)
	b = append(b, f.name...)
	b = binary.LittleEndian.AppendUint16(b, 0x0001)
	b = binary.LittleEndian.AppendUint16(b, 16)
	b = binary.LittleEndian.AppendUint64(b, 0) // sizes: in the descriptor
	b = binary.LittleEndian.AppendUint64(b, 0)
	return b
}

func (f *zipStreamFile) descriptor(crc uint32) []byte {
	b := make([]byte, 0, zipDescriptorLen)
	b = binary.LittleEndian.AppendUint32(b, 0x08074b50)
	b = binary.LittleEndian.AppendUint32(b, crc)
	b = binary.LittleEndian.AppendUint64(b, uint64(f.size))
	b = binary.LittleEndian.AppendUint64(b, uint64(f.size))
	return b
}

func (l *zipLayout) centralDirectory(crcs []uint32) []byte {
	b := make([]byte, 0, l.size-l.central)
	for i, f := range l.files {
		date, clock := f.dosTime()
		b = binary.LittleEndian.AppendUint32(b, 0x02014b50)
		b = binary.LittleEndian.AppendUint16(b, zipVersion) // made by
		b = binary.LittleEndian.AppendUint16(b, zipVersion) // needed
		b = binary.LittleEndian.AppendUint16(b, zipFlags)
		b = binary.LittleEndian.AppendUint16(b, 0)
		b = binary.LittleEndian.AppendUint16(b, clock)
		b = binary.LittleEndian.AppendUint16(b, date)
		b = binary.LittleEndian.AppendUint32(b, crcs[i])
		b = binary.LittleEndian.AppendUint32(b, zipMax32)
		b = binary.LittleEndian.AppendUint32(b, zipMax32)
		b = binary.LittleEndian.AppendUint16(b, uint16(len(f.name)))
		b = binary.LittleEndian.AppendUint16(b, zipCentralExtraLen)
		b = binary.LittleEndian.AppendUint16(b, 0) // comment
		b = binary.LittleEndian.AppendUint16(b, 0) // disk
		b = binary.LittleEndian.AppendUint16(b, 0) // internal attrs
		b = binary.LittleEndian.AppendUint32(b, 0) // external attrs
		b = binary.LittleEndian.AppendUint32(b, zipMax32)
		b = append(b, f.name...)
		b = binary.LittleEndian.AppendUint16(b, 0x0001)
		b = binary.LittleEndian.AppendUint16(b, 24)
		b = binary.LittleEndian.AppendUint64(b, uint64(f.size))
		b = binary.LittleEndian.AppendUint64(b, uint64(f.size))
		b = binary.LittleEndian.AppendUint64(b, uint64(f.header))
	}
	cdSize := uint64(len(b))
	end64 := uint64(l.central) + cdSize
	n := uint64(len(l.files))
	// ZIP64 end of central directory record.
	b = binary.LittleEndian.AppendUint32(b, 0x06064b50)
	b = binary.LittleEndian.AppendUint64(b, 44)
	b = binary.LittleEndian.AppendUint16(b, zipVersion)
	b = binary.LittleEndian.AppendUint16(b, zipVersion)
	b = binary.LittleEndian.AppendUint32(b, 0)
	b = binary.LittleEndian.AppendUint32(b, 0)
	b = binary.LittleEndian.AppendUint64(b, n)
	b = binary.LittleEndian.AppendUint64(b, n)
	b = binary.LittleEndian.AppendUint64(b, cdSize)
	b = binary.LittleEndian.AppendUint64(b, uint64(l.central))
	// ZIP64 end of central directory locator.
	b = binary.LittleEndian.AppendUint32(b, 0x07064b50)
	b = binary.LittleEndian.AppendUint32(b, 0)
	b = binary.LittleEndian.AppendUint64(b, end64)
	b = binary.LittleEndian.AppendUint32(b, 1)
	// End of central directory record, every field deferring to ZIP64.
	b = binary.LittleEndian.AppendUint32(b, 0x06054b50)
	b = binary.LittleEndian.AppendUint16(b, 0)
	b = binary.LittleEndian.AppendUint16(b, 0)
	b = binary.LittleEndian.AppendUint16(b, 0xFFFF)
	b = binary.LittleEndian.AppendUint16(b, 0xFFFF)
	b = binary.LittleEndian.AppendUint32(b, zipMax32)
	b = binary.LittleEndian.AppendUint32(b, zipMax32)
	b = binary.LittleEndian.AppendUint16(b, 0)
	return b
}

// zipCRCCache keeps finished CRCs across requests, so a resumed transfer does
// not reread the files the cut one already sent. Keyed by path, size and
// modification time: a replaced file gets a new key.
var zipCRCCache = struct {
	sync.Mutex
	m     map[string]uint32
	group singleflight.Group
}{m: map[string]uint32{}}

const zipCRCCacheMax = 20000

func zipCRCKey(f *zipStreamFile) string {
	return f.path + "\x00" + strconv.FormatInt(f.size, 10) + "\x00" + strconv.FormatInt(f.modTime.UnixNano(), 10)
}

func zipCRCCached(f *zipStreamFile) (uint32, bool) {
	zipCRCCache.Lock()
	defer zipCRCCache.Unlock()
	crc, ok := zipCRCCache.m[zipCRCKey(f)]
	return crc, ok
}

func zipCRCStore(f *zipStreamFile, crc uint32) {
	zipCRCCache.Lock()
	defer zipCRCCache.Unlock()
	if len(zipCRCCache.m) >= zipCRCCacheMax {
		clear(zipCRCCache.m)
	}
	zipCRCCache.m[zipCRCKey(f)] = crc
}

type zipFileCRC struct {
	crc    uint32
	hashed int64 // bytes [0, hashed) are in crc
	done   bool
}

// zipStream reads the laid-out archive as one io.ReadSeeker, for
// http.ServeContent.
type zipStream struct {
	l       *zipLayout
	pos     int64
	crcs    []zipFileCRC
	central []byte
	open    *os.File
	openIdx int
}

func newZipStream(l *zipLayout) *zipStream {
	return &zipStream{l: l, crcs: make([]zipFileCRC, len(l.files)), openIdx: -1}
}

func (z *zipStream) Close() error {
	if z.open != nil {
		err := z.open.Close()
		z.open, z.openIdx = nil, -1
		return err
	}
	return nil
}

func (z *zipStream) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += z.pos
	case io.SeekEnd:
		offset += z.l.size
	default:
		return 0, errors.New("zipstream: bad whence")
	}
	if offset < 0 {
		return 0, errors.New("zipstream: negative position")
	}
	z.pos = offset
	return offset, nil
}

func (z *zipStream) Read(p []byte) (int, error) {
	if z.pos >= z.l.size {
		return 0, io.EOF
	}
	if z.pos >= z.l.central {
		if z.central == nil {
			crcs := make([]uint32, len(z.l.files))
			for i := range z.l.files {
				crc, err := z.crcOf(i)
				if err != nil {
					return 0, err
				}
				crcs[i] = crc
			}
			z.central = z.l.centralDirectory(crcs)
		}
		n := copy(p, z.central[z.pos-z.l.central:])
		z.pos += int64(n)
		return n, nil
	}
	// The last file whose local header starts at or before pos.
	i := sort.Search(len(z.l.files), func(i int) bool { return z.l.files[i].header > z.pos }) - 1
	f := &z.l.files[i]
	switch {
	case z.pos < f.data:
		n := copy(p, f.localHeader()[z.pos-f.header:])
		z.pos += int64(n)
		return n, nil
	case z.pos < f.data+f.size:
		off := z.pos - f.data
		if rest := f.size - off; int64(len(p)) > rest {
			p = p[:rest]
		}
		n, err := z.readData(i, p, off)
		z.pos += int64(n)
		return n, err
	default:
		crc, err := z.crcOf(i)
		if err != nil {
			return 0, err
		}
		n := copy(p, f.descriptor(crc)[z.pos-f.data-f.size:])
		z.pos += int64(n)
		return n, nil
	}
}

func (z *zipStream) file(i int) (*os.File, error) {
	if z.openIdx == i {
		return z.open, nil
	}
	_ = z.Close() //nolint:errcheck // read-only file handle
	fh, err := os.Open(z.l.files[i].path)
	if err != nil {
		return nil, err
	}
	z.open, z.openIdx = fh, i
	return fh, nil
}

func (z *zipStream) readData(i int, p []byte, off int64) (int, error) {
	fh, err := z.file(i)
	if err != nil {
		return 0, err
	}
	n, err := fh.ReadAt(p, off)
	if n < len(p) {
		// The file shrank since the layout was made: the length already sent
		// is a promise this archive can no longer keep.
		if err == nil || errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return n, fmt.Errorf("zipstream: %s: %w", z.l.files[i].path, err)
	}
	st := &z.crcs[i]
	if !st.done && off > st.hashed {
		if err := z.hashUpTo(i, off); err != nil {
			return 0, err
		}
	}
	if !st.done && off == st.hashed {
		st.crc = crc32.Update(st.crc, crc32.IEEETable, p[:n])
		st.hashed += int64(n)
		if st.hashed == z.l.files[i].size {
			st.done = true
			zipCRCStore(&z.l.files[i], st.crc)
		}
	}
	return n, nil
}

// hashUpTo extends file i's running CRC to byte end by reading the gap.
func (z *zipStream) hashUpTo(i int, end int64) error {
	st := &z.crcs[i]
	fh, err := z.file(i)
	if err != nil {
		return err
	}
	// crc32.Update continues a running CRC: seed it with what is hashed.
	sr := io.NewSectionReader(fh, st.hashed, end-st.hashed)
	buf := make([]byte, 1<<20)
	crc, read := st.crc, int64(0)
	for {
		n, err := sr.Read(buf)
		crc = crc32.Update(crc, crc32.IEEETable, buf[:n])
		read += int64(n)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	if read != end-st.hashed {
		return io.ErrUnexpectedEOF
	}
	st.crc, st.hashed = crc, end
	if end == z.l.files[i].size {
		st.done = true
		zipCRCStore(&z.l.files[i], crc)
	}
	return nil
}

// crcOf returns file i's CRC, from this stream, the cache, or by reading
// whatever this stream has not hashed yet. Concurrent requests needing the
// same file (a download manager fetching the tail in parallel) read it once.
func (z *zipStream) crcOf(i int) (uint32, error) {
	st := &z.crcs[i]
	if st.done {
		return st.crc, nil
	}
	f := &z.l.files[i]
	if crc, ok := zipCRCCached(f); ok {
		st.crc, st.done = crc, true
		return crc, nil
	}
	if st.hashed > 0 {
		// This stream already holds a prefix: read only the rest.
		if err := z.hashUpTo(i, f.size); err != nil {
			return 0, fmt.Errorf("zipstream: crc %s: %w", f.path, err)
		}
		return st.crc, nil
	}
	v, err, _ := zipCRCCache.group.Do(zipCRCKey(f), func() (any, error) {
		if crc, ok := zipCRCCached(f); ok {
			return crc, nil
		}
		fh, err := os.Open(f.path)
		if err != nil {
			return uint32(0), err
		}
		defer fh.Close()
		h := crc32.NewIEEE()
		n, err := io.Copy(h, io.NewSectionReader(fh, 0, f.size))
		if err != nil {
			return uint32(0), err
		}
		if n != f.size {
			return uint32(0), io.ErrUnexpectedEOF
		}
		crc := h.Sum32()
		zipCRCStore(f, crc)
		return crc, nil
	})
	if err != nil {
		return 0, fmt.Errorf("zipstream: crc %s: %w", f.path, err)
	}
	st.crc, st.done = v.(uint32), true
	return st.crc, nil
}
