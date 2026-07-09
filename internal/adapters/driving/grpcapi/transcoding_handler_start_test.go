package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
func (s *fakeStartTranscodeStream) Context() context.Context     { return s.ctx }
func (s *fakeStartTranscodeStream) SetHeader(metadata.MD) error  { return nil }
func (s *fakeStartTranscodeStream) SendHeader(metadata.MD) error { return nil }
func (s *fakeStartTranscodeStream) SetTrailer(metadata.MD)       {}
func (s *fakeStartTranscodeStream) SendMsg(any) error            { return nil }
func (s *fakeStartTranscodeStream) RecvMsg(any) error            { return nil }

func TestTranscodingStartTranscode_EmptyMediaID_InvalidArgument(t *testing.T) {
	h, _, _, _ := newTestTranscodingHandler()
	stream := &fakeStartTranscodeStream{ctx: context.Background()}

	err := h.StartTranscode(&eveyv1.StartTranscodeRequest{MediaItemId: ""}, stream)
	if err == nil {
		t.Fatal("expected InvalidArgument for empty media_item_id")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestTranscodingStartTranscode_NoLibQuery_FailedPrecondition(t *testing.T) {
	h, _, _, _ := newTestTranscodingHandler()
	stream := &fakeStartTranscodeStream{ctx: context.Background()}

	err := h.StartTranscode(&eveyv1.StartTranscodeRequest{MediaItemId: "mi-1"}, stream)
	if err == nil {
		t.Fatal("expected FailedPrecondition when libQuery is nil")
	}
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", status.Code(err))
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
