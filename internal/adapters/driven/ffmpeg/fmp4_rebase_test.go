package ffmpeg

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func be32(v uint32) []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, v); return b }
func be64(v uint64) []byte { b := make([]byte, 8); binary.BigEndian.PutUint64(b, v); return b }

func box(typ string, payload ...[]byte) []byte {
	body := bytes.Join(payload, nil)
	out := make([]byte, 0, 8+len(body))
	out = append(out, be32(uint32(8+len(body)))...)
	out = append(out, typ...)
	return append(out, body...)
}

// fullbox prepends version+flags to the payload.
func fullbox(typ string, ver byte, payload ...[]byte) []byte {
	vf := []byte{ver, 0, 0, 0}
	return box(typ, append([][]byte{vf}, payload...)...)
}

func writeTmp(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "seg.m4s")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func synthSegment(tfdtVer byte, tfdtVal uint64) []byte {
	sidx := fullbox("sidx", 0,
		be32(1),    // reference_ID
		be32(1000), // timescale
		be32(5000), // earliest_presentation_time (v0)
		be32(0),    // first_offset
		be32(0),    // reserved(2)+reference_count(2)
	)
	tfhd := fullbox("tfhd", 0, be32(1)) // track_ID 1
	var tfdt []byte
	if tfdtVer == 1 {
		tfdt = fullbox("tfdt", 1, be64(tfdtVal))
	} else {
		tfdt = fullbox("tfdt", 0, be32(uint32(tfdtVal)))
	}
	moof := box("moof", box("traf", tfhd, tfdt))
	return bytes.Join([][]byte{box("styp", []byte("msdh")), sidx, moof, box("mdat")}, nil)
}

func readSidxEPT(t *testing.T, p string) uint32 {
	t.Helper()
	buf, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	i := bytes.Index(buf, []byte("sidx"))
	return binary.BigEndian.Uint32(buf[i+16 : i+20]) // +4 verflags +4 refID +4 ts
}

func readTfdt(t *testing.T, p string) uint64 {
	t.Helper()
	buf, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	i := bytes.Index(buf, []byte("tfdt"))
	if buf[i+4] == 1 {
		return binary.BigEndian.Uint64(buf[i+8 : i+16])
	}
	return uint64(binary.BigEndian.Uint32(buf[i+8 : i+12]))
}

func TestRebaseShiftsTfdtAndSidx(t *testing.T) {
	p := writeTmp(t, synthSegment(1, 5000))
	// 2500 ms at timescale 1000 → +2500 units
	if err := rebaseFMP4Segment(p, 2500, nil); err != nil {
		t.Fatal(err)
	}
	if got := readSidxEPT(t, p); got != 7500 {
		t.Fatalf("sidx EPT = %d, want 7500", got)
	}
	if got := readTfdt(t, p); got != 7500 {
		t.Fatalf("tfdt = %d, want 7500", got)
	}
}

func TestRebaseV0OverflowLeavesFileUntouched(t *testing.T) {
	orig := synthSegment(0, 0xFFFFFF00)
	p := writeTmp(t, orig)
	err := rebaseFMP4Segment(p, 2500, nil)
	if err == nil {
		t.Fatal("want overflow error")
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(orig, after) {
		t.Fatal("file modified despite error (compute-first-write-last broken)")
	}
}

func TestRebaseInitFallbackWhenNoSidx(t *testing.T) {
	// segment without sidx: timescale must come from the init map
	tfhd := fullbox("tfhd", 0, be32(7))
	tfdt := fullbox("tfdt", 1, be64(1000))
	seg := bytes.Join([][]byte{box("styp", []byte("msdh")), box("moof", box("traf", tfhd, tfdt)), box("mdat")}, nil)
	p := writeTmp(t, seg)

	if err := rebaseFMP4Segment(p, 1000, nil); err == nil {
		t.Fatal("want error without any timescale source")
	}
	if err := rebaseFMP4Segment(p, 1000, map[uint32]uint32{7: 90000}); err != nil {
		t.Fatal(err)
	}
	if got := readTfdt(t, p); got != 1000+90000 {
		t.Fatalf("tfdt = %d, want %d", got, 1000+90000)
	}
}

func TestParseInitTimescales(t *testing.T) {
	tkhd := fullbox("tkhd", 0, be32(0), be32(0), be32(3)) // creation, modification, track_ID
	mdhd := fullbox("mdhd", 0, be32(0), be32(0), be32(48000), be32(0))
	moov := box("moov", box("trak", tkhd, box("mdia", mdhd)))
	p := writeTmp(t, bytes.Join([][]byte{box("ftyp", []byte("iso6")), moov}, nil))

	ts, err := parseInitTimescales(p)
	if err != nil {
		t.Fatal(err)
	}
	if ts[3] != 48000 {
		t.Fatalf("timescale track3 = %d, want 48000", ts[3])
	}
}

func TestFmp4CompleteMoov(t *testing.T) {
	moov := box("moov", box("trak"))
	full := bytes.Join([][]byte{box("ftyp", []byte("iso6")), moov}, nil)
	p := writeTmp(t, full)
	if !fmp4CompleteMoov(p) {
		t.Fatal("complete init reported incomplete")
	}
	if err := os.WriteFile(p, full[:len(full)-5], 0o644); err != nil {
		t.Fatal(err)
	}
	if fmp4CompleteMoov(p) {
		t.Fatal("truncated init reported complete")
	}
}
