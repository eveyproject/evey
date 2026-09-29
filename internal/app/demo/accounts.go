package demo

import (
	"context"
	"fmt"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/id"
)

// The accounts a demo visitor finds under "Users": each shows one product rule
// and `demo` can "connect as" any of them. Nobody signs in as these accounts,
// so their password is random; the PIN is part of the demo (the kids-profile
// rule needs one on the adult profile) and is printed by the seed.
const SeedPIN = "0000"

type seedProfile struct {
	name   string
	locale string
	region string
	theme  string
	kids   bool
	maxAge int
}

type seedAccount struct {
	username, email      string
	maxStreams           int // 0 = unlimited
	maxDevicesPerProfile int // 0 = unlimited
	maxBitrateKbps       int // 0 = unlimited
	disabled             bool
	moviesOnly           bool // library access confined to the films library
	profiles             []seedProfile
	why                  string
}

var seedAccounts = []seedAccount{
	{
		username: "famille", email: "famille@demo.invalid", maxStreams: 2,
		profiles: []seedProfile{
			{name: "Camille", locale: "fr", region: "FR", theme: "default"},
			{name: "Lou", locale: "fr", region: "FR", theme: "light-blue", kids: true, maxAge: 10},
		},
		why: "a household: two streams at once, a kids profile the adult profile guards with a PIN",
	},
	{
		username: "solo", email: "solo@demo.invalid", maxDevicesPerProfile: 1, maxBitrateKbps: 4000,
		profiles: []seedProfile{{name: "Alex", locale: "en", region: "US", theme: "amoled"}},
		why:      "one device at a time and a 4 Mb/s ceiling: the per-user limits from the inside",
	},
	{
		username: "invite", email: "invite@demo.invalid", moviesOnly: true,
		profiles: []seedProfile{{name: "Sam", locale: "es", region: "ES", theme: "dark-red"}},
		why:      "a guest who sees the films library only (library access)",
	},
	{
		username: "ancien", email: "ancien@demo.invalid", disabled: true,
		profiles: []seedProfile{{name: "Robin", locale: "de", region: "DE", theme: "default"}},
		why:      "a disabled account: listed, cannot sign in",
	},
}

// SeedAccounts creates the visitor-facing accounts that do not exist yet and
// reports one line per account. Existing accounts are left alone, so it can
// run at every boot.
func (b *Bootstrapper) SeedAccounts(ctx context.Context) error {
	for _, a := range seedAccounts {
		if existing, err := b.users.GetByUsername(ctx, a.username); err == nil && existing != nil {
			b.log.Debug("demo: account exists", "username", a.username)
			continue
		}
		if err := b.seedAccount(ctx, a); err != nil {
			return fmt.Errorf("seed %s: %w", a.username, err)
		}
		b.log.Info("demo: account created", "username", a.username, "why", a.why)
	}
	return nil
}

func (b *Bootstrapper) seedAccount(ctx context.Context, a seedAccount) error {
	password, err := randomPassword()
	if err != nil {
		return err
	}
	u, err := user.NewUser(a.username, a.email, "pending")
	if err != nil {
		return err
	}
	if a.maxStreams > 0 {
		v := a.maxStreams
		u.MaxStreams = &v
	}
	if a.maxDevicesPerProfile > 0 {
		v := a.maxDevicesPerProfile
		u.MaxDevicesPerProfile = &v
	}
	u.MaxBitrateKbps = a.maxBitrateKbps
	if cerr := b.accounts.CreateUser(ctx, true, u, password); cerr != nil {
		return cerr
	}
	if a.disabled {
		u.Enabled = false
		if uerr := b.users.Update(ctx, u); uerr != nil {
			return fmt.Errorf("disable: %w", uerr)
		}
	}

	// CreateUser made a default profile named after the account; the first
	// seed profile takes its place so the household reads as intended.
	existing, err := b.profiles.GetByUserID(ctx, u.ID)
	if err != nil {
		return fmt.Errorf("list profiles: %w", err)
	}
	pinHash, err := b.hasher.Hash(SeedPIN)
	if err != nil {
		return err
	}
	hasKids := false
	for _, p := range a.profiles {
		hasKids = hasKids || p.kids
	}
	for i, p := range a.profiles {
		opts := []user.ProfileOption{
			user.WithProfileLocale(p.locale),
			user.WithProfileRegion(p.region),
			user.WithProfilePreferences(map[string]string{"theme": p.theme}),
			user.WithProfileIsKids(p.kids),
		}
		switch {
		case p.kids:
			maxAge := p.maxAge
			opts = append(opts, user.WithProfileParental(&user.ParentalSettings{MaxAge: &maxAge, Enabled: true}))
		case hasKids:
			// The rule the kids profile creates: every non-kids profile of the
			// account carries a PIN, or the kids profile is one tap away from
			// unrestricted content.
			opts = append(opts, user.WithProfileParental(&user.ParentalSettings{PINHash: pinHash, Enabled: true}))
		}
		if i == 0 && len(existing) > 0 {
			prof, perr := user.NewProfile(existing[0].ID, u.ID, p.name, append(opts, user.WithProfileDefault(true))...)
			if perr != nil {
				return perr
			}
			prof.CreatedAt = existing[0].CreatedAt
			if err := b.accounts.UpdateProfile(ctx, prof); err != nil {
				return fmt.Errorf("reshape default profile: %w", err)
			}
			continue
		}
		prof, perr := user.NewProfile(shared.ProfileID(id.New("prf-")), u.ID, p.name, opts...)
		if perr != nil {
			return perr
		}
		if err := b.accounts.CreateProfile(ctx, prof); err != nil {
			return fmt.Errorf("profile %s: %w", p.name, err)
		}
	}
	return nil
}

// ConfineAccounts gives the seeded accounts the demo libraries only (and the
// guest the films alone), so "connect as" shows the demo and nothing else the
// instance may hold. libs maps the demo library folder to its id.
func (b *Bootstrapper) ConfineAccounts(ctx context.Context, libs map[string]shared.LibraryID) error {
	if b.access == nil || len(libs) == 0 {
		return nil
	}
	all := make([]shared.LibraryID, 0, len(libs))
	for _, id := range libs {
		all = append(all, id)
	}
	for _, a := range seedAccounts {
		u, err := b.users.GetByUsername(ctx, a.username)
		if err != nil || u == nil {
			continue
		}
		want := all
		if a.moviesOnly {
			if films, ok := libs["movies"]; ok {
				want = []shared.LibraryID{films}
			}
		}
		if err := b.access.ReplaceAccess(ctx, u.ID, want); err != nil {
			return fmt.Errorf("library access for %s: %w", a.username, err)
		}
	}
	return nil
}
