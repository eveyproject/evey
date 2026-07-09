package transcoding

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoSessionLimiter_UnlimitedByDefault(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0)
	for range 50 {
		require.True(t, l.TryAcquire())
	}
	assert.Equal(t, 50, l.Active())
	assert.Equal(t, 0, l.Limit())
}

func TestVideoSessionLimiter_AcquireUntilLimitThenRefuse(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(2)
	require.True(t, l.TryAcquire())
	require.True(t, l.TryAcquire())
	assert.False(t, l.TryAcquire(), "third acquire must be refused at limit=2")
	assert.Equal(t, 2, l.Active())
}

func TestVideoSessionLimiter_ReleaseFreesASlot(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(1)
	require.True(t, l.TryAcquire())
	require.False(t, l.TryAcquire())
	l.Release()
	assert.Equal(t, 0, l.Active())
	require.True(t, l.TryAcquire())
}

func TestVideoSessionLimiter_DoubleReleaseNeverGoesNegative(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(1)
	require.True(t, l.TryAcquire())
	l.Release()
	l.Release() // stray extra release
	assert.Equal(t, 0, l.Active())
	// still usable, not corrupted into a negative-active state that would
	// let unbounded acquires through.
	require.True(t, l.TryAcquire())
	assert.Equal(t, 1, l.Active())
}

func TestVideoSessionLimiter_SetLimitAppliesLiveWithoutKillingActive(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(5)
	require.True(t, l.TryAcquire())
	require.True(t, l.TryAcquire())
	assert.Equal(t, 2, l.Active())

	l.SetLimit(2)
	assert.Equal(t, 2, l.Active(), "SetLimit never kills already-acquired slots")
	assert.False(t, l.TryAcquire(), "at the new lower limit, the next acquire is refused")

	l.Release()
	require.True(t, l.TryAcquire(), "a freed slot is usable again under the new limit")
}

func TestVideoSessionLimiter_SetLimitNegativeClampsToUnlimited(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(1)
	l.SetLimit(-3)
	assert.Equal(t, 0, l.Limit())
	for range 10 {
		require.True(t, l.TryAcquire())
	}
}

func TestVideoSessionLimiter_ConcurrentAcquireRelease_Race(t *testing.T) {
	l := NewVideoSessionLimiter(4)
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			if l.TryAcquire() {
				l.Release()
			}
		})
	}
	wg.Wait()
	assert.Equal(t, 0, l.Active())
}
