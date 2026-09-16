package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/evey-project/evey/internal/app/library"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/transcoding"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/tracing"
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
	_, span := tracing.StartSpan(ctx, "grpc.transcoding.StartTranscode")
	defer span.End()
	if req.MediaItemId == "" {
		return status.Error(codes.InvalidArgument, "media_item_id required")
	}
	// Resolve media item: we need FilePath for InputPath + library check.
	if h.libQuery == nil {
		return status.Error(codes.FailedPrecondition, "library service not configured")
	}
	item, err := h.libQuery.GetMediaItem(ctx, req.MediaItemId)
	if err != nil || item == nil || item.File == nil || item.File.Path == "" {
		return h.translator.TranslatedError(ctx, codes.NotFound, "errors.media.not_found")
	}
	// IDOR: restrict to libraries the caller has access to.
	//
	// Identity first, wiring second - the order its own twin checkJobAccess
	// already documents. Testing the port first put the unauthenticated check
	// INSIDE the wiring check, so a nil userAccess skipped the whole block and
	// let an anonymous caller through. The port is always wired at boot today,
	// so this was never open in practice; it was open by construction, which is
	// how the next wiring change would have shipped it.
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if !claims.IsAdmin {
		if h.userAccess == nil {
			return errUserAccessNotConfigured(ctx, "transcodingHandler.StartTranscode")
		}
		ok, accessErr := h.userAccess.HasLibraryAccess(ctx, shared.UserID(claims.UserID), item.LibraryID)
		if accessErr != nil || !ok {
			return h.translator.TranslatedError(ctx, codes.NotFound, "errors.media.not_found")
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
	job, err := transcoding.NewTranscodeJob(id.New("tj-"), shared.MediaItemID(req.MediaItemId), item.File.Path, req.VideoCodec,
		transcoding.WithJobAudioCodec(req.AudioCodec),
		transcoding.WithJobHWAccel(hwAccelTypeFromProto[req.HwAccel]),
		transcoding.WithJobBitrate(0, bitrateOut),
		transcoding.WithJobResolution("", fmt.Sprintf("%dx%d", req.Width, req.Height)),
	)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := h.svc.StartTranscode(ctx, job); err != nil {
		if errors.Is(err, apptranscoding.ErrMaxConcurrent) {
			return h.translator.TranslatedError(ctx, codes.ResourceExhausted, "errors.transcode.max_concurrent_reached")
		}
		return internalError(ctx, "start transcode", err)
	}
	return stream.Send(&eveyv1.TranscodeProgress{
		JobId:   job.ID,
		State:   eveyv1.TranscodeState_TRANSCODE_STATE_RUNNING,
		Percent: 0,
	})
}

func (h *transcodingHandler) StopTranscode(ctx context.Context, req *eveyv1.StopTranscodeRequest) (*eveyv1.StopTranscodeResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.transcoding.StopTranscode")
	defer span.End()
	if err := h.checkJobAccess(ctx, req.JobId); err != nil {
		return nil, err
	}
	if err := h.svc.StopTranscode(ctx, req.JobId); err != nil {
		return nil, internalError(ctx, "stop transcode", err)
	}
	return &eveyv1.StopTranscodeResponse{}, nil
}

func (h *transcodingHandler) GetTranscodeStatus(ctx context.Context, req *eveyv1.GetTranscodeStatusRequest) (*eveyv1.TranscodeJob, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.transcoding.GetTranscodeStatus")
	defer span.End()
	if err := h.checkJobAccess(ctx, req.JobId); err != nil {
		return nil, err
	}
	progress, err := h.svc.GetProgress(ctx, req.JobId)
	if err != nil {
		slog.WarnContext(ctx, "grpc GetTranscodeStatus: progress query failed", slog.Any("err", err))
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.transcode.job_not_found")
	}
	return &eveyv1.TranscodeJob{
		Id:              progress.JobID,
		ProgressPercent: progress.Percent,
		Fps:             progress.FPS,
		Speed:           progress.Speed,
	}, nil
}

// checkJobAccess is defense-in-depth for StopTranscode + GetTranscodeStatus.
// jobID is 192-bit entropy so not practically enumerable, but an attacker who
// captures one (logs, shared screen) shouldn't be able to control a job
// tied to a library they don't own. Mirrors StartTranscode's library check.
// Returns NotFound on any miss to avoid leaking job-existence.
func (h *transcodingHandler) checkJobAccess(ctx context.Context, jobID string) error {
	if jobID == "" {
		return status.Error(codes.InvalidArgument, "job_id required")
	}
	// Identity first, wiring second. Whether the access port is wired says
	// nothing about who is calling: an unauthenticated caller must still be told
	// it is unauthenticated, and an admin bypasses the per-library check on the
	// strength of its claims alone. Testing the port first turned both into
	// "failed precondition" - a wiring bug that answers an admin with a denial
	// is the mirror image of the fail-open this whole change closes.
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if claims.IsAdmin {
		return nil
	}
	if h.userAccess == nil {
		return errUserAccessNotConfigured(ctx, "transcodingHandler.checkJobAccess")
	}
	if h.libQuery == nil {
		return nil
	}
	job, err := h.svc.GetJob(ctx, jobID)
	if err != nil || job == nil {
		return h.translator.TranslatedError(ctx, codes.NotFound, "errors.transcode.job_not_found")
	}
	item, err := h.libQuery.GetMediaItem(ctx, string(job.MediaItemID))
	if err != nil || item == nil {
		return h.translator.TranslatedError(ctx, codes.NotFound, "errors.transcode.job_not_found")
	}
	ok, accessErr := h.userAccess.HasLibraryAccess(ctx, shared.UserID(claims.UserID), item.LibraryID)
	if accessErr != nil || !ok {
		return h.translator.TranslatedError(ctx, codes.NotFound, "errors.transcode.job_not_found")
	}
	return nil
}

func (h *transcodingHandler) ListActiveTranscodes(ctx context.Context, _ *eveyv1.ListActiveTranscodesRequest) (*eveyv1.ListActiveTranscodesResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.transcoding.ListActiveTranscodes")
	defer span.End()
	jobs, err := h.svc.ListActiveTranscodes(ctx)
	if err != nil {
		return nil, internalError(ctx, "list active", err)
	}
	resp := &eveyv1.ListActiveTranscodesResponse{}
	for _, j := range jobs {
		resp.Jobs = append(resp.Jobs, transcodeJobToProto(j))
	}
	return resp, nil
}

func (h *transcodingHandler) ListTranscodeProfiles(ctx context.Context, _ *eveyv1.ListTranscodeProfilesRequest) (*eveyv1.ListTranscodeProfilesResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.transcoding.ListTranscodeProfiles")
	defer span.End()
	profiles, err := h.svc.ListProfiles(ctx)
	if err != nil {
		return nil, internalError(ctx, "list profiles", err)
	}
	resp := &eveyv1.ListTranscodeProfilesResponse{}
	for _, p := range profiles {
		resp.Profiles = append(resp.Profiles, transcodeProfileToProto(p))
	}
	return resp, nil
}

func (h *transcodingHandler) DetectHardware(ctx context.Context, _ *eveyv1.DetectHardwareRequest) (*eveyv1.DetectHardwareResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.transcoding.DetectHardware")
	defer span.End()
	devices, err := h.svc.DetectHardware(ctx)
	if err != nil {
		return nil, internalError(ctx, "detect hardware", err)
	}
	resp := &eveyv1.DetectHardwareResponse{}
	for _, d := range devices {
		resp.Devices = append(resp.Devices, hwDeviceToProto(d))
	}
	return resp, nil
}
