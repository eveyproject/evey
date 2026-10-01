package config

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

// KeyServerTimezone is the IANA zone the server reads wall-clock times in:
// the opening hours of kids profiles are "18:00 in this zone", whatever the
// clock of the child's device says.
const KeyServerTimezone = "server.timezone"

// DefaultServerTimezone applies when nothing is set and TZ is not either.
const DefaultServerTimezone = "Europe/Paris"

// ErrInvalidValue is a config value the server refuses to store.
var ErrInvalidValue = errors.New("invalid config value")

// ValidateEntry refuses values a key cannot work with. Only keys with a
// format are checked; every other key stays free text.
func ValidateEntry(key, value string) error {
	if key == KeyServerTimezone && value != "" {
		if _, err := time.LoadLocation(value); err != nil {
			return fmt.Errorf("%w: %q is not a known time zone", ErrInvalidValue, value)
		}
	}
	return nil
}

// ServerClock holds the server's time zone, swapped live when an admin
// changes it.
type ServerClock struct {
	loc atomic.Pointer[time.Location]
}

func NewServerClock(loc *time.Location) *ServerClock {
	c := &ServerClock{}
	c.loc.Store(loc)
	return c
}

// Location is never nil.
func (c *ServerClock) Location() *time.Location {
	if c == nil {
		return defaultLocation()
	}
	if l := c.loc.Load(); l != nil {
		return l
	}
	return defaultLocation()
}

func defaultLocation() *time.Location {
	if tz := os.Getenv("TZ"); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			return l
		}
	}
	if l, err := time.LoadLocation(DefaultServerTimezone); err == nil {
		return l
	}
	return time.UTC
}

// LoadServerTimezone reads the stored zone; unset or unreadable falls back to
// TZ, then to DefaultServerTimezone.
func LoadServerTimezone(ctx context.Context, repo shared.ServerConfigRepository) *time.Location {
	e, err := repo.Get(ctx, KeyServerTimezone)
	if err != nil || e == nil || e.Value == "" {
		return defaultLocation()
	}
	l, err := time.LoadLocation(e.Value)
	if err != nil {
		slog.WarnContext(ctx, "server time zone unreadable, using the default", slog.String("value", e.Value), slog.Any("err", err))
		return defaultLocation()
	}
	return l
}

// ApplyServerTimezone loads the zone into clock now and swaps it whenever an
// admin changes it.
func ApplyServerTimezone(ctx context.Context, repo shared.ServerConfigRepository, bus shared.EventBus, clock *ServerClock) {
	clock.loc.Store(LoadServerTimezone(ctx, repo))
	if bus == nil {
		return
	}
	bus.Subscribe(shared.ServerConfigChanged{}.EventName(), func(evtCtx context.Context, ev shared.Event) {
		changed, ok := ev.(shared.ServerConfigChanged)
		if !ok || changed.Key != KeyServerTimezone {
			return
		}
		l := LoadServerTimezone(evtCtx, repo)
		clock.loc.Store(l)
		slog.InfoContext(evtCtx, "server time zone updated", slog.String("zone", l.String()))
	})
}
