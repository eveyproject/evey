package ffmpeg

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// TestHLSHandleRun_PanicRecovered proves a panic inside hlsHandle.run() (the
// goroutine started by `go h.run(processCtx)`) does not crash the process and
// does not leave a WaitReady() caller hanging until its context budget
// expires: the recover path must mark the handle failed and close h.ready
// itself.
//
// The panic is injected by leaving h.cmd nil and cancelling ctx up front, so
// waitForFile returns immediately via ctx.Done() and the error branch's
// h.cmd.Wait() call dereferences a nil *exec.Cmd.
func TestHLSHandleRun_PanicRecovered(t *testing.T) {
	h := &hlsHandle{
		opts: transcoding.HLSProcessOpts{
			OutputDir:     t.TempDir(),
			VideoDecision: "copy",
		},
		cmd:    nil,
		ready:  make(chan struct{}),
		done:   make(chan struct{}),
		stderr: newRingBuffer(1024),
	}
	h.cancel = func() {} // no-op: nothing to cancel, but run() calls it unconditionally

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already-cancelled: waitForFile returns ctx.Err() without waiting

	waitErrCh := make(chan error, 1)
	go func() { waitErrCh <- h.WaitReady(context.Background()) }()

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("h.run() must recover internally, but panic escaped: %v", r)
			}
		}()
		h.run(ctx)
	}()

	// (b) WaitReady must resolve to an error promptly, not hang until its own
	// context's budget/timeout.
	select {
	case err := <-waitErrCh:
		if err == nil {
			t.Fatal("WaitReady must return an error after a panic in run(), got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitReady hung after a panic in run() instead of failing fast")
	}

	select {
	case <-h.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("h.done never closed after a panic in run()")
	}

	if h.Failure() == transcoding.FailureNone {
		t.Fatal("Failure() must not report FailureNone after a panic in run()")
	}
}

// TestCompositeHandleReap_PanicRecovered proves a panic inside
// compositeHandle.reap() (the goroutine started by `go c.reap()`) does not
// crash the process and still unblocks c.Done() for waiters. The panic is
// injected via a nil sub-handle: <-s.Done() dereferences h.done on a nil
// *hlsHandle.
func TestCompositeHandleReap_PanicRecovered(t *testing.T) {
	c := &compositeHandle{
		subs: []*hlsHandle{nil},
		done: make(chan struct{}),
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("c.reap() must recover internally, but panic escaped: %v", r)
			}
		}()
		c.reap()
	}()

	select {
	case <-c.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("c.done never closed after a panic in reap()")
	}
}

// The audio-only handle carries the same contract as its video sibling: a
// panic in its run loop must not crash the server, and must not strand the
// waiters either. Same class of bug, same guarantee.
func TestAudioHandleRun_PanicRecovered(t *testing.T) {
	h := &audioHandle{
		outputDir:    t.TempDir(),
		playlistName: "audio.m3u8",
		cmd:          nil, // nil deref inside run() once the wait path is reached
		ready:        make(chan struct{}),
		done:         make(chan struct{}),
		stderr:       newRingBuffer(1024),
	}
	h.cancel = func() {}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already-cancelled: waitForFile returns immediately, then cmd.Wait() derefs nil

	waitErrCh := make(chan error, 1)
	go func() { waitErrCh <- h.WaitReady(context.Background()) }()

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("audioHandle.run() must recover internally, but panic escaped: %v", r)
			}
		}()
		h.run(ctx)
	}()

	select {
	case err := <-waitErrCh:
		if err == nil {
			t.Fatal("WaitReady must report the failed session, not success")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitReady hung after a panic: the recovery must close ready")
	}

	select {
	case <-h.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done() must close even when run() panicked")
	}

	// Guard against a trivially-passing test: the recovery path must actually
	// have fired (nil cmd.Wait() derefs), not merely an early clean return.
	if h.err == nil || !strings.Contains(h.err.Error(), "panic") {
		t.Fatalf("expected the recovered panic to be recorded on the handle, got %v", h.err)
	}
	if h.Failure() == transcoding.FailureNone {
		t.Error("a panicked session must not classify as FailureNone")
	}
}
