package notification

import (
	"context"
	"log/slog"
	"time"

	"github.com/evey-project/evey/internal/domain/backup"
	"github.com/evey-project/evey/internal/domain/mediarequest"
	"github.com/evey-project/evey/internal/domain/notification"
	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/google/uuid"
)

// event_handlers.go holds the thin adapters from bus events (backup failure,
// plugin error, media request lifecycle) to the notification fan-out. Unlike
// the media-added path, these are stateless: each event maps directly to one
// batch of per-profile notifications.

func (s *CommandService) handleBackupFailed(parentCtx context.Context, ev shared.Event) {
	// Recover locally (mirror of handlePluginError): the event bus RETRIES a
	// panicking handler, and this one loops persist+push+email over every
	// profile - a panic mid-loop (e.g. a malformed push subscription) would
	// re-run the whole loop and duplicate notifications for the profiles
	// already served.
	defer func() {
		if rv := recover(); rv != nil {
			slog.ErrorContext(parentCtx, "handleBackupFailed panic recovered", slog.Any("panic", rv))
		}
	}()

	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()
	e, ok := ev.(backup.BackupFailed)
	if !ok {
		return
	}

	profiles, err := s.profiles.GetAll(ctx)
	if err != nil {
		slog.WarnContext(ctx, "notification: list profiles for backup.failed", slog.Any("err", err))
		return
	}

	expires := time.Now().AddDate(0, 0, defaultRetentionDays)
	title := "Scheduled Backup Failed"
	push := pushSpec{key: "notifications.push.backup_failed.title", fallback: title}
	fanoutID := uuid.New().String()
	for _, p := range profiles {
		n, err := notification.NewNotification(uuid.New().String(), p.ID, notification.TypeBackupFailed, title,
			notification.WithNotificationMessage(e.Error),
			notification.WithNotificationLevel(notification.LevelAll),
			notification.WithNotificationExpiresAt(&expires),
		)
		if err != nil {
			slog.WarnContext(ctx, "notification build failed", slog.String("profileID", string(p.ID)), slog.Any("err", err))
			continue
		}
		s.persistAndDispatch(ctx, n, push, p.Locale, fanoutID)
	}
}

func (s *CommandService) handlePluginError(parentCtx context.Context, ev shared.Event) {
	// Recover locally so a deterministic panic on a single event does not
	// trigger the bus retry loop and burn queue slots at full speed.
	defer func() {
		if rv := recover(); rv != nil {
			var pluginID string
			if e, ok := ev.(plugin.PluginError); ok {
				pluginID = e.PluginID
			}
			slog.ErrorContext(parentCtx, "handlePluginError panic recovered",
				slog.Any("panic", rv),
				slog.String("plugin_id", pluginID))
		}
	}()

	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()
	e, ok := ev.(plugin.PluginError)
	if !ok {
		return
	}

	profiles, err := s.listAdminTargetProfiles(ctx)
	if err != nil {
		slog.WarnContext(ctx, "notification: list admin profiles for plugin.error", slog.Any("err", err))
		return
	}

	title := "Plugin error"
	push := pushSpec{key: "notifications.push.plugin_error.title", fallback: title}
	if e.PluginID != "" {
		title = "Plugin error: " + e.PluginID
		push = pushSpec{
			key:      "notifications.push.plugin_error_id.title",
			params:   map[string]string{"plugin": e.PluginID},
			fallback: title,
		}
	}
	msg := e.ErrorMessage
	if msg == "" {
		msg = "plugin reported an error"
	}
	expires := time.Now().AddDate(0, 0, defaultRetentionDays)
	fanoutID := uuid.New().String()
	for _, p := range profiles {
		n, err := notification.NewNotification(uuid.New().String(), p.ID, notification.TypePluginError, title,
			notification.WithNotificationMessage(msg),
			notification.WithNotificationLevel(notification.LevelAll),
			notification.WithNotificationExpiresAt(&expires),
		)
		if err != nil {
			slog.WarnContext(ctx, "notification build failed", slog.String("profileID", string(p.ID)), slog.Any("err", err))
			continue
		}
		s.persistAndDispatch(ctx, n, push, p.Locale, fanoutID)
	}
	slog.InfoContext(ctx, "plugin error notified",
		slog.String("plugin_id", e.PluginID),
		slog.Int("admins", len(profiles)))
}

// listAdminTargetProfiles prefers the optional AdminProfileLister so
// plugin errors page only admin users. Falls back to all profiles when
// no admin lookup is wired (keeps dev/test setups working).
func (s *CommandService) listAdminTargetProfiles(ctx context.Context) ([]*user.Profile, error) {
	if s.adminLister != nil {
		return s.adminLister.ListAdminProfiles(ctx)
	}
	return s.profiles.GetAll(ctx)
}

// handleMediaRequestCreated pages the operators (admin-only) that a user
// submitted a new media request.
func (s *CommandService) handleMediaRequestCreated(parentCtx context.Context, ev shared.Event) {
	defer func() {
		if rv := recover(); rv != nil {
			slog.ErrorContext(parentCtx, "handleMediaRequestCreated panic recovered", slog.Any("panic", rv))
		}
	}()
	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()
	e, ok := ev.(mediarequest.RequestCreated)
	if !ok {
		return
	}
	profiles, err := s.listAdminTargetProfiles(ctx)
	if err != nil {
		slog.WarnContext(ctx, "notification: list admin profiles for media.request.created", slog.Any("err", err))
		return
	}
	title := "New media request: " + e.Title
	push := pushSpec{
		key:      "notifications.push.media_request.title",
		params:   map[string]string{"title": e.Title},
		fallback: title,
	}
	expires := time.Now().AddDate(0, 0, defaultRetentionDays)
	fanoutID := uuid.New().String()
	for _, p := range profiles {
		n, err := notification.NewNotification(uuid.New().String(), p.ID, notification.TypeMediaRequest, title,
			notification.WithNotificationMessage(string(e.MediaType)),
			notification.WithNotificationLevel(notification.LevelAll),
			notification.WithNotificationExpiresAt(&expires),
		)
		if err != nil {
			slog.WarnContext(ctx, "notification build failed", slog.String("profileID", string(p.ID)), slog.Any("err", err))
			continue
		}
		s.persistAndDispatch(ctx, n, push, p.Locale, fanoutID)
	}
	slog.InfoContext(ctx, "media request notified to admins", slog.String("request_id", e.RequestID), slog.Int("admins", len(profiles)))
}

// handleRequestAnswered notifies the requester of the operator's decision.
func (s *CommandService) handleRequestAnswered(parentCtx context.Context, ev shared.Event) {
	defer func() {
		if rv := recover(); rv != nil {
			slog.ErrorContext(parentCtx, "handleRequestAnswered panic recovered", slog.Any("panic", rv))
		}
	}()
	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()
	e, ok := ev.(mediarequest.RequestAnswered)
	if !ok {
		return
	}
	// Resolve the requester's profile to localise the push title. Missing
	// profile (deleted between request and answer) simply skips.
	locale := ""
	if all, err := s.profiles.GetAll(ctx); err == nil {
		for _, p := range all {
			if p.ID == e.ProfileID {
				locale = p.Locale
				break
			}
		}
	}
	title := "Your media request was updated: " + e.Title
	push := pushSpec{
		key:      "notifications.push.request_answered.title",
		params:   map[string]string{"title": e.Title, "status": string(e.Status)},
		fallback: title,
	}
	msg := "Status: " + string(e.Status)
	if e.Note != "" {
		msg += " - " + e.Note
	}
	expires := time.Now().AddDate(0, 0, defaultRetentionDays)
	n, err := notification.NewNotification(uuid.New().String(), e.ProfileID, notification.TypeRequestAnswered, title,
		notification.WithNotificationMessage(msg),
		notification.WithNotificationLevel(notification.LevelAll),
		notification.WithNotificationExpiresAt(&expires),
	)
	if err != nil {
		slog.WarnContext(ctx, "notification build failed", slog.String("profileID", string(e.ProfileID)), slog.Any("err", err))
		return
	}
	s.persistAndDispatch(ctx, n, push, locale, uuid.New().String())
	slog.InfoContext(ctx, "media request answer notified", slog.String("request_id", e.RequestID), slog.String("status", string(e.Status)))
}
