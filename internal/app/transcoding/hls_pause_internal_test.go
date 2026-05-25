package transcoding

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// recordingHandle is an HLSProcessHandle that counts Pause/Resume calls so the
// flow-control logic can be asserted without a real ffmpeg process.
type recordingHandle struct {
	pauseCalls  atomic.Int32
	resumeCalls atomic.Int32
}

var _ transcoding.HLSProcessHandle = (*recordingHandle)(nil)

func (h *recordingHandle) WaitReady(context.Context) error { return nil }
func (h *recordingHandle) Stop() error                     { return nil }
func (h *recordingHandle) Done() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
func (h *recordingHandle) Err() error    { return nil }
func (h *recordingHandle) Pause() error  { h.pauseCalls.Add(1); return nil }
func (h *recordingHandle) Resume() error { h.resumeCalls.Add(1); return nil }

// An idle session is suspended once (idempotently) and resumed when the client
// fetches a segment again — the core of the "no live-edge jump after a long
// pause" fix.
func TestPauseIfIdleThenResumeOnAccess(t *testing.T) {
	h := &recordingHandle{}
	now := time.Now()
	s := &HLSSession{handle: h, LastAccess: now.Add(-30 * time.Second)}

	if !s.pauseIfIdle(20*time.Second, now) {
		t.Fatal("expected an idle session to be suspended")
	}
	if got := h.pauseCalls.Load(); got != 1 {
		t.Fatalf("pause calls = %d, want 1", got)
	}
	if !s.paused {
		t.Fatal("session should be marked paused")
	}

	// A second sweep must not signal SIGSTOP again.
	if s.pauseIfIdle(20*time.Second, now) {
		t.Fatal("an already-paused session should not pause again")
	}
	if got := h.pauseCalls.Load(); got != 1 {
		t.Fatalf("pause calls = %d, want 1 (idempotent)", got)
	}

	// A segment/playlist fetch resumes ffmpeg.
	s.MarkAccess("Mozilla/5.0")
	if got := h.resumeCalls.Load(); got != 1 {
		t.Fatalf("resume calls = %d, want 1", got)
	}
	if s.paused {
		t.Fatal("session should be active after MarkAccess")
	}

	// MarkAccess on an already-active session must not re-signal SIGCONT.
	s.MarkAccess("")
	if got := h.resumeCalls.Load(); got != 1 {
		t.Fatalf("resume calls = %d, want 1 (no resume while active)", got)
	}
}

func TestPauseIfIdleSkipsActiveAndCachedSessions(t *testing.T) {
	now := time.Now()

	// Recently accessed → still active, must not pause.
	h := &recordingHandle{}
	active := &HLSSession{handle: h, LastAccess: now.Add(-2 * time.Second)}
	if active.pauseIfIdle(20*time.Second, now) {
		t.Fatal("a recently-accessed session must not be paused")
	}
	if h.pauseCalls.Load() != 0 {
		t.Fatal("no SIGSTOP expected for an active session")
	}

	// Cached transcode (nil handle) → nothing to suspend.
	cached := &HLSSession{handle: nil, LastAccess: now.Add(-60 * time.Second)}
	if cached.pauseIfIdle(20*time.Second, now) {
		t.Fatal("a cached session (nil handle) must not be paused")
	}
}
