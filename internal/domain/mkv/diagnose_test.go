package mkv

import "testing"

func TestHasIndexDefect_IndexSparse(t *testing.T) {
	d := &Diagnosis{Findings: []DiagnosisFinding{{Kind: "index-sparse", Detail: "60s hole"}}}
	if !d.HasIndexDefect() {
		t.Fatal("index-sparse must count as an index defect: video cues exist but leave a hole too wide to seek into")
	}
}

func TestWrongContainer(t *testing.T) {
	wc := &Diagnosis{Findings: []DiagnosisFinding{{Kind: "wrong-container", Remedy: "rename or remux to MP4"}}}
	if !wc.WrongContainer() {
		t.Fatal("a wrong-container finding must be reported: a .mkv that is really MP4/MOV")
	}
	if wc.HasIndexDefect() {
		t.Fatal("wrong-container is NOT an index defect: there is no Matroska Cues to build, the remedy is rename/remux")
	}
	if (&Diagnosis{Findings: []DiagnosisFinding{{Kind: "no-index"}}}).WrongContainer() {
		t.Fatal("an ordinary index defect must not be reported as wrong-container")
	}
}

func TestMaxAudioDelayMs(t *testing.T) {
	// Multiple tracks: the result must be the MAX, not the first/last/any entry
	// (map iteration order is random, so a "return any" mutant would survive a
	// single-entry test).
	d := &Diagnosis{AudioDelaysMs: map[uint64]int64{1: 200, 2: 900, 3: 50}}
	if got := d.MaxAudioDelayMs(); got != 900 {
		t.Fatalf("MaxAudioDelayMs = %d, want 900 (the maximum across tracks)", got)
	}
	if got := (&Diagnosis{}).MaxAudioDelayMs(); got != 0 {
		t.Fatalf("no tracks must yield 0, got %d", got)
	}
}

func TestTrailingJunk(t *testing.T) {
	// Surplus bytes past the declared Segment end: the media is complete, so
	// this is trailing-junk (harmless rewrite), NOT damaged (resync) nor
	// truncated (re-download).
	tj := &Diagnosis{Findings: []DiagnosisFinding{{Kind: "trailing-junk", Detail: "512 byte(s) beyond the declared Segment end"}}}
	if !tj.TrailingJunk() {
		t.Fatal("a trailing-junk finding must be reported")
	}
	if tj.Damaged() {
		t.Fatal("trailing junk is NOT mid-Segment damage: it must not be reported as damaged")
	}
	// Mid-Segment corruption is damaged, not trailing junk.
	dmg := &Diagnosis{Findings: []DiagnosisFinding{{Kind: "damaged", Detail: "1 damaged range"}}}
	if !dmg.Damaged() {
		t.Fatal("a damaged finding must be reported as damaged")
	}
	if dmg.TrailingJunk() {
		t.Fatal("mid-Segment damage must not be reported as trailing junk")
	}
}
