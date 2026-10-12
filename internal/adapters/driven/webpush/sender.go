package webpush

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	wp "github.com/SherClockHolmes/webpush-go"
	"github.com/eveyproject/evey/internal/domain/notification"
	"github.com/eveyproject/evey/internal/pkg/ssrf"
)

type Sender struct {
	vapidPublicKey  string
	vapidPrivateKey string
	vapidSubject    string
	httpClient      wp.HTTPClient
}

func NewSender(publicKey, privateKey, subject string) *Sender {
	return &Sender{
		vapidPublicKey:  publicKey,
		vapidPrivateKey: privateKey,
		vapidSubject:    subject,
		// The push endpoint URL is supplied by the client when it registers a
		// subscription, so delivery must go through the SSRF-safe client: it
		// blocks dialing private/reserved IPs, preventing a malicious client
		// from registering endpoint=http://169.254.169.254/... and turning the
		// server into an internal-network request proxy.
		httpClient: ssrf.SafeHTTPClient(30 * time.Second),
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

	opts := &wp.Options{
		VAPIDPublicKey:  s.vapidPublicKey,
		VAPIDPrivateKey: s.vapidPrivateKey,
		Subscriber:      s.vapidSubject,
	}
	if s.httpClient != nil {
		opts.HTTPClient = s.httpClient
	}
	resp, err := wp.SendNotification(raw, wpSub, opts)
	if err != nil {
		return fmt.Errorf("webpush send: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusGone {
		return notification.ErrSubscriptionGone
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webpush send: status %d", resp.StatusCode)
	}
	return nil
}
