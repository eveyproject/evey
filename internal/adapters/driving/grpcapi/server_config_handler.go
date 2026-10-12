package grpcapi

import (
	"context"
	"errors"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appconfig "github.com/eveyproject/evey/internal/app/config"
	"github.com/eveyproject/evey/internal/domain/shared"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	ctx, span := tracing.StartSpan(ctx, "grpc.server_config.GetServerConfigEntry")
	defer span.End()
	if req.Key == "" {
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.config.key_required")
	}
	e, err := h.svc.Get(ctx, req.Key)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.config.key_not_found")
		}
		return nil, internalError(ctx, "get config", err)
	}
	// A credential never leaves the server, not even to an admin: this response
	// is JSON in a browser, and the panel only ever needed to know whether a
	// value is set. See appconfig.MaskSecretValue.
	return &eveyv1.ServerConfigEntryResponse{Key: e.Key, Value: appconfig.MaskSecretValue(e.Key, e.Value), UpdatedAt: toTimestamppb(e.UpdatedAt)}, nil
}

func (h *serverConfigHandler) SetServerConfigEntry(ctx context.Context, req *eveyv1.SetServerConfigEntryRequest) (*eveyv1.SetServerConfigEntryResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.server_config.SetServerConfigEntry")
	defer span.End()
	if req.Key == "" {
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.config.key_required")
	}
	// Write-only secrets: the caller was served a mask, so a mask coming back
	// means "unchanged", not "set my password to eight bullets". Forms that
	// submit every field at once (the SMTP card) would otherwise destroy the
	// stored password on any unrelated edit.
	if appconfig.ShouldKeepStoredSecret(req.Key, req.Value) {
		return &eveyv1.SetServerConfigEntryResponse{}, nil
	}
	if err := h.svc.Set(ctx, req.Key, req.Value); err != nil {
		if errors.Is(err, appconfig.ErrInvalidValue) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, internalError(ctx, "set config", err)
	}
	return &eveyv1.SetServerConfigEntryResponse{}, nil
}

func (h *serverConfigHandler) ListServerConfigEntries(ctx context.Context, _ *eveyv1.ListServerConfigEntriesRequest) (*eveyv1.ListServerConfigEntriesResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.server_config.ListServerConfigEntries")
	defer span.End()
	entries, err := h.svc.List(ctx)
	if err != nil {
		return nil, internalError(ctx, "list config", err)
	}
	resp := &eveyv1.ListServerConfigEntriesResponse{}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, &eveyv1.ServerConfigEntry{Key: e.Key, Value: appconfig.MaskSecretValue(e.Key, e.Value), UpdatedAt: toTimestamppb(e.UpdatedAt)})
	}
	return resp, nil
}

func (h *serverConfigHandler) DeleteServerConfigEntry(ctx context.Context, req *eveyv1.DeleteServerConfigEntryRequest) (*eveyv1.DeleteServerConfigEntryResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.server_config.DeleteServerConfigEntry")
	defer span.End()
	if req.Key == "" {
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.config.key_required")
	}
	if err := h.svc.Delete(ctx, req.Key); err != nil {
		return nil, internalError(ctx, "delete config", err)
	}
	return &eveyv1.DeleteServerConfigEntryResponse{}, nil
}
