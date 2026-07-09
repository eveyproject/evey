package transcoding

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoSessionLimiter_UnlimitedByDefault(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0)
	for i := range 50 {
		require.True(t, l.TryAcquire(fmt.Sprintf("stream-%d", i)))
	}
	assert.Equal(t, 50, l.Active(), "unlimited admits every distinct stream")
	assert.Equal(t, 0, l.Limit())
}

func TestVideoSessionLimiter_AcquireUntilLimitThenRefuse(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(2)
	require.True(t, l.TryAcquire("stream-1"))
	require.True(t, l.TryAcquire("stream-2"))
	assert.False(t, l.TryAcquire("stream-3"), "a third distinct stream must be refused at limit=2")
	assert.Equal(t, 2, l.Active())
}

func TestVideoSessionLimiter_ReleaseFreesASlot(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(1)
	require.True(t, l.TryAcquire("stream-1"))
	require.False(t, l.TryAcquire("stream-2"), "a second distinct stream is refused at limit=1")
	l.Release("stream-1")
	assert.Equal(t, 0, l.Active())
	require.True(t, l.TryAcquire("stream-2"))
}

func TestVideoSessionLimiter_DoubleReleaseNeverGoesNegative(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(1)
	require.True(t, l.TryAcquire("h1"))
	l.Release("h1")
	l.Release("h1") // stray extra release
	assert.Equal(t, 0, l.Active())
	// still usable, not corrupted into a negative-active state that would
	// let unbounded acquires through.
	require.True(t, l.TryAcquire("h1"))
	assert.Equal(t, 1, l.Active())
}

func TestVideoSessionLimiter_SetLimitAppliesLiveWithoutKillingActive(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(5)
	require.True(t, l.TryAcquire("stream-1"))
	require.True(t, l.TryAcquire("stream-2"))
	assert.Equal(t, 2, l.Active())

	l.SetLimit(2)
	assert.Equal(t, 2, l.Active(), "SetLimit never kills already-acquired slots")
	assert.False(t, l.TryAcquire("stream-3"), "at the new lower limit, a NEW stream is refused")
	assert.True(t, l.TryAcquire("stream-1"), "an existing stream's sibling encoder still joins its own slot")
	l.Release("stream-1")

	l.Release("stream-1")
	require.True(t, l.TryAcquire("stream-3"), "a freed slot is usable again under the new limit")
}

func TestVideoSessionLimiter_SetLimitNegativeClampsToUnlimited(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(1)
	l.SetLimit(-3)
	assert.Equal(t, 0, l.Limit())
	for range 10 {
		require.True(t, l.TryAcquire("h1"))
	}
}

func TestVideoSessionLimiter_ConcurrentAcquireRelease_Race(t *testing.T) {
	l := NewVideoSessionLimiter(4)
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			if l.TryAcquire("h1") {
				l.Release("h1")
			}
		})
	}
	wg.Wait()
	assert.Equal(t, 0, l.Active())
}

// The bug that live testing caught: with the cap at 1 and slots counted per
// ffmpeg process, one viewer whose player probed a second ABR rung refused
// ITSELF (503). Slots are per STREAM: sibling encoders of the same decision
// hash join the stream's slot instead of competing for a new one.
func TestVideoSessionLimiter_SameStreamJoinsItsOwnSlot(t *testing.T) {
	l := NewVideoSessionLimiter(1)

	if !l.TryAcquire("stream-A") {
		t.Fatal("first encoder of the only allowed stream must be admitted")
	}
	if !l.TryAcquire("stream-A") {
		t.Fatal("an ABR sibling of the SAME stream must never be refused by its own slot")
	}
	if l.Active() != 1 {
		t.Errorf("two encoders of one stream = 1 slot, got %d", l.Active())
	}
	if l.TryAcquire("stream-B") {
		t.Error("a DIFFERENT stream must still be refused at capacity")
	}

	// The slot frees only once the stream's last encoder is gone.
	l.Release("stream-A")
	if l.Active() != 1 {
		t.Errorf("slot must survive while one sibling encoder remains, got %d", l.Active())
	}
	if l.TryAcquire("stream-B") {
		t.Error("still at capacity: the stream has not fully released")
	}
	l.Release("stream-A")
	if l.Active() != 0 {
		t.Errorf("last encoder gone = slot freed, got %d", l.Active())
	}
	if !l.TryAcquire("stream-B") {
		t.Error("a new stream must be admitted once the slot freed")
	}
}

func TestVideoSessionLimiter_ReleaseOfUnknownStreamIsNoop(t *testing.T) {
	l := NewVideoSessionLimiter(2)
	l.Release("never-acquired")
	if l.Active() != 0 {
		t.Fatalf("stray release must not corrupt the count, got %d", l.Active())
	}
	if !l.TryAcquire("a") || !l.TryAcquire("b") || l.TryAcquire("c") {
		t.Fatal("capacity must still be exactly 2 after a stray release")
	}
}
