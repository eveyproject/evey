package transcoding_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/transcoding"
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

func TestJobStatus_IsValid(t *testing.T) {
	t.Parallel()
	cases := map[transcoding.JobStatus]bool{
		transcoding.Queued:        true,
		transcoding.Running:       true,
		transcoding.Completed:     true,
		transcoding.Failed:        true,
		transcoding.Cancelled:     true,
		transcoding.JobStatus(""): false,
		"garbage":                 false,
	}
	for s, want := range cases {
		if s.IsValid() != want {
			t.Errorf("%q.IsValid() = %v, want %v", s, !want, want)
		}
	}
}

func TestTranscodeJob_IsTerminal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status string
		want   bool
	}{
		{string(transcoding.Queued), false},
		{string(transcoding.Running), false},
		{string(transcoding.Completed), true},
		{string(transcoding.Failed), true},
		{string(transcoding.Cancelled), true},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			t.Parallel()
			j := &transcoding.TranscodeJob{Status: transcoding.JobStatus(tt.status)}
			if got := j.IsTerminal(); got != tt.want {
				t.Errorf("IsTerminal() = %v, want %v", got, tt.want)
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

func TestTranscodeJob_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		job     transcoding.TranscodeJob
		wantErr string
	}{
		{
			name:    "valid",
			job:     transcoding.TranscodeJob{ID: "tj-1", MediaItemID: "m1", InputPath: "/a.mkv", OutputCodec: "h264"},
			wantErr: "",
		},
		{
			name:    "missing ID",
			job:     transcoding.TranscodeJob{MediaItemID: "m1", InputPath: "/a.mkv", OutputCodec: "h264"},
			wantErr: "transcode job ID is required",
		},
		{
			name:    "missing media item ID",
			job:     transcoding.TranscodeJob{ID: "tj-1", InputPath: "/a.mkv", OutputCodec: "h264"},
			wantErr: "transcode job media item ID is required",
		},
		{
			name:    "missing input path",
			job:     transcoding.TranscodeJob{ID: "tj-1", MediaItemID: "m1", OutputCodec: "h264"},
			wantErr: "transcode job input path is required",
		},
		{
			name:    "missing output codec",
			job:     transcoding.TranscodeJob{ID: "tj-1", MediaItemID: "m1", InputPath: "/a.mkv"},
			wantErr: "transcode job output codec is required",
		},
		{
			name:    "invalid status",
			job:     transcoding.TranscodeJob{ID: "tj-1", MediaItemID: "m1", InputPath: "/a.mkv", OutputCodec: "h264", Status: "bogus"},
			wantErr: "transcode job: invalid status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.job.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestTranscodeProfile_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		profile transcoding.TranscodeProfile
		wantErr string
	}{
		{
			name:    "valid",
			profile: transcoding.TranscodeProfile{Name: "1080p", VideoCodec: "h264", Container: "mp4"},
			wantErr: "",
		},
		{
			name:    "missing name",
			profile: transcoding.TranscodeProfile{VideoCodec: "h264", Container: "mp4"},
			wantErr: "transcode profile name is required",
		},
		{
			name:    "missing video codec",
			profile: transcoding.TranscodeProfile{Name: "1080p", Container: "mp4"},
			wantErr: "transcode profile video codec is required",
		},
		{
			name:    "missing container",
			profile: transcoding.TranscodeProfile{Name: "1080p", VideoCodec: "h264"},
			wantErr: "transcode profile container is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.profile.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestNewTranscodeJob(t *testing.T) {
	t.Parallel()

	t.Run("happy path defaults status to Queued", func(t *testing.T) {
		t.Parallel()
		j, err := transcoding.NewTranscodeJob("tj-1", "m1", "/a.mkv", "h264",
			transcoding.WithJobAudioCodec("aac"),
			transcoding.WithJobHWAccel("vaapi"),
			transcoding.WithJobBitrate(1_000_000, 2_000_000),
			transcoding.WithJobResolution("1920x1080", "1280x720"),
		)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if j.Status != transcoding.Queued || j.AudioCodec != "aac" || j.HWAccelType != "vaapi" ||
			j.BitrateIn != 1_000_000 || j.BitrateOut != 2_000_000 ||
			j.ResolutionIn != "1920x1080" || j.ResolutionOut != "1280x720" {
			t.Errorf("fields not wired: %+v", j)
		}
	})

	t.Run("missing id rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := transcoding.NewTranscodeJob("", "m1", "/a.mkv", "h264"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("missing output codec rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := transcoding.NewTranscodeJob("tj-1", "m1", "/a.mkv", ""); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestDefaultHLSProfiles(t *testing.T) {
	t.Parallel()

	profiles := transcoding.DefaultHLSProfiles
	if len(profiles) != 4 {
		t.Fatalf("expected 4 default HLS profiles, got %d", len(profiles))
	}

	names := []string{"360p", "480p", "720p", "1080p"}
	for i, p := range profiles {
		if p.Name != names[i] {
			t.Errorf("profile[%d].Name = %q, want %q", i, p.Name, names[i])
		}
		if p.Width <= 0 || p.Height <= 0 {
			t.Errorf("profile %s has invalid dimensions %dx%d", p.Name, p.Width, p.Height)
		}
		if p.VideoBitrate <= 0 {
			t.Errorf("profile %s has invalid video bitrate %d", p.Name, p.VideoBitrate)
		}
		if p.AudioBitrate <= 0 {
			t.Errorf("profile %s has invalid audio bitrate %d", p.Name, p.AudioBitrate)
		}
	}

	for i := 1; i < len(profiles); i++ {
		if profiles[i].Height <= profiles[i-1].Height {
			t.Errorf("profiles not in ascending resolution order: %s (%d) <= %s (%d)",
				profiles[i].Name, profiles[i].Height, profiles[i-1].Name, profiles[i-1].Height)
		}
	}
}

func TestDefaultWebProfile(t *testing.T) {
	t.Parallel()

	p := transcoding.DefaultWebProfile
	if len(p.VideoCodecs) == 0 {
		t.Error("DefaultWebProfile should have video codecs")
	}
	if len(p.AudioCodecs) == 0 {
		t.Error("DefaultWebProfile should have audio codecs")
	}
	if len(p.Containers) == 0 {
		t.Error("DefaultWebProfile should have containers")
	}
	if p.MaxWidth <= 0 || p.MaxHeight <= 0 {
		t.Error("DefaultWebProfile should have positive max dimensions")
	}
}

func TestDefaultAudioCodec(t *testing.T) {
	t.Parallel()
	if transcoding.DefaultAudioCodec != "aac" {
		t.Errorf("DefaultAudioCodec = %q, want %q", transcoding.DefaultAudioCodec, "aac")
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
