package grpcapi

import (
	"context"
	"fmt"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	"github.com/gravity-zero/evey/internal/domain/transcoding"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type transcodingHandler struct {
	eveyv1.UnimplementedTranscodingServiceServer
	svc *apptranscoding.Service
}

func (h *transcodingHandler) StartTranscode(req *eveyv1.StartTranscodeRequest, stream eveyv1.TranscodingService_StartTranscodeServer) error {
	job := &transcoding.TranscodeJob{
		ID:            fmt.Sprintf("tj-%d", nowUnixNano()),
		MediaItemID:   req.MediaItemId,
		OutputCodec:   req.VideoCodec,
		HWAccelType:   hwAccelTypeFromProto[req.HwAccel],
		BitrateOut:    req.Bitrate,
		ResolutionOut: fmt.Sprintf("%dx%d", req.Width, req.Height),
	}
	if err := h.svc.StartTranscode(stream.Context(), job); err != nil {
		return status.Errorf(codes.Internal, "start transcode: %v", err)
	}
	return stream.Send(&eveyv1.TranscodeProgress{
		JobId:   job.ID,
		State:   eveyv1.TranscodeState_TRANSCODE_STATE_RUNNING,
		Percent: 0,
	})
}

func (h *transcodingHandler) StopTranscode(ctx context.Context, req *eveyv1.StopTranscodeRequest) (*eveyv1.StopTranscodeResponse, error) {
	if err := h.svc.StopTranscode(ctx, req.JobId); err != nil {
		return nil, status.Errorf(codes.Internal, "stop transcode: %v", err)
	}
	return &eveyv1.StopTranscodeResponse{}, nil
}

func (h *transcodingHandler) GetTranscodeStatus(ctx context.Context, req *eveyv1.GetTranscodeStatusRequest) (*eveyv1.TranscodeJob, error) {
	progress, err := h.svc.GetProgress(ctx, req.JobId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "job not found: %v", err)
	}
	return &eveyv1.TranscodeJob{
		Id:              progress.JobID,
		ProgressPercent: progress.Percent,
		Fps:             progress.FPS,
		Speed:           progress.Speed,
	}, nil
}

func (h *transcodingHandler) ListActiveTranscodes(ctx context.Context, _ *eveyv1.ListActiveTranscodesRequest) (*eveyv1.ListActiveTranscodesResponse, error) {
	jobs, err := h.svc.ListActiveTranscodes(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list active: %v", err)
	}
	resp := &eveyv1.ListActiveTranscodesResponse{}
	for _, j := range jobs {
		resp.Jobs = append(resp.Jobs, transcodeJobToProto(j))
	}
	return resp, nil
}

func (h *transcodingHandler) ListTranscodeProfiles(ctx context.Context, _ *eveyv1.ListTranscodeProfilesRequest) (*eveyv1.ListTranscodeProfilesResponse, error) {
	profiles, err := h.svc.ListProfiles(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list profiles: %v", err)
	}
	resp := &eveyv1.ListTranscodeProfilesResponse{}
	for _, p := range profiles {
		resp.Profiles = append(resp.Profiles, transcodeProfileToProto(p))
	}
	return resp, nil
}

func (h *transcodingHandler) DetectHardware(ctx context.Context, _ *eveyv1.DetectHardwareRequest) (*eveyv1.DetectHardwareResponse, error) {
	devices, err := h.svc.DetectHardware(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "detect hardware: %v", err)
	}
	resp := &eveyv1.DetectHardwareResponse{}
	for _, d := range devices {
		resp.Devices = append(resp.Devices, hwDeviceToProto(d))
	}
	return resp, nil
}
