package library

import "testing"

func TestRefreshCodec(t *testing.T) {
	for _, c := range []struct {
		stored, probed, want string
		kept                 bool
	}{
		{"h264", "h264", "h264", false},
		{"subrip", "mov_text", "mov_text", false}, // the MP4 timed-text rename
		{"mov_text", "subrip", "mov_text", true},  // never the other way
		{"ass", "subrip", "ass", true},
		{"ac3", "eac3", "ac3", true},
	} {
		got, kept := RefreshCodec(c.stored, c.probed)
		if got != c.want || kept != c.kept {
			t.Errorf("RefreshCodec(%q, %q) = %q, %v; want %q, %v", c.stored, c.probed, got, kept, c.want, c.kept)
		}
	}
}

func TestRefreshTitle(t *testing.T) {
	for _, c := range []struct {
		stored  string
		probed  MediaStream
		want    string
		kept    bool
		explain string
	}{
		{"French", MediaStream{Title: "French"}, "French", false, "same title"},
		{"video handler", MediaStream{HandlerName: "video handler"}, "", false, "the handler name the old probe stored"},
		{"Commentaire", MediaStream{HandlerName: "SoundHandler"}, "Commentaire", true, "not the handler name: an edit or a real title"},
		{"video handler", MediaStream{Title: "Main", HandlerName: "video handler"}, "video handler", true, "the probe has a real title: not ours to pick"},
		{"", MediaStream{Title: "VFF E-AC3 5.1"}, "", true, "a stored empty title is not filled in"},
		{"Forced", MediaStream{}, "Forced", true, "no handler name, nothing proves the title came from it"},
	} {
		got, kept := RefreshTitle(c.stored, &c.probed)
		if got != c.want || kept != c.kept {
			t.Errorf("%s: RefreshTitle(%q) = %q, %v; want %q, %v", c.explain, c.stored, got, kept, c.want, c.kept)
		}
	}
}
