package fakes

import (
	"context"

	"github.com/evey-project/evey/internal/domain/mkv"
)

type FakeContainerWriter struct {
	MuxCalls              []mkv.MuxOptions
	DemuxCalls            []string
	AddTrackCalls         []mkv.TrackInput
	RemoveTrackCalls      []uint64
	ExtractAttachmentCalls []uint64
	Err                   error
}

func NewFakeContainerWriter() *FakeContainerWriter {
	return &FakeContainerWriter{}
}

func (w *FakeContainerWriter) Mux(_ context.Context, opts mkv.MuxOptions) error {
	if w.Err != nil {
		return w.Err
	}
	w.MuxCalls = append(w.MuxCalls, opts)
	return nil
}

func (w *FakeContainerWriter) Demux(_ context.Context, path string, _ mkv.DemuxOptions) error {
	if w.Err != nil {
		return w.Err
	}
	w.DemuxCalls = append(w.DemuxCalls, path)
	return nil
}

func (w *FakeContainerWriter) AddTrack(_ context.Context, _ string, input mkv.TrackInput) error {
	if w.Err != nil {
		return w.Err
	}
	w.AddTrackCalls = append(w.AddTrackCalls, input)
	return nil
}

func (w *FakeContainerWriter) RemoveTrack(_ context.Context, _ string, trackID uint64) error {
	if w.Err != nil {
		return w.Err
	}
	w.RemoveTrackCalls = append(w.RemoveTrackCalls, trackID)
	return nil
}

func (w *FakeContainerWriter) ExtractAttachment(_ context.Context, _ string, attachmentID uint64, _ string) error {
	if w.Err != nil {
		return w.Err
	}
	w.ExtractAttachmentCalls = append(w.ExtractAttachmentCalls, attachmentID)
	return nil
}
