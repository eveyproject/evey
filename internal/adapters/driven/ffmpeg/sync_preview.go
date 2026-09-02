package ffmpeg

// A few seconds of picture with one audio track, small enough for a browser.
//
// The point is to let someone SEE a desync, because nothing measurable here
// says whether the sound follows the image. Two things learned the hard way on
// real files shape this:
//
//   - the two windows must come from the SAME seek. Asking for the picture at
//     T and the audio at T-D with two separate `-ss` values produced clips whose
//     real offset was not D at all, and an operator judged several of them for
//     nothing. Here the audio window is taken from a second input at the same
//     seek and shifted in the filter graph, so what the name says is what plays.
//   - the video is re-encoded small on purpose. These files are 4K HEVC in
//     Matroska with AC-3 or DTS: a browser plays none of it, and a preview
//     nobody can open is not a preview.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

const (
	// previewWidth keeps a clip a couple of megabytes: enough to read lips,
	// cheap to build and to send.
	previewWidth = 854
	// previewCRF trades quality for size. Sync is judged on timing, not detail.
	previewCRF = "30"
)

// SyncPreview renders the clips the media-health screen shows.
type SyncPreview struct {
	ffmpegPath string
}

// NewSyncPreview builds the renderer.
func NewSyncPreview(ffmpegPath string) *SyncPreview { return &SyncPreview{ffmpegPath: ffmpegPath} }

// RenderSyncPreview implements maintenance.SyncPreviewRenderer.
func (p *SyncPreview) RenderSyncPreview(ctx context.Context, req appmaintenance.SyncPreviewRequest) ([]byte, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.RenderSyncPreview")
	defer span.End()

	dir, err := os.MkdirTemp("", "evey-syncpreview-")
	if err != nil {
		return nil, fmt.Errorf("sync preview: temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	out := filepath.Join(dir, "preview.mp4")

	// The audio is delayed by taking it from earlier in the film. Both inputs
	// are seeked with the same expression so the shift is exactly what was
	// asked - the trap that made a whole ladder of test clips meaningless.
	audioAt := req.AtSec - req.ShiftSec
	if audioAt < 0 {
		audioAt = 0
	}
	args := []string{
		"-v", "error", "-nostdin", "-y",
		"-ss", strconv.FormatFloat(req.AtSec, 'f', 3, 64), "-t", strconv.FormatFloat(req.LenSec, 'f', 3, 64), "-i", req.Path,
		"-ss", strconv.FormatFloat(audioAt, 'f', 3, 64), "-t", strconv.FormatFloat(req.LenSec, 'f', 3, 64), "-i", req.Path,
		"-map", "0:v:0", "-map", fmt.Sprintf("1:a:%d", req.Track),
		"-vf", fmt.Sprintf("scale=%d:-2", previewWidth),
		"-c:v", "libx264", "-preset", "veryfast", "-crf", previewCRF,
		"-c:a", "aac", "-b:a", "128k", "-ac", "2",
		"-movflags", "+faststart",
		out,
	}
	cmd := exec.CommandContext(ctx, p.ffmpegPath, args...) //nolint:gosec // operator media path, fixed binary
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		return nil, fmt.Errorf("sync preview: render: %w: %s", runErr, stderr.String())
	}

	data, err := os.ReadFile(out) //nolint:gosec // path built here, inside our own temp dir
	if err != nil {
		return nil, fmt.Errorf("sync preview: read clip: %w", err)
	}
	if len(data) == 0 {
		// An empty file means ffmpeg found nothing at that position - a clip
		// past the end, most likely. Say so instead of sending zero bytes the
		// browser will render as a broken player.
		return nil, fmt.Errorf("sync preview: nothing to show at %.0fs", req.AtSec)
	}
	return data, nil
}
