package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// hls_handle.go groups the ffmpeg process lifecycle: compositeHandle,
// hlsHandle, the stderr ringBuffer, and the wait-for-file/segment helpers.

// compositeHandle presents N single-output ffmpeg sub-handles (one per ABR
// rung) as one HLSProcessHandle. Done closes when ALL rungs exit; Failure
// surfaces a HW-capability class from any rung (so the orchestrator demotes the
// host) else the first non-None; Stop/Pause/Resume fan out to every rung.
type compositeHandle struct {
	ctx     context.Context
	subs    []*hlsHandle
	cancel  context.CancelFunc
	done    chan struct{}
	mu      sync.Mutex
	stopped bool
}

func (c *compositeHandle) reap() {
	defer close(c.done)
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(c.ctx, "panic in hls composite reap",
				slog.Any("recover", r), slog.String("stack", string(debug.Stack())))
		}
	}()
	for _, s := range c.subs {
		<-s.Done()
	}
}

func (c *compositeHandle) WaitReady(ctx context.Context) error {
	for _, s := range c.subs {
		if err := s.WaitReady(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (c *compositeHandle) Stop() error {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return nil
	}
	c.stopped = true
	c.mu.Unlock()
	c.cancel()
	var wg sync.WaitGroup
	errs := make([]error, len(c.subs))
	for i, s := range c.subs {
		wg.Add(1)
		go func(idx int, h *hlsHandle) { defer wg.Done(); errs[idx] = h.Stop() }(i, s)
	}
	wg.Wait()
	return errors.Join(errs...)
}

func (c *compositeHandle) Done() <-chan struct{} { return c.done }

func (c *compositeHandle) Err() error {
	for _, s := range c.subs {
		if e := s.Err(); e != nil {
			return e
		}
	}
	return nil
}

func (c *compositeHandle) Failure() transcoding.FailureClass {
	first := transcoding.FailureNone
	for _, s := range c.subs {
		f := s.Failure()
		if f.IsHWCapability() {
			return f // any rung's HW failure → demote the host
		}
		if first == transcoding.FailureNone {
			first = f
		}
	}
	return first
}

func (c *compositeHandle) Pause() error {
	var errs []error
	for _, s := range c.subs {
		errs = append(errs, s.Pause())
	}
	return errors.Join(errs...)
}

func (c *compositeHandle) Resume() error {
	var errs []error
	for _, s := range c.subs {
		errs = append(errs, s.Resume())
	}
	return errors.Join(errs...)
}

type hlsHandle struct {
	opts      transcoding.HLSProcessOpts
	mu        sync.Mutex
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	ready     chan struct{}
	readyOnce sync.Once // guards ready: run()'s error paths AND its panic recovery can both race to close it
	done      chan struct{}
	// procExited closes the instant the ffmpeg process itself exits (single
	// owner of cmd.Wait; procWaitErr holds its result). The file/segment
	// waits select on it so a dead-on-arrival ffmpeg fails the session in
	// milliseconds instead of burning the full 30s playlist timeout while
	// holding a video admission slot and a viewer's request.
	procExited  chan struct{}
	procWaitErr error
	err         error
	exitErr     error // raw ffmpeg exit error, for failure classification (guarded by mu)
	stderr      *ringBuffer
	paused      bool // ffmpeg currently SIGSTOPped (client idle)
	// noMaster suppresses the synthesised single-rung master.m3u8. Set on the
	// sub-processes of a per-variant composite handle (the VOD pipeline serves a
	// precomputed master, and N siblings must not each clobber master.m3u8 with
	// a one-rung playlist).
	noMaster bool
}

// awaitProcExit blocks until the ffmpeg process has exited and returns its
// exit error. Replaces direct h.awaitProcExit() calls: Wait may only be called
// once per process, and the exit watcher above owns that call.
func (h *hlsHandle) awaitProcExit() error {
	<-h.procExited
	return h.procWaitErr
}

func (h *hlsHandle) setExitErr(err error) {
	h.mu.Lock()
	h.exitErr = err
	h.mu.Unlock()
}

// closeReady closes h.ready at most once. run() closes it on several distinct
// return paths (including the panic-recovery path below), so a plain
// close(h.ready) would double-close and panic if two of those paths were ever
// reached for the same handle.
func (h *hlsHandle) closeReady() {
	h.readyOnce.Do(func() { close(h.ready) })
}

func (h *hlsHandle) run(ctx context.Context) {
	defer close(h.done)
	// A panic here (e.g. a bug in the file/segment polling below, or in a
	// future change) would otherwise crash the whole server: this goroutine
	// has no caller to propagate to. Recover, classify it as a failed
	// session, and unblock anyone in WaitReady()/cache.Wait() so the request
	// fails fast instead of hanging until its budget/timeout expires.
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "panic in hls ffmpeg run loop",
				slog.Any("recover", r), slog.String("stack", string(debug.Stack())))
			h.err = fmt.Errorf("panic in hls transcoder run loop: %v", r)
			h.setExitErr(h.err)
			h.cancel()
			h.closeReady()
		}
	}()

	// Single-variant outputs (copy + single-profile transcode) write only
	// stream_0.m3u8; we synthesise master.m3u8 manually. Multi-variant
	// transcodes use ffmpeg's -master_pl_name and produce master.m3u8 directly.
	isSingle := h.opts.VideoDecision == "copy" || len(h.opts.Profiles) <= 1

	if isSingle {
		streamName := fmt.Sprintf("stream_%d", h.opts.VariantIndex)
		streamPath := filepath.Join(h.opts.OutputDir, streamName+".m3u8")
		if err := waitForFile(ctx, h.procExited, streamPath, 30*time.Second, 100*time.Millisecond); err != nil {
			h.err = fmt.Errorf("waiting for %s.m3u8: %w", streamName, err)
			h.cancel()
			we := h.awaitProcExit()
			h.setExitErr(we)
			if we != nil {
				slog.Debug("ffmpeg wait after cancel", slog.Any("err", we))
			}
			h.closeReady()
			return
		}
		// Standalone single-variant handles synthesise the one-rung master; a
		// per-variant composite's siblings set noMaster (the VOD pipeline serves
		// a precomputed master, and N siblings must not clobber master.m3u8).
		if !h.noMaster {
			master := fmt.Sprintf("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-STREAM-INF:BANDWIDTH=5000000\n%s.m3u8\n", streamName)
			if err := os.WriteFile(filepath.Join(h.opts.OutputDir, "master.m3u8"), []byte(master), 0644); err != nil {
				h.err = fmt.Errorf("write master.m3u8: %w", err)
				h.cancel()
				we := h.awaitProcExit()
				h.setExitErr(we)
				if we != nil {
					slog.Debug("ffmpeg wait after cancel", slog.Any("err", we))
				}
				h.closeReady()
				return
			}
		}
		if err := waitForSegment(ctx, h.procExited, h.opts.OutputDir, streamName, 10*time.Second, 100*time.Millisecond); err != nil {
			slog.WarnContext(ctx, "hls: first segment not yet ready, proceeding anyway", slog.String("output_dir", h.opts.OutputDir), slog.Int("variant", h.opts.VariantIndex), slog.Any("err", err))
		}
	} else {
		masterPath := filepath.Join(h.opts.OutputDir, "master.m3u8")
		if err := waitForFile(ctx, h.procExited, masterPath, 30*time.Second, 100*time.Millisecond); err != nil {
			h.err = fmt.Errorf("waiting for master.m3u8: %w", err)
			h.cancel()
			we := h.awaitProcExit()
			h.setExitErr(we)
			if we != nil {
				slog.Debug("ffmpeg wait after cancel", slog.Any("err", we))
			}
			h.closeReady()
			return
		}
		if err := waitForSegment(ctx, h.procExited, h.opts.OutputDir, "stream_0", 10*time.Second, 100*time.Millisecond); err != nil {
			slog.WarnContext(ctx, "hls: first segment not yet ready, proceeding anyway", slog.String("output_dir", h.opts.OutputDir), slog.Int("variant", h.opts.VariantIndex), slog.Any("err", err))
		}
	}

	h.closeReady()

	we := h.awaitProcExit()
	h.setExitErr(we)
	if we != nil && ctx.Err() == nil {
		slog.WarnContext(ctx, "hls: ffmpeg exited with error",
			slog.Any("err", we),
			slog.String("stderr_tail", h.stderr.String()))
	}
}

// Failure classifies why the process exited, for the level-2 HW safety net. It
// is only meaningful after Done() has closed; before that exitErr is nil and it
// reports FailureNone. Reads the raw exit error + the captured stderr tail.
func (h *hlsHandle) Failure() transcoding.FailureClass {
	h.mu.Lock()
	e := h.exitErr
	h.mu.Unlock()
	return ClassifyHWFailure(e, h.stderr.String())
}

// ringBuffer keeps the last N bytes written to it. We use it to capture
// ffmpeg's stderr tail without unbounded memory growth: ffmpeg can spam
// MBs of progress lines, we only care about the last few KB which carry
// the fatal error message on exit.
type ringBuffer struct {
	mu   sync.Mutex
	buf  []byte
	size int
	full bool
	pos  int
}

func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{buf: make([]byte, size), size: size}
}

func (r *ringBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(p)
	for _, b := range p {
		r.buf[r.pos] = b
		r.pos++
		if r.pos >= r.size {
			r.pos = 0
			r.full = true
		}
	}
	return n, nil
}

func (r *ringBuffer) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		return string(r.buf[:r.pos])
	}
	out := make([]byte, 0, r.size)
	out = append(out, r.buf[r.pos:]...)
	out = append(out, r.buf[:r.pos]...)
	return string(out)
}

func (h *hlsHandle) WaitReady(ctx context.Context) error {
	select {
	case <-h.ready:
		return h.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *hlsHandle) Stop() error {
	// A SIGSTOPped process can't observe ctx cancellation. Always SIGCONT first
	// so the cancel→graceful-exit path completes in milliseconds; otherwise we
	// pay the full 5s timeout and fall through to SIGKILL on every throttled
	// session teardown.
	h.mu.Lock()
	if h.paused && h.cmd != nil && h.cmd.Process != nil {
		// Failure here only matters if the process has already exited,
		// in which case the Stop path below treats the missing process
		// as already-stopped - nothing to recover.
		if err := h.cmd.Process.Signal(syscall.SIGCONT); err != nil {
			_ = err // best-effort wake before stop
		}
		h.paused = false
	}
	h.mu.Unlock()

	h.cancel()
	select {
	case <-h.done:
	case <-time.After(5 * time.Second):
		h.mu.Lock()
		if h.cmd != nil && h.cmd.Process != nil {
			if err := h.cmd.Process.Kill(); err != nil {
				slog.Warn("ffmpeg kill after timeout failed", slog.String("output_dir", h.opts.OutputDir), slog.Int("variant", h.opts.VariantIndex), slog.Any("err", err))
			}
		}
		h.mu.Unlock()
	}
	return nil
}

func (h *hlsHandle) Done() <-chan struct{} {
	return h.done
}

func (h *hlsHandle) Err() error {
	return h.err
}

// Pause sends SIGSTOP so ffmpeg stops encoding while the client isn't fetching
// segments. With the event-mode playlist nothing can be evicted underneath the
// paused user, but -readrate 1.5 still lets ffmpeg keep encoding ~1.5× real-time
// during a long pause - burning CPU and disk on segments the user may never
// reach. Suspending while idle keeps the playlist stable and the host quiet.
// Idempotent; a no-op once the process has exited.
func (h *hlsHandle) Pause() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.paused || h.cmd == nil || h.cmd.Process == nil {
		return nil
	}
	if err := h.cmd.Process.Signal(syscall.SIGSTOP); err != nil {
		// Process likely already exited (finished/killed) - nothing to suspend.
		return err
	}
	h.paused = true
	return nil
}

// Resume sends SIGCONT to let a paused ffmpeg continue. Idempotent.
func (h *hlsHandle) Resume() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.paused || h.cmd == nil || h.cmd.Process == nil {
		return nil
	}
	if err := h.cmd.Process.Signal(syscall.SIGCONT); err != nil {
		return err
	}
	h.paused = false
	return nil
}

// --- ffmpeg arg building ---

func waitForFile(ctx context.Context, exited <-chan struct{}, path string, timeout, interval time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-exited:
			// The process is gone: one final look (it may have written the
			// file in its last breath), then fail fast instead of burning the
			// remaining timeout on a file that can no longer appear. A
			// dead-on-arrival ffmpeg (vanished source, bad args) otherwise
			// held its video admission slot and its viewer's request for the
			// full 30s.
			if _, err := os.Stat(path); err == nil {
				return nil
			}
			return fmt.Errorf("ffmpeg exited before producing %s", path)
		case <-deadline:
			return fmt.Errorf("timeout waiting for %s", path)
		case <-ticker.C:
			if _, err := os.Stat(path); err == nil {
				return nil
			}
		}
	}
}

func waitForSegment(ctx context.Context, exited <-chan struct{}, dir, streamPrefix string, timeout, interval time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	scan := func() bool {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			name := e.Name()
			if len(name) > len(streamPrefix) && name[:len(streamPrefix)] == streamPrefix && (strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".m4s")) {
				return true
			}
		}
		return false
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-exited:
			if scan() {
				return nil
			}
			return fmt.Errorf("ffmpeg exited before the first segment of %s", streamPrefix)
		case <-deadline:
			return fmt.Errorf("timeout waiting for first segment of %s", streamPrefix)
		case <-ticker.C:
			if scan() {
				return nil
			}
		}
	}
}
