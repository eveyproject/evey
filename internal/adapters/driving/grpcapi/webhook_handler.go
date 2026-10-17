package grpcapi

import (
	"context"
	"errors"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appwebhook "github.com/eveyproject/evey/internal/app/webhook"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/webhook"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type webhookHandler struct {
	eveyv1.UnimplementedWebhookServiceServer
	svc *appwebhook.Service
}

func (h *webhookHandler) CreateWebhook(ctx context.Context, req *eveyv1.CreateWebhookRequest) (*eveyv1.WebhookConfig, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.webhook.CreateWebhook")
	defer span.End()
	cfg := &webhook.Config{
		Name:    req.Name,
		URL:     req.Url,
		Events:  req.Events,
		Secret:  req.Secret,
		Headers: req.Headers,
		Enabled: req.Enabled,
	}
	if err := h.svc.Create(ctx, cfg); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		if errors.Is(err, appwebhook.ErrInvalidConfig) {
			return nil, status.Error(codes.InvalidArgument, "invalid webhook configuration")
		}
		return nil, internalError(ctx, "create webhook", err)
	}
	return webhookConfigToProto(cfg), nil
}

func (h *webhookHandler) ListWebhooks(ctx context.Context, _ *eveyv1.ListWebhooksRequest) (*eveyv1.ListWebhooksResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.webhook.ListWebhooks")
	defer span.End()
	configs, err := h.svc.List(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "list webhooks", err)
	}
	resp := &eveyv1.ListWebhooksResponse{}
	for _, c := range configs {
		resp.Webhooks = append(resp.Webhooks, webhookConfigToProto(c))
	}
	return resp, nil
}

func (h *webhookHandler) UpdateWebhook(ctx context.Context, req *eveyv1.UpdateWebhookRequest) (*eveyv1.WebhookConfig, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.webhook.UpdateWebhook")
	defer span.End()
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
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		if errors.Is(err, appwebhook.ErrInvalidConfig) {
			return nil, status.Error(codes.InvalidArgument, "invalid webhook configuration")
		}
		if errors.Is(err, shared.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "webhook not found")
		}
		return nil, internalError(ctx, "update webhook", err)
	}
	return webhookConfigToProto(cfg), nil
}

func (h *webhookHandler) DeleteWebhook(ctx context.Context, req *eveyv1.DeleteWebhookRequest) (*eveyv1.DeleteWebhookResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.webhook.DeleteWebhook")
	defer span.End()
	if err := h.svc.Delete(ctx, req.Id); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		if errors.Is(err, shared.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "webhook not found")
		}
		return nil, internalError(ctx, "delete webhook", err)
	}
	return &eveyv1.DeleteWebhookResponse{}, nil
}

func (h *webhookHandler) ListWebhookDeliveries(ctx context.Context, req *eveyv1.ListWebhookDeliveriesRequest) (*eveyv1.ListWebhookDeliveriesResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.webhook.ListWebhookDeliveries")
	defer span.End()
	// Cap the page size (admin-only endpoint, but keep parity with every other
	// list handler so a huge Limit can't drive an unbounded query). A non-positive
	// limit keeps the repo's own default.
	limit := min(int(req.Limit), maxPageSize)
	deliveries, err := h.svc.ListDeliveries(ctx, req.WebhookId, limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "list deliveries", err)
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

// maskHeaders masks every custom header value like the secret: they routinely
// carry the receiver's credentials (Authorization, X-Api-Key). The app layer
// keeps the stored value when Update receives the mask back.
func maskHeaders(h map[string]string) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = maskSecret(v)
	}
	return out
}

func webhookConfigToProto(c *webhook.Config) *eveyv1.WebhookConfig {
	return &eveyv1.WebhookConfig{
		Id:        c.ID,
		Name:      c.Name,
		Url:       c.URL,
		Events:    c.Events,
		Secret:    maskSecret(c.Secret),
		Headers:   maskHeaders(c.Headers),
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
