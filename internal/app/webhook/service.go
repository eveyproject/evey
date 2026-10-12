package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/webhook"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"github.com/eveyproject/evey/internal/pkg/urlvalidator"
	"github.com/google/uuid"
)

const (
	maxConcurrentDeliveries = 10
	deliveryTimeout         = 30 * time.Second
)

var defaultRetryDelays = []time.Duration{1 * time.Second, 5 * time.Second, 30 * time.Second}

// ErrInvalidConfig wraps a rejected webhook configuration (failed Validate or a
// blocked/invalid URL) so the transport can map it to InvalidArgument rather
// than a generic Internal error. Distinguishable with errors.Is.
var ErrInvalidConfig = errors.New("invalid webhook configuration")

// ErrDeliveryFailed wraps a Test() attempt that never got a 2xx response (network
// error, timeout, or non-2xx after every retry) so the transport can map it to a
// client-visible error rather than a generic Internal one - an admin clicking
// "Test" needs to know their webhook target is broken, not see a muted 500.
var ErrDeliveryFailed = errors.New("webhook test delivery failed")

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
				_, _ = s.deliver(dCtx, c, ev.EventName(), ev)
			}(cfg)
		default:
			s.log.Warn("webhook: delivery queue full, dropping", slog.String("webhook", cfg.ID))
		}
	}
}

// deliver sends one event to cfg's URL (signing, retries, timeout) and audits
// the attempt. It returns the outcome (lastErr empty on a 2xx, statusCode the
// receiver's response) so callers like Test - which need to report success or
// failure back to an admin - don't have to duplicate the send/retry logic.
func (s *Service) deliver(ctx context.Context, cfg *webhook.Config, eventName string, payload any) (lastErr string, statusCode int) {
	if err := s.validateURL(cfg.URL); err != nil {
		s.log.Warn("webhook URL blocked", slog.String("webhook", cfg.ID), slog.Any("err", err))
		return err.Error(), 0
	}

	body, err := json.Marshal(map[string]any{
		"event":   eventName,
		"payload": payload,
	})
	if err != nil {
		s.log.Warn("webhook: marshal payload failed", slog.String("webhookID", cfg.ID), slog.Any("err", err))
		return err.Error(), 0
	}

	// The delivery id is minted BEFORE sending and rides every attempt as the
	// X-Evey-Delivery header: delivery is at-least-once (network timeouts make
	// retries indistinguishable from duplicates on the receiving end), so a
	// consumer needs a stable occurrence id to deduplicate on. Previously the
	// id was stored server-side only - never sent - leaving consumers no way
	// to tell a retry from a genuinely new event.
	deliveryID := uuid.New().String()
	lastErr, statusCode = s.attemptDeliver(ctx, cfg, deliveryID, body)

	// Persist the audit on a context detached from the delivery deadline. The
	// retry budget (1+5+30s) can exceed deliveryTimeout (30s), so when retries
	// are exhausted ctx is already expired by the time we get here - and the
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
	return lastErr, statusCode
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
		return fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}
	if err := s.validateURL(cfg.URL); err != nil {
		return fmt.Errorf("%w: invalid URL: %w", ErrInvalidConfig, err)
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
		return fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}
	if err := s.validateURL(cfg.URL); err != nil {
		return fmt.Errorf("%w: invalid URL: %w", ErrInvalidConfig, err)
	}

	// The client never sees a webhook's secret in clear (grpcapi masks it on
	// every read), so whatever it sends back on Update is either that mask or
	// an empty string - never proof of an intentional change. Both admin UIs
	// round-trip the value they last read, so trusting cfg.Secret verbatim
	// here would overwrite a working secret with "abcd****" and every future
	// delivery would be signed with the mask and silently rejected by the
	// receiver. Resolve against the currently stored secret instead: keep it
	// unless the client actually sent a genuinely new one. If the record can't
	// be read (e.g. a transient repo error), fall through and let repo.Update
	// fail the same way it always did - we only skip the merge, not the write.
	if existing, err := s.repo.GetByID(ctx, cfg.ID); err == nil {
		cfg.Secret = resolveUpdatedSecret(existing.Secret, cfg.Secret)
	} else if !errors.Is(err, shared.ErrNotFound) {
		s.log.WarnContext(ctx, "webhook: could not read existing config before update, secret merge skipped", slog.String("webhookID", cfg.ID), slog.Any("err", err))
	}

	return s.repo.Update(ctx, cfg)
}

// resolveUpdatedSecret decides what secret Update should persist, given the
// currently stored one and whatever the client sent. It exists to close a
// data-destruction bug: a client can only ever have seen the masked secret
// (grpcapi.webhookConfigToProto never returns it in clear), so an incoming
// value that reproduces that mask - or is empty, since proto3 can't tell
// "absent" from "" on a bare string field - means "unchanged", not "clear it
// out" or "here is the real secret". Only a value that does NOT match the
// mask of the stored secret is treated as an intentional rotation.
func resolveUpdatedSecret(stored, incoming string) string {
	if incoming == "" {
		return stored
	}
	if constantTimeEqualStrings(incoming, maskStoredSecret(stored)) {
		return stored
	}
	return incoming
}

// maskStoredSecret mirrors grpcapi.maskSecret exactly. The app layer can't
// import that driving adapter (it would invert the hexagonal dependency and,
// since grpcapi already imports this package, create an import cycle), yet
// this is the only place that holds both the stored secret and the incoming
// value, so the merge decision has to live here. Keep this in lockstep with
// grpcapi.maskSecret if that format ever changes.
func maskStoredSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return s[:4] + "****"
}

// constantTimeEqualStrings compares two secret-derived values without leaking
// timing information, matching the ConstantTimeCompare pattern already used
// for CSRF and scrape-token checks elsewhere in this codebase.
func constantTimeEqualStrings(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
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

// testEventName marks a delivery as an operator-triggered test rather than a
// real domain event, so it is distinguishable in the delivery audit log.
const testEventName = "webhook.test"

// TestResult is the outcome of a manual "Test" ping, returned so the caller
// (the admin HTTP handler) can report both success and failure without
// reaching into delivery internals.
type TestResult struct {
	StatusCode int
	DurationMS int64
}

// Test sends a synthetic event through the exact same delivery path as real
// events (HMAC signature, headers, retries, timeout), so a successful test
// proves the configured URL is actually reachable - not just well-formed. The
// attempt is recorded in the delivery audit log like any other delivery.
func (s *Service) Test(ctx context.Context, id string) (*TestResult, error) {
	ctx, span := tracing.StartSpan(ctx, "app.webhook.Test")
	defer span.End()

	cfg, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	dCtx, cancel := context.WithTimeout(ctx, deliveryTimeout)
	defer cancel()
	start := time.Now()
	lastErr, statusCode := s.deliver(dCtx, cfg, testEventName, map[string]string{"message": "This is a test event from Evey"})
	if lastErr != "" {
		return nil, fmt.Errorf("%w: %s", ErrDeliveryFailed, lastErr)
	}
	return &TestResult{StatusCode: statusCode, DurationMS: time.Since(start).Milliseconds()}, nil
}
