package ebml

import (
	"context"
	"errors"

	"github.com/gravity-zero/evey/internal/domain/mkv"
)

type Writer struct{}

func NewWriter() *Writer {
	return &Writer{}
}

func (w *Writer) Mux(_ context.Context, _ mkv.MuxOptions) error {
	return errors.New("TODO: implement EBML muxing")
}

func (w *Writer) Demux(_ context.Context, _ string, _ mkv.DemuxOptions) error {
	return errors.New("TODO: implement EBML demuxing")
}

func (w *Writer) AddTrack(_ context.Context, _ string, _ mkv.TrackInput) error {
	return errors.New("TODO: implement EBML add track")
}

func (w *Writer) RemoveTrack(_ context.Context, _ string, _ uint64) error {
	return errors.New("TODO: implement EBML remove track")
}

func (w *Writer) ExtractAttachment(_ context.Context, _ string, _ uint64, _ string) error {
	return errors.New("TODO: implement EBML extract attachment")
}
