package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/webhook"
	"github.com/evey-project/evey/internal/pkg/urlvalidator"
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

	lastErr, statusCode := s.attemptDeliver(ctx, cfg, body)

	if err := s.deliveries.Create(ctx, &webhook.Delivery{
		ID:         uuid.New().String(),
		WebhookID:  cfg.ID,
		Event:      eventName,
		StatusCode: statusCode,
		Error:      lastErr,
		SentAt:     time.Now(),
	}); err != nil {
		s.log.Warn("store webhook delivery failed", slog.String("webhookID", cfg.ID), slog.Any("err", err))
	}
}

func (s *Service) attemptDeliver(ctx context.Context, cfg *webhook.Config, body []byte) (string, int) {
	var lastErr string
	var statusCode int

	headers := make(map[string]string, len(cfg.Headers)+1)
	headers["X-Evey-Signature"] = sign(cfg.Secret, body)
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
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid webhook: %w", err)
	}
	if err := s.validateURL(cfg.URL); err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	cfg.ID = uuid.New().String()
	cfg.CreatedAt = time.Now()
	return s.repo.Create(ctx, cfg)
}

func (s *Service) List(ctx context.Context) ([]*webhook.Config, error) {
	return s.repo.List(ctx)
}

func (s *Service) Update(ctx context.Context, cfg *webhook.Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid webhook: %w", err)
	}
	if err := s.validateURL(cfg.URL); err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	return s.repo.Update(ctx, cfg)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) ListDeliveries(ctx context.Context, webhookID string, limit int) ([]*webhook.Delivery, error) {
	return s.deliveries.ListByWebhook(ctx, webhookID, limit)
}
