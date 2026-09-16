package ffmpeg

import "testing"

// The bench ladders pin the HEIGHT; scale_qsv has no "-2" so it gets the
// expression that stands for it, every other scaler gets the short form.
func TestHwScaleToHeight(t *testing.T) {
	cases := map[string]struct{ scaler, want string }{
		"qsv gets the expression": {"scale_qsv", "scale_qsv=w=trunc(720*iw/ih/2)*2:h=720"},
		"vaapi short form":        {"scale_vaapi", "scale_vaapi=-2:720"},
		"cuda short form":         {"scale_cuda", "scale_cuda=-2:720"},
		"software default":        {"", "scale=-2:720"},
	}
	for name, c := range cases {
		if got := hwScaleToHeight(c.scaler, 720); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}

// A CUDA index binds the filter device; the default GPU needs no index.
func TestCudaInitDeviceArgs(t *testing.T) {
	if got := cudaInitDeviceArgs(""); len(got) != 4 || got[1] != "cuda=cu" || got[3] != "cu" {
		t.Errorf("default gpu: %v", got)
	}
	if got := cudaInitDeviceArgs("1"); got[1] != "cuda=cu:1" {
		t.Errorf("indexed gpu: %v", got)
	}
	if got := nvencGPUArgs(""); got != nil {
		t.Errorf("default gpu needs no -gpu: %v", got)
	}
	if got := nvencGPUArgs("2"); len(got) != 2 || got[0] != "-gpu" || got[1] != "2" {
		t.Errorf("pinned gpu: %v", got)
	}
}
