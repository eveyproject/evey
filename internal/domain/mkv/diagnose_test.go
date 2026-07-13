package mkv

import "testing"

func TestHasIndexDefect_IndexSparse(t *testing.T) {
	d := &Diagnosis{Findings: []DiagnosisFinding{{Kind: "index-sparse", Detail: "60s hole"}}}
	if !d.HasIndexDefect() {
		t.Fatal("index-sparse must count as an index defect: video cues exist but leave a hole too wide to seek into")
	}
}
