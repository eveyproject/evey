package ffmpeg

import "github.com/eveyproject/evey/internal/domain/transcoding"

// Test-only re-exports of the unexported filter builders so the external
// ffmpeg_test package can assert their pure string output.

func DeinterlaceFilterForTest(opts transcoding.HLSProcessOpts) string {
	return deinterlaceFilterFor(opts)
}

func ScaleFilterForTest(opts transcoding.HLSProcessOpts, width int) string {
	return scaleFilterFor(opts, width)
}
