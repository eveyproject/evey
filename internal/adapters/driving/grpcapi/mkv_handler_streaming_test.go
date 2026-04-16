package grpcapi

import (
	"context"
	"errors"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type fakeExtractTrackStream struct {
	grpc.ServerStream
	ctx  context.Context
	sent []*eveyv1.ExtractDataChunk
}

func (s *fakeExtractTrackStream) Send(c *eveyv1.ExtractDataChunk) error {
	s.sent = append(s.sent, c)
	return nil
}
func (s *fakeExtractTrackStream) Context() context.Context { return s.ctx }
func (s *fakeExtractTrackStream) SetHeader(metadata.MD) error  { return nil }
func (s *fakeExtractTrackStream) SendHeader(metadata.MD) error { return nil }
func (s *fakeExtractTrackStream) SetTrailer(metadata.MD)       {}
func (s *fakeExtractTrackStream) SendMsg(any) error            { return nil }
func (s *fakeExtractTrackStream) RecvMsg(any) error            { return nil }

type fakeMuxStream struct {
	grpc.ServerStream
	ctx  context.Context
	sent []*eveyv1.MuxProgress
}

func (s *fakeMuxStream) Send(p *eveyv1.MuxProgress) error {
	s.sent = append(s.sent, p)
	return nil
}
func (s *fakeMuxStream) Context() context.Context         { return s.ctx }
func (s *fakeMuxStream) SetHeader(metadata.MD) error      { return nil }
func (s *fakeMuxStream) SendHeader(metadata.MD) error     { return nil }
func (s *fakeMuxStream) SetTrailer(metadata.MD)           {}
func (s *fakeMuxStream) SendMsg(any) error                { return nil }
func (s *fakeMuxStream) RecvMsg(any) error                { return nil }

type fakeDemuxStream struct {
	grpc.ServerStream
	ctx  context.Context
	sent []*eveyv1.DemuxProgress
}

func (s *fakeDemuxStream) Send(p *eveyv1.DemuxProgress) error {
	s.sent = append(s.sent, p)
	return nil
}
func (s *fakeDemuxStream) Context() context.Context         { return s.ctx }
func (s *fakeDemuxStream) SetHeader(metadata.MD) error      { return nil }
func (s *fakeDemuxStream) SendHeader(metadata.MD) error     { return nil }
func (s *fakeDemuxStream) SetTrailer(metadata.MD)           {}
func (s *fakeDemuxStream) SendMsg(any) error                { return nil }
func (s *fakeDemuxStream) RecvMsg(any) error                { return nil }

func TestMKVExtractTrackStreamSuccess(t *testing.T) {
	h, _, _ := newTestMKVHandler()
	stream := &fakeExtractTrackStream{ctx: context.Background()}
	req := &eveyv1.ExtractTrackRequest{Path: "/media/movie.mkv", TrackId: 2}
	if err := h.ExtractTrack(req, stream); err != nil {
		t.Fatalf("ExtractTrack: %v", err)
	}
	if len(stream.sent) != 1 {
		t.Fatalf("expected 1 chunk sent, got %d", len(stream.sent))
	}
}

func TestMKVExtractTrackStreamError(t *testing.T) {
	h, _, writer := newTestMKVHandler()
	writer.Err = errors.New("extract boom")
	stream := &fakeExtractTrackStream{ctx: context.Background()}
	req := &eveyv1.ExtractTrackRequest{Path: "/media/movie.mkv", TrackId: 2}
	if err := h.ExtractTrack(req, stream); err == nil {
		t.Fatal("expected error from ExtractTrack")
	}
	if len(stream.sent) != 0 {
		t.Errorf("expected no chunk on error, got %d", len(stream.sent))
	}
}

func TestMKVMuxContainerStreamSuccess(t *testing.T) {
	h, _, writer := newTestMKVHandler()
	stream := &fakeMuxStream{ctx: context.Background()}
	req := &eveyv1.MuxContainerRequest{
		OutputPath: "/out/final.mkv",
		Inputs: []*eveyv1.MuxInput{
			{Path: "/in/a.mkv", TrackIds: []int32{1, 2}},
			{Path: "/in/b.mkv", TrackIds: []int32{3}},
		},
		Chapters: []*eveyv1.Chapter{
			{Id: "1", Title: "Intro", StartMs: 0, EndMs: 5000},
		},
	}
	if err := h.MuxContainer(req, stream); err != nil {
		t.Fatalf("MuxContainer: %v", err)
	}
	if len(stream.sent) != 1 {
		t.Fatalf("expected 1 progress chunk, got %d", len(stream.sent))
	}
	if stream.sent[0].Percent != 100 || stream.sent[0].Phase != "complete" {
		t.Errorf("unexpected progress: %+v", stream.sent[0])
	}
	if len(writer.MuxCalls) != 1 {
		t.Fatalf("expected 1 mux call, got %d", len(writer.MuxCalls))
	}
	opts := writer.MuxCalls[0]
	if opts.OutputPath != "/out/final.mkv" {
		t.Errorf("unexpected output path: %q", opts.OutputPath)
	}
	if len(opts.Tracks) != 3 {
		t.Errorf("expected 3 tracks flattened, got %d", len(opts.Tracks))
	}
	if len(opts.Chapters) != 1 || opts.Chapters[0].Title != "Intro" {
		t.Errorf("unexpected chapters: %+v", opts.Chapters)
	}
}

func TestMKVMuxContainerStreamError(t *testing.T) {
	h, _, writer := newTestMKVHandler()
	writer.Err = errors.New("mux boom")
	stream := &fakeMuxStream{ctx: context.Background()}
	req := &eveyv1.MuxContainerRequest{OutputPath: "/out/final.mkv"}
	if err := h.MuxContainer(req, stream); err == nil {
		t.Fatal("expected error")
	}
	if len(stream.sent) != 0 {
		t.Errorf("expected no progress on error, got %d", len(stream.sent))
	}
}

func TestMKVDemuxContainerStreamSuccess(t *testing.T) {
	h, _, writer := newTestMKVHandler()
	stream := &fakeDemuxStream{ctx: context.Background()}
	req := &eveyv1.DemuxContainerRequest{
		Path:      "/media/movie.mkv",
		OutputDir: "/out",
		TrackIds:  []int32{1, 2, 3},
	}
	if err := h.DemuxContainer(req, stream); err != nil {
		t.Fatalf("DemuxContainer: %v", err)
	}
	if len(stream.sent) != 1 {
		t.Fatalf("expected 1 progress chunk, got %d", len(stream.sent))
	}
	if stream.sent[0].Percent != 100 {
		t.Errorf("unexpected progress: %+v", stream.sent[0])
	}
	if len(writer.DemuxCalls) != 1 || writer.DemuxCalls[0] != "/media/movie.mkv" {
		t.Errorf("expected demux call on /media/movie.mkv, got %+v", writer.DemuxCalls)
	}
}

func TestMKVDemuxContainerStreamError(t *testing.T) {
	h, _, writer := newTestMKVHandler()
	writer.Err = errors.New("demux boom")
	stream := &fakeDemuxStream{ctx: context.Background()}
	req := &eveyv1.DemuxContainerRequest{Path: "/media/movie.mkv", OutputDir: "/out"}
	if err := h.DemuxContainer(req, stream); err == nil {
		t.Fatal("expected error")
	}
	if len(stream.sent) != 0 {
		t.Errorf("expected no progress on error, got %d", len(stream.sent))
	}
}

func TestMKVAddTrackSuccess(t *testing.T) {
	h, _, writer := newTestMKVHandler()
	req := &eveyv1.AddTrackRequest{
		ContainerPath: "/media/movie.mkv",
		TrackPath:     "/tracks/fre.srt",
		Language:      "fre",
		Name:          "French",
		IsDefault:     true,
	}
	resp, err := h.AddTrack(context.Background(), req)
	if err != nil {
		t.Fatalf("AddTrack: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if len(writer.AddTrackCalls) != 1 {
		t.Fatalf("expected 1 AddTrack call, got %d", len(writer.AddTrackCalls))
	}
	got := writer.AddTrackCalls[0]
	if got.SourcePath != "/tracks/fre.srt" || got.Language != "fre" || got.Name != "French" || !got.IsDefault {
		t.Errorf("unexpected AddTrack input: %+v", got)
	}
}

func TestMKVAddTrackError(t *testing.T) {
	h, _, writer := newTestMKVHandler()
	writer.Err = errors.New("add boom")
	_, err := h.AddTrack(context.Background(), &eveyv1.AddTrackRequest{
		ContainerPath: "/media/movie.mkv",
		TrackPath:     "/tracks/fre.srt",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMKVRemoveTrackSuccess(t *testing.T) {
	h, _, writer := newTestMKVHandler()
	req := &eveyv1.RemoveTrackRequest{ContainerPath: "/media/movie.mkv", TrackId: 3}
	resp, err := h.RemoveTrack(context.Background(), req)
	if err != nil {
		t.Fatalf("RemoveTrack: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if len(writer.RemoveTrackCalls) != 1 || writer.RemoveTrackCalls[0] != 3 {
		t.Errorf("expected RemoveTrack(3), got %+v", writer.RemoveTrackCalls)
	}
}

func TestMKVRemoveTrackError(t *testing.T) {
	h, _, writer := newTestMKVHandler()
	writer.Err = errors.New("remove boom")
	_, err := h.RemoveTrack(context.Background(), &eveyv1.RemoveTrackRequest{
		ContainerPath: "/media/movie.mkv",
		TrackId:       3,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
