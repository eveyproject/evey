package quickconnecthub

import "sync"

// Hub is an in-memory pub/sub that lets SSE handlers wait for a
// QuickConnect code to be authorized.
type Hub struct {
	mu   sync.Mutex
	subs map[string][]chan struct{}
}

func New() *Hub {
	return &Hub{subs: make(map[string][]chan struct{})}
}

// Subscribe returns a channel that is closed when code is authorized.
// The caller must call Unsubscribe when done.
func (h *Hub) Subscribe(code string) chan struct{} {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.subs[code] = append(h.subs[code], ch)
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes ch from the subscribers for code.
func (h *Hub) Unsubscribe(code string, ch chan struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	list := h.subs[code]
	for i, c := range list {
		if c == ch {
			h.subs[code] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(h.subs[code]) == 0 {
		delete(h.subs, code)
	}
}

// Notify closes all subscriber channels for code, waking up SSE handlers.
func (h *Hub) Notify(code string) {
	h.mu.Lock()
	list := h.subs[code]
	delete(h.subs, code)
	h.mu.Unlock()
	for _, ch := range list {
		close(ch)
	}
}
