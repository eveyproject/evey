package webpush

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	wp "github.com/SherClockHolmes/webpush-go"
	"github.com/gravity-zero/evey/internal/domain/notification"
)

type Sender struct {
	vapidPublicKey  string
	vapidPrivateKey string
	vapidSubject    string
}

func NewSender(publicKey, privateKey, subject string) *Sender {
	return &Sender{
		vapidPublicKey:  publicKey,
		vapidPrivateKey: privateKey,
		vapidSubject:    subject,
	}
}

func (s *Sender) Send(_ context.Context, sub notification.PushSubscription, title, body string, data map[string]string) error {
	payload := map[string]any{
		"title": title,
		"body":  body,
	}
	if len(data) > 0 {
		payload["data"] = data
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	wpSub := &wp.Subscription{
		Endpoint: sub.Endpoint,
		Keys: wp.Keys{
			P256dh: sub.P256dh,
			Auth:   sub.Auth,
		},
	}

	resp, err := wp.SendNotification(raw, wpSub, &wp.Options{
		VAPIDPublicKey:  s.vapidPublicKey,
		VAPIDPrivateKey: s.vapidPrivateKey,
		Subscriber:      s.vapidSubject,
	})
	if err != nil {
		return fmt.Errorf("webpush send: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusGone {
		return fmt.Errorf("subscription expired (410 Gone)")
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webpush send: status %d", resp.StatusCode)
	}
	return nil
}
