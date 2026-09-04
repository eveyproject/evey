package ffmpeg

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/reduceddl"
)

// The argv IS the behavior. Every flag asserted here is load-bearing for the
// reassembly on the phone, not taste - see the file header of
// reduced_transcode.go. A reorder is a product change, not a refactor.
func TestBuildReducedArgs(t *testing.T) {
	args := BuildReducedArgs("/media/film.mkv", "/cache/reduced/j/slice-%05d.mkv", "/cache/reduced/j/slices-00000.csv",
		reduceddl.Spec{Width: 1280, SliceSec: 600})

	assert.Equal(t, []string{
		"-hide_banner", "-v", "error", "-nostdin", "-y",
		"-i", "/media/film.mkv",
		"-map", "0:v:0", "-map", "0:a", "-map", "0:s?",
		"-vf", "scale=1280:-2",
		"-c:v", "libx264", "-preset", "veryfast",
		"-crf", "23",
		"-maxrate", "2500k",
		"-bufsize", "5000k",
		"-profile:v", "high", "-level", "4.0", "-pix_fmt", "yuv420p",
		"-force_key_frames", "expr:gte(t,n_forced*2)",
		"-c:a", "aac", "-ac", "2", "-b:a", "128k",
		// Derived, not literal - see the same note in compat_transcode_test.go.
		"-rematrix_maxval", fmt.Sprintf("%.4f", transcoding.DownmixRematrixMaxval),
		"-c:s", "copy",
		"-f", "segment",
		"-segment_format", "matroska",
		"-segment_time", "600",
		"-reset_timestamps", "1",
		"-segment_start_number", "0",
		"-segment_list", "/cache/reduced/j/slices-00000.csv",
		"-segment_list_type", "csv",
		"-segment_list_flags", "+live",
		"/cache/reduced/j/slice-%05d.mkv",
	}, args)
}

// A resume must seek the input AND renumber the output, or the second run
// overwrites slice 0 and the phone joins the middle of the film onto its start.
func TestBuildReducedArgsResumesAtTheRightSlice(t *testing.T) {
	// StartSec is the muxer's recorded end of slice 3 - deliberately NOT
	// 4*600. Slices run a little long (the cut waits for the next keyframe),
	// so seeking to the arithmetic guess would skip 9 s of film here.
	args := BuildReducedArgs("/media/film.mkv", "/j/slice-%05d.mkv", "/j/slices-00004.csv",
		reduceddl.Spec{Width: 1280, SliceSec: 600, StartSlice: 4, StartSec: 2409.6})

	// -ss must come BEFORE -i to be an input seek (fast); after -i it would
	// decode and discard 40 minutes of film.
	ssAt := indexOf(t, args, "-ss")
	iAt := indexOf(t, args, "-i")
	assert.Less(t, ssAt, iAt, "-ss must precede -i (input seek, not output trim)")
	assert.Equal(t, "2409.600", args[ssAt+1], "the recorded end of the last finished slice")
	assert.Equal(t, "4", args[indexOf(t, args, "-segment_start_number")+1])

	// And a run that starts at zero must not carry a pointless seek.
	fresh := BuildReducedArgs("/media/film.mkv", "/j/slice-%05d.mkv", "/j/slices-00000.csv",
		reduceddl.Spec{Width: 1280, SliceSec: 600})
	assert.NotContains(t, fresh, "-ss")
}

// The tonemap folds the downscale into its own scale, so the plain scale must
// step aside - two scales would resize twice and waste the weak NAS's CPU.
func TestBuildReducedArgsToneMapsHDRAndScalesOnce(t *testing.T) {
	args := BuildReducedArgs("/media/hdr.mkv", "/j/slice-%05d.mkv", "/j/slices-00000.csv",
		reduceddl.Spec{Width: 1280, SliceSec: 600, ToneMap: true})

	vf := args[indexOf(t, args, "-vf")+1]
	assert.Contains(t, vf, "tonemap")
	assert.Contains(t, vf, "1280", "the delivery width must be folded into the tonemap")
	assert.NotEqual(t, "scale=1280:-2", vf, "the plain scale must give way to the tonemap chain")
	assert.Equal(t, 1, strings.Count(vf, "1280"), "the frame must be resized once, not twice")
}

// A zero SliceSec is a caller bug, not a licence to emit "-segment_time 0"
// (which makes ffmpeg cut on every keyframe and produce thousands of files).
func TestBuildReducedArgsFallsBackToTheDefaultSliceLength(t *testing.T) {
	args := BuildReducedArgs("/media/film.mkv", "/j/slice-%05d.mkv", "/j/slices-00000.csv",
		reduceddl.Spec{Width: 1280})
	assert.Equal(t, "600", args[indexOf(t, args, "-segment_time")+1])
}

func indexOf(t *testing.T, args []string, flag string) int {
	t.Helper()
	for i, a := range args {
		if a == flag {
			require.Less(t, i+1, len(args), "%s has no value", flag)
			return i
		}
	}
	t.Fatalf("%s not in argv: %v", flag, args)
	return -1
}

// -max_interleave_delta 0 does the OPPOSITE of what its comment claimed: it
// makes libavformat buffer until every stream has a packet, so a sparse
// subtitle track holds minutes of encoded video in memory (ffmpeg docs, and
// premise 8 of the 2026-08-24 OOM forensics). The default 10 s bound stays.
func TestBuildReducedArgs_InterleaveStaysBounded(t *testing.T) {
	t.Parallel()
	args := BuildReducedArgs("/media/film.mkv", "/cache/reduced/j/slice-%05d.mkv", "/cache/reduced/j/slices-00000.csv", reduceddl.Spec{Width: 1280})
	for _, a := range args {
		if a == "-max_interleave_delta" {
			t.Fatal("-max_interleave_delta must not be set on the reduced production")
		}
	}
}
