package grpcapi

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/user"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
)

type stubHours struct {
	acc user.ProfileAccess
	loc *time.Location
}

func (s stubHours) Access(context.Context, string) (user.ProfileAccess, error) { return s.acc, nil }
func (s stubHours) Location() *time.Location                                   { return s.loc }

// Ten minutes before a kids profile closes, its player is told once - not on
// every progress tick - with the closing time in the server's zone.
func TestClosingSoonMessage_OncePerSessionAndClosing(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Paris")
	closes := time.Date(2026, 10, 5, 22, 0, 0, 0, loc)
	hours := stubHours{acc: user.ProfileAccess{Open: true, ClosesAt: closes}, loc: loc}
	var warned closingWarnings
	ctx := context.Background()

	if msg := closingSoonMessage(ctx, hours, &warned, nil, "kid", "s1", closes.Add(-11*time.Minute)); msg != "" {
		t.Errorf("11 minutes before: %q, want nothing yet", msg)
	}
	msg := closingSoonMessage(ctx, hours, &warned, nil, "kid", "s1", closes.Add(-9*time.Minute))
	if !strings.HasPrefix(msg, "message:") {
		t.Fatalf("9 minutes before: %q, want a message command", msg)
	}
	if again := closingSoonMessage(ctx, hours, &warned, nil, "kid", "s1", closes.Add(-6*time.Minute)); again != "" {
		t.Errorf("next tick: %q, want the warning only once", again)
	}
	if other := closingSoonMessage(ctx, hours, &warned, nil, "kid", "s2", closes.Add(-6*time.Minute)); other == "" {
		t.Error("another session of the profile is warned too")
	}
	adult := stubHours{acc: user.ProfileAccess{Open: true}, loc: loc}
	if msg := closingSoonMessage(ctx, adult, &warned, nil, "adult", "s3", closes.Add(-5*time.Minute)); msg != "" {
		t.Errorf("no closing (adult / overridden): %q, want nothing", msg)
	}
}

// The warning names a real catalogue entry: a wrong key reaches the child's
// screen as "auth.profile.closingSoon" and no guard catches it (only errors.*
// keys are scanned).
func TestClosingSoonMessage_ResolvesInTheCatalogue(t *testing.T) {
	tr, err := eveyi18n.NewFromDir("../../../../i18n")
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Europe/Paris")
	closes := time.Date(2026, 10, 5, 22, 0, 0, 0, loc)
	hours := stubHours{acc: user.ProfileAccess{Open: true, ClosesAt: closes}, loc: loc}
	var warned closingWarnings
	ctx := eveyi18n.ContextWithLocale(context.Background(), "fr")
	msg := closingSoonMessage(ctx, hours, &warned, tr, "kid", "s1", closes.Add(-9*time.Minute))
	if msg != "message:Ce profil se ferme à 22:00 (encore 9 min)." {
		t.Errorf("warning = %q, want the French catalogue text with time and minutes", msg)
	}
}
