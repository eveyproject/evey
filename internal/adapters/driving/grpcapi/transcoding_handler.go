package grpcapi

import (
	"context"
	"fmt"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	"github.com/gravity-zero/evey/internal/domain/transcoding"
	"github.com/gravity-zero/evey/internal/pkg/id"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type transcodingHandler struct {
	eveyv1.UnimplementedTranscodingServiceServer
	svc       *apptranscoding.Service
	userQuery *appuser.QueryService
}

func (h *transcodingHandler) StartTranscode(req *eveyv1.StartTranscodeRequest, stream eveyv1.TranscodingService_StartTranscodeServer) error {
	bitrateOut := req.Bitrate
	if h.userQuery != nil {
		if claims := interceptors.ClaimsFromContext(stream.Context()); claims != nil && claims.ProfileID != "" {
			if prof, err := h.userQuery.GetProfileByID(stream.Context(), claims.ProfileID); err == nil && prof.MaxBitrateKbps != nil {
				capKbps := int64(*prof.MaxBitrateKbps) * 1000
				if bitrateOut == 0 || bitrateOut > capKbps {
					bitrateOut = capKbps
				}
			}
		}
	}
	job := &transcoding.TranscodeJob{
		ID:            id.New("tj-"),
		MediaItemID:   req.MediaItemId,
		OutputCodec:   req.VideoCodec,
		AudioCodec:    req.AudioCodec,
		HWAccelType:   hwAccelTypeFromProto[req.HwAccel],
		BitrateOut:    bitrateOut,
		ResolutionOut: fmt.Sprintf("%dx%d", req.Width, req.Height),
	}
	if err := h.svc.StartTranscode(stream.Context(), job); err != nil {
		return internalError("start transcode", err)
	}
	return stream.Send(&eveyv1.TranscodeProgress{
		JobId:   job.ID,
		State:   eveyv1.TranscodeState_TRANSCODE_STATE_RUNNING,
		Percent: 0,
	})
}

func (h *transcodingHandler) StopTranscode(ctx context.Context, req *eveyv1.StopTranscodeRequest) (*eveyv1.StopTranscodeResponse, error) {
	if err := h.svc.StopTranscode(ctx, req.JobId); err != nil {
		return nil, internalError("stop transcode", err)
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
		return nil, internalError("list active", err)
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
		return nil, internalError("list profiles", err)
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
		return nil, internalError("detect hardware", err)
	}
	resp := &eveyv1.DetectHardwareResponse{}
	for _, d := range devices {
		resp.Devices = append(resp.Devices, hwDeviceToProto(d))
	}
	return resp, nil
}
