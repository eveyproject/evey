package mkvgo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/ops"
	"github.com/gravity-zero/mkvgo/mkv/reader"

	"github.com/gravity-zero/evey/internal/domain/mkv"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

// --- ContainerReader ---

func (a *Adapter) Open(ctx context.Context, path string) (*mkv.Container, error) {
	c, err := reader.Open(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("mkvgo open: %w", err)
	}
	return fromExtContainer(c, path), nil
}

func (a *Adapter) ReadTracks(ctx context.Context, path string) ([]mkv.Track, error) {
	c, err := a.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.Tracks, nil
}

func (a *Adapter) ReadChapters(ctx context.Context, path string) ([]mkv.Chapter, error) {
	c, err := a.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.Chapters, nil
}

func (a *Adapter) ReadAttachments(ctx context.Context, path string) ([]mkv.Attachment, error) {
	c, err := a.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.Attachments, nil
}

func (a *Adapter) ReadTags(ctx context.Context, path string) ([]mkv.Tag, error) {
	c, err := a.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.Tags, nil
}

// --- ContainerWriter ---

func (a *Adapter) Mux(ctx context.Context, opts mkv.MuxOptions) error {
	extOpts := extmkv.MuxOptions{
		OutputPath: opts.OutputPath,
	}
	for _, t := range opts.Tracks {
		extOpts.Tracks = append(extOpts.Tracks, toExtTrackInput(t))
	}
	return ops.Mux(ctx, extOpts)
}

func (a *Adapter) Demux(ctx context.Context, path string, opts mkv.DemuxOptions) error {
	extOpts := extmkv.DemuxOptions{
		SourcePath: path,
		OutputDir:  opts.OutputDir,
		TrackIDs:   opts.TrackIDs,
	}
	return ops.Demux(ctx, extOpts)
}

func (a *Adapter) AddTrack(ctx context.Context, containerPath string, input mkv.TrackInput) error {
	tmpOut := containerPath + ".tmp.mkv"
	if err := ops.AddTrack(ctx, containerPath, tmpOut, toExtTrackInput(input)); err != nil {
		return fmt.Errorf("mkvgo add track: %w", err)
	}
	return os.Rename(tmpOut, containerPath)
}

func (a *Adapter) RemoveTrack(ctx context.Context, containerPath string, trackID uint64) error {
	tmpOut := containerPath + ".tmp.mkv"
	if err := ops.RemoveTrack(ctx, containerPath, tmpOut, []uint64{trackID}); err != nil {
		return fmt.Errorf("mkvgo remove track: %w", err)
	}
	return os.Rename(tmpOut, containerPath)
}

func (a *Adapter) ExtractAttachment(ctx context.Context, containerPath string, attachmentID uint64, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	return ops.ExtractAttachment(ctx, containerPath, attachmentID, destPath)
}

// --- converters ---

func fromExtContainer(c *extmkv.Container, path string) *mkv.Container {
	out := &mkv.Container{
		Path:       path,
		DurationMs: c.DurationMs,
		Info: mkv.SegmentInfo{
			Title:         c.Info.Title,
			MuxingApp:     c.Info.MuxingApp,
			WritingApp:    c.Info.WritingApp,
			Duration:      c.Info.Duration,
			TimecodeScale: c.Info.TimecodeScale,
			DateUTC:       c.Info.DateUTC,
		},
	}
	for _, t := range c.Tracks {
		out.Tracks = append(out.Tracks, fromExtTrack(t))
	}
	for _, ch := range c.Chapters {
		out.Chapters = append(out.Chapters, fromExtChapter(ch))
	}
	for _, att := range c.Attachments {
		out.Attachments = append(out.Attachments, mkv.Attachment{
			ID:       att.ID,
			Name:     att.Name,
			MIMEType: att.MIMEType,
			Size:     att.Size,
			Data:     att.Data,
		})
	}
	for _, tag := range c.Tags {
		out.Tags = append(out.Tags, fromExtTag(tag))
	}
	return out
}

func fromExtTrack(t extmkv.Track) mkv.Track {
	out := mkv.Track{
		ID:        t.ID,
		Type:      mkv.TrackType(t.Type),
		Codec:     t.Codec,
		Language:  t.Language,
		Name:      t.Name,
		IsDefault: t.IsDefault,
		IsForced:  t.IsForced,
	}
	if t.Width != nil {
		w := *t.Width
		out.Width = &w
	}
	if t.Height != nil {
		h := *t.Height
		out.Height = &h
	}
	if t.Channels != nil {
		ch := *t.Channels
		out.Channels = &ch
	}
	if t.SampleRate != nil {
		sr := *t.SampleRate
		out.SampleRate = &sr
	}
	if t.BitDepth != nil {
		bd := *t.BitDepth
		out.BitDepth = &bd
	}
	return out
}

func fromExtChapter(ch extmkv.Chapter) mkv.Chapter {
	return mkv.Chapter{
		ID:      ch.ID,
		Title:   ch.Title,
		StartMs: ch.StartMs,
		EndMs:   ch.EndMs,
	}
}

func fromExtTag(tag extmkv.Tag) mkv.Tag {
	out := mkv.Tag{
		TargetType: tag.TargetType,
		TargetID:   tag.TargetID,
	}
	for _, st := range tag.SimpleTags {
		out.SimpleTag = append(out.SimpleTag, mkv.SimpleTag{
			Name:     st.Name,
			Value:    st.Value,
			Language: st.Language,
		})
	}
	return out
}

func toExtTrackInput(t mkv.TrackInput) extmkv.TrackInput {
	return extmkv.TrackInput{
		SourcePath: t.SourcePath,
		TrackID:    t.TrackID,
		Language:   t.Language,
		Name:       t.Name,
		IsDefault:  t.IsDefault,
	}
}
