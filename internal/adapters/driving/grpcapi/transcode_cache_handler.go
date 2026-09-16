package grpcapi

import (
	"context"
	"errors"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type transcodeCacheHandler struct {
	eveyv1.UnimplementedTranscodeCacheServiceServer
	svc *apptranscoding.Service
}

func (h *transcodeCacheHandler) GetTranscodeCache(ctx context.Context, _ *eveyv1.GetTranscodeCacheRequest) (*eveyv1.TranscodeCacheInfo, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.transcode_cache.GetTranscodeCache")
	defer span.End()
	info, err := h.svc.GetCacheInfo(ctx)
	if err != nil {
		return nil, internalError(ctx, "get cache info", err)
	}
	resp := &eveyv1.TranscodeCacheInfo{
		TotalSize:              info.TotalSize,
		FileCount:              int32(info.FileCount),
		ActiveEncodingSessions: int32(h.svc.ActiveEncodingSessions()),
	}
	for _, f := range info.Files {
		resp.Files = append(resp.Files, &eveyv1.CacheFileEntry{
			Name:         f.Name,
			Size:         f.Size,
			LastModified: timestamppb.New(f.LastModified),
		})
	}
	return resp, nil
}

func (h *transcodeCacheHandler) ClearTranscodeCache(ctx context.Context, _ *eveyv1.ClearTranscodeCacheRequest) (*eveyv1.ClearTranscodeCacheResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.transcode_cache.ClearTranscodeCache")
	defer span.End()
	if err := h.svc.ClearCache(ctx); err != nil {
		return nil, internalError(ctx, "clear cache", err)
	}
	return &eveyv1.ClearTranscodeCacheResponse{}, nil
}

func (h *transcodeCacheHandler) DeleteTranscodeCacheFile(ctx context.Context, req *eveyv1.DeleteTranscodeCacheFileRequest) (*eveyv1.DeleteTranscodeCacheFileResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.transcode_cache.DeleteTranscodeCacheFile")
	defer span.End()
	if err := h.svc.DeleteCacheFile(ctx, req.Filename); err != nil {
		switch {
		case errors.Is(err, transcoding.ErrCacheFileNotFound):
			return nil, status.Error(codes.NotFound, "cache file not found")
		case errors.Is(err, transcoding.ErrCacheFilenameInvalid):
			return nil, status.Error(codes.InvalidArgument, "invalid filename")
		}
		return nil, internalError(ctx, "delete cache file", err)
	}
	return &eveyv1.DeleteTranscodeCacheFileResponse{}, nil
}
