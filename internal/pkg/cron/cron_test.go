package cron_test

import (
	"testing"
	"time"

	"github.com/evey-project/evey/internal/pkg/cron"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNextTime_EveryHour(t *testing.T) {
	from := time.Date(2026, 3, 10, 14, 30, 0, 0, time.UTC)
	next, err := cron.NextTime("0 * * * *", from)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 3, 10, 15, 0, 0, 0, time.UTC), next)
}

func TestNextTime_Every6Hours(t *testing.T) {
	from := time.Date(2026, 3, 10, 14, 0, 0, 0, time.UTC)
	next, err := cron.NextTime("0 */6 * * *", from)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 3, 10, 18, 0, 0, 0, time.UTC), next)
}

func TestNextTime_Daily(t *testing.T) {
	from := time.Date(2026, 3, 10, 3, 30, 0, 0, time.UTC)
	next, err := cron.NextTime("0 3 * * *", from)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 3, 11, 3, 0, 0, 0, time.UTC), next)
}

func TestNextTime_InvalidExpr(t *testing.T) {
	_, err := cron.NextTime("bad", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected 5 fields")
}

func TestNextTime_InvalidStep(t *testing.T) {
	_, err := cron.NextTime("*/0 * * * *", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid step")
}

func TestNextTime_InvalidRange(t *testing.T) {
	_, err := cron.NextTime("* * 31-1 * *", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid range")
}

func TestNextTime_OutOfRangeValue(t *testing.T) {
	_, err := cron.NextTime("60 * * * *", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid value")
}

func TestNextTime_CommaValues(t *testing.T) {
	from := time.Date(2026, 3, 10, 14, 0, 0, 0, time.UTC)
	next, err := cron.NextTime("0,30 * * * *", from)
	require.NoError(t, err)
	assert.Equal(t, 30, next.Minute())
	assert.Equal(t, 14, next.Hour())
}

func TestNextTime_EveryMinute(t *testing.T) {
	from := time.Date(2026, 3, 10, 14, 30, 45, 0, time.UTC)
	next, err := cron.NextTime("* * * * *", from)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 3, 10, 14, 31, 0, 0, time.UTC), next)
}

func TestNextTime_Weekly(t *testing.T) {
	from := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	next, err := cron.NextTime("0 0 * * 0", from)
	require.NoError(t, err)
	assert.Equal(t, 0, int(next.Weekday()))
}

func TestNextTime_TooFewFields(t *testing.T) {
	_, err := cron.NextTime("0 *", time.Now())
	require.Error(t, err)
}

func TestNextTime_TooManyFields(t *testing.T) {
	_, err := cron.NextTime("0 * * * * *", time.Now())
	require.Error(t, err)
}

// Field boundaries: a range includes both ends, a range or value past the
// field's limits is refused, and "*" and "*/n" reach the field's last value.
// Ranges had no working-case test at all and every boundary survived
// mutation (2026-09-28) - a backup schedule set to "22-23" would silently
// skip 23:00 if the upper end were exclusive.
func TestNextTime_FieldBoundaries(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 3, 10, h, m, 0, 0, time.UTC) }
	cases := []struct {
		name, expr string
		from, want time.Time
	}{
		{"a range includes its upper end", "0 22-23 * * *", at(22, 30), at(23, 0)},
		{"a range includes its lower end", "0 22-23 * * *", at(21, 30), at(22, 0)},
		{"the last minute and hour are accepted", "59 23 * * *", at(23, 58), at(23, 59)},
		{"* reaches the field's last value", "* * * * *", at(23, 58), at(23, 59)},
		{"a step reaches the field's last value", "*/59 * * * *", at(10, 30), at(10, 59)},
		{"a range may start at the field's first value", "0 0-1 * * *", at(0, 30), at(1, 0)},
		{"a one-value range is valid", "0 5-5 * * *", at(4, 0), at(5, 0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			next, err := cron.NextTime(c.expr, c.from)
			require.NoError(t, err)
			assert.Equal(t, c.want, next)
		})
	}
	for _, bad := range []string{"0 0-24 * * *", "0 * 0-5 * *", "0 * * 1-13 *", "0 * * * 0-7"} {
		_, err := cron.NextTime(bad, at(0, 0))
		assert.Error(t, err, "%q reaches past its field and must be refused", bad)
	}
}
