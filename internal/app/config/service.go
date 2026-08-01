package config

import (
	"context"
	"log/slog"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// Persisted setting keys live here, not in the handlers, because the SAME
// setting is written by two entry points (the hand-written REST route and the
// gRPC one) and read back by a third at boot. When the key was a private
// constant of the gRPC package, the REST route could not name it - and did not
// persist at all, so every change an operator made through the admin panel was
// lost on the next restart while the panel showed it applied.
const (
	KeyPlaybackSkipForward  = "playback.skip_forward_seconds"
	KeyPlaybackSkipBackward = "playback.skip_backward_seconds"
)

type Service struct {
	repo   shared.ServerConfigRepository
	events shared.EventBus
}

func NewService(repo shared.ServerConfigRepository, opts ...Option) *Service {
	s := &Service{repo: repo}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Option configures the Service. WithEventBus enables live config reload:
// every Set/Delete publishes a ServerConfigChanged event so subscribers
// holding cached state (auto-download flag, intro providers, etc.) can
// react without a server restart. Optional - when nil the service is a
// pure CRUD facade.
type Option func(*Service)

func WithEventBus(bus shared.EventBus) Option {
	return func(s *Service) { s.events = bus }
}

func (s *Service) Get(ctx context.Context, key string) (*shared.ServerConfigEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "app.config.Get")
	defer span.End()
	return s.repo.Get(ctx, key)
}

func (s *Service) List(ctx context.Context) ([]*shared.ServerConfigEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "app.config.List")
	defer span.End()
	return s.repo.List(ctx)
}

func (s *Service) Set(ctx context.Context, key, value string) error {
	ctx, span := tracing.StartSpan(ctx, "app.config.Set")
	defer span.End()
	if err := s.repo.Set(ctx, key, value); err != nil {
		return err
	}
	s.publishChange(ctx, key, value, false)
	return nil
}

func (s *Service) Delete(ctx context.Context, key string) error {
	ctx, span := tracing.StartSpan(ctx, "app.config.Delete")
	defer span.End()
	if err := s.repo.Delete(ctx, key); err != nil {
		return err
	}
	s.publishChange(ctx, key, "", true)
	return nil
}

func (s *Service) publishChange(ctx context.Context, key, value string, deleted bool) {
	if s.events == nil {
		return
	}
	evt := shared.ServerConfigChanged{
		Key:        key,
		Value:      value,
		Deleted:    deleted,
		OccurredAt: time.Now(),
	}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
}
