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
