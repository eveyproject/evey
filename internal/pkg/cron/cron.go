// Package cron provides a minimal 5-field cron expression parser used by
// both the library scan scheduler and the backup scheduler.
package cron

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// NextTime computes the next run time from a standard 5-field cron expression.
// Supports: minute, hour, day-of-month, month, day-of-week.
// Fields accept: exact values, *, */N, comma-separated values, and ranges (lo-hi).
func NextTime(expr string, from time.Time) (time.Time, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("cron: expected 5 fields, got %d", len(fields))
	}
	minuteSet, err := parseField(fields[0], 0, 59)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron minute: %w", err)
	}
	hourSet, err := parseField(fields[1], 0, 23)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron hour: %w", err)
	}
	domSet, err := parseField(fields[2], 1, 31)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron day-of-month: %w", err)
	}
	monthSet, err := parseField(fields[3], 1, 12)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron month: %w", err)
	}
	dowSet, err := parseField(fields[4], 0, 6)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron day-of-week: %w", err)
	}

	t := from.Truncate(time.Minute).Add(time.Minute)
	limit := from.Add(2 * 366 * 24 * time.Hour)
	for t.Before(limit) {
		if !monthSet[int(t.Month())] {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !domSet[t.Day()] || !dowSet[int(t.Weekday())] {
			t = t.Add(24 * time.Hour)
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
			continue
		}
		if !hourSet[t.Hour()] {
			t = t.Add(time.Hour)
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
			continue
		}
		if !minuteSet[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}
		return t, nil
	}
	return time.Time{}, errors.New("cron: no next run found within 2 years")
}

func parseField(field string, min, max int) (map[int]bool, error) {
	result := make(map[int]bool)
	parts := strings.SplitSeq(field, ",")
	for part := range parts {
		part = strings.TrimSpace(part)
		if part == "*" {
			for i := min; i <= max; i++ {
				result[i] = true
			}
			continue
		}
		if strings.HasPrefix(part, "*/") {
			step, err := strconv.Atoi(part[2:])
			if err != nil || step <= 0 {
				return nil, fmt.Errorf("invalid step %q", part)
			}
			for i := min; i <= max; i += step {
				result[i] = true
			}
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, err1 := strconv.Atoi(bounds[0])
			hi, err2 := strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil || lo < min || hi > max || lo > hi {
				return nil, fmt.Errorf("invalid range %q", part)
			}
			for i := lo; i <= hi; i++ {
				result[i] = true
			}
			continue
		}
		val, err := strconv.Atoi(part)
		if err != nil || val < min || val > max {
			return nil, fmt.Errorf("invalid value %q", part)
		}
		result[val] = true
	}
	if len(result) == 0 {
		return nil, errors.New("empty field")
	}
	return result, nil
}
