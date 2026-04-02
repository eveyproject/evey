package fakes

import "sync"

type FakeMessageSender struct {
	mu       sync.Mutex
	Messages [][]byte
	Err      error
}

func NewFakeMessageSender() *FakeMessageSender {
	return &FakeMessageSender{}
}

func (s *FakeMessageSender) SendMessage(data []byte) error {
	if s.Err != nil {
		return s.Err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	s.Messages = append(s.Messages, cp)
	return nil
}

func (s *FakeMessageSender) Received() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([][]byte, len(s.Messages))
	copy(cp, s.Messages)
	return cp
}
