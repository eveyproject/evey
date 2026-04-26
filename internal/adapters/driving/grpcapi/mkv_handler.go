package grpcapi

import (
	"context"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appmkv "github.com/evey-project/evey/internal/app/mkv"
	"github.com/evey-project/evey/internal/domain/mkv"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type mkvHandler struct {
	eveyv1.UnimplementedMKVServiceServer
	svc *appmkv.Service
}

func (h *mkvHandler) GetContainerInfo(ctx context.Context, req *eveyv1.GetContainerInfoRequest) (*eveyv1.Container, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.mkv.GetContainerInfo")
	defer span.End()
	c, err := h.svc.GetContainerInfo(ctx, req.Path)
	if err != nil {
		return nil, internalError("get container info", err)
	}
	return mkvContainerToProto(c), nil
}

func (h *mkvHandler) ListTracks(ctx context.Context, req *eveyv1.ListMkvTracksRequest) (*eveyv1.ListMkvTracksResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.mkv.ListTracks")
	defer span.End()
	tracks, err := h.svc.ListTracks(ctx, req.Path)
	if err != nil {
		return nil, internalError("list tracks", err)
	}
	resp := &eveyv1.ListMkvTracksResponse{}
	for _, t := range tracks {
		resp.Tracks = append(resp.Tracks, mkvTrackToProto(t))
	}
	return resp, nil
}

func (h *mkvHandler) ListChapters(ctx context.Context, req *eveyv1.ListChaptersRequest) (*eveyv1.ListChaptersResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.mkv.ListChapters")
	defer span.End()
	chapters, err := h.svc.ListChapters(ctx, req.Path)
	if err != nil {
		return nil, internalError("list chapters", err)
	}
	resp := &eveyv1.ListChaptersResponse{}
	for _, ch := range chapters {
		resp.Chapters = append(resp.Chapters, mkvChapterToProto(ch))
	}
	return resp, nil
}

func (h *mkvHandler) ListAttachments(ctx context.Context, req *eveyv1.ListAttachmentsRequest) (*eveyv1.ListAttachmentsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.mkv.ListAttachments")
	defer span.End()
	attachments, err := h.svc.ListAttachments(ctx, req.Path)
	if err != nil {
		return nil, internalError("list attachments", err)
	}
	resp := &eveyv1.ListAttachmentsResponse{}
	for _, a := range attachments {
		resp.Attachments = append(resp.Attachments, mkvAttachmentToProto(a))
	}
	return resp, nil
}

func (h *mkvHandler) ExtractTrack(req *eveyv1.ExtractTrackRequest, stream eveyv1.MKVService_ExtractTrackServer) error {
	_, span := tracing.StartSpan(stream.Context(), "grpc.mkv.ExtractTrack")
	defer span.End()
	if err := h.svc.ExtractTrack(stream.Context(), req.Path, uint64(req.TrackId), ""); err != nil {
		return internalError("extract track", err)
	}
	return stream.Send(&eveyv1.ExtractDataChunk{})
}

func (h *mkvHandler) ExtractAttachment(ctx context.Context, req *eveyv1.ExtractAttachmentRequest) (*eveyv1.ExtractAttachmentResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.mkv.ExtractAttachment")
	defer span.End()
	if err := h.svc.ExtractAttachment(ctx, req.Path, uint64(req.AttachmentId), req.OutputPath); err != nil {
		return nil, internalError("extract attachment", err)
	}
	return &eveyv1.ExtractAttachmentResponse{
		OutputPath: req.OutputPath,
	}, nil
}

func (h *mkvHandler) MuxContainer(req *eveyv1.MuxContainerRequest, stream eveyv1.MKVService_MuxContainerServer) error {
	_, span := tracing.StartSpan(stream.Context(), "grpc.mkv.MuxContainer")
	defer span.End()
	opts := mkv.MuxOptions{
		OutputPath: req.OutputPath,
	}
	for _, input := range req.Inputs {
		for _, trackID := range input.TrackIds {
			opts.Tracks = append(opts.Tracks, mkv.TrackInput{
				SourcePath: input.Path,
				TrackID:    uint64(trackID),
			})
		}
	}
	for _, ch := range req.Chapters {
		opts.Chapters = append(opts.Chapters, mkvChapterFromProto(ch))
	}
	if err := h.svc.MuxContainer(stream.Context(), opts); err != nil {
		return internalError("mux container", err)
	}
	return stream.Send(&eveyv1.MuxProgress{Percent: 100, Phase: "complete"})
}

func (h *mkvHandler) DemuxContainer(req *eveyv1.DemuxContainerRequest, stream eveyv1.MKVService_DemuxContainerServer) error {
	_, span := tracing.StartSpan(stream.Context(), "grpc.mkv.DemuxContainer")
	defer span.End()
	var trackIDs []uint64
	for _, id := range req.TrackIds {
		trackIDs = append(trackIDs, uint64(id))
	}
	opts := mkv.DemuxOptions{
		OutputDir: req.OutputDir,
		TrackIDs:  trackIDs,
	}
	if err := h.svc.DemuxContainer(stream.Context(), req.Path, opts); err != nil {
		return internalError("demux container", err)
	}
	return stream.Send(&eveyv1.DemuxProgress{Percent: 100})
}

func (h *mkvHandler) AddTrack(ctx context.Context, req *eveyv1.AddTrackRequest) (*eveyv1.AddTrackResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.mkv.AddTrack")
	defer span.End()
	input := mkv.TrackInput{
		SourcePath: req.TrackPath,
		Language:   req.Language,
		Name:       req.Name,
		IsDefault:  req.IsDefault,
	}
	if err := h.svc.AddTrack(ctx, req.ContainerPath, input); err != nil {
		return nil, internalError("add track", err)
	}
	return &eveyv1.AddTrackResponse{}, nil
}

func (h *mkvHandler) RemoveTrack(ctx context.Context, req *eveyv1.RemoveTrackRequest) (*eveyv1.RemoveTrackResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.mkv.RemoveTrack")
	defer span.End()
	if err := h.svc.RemoveTrack(ctx, req.ContainerPath, uint64(req.TrackId)); err != nil {
		return nil, internalError("remove track", err)
	}
	return &eveyv1.RemoveTrackResponse{}, nil
}
