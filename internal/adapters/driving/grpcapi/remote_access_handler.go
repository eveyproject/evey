package grpcapi

import (
	"context"
	"encoding/json"
	"log/slog"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/config"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/eveyproject/evey/internal/pkg/network"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"github.com/eveyproject/evey/internal/pkg/urlvalidator"
	"google.golang.org/grpc/codes"
)

const remoteAccessCfgKey = "remote_access"

type remoteAccessConfigStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

// The handler holds no mutex of its own: these fields are shared with the REST
// handler and with the request-time remote-access gate, so the only lock that
// means anything is the config's own (cfg.WithReadLock / ApplyServerSettings).
// A private mutex here excluded neither of them.
type remoteAccessHandler struct {
	eveyv1.UnimplementedRemoteAccessServiceServer
	cfg        *config.Config
	store      remoteAccessConfigStore
	translator *eveyi18n.Translator
}

func (h *remoteAccessHandler) GetRemoteAccess(ctx context.Context, _ *eveyv1.GetRemoteAccessRequest) (*eveyv1.RemoteAccessConfig, error) {
	_, span := tracing.StartSpan(ctx, "grpc.remote_access.GetRemoteAccess")
	defer span.End()
	var out *eveyv1.RemoteAccessConfig
	h.cfg.WithReadLock(func() {
		out = raConfigToProto(&h.cfg.RemoteAccess, h.cfg.Server.BaseURL)
		out.ServerBaseUrl = h.cfg.Server.BaseURL
	})
	out.TrustedProxies, out.TrustedProxiesFromPanel = h.cfg.TrustedProxiesView()
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
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.auth.admin_access_required")
	}
	if req.ExternalUrl != nil {
		if err := urlvalidator.ValidateExternalURL(*req.ExternalUrl); err != nil {
			return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.admin.external_url_invalid", map[string]string{"reason": err.Error()})
		}
	}
	var trustedProxies []string
	if len(req.TrustedProxies) > 0 {
		list, err := config.PanelTrustedProxies(req.TrustedProxies)
		if err != nil {
			return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.admin.trusted_proxies_invalid", map[string]string{"reason": err.Error()})
		}
		trustedProxies = list
	}
	var snapshot config.RemoteAccessConfig
	var baseURL string
	h.cfg.ApplyServerSettings(func() {
		if req.Enabled != nil {
			h.cfg.RemoteAccess.Enabled = *req.Enabled
			// Mark it as an operator choice so the boot-time inference
			// (config.ResolveRemoteAccess) never overrides it - see access_handler.go.
			h.cfg.RemoteAccess.EnabledExplicit = true
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
		switch {
		case len(trustedProxies) > 0:
			h.cfg.RemoteAccess.TrustedProxies = trustedProxies
		case req.ClearTrustedProxies:
			h.cfg.RemoteAccess.TrustedProxies = nil
		}
		snapshot = h.cfg.RemoteAccess
		baseURL = h.cfg.Server.BaseURL
	})

	if h.store != nil {
		if data, err := json.Marshal(snapshot); err == nil {
			if err := h.store.Set(ctx, remoteAccessCfgKey, string(data)); err != nil {
				slog.ErrorContext(ctx, "persist remote access config", slog.Any("err", err))
			}
		}
	}
	out := raConfigToProto(&snapshot, baseURL)
	out.ServerBaseUrl = baseURL
	out.TrustedProxies, out.TrustedProxiesFromPanel = h.cfg.TrustedProxiesView()
	return out, nil
}

func (h *remoteAccessHandler) GetNetworkInfo(ctx context.Context, _ *eveyv1.GetNetworkInfoRequest) (*eveyv1.NetworkInfo, error) {
	_, span := tracing.StartSpan(ctx, "grpc.remote_access.GetNetworkInfo")
	defer span.End()
	ifaces, err := network.NetworkInterfaces()
	if err != nil {
		return nil, internalError(ctx, "get network interfaces", err)
	}
	resp := &eveyv1.NetworkInfo{}
	for _, i := range ifaces {
		resp.Interfaces = append(resp.Interfaces, &eveyv1.NetworkInterface{Name: i.Name, Address: i.Address})
	}
	return resp, nil
}

func raConfigToProto(c *config.RemoteAccessConfig, serverBaseURL string) *eveyv1.RemoteAccessConfig {
	// server_base_url, published and lockout_risk are all declared by the proto.
	// The first was declared and never filled; leaving a contract field empty is
	// the same defect as not declaring it, with the added cost that a client can
	// reasonably believe it.
	published, lockoutRisk := config.RemoteAccessStatus(c, serverBaseURL)
	return &eveyv1.RemoteAccessConfig{
		Enabled:              c.Enabled,
		ExternalUrl:          c.ExternalURL,
		LanNetworks:          c.LANNetworks,
		MaxRemoteBitrateKbps: int32(c.MaxRemoteBitrateKbps),
		ServerBaseUrl:        serverBaseURL,
		Published:            published,
		LockoutRisk:          lockoutRisk,
	}
}
