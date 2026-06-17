package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/webhook"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"github.com/evey-project/evey/internal/pkg/urlvalidator"
	"github.com/google/uuid"
)

const (
	maxConcurrentDeliveries = 10
	deliveryTimeout         = 30 * time.Second
)

var defaultRetryDelays = []time.Duration{1 * time.Second, 5 * time.Second, 30 * time.Second}

type Service struct {
	repo        webhook.Repository
	deliveries  webhook.DeliveryRepository
	sender      webhook.Sender
	log         *slog.Logger
	validateURL func(string) error
	sem         chan struct{}
	retryDelays []time.Duration
}

func NewService(
	repo webhook.Repository,
	deliveries webhook.DeliveryRepository,
	sender webhook.Sender,
	log *slog.Logger,
	events shared.EventBus,
) *Service {
	s := &Service{
		repo:        repo,
		deliveries:  deliveries,
		sender:      sender,
		log:         log,
		validateURL: urlvalidator.ValidateWebhookURL,
		sem:         make(chan struct{}, maxConcurrentDeliveries),
		retryDelays: defaultRetryDelays,
	}
	events.Subscribe("*", s.handleAny)
	return s
}

func (s *Service) SetURLValidator(fn func(string) error) {
	s.validateURL = fn
}

func (s *Service) SetRetryDelays(delays []time.Duration) {
	s.retryDelays = delays
}

func (s *Service) handleAny(ctx context.Context, ev shared.Event) {
	cfgs, err := s.repo.List(ctx)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			s.log.Error("webhook: list configs failed", slog.Any("err", err))
		}
		return
	}
	for _, cfg := range cfgs {
		if !cfg.Enabled {
			continue
		}
		if !cfg.MatchesEvent(ev.EventName()) {
			continue
		}
		select {
		case s.sem <- struct{}{}:
			go func(c *webhook.Config) {
				defer func() { <-s.sem }()
				defer func() {
					if r := recover(); r != nil {
						s.log.Error("webhook: goroutine panic", slog.Any("recover", r), slog.String("webhook", c.ID))
					}
				}()
				dCtx, cancel := context.WithTimeout(ctx, deliveryTimeout)
				defer cancel()
				s.deliver(dCtx, c, ev.EventName(), ev)
			}(cfg)
		default:
			s.log.Warn("webhook: delivery queue full, dropping", slog.String("webhook", cfg.ID))
		}
	}
}

func (s *Service) deliver(ctx context.Context, cfg *webhook.Config, eventName string, payload any) {
	if err := s.validateURL(cfg.URL); err != nil {
		s.log.Warn("webhook URL blocked", slog.String("webhook", cfg.ID), slog.Any("err", err))
		return
	}

	body, err := json.Marshal(map[string]any{
		"event":   eventName,
		"payload": payload,
	})
	if err != nil {
		s.log.Warn("webhook: marshal payload failed", slog.String("webhookID", cfg.ID), slog.Any("err", err))
		return
	}

	// The delivery id is minted BEFORE sending and rides every attempt as the
	// X-Evey-Delivery header: delivery is at-least-once (network timeouts make
	// retries indistinguishable from duplicates on the receiving end), so a
	// consumer needs a stable occurrence id to deduplicate on. Previously the
	// id was stored server-side only — never sent — leaving consumers no way
	// to tell a retry from a genuinely new event.
	deliveryID := uuid.New().String()
	lastErr, statusCode := s.attemptDeliver(ctx, cfg, deliveryID, body)

	// Persist the audit on a context detached from the delivery deadline. The
	// retry budget (1+5+30s) can exceed deliveryTimeout (30s), so when retries
	// are exhausted ctx is already expired by the time we get here — and the
	// exhausted-retry record is exactly the one worth keeping. Writing under the
	// dead ctx would silently drop it; give the write its own short budget.
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.deliveries.Create(auditCtx, &webhook.Delivery{
		ID:         deliveryID,
		WebhookID:  cfg.ID,
		Event:      eventName,
		StatusCode: statusCode,
		Error:      lastErr,
		SentAt:     time.Now(),
	}); err != nil {
		s.log.Warn("store webhook delivery failed", slog.String("webhookID", cfg.ID), slog.Any("err", err))
	}
}

func (s *Service) attemptDeliver(ctx context.Context, cfg *webhook.Config, deliveryID string, body []byte) (string, int) {
	var lastErr string
	var statusCode int

	headers := make(map[string]string, len(cfg.Headers)+2)
	headers["X-Evey-Signature"] = sign(cfg.Secret, body)
	headers["X-Evey-Delivery"] = deliveryID
	maps.Copy(headers, cfg.Headers)

	for attempt := 0; attempt <= len(s.retryDelays); attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(s.retryDelays[attempt-1]):
			case <-ctx.Done():
				return ctx.Err().Error(), statusCode
			}
		}

		sc, err := s.sender.Send(ctx, cfg.URL, headers, body)
		if err != nil {
			lastErr = err.Error()
			s.log.Warn("webhook delivery failed", slog.String("webhook", cfg.ID), slog.Int("attempt", attempt+1), slog.Any("err", err))
			continue
		}
		statusCode = sc

		if sc < 300 {
			return "", statusCode
		}
		lastErr = fmt.Sprintf("HTTP %d", sc)
		s.log.Warn("webhook non-2xx", slog.String("webhook", cfg.ID), slog.Int("status", sc))
	}

	return lastErr, statusCode
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return fmt.Sprintf("sha256=%x", mac.Sum(nil))
}

func (s *Service) Create(ctx context.Context, cfg *webhook.Config) error {
	ctx, span := tracing.StartSpan(ctx, "app.webhook.Create")
	defer span.End()
	cfg.ID = uuid.New().String()
	cfg.CreatedAt = time.Now()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid webhook: %w", err)
	}
	if err := s.validateURL(cfg.URL); err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	return s.repo.Create(ctx, cfg)
}

func (s *Service) List(ctx context.Context) ([]*webhook.Config, error) {
	ctx, span := tracing.StartSpan(ctx, "app.webhook.List")
	defer span.End()
	return s.repo.List(ctx)
}

func (s *Service) Update(ctx context.Context, cfg *webhook.Config) error {
	ctx, span := tracing.StartSpan(ctx, "app.webhook.Update")
	defer span.End()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid webhook: %w", err)
	}
	if err := s.validateURL(cfg.URL); err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	return s.repo.Update(ctx, cfg)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "app.webhook.Delete")
	defer span.End()
	return s.repo.Delete(ctx, id)
}

func (s *Service) ListDeliveries(ctx context.Context, webhookID string, limit int) ([]*webhook.Delivery, error) {
	ctx, span := tracing.StartSpan(ctx, "app.webhook.ListDeliveries")
	defer span.End()
	return s.deliveries.ListByWebhook(ctx, webhookID, limit)
}
