package ffmpeg

// A few seconds of picture with one audio track, small enough for a browser.
//
// The point is to let someone SEE a desync, because nothing measurable here
// says whether the sound follows the image. Three things learned the hard way
// on real files shape this:
//
//   - the two windows must come from the SAME seek. Asking for the picture at
//     T and the audio at T-D with two separate `-ss` values produced clips whose
//     real offset was not D at all, and an operator judged several of them for
//     nothing. The audio window is a second input at the same seek minus the
//     shift (BuildClipArgs), so what the name says is what plays.
//   - the video is re-encoded small on purpose. These files are 4K HEVC in
//     Matroska with AC-3 or DTS: a browser plays none of it, and a preview
//     nobody can open is not a preview.
//   - the picture is drawn by the PLAYER'S route (PicturePlan): the same
//     accelerator, tone-map, deinterlace and SAR the viewer's session gets.
//     A private chain here once wrote H.264 High 10 from every 10-bit source
//     (Firefox: "the file is corrupt") and decoded 4K HEVC on three CPU
//     threads while the operator waited. A GPU route that fails to start is
//     retried in software: the operator must see the clip, and a preview
//     never demotes the host's calibration the way a session would.

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/evey-project/evey/internal/pkg/procfork"

	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// previewWidth keeps a clip a couple of megabytes: enough to read lips, cheap
// to build and to send.
const previewWidth = 854

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

	spec := ClipSpec{
		Path: req.Path, AtSec: req.AtSec, LenSec: req.LenSec,
		AudioTrack: req.Track, AudioShiftSec: req.ShiftSec,
		Width: previewWidth, Picture: req.Picture,
		Out: filepath.Join(dir, "preview.mp4"),
	}
	data, err := p.render(ctx, spec)
	if err != nil && spec.Picture.HW() {
		slog.WarnContext(ctx, "sync preview: GPU route failed, drawing the clip in software",
			slog.String("accel", string(spec.Picture.Accel)), slog.String("topology", spec.Picture.Topology.Name()), slog.Any("err", err))
		spec.Picture = spec.Picture.Software()
		data, err = p.render(ctx, spec)
	}
	return data, err
}

// render runs one clip build. An operator is waiting on it (NoNice: at the
// default nice 19 it shared the CPU equally with the library scans and a
// twenty-second clip took minutes on a busy box); a GPU session maps address
// space far beyond its RSS, so the memory cap is lifted for it as the
// transcoder does.
func (p *SyncPreview) render(ctx context.Context, spec ClipSpec) ([]byte, error) {
	args, err := BuildClipArgs(spec)
	if err != nil {
		return nil, fmt.Errorf("sync preview: %w", err)
	}
	opts := procfork.Options{Class: procfork.Interactive, Site: "sync_preview.render", NoNice: true, NoMemCap: spec.Picture.HW()}
	if log, runErr := procfork.CombinedOutput(ctx, opts, p.ffmpegPath, args...); runErr != nil {
		return nil, fmt.Errorf("sync preview: render: %w: %s", runErr, log)
	}

	data, err := os.ReadFile(spec.Out) //nolint:gosec // path built here, inside our own temp dir
	if err != nil {
		return nil, fmt.Errorf("sync preview: read clip: %w", err)
	}
	if len(data) == 0 {
		// An empty file means ffmpeg found nothing at that position - a clip
		// past the end, most likely. Say so instead of sending zero bytes the
		// browser will render as a broken player.
		return nil, fmt.Errorf("sync preview: nothing to show at %.0fs", spec.AtSec)
	}
	return data, nil
}
