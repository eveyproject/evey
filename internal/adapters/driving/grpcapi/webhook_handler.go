package grpcapi

import (
	"context"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appwebhook "github.com/gravity-zero/evey/internal/app/webhook"
	"github.com/gravity-zero/evey/internal/domain/webhook"
)

type webhookHandler struct {
	eveyv1.UnimplementedWebhookServiceServer
	svc *appwebhook.Service
}

func (h *webhookHandler) CreateWebhook(ctx context.Context, req *eveyv1.CreateWebhookRequest) (*eveyv1.WebhookConfig, error) {
	cfg := &webhook.Config{
		Name:    req.Name,
		URL:     req.Url,
		Events:  req.Events,
		Secret:  req.Secret,
		Headers: req.Headers,
		Enabled: req.Enabled,
	}
	if err := h.svc.Create(ctx, cfg); err != nil {
		return nil, internalError("create webhook", err)
	}
	return webhookConfigToProto(cfg), nil
}

func (h *webhookHandler) ListWebhooks(ctx context.Context, _ *eveyv1.ListWebhooksRequest) (*eveyv1.ListWebhooksResponse, error) {
	configs, err := h.svc.List(ctx)
	if err != nil {
		return nil, internalError("list webhooks", err)
	}
	resp := &eveyv1.ListWebhooksResponse{}
	for _, c := range configs {
		resp.Webhooks = append(resp.Webhooks, webhookConfigToProto(c))
	}
	return resp, nil
}

func (h *webhookHandler) UpdateWebhook(ctx context.Context, req *eveyv1.UpdateWebhookRequest) (*eveyv1.WebhookConfig, error) {
	cfg := &webhook.Config{
		ID:      req.Id,
		Name:    req.Name,
		URL:     req.Url,
		Events:  req.Events,
		Secret:  req.Secret,
		Headers: req.Headers,
		Enabled: req.Enabled,
	}
	if err := h.svc.Update(ctx, cfg); err != nil {
		return nil, internalError("update webhook", err)
	}
	return webhookConfigToProto(cfg), nil
}

func (h *webhookHandler) DeleteWebhook(ctx context.Context, req *eveyv1.DeleteWebhookRequest) (*eveyv1.DeleteWebhookResponse, error) {
	if err := h.svc.Delete(ctx, req.Id); err != nil {
		return nil, internalError("delete webhook", err)
	}
	return &eveyv1.DeleteWebhookResponse{}, nil
}

func (h *webhookHandler) ListWebhookDeliveries(ctx context.Context, req *eveyv1.ListWebhookDeliveriesRequest) (*eveyv1.ListWebhookDeliveriesResponse, error) {
	deliveries, err := h.svc.ListDeliveries(ctx, req.WebhookId, int(req.Limit))
	if err != nil {
		return nil, internalError("list deliveries", err)
	}
	resp := &eveyv1.ListWebhookDeliveriesResponse{}
	for _, d := range deliveries {
		resp.Deliveries = append(resp.Deliveries, webhookDeliveryToProto(d))
	}
	return resp, nil
}

// maskSecret returns "****" for an empty secret, or the first 4 chars followed by "****".
// The raw secret is never returned to callers.
func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return s[:4] + "****"
}

func webhookConfigToProto(c *webhook.Config) *eveyv1.WebhookConfig {
	return &eveyv1.WebhookConfig{
		Id:        c.ID,
		Name:      c.Name,
		Url:       c.URL,
		Events:    c.Events,
		Secret:    maskSecret(c.Secret),
		Headers:   c.Headers,
		Enabled:   c.Enabled,
		CreatedAt: toTimestamppb(c.CreatedAt),
	}
}

func webhookDeliveryToProto(d *webhook.Delivery) *eveyv1.WebhookDelivery {
	return &eveyv1.WebhookDelivery{
		Id:         d.ID,
		WebhookId:  d.WebhookID,
		Event:      d.Event,
		StatusCode: int32(d.StatusCode),
		Error:      d.Error,
		SentAt:     toTimestamppb(d.SentAt),
	}
}
