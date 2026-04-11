package grpcapi

import (
	"context"
	"errors"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appconfig "github.com/gravity-zero/evey/internal/app/config"
	"github.com/gravity-zero/evey/internal/domain/shared"
	eveyi18n "github.com/gravity-zero/evey/internal/pkg/i18n"
	"google.golang.org/grpc/codes"
)

type serverConfigHandler struct {
	eveyv1.UnimplementedServerConfigServiceServer
	svc        *appconfig.Service
	translator *eveyi18n.Translator
}

func NewServerConfigHandler(svc *appconfig.Service) *serverConfigHandler {
	return &serverConfigHandler{svc: svc}
}

func (h *serverConfigHandler) SetTranslator(t *eveyi18n.Translator) {
	h.translator = t
}

func (h *serverConfigHandler) GetServerConfigEntry(ctx context.Context, req *eveyv1.GetServerConfigEntryRequest) (*eveyv1.ServerConfigEntryResponse, error) {
	if req.Key == "" {
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.config.key_required")
	}
	e, err := h.svc.Get(ctx, req.Key)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.config.key_not_found")
		}
		return nil, internalError("get config", err)
	}
	return &eveyv1.ServerConfigEntryResponse{Key: e.Key, Value: e.Value}, nil
}

func (h *serverConfigHandler) SetServerConfigEntry(ctx context.Context, req *eveyv1.SetServerConfigEntryRequest) (*eveyv1.SetServerConfigEntryResponse, error) {
	if req.Key == "" {
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.config.key_required")
	}
	if err := h.svc.Set(ctx, req.Key, req.Value); err != nil {
		return nil, internalError("set config", err)
	}
	return &eveyv1.SetServerConfigEntryResponse{}, nil
}

func (h *serverConfigHandler) ListServerConfigEntries(ctx context.Context, _ *eveyv1.ListServerConfigEntriesRequest) (*eveyv1.ListServerConfigEntriesResponse, error) {
	entries, err := h.svc.List(ctx)
	if err != nil {
		return nil, internalError("list config", err)
	}
	resp := &eveyv1.ListServerConfigEntriesResponse{}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, &eveyv1.ServerConfigEntry{Key: e.Key, Value: e.Value})
	}
	return resp, nil
}

func (h *serverConfigHandler) DeleteServerConfigEntry(ctx context.Context, req *eveyv1.DeleteServerConfigEntryRequest) (*eveyv1.DeleteServerConfigEntryResponse, error) {
	if req.Key == "" {
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.config.key_required")
	}
	if err := h.svc.Delete(ctx, req.Key); err != nil {
		return nil, internalError("delete config", err)
	}
	return &eveyv1.DeleteServerConfigEntryResponse{}, nil
}
