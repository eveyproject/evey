package grpcapi

import (
	"context"
	"database/sql"
	"errors"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type serverConfigHandler struct {
	eveyv1.UnimplementedServerConfigServiceServer
	repo shared.ServerConfigRepository
}

func NewServerConfigHandler(repo shared.ServerConfigRepository) *serverConfigHandler {
	return &serverConfigHandler{repo: repo}
}

func (h *serverConfigHandler) GetServerConfigEntry(ctx context.Context, req *eveyv1.GetServerConfigEntryRequest) (*eveyv1.ServerConfigEntryResponse, error) {
	if req.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "key required")
	}
	e, err := h.repo.Get(ctx, req.Key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "key not found: %s", req.Key)
		}
		return nil, status.Errorf(codes.Internal, "get config: %v", err)
	}
	return &eveyv1.ServerConfigEntryResponse{Key: e.Key, Value: e.Value}, nil
}

func (h *serverConfigHandler) SetServerConfigEntry(ctx context.Context, req *eveyv1.SetServerConfigEntryRequest) (*eveyv1.SetServerConfigEntryResponse, error) {
	if req.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "key required")
	}
	if err := h.repo.Set(ctx, req.Key, req.Value); err != nil {
		return nil, status.Errorf(codes.Internal, "set config: %v", err)
	}
	return &eveyv1.SetServerConfigEntryResponse{}, nil
}

func (h *serverConfigHandler) ListServerConfigEntries(ctx context.Context, _ *eveyv1.ListServerConfigEntriesRequest) (*eveyv1.ListServerConfigEntriesResponse, error) {
	entries, err := h.repo.List(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list config: %v", err)
	}
	resp := &eveyv1.ListServerConfigEntriesResponse{}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, &eveyv1.ServerConfigEntry{Key: e.Key, Value: e.Value})
	}
	return resp, nil
}

func (h *serverConfigHandler) DeleteServerConfigEntry(ctx context.Context, req *eveyv1.DeleteServerConfigEntryRequest) (*eveyv1.DeleteServerConfigEntryResponse, error) {
	if req.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "key required")
	}
	if err := h.repo.Delete(ctx, req.Key); err != nil {
		return nil, status.Errorf(codes.Internal, "delete config: %v", err)
	}
	return &eveyv1.DeleteServerConfigEntryResponse{}, nil
}
