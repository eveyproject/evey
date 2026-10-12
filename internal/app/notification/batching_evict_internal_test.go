package notification

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	libevent "github.com/eveyproject/evey/internal/domain/library"
	notiffakes "github.com/eveyproject/evey/internal/domain/notification/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

// The eviction used to leave the entry in s.batches until the detached flush
// goroutine re-acquired the mutex: under a burst of brand-new keys, every
// insert re-elected the SAME oldest entry and the map grew past
// maxBatchEntries - the cap was advisory. The pop now happens under the held
// lock, so the cap is hard. Internal test: the invariant lives on a private
// map.
func TestBufferSeriesEvent_CapStaysHardUnderBurst(t *testing.T) {
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	profiles := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	s := NewCommandService(repo, settings, nil, profiles, bus, WithBatchDelay(time.Hour))

	for i := range maxBatchEntries + 50 {
		s.bufferLocked(libevent.MediaItemAdded{
			MediaItemID: "mi-burst",
			LibraryID:   "lib1",
			Title:       fmt.Sprintf("Some Series %d", i),
			SeriesTitle: fmt.Sprintf("Some Series %d", i),
			Type:        libevent.Series,
			OccurredAt:  time.Now(),
		})
	}

	s.mu.Lock()
	size := len(s.batches)
	s.mu.Unlock()
	assert.LessOrEqual(t, size, maxBatchEntries,
		"the batch map exceeded its cap: eviction must pop under the held lock, not defer to the flush goroutine")
}

// bufferLocked mirrors handleMediaItemAdded's locking around
// bufferSeriesEvent (caller must hold s.mu).
func (s *CommandService) bufferLocked(e libevent.MediaItemAdded) {
	s.mu.Lock()
	s.bufferSeriesEvent(e)
	s.mu.Unlock()
}
