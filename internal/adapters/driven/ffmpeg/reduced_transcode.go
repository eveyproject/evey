package ffmpeg

// reduced_transcode.go - produce the SIZE-REDUCED (720p) download variant of a
// media file, in slices, for the mobile app.
//
// Why this exists: a 4K remux is 40+ GB and simply does not fit on a phone.
// The owner's constraints are (a) no second permanent copy, on the server or
// on the phone, and (b) the final file is reassembled on the DEVICE. So the
// server never publishes a whole reduced file: it emits consecutive Matroska
// slices, the phone pulls them one at a time and joins them locally (mkvgo's
// ops.Join), and each slice is dropped server-side as soon as the phone moves
// past it.
//
// The one design decision that makes the whole thing work: ONE ffmpeg process
// produces every slice, through the `segment` muxer, instead of one ffmpeg run
// per slice. It matters far more than it looks:
//
//   - Concatenability is free. Every slice comes out of the SAME encoder
//     instance, so SPS/PPS and the AAC extradata are byte-identical across
//     slices - exactly what ops.Join refuses to guess at (it compares
//     CodecPrivate and bails on a mismatch).
//   - A/V drift is structurally impossible. Packets are contiguous across a
//     cut: the last audio packet of slice i ends exactly where the first of
//     slice i+1 begins. ops.Join appends each track against ITS OWN end, so
//     the join is exact. N independent encodes could not promise this - each
//     one rounds its audio up to a whole AAC frame, and 21 ms per boundary
//     compounds into a quarter-second of lip-sync error over a feature film.
//
// Unlike the compat variant (audio-only, ~10-30x realtime), this re-encodes
// VIDEO: budget an order of magnitude near realtime on the weak-NAS target.
// A two-hour film can mean an hour of preparation, which is precisely why the
// unit of delivery is a slice and not a file.

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/eveyproject/evey/internal/pkg/procfork"

	domainmkv "github.com/eveyproject/evey/internal/domain/mkv"
	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/eveyproject/evey/internal/pkg/reduceddl"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// reducedTimeout bounds one production. Video re-encode runs near realtime on
// the weak NAS, so a long film legitimately takes hours; this only catches a
// wedged process.
const reducedTimeout = 8 * time.Hour

// reducedHeadroomBytes is the safety margin on top of the estimated output in
// the space preflight, mirroring the compat transcoder's contract.
const reducedHeadroomBytes = 512 << 20

// ReducedTranscoder produces reduced-variant slices by shelling out to ffmpeg.
type ReducedTranscoder struct {
	ffmpegPath string
}

func NewReducedTranscoder(ffmpegPath string) *ReducedTranscoder {
	return &ReducedTranscoder{ffmpegPath: ffmpegPath}
}

// BuildReducedArgs is the exact ffmpeg argv (exported because the command line
// IS the behavior, and the unit test asserts on it). Notes on the choices that
// are load-bearing rather than taste:
//
//   - "-f segment -segment_format matroska": one encoder, N standalone MKVs.
//     See the file header for why this is not N ffmpeg runs.
//   - "-reset_timestamps 1": each slice starts at 0, which is what ops.Join
//     expects (it re-offsets per track on the way in).
//   - "-force_key_frames expr:gte(t,n_forced*2)": the muxer only cuts on a
//     keyframe, so it needs one shortly after every boundary. Two seconds,
//     not the slice length - reduceddl.KeyframeSec explains why the tidier
//     choice produces slices of twice the intended size.
//   - "-segment_list ... -segment_list_flags +live": the list gains a line
//     when a slice is CLOSED. That is the readiness signal the handler serves
//     on; the file merely existing means ffmpeg is still writing into it.
//   - no "-max_interleave_delta 0": zero means "buffer until every stream has
//     a packet", so a sparse subtitle track held minutes of encoded video in
//     memory and stalled delivery. The default 10 s bound is the fix.
//   - no "-map_chapters": chapter timestamps are whole-file, and copying them
//     into every timestamp-reset slice would produce garbage after the join.
//     Chapters are a known, accepted loss on the reduced variant.
//
// The picture chain (deinterlace, tone-map or scale, SAR) is the SAME software
// chain a player session runs (softwareVideoFilter, shared with the clip
// builder in clip_args.go) instead of a private copy of the tone-map string.
func BuildReducedArgs(src, pattern, listPath string, spec reduceddl.Spec, picture transcoding.PicturePlan) []string {
	slice := spec.SliceSec
	if slice <= 0 {
		slice = reduceddl.SliceSec
	}

	args := []string{"-hide_banner", "-v", "error", "-nostdin", "-y"}
	// Resume: seek the input to where the last finished slice ENDED (recorded
	// by the muxer), not to StartSlice*SliceSec - slices run a little long, so
	// the arithmetic guess would leave a gap in the middle of the film.
	if spec.StartSlice > 0 && spec.StartSec > 0 {
		args = append(args, "-ss", strconv.FormatFloat(spec.StartSec, 'f', 3, 64))
	}
	args = append(args, "-i", src)
	// Video 0 only: a reduced copy has one picture track by definition. Audio
	// and subtitles all ride along ("?" = do not fail a film with no subs).
	args = append(args, "-map", "0:v:0", "-map", "0:a", "-map", "0:s?")

	// This production runs for hours under nice, sharing the box with live
	// viewers: it must never contend for the GPU a playback session needs, so
	// it always draws the picture in software - whatever route the caller
	// resolved the plan through.
	picture = picture.Software()
	opts := transcoding.HLSProcessOpts{FilePath: src, SubtitleTrackIndex: -1, Profiles: []transcoding.HLSProfile{{Width: spec.Width}}}
	picture.ApplyTo(&opts)
	args = append(args, "-vf", softwareVideoFilter(opts, spec.Width))

	args = append(args,
		// veryfast, not slower: the target is a weak NAS running this under
		// nice while someone may be streaming. Capped CRF (quality-driven but
		// VBV-bounded) is what makes EstimatedReducedBytes honest.
		"-c:v", "libx264", "-preset", "veryfast",
		"-crf", "23",
		"-maxrate", fmt.Sprintf("%dk", reduceddl.VideoKbps),
		"-bufsize", fmt.Sprintf("%dk", 2*reduceddl.VideoKbps),
		// Pinned so a resumed production's encoder emits the SAME SPS/PPS as
		// the first one - ops.Join compares CodecPrivate byte for byte.
		"-profile:v", "high", "-level", "4.0", "-pix_fmt", "yuv420p",
		// Keyframes on a 2 s grid, NOT on the slice grid - see
		// reduceddl.KeyframeSec. This is what gives the muxer a cut point just
		// after every boundary, and what makes the downloaded file seekable.
		"-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", reduceddl.KeyframeSec),
		"-c:a", "aac", "-ac", "2", "-b:a", fmt.Sprintf("%dk", reduceddl.AudioKbps),
	)
	// Same hardcoded stereo fold as the compat production, same permanent
	// consequence in the delivered file - see domain/transcoding/audio_downmix.go.
	args = append(args, "-af", transcoding.AudioFoldFilter(0, 2))
	args = append(args,
		"-c:s", "copy",
		"-f", "segment",
		"-segment_format", "matroska",
		"-segment_time", strconv.Itoa(slice),
		"-reset_timestamps", "1",
		"-segment_start_number", strconv.Itoa(spec.StartSlice),
		"-segment_list", listPath,
		"-segment_list_type", "csv",
		"-segment_list_flags", "+live",
		pattern,
	)
	return args
}

// TranscodeReducedSlices runs the production into dir, writing slices named by
// reduceddl.SliceName and appending to reduceddl.ListName as each one closes.
// It returns when ffmpeg exits; the caller polls the list to deliver slices
// while this is still running - that IS the feature.
//
// durationMs is the source's play length (the permille denominator). estBytes
// sizes the space preflight.
func (t *ReducedTranscoder) TranscodeReducedSlices(ctx context.Context, src, dir string, spec reduceddl.Spec, picture transcoding.PicturePlan, durationMs, estBytes int64, onProgress func(permille int)) error {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.TranscodeReducedSlices")
	defer span.End()

	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("reduced-transcode: stat source: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("reduced-transcode: create job dir: %w", err)
	}
	// The whole variant never has to fit at once (slices are released as the
	// phone consumes them), but a phone that stops fetching leaves the full
	// thing on disk - so budget for it.
	var st syscall.Statfs_t
	if serr := syscall.Statfs(dir, &st); serr == nil && estBytes > 0 {
		if avail := st.Bavail * uint64(st.Bsize); avail < uint64(estBytes)+reducedHeadroomBytes { //nolint:gosec // sizes are positive
			return fmt.Errorf(
				"reduced-transcode: %w: needs ~%d MiB free in the cache volume, %d MiB available",
				domainmkv.ErrInsufficientSpace, (estBytes+reducedHeadroomBytes)>>20, int64(avail)>>20) //nolint:gosec // display only
		}
	}

	ctx, cancel := context.WithTimeout(ctx, reducedTimeout)
	defer cancel()

	pattern := filepath.Join(dir, reduceddl.SlicePattern)
	listPath := filepath.Join(dir, reduceddl.ListName(spec.StartSlice))
	argv := append([]string{"-progress", "pipe:1", "-nostats"}, BuildReducedArgs(src, pattern, listPath, spec, picture)...)
	cmd, finish, err := procfork.Command(ctx, procfork.Options{Class: procfork.Production, Site: "reduced.transcode", Source: procfork.Source{Width: spec.SourceWidth, Height: spec.SourceHeight, TenBit: spec.SourceTenBit}}, t.ffmpegPath, argv...)
	if err != nil {
		return fmt.Errorf("reduced-transcode: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("reduced-transcode: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("reduced-transcode: start ffmpeg: %w", err)
	}
	// A resumed production reports against the whole film, not against what is
	// left of it: the user's progress bar must not jump back to 0.
	offsetMs := int64(spec.StartSec * 1000)
	scanReducedProgress(stdout, durationMs, offsetMs, onProgress)
	if err := finish(cmd.Wait(), stderr.Bytes()); err != nil {
		return fmt.Errorf("reduced-transcode: ffmpeg: %w: %s", err, firstLine(stderr.Bytes()))
	}
	return nil
}

// scanReducedProgress turns ffmpeg's "-progress" stream into permille reports
// against the WHOLE film (offsetMs shifts a resumed run back onto that scale).
func scanReducedProgress(r io.Reader, durationMs, offsetMs int64, onProgress func(permille int)) {
	if durationMs <= 0 || onProgress == nil {
		_, _ = io.Copy(io.Discard, r)
		return
	}
	last := -1
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		val, ok := strings.CutPrefix(sc.Text(), "out_time_us=")
		if !ok {
			continue
		}
		us, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		if err != nil || us < 0 {
			continue
		}
		// 1000 is the caller's "every slice is on disk", not ours.
		p := min(int((us/1000+offsetMs)*1000/durationMs), 999)
		if p != last {
			last = p
			onProgress(p)
		}
	}
}
