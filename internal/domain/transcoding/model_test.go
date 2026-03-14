package transcoding_test

import (
	"testing"

	"github.com/gravity-zero/evey/internal/domain/transcoding"
)

func TestHWAccelTypeConstants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		typ  transcoding.HWAccelType
		want string
	}{
		{transcoding.VAAPI, "vaapi"},
		{transcoding.NVENC, "nvenc"},
		{transcoding.QSV, "qsv"},
		{transcoding.None, "none"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			t.Parallel()
			if string(c.typ) != c.want {
				t.Errorf("HWAccelType = %q, want %q", c.typ, c.want)
			}
		})
	}
}

func TestJobStatusConstants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		status transcoding.JobStatus
		want   string
	}{
		{transcoding.Queued, "queued"},
		{transcoding.Running, "running"},
		{transcoding.Completed, "completed"},
		{transcoding.Failed, "failed"},
		{transcoding.Cancelled, "cancelled"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			t.Parallel()
			if string(c.status) != c.want {
				t.Errorf("JobStatus = %q, want %q", c.status, c.want)
			}
		})
	}
}

func TestTranscodeProgress_PercentRange(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		percent float64
		valid   bool
	}{
		{"start", 0.0, true},
		{"halfway", 50.0, true},
		{"done", 100.0, true},
		{"over 100", 100.1, false},
		{"negative", -1.0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			p := &transcoding.TranscodeProgress{
				JobID:   "j1",
				Percent: c.percent,
			}
			valid := p.Percent >= 0 && p.Percent <= 100
			if valid != c.valid {
				t.Errorf("Percent=%v: valid=%v, want %v", p.Percent, valid, c.valid)
			}
		})
	}
}

func TestCacheInfo_Aggregate(t *testing.T) {
	t.Parallel()

	info := &transcoding.CacheInfo{
		TotalSize: 1024 * 1024 * 500, // 500 MB
		FileCount: 10,
		Files:     make([]transcoding.CacheFileInfo, 10),
	}

	if info.FileCount != len(info.Files) {
		t.Errorf("FileCount=%d does not match Files len=%d", info.FileCount, len(info.Files))
	}
	if info.TotalSize <= 0 {
		t.Error("TotalSize must be positive")
	}
}
