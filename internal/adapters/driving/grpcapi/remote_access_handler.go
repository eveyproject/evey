package grpcapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/config"
	"github.com/evey-project/evey/internal/pkg/network"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"github.com/evey-project/evey/internal/pkg/urlvalidator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const remoteAccessCfgKey = "remote_access"

type remoteAccessConfigStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

type remoteAccessHandler struct {
	eveyv1.UnimplementedRemoteAccessServiceServer
	mu    sync.RWMutex
	cfg   *config.Config
	store remoteAccessConfigStore
}

func (h *remoteAccessHandler) GetRemoteAccess(ctx context.Context, _ *eveyv1.GetRemoteAccessRequest) (*eveyv1.RemoteAccessConfig, error) {
	_, span := tracing.StartSpan(ctx, "grpc.remote_access.GetRemoteAccess")
	defer span.End()
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := raConfigToProto(&h.cfg.RemoteAccess)
	out.ServerBaseUrl = h.cfg.Server.BaseURL
	return out, nil
}

func (h *remoteAccessHandler) UpdateRemoteAccess(ctx context.Context, req *eveyv1.UpdateRemoteAccessRequest) (*eveyv1.RemoteAccessConfig, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.remote_access.UpdateRemoteAccess")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !claims.IsAdmin {
		return nil, status.Error(codes.PermissionDenied, "admin only")
	}
	if req.ExternalUrl != nil {
		if err := urlvalidator.ValidateExternalURL(*req.ExternalUrl); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid ExternalURL: %v", err)
		}
	}
	h.mu.Lock()
	if req.Enabled != nil {
		h.cfg.RemoteAccess.Enabled = *req.Enabled
	}
	if req.ExternalUrl != nil {
		h.cfg.RemoteAccess.ExternalURL = *req.ExternalUrl
	}
	if len(req.LanNetworks) > 0 {
		h.cfg.RemoteAccess.LANNetworks = req.LanNetworks
	}
	if req.MaxRemoteBitrateKbps != nil {
		h.cfg.RemoteAccess.MaxRemoteBitrateKbps = int(*req.MaxRemoteBitrateKbps)
	}
	snapshot := h.cfg.RemoteAccess
	baseURL := h.cfg.Server.BaseURL
	h.mu.Unlock()

	if h.store != nil {
		if data, err := json.Marshal(snapshot); err == nil {
			if err := h.store.Set(ctx, remoteAccessCfgKey, string(data)); err != nil {
				slog.Error("persist remote access config", slog.Any("err", err))
			}
		}
	}
	out := raConfigToProto(&snapshot)
	out.ServerBaseUrl = baseURL
	return out, nil
}

func (h *remoteAccessHandler) GetNetworkInfo(ctx context.Context, _ *eveyv1.GetNetworkInfoRequest) (*eveyv1.NetworkInfo, error) {
	_, span := tracing.StartSpan(ctx, "grpc.remote_access.GetNetworkInfo")
	defer span.End()
	ifaces, err := network.NetworkInterfaces()
	if err != nil {
		return nil, internalError("get network interfaces", err)
	}
	resp := &eveyv1.NetworkInfo{}
	for _, i := range ifaces {
		resp.Interfaces = append(resp.Interfaces, &eveyv1.NetworkInterface{Name: i.Name, Address: i.Address})
	}
	return resp, nil
}

func raConfigToProto(c *config.RemoteAccessConfig) *eveyv1.RemoteAccessConfig {
	return &eveyv1.RemoteAccessConfig{
		Enabled:              c.Enabled,
		ExternalUrl:          c.ExternalURL,
		LanNetworks:          c.LANNetworks,
		MaxRemoteBitrateKbps: int32(c.MaxRemoteBitrateKbps),
	}
}
