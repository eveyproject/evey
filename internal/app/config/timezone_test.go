package config

import (
	"context"
	"errors"
	"testing"

	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// The admin's zone is validated on write, served live after a change, and a
// stored zone that no longer loads falls back instead of breaking the clock.
func TestServerTimezone_ValidatedAndSwappedLive(t *testing.T) {
	ctx := context.Background()
	repo := sharedfakes.NewFakeServerConfigRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := NewService(repo, WithEventBus(bus))
	clock := NewServerClock(nil)
	ApplyServerTimezone(ctx, repo, bus, clock)

	if err := svc.Set(ctx, KeyServerTimezone, "Mars/Olympus"); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("an unknown zone must be refused, got %v", err)
	}
	if err := svc.Set(ctx, KeyServerTimezone, "America/New_York"); err != nil {
		t.Fatal(err)
	}
	if got := clock.Location().String(); got != "America/New_York" {
		t.Errorf("clock = %s after the change, want America/New_York", got)
	}

	_ = repo.Set(ctx, KeyServerTimezone, "Not/AZone")
	if l := LoadServerTimezone(ctx, repo); l == nil || l.String() == "Not/AZone" {
		t.Errorf("an unreadable stored zone must fall back, got %v", l)
	}
	if err := ValidateEntry("server.name", "anything"); err != nil {
		t.Errorf("free-text keys are not validated: %v", err)
	}
	var _ shared.ServerConfigRepository = repo
}
