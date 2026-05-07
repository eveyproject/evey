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
