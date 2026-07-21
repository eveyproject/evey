package library

import (
	"testing"
	"time"
)

// The periodic scan walks every library root - one getdents per directory - on
// the SAME disk the viewer is streaming from. It ran every 2 min regardless of
// playback, which is the disk-contention class this server keeps hitting on
// weak NAS hardware. It now backs off while anything is playing.
func TestShouldScanNow(t *testing.T) {
	base := time.Unix(1000, 0)
	playing := func(n int) func() int { return func() int { return n } }
	interval := 2 * time.Minute

	cases := []struct {
		name     string
		active   func() int
		lastScan time.Time
		now      time.Time
		interval time.Duration
		want     bool
	}{
		{"idle keeps the normal cadence", playing(0), base, base.Add(2 * time.Minute), interval, true},
		{"unwired gate keeps the old behaviour", nil, base, base.Add(2 * time.Minute), interval, true},
		{"playing, inside the backoff window", playing(1), base, base.Add(2 * time.Minute), interval, false},
		{"playing, past the backoff window", playing(1), base, base.Add(16 * time.Minute), interval, true},
		{"playing, first ever tick still scans", playing(1), time.Time{}, base, interval, true},
		// An operator who asks for a SLOWER cadence than the backoff must get
		// theirs - the guard may only ever reduce scanning, never increase it.
		{"operator interval wider than the backoff wins", playing(1), base, base.Add(16 * time.Minute), time.Hour, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &ScanService{ingestActivePlayback: tc.active}
			if got := s.shouldScanNow(tc.now, tc.lastScan, tc.interval); got != tc.want {
				t.Fatalf("shouldScanNow = %v, want %v", got, tc.want)
			}
		})
	}
}
