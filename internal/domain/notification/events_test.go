package notification_test

import (
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/notification"
)

func TestNotificationEventNames(t *testing.T) {
	t.Parallel()

	now := time.Now()
	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{notification.NotificationCreated{OccurredAt: now}, "notification.created"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			t.Parallel()
			got := c.event.EventName()
			if got != c.want {
				t.Errorf("EventName() = %q, want %q", got, c.want)
			}
		})
	}
}
