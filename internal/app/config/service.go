package config

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/shared"
)

type Service struct {
	repo shared.ServerConfigRepository
}

func NewService(repo shared.ServerConfigRepository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Get(ctx context.Context, key string) (*shared.ServerConfigEntry, error) {
	return s.repo.Get(ctx, key)
}

func (s *Service) List(ctx context.Context) ([]*shared.ServerConfigEntry, error) {
	return s.repo.List(ctx)
}

func (s *Service) Set(ctx context.Context, key, value string) error {
	return s.repo.Set(ctx, key, value)
}

func (s *Service) Delete(ctx context.Context, key string) error {
	return s.repo.Delete(ctx, key)
}
