package ffmpeg

import (
	"slices"
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// itsoffsetOf is the -itsoffset an argument list gives its input, "" for none,
// and whether it comes before that input's -i.
func itsoffsetOf(args []string) (string, bool) {
	i := slices.Index(args, "-itsoffset")
	if i < 0 {
		return "", true
	}
	return args[i+1], i < slices.Index(args, "-i")
}

// A rendition is the only audio such a playback has: the user's resync and a
// source start delay are applied to it by the muxed ladder's rule, one shift.
func TestAudioOnlyArgs_ShiftTheInputLikeTheMuxedLadder(t *testing.T) {
	t.Parallel()
	base := transcoding.AudioProcessOpts{FilePath: "/f.mkv", OutputDir: "/o", AudioCodec: "aac", AudioBitrate: 192, MaxAudioChannels: 2}
	cases := map[string]struct {
		user, delay float64
		want        string
	}{
		"nothing":                          {0, 0, ""},
		"a delay under the floor stays":    {0, 0.05, ""},
		"a mux-defect delay is cancelled":  {0, 0.896, "-0.896"},
		"the user's resync, audio later":   {0.5, 0, "0.500"},
		"the user's resync, audio earlier": {-0.3, 0, "-0.300"},
		"both compose":                     {0.5, 0.896, "-0.396"},
	}
	for name, c := range cases {
		o := base
		o.AudioUserOffsetSec, o.AudioStartDelaySec = c.user, c.delay
		got, before := itsoffsetOf(BuildFFmpegAudioOnlyArgs(o))
		if got != c.want || !before {
			t.Errorf("%s: -itsoffset %q (before -i: %t), want %q", name, got, before, c.want)
		}
	}

	seek := base
	seek.StartSec, seek.StartNumber, seek.AudioUserOffsetSec = 30.03, 10, 0.5
	args := BuildFFmpegAudioOnlyArgs(seek)
	if got, _ := itsoffsetOf(args); got != "0.500" || slices.Index(args, "-ss") > slices.Index(args, "-itsoffset") {
		t.Errorf("a seek keeps its target and the shift: %v", args)
	}

	copied := base
	copied.AudioCodec, copied.AudioUserOffsetSec = "copy", 0.5
	silence := base
	silence.SilenceDurationSec, silence.AudioUserOffsetSec = 3, 0.5
	for name, o := range map[string]transcoding.AudioProcessOpts{"copy": copied, "silence": silence} {
		if got, _ := itsoffsetOf(BuildFFmpegAudioOnlyArgs(o)); got != "" {
			t.Errorf("%s takes no shift: %q", name, got)
		}
	}
}

// The re-encoded track lands on the file's origin rather than on its own first
// sample: rebased (asetpts=PTS-STARTPTS first, as this once was) every start
// offset and every resync vanished.
func TestAudioOnlyArgs_KeepTheTrackOnTheFilesOrigin(t *testing.T) {
	t.Parallel()
	args := BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{FilePath: "/f.mkv", OutputDir: "/o", AudioCodec: "aac", AudioBitrate: 192, MaxAudioChannels: 2, SourceAudioChannels: 6})
	af := args[slices.Index(args, "-af")+1]
	if !strings.HasPrefix(af, audioOriginFilter+",") {
		t.Fatalf("the chain must open on the origin alignment: %q", af)
	}
	if strings.Count(af, "asetpts=PTS-STARTPTS") != 1 || !strings.HasSuffix(af, "atrim=start=0.042667,asetpts=PTS-STARTPTS") && !strings.HasSuffix(af, "atrim=start=0.021333,asetpts=PTS-STARTPTS") {
		t.Fatalf("only the priming trim rebases, at the end: %q", af)
	}
}
