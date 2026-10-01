package user

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/user"
	ufakes "github.com/evey-project/evey/internal/domain/user/fakes"
)

type fixedClock struct{ loc *time.Location }

func (c fixedClock) Location() *time.Location { return c.loc }

func scheduleFixture(t *testing.T) (*ProfileSchedules, *ufakes.FakeProfileRepository, *time.Time, *time.Location) {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	repo := ufakes.NewFakeProfileRepository()
	ctx := context.Background()
	sched := user.DefaultKidsSchedule()
	kid := &user.Profile{ID: "kid", UserID: "u1", Name: "Kid", IsKids: true, Schedule: &sched}
	parent := &user.Profile{ID: "parent", UserID: "u1", Name: "Parent", Parental: &user.ParentalSettings{PINHash: "hashed:1234"}}
	kidPIN := &user.Profile{ID: "kid2", UserID: "u1", Name: "Kid 2", IsKids: true, Parental: &user.ParentalSettings{PINHash: "hashed:0000"}}
	for _, p := range []*user.Profile{kid, parent, kidPIN} {
		if err := repo.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 10, 5, 17, 55, 0, 0, loc) // a Monday, 5 min before opening
	svc := NewProfileSchedules(repo, ufakes.NewFakePasswordHasher(), fixedClock{loc}).WithNow(func() time.Time { return now })
	return svc, repo, &now, loc
}

// The answer is recomputed on every call: a cached profile still opens and
// closes on the minute.
func TestProfileSchedules_AccessFollowsTheClockThroughTheCache(t *testing.T) {
	svc, _, now, loc := scheduleFixture(t)
	ctx := context.Background()
	acc, err := svc.Access(ctx, "kid")
	if err != nil || acc.Open {
		t.Fatalf("17:55: %+v %v, want closed", acc, err)
	}
	*now = time.Date(2026, 10, 5, 18, 0, 0, 0, loc)
	if acc, _ := svc.Access(ctx, "kid"); !acc.Open {
		t.Error("18:00 from the cached profile: want open")
	}
	if acc, _ := svc.Access(ctx, ""); !acc.Open {
		t.Error("no profile (service token) is never closed")
	}
	if _, err := svc.Access(ctx, "ghost"); err == nil {
		t.Error("an unreadable profile must be an error the caller refuses on")
	}
}

// Only an ADULT profile's PIN unlocks, until the next opening.
func TestProfileSchedules_UnlockWithAParentPINOnly(t *testing.T) {
	svc, repo, now, loc := scheduleFixture(t)
	ctx := context.Background()
	kid, _ := repo.GetByID(ctx, "kid")

	if _, err := svc.Unlock(ctx, kid, "0000"); !errors.Is(err, ErrWrongParentPIN) {
		t.Errorf("another kid's PIN: %v, want ErrWrongParentPIN", err)
	}
	until, err := svc.Unlock(ctx, kid, "1234")
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 10, 5, 18, 0, 0, 0, loc); !until.Equal(want) {
		t.Errorf("unlocked until %v, want the next opening %v", until, want)
	}
	if acc, _ := svc.Access(ctx, "kid"); !acc.Open || !acc.Overridden {
		t.Errorf("after the PIN: %+v, want open (overridden), cache invalidated", acc)
	}

	*now = time.Date(2026, 10, 5, 22, 5, 0, 0, loc)
	if acc, _ := svc.Access(ctx, "kid"); acc.Open {
		t.Error("the PIN lasted past the evening window: want closed again at 22:05")
	}
}

func TestProfileSchedules_UnlockWithoutAnyParentPIN(t *testing.T) {
	svc, repo, _, _ := scheduleFixture(t)
	ctx := context.Background()
	parent, _ := repo.GetByID(ctx, "parent")
	if err := repo.UpdateParentalSettings(ctx, parent.ID, nil); err != nil {
		t.Fatal(err)
	}
	kid, _ := repo.GetByID(ctx, "kid")
	if _, err := svc.Unlock(ctx, kid, "1234"); !errors.Is(err, ErrNoParentPIN) {
		t.Errorf("no adult PIN: %v, want ErrNoParentPIN", err)
	}
}

// New hours apply at once: the cache is dropped and a PIN override with it.
func TestProfileSchedules_SetScheduleAppliesNowAndKidsOnly(t *testing.T) {
	svc, repo, _, _ := scheduleFixture(t)
	ctx := context.Background()
	kid, _ := repo.GetByID(ctx, "kid")
	if _, err := svc.Unlock(ctx, kid, "1234"); err != nil {
		t.Fatal(err)
	}
	early := user.AccessSchedule{Enabled: true, Mode: user.ScheduleModeDaily, Rules: []user.ScheduleRule{
		{Days: []time.Weekday{time.Monday}, Ranges: []user.TimeRange{{Start: 17 * 60, End: 18 * 60}}},
	}}
	if _, err := svc.Access(ctx, "kid"); err != nil { // warm the cache
		t.Fatal(err)
	}
	if err := svc.SetSchedule(ctx, kid, &early); err != nil {
		t.Fatal(err)
	}
	acc, _ := svc.Access(ctx, "kid")
	if !acc.Open || acc.Overridden {
		t.Errorf("17:55 under the new 17-18 hours: %+v, want open by schedule, override dropped", acc)
	}
	stored, _ := repo.GetByID(ctx, "kid")
	if stored.ScheduleOverrideUntil != nil {
		t.Error("the PIN override must be cleared when the hours change")
	}

	parent, _ := repo.GetByID(ctx, "parent")
	if err := svc.SetSchedule(ctx, parent, &early); !errors.Is(err, ErrScheduleNotKids) {
		t.Errorf("adult profile: %v, want ErrScheduleNotKids", err)
	}
	bad := user.AccessSchedule{Enabled: true, Mode: "monthly"}
	if err := svc.SetSchedule(ctx, kid, &bad); !errors.Is(err, user.ErrInvalidSchedule) {
		t.Errorf("invalid schedule: %v, want ErrInvalidSchedule", err)
	}
}
