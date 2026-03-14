package fakes

import (
	"context"
	"sync"
)

type PushCall struct {
	Tokens []string
	Title  string
	Body   string
	Data   map[string]string
}

type FakePushSender struct {
	mu    sync.Mutex
	Calls []PushCall
	Err   error
}

func NewFakePushSender() *FakePushSender {
	return &FakePushSender{}
}

func (s *FakePushSender) SendToTokens(_ context.Context, tokens []string, title, body string, data map[string]string) error {
	if s.Err != nil {
		return s.Err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls = append(s.Calls, PushCall{Tokens: tokens, Title: title, Body: body, Data: data})
	return nil
}
