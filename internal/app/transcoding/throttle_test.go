package transcoding_test

import (
	"testing"

	"github.com/gravity-zero/evey/internal/app/transcoding"
	"github.com/stretchr/testify/assert"
)

func TestAcquire_UnderLimit(t *testing.T) {
	th := transcoding.NewTranscodeThrottle()
	assert.True(t, th.Acquire("user1", 3))
	assert.True(t, th.Acquire("user1", 3))
}

func TestAcquire_AtLimit_Rejected(t *testing.T) {
	th := transcoding.NewTranscodeThrottle()
	th.Acquire("user1", 2)
	th.Acquire("user1", 2)
	assert.False(t, th.Acquire("user1", 2))
}

func TestAcquire_OverLimit_Rejected(t *testing.T) {
	th := transcoding.NewTranscodeThrottle()
	th.Acquire("user1", 1)
	assert.False(t, th.Acquire("user1", 1))
	assert.False(t, th.Acquire("user1", 1))
}

func TestAcquire_Unlimited_WhenMaxZero(t *testing.T) {
	th := transcoding.NewTranscodeThrottle()
	for range 100 {
		assert.True(t, th.Acquire("user1", 0))
	}
}

func TestRelease_DecrementsCount(t *testing.T) {
	th := transcoding.NewTranscodeThrottle()
	th.Acquire("user1", 1)
	assert.False(t, th.Acquire("user1", 1))
	th.Release("user1")
	assert.True(t, th.Acquire("user1", 1))
}

func TestRelease_NeverGoesBelowZero(t *testing.T) {
	th := transcoding.NewTranscodeThrottle()
	th.Release("nonexistent")
	assert.True(t, th.Acquire("nonexistent", 1))
}

func TestAcquire_BoundaryExact_MaxStreams(t *testing.T) {
	th := transcoding.NewTranscodeThrottle()
	max := 3
	for i := range max {
		assert.True(t, th.Acquire("user1", max), "acquire %d should succeed", i)
	}
	assert.False(t, th.Acquire("user1", max), "acquire at max should fail")
	th.Release("user1")
	assert.True(t, th.Acquire("user1", max), "acquire after release should succeed")
}

func TestAcquire_IsolatedPerUser(t *testing.T) {
	th := transcoding.NewTranscodeThrottle()
	th.Acquire("user1", 1)
	assert.False(t, th.Acquire("user1", 1))
	assert.True(t, th.Acquire("user2", 1))
}
