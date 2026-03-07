package ebml

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/gravity-zero/evey/internal/domain/mkv"
)

type Writer struct {
	ffmpegPath string
}

func NewWriter(ffmpegPath string) *Writer {
	return &Writer{ffmpegPath: ffmpegPath}
}

func (w *Writer) Mux(ctx context.Context, opts mkv.MuxOptions) error {
	args := []string{"-y"}

	for _, t := range opts.Tracks {
		args = append(args, "-i", t.SourcePath)
	}

	for i, t := range opts.Tracks {
		args = append(args, "-map", strconv.Itoa(i))
		if t.Language != "" {
			args = append(args, fmt.Sprintf("-metadata:s:%d", i), "language="+t.Language)
		}
		if t.Name != "" {
			args = append(args, fmt.Sprintf("-metadata:s:%d", i), "title="+t.Name)
		}
	}

	args = append(args, "-c", "copy", opts.OutputPath)

	cmd := exec.CommandContext(ctx, w.ffmpegPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg mux: %w: %s", err, string(out))
	}
	return nil
}

func (w *Writer) Demux(ctx context.Context, path string, opts mkv.DemuxOptions) error {
	args := []string{"-i", path}

	for _, tid := range opts.TrackIDs {
		args = append(args, "-map", fmt.Sprintf("0:%d", tid))
	}

	args = append(args, "-c", "copy")
	outFile := fmt.Sprintf("%s/demuxed_%s.mkv", opts.OutputDir, strings.ReplaceAll(fmt.Sprintf("%v", opts.TrackIDs), " ", "_"))
	args = append(args, "-y", outFile)

	cmd := exec.CommandContext(ctx, w.ffmpegPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg demux: %w: %s", err, string(out))
	}
	return nil
}

func (w *Writer) AddTrack(ctx context.Context, containerPath string, input mkv.TrackInput) error {
	tmpOut := containerPath + ".tmp.mkv"
	args := []string{
		"-i", containerPath,
		"-i", input.SourcePath,
		"-map", "0", "-map", "1",
		"-c", "copy",
	}
	if input.Language != "" {
		args = append(args, "-metadata:s:a:0", "language="+input.Language)
	}
	args = append(args, "-y", tmpOut)

	cmd := exec.CommandContext(ctx, w.ffmpegPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg add track: %w: %s", err, string(out))
	}

	if err := os.Rename(tmpOut, containerPath); err != nil {
		return fmt.Errorf("replace original: %w", err)
	}
	return nil
}

func (w *Writer) RemoveTrack(ctx context.Context, containerPath string, trackID uint64) error {
	tmpOut := containerPath + ".tmp.mkv"
	args := []string{
		"-i", containerPath,
		"-map", "0",
		"-map", fmt.Sprintf("-0:%d", trackID),
		"-c", "copy",
		"-y", tmpOut,
	}

	cmd := exec.CommandContext(ctx, w.ffmpegPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg remove track: %w: %s", err, string(out))
	}

	if err := os.Rename(tmpOut, containerPath); err != nil {
		return fmt.Errorf("replace original: %w", err)
	}
	return nil
}

func (w *Writer) ExtractAttachment(ctx context.Context, containerPath string, attachmentID uint64, destPath string) error {
	args := []string{
		"-dump_attachment:" + strconv.FormatUint(attachmentID, 10), destPath,
		"-i", containerPath,
		"-y",
	}

	cmd := exec.CommandContext(ctx, w.ffmpegPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg extract attachment: %w: %s", err, string(out))
	}
	return nil
}
