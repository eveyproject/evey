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

	"github.com/eveyproject/evey/internal/domain/mkv"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

// ebmlContainerExts holds the file extensions backed by an EBML container
// (Matroska / WebM family). The mkvgo reader can only parse these - calling
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
// mp4 reader (OpenMeta / RemuxFromMP4) can parse. .mov (QuickTime) is included
// since mkvgo v0.12.0, which parses the non-faststart layout raw iPhone/camera
// files use (wide + mdat first, moov at the end) plus the QuickTime audio
// sample-description variants; a file it still cannot parse errors cleanly and
// falls back to ffprobe.
var mp4ContainerExts = map[string]bool{
	".mp4": true, ".m4v": true, ".m4a": true, ".mov": true,
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
	c, err := a.openMeta(ctx, path, false)
	if err != nil {
		return nil, err
	}
	return fromExtContainer(c, path), nil
}

// openMeta is the one head-only read behind every ContainerReader method:
// Info, Tracks, Tags, Chapters and the attachment TABLE come from the head,
// no cluster walk. Attachment payloads (a font set can be tens of MB) stay on
// disk unless withData: ReadAttachments is the only caller that wants them,
// and it loads each one on request.
func (a *Adapter) openMeta(ctx context.Context, path string, withData bool) (*extmkv.Container, error) {
	c, err := reader.OpenMeta(ctx, path,
		reader.WithTags(), reader.WithChapters(), reader.WithAttachments(), reader.WithoutAttachmentData())
	if err != nil {
		return nil, fmt.Errorf("mkvgo open: %w", err)
	}
	if withData {
		for i := range c.Attachments {
			if err := ops.LoadAttachmentData(ctx, &c.Attachments[i]); err != nil {
				return nil, fmt.Errorf("mkvgo open: attachment %d payload: %w", c.Attachments[i].ID, err)
			}
		}
	}
	return c, nil
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

	if !IsEBMLContainer(path) {
		return nil, nil
	}
	c, err := a.openMeta(ctx, path, true)
	if err != nil {
		return nil, err
	}
	return fromExtContainer(c, path).Attachments, nil
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

// --- MP4 remux ---

// CanRemux reports whether path is a source mkvgo can repackage into MP4 - an
// EBML container (Matroska / WebM). Any other container must go through a
// transcode instead.
func (a *Adapter) CanRemux(path string) bool {
	return IsEBMLContainer(path)
}

// RemuxToMP4 repackages the EBML (Matroska/WebM) file at sourcePath into a
// fast-start MP4 at outputPath WITHOUT re-encoding: each track's compressed
// samples are copied verbatim and the moov box is written before mdat, so a
// player can start over progressive HTTP without first reading to EOF.
//
// Subtitles: SRT and WebVTT ride along as tx3g timed text; ASS/SSA are flattened
// to tx3g too (FlattenStyledSubs) - text only, styling/positioning/karaoke is
// lost, but that beats dropping subs on the anime/MKV case this path most often
// serves (cast/download play the standalone file, no sidecar). NativeWebVTT is
// left off on purpose: native wvtt is lossless but ffmpeg's MP4 demuxer can't
// read it, and widest playback compat matters more here than cue styling.
//
// Genuinely uncarriable tracks - bitmap subtitles (PGS/VobSub), TrueHD audio -
// are dropped rather than aborting the whole remux (SkipUnsupported); the output
// still plays, minus those tracks (each logged via OnDrop).
func (a *Adapter) RemuxToMP4(ctx context.Context, sourcePath, outputPath string) error {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.RemuxToMP4")
	defer span.End()

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	err := mp4.RemuxToMP4(ctx, sourcePath, outputPath, mp4.Options{
		FastStart:         true,
		SkipUnsupported:   true,
		FlattenStyledSubs: true, // carry ASS/SSA as text-only tx3g instead of dropping
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
