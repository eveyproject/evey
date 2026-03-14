package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/webhook"
	"github.com/gravity-zero/evey/internal/pkg/urlvalidator"
)

const (
	maxConcurrentDeliveries = 10
	deliveryTimeout         = 30 * time.Second
)

var defaultRetryDelays = []time.Duration{1 * time.Second, 5 * time.Second, 30 * time.Second}

type Service struct {
	repo        webhook.Repository
	deliveries  webhook.DeliveryRepository
	log         *slog.Logger
	httpClient  *http.Client
	validateURL func(string) error
	sem         chan struct{}
	retryDelays []time.Duration
}

func NewService(
	repo webhook.Repository,
	deliveries webhook.DeliveryRepository,
	log *slog.Logger,
	events shared.EventBus,
) *Service {
	s := &Service{
		repo:        repo,
		deliveries:  deliveries,
		log:         log,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
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
		s.log.Error("webhook: list configs failed", slog.Any("err", err))
		return
	}
	for _, cfg := range cfgs {
		if !cfg.Enabled {
			continue
		}
		if !matchesAny(cfg.Events, ev.EventName()) {
			continue
		}
		select {
		case s.sem <- struct{}{}:
			go func(c *webhook.Config) {
				defer func() { <-s.sem }()
				dCtx, cancel := context.WithTimeout(ctx, deliveryTimeout)
				defer cancel()
				s.deliver(dCtx, c, ev.EventName(), ev)
			}(cfg)
		default:
			s.log.Warn("webhook: delivery queue full, dropping", slog.String("webhook", cfg.ID))
		}
	}
}

func matchesAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if p == "*" || p == name {
			return true
		}
	}
	return false
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

	for attempt := 0; attempt <= len(s.retryDelays); attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(s.retryDelays[attempt-1]):
			case <-ctx.Done():
				return ctx.Err().Error(), statusCode
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
		if err != nil {
			s.log.Error("webhook: bad URL", slog.String("webhook", cfg.ID), slog.Any("err", err))
			lastErr = err.Error()
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Evey-Signature", sign(cfg.Secret, body))
		for k, v := range cfg.Headers {
			req.Header.Set(k, v)
		}

		resp, err := s.httpClient.Do(req)
		if err != nil {
			lastErr = err.Error()
			s.log.Warn("webhook delivery failed", slog.String("webhook", cfg.ID), slog.Int("attempt", attempt+1), slog.Any("err", err))
			continue
		}
		resp.Body.Close()
		statusCode = resp.StatusCode

		if resp.StatusCode < 300 {
			return "", statusCode
		}
		lastErr = fmt.Sprintf("HTTP %d", resp.StatusCode)
		s.log.Warn("webhook non-2xx", slog.String("webhook", cfg.ID), slog.Int("status", resp.StatusCode))
	}

	return lastErr, statusCode
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return fmt.Sprintf("sha256=%x", mac.Sum(nil))
}

func (s *Service) Create(ctx context.Context, cfg *webhook.Config) error {
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
