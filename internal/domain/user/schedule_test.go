package user

import (
	"errors"
	"testing"
	"time"
)

func paris(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func at(loc *time.Location, y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, loc)
}

// 2026-10-05 is a Monday, 2026-10-10 a Saturday.
func TestDefaultKidsSchedule_WeekdayEveningAndWeekendTwoRanges(t *testing.T) {
	loc := paris(t)
	s := DefaultKidsSchedule()
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		at   time.Time
		open bool
	}{
		{"monday 17:59", at(loc, 2026, 10, 5, 17, 59), false},
		{"monday 18:00", at(loc, 2026, 10, 5, 18, 0), true},
		{"monday 21:59", at(loc, 2026, 10, 5, 21, 59), true},
		{"monday 22:00", at(loc, 2026, 10, 5, 22, 0), false},
		{"saturday 10:00", at(loc, 2026, 10, 10, 10, 0), true},
		{"saturday 13:00, lunch break", at(loc, 2026, 10, 10, 13, 0), false},
		{"saturday 20:59", at(loc, 2026, 10, 10, 20, 59), true},
		{"sunday 21:30", at(loc, 2026, 10, 11, 21, 30), false},
	}
	for _, c := range cases {
		if open, _, _ := s.Window(c.at); open != c.open {
			t.Errorf("%s: open=%v, want %v", c.name, open, c.open)
		}
	}
	_, closes, _ := s.Window(at(loc, 2026, 10, 5, 19, 0))
	if !closes.Equal(at(loc, 2026, 10, 5, 22, 0)) {
		t.Errorf("monday evening closes at %v, want 22:00", closes)
	}
	_, _, next := s.Window(at(loc, 2026, 10, 10, 12, 30))
	if !next.Equal(at(loc, 2026, 10, 10, 14, 0)) {
		t.Errorf("saturday lunch reopens at %v, want 14:00", next)
	}
	_, _, next = s.Window(at(loc, 2026, 10, 9, 23, 0))
	if !next.Equal(at(loc, 2026, 10, 10, 9, 0)) {
		t.Errorf("friday night reopens at %v, want saturday 9:00", next)
	}
}

// A range past midnight runs into the next day, and an evening range that
// meets the next morning's is one window: no closing warning at midnight.
func TestSchedule_PastMidnightAndMergedWindows(t *testing.T) {
	loc := paris(t)
	s := AccessSchedule{Enabled: true, Mode: ScheduleModeDaily, Rules: []ScheduleRule{
		{Days: []time.Weekday{time.Friday}, Ranges: []TimeRange{{Start: 20 * 60, End: 30}}},
		{Days: []time.Weekday{time.Saturday}, Ranges: []TimeRange{{Start: 0, End: 60}}},
	}}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	open, closes, _ := s.Window(at(loc, 2026, 10, 9, 23, 0))
	if !open || !closes.Equal(at(loc, 2026, 10, 10, 1, 0)) {
		t.Errorf("friday 23:00: open=%v closes=%v, want open until saturday 1:00", open, closes)
	}
	if open, _, _ := s.Window(at(loc, 2026, 10, 10, 0, 15)); !open {
		t.Error("saturday 0:15 must still be the friday evening window")
	}
}

// Wall-clock times hold across a daylight-saving change: 2026-10-25 (Paris
// goes back an hour at 3:00), a Sunday.
func TestSchedule_KeepsWallClockAcrossDST(t *testing.T) {
	loc := paris(t)
	s := DefaultKidsSchedule()
	_, _, next := s.Window(at(loc, 2026, 10, 25, 7, 0))
	if h, m := next.Hour(), next.Minute(); h != 9 || m != 0 {
		t.Errorf("DST sunday opens at %02d:%02d, want 09:00", h, m)
	}
	if _, closes, _ := s.Window(at(loc, 2026, 10, 25, 15, 0)); closes.Hour() != 21 {
		t.Errorf("DST sunday afternoon closes at %v, want 21:00", closes)
	}
}

func TestSchedule_EnabledWithoutRulesIsAlwaysClosed(t *testing.T) {
	loc := paris(t)
	s := AccessSchedule{Enabled: true, Mode: ScheduleModeWeek}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	open, _, next := s.Window(at(loc, 2026, 10, 5, 19, 0))
	if open || !next.IsZero() {
		t.Errorf("no rule: open=%v next=%v, want closed with no opening", open, next)
	}
}

func TestSchedule_ValidateRejects(t *testing.T) {
	bad := []AccessSchedule{
		{Enabled: true, Mode: "monthly"},
		{Enabled: true, Mode: ScheduleModeWeek, Rules: []ScheduleRule{{Ranges: []TimeRange{{Start: 60, End: 120}}}}},
		{Enabled: true, Mode: ScheduleModeWeek, Rules: []ScheduleRule{{Days: []time.Weekday{time.Monday}}}},
		{Enabled: true, Mode: ScheduleModeWeek, Rules: []ScheduleRule{{Days: []time.Weekday{7}, Ranges: []TimeRange{{Start: 60, End: 120}}}}},
		{Enabled: true, Mode: ScheduleModeWeek, Rules: []ScheduleRule{{Days: []time.Weekday{time.Monday}, Ranges: []TimeRange{{Start: 60, End: 60}}}}},
		{Enabled: true, Mode: ScheduleModeWeek, Rules: []ScheduleRule{{Days: []time.Weekday{time.Monday}, Ranges: []TimeRange{{Start: 1440, End: 60}}}}},
	}
	for i, s := range bad {
		if err := s.Validate(); !errors.Is(err, ErrInvalidSchedule) {
			t.Errorf("case %d: err=%v, want ErrInvalidSchedule", i, err)
		}
	}
}

// Only a kids profile with an enabled schedule is ever closed; a parent's PIN
// keeps it open until the next opening, where the schedule takes over.
func TestProfileAccessAt_KidsOnlyAndPINOverride(t *testing.T) {
	loc := paris(t)
	s := DefaultKidsSchedule()
	monday8am := at(loc, 2026, 10, 5, 8, 0)

	adult := &Profile{Schedule: &s}
	if !adult.AccessAt(monday8am, loc).Open {
		t.Error("an adult profile is never closed by a schedule")
	}
	kid := &Profile{IsKids: true, Schedule: &s}
	acc := kid.AccessAt(monday8am, loc)
	if acc.Open || !acc.NextOpen.Equal(at(loc, 2026, 10, 5, 18, 0)) {
		t.Errorf("kids monday 8:00: %+v, want closed until 18:00", acc)
	}

	until := kid.OverrideUntil(monday8am, loc)
	if !until.Equal(at(loc, 2026, 10, 5, 18, 0)) {
		t.Errorf("override until %v, want the next opening 18:00", until)
	}
	kid.ScheduleOverrideUntil = &until
	if acc := kid.AccessAt(monday8am.Add(time.Hour), loc); !acc.Open || !acc.Overridden {
		t.Errorf("after the PIN: %+v, want open and overridden", acc)
	}
	if acc := kid.AccessAt(at(loc, 2026, 10, 5, 22, 30), loc); acc.Open {
		t.Error("the override ends at the next opening: closed again after 22:00")
	}

	off := s
	off.Enabled = false
	kid.Schedule = &off
	kid.ScheduleOverrideUntil = nil
	if !kid.AccessAt(monday8am, loc).Open {
		t.Error("a disabled schedule never closes the profile")
	}
}
