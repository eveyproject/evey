package mkvgo

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gravity-zero/mkvgo/ebml"
	extmkv "github.com/gravity-zero/mkvgo/mkv"
)

// Synthetic EBML fixtures: just enough structure for the head probe. The
// cluster/cues payloads are garbage on purpose — a full reader.Open would
// choke on them, which is how these tests prove the fast path was taken.

func elem(t *testing.T, id uint32, payload []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	if _, err := ebml.WriteElementHeader(&b, id, int64(len(payload))); err != nil {
		t.Fatalf("write element header: %v", err)
	}
	b.Write(payload)
	return b.Bytes()
}

func uintElem(t *testing.T, id uint32, val uint64, n int) []byte {
	t.Helper()
	var p bytes.Buffer
	if _, err := ebml.WriteUint(&p, val, n); err != nil {
		t.Fatalf("write uint: %v", err)
	}
	return elem(t, id, p.Bytes())
}

// seekEntry builds a Seek element pointing targetID at pos. Fixed-width fields
// keep the SeekHead length independent of the position value, so offsets can
// be computed with a placeholder first.
func seekEntry(t *testing.T, targetID uint64, pos uint64) []byte {
	t.Helper()
	payload := append(uintElem(t, 0x53AB, targetID, 4), uintElem(t, 0x53AC, pos, 8)...)
	return elem(t, 0x4DBB, payload)
}

func writeFixture(t *testing.T, name string, segChildren ...[]byte) string {
	t.Helper()
	var seg bytes.Buffer
	for _, c := range segChildren {
		seg.Write(c)
	}
	file := append(elem(t, 0x1A45DFA3, nil), elem(t, extmkv.IDSegment, seg.Bytes())...)
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, file, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHasSeekIndex_FastPath_SeekHeadListsCues(t *testing.T) {
	void := elem(t, extmkv.IDVoid, make([]byte, 32))
	cues := elem(t, extmkv.IDCues, []byte("not real cue entries"))
	// Fixed-width entries keep the SeekHead length stable across rewrites.
	placeholder := elem(t, extmkv.IDSeekHead, seekEntry(t, extmkv.IDCues, 0))
	cuesPos := uint64(len(placeholder) + len(void))
	sh := elem(t, extmkv.IDSeekHead, seekEntry(t, extmkv.IDCues, cuesPos))
	path := writeFixture(t, "indexed.mkv", sh, void, cues)

	has, err := New().HasSeekIndex(context.Background(), path)
	if err != nil {
		t.Fatalf("HasSeekIndex (fast path should never full-parse this file): %v", err)
	}
	if !has {
		t.Error("SeekHead lists a verified Cues element → must report indexed")
	}
}

func TestHasSeekIndex_FastPath_SeekHeadWithoutCuesEntry(t *testing.T) {
	// A clean head SeekHead referencing only Info, then a garbage cluster a
	// full parse would reject — the trusted negative must come from the probe.
	sh := elem(t, extmkv.IDSeekHead, seekEntry(t, extmkv.IDInfo, 0))
	cluster := elem(t, extmkv.IDCluster, []byte("garbage blocks a full parse would reject"))
	path := writeFixture(t, "unindexed.mkv", sh, cluster)

	has, err := New().HasSeekIndex(context.Background(), path)
	if err != nil {
		t.Fatalf("HasSeekIndex (probe must confirm the negative, no full parse): %v", err)
	}
	if has {
		t.Error("no Cues entry in the head SeekHead → must report un-indexed")
	}
}

func TestHasSeekIndex_FastPath_CuesAtHead(t *testing.T) {
	// mkvmerge --cues-before-clusters: the Cues element itself sits at the head.
	cues := elem(t, extmkv.IDCues, []byte("cues first"))
	cluster := elem(t, extmkv.IDCluster, []byte("garbage"))
	path := writeFixture(t, "cueshead.mkv", cues, cluster)

	has, err := New().HasSeekIndex(context.Background(), path)
	if err != nil {
		t.Fatalf("HasSeekIndex: %v", err)
	}
	if !has {
		t.Error("a head Cues element must report indexed")
	}
}

func TestProbeCuesPresence_Unconfirmed_FallsBackCases(t *testing.T) {
	// (1) No SeekHead, clusters start immediately → undecidable at the head.
	cluster := elem(t, extmkv.IDCluster, []byte("opaque"))
	noSH := writeFixture(t, "nosh.mkv", cluster)
	if _, confirmed := probeCuesPresence(noSH); confirmed {
		t.Error("clusters without a head SeekHead must be unconfirmed (full-parse fallback)")
	}

	// (2) Stale SeekHead: the Cues entry points at a Void element.
	void := elem(t, extmkv.IDVoid, make([]byte, 16))
	placeholder := elem(t, extmkv.IDSeekHead, seekEntry(t, extmkv.IDCues, 0))
	sh := elem(t, extmkv.IDSeekHead, seekEntry(t, extmkv.IDCues, uint64(len(placeholder))))
	stale := writeFixture(t, "stale.mkv", sh, void)
	if _, confirmed := probeCuesPresence(stale); confirmed {
		t.Error("a SeekHead pointing Cues at a non-Cues element must be unconfirmed")
	}

	// (3) Not an EBML file at all.
	garbage := filepath.Join(t.TempDir(), "g.mkv")
	if err := os.WriteFile(garbage, []byte("random bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, confirmed := probeCuesPresence(garbage); confirmed {
		t.Error("garbage must be unconfirmed")
	}
}

func TestProbeCuesPresence_TwoLevelSeekHead(t *testing.T) {
	// Head SeekHead lists only a second SeekHead, which lists the Cues —
	// the mkvmerge two-level meta-seek pattern.
	void := elem(t, extmkv.IDVoid, make([]byte, 8))
	cues := elem(t, extmkv.IDCues, []byte("cue data"))

	// Fixed-width entries keep every length stable across rewrites.
	sh1Placeholder := elem(t, extmkv.IDSeekHead, seekEntry(t, extmkv.IDSeekHead, 0))
	sh2Placeholder := elem(t, extmkv.IDSeekHead, seekEntry(t, extmkv.IDCues, 0))
	sh2Pos := uint64(len(sh1Placeholder) + len(void))
	cuesPos := sh2Pos + uint64(len(sh2Placeholder))
	sh1 := elem(t, extmkv.IDSeekHead, seekEntry(t, extmkv.IDSeekHead, sh2Pos))
	sh2 := elem(t, extmkv.IDSeekHead, seekEntry(t, extmkv.IDCues, cuesPos))
	path := writeFixture(t, "twolevel.mkv", sh1, void, sh2, cues)

	has, confirmed := probeCuesPresence(path)
	if !confirmed || !has {
		t.Errorf("two-level SeekHead must confirm Cues, got has=%v confirmed=%v", has, confirmed)
	}
}
