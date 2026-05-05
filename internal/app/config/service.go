package config

import (
	"context"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type Service struct {
	repo shared.ServerConfigRepository
}

func NewService(repo shared.ServerConfigRepository) *Service {
	return &Service{repo: repo}
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
	return s.repo.Set(ctx, key, value)
}

func (s *Service) Delete(ctx context.Context, key string) error {
	ctx, span := tracing.StartSpan(ctx, "app.config.Delete")
	defer span.End()
	return s.repo.Delete(ctx, key)
}
