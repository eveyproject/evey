package fakes

import (
	"context"
	"sync"
)

type FakeSender struct {
	mu         sync.Mutex
	StatusCode int
	Err        error
	Calls      []SenderCall
}

type SenderCall struct {
	URL     string
	Headers map[string]string
	Body    []byte
}

func NewFakeSender() *FakeSender {
	return &FakeSender{StatusCode: 200}
}

func (s *FakeSender) Send(_ context.Context, url string, headers map[string]string, body []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls = append(s.Calls, SenderCall{URL: url, Headers: headers, Body: body})
	return s.StatusCode, s.Err
}
