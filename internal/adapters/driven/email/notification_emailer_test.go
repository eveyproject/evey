package email

import (
	"context"
	"errors"
	"strings"
	"testing"

	domainnotif "github.com/eveyproject/evey/internal/domain/notification"
	"github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
)

func movieCreated(profileID, fanout string) domainnotif.NotificationCreated {
	return domainnotif.NotificationCreated{
		NotificationID: "n-" + profileID,
		ProfileID:      shared.ProfileID("prof-" + profileID),
		Type:           domainnotif.TypeNewMovie,
		Title:          "new_movie:Big Buck Bunny",
		FanoutID:       fanout,
		TitleKey:       "notifications.push.new_movie.title",
		TitleParams:    map[string]string{"label": "Big Buck Bunny"},
		TitleFallback:  "New Movie: Big Buck Bunny",
	}
}

// newLocalizedEmailer: account usr-1 (two profiles, fr + no locale) and usr-2
// (one profile, en), the real catalogue, server language fr.
func newLocalizedEmailer(t *testing.T, sender Sender, baseURL string) *fakeBus {
	t.Helper()
	tr, err := eveyi18n.NewFromDir("../../../../i18n")
	if err != nil {
		t.Fatal(err)
	}
	profiles := &fakeProfileGetter{profiles: map[string]*domainuser.Profile{
		"prof-a": {ID: "prof-a", UserID: "usr-1", Locale: "fr"},
		"prof-b": {ID: "prof-b", UserID: "usr-1", Locale: ""},
		"prof-c": {ID: "prof-c", UserID: "usr-2", Locale: "en"},
	}}
	users := &fakeUserGetter{users: map[string]*domainuser.User{
		"usr-1": {ID: "usr-1", Email: "famille@example.com"},
		"usr-2": {ID: "usr-2", Email: "solo@example.com"},
	}}
	bus := &fakeBus{}
	NewNotificationEmailer(sender, profiles, users, func() string { return "Salon" },
		WithNotificationLocale(tr, func() string { return "fr" }),
		WithNotificationBaseURL(func() string { return baseURL })).Register(bus)
	return bus
}

func TestNotificationEmailer_LocalizedSubjectAndBody(t *testing.T) {
	sender := &fakeSender{}
	bus := newLocalizedEmailer(t, sender, "https://evey.example.org/")
	_ = bus.Publish(context.Background(), movieCreated("a", "f1"))
	_ = bus.Publish(context.Background(), movieCreated("c", "f1"))

	if len(sender.sent) != 2 {
		t.Fatalf("expected 2 emails (one per account), got %d", len(sender.sent))
	}
	want := []struct{ to, subject, body string }{
		{"famille@example.com", "Nouveau film : Big Buck Bunny", "Nouveau film : Big Buck Bunny\n\nOuvrir Salon : https://evey.example.org\n"},
		{"solo@example.com", "New Movie: Big Buck Bunny", "New Movie: Big Buck Bunny\n\nOpen Salon: https://evey.example.org\n"},
	}
	for i, w := range want {
		got := sender.sent[i]
		if got.to[0] != w.to || got.subject != w.subject || got.body != w.body {
			t.Errorf("mail %d = %q / %q / %q, want %q / %q / %q", i, got.to[0], got.subject, got.body, w.to, w.subject, w.body)
		}
		if strings.Contains(got.subject+got.body, "new_movie:") {
			t.Errorf("mail %d leaks the storage prefix: %q", i, got.subject+got.body)
		}
	}
}

// A profile without a locale reads the server language; no public URL means
// no link, just the server name.
func TestNotificationEmailer_ServerLocaleAndNoURL(t *testing.T) {
	sender := &fakeSender{}
	bus := newLocalizedEmailer(t, sender, "")
	ev := movieCreated("b", "f1")
	ev.TitleKey, ev.TitleParams, ev.TitleFallback = "notifications.push.backup_failed.title", nil, "Scheduled Backup Failed"
	ev.Message = "disk full"
	_ = bus.Publish(context.Background(), ev)

	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 email, got %d", len(sender.sent))
	}
	if got, want := sender.sent[0].body, "Sauvegarde planifiée échouée\ndisk full\n\nEnvoyé par Salon.\n"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

// Two profiles of one account in one fan-out are one mail; a later fan-out
// (another title) mails again.
func TestNotificationEmailer_OneMailPerAccountPerFanout(t *testing.T) {
	sender := &fakeSender{}
	bus := newLocalizedEmailer(t, sender, "https://evey.example.org")
	_ = bus.Publish(context.Background(), movieCreated("a", "f1"))
	_ = bus.Publish(context.Background(), movieCreated("b", "f1"))
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 email for two profiles of one account, got %d", len(sender.sent))
	}
	_ = bus.Publish(context.Background(), movieCreated("b", "f2"))
	if len(sender.sent) != 2 {
		t.Fatalf("expected a second email for a new fan-out, got %d", len(sender.sent))
	}
}

type flakySender struct {
	fakeSender
	fail int
}

func (f *flakySender) Send(to []string, subject, body string) error {
	if f.fail > 0 {
		f.fail--
		return errors.New("smtp down")
	}
	return f.fakeSender.Send(to, subject, body)
}

// A failed send does not swallow the account's mail: the next profile retries.
func TestNotificationEmailer_FailedSendReleasesAccount(t *testing.T) {
	sender := &flakySender{fail: 1}
	bus := newLocalizedEmailer(t, sender, "")
	_ = bus.Publish(context.Background(), movieCreated("a", "f1"))
	_ = bus.Publish(context.Background(), movieCreated("b", "f1"))
	if len(sender.sent) != 1 {
		t.Fatalf("expected the second profile to retry the mail, got %d", len(sender.sent))
	}
}

// A publisher that predates the push title still never mails "new_movie:".
func TestNotificationEmailer_LegacyTitleStripsPrefix(t *testing.T) {
	sender := &fakeSender{}
	bus := newLocalizedEmailer(t, sender, "")
	_ = bus.Publish(context.Background(), domainnotif.NotificationCreated{
		ProfileID: "prof-c", Type: domainnotif.TypeNewMovie, Title: "new_movie:Big Buck Bunny",
	})
	if len(sender.sent) != 1 || sender.sent[0].subject != "Big Buck Bunny" {
		t.Fatalf("subject = %v, want Big Buck Bunny", sender.sent)
	}
}
