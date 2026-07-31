package transcoding

import "testing"

func validOpts() HLSProcessOpts {
	return HLSProcessOpts{
		FilePath:      "/media/film.mkv",
		OutputDir:     "/cache/segments/abc",
		Profiles:      []HLSProfile{{Name: "1080p", Width: 1920, Height: 1080}},
		VideoDecision: "transcode",
		AudioDecision: "transcode",
	}
}

// Validate is the last thing standing between a caller's mistake and a forked
// ffmpeg that dies on a cryptic exit code, which then reads as a broken media
// file rather than a bad request.
func TestHLSProcessOpts_Validate(t *testing.T) {
	if err := validOpts().Validate(); err != nil {
		t.Fatalf("a well-formed opts must pass: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*HLSProcessOpts)
	}{
		{"no source path", func(o *HLSProcessOpts) { o.FilePath = "" }},
		{"no output dir", func(o *HLSProcessOpts) { o.OutputDir = "" }},
		{"no profiles", func(o *HLSProcessOpts) { o.Profiles = nil }},
		{"typo'd video decision", func(o *HLSProcessOpts) { o.VideoDecision = "remux" }},
		{"typo'd audio decision", func(o *HLSProcessOpts) { o.AudioDecision = "passthrough" }},
		{"negative start", func(o *HLSProcessOpts) { o.StartSec = -1 }},
		{"negative start number", func(o *HLSProcessOpts) { o.StartNumber = -1 }},
		// -c:v copy on a rung the manifest advertises as re-encoded is the
		// scrambled-frames class of bug, not a cosmetic mismatch.
		{"copy-on-seek without a video copy", func(o *HLSProcessOpts) {
			o.CopyOnSeek = true
			o.VideoDecision = "transcode"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := validOpts()
			tt.mutate(&o)
			if err := o.Validate(); err == nil {
				t.Errorf("%s must be refused before a process is forked", tt.name)
			}
		})
	}
}

// The multi-variant path leaves VideoDecision empty on purpose, and the copy
// rung legitimately pairs CopyOnSeek with a video copy. Neither may be refused.
func TestHLSProcessOpts_Validate_AcceptsTheRealShapes(t *testing.T) {
	multiVariant := validOpts()
	multiVariant.VideoDecision = ""
	multiVariant.Profiles = []HLSProfile{{Name: "480p"}, {Name: "1080p"}}
	if err := multiVariant.Validate(); err != nil {
		t.Errorf("multi-variant (empty VideoDecision) must pass: %v", err)
	}

	copySeek := validOpts()
	copySeek.VideoDecision = "copy"
	copySeek.AudioDecision = "copy"
	copySeek.CopyOnSeek = true
	copySeek.StartSec = 231.05
	copySeek.StartNumber = 34
	if err := copySeek.Validate(); err != nil {
		t.Errorf("the copy rung's seek shape must pass: %v", err)
	}
}
