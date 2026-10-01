package email

import (
	"context"
	"log/slog"

	domainnotif "github.com/evey-project/evey/internal/domain/notification"
	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
)

// ProfileGetter resolves a profile by ID.
type ProfileGetter interface {
	GetByID(ctx context.Context, id shared.ProfileID) (*domainuser.Profile, error)
}

// UserGetter resolves a user by ID.
type UserGetter interface {
	GetByID(ctx context.Context, id shared.UserID) (*domainuser.User, error)
}

// NotificationEmailer subscribes to notification.created and sends emails.
type NotificationEmailer struct {
	sender   Sender
	profiles ProfileGetter
	users    UserGetter
	// serverNameFn names the instance in the body (admin "Server name"); read
	// per mail so a rename applies without a restart. nil = "Evey".
	serverNameFn func() string
}

func NewNotificationEmailer(sender Sender, profiles ProfileGetter, users UserGetter, serverNameFn func() string) *NotificationEmailer {
	return &NotificationEmailer{sender: sender, profiles: profiles, users: users, serverNameFn: serverNameFn}
}

// Register subscribes to notification.created on the given bus.
func (e *NotificationEmailer) Register(bus shared.EventBus) {
	bus.Subscribe("notification.created", e.handle)
}

func (e *NotificationEmailer) handle(ctx context.Context, ev shared.Event) {
	notif, ok := ev.(domainnotif.NotificationCreated)
	if !ok {
		return
	}
	// Skip the profile/user lookups while no SMTP host is in force.
	if c, ok := e.sender.(interface{ Configured() bool }); ok && !c.Configured() {
		return
	}

	profile, err := e.profiles.GetByID(ctx, notif.ProfileID)
	if err != nil {
		slog.Warn("email: get profile", slog.String("profile_id", string(notif.ProfileID)), slog.Any("err", err))
		return
	}

	u, err := e.users.GetByID(ctx, profile.UserID)
	if err != nil {
		slog.Warn("email: get user", slog.String("user_id", string(profile.UserID)), slog.Any("err", err))
		return
	}

	if u.Email == "" {
		return
	}

	subject := notif.Title
	body := notif.Title + "\n\nYou have a new notification on " + serverNameOrDefault(e.serverNameFn) + "."

	if err := e.sender.Send([]string{u.Email}, subject, body); err != nil {
		slog.Warn("email: send notification", slog.String("to", u.Email), slog.Any("err", err))
	}
}
