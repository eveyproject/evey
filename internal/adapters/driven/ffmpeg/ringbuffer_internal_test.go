package ffmpeg

import "testing"

// Verifies the ring buffer keeps the tail when overfilled — load-bearing
// for the ffmpeg stderr capture: a multi-MB progress stream must collapse
// to its last 8 KB so the fatal-error line survives in logs.
func TestRingBuffer_TailCapture(t *testing.T) {
	rb := newRingBuffer(10)
	rb.Write([]byte("hello"))
	if got := rb.String(); got != "hello" {
		t.Fatalf("partial: got %q want hello", got)
	}
	rb.Write([]byte("123456789012345"))
	if got := rb.String(); got != "6789012345" {
		t.Fatalf("overflow: got %q want 6789012345", got)
	}
}
