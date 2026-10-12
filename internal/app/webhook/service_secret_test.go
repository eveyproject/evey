package webhook_test

import (
	"context"
	"strings"
	"testing"
	"time"

	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	webhookdomain "github.com/eveyproject/evey/internal/domain/webhook"
)

// A `*` webhook receives every event, so the raw password reset token must
// never reach its body (nor the stored delivery row): it IS the account.
func TestHandleAny_PasswordResetTokenNeverLeavesTheProcess(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	_, sender := newTestService(repo, deliveries, bus)

	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "all", URL: "http://hook.example", Events: []string{"*"}, Enabled: true,
	})
	repo.mu.Unlock()

	const token = "0123456789abcdef-raw-reset-token"
	_ = bus.Publish(context.Background(), domainuser.PasswordResetRequested{
		UserID: "u1", Email: "reset@example.com", ResetToken: token,
		ExpiresAt: time.Now().Add(30 * time.Minute), OccurredAt: time.Now(),
	})

	waitForCalls(t, sender, 1, 2*time.Second)
	waitForDeliveries(t, deliveries, 1, 2*time.Second)
	body := string(sender.Calls()[0].Body)
	if !strings.Contains(body, "user.password.reset_requested") {
		t.Fatalf("event not delivered: %s", body)
	}
	if strings.Contains(body, token) {
		t.Fatalf("webhook body carries the raw reset token: %s", body)
	}
	deliveries.mu.Lock()
	defer deliveries.mu.Unlock()
	for _, d := range deliveries.data {
		if strings.Contains(d.Event+d.Error, token) {
			t.Fatalf("stored delivery carries the raw reset token: %+v", d)
		}
	}
}
