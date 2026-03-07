package mkv

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/mkv"
)

type Service struct {
	reader mkv.ContainerReader
	writer mkv.ContainerWriter
}

func NewService(reader mkv.ContainerReader, writer mkv.ContainerWriter) *Service {
	return &Service{reader: reader, writer: writer}
}

func (s *Service) GetContainerInfo(ctx context.Context, path string) (*mkv.Container, error) {
	return s.reader.Open(ctx, path)
}

func (s *Service) ListTracks(ctx context.Context, path string) ([]mkv.Track, error) {
	return s.reader.ReadTracks(ctx, path)
}

func (s *Service) ListChapters(ctx context.Context, path string) ([]mkv.Chapter, error) {
	return s.reader.ReadChapters(ctx, path)
}

func (s *Service) ListAttachments(ctx context.Context, path string) ([]mkv.Attachment, error) {
	return s.reader.ReadAttachments(ctx, path)
}

func (s *Service) ExtractTrack(ctx context.Context, containerPath string, trackID uint64, outputDir string) error {
	return s.writer.ExtractAttachment(ctx, containerPath, trackID, outputDir)
}

func (s *Service) ExtractAttachment(ctx context.Context, containerPath string, attachmentID uint64, destPath string) error {
	return s.writer.ExtractAttachment(ctx, containerPath, attachmentID, destPath)
}

func (s *Service) MuxContainer(ctx context.Context, opts mkv.MuxOptions) error {
	return s.writer.Mux(ctx, opts)
}

func (s *Service) DemuxContainer(ctx context.Context, path string, opts mkv.DemuxOptions) error {
	return s.writer.Demux(ctx, path, opts)
}

func (s *Service) AddTrack(ctx context.Context, containerPath string, input mkv.TrackInput) error {
	return s.writer.AddTrack(ctx, containerPath, input)
}

func (s *Service) RemoveTrack(ctx context.Context, containerPath string, trackID uint64) error {
	return s.writer.RemoveTrack(ctx, containerPath, trackID)
}
