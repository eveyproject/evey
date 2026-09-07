package user

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// setup.go holds the first-run bootstrap: creating the initial admin user and
// its default profile atomically, and the one-shot state that latches Setup
// closed once an admin exists.

func (s *CommandService) InitSetupState(ctx context.Context) {
	count, err := s.users.CountAdmins(ctx)
	if err == nil && count > 0 {
		s.setupDone = true
	}
}

// SetupOption configures optional inputs for the initial admin Setup.
type SetupOption func(*setupOptions)

type setupOptions struct {
	locale string
	region string
}

// WithSetupLocale seeds the admin default profile with the language the user
// picked in the setup wizard, so the JWT locale claim and subsequent sessions
// stay in that language instead of falling back to the DB column default.
func WithSetupLocale(locale string) SetupOption {
	return func(o *setupOptions) { o.locale = locale }
}

// WithSetupRegion seeds the admin default profile with an ISO-3166 region.
// Empty string is ignored (region stays nil → server fallback at runtime).
func WithSetupRegion(region string) SetupOption {
	return func(o *setupOptions) { o.region = region }
}

func (s *CommandService) Setup(ctx context.Context, username, password, email string, opts ...SetupOption) (*user.User, error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.Setup")
	defer span.End()
	var cfg setupOptions
	for _, fn := range opts {
		fn(&cfg)
	}
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	if s.setupDone {
		return nil, ErrSetupDone
	}
	if username == "" {
		return nil, ErrUsernameRequired
	}
	if len(password) < minPasswordLength {
		return nil, ErrPasswordTooShort
	}
	admins, err := s.users.CountAdmins(ctx)
	if err != nil {
		return nil, fmt.Errorf("CommandService.Setup: %w", err)
	}
	if admins > 0 {
		s.setupDone = true
		return nil, ErrSetupDone
	}

	hash, err := s.hasher.Hash(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u, err := user.NewUser(username, email, hash)
	if err != nil {
		return nil, fmt.Errorf("CommandService.Setup: %w", err)
	}
	u.IsAdmin = true

	profileOpts := []user.ProfileOption{
		user.WithProfileDefault(true),
		user.WithProfileLocale(cfg.locale),
	}
	if cfg.region != "" {
		profileOpts = append(profileOpts, user.WithProfileRegion(cfg.region))
	}
	p, err := user.NewProfile(shared.ProfileID(id.New("prf-")), u.ID, u.Username, profileOpts...)
	if err != nil {
		return nil, fmt.Errorf("CommandService.Setup: build default profile: %w", err)
	}

	// The admin user and its default profile must be created atomically: a
	// profile-insert failure after the user row commits would leave the sole
	// admin with NO default profile AND lock Setup forever (the retry sees
	// CountAdmins>0 → ErrSetupDone), with no code path that heals a missing
	// default profile. Wrap both in one tx, mirroring UserService.CreateUser.
	apply := func(ctx context.Context) error {
		if err := s.users.Create(ctx, u); err != nil {
			return fmt.Errorf("create admin user: %w", err)
		}
		if err := s.profiles.Create(ctx, p); err != nil {
			return fmt.Errorf("create default profile: %w", err)
		}
		return nil
	}
	var txErr error
	if s.uow != nil {
		txErr = s.uow.Do(ctx, apply)
	} else {
		txErr = apply(ctx)
	}
	if txErr != nil {
		return nil, fmt.Errorf("CommandService.Setup: %w", txErr)
	}

	// Events are published after the commit so a rolled-back Setup never emits
	// a phantom UserCreated/ProfileCreated.
	evtSetupCreated := user.UserCreated{UserID: u.ID, Username: u.Username, IsAdmin: true, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtSetupCreated); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtSetupCreated.EventName()), slog.Any("err", err))
	}
	evtProfileCreated := user.ProfileCreated{ProfileID: p.ID, UserID: p.UserID, Name: p.Name, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtProfileCreated); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtProfileCreated.EventName()), slog.Any("err", err))
	}

	s.setupDone = true
	return u, nil
}
