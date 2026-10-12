package library

import (
	"context"
	"log/slog"
	"strings"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/mkv"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/google/uuid"
)

type MetadataExtractionService struct {
	mkvReader    mkv.ContainerReader
	chapters     library.ChapterRepository
	mediaStreams library.MediaStreamRepository
	subScanner   library.ExternalSubtitleScanner
}

func NewMetadataExtractionService(
	chapters library.ChapterRepository,
	reader mkv.ContainerReader,
	mediaStreams library.MediaStreamRepository,
) *MetadataExtractionService {
	return &MetadataExtractionService{
		chapters:     chapters,
		mkvReader:    reader,
		mediaStreams: mediaStreams,
	}
}

func (s *MetadataExtractionService) WithSubtitleScanner(sc library.ExternalSubtitleScanner) {
	s.subScanner = sc
}

func (s *MetadataExtractionService) extractAndStoreChapters(ctx context.Context, mediaItemID, filePath string) {
	if s.chapters == nil || s.mkvReader == nil || !strings.HasSuffix(strings.ToLower(filePath), ".mkv") {
		return
	}
	mkvChapters, err := s.mkvReader.ReadChapters(ctx, filePath)
	if err != nil || len(mkvChapters) == 0 {
		return
	}
	chapters := make([]*library.Chapter, len(mkvChapters))
	for i, ch := range mkvChapters {
		chapters[i] = &library.Chapter{
			ID:          uuid.New().String(),
			MediaItemID: shared.MediaItemID(mediaItemID),
			Title:       ch.Title,
			StartTimeMs: ch.StartMs,
			EndTimeMs:   ch.EndMs,
			SortOrder:   i,
		}
	}
	if err := s.chapters.ReplaceBatch(ctx, shared.MediaItemID(mediaItemID), chapters); err != nil {
		slog.WarnContext(ctx, "store chapters failed", slog.String("mediaItemID", mediaItemID), slog.Any("err", err))
	}
}

func (s *MetadataExtractionService) discoverExternalSubtitles(existing []*library.MediaStream, mediaItemID, filePath string) []*library.MediaStream {
	if s.subScanner == nil {
		return nil
	}
	subs, err := s.subScanner.ScanForSubtitles(filePath)
	if err != nil || len(subs) == 0 {
		return nil
	}

	maxIdx := 0
	for _, st := range existing {
		if st.StreamIndex >= maxIdx {
			maxIdx = st.StreamIndex + 1
		}
	}

	var streams []*library.MediaStream
	for _, sub := range subs {
		path := sub.Path
		codec := sub.Format
		if codec == "vtt" {
			codec = "webvtt"
		}
		st := &library.MediaStream{
			ID:           uuid.New().String(),
			MediaItemID:  shared.MediaItemID(mediaItemID),
			StreamIndex:  maxIdx,
			StreamType:   library.Subtitle,
			Codec:        codec,
			Language:     sub.Language,
			IsForced:     sub.IsForced,
			IsExternal:   true,
			ExternalPath: &path,
		}
		streams = append(streams, st)
		maxIdx++
	}
	return streams
}
