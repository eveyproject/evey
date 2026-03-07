package grpcapi

import (
	"context"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appmkv "github.com/gravity-zero/evey/internal/app/mkv"
	"github.com/gravity-zero/evey/internal/domain/mkv"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mkvHandler struct {
	eveyv1.UnimplementedMKVServiceServer
	svc *appmkv.Service
}

func (h *mkvHandler) GetContainerInfo(ctx context.Context, req *eveyv1.GetContainerInfoRequest) (*eveyv1.Container, error) {
	c, err := h.svc.GetContainerInfo(ctx, req.Path)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get container info: %v", err)
	}
	return mkvContainerToProto(c), nil
}

func (h *mkvHandler) ListTracks(ctx context.Context, req *eveyv1.ListTracksRequest) (*eveyv1.ListTracksResponse, error) {
	tracks, err := h.svc.ListTracks(ctx, req.Path)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list tracks: %v", err)
	}
	resp := &eveyv1.ListTracksResponse{}
	for _, t := range tracks {
		resp.Tracks = append(resp.Tracks, mkvTrackToProto(t))
	}
	return resp, nil
}

func (h *mkvHandler) ListChapters(ctx context.Context, req *eveyv1.ListChaptersRequest) (*eveyv1.ListChaptersResponse, error) {
	chapters, err := h.svc.ListChapters(ctx, req.Path)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list chapters: %v", err)
	}
	resp := &eveyv1.ListChaptersResponse{}
	for _, ch := range chapters {
		resp.Chapters = append(resp.Chapters, mkvChapterToProto(ch))
	}
	return resp, nil
}

func (h *mkvHandler) ListAttachments(ctx context.Context, req *eveyv1.ListAttachmentsRequest) (*eveyv1.ListAttachmentsResponse, error) {
	attachments, err := h.svc.ListAttachments(ctx, req.Path)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list attachments: %v", err)
	}
	resp := &eveyv1.ListAttachmentsResponse{}
	for _, a := range attachments {
		resp.Attachments = append(resp.Attachments, mkvAttachmentToProto(a))
	}
	return resp, nil
}

func (h *mkvHandler) ExtractTrack(req *eveyv1.ExtractTrackRequest, stream eveyv1.MKVService_ExtractTrackServer) error {
	if err := h.svc.ExtractTrack(stream.Context(), req.Path, uint64(req.TrackId), ""); err != nil {
		return status.Errorf(codes.Internal, "extract track: %v", err)
	}
	return stream.Send(&eveyv1.ExtractDataChunk{})
}

func (h *mkvHandler) ExtractAttachment(ctx context.Context, req *eveyv1.ExtractAttachmentRequest) (*eveyv1.ExtractAttachmentResponse, error) {
	if err := h.svc.ExtractAttachment(ctx, req.Path, uint64(req.AttachmentId), req.OutputPath); err != nil {
		return nil, status.Errorf(codes.Internal, "extract attachment: %v", err)
	}
	return &eveyv1.ExtractAttachmentResponse{
		OutputPath: req.OutputPath,
	}, nil
}

func (h *mkvHandler) MuxContainer(req *eveyv1.MuxContainerRequest, stream eveyv1.MKVService_MuxContainerServer) error {
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
		return status.Errorf(codes.Internal, "mux container: %v", err)
	}
	return stream.Send(&eveyv1.MuxProgress{Percent: 100, Phase: "complete"})
}

func (h *mkvHandler) DemuxContainer(req *eveyv1.DemuxContainerRequest, stream eveyv1.MKVService_DemuxContainerServer) error {
	var trackIDs []uint64
	for _, id := range req.TrackIds {
		trackIDs = append(trackIDs, uint64(id))
	}
	opts := mkv.DemuxOptions{
		OutputDir: req.OutputDir,
		TrackIDs:  trackIDs,
	}
	if err := h.svc.DemuxContainer(stream.Context(), req.Path, opts); err != nil {
		return status.Errorf(codes.Internal, "demux container: %v", err)
	}
	return stream.Send(&eveyv1.DemuxProgress{Percent: 100})
}

func (h *mkvHandler) AddTrack(ctx context.Context, req *eveyv1.AddTrackRequest) (*eveyv1.AddTrackResponse, error) {
	input := mkv.TrackInput{
		SourcePath: req.TrackPath,
		Language:   req.Language,
		Name:       req.Name,
		IsDefault:  req.IsDefault,
	}
	if err := h.svc.AddTrack(ctx, req.ContainerPath, input); err != nil {
		return nil, status.Errorf(codes.Internal, "add track: %v", err)
	}
	return &eveyv1.AddTrackResponse{}, nil
}

func (h *mkvHandler) RemoveTrack(ctx context.Context, req *eveyv1.RemoveTrackRequest) (*eveyv1.RemoveTrackResponse, error) {
	if err := h.svc.RemoveTrack(ctx, req.ContainerPath, uint64(req.TrackId)); err != nil {
		return nil, status.Errorf(codes.Internal, "remove track: %v", err)
	}
	return &eveyv1.RemoveTrackResponse{}, nil
}
