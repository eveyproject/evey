package interceptors

import (
	"testing"
	"time"
)

func TestShouldWriteLastUsed(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	const window = 5 * time.Minute
	recent := now.Add(-1 * time.Minute)
	old := now.Add(-10 * time.Minute)
	atWindow := now.Add(-window)

	cases := []struct {
		name string
		last *time.Time
		want bool
	}{
		{"first use (nil) writes", nil, true},
		{"a recent write is skipped", &recent, false},
		{"a write older than the window is refreshed", &old, true},
		{"exactly at the window boundary writes", &atWindow, true},
	}
	for _, c := range cases {
		if got := shouldWriteLastUsed(c.last, now, window); got != c.want {
			t.Errorf("%s: shouldWriteLastUsed = %v, want %v", c.name, got, c.want)
		}
	}
}
