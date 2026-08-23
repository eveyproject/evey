package ffmpeg

import (
	"bytes"
	"context"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Why this test exists (cast freeze investigation, 2026-08-23): the busy guard
// SIGSTOPs a background ffmpeg and writes NOTHING. Reading the logs during the
// incident, the absence of any trace was taken as proof that the guard had not
// suspended anything - and that conclusion was WRONG. An action nobody can see
// is an action nobody can rule out. One line per TRANSITION (not per poll, the
// loop ticks every second) is what makes "was it suspended, and for how long?"
// answerable from the admin log search alone.
func TestPauseProcessWhileBusy_LogsEverySuspendAndResume(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	restore := swapDefaultLogger(&buf, &mu)
	defer restore()

	prevPoll := detectionPausePoll
	detectionPausePoll = 5 * time.Millisecond
	defer func() { detectionPausePoll = prevPoll }()

	cmd := exec.CommandContext(context.Background(), "sleep", "10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	var busy atomic.Bool
	busy.Store(true)
	stop := pauseProcessWhileBusy(cmd.Process, busy.Load)

	waitFor(t, &buf, &mu, "suspended")
	busy.Store(false)
	waitFor(t, &buf, &mu, "resumed")
	stop()

	mu.Lock()
	out := buf.String()
	mu.Unlock()
	if n := strings.Count(out, "suspended"); n != 1 {
		t.Fatalf("want exactly one suspend line (one per transition, not per poll), got %d in:\n%s", n, out)
	}
	// The resume line must carry how long the process stayed stopped: that is
	// the number the incident needed and had to infer from gaps in unrelated
	// traces.
	if !strings.Contains(out, "paused_ms") {
		t.Fatalf("resume line must report the paused duration, got:\n%s", out)
	}
}

// A guard that never fires must stay silent: the loop ticks every second on
// every background decode, and a line per tick would drown the log view.
func TestPauseProcessWhileBusy_SilentWhenNeverBusy(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	restore := swapDefaultLogger(&buf, &mu)
	defer restore()

	prevPoll := detectionPausePoll
	detectionPausePoll = 5 * time.Millisecond
	defer func() { detectionPausePoll = prevPoll }()

	cmd := exec.CommandContext(context.Background(), "sleep", "10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	stop := pauseProcessWhileBusy(cmd.Process, func() bool { return false })
	time.Sleep(60 * time.Millisecond)
	stop()

	mu.Lock()
	out := buf.String()
	mu.Unlock()
	if out != "" {
		t.Fatalf("an idle guard must log nothing, got:\n%s", out)
	}
}

// lockedWriter serialises the handler's writes against the test's reads: the
// guard logs from its own goroutine.
type lockedWriter struct {
	mu  *sync.Mutex
	buf *bytes.Buffer
}

func (w lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func swapDefaultLogger(buf *bytes.Buffer, mu *sync.Mutex) func() {
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(lockedWriter{mu: mu, buf: buf}, nil)))
	return func() { slog.SetDefault(prev) }
}

func waitFor(t *testing.T, buf *bytes.Buffer, mu *sync.Mutex, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := buf.String()
		mu.Unlock()
		if strings.Contains(got, want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	got := buf.String()
	mu.Unlock()
	t.Fatalf("timed out waiting for %q in:\n%s", want, got)
}
