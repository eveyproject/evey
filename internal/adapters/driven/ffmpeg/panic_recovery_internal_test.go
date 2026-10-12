package ffmpeg

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// TestHLSHandleRun_PanicRecovered proves a panic inside hlsHandle.run() (the
// goroutine started by `go h.run(processCtx)`) does not crash the process and
// does not leave a WaitReady() caller hanging until its context budget
// expires: the recover path must mark the handle failed and close h.ready
// itself.
//
// The panic is injected through h.cancel: with ctx already cancelled, waitForFile
// returns at once and run()'s error branch calls h.cancel() before anything
// else, so a cancel that panics on its first call fires a real panic inside
// run(). The recover path's own h.cancel() call is the harmless second one.
// procExited is closed so awaitProcExit can never strand the goroutine even if
// the flow ever reaches it (the old test left it nil and hung on <-nil forever).
func TestHLSHandleRun_PanicRecovered(t *testing.T) {
	h := &hlsHandle{
		opts: transcoding.HLSProcessOpts{
			OutputDir:     t.TempDir(),
			VideoDecision: "copy",
		},
		cmd:        nil,
		ready:      make(chan struct{}),
		done:       make(chan struct{}),
		procExited: make(chan struct{}),
		stderr:     newRingBuffer(1024),
	}
	close(h.procExited) // no process to wait on; keep awaitProcExit non-blocking
	var cancelled bool
	h.cancel = func() {
		if !cancelled {
			cancelled = true
			panic("injected panic in run() error branch")
		}
	}

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
		cmd:          nil,
		ready:        make(chan struct{}),
		done:         make(chan struct{}),
		procExited:   make(chan struct{}),
		stderr:       newRingBuffer(1024),
	}
	close(h.procExited) // no process to wait on; keep awaitProcExit non-blocking
	var cancelled bool
	h.cancel = func() {
		if !cancelled {
			cancelled = true
			panic("injected panic in run() error branch")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already-cancelled: waitForFile returns at once, then h.cancel() panics

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

// Err() may be called by the orchestrator at any moment (its natural-exit
// bookkeeping), concurrently with run() writing h.err on its error paths.
// Both sides must go through the handle's mutex - the race detector is the
// assertion here.
func TestAudioHandleErr_RaceWithRunErrorPath(t *testing.T) {
	h := &audioHandle{
		outputDir:    t.TempDir(),
		playlistName: "audio.m3u8",
		cmd:          nil,
		ready:        make(chan struct{}),
		done:         make(chan struct{}),
		procExited:   make(chan struct{}),
		stderr:       newRingBuffer(1024),
		cancel:       func() {},
	}
	close(h.procExited)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // waitForFile fails immediately -> run() writes h.err

	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				_ = h.Err()
			}
		}
	}()
	h.run(ctx)
	close(stop)
	if h.Err() == nil {
		t.Fatal("run's error branch must surface through Err()")
	}
}
