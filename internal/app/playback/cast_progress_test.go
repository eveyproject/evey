package playback

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type reportCall struct {
	profileID, mediaItemID string
	positionMs             int64
}

type reportRecorder struct {
	mu    sync.Mutex
	calls []reportCall
	err   error
}

func (r *reportRecorder) report(_ context.Context, profileID, mediaItemID string, positionMs int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.calls = append(r.calls, reportCall{profileID, mediaItemID, positionMs})
	return nil
}

func (r *reportRecorder) all() []reportCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]reportCall(nil), r.calls...)
}

func testTracker(rec *reportRecorder) *CastProgressTracker {
	t := NewCastProgressTracker(rec.report)
	return t
}

// The receiver fetches AHEAD of the playhead (it buffers minutes), so the
// reported position is the fetch high-water MINUS a buffer margin - resuming
// slightly before what was watched beats resuming after it.
func TestCastProgress_ReportsHighWaterMinusMargin(t *testing.T) {
	rec := &reportRecorder{}
	tr := testTracker(rec)

	tr.ObserveFetch("p1", "m1", 120_000) // receiver fetched up to 2:00
	tr.flushOnce(context.Background())

	calls := rec.all()
	if len(calls) != 1 {
		t.Fatalf("want 1 report, got %d", len(calls))
	}
	want := 120_000 - castBufferMarginMs
	if calls[0].positionMs != want || calls[0].profileID != "p1" || calls[0].mediaItemID != "m1" {
		t.Fatalf("got %+v, want pos %d for p1/m1", calls[0], want)
	}
}

// Below the margin there is nothing credible to say (the receiver may have
// prefetched the whole opening without the user watching a second).
func TestCastProgress_NoReportBelowMargin(t *testing.T) {
	rec := &reportRecorder{}
	tr := testTracker(rec)

	tr.ObserveFetch("p1", "m1", castBufferMarginMs/2)
	tr.flushOnce(context.Background())

	if n := len(rec.all()); n != 0 {
		t.Fatalf("a fetch inside the margin must not report, got %d calls", n)
	}
}

// Reports are monotonic per (profile, media): a late out-of-order segment
// fetch (receiver refilling behind after a seek) must never rewind the
// position, and an unchanged high-water must not re-report.
func TestCastProgress_MonotonicAndDeduped(t *testing.T) {
	rec := &reportRecorder{}
	tr := testTracker(rec)

	tr.ObserveFetch("p1", "m1", 300_000)
	tr.flushOnce(context.Background())
	tr.ObserveFetch("p1", "m1", 240_000) // behind the high-water
	tr.flushOnce(context.Background())
	tr.flushOnce(context.Background()) // nothing new at all

	calls := rec.all()
	if len(calls) != 1 {
		t.Fatalf("want exactly 1 report (monotonic + deduped), got %d: %+v", len(calls), calls)
	}
}

// A failed report must keep the entry so the next flush retries it.
func TestCastProgress_ReportErrorRetriesNextFlush(t *testing.T) {
	rec := &reportRecorder{err: errors.New("db down")}
	tr := testTracker(rec)

	tr.ObserveFetch("p1", "m1", 300_000)
	tr.flushOnce(context.Background())
	rec.mu.Lock()
	rec.err = nil
	rec.mu.Unlock()
	tr.flushOnce(context.Background())

	calls := rec.all()
	if len(calls) != 1 || calls[0].positionMs != 300_000-castBufferMarginMs {
		t.Fatalf("the position must survive a failed report and land on retry, got %+v", calls)
	}
}

// When the receiver goes quiet (stopped casting, film over), the entry is
// final-flushed then dropped - the map must not grow with dead casts.
func TestCastProgress_IdleEntryFinalFlushedAndDropped(t *testing.T) {
	rec := &reportRecorder{}
	tr := testTracker(rec)
	now := time.Now()
	tr.now = func() time.Time { return now }

	tr.ObserveFetch("p1", "m1", 600_000)
	now = now.Add(castEntryIdleTTL + time.Second)
	tr.flushOnce(context.Background())

	if len(rec.all()) != 1 {
		t.Fatalf("idle entry must still final-flush its progress, got %d calls", len(rec.all()))
	}
	tr.mu.Lock()
	n := len(tr.entries)
	tr.mu.Unlock()
	if n != 0 {
		t.Fatalf("idle entry must be dropped after the final flush, %d left", n)
	}
}

// Stop must flush what is pending: the common end of a cast is the user
// turning everything off - that last position is exactly the one that matters.
func TestCastProgress_StopFinalFlushes(t *testing.T) {
	rec := &reportRecorder{}
	tr := testTracker(rec)
	tr.Start(t.Context())

	tr.ObserveFetch("p1", "m1", 480_000)
	tr.Stop()

	if len(rec.all()) != 1 {
		t.Fatalf("Stop must final-flush pending progress, got %d calls", len(rec.all()))
	}
}
