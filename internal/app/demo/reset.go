package demo

import (
	"context"
	"fmt"
	"time"

	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
)

// ActiveStreams reads how many playbacks are running: the reset never runs
// under a visitor who is watching.
type ActiveStreams interface {
	GetServerStats(ctx context.Context) (*activity.ServerStatsResult, error)
}

// ProgressReset clears a profile's watch progress.
type ProgressReset interface {
	DeleteByProfile(ctx context.Context, profileID string) (int, error)
}

// Credentials are the stored credentials a demo account can hold: API keys
// and invite links. Both are locked on the demo (policy.go) but some were
// minted before the locks, and neither expires on its own: a leftover invite
// lets anyone register an account (RegisterWithInvite is public).
type Credentials interface {
	ListAPIKeys(ctx context.Context, userID shared.UserID) ([]*user.APIKeyModel, error)
	DeleteAPIKey(ctx context.Context, id string) error
	ListInvites(ctx context.Context, creatorID string) ([]*user.InviteLink, error)
	DeleteInvite(ctx context.Context, id string) error
}

// WithCredentialPurge makes the boot and every reset delete the demo accounts'
// API keys and invite links.
func (b *Bootstrapper) WithCredentialPurge(c Credentials) *Bootstrapper {
	b.credentials = c
	return b
}

// demoUsernames is every account the demo owns: the shared administrator and
// the seeded visitor accounts.
func (b *Bootstrapper) demoUsernames() []string {
	names := []string{b.adminUser}
	for _, a := range seedAccounts {
		names = append(names, a.username)
	}
	return names
}

// PurgeCredentials deletes every API key and invite link of the demo accounts.
func (b *Bootstrapper) PurgeCredentials(ctx context.Context) error {
	if b.credentials == nil {
		return nil
	}
	for _, name := range b.demoUsernames() {
		u, err := b.users.GetByUsername(ctx, name)
		if err != nil || u == nil {
			continue
		}
		keys, err := b.credentials.ListAPIKeys(ctx, u.ID)
		if err != nil {
			return fmt.Errorf("demo: list api keys of %s: %w", name, err)
		}
		for _, k := range keys {
			if derr := b.credentials.DeleteAPIKey(ctx, k.ID); derr != nil {
				return fmt.Errorf("demo: delete api key of %s: %w", name, derr)
			}
		}
		invites, err := b.credentials.ListInvites(ctx, string(u.ID))
		if err != nil {
			return fmt.Errorf("demo: list invites of %s: %w", name, err)
		}
		for _, inv := range invites {
			if derr := b.credentials.DeleteInvite(ctx, inv.ID); derr != nil {
				return fmt.Errorf("demo: delete invite of %s: %w", name, derr)
			}
		}
		if n := len(keys) + len(invites); n > 0 {
			b.log.Info("demo: credentials purged", "account", name, "api_keys", len(keys), "invites", len(invites))
		}
	}
	return nil
}

// WithActivity adds a second, live "someone is here" signal to the idle
// check: the play_sessions count lags (a closed tab stays open ~2 min) and
// knows nothing of a running HLS encode without a session row or of music
// (audit 2026-10-01). busy reports true while anything plays.
func (b *Bootstrapper) WithActivity(busy func() bool) *Bootstrapper {
	b.busy = busy
	return b
}

// WithReset enables the periodic reset (RunResets).
func (b *Bootstrapper) WithReset(streams ActiveStreams, progress ProgressReset) *Bootstrapper {
	b.streams, b.progressReset = streams, progress
	return b
}

// RunResets puts the demo back to its seeded state every `every`, when
// nobody is watching, and at the latest every `floor` whatever the traffic.
// Every write a visitor could make to shared state is locked (policy.go), so
// what drifts is the watch progress of the shared profiles; the rest of the
// seed (password, plugin config, home, missing media) is re-asserted by the
// bootstrap itself. It replaces the host-side restore of the data directory
// the demo needed while those writes were open. Blocks until ctx ends.
func (b *Bootstrapper) RunResets(ctx context.Context, every, floor time.Duration) {
	last := time.Now()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			done, err := b.ResetIfIdle(ctx, now.Sub(last) >= floor)
			if err != nil {
				b.log.Warn("demo: reset", "err", err)
				continue
			}
			if done {
				last = now
			}
		}
	}
}

// ResetIfIdle resets when nothing is playing, or regardless when force (the
// daily floor). It reports whether it reset.
func (b *Bootstrapper) ResetIfIdle(ctx context.Context, force bool) (bool, error) {
	if b.streams == nil || b.progressReset == nil {
		return false, nil
	}
	if !force {
		stats, err := b.streams.GetServerStats(ctx)
		if err != nil {
			// Not knowing is not idle: a reset under a viewer is the failure
			// to avoid, the next tick asks again.
			return false, fmt.Errorf("demo reset: read active streams: %w", err)
		}
		if stats != nil && stats.ActiveStreams > 0 {
			b.log.Debug("demo: reset skipped, someone is watching", "streams", stats.ActiveStreams)
			return false, nil
		}
		if b.busy != nil && b.busy() {
			b.log.Debug("demo: reset skipped, a playback or a music session is live")
			return false, nil
		}
	}
	if err := b.Reset(ctx); err != nil {
		return false, err
	}
	b.log.Info("demo: reset to the seeded state", "forced", force)
	return true, nil
}

// Reset clears the watch progress of every demo profile (the `demo` account
// and the seeded visitor accounts a visitor can impersonate), then re-runs the
// idempotent seed: administrator and its published password, libraries,
// plugins and their config, home layout, watch history.
func (b *Bootstrapper) Reset(ctx context.Context) error {
	for _, name := range b.demoUsernames() {
		u, err := b.users.GetByUsername(ctx, name)
		if err != nil || u == nil {
			continue
		}
		profiles, err := b.profiles.GetByUserID(ctx, u.ID)
		if err != nil {
			return fmt.Errorf("demo reset: profiles of %s: %w", name, err)
		}
		for _, p := range profiles {
			if _, err := b.progressReset.DeleteByProfile(ctx, string(p.ID)); err != nil {
				return fmt.Errorf("demo reset: clear progress of %s: %w", name, err)
			}
		}
	}
	if err := b.PurgeCredentials(ctx); err != nil {
		return err
	}
	if err := b.EnsureAdmin(ctx); err != nil {
		return err
	}
	if err := b.SeedAccounts(ctx); err != nil {
		return err
	}
	libs, _, err := b.EnsureLibraries(ctx)
	if err != nil {
		return err
	}
	plugins, err := b.EnsurePlugins(ctx)
	if err != nil {
		return err
	}
	if err := b.ArrangeHome(ctx, libs, plugins); err != nil {
		return err
	}
	return b.SeedHistory(ctx, libs)
}
