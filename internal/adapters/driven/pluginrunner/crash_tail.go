package pluginrunner

import "sync"

// tailBuffer is a thread-safe ring buffer that retains the last maxSize bytes a
// plugin process wrote to stdout/stderr. On an abnormal exit the runner folds
// this tail into the admin-facing error, so the real cause is visible — e.g. a
// node "Error: Cannot find module '@grpc/grpc-js'" (plugin shipped without its
// node_modules) or "bwrap: Failed to make / slave: Permission denied" (seccomp
// profile not applied) — instead of a bare "exit status 1" / "socket timeout".
type tailBuffer struct {
	mu      sync.Mutex
	buf     []byte
	maxSize int
}

func newTailBuffer(maxSize int) *tailBuffer {
	return &tailBuffer{maxSize: maxSize}
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.maxSize {
		// Keep only the last maxSize bytes in a fresh backing array so the
		// larger one can be reclaimed.
		trimmed := make([]byte, t.maxSize)
		copy(trimmed, t.buf[len(t.buf)-t.maxSize:])
		t.buf = trimmed
	}
	return len(p), nil
}

// String returns the retained tail. Empty when the process produced no output.
func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
