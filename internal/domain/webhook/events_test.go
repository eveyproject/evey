package webhook_test

import (
	"testing"

	"github.com/gravity-zero/evey/internal/domain/webhook"
)

func TestEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{webhook.WebhookCreated{}, "webhook.created"},
		{webhook.WebhookUpdated{}, "webhook.updated"},
		{webhook.WebhookDeleted{}, "webhook.deleted"},
		{webhook.DeliveryFailed{}, "webhook.delivery.failed"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
