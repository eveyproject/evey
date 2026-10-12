package notification

import (
	"context"
	"errors"
	"log/slog"

	"github.com/eveyproject/evey/internal/domain/notification"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// push.go holds notification persistence + web-push dispatch and the push
// subscription management API. persistAndDispatch is the shared "persist +
// push + publish" primitive used by both the batching path and the event
// handlers.

// fanoutID is one id per fan-out (shared by its per-profile rows); locale is
// the profile's, for the push title.
func (s *CommandService) persistAndDispatch(ctx context.Context, n *notification.Notification, push pushSpec, locale, fanoutID string) {
	if createErr := s.repo.Create(ctx, n); createErr != nil {
		slog.WarnContext(ctx, "create notification failed", slog.String("profileID", string(n.ProfileID)), slog.Any("err", createErr))
		return
	}
	s.sendPush(ctx, n.ProfileID, push.resolveTitle(s.translator, locale), push.resolveBody(s.translator, locale, n.Message))
	evt := notification.NotificationCreated{
		NotificationID: n.ID,
		ProfileID:      n.ProfileID,
		Type:           n.Type,
		Title:          n.Title,
		MediaItemID:    n.MediaItemID,
		OccurredAt:     n.CreatedAt,
		FanoutID:       fanoutID,
		TitleKey:       push.key,
		TitleParams:    push.params,
		TitleFallback:  push.fallback,
		Message:        n.Message,
	}
	if pubErr := s.events.Publish(ctx, evt); pubErr != nil {
		slog.WarnContext(ctx, "publish notification.created", slog.Any("err", pubErr))
	}
}

func (s *CommandService) sendPush(ctx context.Context, profileID shared.ProfileID, title, message string) {
	if s.push == nil || s.subs == nil {
		return
	}
	subs, err := s.subs.GetByProfile(ctx, profileID)
	if err != nil {
		slog.WarnContext(ctx, "webpush: get subscriptions failed", slog.String("profileID", string(profileID)), slog.Any("err", err))
		return
	}
	for _, sub := range subs {
		sendErr := s.push.Send(ctx, sub, title, message, nil)
		if sendErr == nil {
			continue
		}
		if errors.Is(sendErr, notification.ErrSubscriptionGone) {
			// The endpoint is permanently dead (410); prune it so we stop
			// retrying it on every future notification.
			if delErr := s.subs.DeleteByEndpoint(ctx, sub.UserID, sub.Endpoint); delErr != nil {
				slog.WarnContext(ctx, "webpush: delete gone subscription failed", slog.String("endpoint", sub.Endpoint), slog.Any("err", delErr))
			}
			continue
		}
		slog.WarnContext(ctx, "webpush: send failed", slog.String("profileID", string(profileID)), slog.String("endpoint", sub.Endpoint), slog.Any("err", sendErr))
	}
}

func (s *CommandService) UpsertPushSubscription(ctx context.Context, userID shared.UserID, profileID shared.ProfileID, endpoint, p256dh, auth string) error {
	ctx, span := tracing.StartSpan(ctx, "app.notification.UpsertPushSubscription")
	defer span.End()
	if s.subs == nil {
		return nil
	}
	return s.subs.Upsert(ctx, userID, profileID, endpoint, p256dh, auth)
}

func (s *CommandService) DeletePushByEndpoint(ctx context.Context, userID shared.UserID, endpoint string) error {
	ctx, span := tracing.StartSpan(ctx, "app.notification.DeletePushByEndpoint")
	defer span.End()
	if s.subs == nil {
		return nil
	}
	return s.subs.DeleteByEndpoint(ctx, userID, endpoint)
}

func (s *CommandService) DeletePushByUser(ctx context.Context, userID shared.UserID) error {
	ctx, span := tracing.StartSpan(ctx, "app.notification.DeletePushByUser")
	defer span.End()
	if s.subs == nil {
		return nil
	}
	return s.subs.DeleteByUser(ctx, userID)
}
