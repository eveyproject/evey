package ffmpeg

// clip_args.go builds a short SDR MP4 of a source with one audio track - the
// sync-preview clip an operator judges outside any player session. It shares
// the player's own filter-graph builders (deinterlaceFilterFor, scaleFilterFor,
// resolveToneMap, the hw* GPU stages) instead of keeping a private copy of the
// tone-map string, so what a clip shows is provably the picture the player
// would draw for the same PicturePlan.

import (
	"fmt"
	"strconv"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// ClipSpec is one short MP4 picture of a source with one audio track.
type ClipSpec struct {
	Path          string
	AtSec, LenSec float64
	AudioTrack    int     // ffmpeg audio index of the track heard (0 = first)
	AudioShiftSec float64 // delays the audio by this much against the picture (negative advances it)
	Width         int
	Picture       transcoding.PicturePlan
	Out           string // output .mp4 path
}

// clipQuality is the GPU rate-control target for an ~854px clip: a browser
// preview, not a distribution rung, so a fixed modest VBR ceiling is enough -
// there is no per-title ladder to size it from here.
var clipQuality = transcoding.QualityTarget{Mode: transcoding.RateControlVBR, TargetKbps: 1500, MaxKbps: 2500, BufferKbps: 5000}

// softwareVideoFilter is the CPU picture chain shared by every non-player
// producer of a browser-playable SDR frame (the clip, the reduced download):
// deinterlace, then either the tone-map (which folds the downscale into its
// own linearizing zscale) or a plain scale, exactly as buildSingleTranscodeArgs
// runs them for a session.
func softwareVideoFilter(opts transcoding.HLSProcessOpts, width int) string {
	deint := deinterlaceFilterFor(opts)
	if tm := resolveToneMap(opts, width); tm != "" {
		return deint + "," + tm + ",setsar=1"
	}
	return deint + "," + scaleFilterFor(opts, width)
}

// fmtSec renders a seek/duration the way ffmpeg's -ss/-t want it: 3 decimals,
// so two inputs built from the same expression stay byte-identical.
func fmtSec(sec float64) string {
	return strconv.FormatFloat(sec, 'f', 3, 64)
}

// BuildClipArgs builds the ffmpeg argv for one clip. Pure. It returns an error
// (never a software fallback) when the picture names a GPU route this builder
// cannot express, exactly like the HLS builders (routeRefusal/Expressibility).
func BuildClipArgs(spec ClipSpec) ([]string, error) {
	opts := transcoding.HLSProcessOpts{
		FilePath:           spec.Path,
		SubtitleTrackIndex: -1,
		Profiles:           []transcoding.HLSProfile{{Width: spec.Width}},
	}
	spec.Picture.ApplyTo(&opts)

	// Both windows come from the SAME seek expression, shifted only by the
	// difference between the two -ss values - two independent seeks measured
	// wrong on real files (see sync_preview.go's header).
	audioAt := spec.AtSec - spec.AudioShiftSec
	if audioAt < 0 {
		audioAt = 0
	}

	args := []string{"-v", "error", "-nostdin", "-y"}

	var vf string
	var encArgs []string
	if !spec.Picture.HW() {
		vf = softwareVideoFilter(opts, spec.Width)
		args = append(args, "-ss", fmtSec(spec.AtSec), "-t", fmtSec(spec.LenSec), "-i", spec.Path)
		args = append(args, "-ss", fmtSec(audioAt), "-t", fmtSec(spec.LenSec), "-i", spec.Path)
		encArgs = []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "30", "-profile:v", "high", "-pix_fmt", "yuv420p"}
	} else {
		toneMap := resolveToneMap(opts, 0)
		if err := routeRefusal(opts, false, toneMap); err != nil {
			return nil, err
		}
		// The hwaccel prologue binds to the VIDEO -i that follows it; the audio
		// input right after decodes in software, same as appendHWTranscodeInputArgs.
		if opts.HWSoftwareDecode {
			args = append(args, hwDeviceOnlyArgs(opts.HWAccel, opts.HWDevice)...)
		} else {
			args = append(args, hwInitArgs(opts.HWAccel, opts.HWDevice)...)
		}
		args = append(args, "-ss", fmtSec(spec.AtSec), "-t", fmtSec(spec.LenSec), "-i", spec.Path)
		args = append(args, "-ss", fmtSec(audioAt), "-t", fmtSec(spec.LenSec), "-i", spec.Path)

		vf = hwSourceFilterPrefix(opts)
		if tm := hwToneMapStage(opts); tm != "" {
			vf += tm + ","
		}
		vf += hwRungScaleFilter(opts, spec.Width)

		encArgs = append([]string{"-c:v", videoEncoderFor("h264", opts.HWAccel)}, hwEncoderQualityArgs(opts.HWAccel, "h264", opts.HWFeatures)...)
		encArgs = append(encArgs, hwRateControlArgs(opts.HWAccel, clipQuality, -1)...)
	}

	args = append(args, "-map", "0:v:0", "-map", fmt.Sprintf("1:a:%d", spec.AudioTrack), "-map_chapters", "-1")
	args = append(args, "-vf", vf)
	args = append(args, encArgs...)
	args = append(args, "-c:a", "aac", "-b:a", "128k", "-ac", "2")
	args = append(args, "-movflags", "+faststart", spec.Out)
	return args, nil
}
