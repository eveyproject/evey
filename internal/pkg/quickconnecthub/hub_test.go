package quickconnecthub

import (
	"testing"
	"time"
)

func TestSubscribeAndNotify(t *testing.T) {
	h := New()
	ch := h.Subscribe("ABC123")

	// Notify synchronously from another goroutine - no fixed delay needed.
	// The subscriber channel is buffered; Notify just closes it, so the order
	// between Subscribe and Notify is what matters and is already established.
	go h.Notify("ABC123")

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("expected notification")
	}
}

func TestNotifyWithoutSubscribers(t *testing.T) {
	h := New()
	h.Notify("NONE")
}

func TestUnsubscribe(t *testing.T) {
	h := New()
	ch := h.Subscribe("CODE1")
	h.Unsubscribe("CODE1", ch)

	h.mu.Lock()
	remaining := len(h.subs["CODE1"])
	h.mu.Unlock()

	if remaining != 0 {
		t.Fatalf("expected 0 subscribers, got %d", remaining)
	}
}

func TestUnsubscribeCleansUpEmptySlice(t *testing.T) {
	h := New()
	ch := h.Subscribe("CODE2")
	h.Unsubscribe("CODE2", ch)

	h.mu.Lock()
	_, exists := h.subs["CODE2"]
	h.mu.Unlock()

	if exists {
		t.Fatal("expected key to be deleted")
	}
}

func TestMultipleSubscribers(t *testing.T) {
	h := New()
	ch1 := h.Subscribe("MULTI")
	ch2 := h.Subscribe("MULTI")

	h.Notify("MULTI")

	select {
	case <-ch1:
	case <-time.After(time.Second):
		t.Fatal("ch1 not notified")
	}
	select {
	case <-ch2:
	case <-time.After(time.Second):
		t.Fatal("ch2 not notified")
	}
}

func TestUnsubscribeNonexistent(t *testing.T) {
	h := New()
	ch := make(chan struct{})
	h.Unsubscribe("NOPE", ch)
}
