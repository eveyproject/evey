package ffmpeg

import (
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// The decoupled path (software decode + hwupload + HW scale/encode) exists for
// a source the GPU cannot DECODE. Its whole value is that everything AFTER the
// upload stays the zero-copy pipeline, so these tests pin the two halves: no
// -hwaccel on the input, and the same GPU filters/encoders downstream.

func decoupledOpts(accel transcoding.HWAccelType, device string, nProfiles int) transcoding.HLSProcessOpts {
	opts := sampleHWOpts(accel, device, nProfiles)
	opts.HWSoftwareDecode = true
	return opts
}

func TestDecoupledHWArgs_SingleVariant(t *testing.T) {
	args := buildSingleTranscodeHWArgs(decoupledOpts(transcoding.VAAPI, "/dev/dri/renderD128", 1))
	joined := strings.Join(args, " ")

	if argsContain(args, "-hwaccel") {
		t.Fatalf("decoupled session must NOT ask ffmpeg to HW-decode: %s", joined)
	}
	if !argsContainSeq(args, "-vaapi_device", "/dev/dri/renderD128") {
		t.Errorf("the device must still be bound for the filters: %s", joined)
	}
	if !strings.Contains(joined, "format=nv12,hwupload") {
		t.Errorf("software-decoded frames must be uploaded to the GPU: %s", joined)
	}
	if !strings.Contains(joined, "scale_vaapi=") {
		t.Errorf("the scaler must stay on the GPU: %s", joined)
	}
	if !argsContainSeq(args, "-c:v", "h264_vaapi") {
		t.Errorf("the encoder must stay on the GPU - that is the whole point: %s", joined)
	}
	// The upload has to come BEFORE the scaler: scale_vaapi only accepts GPU
	// surfaces, and ffmpeg would refuse the graph the other way round.
	if strings.Index(joined, "hwupload") > strings.Index(joined, "scale_vaapi") {
		t.Errorf("hwupload must precede the GPU scaler: %s", joined)
	}
}

func TestDecoupledHWArgs_MultiVariant(t *testing.T) {
	args := buildMultiVariantHWArgs(decoupledOpts(transcoding.VAAPI, "/dev/dri/renderD128", 3))
	joined := strings.Join(args, " ")

	if argsContain(args, "-hwaccel") {
		t.Fatalf("decoupled session must NOT ask ffmpeg to HW-decode: %s", joined)
	}
	// One upload, before the split: the ladder must fan out GPU surfaces
	// exactly like the zero-copy path, not upload once per rung.
	if n := strings.Count(joined, "hwupload"); n != 1 {
		t.Errorf("want exactly one upload before the split, got %d: %s", n, joined)
	}
	if strings.Index(joined, "hwupload") > strings.Index(joined, "split=3") {
		t.Errorf("the upload must precede the split: %s", joined)
	}
	if n := strings.Count(joined, "scale_vaapi="); n != 3 {
		t.Errorf("want one GPU scaler per rung, got %d: %s", n, joined)
	}
	if n := strings.Count(joined, "h264_vaapi"); n != 3 {
		t.Errorf("want one GPU encoder per rung, got %d: %s", n, joined)
	}
}

// The deinterlacer stays in SOFTWARE on this path, ahead of the upload: both
// deinterlacers act only on frames the decoder flagged, and that flag lives on
// the host-side frame. A DVD rip is the population this path serves, so losing
// its deinterlace would be losing the point.
func TestDecoupledHWArgs_DeinterlacesInSoftwareBeforeUpload(t *testing.T) {
	args := buildMultiVariantHWArgs(decoupledOpts(transcoding.VAAPI, "/dev/dri/renderD128", 3))
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "bwdif") {
		t.Fatalf("want the software deinterlacer on the decoupled path: %s", joined)
	}
	if strings.Contains(joined, "deinterlace_vaapi") {
		t.Errorf("the GPU deinterlacer reads a flag the upload does not carry: %s", joined)
	}
	if strings.Index(joined, "bwdif") > strings.Index(joined, "hwupload") {
		t.Errorf("deinterlace must run on host frames, before the upload: %s", joined)
	}
}

// QSV expresses the same pipeline with its own device + upload syntax.
func TestDecoupledHWArgs_QSV(t *testing.T) {
	args := buildSingleTranscodeHWArgs(decoupledOpts(transcoding.QSV, "", 1))
	joined := strings.Join(args, " ")

	if !argsContainSeq(args, "-init_hw_device", "qsv=hw", "-filter_hw_device", "hw") {
		t.Errorf("QSV binds its filter device explicitly: %s", joined)
	}
	if !strings.Contains(joined, "hwupload=extra_hw_frames=16,format=qsv") {
		t.Errorf("QSV needs its own upload chain: %s", joined)
	}
}

// A zero-copy session must be byte-identical to what it was before the
// decoupled path existed: same -hwaccel prologue, GPU deinterlacer, no upload.
func TestZeroCopyHWArgs_Unchanged(t *testing.T) {
	args := buildMultiVariantHWArgs(sampleHWOpts(transcoding.VAAPI, "/dev/dri/renderD128", 3))
	joined := strings.Join(args, " ")

	if !argsContainSeq(args, "-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi") {
		t.Errorf("the zero-copy prologue must be untouched: %s", joined)
	}
	if strings.Contains(joined, "hwupload") {
		t.Errorf("a HW-decoded session never uploads - the frames are already there: %s", joined)
	}
	if !strings.Contains(joined, "deinterlace_vaapi=auto=1") {
		t.Errorf("the zero-copy path keeps the GPU deinterlacer: %s", joined)
	}
}

// hwUsable is the builder's own gate. It must refuse a decoupled session it
// cannot express rather than emit a broken filtergraph - the same defence the
// tone-map case has.
func TestHWUsable_DecoupledGuards(t *testing.T) {
	nvenc := decoupledOpts(transcoding.NVENC, "", 1)
	if hwUsable(nvenc, false, "") {
		t.Error("NVENC has no wired upload chain: the session belongs on software")
	}

	toneMapping := decoupledOpts(transcoding.VAAPI, "/dev/dri/renderD128", 1)
	toneMapping.HWTonemap = true
	if hwUsable(toneMapping, false, "tonemapx=...") {
		t.Error("the upload folds to 8-bit on the CPU: a GPU tone-map can no longer run")
	}

	noDevice := decoupledOpts(transcoding.VAAPI, "", 1)
	if hwUsable(noDevice, false, "") {
		t.Error("VAAPI with no render node binds nothing: hwupload would have no target")
	}

	ok := decoupledOpts(transcoding.VAAPI, "/dev/dri/renderD128", 1)
	if !hwUsable(ok, false, "") {
		t.Error("a plain decoupled VAAPI session is exactly what this path is for")
	}
}

// A HW multi-variant seek must not carry -copyts either: the shared init's
// edit list would shift every rung's segments (see TestBuildFFmpegArgs_MultiVariant_WithSeek).
func TestBuildMultiVariantHWArgs_SeekHasNoCopyts(t *testing.T) {
	opts := sampleHWOpts(transcoding.VAAPI, "/dev/dri/renderD128", 3)
	opts.StartSec = 30
	args := buildMultiVariantHWArgs(opts)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-ss 30.000") {
		t.Fatalf("expected the seek, got: %s", joined)
	}
	if strings.Contains(joined, "-copyts") {
		t.Fatalf("-copyts must not be emitted on a HW multi-variant seek: %s", joined)
	}
}
