package transcoding

import "sync"

// TranscodeThrottle tracks active direct-transcode streams per user.
type TranscodeThrottle struct {
	mu      sync.Mutex
	active  map[string]int
}

func NewTranscodeThrottle() *TranscodeThrottle {
	return &TranscodeThrottle{active: make(map[string]int)}
}

// Acquire returns true if the user is allowed another stream (max 0 = unlimited).
// On success the caller must call Release when done.
func (t *TranscodeThrottle) Acquire(userID string, max int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if max > 0 && t.active[userID] >= max {
		return false
	}
	t.active[userID]++
	return true
}

func (t *TranscodeThrottle) Release(userID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.active[userID] > 0 {
		t.active[userID]--
	}
}
