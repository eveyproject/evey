package mkv

import (
	"context"

	"github.com/evey-project/evey/internal/domain/mkv"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type Service struct {
	reader mkv.ContainerReader
	writer mkv.ContainerWriter
}

func NewService(reader mkv.ContainerReader, writer mkv.ContainerWriter) *Service {
	return &Service{reader: reader, writer: writer}
}

func (s *Service) GetContainerInfo(ctx context.Context, path string) (*mkv.Container, error) {
	ctx, span := tracing.StartSpan(ctx, "app.mkv.GetContainerInfo")
	defer span.End()
	return s.reader.Open(ctx, path)
}

func (s *Service) ListTracks(ctx context.Context, path string) ([]mkv.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "app.mkv.ListTracks")
	defer span.End()
	return s.reader.ReadTracks(ctx, path)
}

func (s *Service) ListChapters(ctx context.Context, path string) ([]mkv.Chapter, error) {
	ctx, span := tracing.StartSpan(ctx, "app.mkv.ListChapters")
	defer span.End()
	return s.reader.ReadChapters(ctx, path)
}

func (s *Service) ListAttachments(ctx context.Context, path string) ([]mkv.Attachment, error) {
	ctx, span := tracing.StartSpan(ctx, "app.mkv.ListAttachments")
	defer span.End()
	return s.reader.ReadAttachments(ctx, path)
}

func (s *Service) ExtractTrack(ctx context.Context, containerPath string, trackID uint64, outputDir string) error {
	ctx, span := tracing.StartSpan(ctx, "app.mkv.ExtractTrack")
	defer span.End()
	return s.writer.ExtractAttachment(ctx, containerPath, trackID, outputDir)
}

func (s *Service) ExtractAttachment(ctx context.Context, containerPath string, attachmentID uint64, destPath string) error {
	ctx, span := tracing.StartSpan(ctx, "app.mkv.ExtractAttachment")
	defer span.End()
	return s.writer.ExtractAttachment(ctx, containerPath, attachmentID, destPath)
}

func (s *Service) MuxContainer(ctx context.Context, opts mkv.MuxOptions) error {
	ctx, span := tracing.StartSpan(ctx, "app.mkv.MuxContainer")
	defer span.End()
	return s.writer.Mux(ctx, opts)
}

func (s *Service) DemuxContainer(ctx context.Context, path string, opts mkv.DemuxOptions) error {
	ctx, span := tracing.StartSpan(ctx, "app.mkv.DemuxContainer")
	defer span.End()
	return s.writer.Demux(ctx, path, opts)
}

func (s *Service) AddTrack(ctx context.Context, containerPath string, input mkv.TrackInput) error {
	ctx, span := tracing.StartSpan(ctx, "app.mkv.AddTrack")
	defer span.End()
	return s.writer.AddTrack(ctx, containerPath, input)
}

func (s *Service) RemoveTrack(ctx context.Context, containerPath string, trackID uint64) error {
	ctx, span := tracing.StartSpan(ctx, "app.mkv.RemoveTrack")
	defer span.End()
	return s.writer.RemoveTrack(ctx, containerPath, trackID)
}
