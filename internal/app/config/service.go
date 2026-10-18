package config

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/id"
	"github.com/eveyproject/evey/internal/pkg/tracing"
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
	// KeyServerID holds this server's stable identity, minted on first read.
	KeyServerID = "server.id"
)

type Service struct {
	repo   shared.ServerConfigRepository
	events shared.EventBus
	tx     shared.TransactionalUnit

	idMu     sync.Mutex
	serverID string
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

// WithUnitOfWork makes SetMany all-or-nothing.
func WithUnitOfWork(tx shared.TransactionalUnit) Option {
	return func(s *Service) { s.tx = tx }
}

func (s *Service) Get(ctx context.Context, key string) (*shared.ServerConfigEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "app.config.Get")
	defer span.End()
	return s.repo.Get(ctx, key)
}

// ServerID is this server's stable identity: a random id minted on the first
// read and persisted, so it survives restarts and upgrades and is the same for
// every client. SystemInfo.server_id was declared, read by the web and the
// phone, and never written. Serialised so two first readers cannot mint two.
func (s *Service) ServerID(ctx context.Context) (string, error) {
	ctx, span := tracing.StartSpan(ctx, "app.config.ServerID")
	defer span.End()
	s.idMu.Lock()
	defer s.idMu.Unlock()
	if s.serverID != "" {
		return s.serverID, nil
	}
	e, err := s.repo.Get(ctx, KeyServerID)
	switch {
	case err == nil && e != nil && e.Value != "":
		s.serverID = e.Value
		return s.serverID, nil
	case err != nil && !errors.Is(err, shared.ErrNotFound):
		return "", fmt.Errorf("config.ServerID: %w", err)
	}
	minted := id.New("srv")
	if err := s.repo.Set(ctx, KeyServerID, minted); err != nil {
		return "", fmt.Errorf("config.ServerID: %w", err)
	}
	s.serverID = minted
	return minted, nil
}

func (s *Service) List(ctx context.Context) ([]*shared.ServerConfigEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "app.config.List")
	defer span.End()
	return s.repo.List(ctx)
}

func (s *Service) Set(ctx context.Context, key, value string) error {
	ctx, span := tracing.StartSpan(ctx, "app.config.Set")
	defer span.End()
	if err := ValidateEntry(key, value); err != nil {
		return err
	}
	if err := s.repo.Set(ctx, key, value); err != nil {
		return err
	}
	s.publishChange(ctx, key, value, false)
	return nil
}

// SetMany stores several settings as one change: every value is validated
// first, the writes share one transaction (when a unit of work is wired), and
// change events go out only once all of them are stored. Settings written one
// by one could stop halfway - SSO sign-ups switched on while the domain and
// library restrictions meant to go with them failed to save.
func (s *Service) SetMany(ctx context.Context, entries map[string]string) error {
	ctx, span := tracing.StartSpan(ctx, "app.config.SetMany")
	defer span.End()
	keys := make([]string, 0, len(entries))
	for k := range entries {
		if err := ValidateEntry(k, entries[k]); err != nil {
			return err
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	write := func(ctx context.Context) error {
		for _, k := range keys {
			if err := s.repo.Set(ctx, k, entries[k]); err != nil {
				return fmt.Errorf("set %s: %w", k, err)
			}
		}
		return nil
	}
	var err error
	if s.tx != nil {
		err = s.tx.Do(ctx, write)
	} else {
		err = write(ctx)
	}
	if err != nil {
		return err
	}
	for _, k := range keys {
		s.publishChange(ctx, k, entries[k], false)
	}
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
		Secret:     value != "" && IsSecretKey(key),
	}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
}
