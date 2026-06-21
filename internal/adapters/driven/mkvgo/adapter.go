package mkvgo

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/ops"
	"github.com/gravity-zero/mkvgo/mkv/reader"
	"github.com/gravity-zero/mkvgo/mp4"

	"github.com/evey-project/evey/internal/domain/mkv"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

// ebmlContainerExts holds the file extensions backed by an EBML container
// (Matroska / WebM family). The mkvgo reader can only parse these — calling
// it on a non-EBML file (AVI, MP4, MOV, …) fails with an "expected EBML
// header" error. Callers should treat non-EBML files as containers with no
// chapters/attachments rather than surfacing an error.
var ebmlContainerExts = map[string]bool{
	".mkv": true, ".mka": true, ".mks": true, ".mk3d": true, ".webm": true,
}

// IsEBMLContainer reports whether path's extension is an EBML container the
// mkvgo reader can parse. Callers can skip the read entirely for false
// returns and treat the container as empty.
func IsEBMLContainer(path string) bool {
	return ebmlContainerExts[strings.ToLower(filepath.Ext(path))]
}

// mp4ContainerExts holds the ISO base media file format extensions the mkvgo
// mp4 reader (OpenMeta / RemuxFromMP4) can parse. Kept to the unambiguous
// ISO-BMFF members: .mov (QuickTime, an ISO-BMFF sibling) is deliberately left
// out so a misparse never silently mislabels a track — it falls back to ffprobe.
var mp4ContainerExts = map[string]bool{
	".mp4": true, ".m4v": true,
}

// IsMP4Container reports whether path's extension is an ISO-BMFF container the
// mkvgo mp4 metadata reader (mp4.OpenMeta) can parse head-only.
func IsMP4Container(path string) bool {
	return mp4ContainerExts[strings.ToLower(filepath.Ext(path))]
}

// --- ContainerReader ---

func (a *Adapter) Open(ctx context.Context, path string) (*mkv.Container, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.Open")
	defer span.End()

	if !IsEBMLContainer(path) {
		return &mkv.Container{Path: path}, nil
	}
	c, err := reader.Open(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("mkvgo open: %w", err)
	}
	return fromExtContainer(c, path), nil
}

func (a *Adapter) ReadTracks(ctx context.Context, path string) ([]mkv.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.ReadTracks")
	defer span.End()

	c, err := a.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.Tracks, nil
}

func (a *Adapter) ReadChapters(ctx context.Context, path string) ([]mkv.Chapter, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.ReadChapters")
	defer span.End()

	c, err := a.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.Chapters, nil
}

func (a *Adapter) ReadAttachments(ctx context.Context, path string) ([]mkv.Attachment, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.ReadAttachments")
	defer span.End()

	c, err := a.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.Attachments, nil
}

func (a *Adapter) ReadTags(ctx context.Context, path string) ([]mkv.Tag, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.ReadTags")
	defer span.End()

	c, err := a.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.Tags, nil
}

// --- ContainerWriter ---

func (a *Adapter) Mux(ctx context.Context, opts mkv.MuxOptions) error {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.Mux")
	defer span.End()

	extOpts := extmkv.MuxOptions{
		OutputPath: opts.OutputPath,
	}
	for _, t := range opts.Tracks {
		extOpts.Tracks = append(extOpts.Tracks, toExtTrackInput(t))
	}
	return ops.Mux(ctx, extOpts)
}

func (a *Adapter) Demux(ctx context.Context, path string, opts mkv.DemuxOptions) error {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.Demux")
	defer span.End()

	extOpts := extmkv.DemuxOptions{
		SourcePath: path,
		OutputDir:  opts.OutputDir,
		TrackIDs:   opts.TrackIDs,
	}
	return ops.Demux(ctx, extOpts)
}

func (a *Adapter) AddTrack(ctx context.Context, containerPath string, input mkv.TrackInput) error {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.AddTrack")
	defer span.End()

	tmpOut := containerPath + ".tmp.mkv"
	if err := ops.AddTrack(ctx, containerPath, tmpOut, toExtTrackInput(input)); err != nil {
		return fmt.Errorf("mkvgo add track: %w", err)
	}
	return os.Rename(tmpOut, containerPath)
}

func (a *Adapter) RemoveTrack(ctx context.Context, containerPath string, trackID uint64) error {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.RemoveTrack")
	defer span.End()

	tmpOut := containerPath + ".tmp.mkv"
	if err := ops.RemoveTrack(ctx, containerPath, tmpOut, []uint64{trackID}); err != nil {
		return fmt.Errorf("mkvgo remove track: %w", err)
	}
	return os.Rename(tmpOut, containerPath)
}

func (a *Adapter) ExtractAttachment(ctx context.Context, containerPath string, attachmentID uint64, destPath string) error {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.ExtractAttachment")
	defer span.End()

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	return ops.ExtractAttachment(ctx, containerPath, attachmentID, destPath)
}

// --- MP4 remux ---

// CanRemux reports whether path is a source mkvgo can repackage into MP4 — an
// EBML container (Matroska / WebM). Any other container must go through a
// transcode instead.
func (a *Adapter) CanRemux(path string) bool {
	return IsEBMLContainer(path)
}

// RemuxToMP4 repackages the EBML (Matroska/WebM) file at sourcePath into a
// fast-start MP4 at outputPath WITHOUT re-encoding: each track's compressed
// samples are copied verbatim and the moov box is written before mdat, so a
// player can start over progressive HTTP without first reading to EOF. Tracks
// in codecs MP4 cannot carry (TrueHD audio, bitmap subtitles, …) are dropped
// rather than aborting the whole remux — the output still plays, minus those
// tracks (each logged via OnDrop).
func (a *Adapter) RemuxToMP4(ctx context.Context, sourcePath, outputPath string) error {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.RemuxToMP4")
	defer span.End()

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	err := mp4.RemuxToMP4(ctx, sourcePath, outputPath, mp4.Options{
		FastStart:       true,
		SkipUnsupported: true,
		OnDrop: func(d mp4.DroppedTrack) {
			slog.WarnContext(ctx, "mkvgo remux: dropped track",
				slog.Uint64("track_id", d.ID),
				slog.String("codec", d.Codec),
				slog.String("reason", d.Reason))
		},
	})
	if err != nil {
		return fmt.Errorf("mkvgo remux to mp4: %w", err)
	}
	return nil
}

// --- converters ---

func fromExtContainer(c *extmkv.Container, path string) *mkv.Container {
	// Probe values from the external library are usually >= 0 but a corrupt
	// segment could surface a negative value; log and clamp to zero rather
	// than fail the read so the rest of the metadata stays usable.
	dur, err := shared.NewDurationMs(c.DurationMs)
	if err != nil {
		slog.Warn("mkvgo: negative container duration", slog.String("path", path), slog.Int64("ms", c.DurationMs))
		dur = shared.Duration{}
	}
	out := &mkv.Container{
		Path:     path,
		Duration: dur,
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
