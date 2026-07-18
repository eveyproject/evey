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
