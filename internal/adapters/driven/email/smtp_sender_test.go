package email

import (
	"context"
	"errors"
	"testing"
	"time"

	domainnotif "github.com/gravity-zero/evey/internal/domain/notification"
	"github.com/gravity-zero/evey/internal/domain/shared"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
)

// --- interface compliance ---

var _ Sender = (*SMTPSender)(nil)

// --- SMTPSender tests ---

func TestNewSMTPSender(t *testing.T) {
	s := NewSMTPSender("smtp.example.com", 587, "user", "pass", "from@example.com", "starttls")
	if s == nil {
		t.Fatal("expected non-nil sender")
	}
	if s.host != "smtp.example.com" {
		t.Fatalf("expected host smtp.example.com, got %s", s.host)
	}
	if s.port != 587 {
		t.Fatalf("expected port 587, got %d", s.port)
	}
}

func TestSend_EmptyHost_Noop(t *testing.T) {
	s := NewSMTPSender("", 587, "", "", "from@example.com", "none")
	if err := s.Send([]string{"to@example.com"}, "subject", "body"); err != nil {
		t.Fatalf("expected nil error for empty host, got %v", err)
	}
}

func TestBuildMessage(t *testing.T) {
	msg := buildMessage("from@a.com", []string{"to@b.com", "to@c.com"}, "Hello", "World")
	s := string(msg)
	checks := []string{
		"From: from@a.com\r\n",
		"To: to@b.com, to@c.com\r\n",
		"Subject: Hello\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: text/plain; charset=UTF-8\r\n",
		"\r\nWorld",
	}
	for _, want := range checks {
		if !contains(s, want) {
			t.Errorf("message missing %q", want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- NotificationEmailer tests ---

type fakeSender struct {
	sent []sentEmail
}

type sentEmail struct {
	to      []string
	subject string
	body    string
}

func (f *fakeSender) Send(to []string, subject, body string) error {
	f.sent = append(f.sent, sentEmail{to: to, subject: subject, body: body})
	return nil
}

type fakeProfileGetter struct {
	profiles map[string]*domainuser.Profile
}

func (f *fakeProfileGetter) GetByID(_ context.Context, id shared.ProfileID) (*domainuser.Profile, error) {
	p, ok := f.profiles[string(id)]
	if !ok {
		return nil, errors.New("not found")
	}
	return p, nil
}

type fakeUserGetter struct {
	users map[string]*domainuser.User
}

func (f *fakeUserGetter) GetByID(_ context.Context, id shared.UserID) (*domainuser.User, error) {
	u, ok := f.users[string(id)]
	if !ok {
		return nil, errors.New("not found")
	}
	return u, nil
}

type fakeBus struct {
	handlers map[string]func(context.Context, shared.Event)
}

func (f *fakeBus) Publish(ctx context.Context, event shared.Event) error {
	if h, ok := f.handlers[event.EventName()]; ok {
		h(ctx, event)
	}
	return nil
}

func (f *fakeBus) Subscribe(eventName string, handler func(context.Context, shared.Event)) string {
	if f.handlers == nil {
		f.handlers = make(map[string]func(context.Context, shared.Event))
	}
	f.handlers[eventName] = handler
	return eventName
}

func (f *fakeBus) Unsubscribe(_ string) {}

func TestNotificationEmailer_Register(t *testing.T) {
	bus := &fakeBus{}
	emailer := NewNotificationEmailer(&fakeSender{}, &fakeProfileGetter{}, &fakeUserGetter{})
	emailer.Register(bus)

	if _, ok := bus.handlers["notification.created"]; !ok {
		t.Fatal("expected handler registered for notification.created")
	}
}

func TestNotificationEmailer_SendsEmail(t *testing.T) {
	sender := &fakeSender{}
	profiles := &fakeProfileGetter{profiles: map[string]*domainuser.Profile{
		"prof-1": {ID: "prof-1", UserID: "usr-1"},
	}}
	users := &fakeUserGetter{users: map[string]*domainuser.User{
		"usr-1": {ID: "usr-1", Email: "user@example.com"},
	}}

	bus := &fakeBus{}
	emailer := NewNotificationEmailer(sender, profiles, users)
	emailer.Register(bus)

	ev := domainnotif.NotificationCreated{
		NotificationID: "n-1",
		ProfileID:      "prof-1",
		Title:          "New Movie",
		OccurredAt:     time.Now(),
	}
	_ = bus.Publish(context.Background(), ev)

	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 email, got %d", len(sender.sent))
	}
	if sender.sent[0].to[0] != "user@example.com" {
		t.Fatalf("expected to user@example.com, got %s", sender.sent[0].to[0])
	}
	if sender.sent[0].subject != "New Movie" {
		t.Fatalf("expected subject New Movie, got %s", sender.sent[0].subject)
	}
}

func TestNotificationEmailer_NoEmail_SkipsSend(t *testing.T) {
	sender := &fakeSender{}
	profiles := &fakeProfileGetter{profiles: map[string]*domainuser.Profile{
		"prof-1": {ID: "prof-1", UserID: "usr-1"},
	}}
	users := &fakeUserGetter{users: map[string]*domainuser.User{
		"usr-1": {ID: "usr-1", Email: ""},
	}}

	bus := &fakeBus{}
	emailer := NewNotificationEmailer(sender, profiles, users)
	emailer.Register(bus)

	ev := domainnotif.NotificationCreated{ProfileID: "prof-1", Title: "X", OccurredAt: time.Now()}
	_ = bus.Publish(context.Background(), ev)

	if len(sender.sent) != 0 {
		t.Fatalf("expected 0 emails for empty email, got %d", len(sender.sent))
	}
}

func TestNotificationEmailer_WrongEventType_NoSend(t *testing.T) {
	sender := &fakeSender{}
	profiles := &fakeProfileGetter{profiles: map[string]*domainuser.Profile{}}
	users := &fakeUserGetter{users: map[string]*domainuser.User{}}

	bus := &fakeBus{}
	emailer := NewNotificationEmailer(sender, profiles, users)
	emailer.Register(bus)

	// publish a non-NotificationCreated event
	type fakeEvent struct{}
	fe := fakeEvent{}
	// call handle directly with wrong type
	if h, ok := bus.handlers["notification.created"]; ok {
		h(context.Background(), domainnotif.NotificationRead{ProfileID: "x"})
		_ = fe
	}

	if len(sender.sent) != 0 {
		t.Fatalf("expected 0 emails for wrong event type, got %d", len(sender.sent))
	}
}

func TestNotificationEmailer_UserNotFound_NoSend(t *testing.T) {
	sender := &fakeSender{}
	profiles := &fakeProfileGetter{profiles: map[string]*domainuser.Profile{
		"prof-1": {ID: "prof-1", UserID: "usr-missing"},
	}}
	users := &fakeUserGetter{users: map[string]*domainuser.User{}}

	bus := &fakeBus{}
	emailer := NewNotificationEmailer(sender, profiles, users)
	emailer.Register(bus)

	ev := domainnotif.NotificationCreated{ProfileID: "prof-1", Title: "X", OccurredAt: time.Now()}
	_ = bus.Publish(context.Background(), ev)

	if len(sender.sent) != 0 {
		t.Fatalf("expected 0 emails when user not found, got %d", len(sender.sent))
	}
}

type failSender struct {
	err error
}

func (f *failSender) Send(to []string, subject, body string) error {
	return f.err
}

func TestNotificationEmailer_SendFails_NoError(t *testing.T) {
	sender := &failSender{err: errors.New("smtp failed")}
	profiles := &fakeProfileGetter{profiles: map[string]*domainuser.Profile{
		"prof-1": {ID: "prof-1", UserID: "usr-1"},
	}}
	users := &fakeUserGetter{users: map[string]*domainuser.User{
		"usr-1": {ID: "usr-1", Email: "user@example.com"},
	}}

	bus := &fakeBus{}
	emailer := NewNotificationEmailer(sender, profiles, users)
	emailer.Register(bus)

	ev := domainnotif.NotificationCreated{ProfileID: "prof-1", Title: "Fail", OccurredAt: time.Now()}
	_ = bus.Publish(context.Background(), ev)
	// should not panic, just log
}

func TestSMTPSend_SSLMode_DialError(t *testing.T) {
	s := NewSMTPSender("127.0.0.1", 1, "", "", "from@test.com", "ssl")
	err := s.Send([]string{"to@test.com"}, "subj", "body")
	if err == nil {
		t.Fatal("expected error for ssl dial to invalid addr")
	}
}

func TestSMTPSend_StarttlsMode_DialError(t *testing.T) {
	s := NewSMTPSender("127.0.0.1", 1, "", "", "from@test.com", "starttls")
	err := s.Send([]string{"to@test.com"}, "subj", "body")
	if err == nil {
		t.Fatal("expected error for starttls dial to invalid addr")
	}
}

func TestSMTPSend_PlainMode_DialError(t *testing.T) {
	s := NewSMTPSender("127.0.0.1", 1, "", "", "from@test.com", "none")
	err := s.Send([]string{"to@test.com"}, "subj", "body")
	if err == nil {
		t.Fatal("expected error for plain dial to invalid addr")
	}
}

func TestSMTPSend_WithAuth(t *testing.T) {
	s := NewSMTPSender("127.0.0.1", 1, "user", "pass", "from@test.com", "ssl")
	err := s.Send([]string{"to@test.com"}, "subj", "body")
	if err == nil {
		t.Fatal("expected error for ssl dial with auth to invalid addr")
	}
}

func TestNotificationEmailer_ProfileNotFound_NoSend(t *testing.T) {
	sender := &fakeSender{}
	profiles := &fakeProfileGetter{profiles: map[string]*domainuser.Profile{}}
	users := &fakeUserGetter{users: map[string]*domainuser.User{}}

	bus := &fakeBus{}
	emailer := NewNotificationEmailer(sender, profiles, users)
	emailer.Register(bus)

	ev := domainnotif.NotificationCreated{ProfileID: "missing", Title: "X", OccurredAt: time.Now()}
	_ = bus.Publish(context.Background(), ev)

	if len(sender.sent) != 0 {
		t.Fatalf("expected 0 emails, got %d", len(sender.sent))
	}
}
