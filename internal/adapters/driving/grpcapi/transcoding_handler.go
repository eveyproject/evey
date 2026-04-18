package grpcapi

import (
	"context"
	"errors"
	"fmt"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/evey-project/evey/internal/app/library"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/transcoding"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/id"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type transcodingHandler struct {
	eveyv1.UnimplementedTranscodingServiceServer
	svc        *apptranscoding.Service
	userQuery  *appuser.QueryService
	libQuery   *applibrary.QueryService
	userAccess *appuser.AccessService
	translator *eveyi18n.Translator
}

func (h *transcodingHandler) StartTranscode(req *eveyv1.StartTranscodeRequest, stream eveyv1.TranscodingService_StartTranscodeServer) error {
	ctx := stream.Context()
	if req.MediaItemId == "" {
		return status.Error(codes.InvalidArgument, "media_item_id required")
	}
	// Resolve media item: we need FilePath for InputPath + library check.
	if h.libQuery == nil {
		return status.Error(codes.FailedPrecondition, "library service not configured")
	}
	item, err := h.libQuery.GetMediaItem(ctx, req.MediaItemId)
	if err != nil || item == nil || item.File == nil || item.File.Path == "" {
		return status.Error(codes.NotFound, "media item not found")
	}
	// IDOR: restrict to libraries the caller has access to.
	if h.userAccess != nil {
		claims := interceptors.ClaimsFromContext(ctx)
		if claims == nil {
			return status.Error(codes.Unauthenticated, "unauthenticated")
		}
		if !claims.IsAdmin {
			ok, accessErr := h.userAccess.HasLibraryAccess(ctx, claims.UserID, string(item.LibraryID))
			if accessErr != nil || !ok {
				return status.Error(codes.NotFound, "media item not found")
			}
		}
	}

	bitrateOut := req.Bitrate
	if h.userQuery != nil {
		if claims := interceptors.ClaimsFromContext(ctx); claims != nil && claims.ProfileID != "" {
			if prof, err := h.userQuery.GetProfileByID(ctx, claims.ProfileID); err == nil && prof.MaxBitrateKbps != nil {
				capKbps := int64(*prof.MaxBitrateKbps) * 1000
				if bitrateOut == 0 || bitrateOut > capKbps {
					bitrateOut = capKbps
				}
			}
		}
	}
	job := &transcoding.TranscodeJob{
		ID:            id.New("tj-"),
		MediaItemID:   shared.MediaItemID(req.MediaItemId),
		InputPath:     item.File.Path,
		OutputCodec:   req.VideoCodec,
		AudioCodec:    req.AudioCodec,
		HWAccelType:   hwAccelTypeFromProto[req.HwAccel],
		BitrateOut:    bitrateOut,
		ResolutionOut: fmt.Sprintf("%dx%d", req.Width, req.Height),
	}
	if err := h.svc.StartTranscode(ctx, job); err != nil {
		if errors.Is(err, apptranscoding.ErrMaxConcurrent) {
			return h.translator.TranslatedError(ctx, codes.ResourceExhausted, "errors.transcode.max_concurrent_reached")
		}
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
