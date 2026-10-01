package library

import "testing"

func TestResolutionLabel_TheWidthNamesACroppedFrame(t *testing.T) {
	cases := []struct {
		w, h int
		want string
	}{
		{1920, 1080, "1080p"},
		{1920, 818, "1080p"},
		{1916, 798, "1080p"},
		{3840, 1600, "4K"},
		{3840, 2160, "4K"},
		{2560, 1080, "1440p"},
		{1280, 534, "720p"},
		{1280, 720, "720p"},
		{720, 576, "576p"},
		{720, 480, "480p"},
		{0, 1080, "1080p"},
		{1920, 0, ""},
	}
	for _, c := range cases {
		if got := ResolutionLabel(c.w, c.h); got != c.want {
			t.Errorf("ResolutionLabel(%d, %d) = %q, want %q", c.w, c.h, got, c.want)
		}
	}
}
