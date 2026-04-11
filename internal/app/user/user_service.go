package user

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

type UserService struct {
	users       user.UserRepository
	profiles    user.ProfileRepository
	refreshTkns user.RefreshTokenRepository
	hasher      user.PasswordHasher
	events      shared.EventBus
}

func NewUserService(
	users user.UserRepository,
	profiles user.ProfileRepository,
	refreshTkns user.RefreshTokenRepository,
	hasher user.PasswordHasher,
	events shared.EventBus,
) *UserService {
	return &UserService{
		users:       users,
		profiles:    profiles,
		refreshTkns: refreshTkns,
		hasher:      hasher,
		events:      events,
	}
}

func (s *UserService) CreateUser(ctx context.Context, callerIsAdmin bool, u *user.User, password string) error {
	if !callerIsAdmin {
		return ErrNotAdmin
	}
	if err := u.Validate(); err != nil {
		return fmt.Errorf("invalid user: %w", err)
	}
	if len(password) < 8 {
		return ErrPasswordTooShort
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	u.PasswordHash = hash
	u.CreatedAt = time.Now()
	u.UpdatedAt = time.Now()
	if err := s.users.Create(ctx, u); err != nil {
		return fmt.Errorf("UserService.CreateUser: %w", err)
	}
	evtUserCreated := user.UserCreated{UserID: u.ID, Username: u.Username, IsAdmin: u.IsAdmin, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtUserCreated); err != nil {
		slog.Warn("publish event", slog.String("event", evtUserCreated.EventName()), slog.Any("err", err))
	}

	p := &user.Profile{
		ID:        id.New("prf-"),
		UserID:    u.ID,
		Name:      u.Username,
		IsDefault: true,
		CreatedAt: time.Now(),
	}
	if pErr := s.profiles.Create(ctx, p); pErr != nil {
		slog.Warn("auto-create profile", slog.String("userID", u.ID), slog.Any("err", pErr))
	} else {
		evtProfile := user.ProfileCreated{ProfileID: p.ID, UserID: p.UserID, Name: p.Name, OccurredAt: time.Now()}
		if err := s.events.Publish(ctx, evtProfile); err != nil {
			slog.Warn("publish event", slog.String("event", evtProfile.EventName()), slog.Any("err", err))
		}
	}
	return nil
}

func (s *UserService) UpdateUser(ctx context.Context, u *user.User) error {
	u.UpdatedAt = time.Now()
	if err := s.users.Update(ctx, u); err != nil {
		return fmt.Errorf("UserService.UpdateUser: %w", err)
	}
	return nil
}

func (s *UserService) SetUserBitrateLimit(ctx context.Context, userID string, maxBitrateKbps int) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("UserService.SetUserBitrateLimit: %w", err)
	}
	u.MaxBitrateKbps = maxBitrateKbps
	u.UpdatedAt = time.Now()
	if err := s.users.Update(ctx, u); err != nil {
		return fmt.Errorf("UserService.SetUserBitrateLimit: %w", err)
	}
	return nil
}

func (s *UserService) ChangePassword(ctx context.Context, userID, newPassword string) error {
	if len(newPassword) < 8 {
		return ErrPasswordTooShort
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("UserService.ChangePassword: %w", err)
	}
	hash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	u.PasswordHash = hash
	u.UpdatedAt = time.Now()
	if err := s.users.Update(ctx, u); err != nil {
		return fmt.Errorf("UserService.ChangePassword: %w", err)
	}
	if err := s.refreshTkns.DeleteByUserID(ctx, userID); err != nil {
		slog.Warn("delete refresh tokens after password change", slog.String("userID", userID), slog.Any("err", err))
	}
	evt := user.PasswordChanged{UserID: userID, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *UserService) DeleteUser(ctx context.Context, userID string) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("UserService.DeleteUser: %w", err)
	}
	if u.IsAdmin {
		count, err := s.users.CountAdmins(ctx)
		if err != nil {
			return fmt.Errorf("count admins: %w", err)
		}
		if count <= 1 {
			return ErrLastAdmin
		}
	}
	if err := s.users.Delete(ctx, userID); err != nil {
		return fmt.Errorf("UserService.DeleteUser: %w", err)
	}
	if err := s.refreshTkns.DeleteByUserID(ctx, userID); err != nil {
		slog.Warn("delete refresh tokens for deleted user failed", slog.String("userID", userID), slog.Any("err", err))
	}
	evtDeleted := user.UserDeleted{UserID: userID, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtDeleted); err != nil {
		slog.Warn("publish event", slog.String("event", evtDeleted.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *UserService) DisableUser(ctx context.Context, userID string) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("UserService.DisableUser: %w", err)
	}
	if u.IsAdmin {
		count, err := s.users.CountAdmins(ctx)
		if err != nil {
			return fmt.Errorf("count admins: %w", err)
		}
		if count <= 1 {
			return ErrLastAdmin
		}
	}
	u.Enabled = false
	u.UpdatedAt = time.Now()
	if err := s.users.Update(ctx, u); err != nil {
		return fmt.Errorf("UserService.DisableUser: %w", err)
	}
	if err := s.refreshTkns.DeleteByUserID(ctx, userID); err != nil {
		slog.Warn("delete refresh tokens for disabled user failed", slog.String("userID", userID), slog.Any("err", err))
	}
	evtDisabled := user.UserDisabled{UserID: userID, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtDisabled); err != nil {
		slog.Warn("publish event", slog.String("event", evtDisabled.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *UserService) CreateProfile(ctx context.Context, p *user.Profile) error {
	if err := p.Validate(); err != nil {
		return fmt.Errorf("invalid profile: %w", err)
	}
	p.CreatedAt = time.Now()
	if err := s.profiles.Create(ctx, p); err != nil {
		return fmt.Errorf("UserService.CreateProfile: %w", err)
	}
	evtProfileCreated := user.ProfileCreated{ProfileID: p.ID, UserID: p.UserID, Name: p.Name, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtProfileCreated); err != nil {
		slog.Warn("publish event", slog.String("event", evtProfileCreated.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *UserService) UpdateProfile(ctx context.Context, p *user.Profile) error {
	if err := s.profiles.Update(ctx, p); err != nil {
		return fmt.Errorf("UserService.UpdateProfile: %w", err)
	}
	return nil
}

func (s *UserService) DeleteProfile(ctx context.Context, profileID string) error {
	p, err := s.profiles.GetByID(ctx, profileID)
	if err != nil {
		return fmt.Errorf("UserService.DeleteProfile: %w", err)
	}
	if err := s.profiles.Delete(ctx, profileID); err != nil {
		return fmt.Errorf("UserService.DeleteProfile: %w", err)
	}
	evt := user.ProfileDeleted{ProfileID: profileID, UserID: p.UserID, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}
