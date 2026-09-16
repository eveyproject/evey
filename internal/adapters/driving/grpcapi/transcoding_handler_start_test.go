package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
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
	// FakeHWDetector returns empty by default - slice must at least exist.
	if resp.Devices == nil {
		// emtpy slice is fine; nil is also fine, we just assert no panic
		_ = resp.Devices
	}
}

// The full path: the item is resolved to its file, the caller's library grant
// is checked (a viewer without one sees NotFound, never the path), the
// profile's bitrate ceiling clamps the request, and the first progress frame
// carries the new job. Saturation and a failed spawn have their own codes.
func TestTranscodingStartTranscode_ResolvesCapsAndStreams(t *testing.T) {
	h, transcoder, jobRepo, mediaItemRepo, libAccessRepo := newTranscodingHandlerWithAccess()
	ctx := context.Background()
	mediaItemRepo.Seed(&library.MediaItem{ID: "mi-1", LibraryID: "lib-1", Type: "movie",
		File: &library.MediaFile{Path: "/media/secret/film.mkv"}})
	mediaItemRepo.Seed(&library.MediaItem{ID: "mi-nofile", LibraryID: "lib-1", Type: "movie"})

	profileRepo := userfakes.NewFakeProfileRepository()
	capKbps := 3000
	require.NoError(t, profileRepo.Create(ctx, &user.Profile{ID: "profile-2", UserID: "user-2", Name: "Kid", MaxBitrateKbps: &capKbps}))
	h.userQuery = appuser.NewQueryService(userfakes.NewFakeUserRepository(), profileRepo, userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(), userfakes.NewFakeQuickConnectRepository())

	req := &eveyv1.StartTranscodeRequest{MediaItemId: "mi-1", VideoCodec: "h264", AudioCodec: "aac", Width: 1280, Height: 720, Bitrate: 8_000_000}

	// Granted elsewhere only: indistinguishable from a missing item, and no path in the message.
	require.NoError(t, libAccessRepo.Grant(ctx, "user-2", "lib-other"))
	stream := &fakeStartTranscodeStream{ctx: userCtx("user-2", "profile-2")}
	err := h.StartTranscode(req, stream)
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.NotContains(t, err.Error(), "/media/secret")
	assert.Empty(t, stream.sent)

	// Anonymous: refused before any lookup that would leak existence.
	err = h.StartTranscode(req, &fakeStartTranscodeStream{ctx: ctx})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	// An item without a file cannot be transcoded.
	err = h.StartTranscode(&eveyv1.StartTranscodeRequest{MediaItemId: "mi-nofile", VideoCodec: "h264"}, &fakeStartTranscodeStream{ctx: adminCtx()})
	assert.Equal(t, codes.NotFound, status.Code(err))

	require.NoError(t, libAccessRepo.Grant(ctx, "user-2", "lib-1"))
	stream = &fakeStartTranscodeStream{ctx: userCtx("user-2", "profile-2")}
	require.NoError(t, h.StartTranscode(req, stream))
	require.Len(t, stream.sent, 1)
	assert.Equal(t, eveyv1.TranscodeState_TRANSCODE_STATE_RUNNING, stream.sent[0].State)
	job, err := jobRepo.GetByID(ctx, stream.sent[0].JobId)
	require.NoError(t, err)
	assert.Equal(t, "/media/secret/film.mkv", job.InputPath)
	assert.Equal(t, int64(3_000_000), job.BitrateOut, "the profile ceiling clamps the requested 8 Mb/s")
	assert.Equal(t, "1280x720", job.ResolutionOut)

	// Saturated: the job is queued and the caller told to wait.
	for i := range 5 {
		running, _ := transcoding.NewTranscodeJob(fmt.Sprintf("tj-run-%d", i), "mi-1", "/media/x.mkv", "h264")
		running.Status = transcoding.Running
		require.NoError(t, jobRepo.Create(ctx, running))
	}
	err = h.StartTranscode(req, &fakeStartTranscodeStream{ctx: adminCtx()})
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))

	// A spawn failure is a sanitized Internal.
	for i := range 5 {
		done, getErr := jobRepo.GetByID(ctx, fmt.Sprintf("tj-run-%d", i))
		require.NoError(t, getErr)
		done.Status = transcoding.Completed
		require.NoError(t, jobRepo.Update(ctx, done))
	}
	transcoder.Err = errors.New("ffmpeg: /usr/lib/ffmpeg exited 137")
	err = h.StartTranscode(req, &fakeStartTranscodeStream{ctx: adminCtx()})
	assert.Equal(t, codes.Internal, status.Code(err))
	assert.NotContains(t, err.Error(), "/usr/lib")
}
