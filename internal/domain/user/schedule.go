package user

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"
)

// AccessSchedule is when a kids profile may be used: a list of rules, each a
// set of weekdays and one or more time ranges on them. The profile is open
// while ANY rule allows it. Enforced by the server in its own time zone, so
// changing the clock of the child's tablet changes nothing.
type AccessSchedule struct {
	Enabled bool
	// Mode only remembers how the editor showed the rules (weekdays/weekend
	// blocks or one block per day); the rules alone decide.
	Mode  ScheduleMode
	Rules []ScheduleRule
}

type ScheduleMode string

const (
	ScheduleModeWeek  ScheduleMode = "week"
	ScheduleModeDaily ScheduleMode = "daily"
)

type ScheduleRule struct {
	Days   []time.Weekday
	Ranges []TimeRange
}

// TimeRange is a span of the day in minutes since midnight. An End at or
// before Start runs past midnight into the next day (20:00-00:30).
type TimeRange struct {
	Start int
	End   int
}

const (
	minutesPerDay        = 24 * 60
	maxScheduleRules     = 50
	maxRangesPerRule     = 12
	scheduleLookaheadDay = 8
)

var ErrInvalidSchedule = errors.New("invalid access schedule")

// DefaultKidsSchedule is what the editor proposes: weekdays 18:00-22:00,
// weekend 9:00-12:00 and 14:00-21:00 (user, 2026-10-01).
func DefaultKidsSchedule() AccessSchedule {
	return AccessSchedule{
		Enabled: true,
		Mode:    ScheduleModeWeek,
		Rules: []ScheduleRule{
			{
				Days:   []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday},
				Ranges: []TimeRange{{Start: 18 * 60, End: 22 * 60}},
			},
			{
				Days:   []time.Weekday{time.Saturday, time.Sunday},
				Ranges: []TimeRange{{Start: 9 * 60, End: 12 * 60}, {Start: 14 * 60, End: 21 * 60}},
			},
		},
	}
}

// Validate rejects what the server cannot honour. An enabled schedule with no
// rule is valid and means "always closed": the editor says so, the server
// does not guess an opening.
func (s AccessSchedule) Validate() error {
	if s.Mode != ScheduleModeWeek && s.Mode != ScheduleModeDaily {
		return fmt.Errorf("%w: mode %q", ErrInvalidSchedule, s.Mode)
	}
	if len(s.Rules) > maxScheduleRules {
		return fmt.Errorf("%w: %d rules, at most %d", ErrInvalidSchedule, len(s.Rules), maxScheduleRules)
	}
	for i, r := range s.Rules {
		if len(r.Days) == 0 {
			return fmt.Errorf("%w: rule %d has no day", ErrInvalidSchedule, i+1)
		}
		for _, d := range r.Days {
			if d < time.Sunday || d > time.Saturday {
				return fmt.Errorf("%w: rule %d day %d", ErrInvalidSchedule, i+1, d)
			}
		}
		if len(r.Ranges) == 0 || len(r.Ranges) > maxRangesPerRule {
			return fmt.Errorf("%w: rule %d needs 1 to %d time ranges", ErrInvalidSchedule, i+1, maxRangesPerRule)
		}
		for _, tr := range r.Ranges {
			if tr.Start < 0 || tr.Start >= minutesPerDay || tr.End < 0 || tr.End > minutesPerDay {
				return fmt.Errorf("%w: rule %d range %d-%d out of the day", ErrInvalidSchedule, i+1, tr.Start, tr.End)
			}
			if tr.Start == tr.End {
				return fmt.Errorf("%w: rule %d range starts and ends at the same time", ErrInvalidSchedule, i+1)
			}
		}
	}
	return nil
}

// interval is one concrete opening, [from, to).
type interval struct{ from, to time.Time }

// windows lists the merged openings from the day before t to a week after, in
// t's location. Built from calendar dates, not by adding minutes, so a range
// keeps its wall-clock times across a daylight-saving change, and an evening
// range that runs into the next day's morning range is one window.
func (s AccessSchedule) windows(t time.Time) []interval {
	loc := t.Location()
	y, m, d := t.Date()
	var out []interval
	for off := -1; off <= scheduleLookaheadDay; off++ {
		day := time.Date(y, m, d+off, 0, 0, 0, 0, loc)
		for _, r := range s.Rules {
			if !slices.Contains(r.Days, day.Weekday()) {
				continue
			}
			for _, tr := range r.Ranges {
				from := time.Date(day.Year(), day.Month(), day.Day(), 0, tr.Start, 0, 0, loc)
				endDay := day.Day()
				if tr.End <= tr.Start {
					endDay++
				}
				to := time.Date(day.Year(), day.Month(), endDay, 0, tr.End, 0, 0, loc)
				out = append(out, interval{from, to})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].from.Before(out[j].from) })
	merged := out[:0]
	for _, iv := range out {
		if n := len(merged); n > 0 && !iv.from.After(merged[n-1].to) {
			if iv.to.After(merged[n-1].to) {
				merged[n-1].to = iv.to
			}
			continue
		}
		merged = append(merged, iv)
	}
	return merged
}

// Window reports whether t falls in an opening and, if so, when that opening
// ends; if not, when the next one starts. A zero time means "none within a
// week" (an enabled schedule with no rule).
func (s AccessSchedule) Window(t time.Time) (open bool, closesAt, nextOpen time.Time) {
	if !s.Enabled {
		return true, time.Time{}, time.Time{}
	}
	for _, iv := range s.windows(t) {
		if !t.Before(iv.from) && t.Before(iv.to) {
			return true, iv.to, time.Time{}
		}
		if iv.from.After(t) {
			return false, time.Time{}, iv.from
		}
	}
	return false, time.Time{}, time.Time{}
}

// ProfileAccess is whether a profile may be used at a given time.
type ProfileAccess struct {
	Open bool
	// ClosesAt is when the current opening ends (zero when there is no
	// schedule or a parent lifted it).
	ClosesAt time.Time
	// NextOpen is when a closed profile opens again (zero: not within a week).
	NextOpen time.Time
	// Overridden is set while a parent's PIN keeps a closed profile open.
	Overridden bool
}

// ScheduleApplies reports whether the profile has opening hours to enforce:
// a kids profile with an enabled schedule.
func (p *Profile) ScheduleApplies() bool {
	return p != nil && p.IsKids && p.Schedule != nil && p.Schedule.Enabled
}

// AccessAt says whether the profile may be used at now, read in loc (the
// server's time zone).
func (p *Profile) AccessAt(now time.Time, loc *time.Location) ProfileAccess {
	if !p.ScheduleApplies() {
		return ProfileAccess{Open: true}
	}
	t := now.In(loc)
	open, closesAt, nextOpen := p.Schedule.Window(t)
	if open {
		return ProfileAccess{Open: true, ClosesAt: closesAt}
	}
	if p.ScheduleOverrideUntil != nil && now.Before(*p.ScheduleOverrideUntil) {
		return ProfileAccess{Open: true, Overridden: true, NextOpen: nextOpen}
	}
	return ProfileAccess{Open: false, NextOpen: nextOpen}
}

// OverrideUntil is how long a parent's PIN keeps a closed profile open: until
// the next opening starts, where the schedule takes over (user, 2026-10-01).
// With no opening within a week, until the end of the current day.
func (p *Profile) OverrideUntil(now time.Time, loc *time.Location) time.Time {
	t := now.In(loc)
	if p.Schedule != nil {
		if _, _, next := p.Schedule.Window(t); !next.IsZero() {
			return next
		}
	}
	y, m, d := t.Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, loc)
}
