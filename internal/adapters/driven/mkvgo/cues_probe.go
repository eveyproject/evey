package mkvgo

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"

	"github.com/gravity-zero/mkvgo/ebml"
	extmkv "github.com/gravity-zero/mkvgo/mkv"
)

// The Cues-presence probe answers "does this file have a seek index?" from the
// head SeekHead alone — a handful of small reads + one verification seek —
// instead of reader.Open's full parse (which walks every cluster: seconds per
// file, hours over a library, the reason the admin scan needed to go async).
//
// It is deliberately conservative: a positive requires the SeekHead-referenced
// offset to actually hold a Cues element, and anything surprising returns
// "unconfirmed" so the caller falls back to the exact full parse. A confirmed
// negative (a clean head SeekHead with no Cues entry) is trusted: every common
// muxer (mkvmerge, ffmpeg) lists Cues there when it writes them, and the worst
// case of a wrong negative is a repair attempt that re-checks with a full parse
// and skips the file untouched.

// probeSeekHeadHops bounds SeekHead→SeekHead chasing (mkvmerge's two-level
// meta-seek pattern) so a malicious file can't loop the probe.
const probeSeekHeadHops = 2

// probeMaxHeadElements bounds how many top-level elements are walked before
// giving up — head metadata sits within the first few.
const probeMaxHeadElements = 12

// probeMaxSeekHeadBytes caps how much SeekHead payload is read into memory.
const probeMaxSeekHeadBytes = 64 << 10

// headReader returns a buffered reader positioned at pos. The EBML primitives
// read byte-at-a-time; unbuffered that is one syscall per byte, which is what
// made the probe slow on network mounts. One 4 KiB fill covers a whole element
// header (and usually the SeekHead payload too).
func headReader(f *os.File, pos int64) *bufio.Reader {
	return bufio.NewReaderSize(io.NewSectionReader(f, pos, 1<<62), 4<<10)
}

// probeCuesPresence reports (hasCues, confirmed). confirmed == false means the
// probe could not decide and the caller must fall back to a full parse.
func probeCuesPresence(path string) (bool, bool) {
	f, err := os.Open(path)
	if err != nil {
		return false, false
	}
	defer f.Close()

	// EBML header element, then the Segment.
	r := headReader(f, 0)
	eh, n, err := ebml.ReadElementHeader(r)
	if err != nil || eh.ID != 0x1A45DFA3 || eh.Size < 0 {
		return false, false
	}
	pos := int64(n) + eh.Size

	r = headReader(f, pos)
	eh, n, err = ebml.ReadElementHeader(r)
	if err != nil || eh.ID != extmkv.IDSegment {
		return false, false
	}
	segStart := pos + int64(n) // SeekPositions are relative to here

	pos = segStart
	hops := probeSeekHeadHops
	for range probeMaxHeadElements {
		r = headReader(f, pos)
		eh, n, err := ebml.ReadElementHeader(r)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				// Clean end of file without ever meeting a Cues element.
				return false, true
			}
			return false, false
		}
		if eh.Size < 0 {
			// Unknown-size element (streamed cluster) — can't skip past it.
			return false, false
		}
		switch eh.ID {
		case extmkv.IDCues:
			return true, true
		case extmkv.IDCluster:
			// Clusters begin and no head SeekHead listed Cues. Cues could still
			// trail the clusters unlisted — undecidable without a full parse.
			return false, false
		case extmkv.IDSeekHead:
			cuesPos, nextSH, ok := parseSeekHeadPayload(r, eh.Size)
			if !ok {
				return false, false
			}
			if cuesPos >= 0 {
				return verifyElementAt(f, segStart+cuesPos, extmkv.IDCues)
			}
			if nextSH >= 0 && hops > 0 {
				hops--
				pos = segStart + nextSH
				continue
			}
			// A clean SeekHead with no Cues entry — trusted negative.
			return false, true
		}
		pos += int64(n) + eh.Size
	}
	return false, false
}

// parseSeekHeadPayload reads a SeekHead payload (from the reader already
// positioned right after the SeekHead header) and returns the SeekPosition of
// the Cues entry and of a chained SeekHead entry (-1 when absent each).
func parseSeekHeadPayload(src io.Reader, size int64) (cuesPos, nextSeekHead int64, ok bool) {
	cuesPos, nextSeekHead = -1, -1
	if size <= 0 || size > probeMaxSeekHeadBytes {
		return 0, 0, false
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(src, buf); err != nil {
		return 0, 0, false
	}
	r := bytes.NewReader(buf)
	for r.Len() > 0 {
		eh, _, err := ebml.ReadElementHeader(r)
		if err != nil || eh.Size < 0 || eh.Size > int64(r.Len()) {
			return 0, 0, false
		}
		if eh.ID != 0x4DBB { // Seek entry
			if _, err := r.Seek(eh.Size, io.SeekCurrent); err != nil {
				return 0, 0, false
			}
			continue
		}
		entry := make([]byte, eh.Size)
		if _, err := io.ReadFull(r, entry); err != nil {
			return 0, 0, false
		}
		id, sPos, ok := parseSeekEntry(entry)
		if !ok {
			return 0, 0, false
		}
		switch id {
		case extmkv.IDCues:
			cuesPos = sPos
		case extmkv.IDSeekHead:
			nextSeekHead = sPos
		}
	}
	return cuesPos, nextSeekHead, true
}

// parseSeekEntry extracts (SeekID, SeekPosition) from one Seek element payload.
func parseSeekEntry(payload []byte) (id uint64, sPos int64, ok bool) {
	sPos = -1
	r := bytes.NewReader(payload)
	for r.Len() > 0 {
		eh, _, err := ebml.ReadElementHeader(r)
		if err != nil || eh.Size < 0 || eh.Size > int64(r.Len()) {
			return 0, 0, false
		}
		switch eh.ID {
		case 0x53AB: // SeekID — the raw EBML ID bytes of the referenced element
			v, err := ebml.ReadUint(r, eh.Size)
			if err != nil {
				return 0, 0, false
			}
			id = v
		case 0x53AC: // SeekPosition
			v, err := ebml.ReadUint(r, eh.Size)
			if err != nil {
				return 0, 0, false
			}
			sPos = int64(v)
		default:
			if _, err := r.Seek(eh.Size, io.SeekCurrent); err != nil {
				return 0, 0, false
			}
		}
	}
	return id, sPos, sPos >= 0
}

// verifyElementAt confirms the element at abs offset is wantID. A mismatch is
// a stale/odd SeekHead — unconfirmed, so the caller falls back to a full parse.
func verifyElementAt(f *os.File, abs int64, wantID uint32) (bool, bool) {
	if abs < 0 {
		return false, false
	}
	id, _, err := ebml.ReadElementID(headReader(f, abs))
	if err != nil {
		return false, false
	}
	if id != wantID {
		return false, false
	}
	return true, true
}
