package transcoding

import "testing"

func TestSourceClassFor(t *testing.T) {
	cases := []struct {
		name   string
		height int
		want   string
	}{
		{"4k_exact", 2160, "2160p"},
		{"4k_letterboxed", 1600, "1440p"}, // 3840x1600 source → still high-res bucket
		{"qhd", 1440, "1440p"},
		{"fhd_exact", 1080, "1080p"},
		{"fhd_letterboxed", 1080, "1080p"},
		{"hd_720", 720, ""},
		{"sd_480", 480, ""},
		{"zero", 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SourceClassFor(c.height); got != c.want {
				t.Errorf("height %d → %q; want %q", c.height, got, c.want)
			}
		})
	}
}
