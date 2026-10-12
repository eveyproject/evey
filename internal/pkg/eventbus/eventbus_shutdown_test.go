package eventbus

import (
	"context"
	"sync"
	"testing"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type shutdownEvent struct{}

func (shutdownEvent) EventName() string { return "test.shutdown" }

// Stop() closes the dispatch queue while publishers may be mid-Publish. The
// guard that checks "are we stopping" and the send onto the queue sit in two
// separate selects, so a publisher can pass the guard, lose the scheduler, and
// come back to send on a channel Stop has since closed - an unrecovered
// `send on closed channel` that takes the whole process down.
//
// This is not a laboratory case: Stop() runs before the gRPC server stops
// accepting requests, background workers are still publishing, and the admin
// panel's restart button sends the process its own SIGTERM - so an operator
// clicking restart can crash the server instead of stopping it cleanly.
func TestEventBus_PublishDuringStop_DoesNotPanic(t *testing.T) {
	for range 40 {
		bus := New()
		bus.Subscribe("test.shutdown", func(context.Context, shared.Event) {})

		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				for range 50 {
					// Must never panic, whatever Stop is doing concurrently.
					bus.Publish(context.Background(), shutdownEvent{})
				}
			})
		}
		go bus.Stop()
		wg.Wait()
	}
}
