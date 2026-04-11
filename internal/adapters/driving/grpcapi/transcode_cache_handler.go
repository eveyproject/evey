package grpcapi

import (
	"context"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type transcodeCacheHandler struct {
	eveyv1.UnimplementedTranscodeCacheServiceServer
	svc *apptranscoding.Service
}

func (h *transcodeCacheHandler) GetTranscodeCache(ctx context.Context, _ *eveyv1.GetTranscodeCacheRequest) (*eveyv1.TranscodeCacheInfo, error) {
	info, err := h.svc.GetCacheInfo(ctx)
	if err != nil {
		return nil, internalError("get cache info", err)
	}
	resp := &eveyv1.TranscodeCacheInfo{
		TotalSize: info.TotalSize,
		FileCount: int32(info.FileCount),
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
	if err := h.svc.ClearCache(ctx); err != nil {
		return nil, internalError("clear cache", err)
	}
	return &eveyv1.ClearTranscodeCacheResponse{}, nil
}

func (h *transcodeCacheHandler) DeleteTranscodeCacheFile(ctx context.Context, req *eveyv1.DeleteTranscodeCacheFileRequest) (*eveyv1.DeleteTranscodeCacheFileResponse, error) {
	if err := h.svc.DeleteCacheFile(ctx, req.Filename); err != nil {
		return nil, internalError("delete cache file", err)
	}
	return &eveyv1.DeleteTranscodeCacheFileResponse{}, nil
}
