package grpcapi

import (
	"context"
	"encoding/json"
	"sync"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/config"
	"github.com/gravity-zero/evey/internal/pkg/network"
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

func (h *remoteAccessHandler) GetRemoteAccess(_ context.Context, _ *eveyv1.GetRemoteAccessRequest) (*eveyv1.RemoteAccessConfig, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return raConfigToProto(&h.cfg.RemoteAccess), nil
}

func (h *remoteAccessHandler) UpdateRemoteAccess(ctx context.Context, req *eveyv1.UpdateRemoteAccessRequest) (*eveyv1.RemoteAccessConfig, error) {
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
	h.mu.Unlock()

	if h.store != nil {
		if data, err := json.Marshal(snapshot); err == nil {
			_ = h.store.Set(ctx, remoteAccessCfgKey, string(data))
		}
	}
	return raConfigToProto(&snapshot), nil
}

func (h *remoteAccessHandler) GetNetworkInfo(_ context.Context, _ *eveyv1.GetNetworkInfoRequest) (*eveyv1.NetworkInfo, error) {
	ifaces, err := network.NetworkInterfaces()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get network interfaces: %v", err)
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
