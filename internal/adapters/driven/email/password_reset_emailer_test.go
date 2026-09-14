package email

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
)

func TestPasswordResetEmailer_Register(t *testing.T) {
	bus := &fakeBus{}
	emailer := NewPasswordResetEmailer(&fakeSender{}, func() string { return "https://evey.test" }, nil)
	emailer.Register(bus)

	if _, ok := bus.handlers["user.password.reset_requested"]; !ok {
		t.Fatal("expected handler registered for user.password.reset_requested")
	}
}

func TestPasswordResetEmailer_SendsEmail(t *testing.T) {
	sender := &fakeSender{}
	bus := &fakeBus{}
	emailer := NewPasswordResetEmailer(sender, func() string { return "https://evey.test" }, nil)
	emailer.Register(bus)

	ev := domainuser.PasswordResetRequested{
		UserID:     shared.UserID("usr-1"),
		Email:      "reset@example.com",
		ResetToken: "tok-abc",
		ExpiresAt:  time.Now().Add(30 * time.Minute),
		OccurredAt: time.Now(),
	}
	_ = bus.Publish(context.Background(), ev)

	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 email, got %d", len(sender.sent))
	}
	if sender.sent[0].to[0] != "reset@example.com" {
		t.Errorf("to = %v, want reset@example.com", sender.sent[0].to)
	}
	if sender.sent[0].subject != "Evey - Password Reset" {
		t.Errorf("subject = %q, want Evey - Password Reset", sender.sent[0].subject)
	}
	if !containsStr(sender.sent[0].body, "https://evey.test/reset-password?token=tok-abc") {
		t.Errorf("body missing reset URL: %q", sender.sent[0].body)
	}
}

// The admin's server name brands the subject; a blank name falls back to Evey
// rather than sending " - Password Reset".
func TestPasswordResetEmailer_SubjectCarriesServerName(t *testing.T) {
	for name, want := range map[string]string{"Salon": "Salon - Password Reset", "  ": "Evey - Password Reset"} {
		sender := &fakeSender{}
		bus := &fakeBus{}
		NewPasswordResetEmailer(sender, func() string { return "https://evey.test" }, func() string { return name }).Register(bus)
		_ = bus.Publish(context.Background(), domainuser.PasswordResetRequested{Email: "r@example.com", ResetToken: "t"})
		if len(sender.sent) != 1 || sender.sent[0].subject != want {
			t.Errorf("name %q: sent=%+v, want subject %q", name, sender.sent, want)
		}
	}
}

func TestPasswordResetEmailer_WrongEventType_NoSend(t *testing.T) {
	sender := &fakeSender{}
	bus := &fakeBus{}
	emailer := NewPasswordResetEmailer(sender, func() string { return "https://evey.test" }, nil)
	emailer.Register(bus)

	// publish a different event on the same topic by calling the handler directly
	if h, ok := bus.handlers["user.password.reset_requested"]; ok {
		h(context.Background(), domainuser.PasswordChanged{UserID: "usr-1"})
	}

	if len(sender.sent) != 0 {
		t.Fatalf("expected 0 emails for wrong event type, got %d", len(sender.sent))
	}
}

func TestPasswordResetEmailer_SendFails_NoPanic(t *testing.T) {
	sender := &failSender{err: errors.New("smtp down")}
	bus := &fakeBus{}
	emailer := NewPasswordResetEmailer(sender, func() string { return "https://evey.test" }, nil)
	emailer.Register(bus)

	ev := domainuser.PasswordResetRequested{
		UserID:     shared.UserID("usr-2"),
		Email:      "user@example.com",
		ResetToken: "tok-xyz",
		OccurredAt: time.Now(),
	}
	_ = bus.Publish(context.Background(), ev)
	// should not panic; handler swallows error after logging
}
