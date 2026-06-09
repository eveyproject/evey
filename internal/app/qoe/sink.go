// Package qoe owns the ingest path for CMCD (CTA-5004) playback telemetry.
// Phase 1: a non-blocking sink that buffers events and forwards them to the
// repository on a worker goroutine. Phase 3 will plug a second consumer onto
// the same channel for the live feedback loop (throttle wake-up, prefetch
// adaptation) without touching the HTTP path.
//
// Non-blocking is non-negotiable: every CMCD record costs us a goroutine on
// the segment hot path and we'd rather drop a metric than add latency to
// playback. Drops are counted via Prometheus so we can size the buffer if
// they ever actually happen.
package qoe

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"github.com/evey-project/evey/internal/pkg/cmcd"
)

// EventSink is the boundary the HTTP handler talks to. Everything the handler
// knows about CMCD persistence is on this two-method interface, so swapping
// the SQL backend for a no-op (tests) or a fan-out (Phase 3) doesn't ripple.
type EventSink interface {
	// Record enqueues one event. MUST NOT block. MUST be safe to call from
	// any goroutine.
	Record(evt Event)
	// Close drains the pending buffer with a deadline and stops the worker.
	Close(ctx context.Context) error
}

// Event is the app-layer view of a single CMCD record. It carries the parsed
// payload plus the context we resolved on the HTTP side (who, what media,
// which segment). The repo translates this into a storage row.
type Event struct {
	UserID       string // empty for anon/shared-link plays
	MediaItemID  string
	VariantIdx   int
	SegmentIdx   int
	IsInit       bool
	IsManifest   bool
	IsVariantPL  bool
	DeviceClass  string
	RemoteIP     string
	Payload      cmcd.Payload
}

// Storage is the narrow contract the sink needs from the persistence layer.
// Defined here (not imported from postgres) so the app package doesn't
// reach into the adapter.
type Storage interface {
	InsertEvent(ctx context.Context, evt Event) error
}

// DropCounter is the metric port. Pass a Prometheus counter via
// prometheusDropCounter, or a no-op in tests.
type DropCounter interface {
	Inc()
}

// nopCounter is the default if no metric is wired (tests).
type nopCounter struct{}

func (nopCounter) Inc() {}

// BufferedSink is the production sink: one worker goroutine drains a channel
// into Storage. Channel-full causes drops, never backpressure.
type BufferedSink struct {
	storage Storage
	ch      chan Event
	dropMtr DropCounter
	logger  *slog.Logger
	// sendMu serializes the send on ch against Close's close(ch): Record holds
	// it RLocked across the (re-checked) send, Close takes it Locked before
	// close(ch). While any Record holds the RLock the channel cannot be closed,
	// so a send can never race the close — no "send on closed channel" panic on
	// shutdown while telemetry is still arriving. closed stays an atomic for the
	// lock-free fast-path return; the RLock is uncontended between concurrent
	// Records (readers) so the hot path stays non-blocking.
	sendMu   sync.RWMutex
	closed   atomic.Bool
	workerWG sync.WaitGroup
}

// SinkOptions tune the buffer + worker. Zero values produce 1024-event
// buffer and 1 worker — fine for self-host workloads (single-server, dozens
// of concurrent sessions).
type SinkOptions struct {
	BufferSize int
	DropMetric DropCounter
}

// NewBufferedSink starts a worker that drains the channel into storage. The
// returned sink is safe to call from any goroutine. Call Close on shutdown.
func NewBufferedSink(storage Storage, opts SinkOptions, logger *slog.Logger) *BufferedSink {
	if opts.BufferSize <= 0 {
		opts.BufferSize = 1024
	}
	if opts.DropMetric == nil {
		opts.DropMetric = nopCounter{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	s := &BufferedSink{
		storage: storage,
		ch:      make(chan Event, opts.BufferSize),
		dropMtr: opts.DropMetric,
		logger:  logger.With("component", "qoe_sink"),
	}
	s.workerWG.Add(1)
	go s.worker()
	return s
}

// Record is non-blocking. Drops on full buffer and counts the drop. This is
// the right tradeoff for telemetry — a slow DB must never delay a segment.
func (s *BufferedSink) Record(evt Event) {
	if s.closed.Load() {
		return
	}
	s.sendMu.RLock()
	defer s.sendMu.RUnlock()
	if s.closed.Load() {
		// Close ran between the lock-free fast-path check and acquiring the
		// RLock — the channel is closed (or about to be); drop, don't send.
		return
	}
	select {
	case s.ch <- evt:
	default:
		s.dropMtr.Inc()
		// TODO: remove after CMCD stabilization — verbose drop warning to
		// surface a misconfigured buffer size during the Phase 1 rollout.
		s.logger.Warn("qoe sink buffer full, event dropped",
			slog.String("media_id", evt.MediaItemID),
			slog.Int("variant", evt.VariantIdx),
			slog.Int("seg", evt.SegmentIdx))
	}
}

func (s *BufferedSink) Close(ctx context.Context) error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	// Take the write lock so any in-flight Record send completes before we
	// close — closed is already true, so no new send will start past the
	// re-check. This is what makes close(ch) safe against concurrent Record.
	s.sendMu.Lock()
	close(s.ch)
	s.sendMu.Unlock()
	done := make(chan struct{})
	go func() {
		s.workerWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *BufferedSink) worker() {
	defer s.workerWG.Done()
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("qoe sink worker panic recovered",
				slog.Any("err", r),
				slog.String("stack", string(debug.Stack())))
		}
	}()
	for evt := range s.ch {
		// TODO: remove after CMCD stabilization — verbose per-event log to
		// confirm payloads actually arrive during Phase 1.
		s.logger.Debug("qoe ingest",
			slog.String("media_id", evt.MediaItemID),
			slog.String("user_id", evt.UserID),
			slog.String("session_id", evt.Payload.SessionID),
			slog.Int("variant", evt.VariantIdx),
			slog.Int("seg", evt.SegmentIdx),
			slog.Int("buffer_ms", evt.Payload.BufferLengthMs),
			slog.Int("encoded_kbps", evt.Payload.EncodedKbps),
			slog.Int("measured_kbps", evt.Payload.MeasuredKbps),
			slog.Bool("starved", evt.Payload.BufferStarved),
			slog.Bool("startup", evt.Payload.Startup))

		// Use Background — the HTTP request context is long gone by the
		// time we drain this event. The query has its own withTimeout.
		if err := s.storage.InsertEvent(context.Background(), evt); err != nil {
			s.logger.Warn("qoe sink: insert failed",
				slog.String("media_id", evt.MediaItemID),
				slog.Any("err", err))
		}
	}
}

// NoopSink is the test/dev double — Record is a no-op, Close returns nil.
type NoopSink struct{}

func (NoopSink) Record(Event)                       {}
func (NoopSink) Close(context.Context) error        { return nil }
