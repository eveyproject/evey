package ffmpeg

import (
	"slices"
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// An AAC track copied out of an MPEG-TS is ADTS-framed and must be re-framed
// for fMP4; the filter is scoped to that one case, since it is a no-op on raw
// AAC and refuses any other codec.
func TestAdtsReframeArgs(t *testing.T) {
	t.Parallel()
	want := []string{"-bsf:a", "aac_adtstoasc"}
	for _, c := range []struct {
		codec, path string
		reframe     bool
	}{
		{"aac", "/m/film.ts", true},
		{"AAC", "/m/film.M2TS", true},
		{"aac", "/m/film.mts", true},
		{"aac", "/m/film.mkv", false},
		{"aac", "/m/film.mp4", false},
		{"ac3", "/m/film.ts", false},
		{"eac3", "/m/film.ts", false},
		{"", "/m/film.ts", false},
		{"copy", "/m/film.ts", false},
	} {
		got := adtsReframeArgs(c.codec, c.path)
		if c.reframe && !slices.Equal(got, want) {
			t.Fatalf("%s in %s must be re-framed, got %v", c.codec, c.path, got)
		}
		if !c.reframe && got != nil {
			t.Fatalf("%s in %s must not be re-framed, got %v", c.codec, c.path, got)
		}
	}
}

// The three copy sites carry the filter for an MPEG-TS source - the separate
// rendition, the muxed rung and the copy rung - and none of them for a
// Matroska one.
func TestAudioCopy_ReframesADTSOnEveryCopySite(t *testing.T) {
	t.Parallel()
	rendition := func(path string) string {
		return strings.Join(BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{
			FilePath: path, OutputDir: "/out", AudioCodec: "copy", SourceAudioCodec: "aac",
		}), " ")
	}
	muxed := func(path string) string {
		return strings.Join(BuildFFmpegArgs(transcoding.HLSProcessOpts{
			FilePath: path, OutputDir: "/out", VideoDecision: "transcode", AudioDecision: "copy", AudioCodec: "aac",
			SubtitleTrackIndex: -1, UseFMP4: true,
			Profiles: []transcoding.HLSProfile{{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2800, AudioBitrate: 192}},
		}), " ")
	}
	copyRung := func(path string) string {
		return strings.Join(BuildFFmpegArgs(transcoding.HLSProcessOpts{
			FilePath: path, OutputDir: "/out", VideoDecision: "copy", AudioDecision: "copy", AudioCodec: "aac",
			SubtitleTrackIndex: -1, UseFMP4: true,
			Profiles: []transcoding.HLSProfile{{Name: "copy", Width: 1280, Height: 720, VideoBitrate: 2800, AudioBitrate: 192, Copy: true}},
		}), " ")
	}
	for name, build := range map[string]func(string) string{"rendition": rendition, "muxed": muxed, "copy rung": copyRung} {
		ts := build("/m/film.ts")
		if !strings.Contains(ts, "-c:a copy -bsf:a aac_adtstoasc") {
			t.Fatalf("%s on an MPEG-TS must re-frame the copied AAC:\n%s", name, ts)
		}
		mkv := build("/m/film.mkv")
		if strings.Contains(mkv, "aac_adtstoasc") {
			t.Fatalf("%s on a Matroska must not re-frame:\n%s", name, mkv)
		}
	}
	// A re-encoded rendition never carries the filter, whatever the source.
	enc := strings.Join(BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{
		FilePath: "/m/film.ts", OutputDir: "/out", AudioCodec: "aac", SourceAudioCodec: "aac",
	}), " ")
	if strings.Contains(enc, "aac_adtstoasc") {
		t.Fatalf("a re-encode must not re-frame:\n%s", enc)
	}
}
