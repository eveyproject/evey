package library

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The bound is max(floor, 12% of the runtime), expressed in LEAD (distance to
// the end of the file) - never in position percentage, which is meaningless
// here since the credits scan only ever looks at the last 20% of the file.
func TestCreditsLeadImplausible(t *testing.T) {
	t.Parallel()
	const episodeMs = 42 * 60 * 1000 // 12% = 302_400, floor wins below that
	const filmMs = 130 * 60 * 1000   // 12% = 936_000, fraction wins
	const shortEpMs = 22 * 60 * 1000 // 12% = 158_400, floor wins
	cases := []struct {
		name              string
		leadMs, runtimeMs int64
		floorMs           int64
		want              bool
	}{
		{"episode credits 90s before the end", 90_000, episodeMs, SuspiciousCreditsLeadMs, false},
		{"episode credits at the 600s scan bound", 600_000, episodeMs, SuspiciousCreditsLeadMs, true},
		{"episode just under the 12% relaxation", 302_000, episodeMs, SuspiciousCreditsLeadMs, false},
		{"short episode keeps the 4-min floor", 200_000, shortEpMs, SuspiciousCreditsLeadMs, false},
		{"short episode past the floor", 250_000, shortEpMs, SuspiciousCreditsLeadMs, true},
		{"film rolling 12 minutes of credits", 720_000, filmMs, SuspiciousCreditsLeadMs, false},
		{"film credits 20 minutes early", 1_200_000, filmMs, SuspiciousCreditsLeadMs, true},
		{"zero floor disables the bound", 600_000, episodeMs, 0, false},
		{"unknown runtime cannot judge", 600_000, 0, SuspiciousCreditsLeadMs, false},
		{"raised floor accepts more", 600_000, episodeMs, 10 * 60 * 1000, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, CreditsLeadImplausible(tc.leadMs, tc.runtimeMs, tc.floorMs))
		})
	}
}
