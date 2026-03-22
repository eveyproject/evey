package webpush

import (
	"context"
	"testing"

	"github.com/gravity-zero/evey/internal/domain/notification"
)

func TestSend_InvalidEndpoint(t *testing.T) {
	s := NewSender("BDummyPublicKey", "dummyPrivateKey", "mailto:test@localhost")
	sub := notification.PushSubscription{
		Endpoint: "http://invalid.endpoint.test/push",
		P256dh:   "AAAA",
		Auth:     "BBBB",
	}
	err := s.Send(context.Background(), sub, "title", "body", nil)
	if err == nil {
		t.Fatal("expected error for invalid subscription")
	}
}
