package qoe

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/pkg/cmcd"
)

// captureStorage records every event it receives. block can be set to make
// inserts hang, which lets us prove buffer-full → drop without flakiness.
type captureStorage struct {
	mu     sync.Mutex
	events []Event
	block  chan struct{}
	err    error
}

func (c *captureStorage) InsertEvent(_ context.Context, evt Event) error {
	if c.block != nil {
		<-c.block
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.events = append(c.events, evt)
	return nil
}

func (c *captureStorage) snapshot() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Event, len(c.events))
	copy(out, c.events)
	return out
}

type countingDrops struct{ n atomic.Int64 }

func (c *countingDrops) Inc()      { c.n.Add(1) }
func (c *countingDrops) load() int { return int(c.n.Load()) }

func TestBufferedSink_RecordIsNonBlocking_DrainsToStorage(t *testing.T) {
	cap := &captureStorage{}
	drops := &countingDrops{}
	sink := NewBufferedSink(cap, SinkOptions{BufferSize: 16, DropMetric: drops}, nil)
	defer func() { _ = sink.Close(context.Background()) }()

	for i := range 10 {
		sink.Record(Event{
			MediaItemID: "media-1",
			VariantIdx:  i % 3,
			SegmentIdx:  i,
			Payload:     cmcd.Payload{EncodedKbps: 2400, BufferLengthMs: 12000},
		})
	}

	require.Eventually(t, func() bool {
		return len(cap.snapshot()) == 10
	}, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, 0, drops.load(), "no drops at buffer < cap")
}

func TestBufferedSink_DropsOnBufferFullWithoutBlocking(t *testing.T) {
	cap := &captureStorage{block: make(chan struct{})}
	drops := &countingDrops{}
	sink := NewBufferedSink(cap, SinkOptions{BufferSize: 4, DropMetric: drops}, nil)

	// Storage hangs → 1 event in worker mid-insert, 4 fill the channel, the rest drop.
	for i := range 20 {
		sink.Record(Event{MediaItemID: "x", SegmentIdx: i})
	}

	// Drops should be observable already (no flush happened).
	require.Eventually(t, func() bool {
		return drops.load() >= 15
	}, time.Second, 10*time.Millisecond, "buffer-full path must count drops")

	// Unblock storage so Close() can drain. Use a non-blocking close + drain
	// loop so the worker can proceed without us pinning the goroutine.
	go func() {
		for {
			select {
			case <-cap.block:
				return
			default:
				cap.block <- struct{}{}
			}
		}
	}()
	_ = sink.Close(context.Background())
}

func TestBufferedSink_RecordAfterCloseIsNoop(t *testing.T) {
	cap := &captureStorage{}
	drops := &countingDrops{}
	sink := NewBufferedSink(cap, SinkOptions{BufferSize: 4, DropMetric: drops}, nil)

	require.NoError(t, sink.Close(context.Background()))
	sink.Record(Event{MediaItemID: "post-close"})
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, cap.snapshot())
	assert.Equal(t, 0, drops.load(), "post-close records aren't drops, just no-ops")
}

func TestBufferedSink_StorageErrorDoesNotKillWorker(t *testing.T) {
	cap := &captureStorage{err: errors.New("db down")}
	sink := NewBufferedSink(cap, SinkOptions{BufferSize: 8}, nil)
	defer func() { _ = sink.Close(context.Background()) }()

	for i := range 5 {
		sink.Record(Event{MediaItemID: "x", SegmentIdx: i})
	}

	// Give worker time to attempt all 5 inserts.
	time.Sleep(100 * time.Millisecond)
	// Worker survives errors → channel keeps draining → subsequent Record
	// calls still don't block.
	sink.Record(Event{MediaItemID: "y"})
}

// A storage that PANICS (not just errors) on one event must not kill the worker:
// the per-iteration recover logs it and the loop keeps draining, so a later event
// still lands. A recover on the outer loop instead would exit the goroutine and
// silently drop all subsequent telemetry.
func TestBufferedSink_StoragePanicDoesNotKillWorker(t *testing.T) {
	st := &panicOnceStorage{}
	sink := NewBufferedSink(st, SinkOptions{BufferSize: 8}, nil)
	sink.Record(Event{MediaItemID: "boom"}) // first insert panics (recovered)
	sink.Record(Event{MediaItemID: "ok"})   // must still be processed
	require.NoError(t, sink.Close(context.Background()))

	got := st.snapshot()
	require.Len(t, got, 1, "worker must survive the panic and process the 2nd event")
	assert.Equal(t, "ok", got[0].MediaItemID)
}

// panicOnceStorage panics on its first InsertEvent, then records normally.
type panicOnceStorage struct {
	mu     sync.Mutex
	events []Event
	fired  bool
}

func (p *panicOnceStorage) InsertEvent(_ context.Context, evt Event) error {
	p.mu.Lock()
	if !p.fired {
		p.fired = true
		p.mu.Unlock()
		panic("simulated storage panic")
	}
	p.events = append(p.events, evt)
	p.mu.Unlock()
	return nil
}

func (p *panicOnceStorage) snapshot() []Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Event, len(p.events))
	copy(out, p.events)
	return out
}

func TestBufferedSink_RecordDuringClose_NoSendOnClosedPanic(t *testing.T) {
	// Regression: Record's lock-free closed.Load() check and Close's close(ch)
	// raced - a Record that passed the check, then resumed its send after
	// close(ch) ran, panicked "send on closed channel" (in the HTTP goroutine,
	// where the worker's recover() does NOT catch it). sendMu now serializes
	// the send against the close. A panic in a Record goroutine crashes the
	// test binary, so completing all iterations == the window is closed.
	// Repeat with many concurrent senders to make the interleaving likely.
	for range 50 {
		sink := NewBufferedSink(&captureStorage{}, SinkOptions{BufferSize: 4}, nil)

		const senders = 16
		var wg sync.WaitGroup
		start := make(chan struct{})
		for range senders {
			wg.Go(func() {
				<-start
				for i := range 200 {
					sink.Record(Event{MediaItemID: "m", SegmentIdx: i})
				}
			})
		}
		close(start) // unleash senders, then close while sends are in flight
		require.NoError(t, sink.Close(context.Background()))
		wg.Wait()
	}
}
