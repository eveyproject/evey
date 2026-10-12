package fakes

import (
	"context"
	"sync"

	"github.com/eveyproject/evey/internal/domain/notification"
)

type PushCall struct {
	Sub   notification.PushSubscription
	Title string
	Body  string
	Data  map[string]string
}

type FakePushSender struct {
	mu    sync.Mutex
	Calls []PushCall
	Err   error
}

func NewFakePushSender() *FakePushSender {
	return &FakePushSender{}
}

func (s *FakePushSender) Send(_ context.Context, sub notification.PushSubscription, title, body string, data map[string]string) error {
	if s.Err != nil {
		return s.Err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls = append(s.Calls, PushCall{Sub: sub, Title: title, Body: body, Data: data})
	return nil
}
