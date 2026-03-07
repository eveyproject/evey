package ebml

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/gravity-zero/evey/internal/domain/mkv"
)

type Reader struct{}

func NewReader() *Reader {
	return &Reader{}
}

func (r *Reader) Open(ctx context.Context, path string) (*mkv.Container, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	p := &parser{r: f}
	c := &mkv.Container{Path: path}

	if err := p.parseEBMLHeader(); err != nil {
		return nil, fmt.Errorf("ebml header: %w", err)
	}

	if err := p.parseSegment(ctx, c); err != nil {
		return nil, fmt.Errorf("segment: %w", err)
	}

	if c.Info.Duration > 0 && c.Info.TimecodeScale > 0 {
		c.DurationMs = int64(c.Info.Duration * float64(c.Info.TimecodeScale) / 1e6)
	}

	return c, nil
}

func (r *Reader) ReadTracks(ctx context.Context, path string) ([]mkv.Track, error) {
	c, err := r.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.Tracks, nil
}

func (r *Reader) ReadChapters(ctx context.Context, path string) ([]mkv.Chapter, error) {
	c, err := r.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.Chapters, nil
}

func (r *Reader) ReadAttachments(ctx context.Context, path string) ([]mkv.Attachment, error) {
	c, err := r.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.Attachments, nil
}

func (r *Reader) ReadTags(ctx context.Context, path string) ([]mkv.Tag, error) {
	c, err := r.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.Tags, nil
}

// ReadVINT reads a variable-length integer from r. Returns the value and total bytes consumed.
func ReadVINT(r io.Reader) (uint64, int, error) {
	var first [1]byte
	if _, err := io.ReadFull(r, first[:]); err != nil {
		return 0, 0, err
	}
	b := first[0]
	if b == 0 {
		return 0, 0, fmt.Errorf("invalid VINT: leading zero byte")
	}

	width := 1
	for i := 7; i >= 0; i-- {
		if b&(1<<uint(i)) != 0 {
			width = 8 - i
			break
		}
	}

	val := uint64(b)
	if width > 1 {
		rest := make([]byte, width-1)
		if _, err := io.ReadFull(r, rest); err != nil {
			return 0, 0, err
		}
		for _, rb := range rest {
			val = (val << 8) | uint64(rb)
		}
	}

	return val, width, nil
}

// ReadElementID reads an EBML element ID
func ReadElementID(r io.Reader) (uint32, int, error) {
	val, n, err := ReadVINT(r)
	if err != nil {
		return 0, 0, err
	}
	return uint32(val), n, nil
}

// ReadDataSize reads an EBML data size (strips the VINT marker bit)
func ReadDataSize(r io.Reader) (int64, int, error) {
	val, n, err := ReadVINT(r)
	if err != nil {
		return 0, 0, err
	}
	// Strip the leading marker bit
	mask := uint64(1) << uint(n*7)
	size := int64(val & ^mask)
	// All-ones means unknown size
	if val == mask|(mask-1) {
		return -1, n, nil
	}
	return size, n, nil
}

type parser struct {
	r io.ReadSeeker
}

type elementHeader struct {
	id   uint32
	size int64
}

func (p *parser) readHeader() (elementHeader, int, error) {
	id, n1, err := ReadElementID(p.r)
	if err != nil {
		return elementHeader{}, 0, err
	}
	size, n2, err := ReadDataSize(p.r)
	if err != nil {
		return elementHeader{}, 0, err
	}
	return elementHeader{id: id, size: size}, n1 + n2, nil
}

func (p *parser) readUint(size int64) (uint64, error) {
	buf := make([]byte, size)
	if _, err := io.ReadFull(p.r, buf); err != nil {
		return 0, err
	}
	var val uint64
	for _, b := range buf {
		val = (val << 8) | uint64(b)
	}
	return val, nil
}

func (p *parser) readFloat(size int64) (float64, error) {
	if size == 4 {
		buf := make([]byte, 4)
		if _, err := io.ReadFull(p.r, buf); err != nil {
			return 0, err
		}
		bits := binary.BigEndian.Uint32(buf)
		return float64(math.Float32frombits(bits)), nil
	}
	buf := make([]byte, 8)
	if _, err := io.ReadFull(p.r, buf); err != nil {
		return 0, err
	}
	bits := binary.BigEndian.Uint64(buf)
	return math.Float64frombits(bits), nil
}

func (p *parser) readString(size int64) (string, error) {
	buf := make([]byte, size)
	if _, err := io.ReadFull(p.r, buf); err != nil {
		return "", err
	}
	// Trim null bytes
	for len(buf) > 0 && buf[len(buf)-1] == 0 {
		buf = buf[:len(buf)-1]
	}
	return string(buf), nil
}

func (p *parser) readBytes(size int64) ([]byte, error) {
	buf := make([]byte, size)
	if _, err := io.ReadFull(p.r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func (p *parser) skip(size int64) error {
	_, err := p.r.Seek(size, io.SeekCurrent)
	return err
}

func (p *parser) parseEBMLHeader() error {
	h, _, err := p.readHeader()
	if err != nil {
		return err
	}
	if h.id != IDEBMLHeader {
		return fmt.Errorf("expected EBML header (0x%X), got 0x%X", IDEBMLHeader, h.id)
	}
	return p.skip(h.size)
}

func (p *parser) parseSegment(ctx context.Context, c *mkv.Container) error {
	h, _, err := p.readHeader()
	if err != nil {
		return err
	}
	if h.id != IDSegment {
		return fmt.Errorf("expected Segment (0x%X), got 0x%X", IDSegment, h.id)
	}

	var endPos int64 = -1
	if h.size >= 0 {
		cur, _ := p.r.Seek(0, io.SeekCurrent)
		endPos = cur + h.size
	}

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if endPos >= 0 {
			cur, _ := p.r.Seek(0, io.SeekCurrent)
			if cur >= endPos {
				break
			}
		}

		eh, _, err := p.readHeader()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch eh.id {
		case IDInfo:
			if err := p.parseInfo(eh.size, c); err != nil {
				return err
			}
		case IDTracks:
			if err := p.parseTracks(eh.size, c); err != nil {
				return err
			}
		case IDChapters:
			if err := p.parseChapters(eh.size, c); err != nil {
				return err
			}
		case IDAttachments:
			if err := p.parseAttachments(eh.size, c); err != nil {
				return err
			}
		case IDTags:
			if err := p.parseTags(eh.size, c); err != nil {
				return err
			}
		default:
			if eh.size < 0 {
				return fmt.Errorf("unknown-size element 0x%X cannot be skipped", eh.id)
			}
			if err := p.skip(eh.size); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *parser) parseInfo(size int64, c *mkv.Container) error {
	cur, _ := p.r.Seek(0, io.SeekCurrent)
	end := cur + size
	c.Info.TimecodeScale = 1000000 // default

	for {
		pos, _ := p.r.Seek(0, io.SeekCurrent)
		if pos >= end {
			break
		}
		eh, _, err := p.readHeader()
		if err != nil {
			return err
		}
		switch eh.id {
		case IDTimecodeScale:
			v, err := p.readUint(eh.size)
			if err != nil {
				return err
			}
			c.Info.TimecodeScale = int64(v)
		case IDDuration:
			v, err := p.readFloat(eh.size)
			if err != nil {
				return err
			}
			c.Info.Duration = v
		case IDMuxingApp:
			v, err := p.readString(eh.size)
			if err != nil {
				return err
			}
			c.Info.MuxingApp = v
		case IDWritingApp:
			v, err := p.readString(eh.size)
			if err != nil {
				return err
			}
			c.Info.WritingApp = v
		case IDTitle:
			v, err := p.readString(eh.size)
			if err != nil {
				return err
			}
			c.Info.Title = v
		case IDDateUTC:
			v, err := p.readUint(eh.size)
			if err != nil {
				return err
			}
			// Matroska epoch: 2001-01-01T00:00:00 UTC
			epoch := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
			t := epoch.Add(time.Duration(int64(v)))
			c.Info.DateUTC = &t
		default:
			if err := p.skip(eh.size); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *parser) parseTracks(size int64, c *mkv.Container) error {
	cur, _ := p.r.Seek(0, io.SeekCurrent)
	end := cur + size

	for {
		pos, _ := p.r.Seek(0, io.SeekCurrent)
		if pos >= end {
			break
		}
		eh, _, err := p.readHeader()
		if err != nil {
			return err
		}
		if eh.id == IDTrackEntry {
			track, err := p.parseTrackEntry(eh.size)
			if err != nil {
				return err
			}
			c.Tracks = append(c.Tracks, track)
		} else {
			if err := p.skip(eh.size); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *parser) parseTrackEntry(size int64) (mkv.Track, error) {
	cur, _ := p.r.Seek(0, io.SeekCurrent)
	end := cur + size
	t := mkv.Track{Language: "eng", IsDefault: true}

	for {
		pos, _ := p.r.Seek(0, io.SeekCurrent)
		if pos >= end {
			break
		}
		eh, _, err := p.readHeader()
		if err != nil {
			return t, err
		}
		switch eh.id {
		case IDTrackNumber:
			v, err := p.readUint(eh.size)
			if err != nil {
				return t, err
			}
			t.ID = v
		case IDTrackType:
			v, err := p.readUint(eh.size)
			if err != nil {
				return t, err
			}
			switch v {
			case TrackTypeVideo:
				t.Type = mkv.VideoTrack
			case TrackTypeAudio:
				t.Type = mkv.AudioTrack
			case TrackTypeSubtitle:
				t.Type = mkv.SubtitleTrack
			}
		case IDCodecID:
			v, err := p.readString(eh.size)
			if err != nil {
				return t, err
			}
			if short, ok := codecShortName[v]; ok {
				t.Codec = short
			} else {
				t.Codec = v
			}
		case IDLanguage:
			v, err := p.readString(eh.size)
			if err != nil {
				return t, err
			}
			t.Language = v
		case IDName:
			v, err := p.readString(eh.size)
			if err != nil {
				return t, err
			}
			t.Name = v
		case IDFlagDefault:
			v, err := p.readUint(eh.size)
			if err != nil {
				return t, err
			}
			t.IsDefault = v == 1
		case IDFlagForced:
			v, err := p.readUint(eh.size)
			if err != nil {
				return t, err
			}
			t.IsForced = v == 1
		case IDVideo:
			if err := p.parseVideoSettings(eh.size, &t); err != nil {
				return t, err
			}
		case IDAudio:
			if err := p.parseAudioSettings(eh.size, &t); err != nil {
				return t, err
			}
		default:
			if err := p.skip(eh.size); err != nil {
				return t, err
			}
		}
	}
	return t, nil
}

func (p *parser) parseVideoSettings(size int64, t *mkv.Track) error {
	cur, _ := p.r.Seek(0, io.SeekCurrent)
	end := cur + size

	for {
		pos, _ := p.r.Seek(0, io.SeekCurrent)
		if pos >= end {
			break
		}
		eh, _, err := p.readHeader()
		if err != nil {
			return err
		}
		switch eh.id {
		case IDPixelWidth:
			v, err := p.readUint(eh.size)
			if err != nil {
				return err
			}
			w := uint32(v)
			t.Width = &w
		case IDPixelHeight:
			v, err := p.readUint(eh.size)
			if err != nil {
				return err
			}
			h := uint32(v)
			t.Height = &h
		default:
			if err := p.skip(eh.size); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *parser) parseAudioSettings(size int64, t *mkv.Track) error {
	cur, _ := p.r.Seek(0, io.SeekCurrent)
	end := cur + size

	for {
		pos, _ := p.r.Seek(0, io.SeekCurrent)
		if pos >= end {
			break
		}
		eh, _, err := p.readHeader()
		if err != nil {
			return err
		}
		switch eh.id {
		case IDSamplingFreq:
			v, err := p.readFloat(eh.size)
			if err != nil {
				return err
			}
			t.SampleRate = &v
		case IDChannels:
			v, err := p.readUint(eh.size)
			if err != nil {
				return err
			}
			ch := uint8(v)
			t.Channels = &ch
		case IDBitDepth:
			v, err := p.readUint(eh.size)
			if err != nil {
				return err
			}
			bd := uint8(v)
			t.BitDepth = &bd
		default:
			if err := p.skip(eh.size); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *parser) parseChapters(size int64, c *mkv.Container) error {
	cur, _ := p.r.Seek(0, io.SeekCurrent)
	end := cur + size

	for {
		pos, _ := p.r.Seek(0, io.SeekCurrent)
		if pos >= end {
			break
		}
		eh, _, err := p.readHeader()
		if err != nil {
			return err
		}
		if eh.id == IDEditionEntry {
			if err := p.parseEditionEntry(eh.size, c); err != nil {
				return err
			}
		} else {
			if err := p.skip(eh.size); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *parser) parseEditionEntry(size int64, c *mkv.Container) error {
	cur, _ := p.r.Seek(0, io.SeekCurrent)
	end := cur + size

	for {
		pos, _ := p.r.Seek(0, io.SeekCurrent)
		if pos >= end {
			break
		}
		eh, _, err := p.readHeader()
		if err != nil {
			return err
		}
		if eh.id == IDChapterAtom {
			ch, err := p.parseChapterAtom(eh.size)
			if err != nil {
				return err
			}
			c.Chapters = append(c.Chapters, ch)
		} else {
			if err := p.skip(eh.size); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *parser) parseChapterAtom(size int64) (mkv.Chapter, error) {
	cur, _ := p.r.Seek(0, io.SeekCurrent)
	end := cur + size
	ch := mkv.Chapter{}

	for {
		pos, _ := p.r.Seek(0, io.SeekCurrent)
		if pos >= end {
			break
		}
		eh, _, err := p.readHeader()
		if err != nil {
			return ch, err
		}
		switch eh.id {
		case IDChapterUID:
			v, err := p.readUint(eh.size)
			if err != nil {
				return ch, err
			}
			ch.ID = v
		case IDChapterTimeStart:
			v, err := p.readUint(eh.size)
			if err != nil {
				return ch, err
			}
			ch.StartMs = int64(v / 1000000) // ns to ms
		case IDChapterTimeEnd:
			v, err := p.readUint(eh.size)
			if err != nil {
				return ch, err
			}
			ch.EndMs = int64(v / 1000000)
		case IDChapterDisplay:
			if err := p.parseChapterDisplay(eh.size, &ch); err != nil {
				return ch, err
			}
		default:
			if err := p.skip(eh.size); err != nil {
				return ch, err
			}
		}
	}
	return ch, nil
}

func (p *parser) parseChapterDisplay(size int64, ch *mkv.Chapter) error {
	cur, _ := p.r.Seek(0, io.SeekCurrent)
	end := cur + size

	for {
		pos, _ := p.r.Seek(0, io.SeekCurrent)
		if pos >= end {
			break
		}
		eh, _, err := p.readHeader()
		if err != nil {
			return err
		}
		if eh.id == IDChapString {
			v, err := p.readString(eh.size)
			if err != nil {
				return err
			}
			ch.Title = v
		} else {
			if err := p.skip(eh.size); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *parser) parseAttachments(size int64, c *mkv.Container) error {
	cur, _ := p.r.Seek(0, io.SeekCurrent)
	end := cur + size

	for {
		pos, _ := p.r.Seek(0, io.SeekCurrent)
		if pos >= end {
			break
		}
		eh, _, err := p.readHeader()
		if err != nil {
			return err
		}
		if eh.id == IDAttachedFile {
			att, err := p.parseAttachedFile(eh.size)
			if err != nil {
				return err
			}
			c.Attachments = append(c.Attachments, att)
		} else {
			if err := p.skip(eh.size); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *parser) parseAttachedFile(size int64) (mkv.Attachment, error) {
	cur, _ := p.r.Seek(0, io.SeekCurrent)
	end := cur + size
	att := mkv.Attachment{}

	for {
		pos, _ := p.r.Seek(0, io.SeekCurrent)
		if pos >= end {
			break
		}
		eh, _, err := p.readHeader()
		if err != nil {
			return att, err
		}
		switch eh.id {
		case IDFileUID:
			v, err := p.readUint(eh.size)
			if err != nil {
				return att, err
			}
			att.ID = v
		case IDFileName:
			v, err := p.readString(eh.size)
			if err != nil {
				return att, err
			}
			att.Name = v
		case IDFileMimeType:
			v, err := p.readString(eh.size)
			if err != nil {
				return att, err
			}
			att.MIMEType = v
		case IDFileData:
			data, err := p.readBytes(eh.size)
			if err != nil {
				return att, err
			}
			att.Data = data
			att.Size = eh.size
		default:
			if err := p.skip(eh.size); err != nil {
				return att, err
			}
		}
	}
	return att, nil
}

func (p *parser) parseTags(size int64, c *mkv.Container) error {
	cur, _ := p.r.Seek(0, io.SeekCurrent)
	end := cur + size

	for {
		pos, _ := p.r.Seek(0, io.SeekCurrent)
		if pos >= end {
			break
		}
		eh, _, err := p.readHeader()
		if err != nil {
			return err
		}
		if eh.id == IDTag {
			tag, err := p.parseTag(eh.size)
			if err != nil {
				return err
			}
			c.Tags = append(c.Tags, tag)
		} else {
			if err := p.skip(eh.size); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *parser) parseTag(size int64) (mkv.Tag, error) {
	cur, _ := p.r.Seek(0, io.SeekCurrent)
	end := cur + size
	tag := mkv.Tag{}

	for {
		pos, _ := p.r.Seek(0, io.SeekCurrent)
		if pos >= end {
			break
		}
		eh, _, err := p.readHeader()
		if err != nil {
			return tag, err
		}
		switch eh.id {
		case IDTargets:
			if err := p.parseTargets(eh.size, &tag); err != nil {
				return tag, err
			}
		case IDSimpleTag:
			st, err := p.parseSimpleTag(eh.size)
			if err != nil {
				return tag, err
			}
			tag.SimpleTag = append(tag.SimpleTag, st)
		default:
			if err := p.skip(eh.size); err != nil {
				return tag, err
			}
		}
	}
	return tag, nil
}

func (p *parser) parseTargets(size int64, tag *mkv.Tag) error {
	cur, _ := p.r.Seek(0, io.SeekCurrent)
	end := cur + size

	for {
		pos, _ := p.r.Seek(0, io.SeekCurrent)
		if pos >= end {
			break
		}
		eh, _, err := p.readHeader()
		if err != nil {
			return err
		}
		switch eh.id {
		case IDTargetType:
			v, err := p.readString(eh.size)
			if err != nil {
				return err
			}
			tag.TargetType = v
		case IDTagTrackUID:
			v, err := p.readUint(eh.size)
			if err != nil {
				return err
			}
			tag.TargetID = v
		default:
			if err := p.skip(eh.size); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *parser) parseSimpleTag(size int64) (mkv.SimpleTag, error) {
	cur, _ := p.r.Seek(0, io.SeekCurrent)
	end := cur + size
	st := mkv.SimpleTag{}

	for {
		pos, _ := p.r.Seek(0, io.SeekCurrent)
		if pos >= end {
			break
		}
		eh, _, err := p.readHeader()
		if err != nil {
			return st, err
		}
		switch eh.id {
		case IDTagName:
			v, err := p.readString(eh.size)
			if err != nil {
				return st, err
			}
			st.Name = v
		case IDTagString:
			v, err := p.readString(eh.size)
			if err != nil {
				return st, err
			}
			st.Value = v
		case IDTagLanguage:
			v, err := p.readString(eh.size)
			if err != nil {
				return st, err
			}
			st.Language = v
		default:
			if err := p.skip(eh.size); err != nil {
				return st, err
			}
		}
	}
	return st, nil
}
