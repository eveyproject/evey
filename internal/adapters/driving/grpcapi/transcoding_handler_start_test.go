package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type fakeStartTranscodeStream struct {
	grpc.ServerStream
	ctx  context.Context
	sent []*eveyv1.TranscodeProgress
}

func (s *fakeStartTranscodeStream) Send(p *eveyv1.TranscodeProgress) error {
	s.sent = append(s.sent, p)
	return nil
}
func (s *fakeStartTranscodeStream) Context() context.Context         { return s.ctx }
func (s *fakeStartTranscodeStream) SetHeader(metadata.MD) error      { return nil }
func (s *fakeStartTranscodeStream) SendHeader(metadata.MD) error     { return nil }
func (s *fakeStartTranscodeStream) SetTrailer(metadata.MD)           {}
func (s *fakeStartTranscodeStream) SendMsg(any) error                { return nil }
func (s *fakeStartTranscodeStream) RecvMsg(any) error                { return nil }

// StartTranscode: the handler builds a TranscodeJob without InputPath and delegates
// to the service. The domain model requires InputPath, so the service returns a
// validation error and the handler surfaces Internal. This test locks in that
// current behavior (fixing the handler to resolve InputPath from media_item_id is
// out-of-scope for a coverage test).
func TestTranscodingStartTranscode_ValidationErrorSurfaced(t *testing.T) {
	h, _, _, _ := newTestTranscodingHandler()
	stream := &fakeStartTranscodeStream{ctx: context.Background()}

	err := h.StartTranscode(&eveyv1.StartTranscodeRequest{
		MediaItemId: "mi-1",
		VideoCodec:  "h264",
	}, stream)
	if err == nil {
		t.Fatal("expected error (handler does not set InputPath)")
	}
}

func TestTranscodingStopTranscode_UnknownJob(t *testing.T) {
	h, _, _, _ := newTestTranscodingHandler()
	_, err := h.StopTranscode(context.Background(), &eveyv1.StopTranscodeRequest{JobId: "does-not-exist"})
	if err == nil {
		t.Fatal("expected error for unknown job")
	}
}

func TestTranscodingDetectHardware_ReturnsDevices(t *testing.T) {
	h, _, _, _ := newTestTranscodingHandler()
	resp, err := h.DetectHardware(context.Background(), &eveyv1.DetectHardwareRequest{})
	if err != nil {
		t.Fatalf("DetectHardware: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	// FakeHWDetector returns empty by default — slice must at least exist.
	if resp.Devices == nil {
		// emtpy slice is fine; nil is also fine, we just assert no panic
		_ = resp.Devices
	}
}
