package httpapi

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"testing"
)

// The blob is what the player parses, so its shape is a contract: magic, a
// length-prefixed JSON header, then the PNGs back to back at the offsets the
// header names. A reader that guesses instead of checking would render a
// stranger's pixels at the first format change.
func TestPGSBlob_RoundTripsHeaderAndImages(t *testing.T) {
	imgs := [][]byte{[]byte("PNG-A"), []byte("PNG-BB"), []byte("PNG-CCC")}
	h := pgsHeader{ScreenW: 1920, ScreenH: 1080}
	off := 0
	for i, im := range imgs {
		h.Cues = append(h.Cues, pgsCue{
			StartMs: int64(i * 1000), EndMs: int64(i*1000 + 800),
			X: i, Y: 10 * i, W: 100, H: 40,
			Off: off, Len: len(im), Forced: i == 1,
		})
		off += len(im)
	}

	blob, err := encodePGSBlob(h, imgs)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if got := string(blob[:len(pgsBlobMagic)]); got != pgsBlobMagic {
		t.Fatalf("magic = %q, want %q", got, pgsBlobMagic)
	}

	back, err := decodePGSBlobHeader(blob)
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	if back.ScreenW != 1920 || back.ScreenH != 1080 {
		t.Errorf("plane = %dx%d, want 1920x1080", back.ScreenW, back.ScreenH)
	}
	if len(back.Cues) != len(imgs) {
		t.Fatalf("cues = %d, want %d", len(back.Cues), len(imgs))
	}
	if !back.Cues[1].Forced {
		t.Error("the per-cue Forced flag must survive the round trip: a full track mixes forced and normal cues")
	}

	// The offsets must actually land on the images, which is the only thing the
	// player relies on to slice the body.
	body := blob[len(pgsBlobMagic)+4+headerLen(t, blob):]
	for i, c := range back.Cues {
		if got := body[c.Off : c.Off+c.Len]; !bytes.Equal(got, imgs[i]) {
			t.Errorf("cue %d: body slice = %q, want %q", i, got, imgs[i])
		}
	}
}

func headerLen(t *testing.T, blob []byte) int {
	t.Helper()
	return int(binary.BigEndian.Uint32(blob[len(pgsBlobMagic):]))
}

// A track with no cue over the whole film is a legitimate answer, not a
// failure: a forced track carries a handful of cues and can be empty. The blob
// must still be well-formed so the player shows nothing instead of erroring.
func TestPGSBlob_EmptyTrackIsStillWellFormed(t *testing.T) {
	blob, err := encodePGSBlob(pgsHeader{ScreenW: 1920, ScreenH: 1080}, nil)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	h, err := decodePGSBlobHeader(blob)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(h.Cues) != 0 {
		t.Errorf("cues = %d, want 0", len(h.Cues))
	}
	if h.ScreenW != 1920 {
		t.Errorf("the plane must survive even with no cue, got %d", h.ScreenW)
	}
}

// Refuse rather than guess. A truncated or foreign body must not be parsed as a
// subtitle - the offsets it names would index into whatever follows.
func TestPGSBlob_RefusesGarbage(t *testing.T) {
	cases := map[string][]byte{
		"vide":             nil,
		"trop court":       []byte("EVEY"),
		"mauvais magique":  append([]byte("NOTEVEY1"), 0, 0, 0, 2, '{', '}'),
		"longueur au-dela": append([]byte(pgsBlobMagic), 0xFF, 0xFF, 0xFF, 0xFF),
	}
	for name, blob := range cases {
		if _, err := decodePGSBlobHeader(blob); err == nil {
			t.Errorf("%s: expected a refusal, got none", name)
		}
	}
}

// The plane the coordinates refer to is NOT the video resolution: every 2160p
// disc measured carries a 1920x1080 subtitle plane. Scaling a cue by the video
// size would offset and mis-size it by a factor of two, so the plane travels
// with the cues rather than being inferred at the far end.
func TestPGSBlob_CarriesTheSubtitlePlane(t *testing.T) {
	blob, err := encodePGSBlob(pgsHeader{ScreenW: 1920, ScreenH: 1080,
		Cues: []pgsCue{{StartMs: 0, EndMs: 1, W: 800, H: 60}}}, [][]byte{nil})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	h, err := decodePGSBlobHeader(blob)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if h.ScreenW != 1920 || h.ScreenH != 1080 {
		t.Fatalf("plane = %dx%d, want the SUBTITLE plane 1920x1080", h.ScreenW, h.ScreenH)
	}
}

// Only PGS is decodable here. Answering 200 with an empty cue list for VobSub
// would put a subtitle in the viewer's menu that draws nothing.
func TestPGSServable_PGSOnly(t *testing.T) {
	for codec, want := range map[string]bool{
		"hdmv_pgs_subtitle": true,
		"pgs":               true,
		"HDMV_PGS_SUBTITLE": true,
		"dvd_subtitle":      false,
		"vobsub":            false,
		"subrip":            false,
		"ass":               false,
	} {
		if got := pgsServable(codec); got != want {
			t.Errorf("pgsServable(%q) = %v, want %v", codec, got, want)
		}
	}
}

// The two kinds share a (file, track) pair but hold different bytes, so they
// must not collide. And the VTT key has to stay BYTE-IDENTICAL to what it was:
// every entry already on disk was written under it, and each one costs a full
// read of a film to rebuild.
func TestSubtitleCache_KindNamespacing(t *testing.T) {
	c := subtitleCache{dir: t.TempDir()}
	f := t.TempDir() + "/movie.mkv"
	if err := writeFileForTest(f); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	plain := c.keyFor(f, 2)
	vtt := c.keyForKind(f, 2, subtitleKindVTT)
	pgs := c.keyForKind(f, 2, subtitleKindPGS)

	if vtt != plain {
		t.Errorf("the VTT key must not move: got %q, want the pre-existing %q", vtt, plain)
	}
	if pgs == vtt {
		t.Error("a bitmap blob and a WebVTT must not share a key")
	}
	if got := c.path(pgs); got[len(got)-4:] != ".bin" {
		t.Errorf("a bitmap blob must not be filed as .vtt, got %q", got)
	}
	if got := c.path(vtt); got[len(got)-4:] != ".vtt" {
		t.Errorf("WebVTT must keep its extension, got %q", got)
	}
}

func writeFileForTest(path string) error {
	return os.WriteFile(path, []byte("x"), 0o600)
}

// Header size is bounded before anything in the body is trusted.
func TestPGSBlob_HeaderCap(t *testing.T) {
	big := pgsHeader{}
	for i := range 200000 {
		big.Cues = append(big.Cues, pgsCue{StartMs: int64(i), EndMs: int64(i) + 1})
	}
	hj, _ := json.Marshal(big)
	if len(hj) <= pgsHeaderMaxBytes {
		t.Skipf("fixture header is only %d bytes, under the %d cap", len(hj), pgsHeaderMaxBytes)
	}
	if _, err := encodePGSBlob(big, nil); err == nil {
		t.Error("a header past the cap must be refused, not written")
	}
}
