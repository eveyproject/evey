package webhook_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	webhookdomain "github.com/eveyproject/evey/internal/domain/webhook"
)

// A configured header named like the signature or the delivery id, in any
// case, must not reach the receiver in their place: with an exact name it
// replaced the HMAC outright, with another case the real sender kept whichever
// it set last in random map order.
func TestDeliver_ConfiguredHeadersCannotReplaceSignatureOrDeliveryID(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	svc, sender := newTestService(repo, &fakeDeliveryRepo{}, sharedfakes.NewFakeEventBus())
	ctx := context.Background()
	cfg := &webhookdomain.Config{Name: "n", URL: "http://hook.example", Events: []string{"*"}, Enabled: true, Secret: "s3cret",
		Headers: map[string]string{"X-Evey-Signature": "forged", "x-evey-signature": "forged", "X-EVEY-DELIVERY": "fixed", "X-Custom": "kept"}}
	if err := svc.Create(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Test(ctx, cfg.ID); err != nil {
		t.Fatal(err)
	}

	call := sender.Calls()[0]
	mac := hmac.New(sha256.New, []byte("s3cret"))
	mac.Write(call.Body)
	want := fmt.Sprintf("sha256=%x", mac.Sum(nil))
	for k, v := range call.Headers {
		switch http.CanonicalHeaderKey(k) {
		case "X-Evey-Signature":
			if k != "X-Evey-Signature" || v != want {
				t.Errorf("signature header %q=%q, want only X-Evey-Signature=%q", k, v, want)
			}
		case "X-Evey-Delivery":
			if k != "X-Evey-Delivery" || v == "fixed" {
				t.Errorf("delivery header %q=%q, want only the minted X-Evey-Delivery", k, v)
			}
		}
	}
	if call.Headers["X-Custom"] != "kept" {
		t.Errorf("X-Custom = %q, want the configured value", call.Headers["X-Custom"])
	}
}

// With every delivery slot busy the event is dropped, and the drop must show
// in the delivery log the admin reads, not only in the server log.
func TestHandleAny_DroppedDeliveryIsRecorded(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	svc, sender := newTestService(repo, deliveries, bus)
	gate := make(chan struct{})
	sender.gate = gate
	ctx := context.Background()
	cfg := &webhookdomain.Config{Name: "n", URL: "http://hook.example", Events: []string{"*"}, Enabled: true}
	if err := svc.Create(ctx, cfg); err != nil {
		t.Fatal(err)
	}

	for range 11 {
		_ = bus.Publish(ctx, fakeEvent{"test.event"})
	}
	waitForCalls(t, sender, 10, 2*time.Second)
	waitForDeliveries(t, deliveries, 1, 2*time.Second)
	close(gate)
	waitForDeliveries(t, deliveries, 11, 2*time.Second)

	deliveries.mu.Lock()
	defer deliveries.mu.Unlock()
	dropped := 0
	for _, d := range deliveries.data {
		if strings.Contains(d.Error, "queue full") {
			dropped++
			if d.WebhookID != cfg.ID || d.Event != "test.event" {
				t.Errorf("dropped record = %+v, want webhook %s event test.event", d, cfg.ID)
			}
		}
	}
	if dropped != 1 {
		t.Fatalf("dropped records = %d, want 1", dropped)
	}
}
