package webhook_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/webhooksender"
	"github.com/evey-project/evey/internal/app/webhook"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	webhookdomain "github.com/evey-project/evey/internal/domain/webhook"
	webhookfakes "github.com/evey-project/evey/internal/domain/webhook/fakes"
)

// --- fakes ---

type fakeWebhookRepo struct {
	mu   sync.Mutex
	data []*webhookdomain.Config
}

func (r *fakeWebhookRepo) Create(_ context.Context, cfg *webhookdomain.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = append(r.data, cfg)
	return nil
}

func (r *fakeWebhookRepo) GetByID(_ context.Context, id string) (*webhookdomain.Config, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.data {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, nil
}

func (r *fakeWebhookRepo) List(_ context.Context) ([]*webhookdomain.Config, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]*webhookdomain.Config, len(r.data))
	copy(cp, r.data)
	return cp, nil
}

func (r *fakeWebhookRepo) Update(_ context.Context, cfg *webhookdomain.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, c := range r.data {
		if c.ID == cfg.ID {
			r.data[i] = cfg
			return nil
		}
	}
	return nil
}

func (r *fakeWebhookRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, c := range r.data {
		if c.ID == id {
			r.data = append(r.data[:i], r.data[i+1:]...)
			return nil
		}
	}
	return nil
}

type fakeDeliveryRepo struct {
	mu   sync.Mutex
	data []*webhookdomain.Delivery
}

func (r *fakeDeliveryRepo) Create(_ context.Context, d *webhookdomain.Delivery) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = append(r.data, d)
	return nil
}

func (r *fakeDeliveryRepo) ListByWebhook(_ context.Context, webhookID string, limit int) ([]*webhookdomain.Delivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []*webhookdomain.Delivery
	for _, d := range r.data {
		if d.WebhookID == webhookID {
			result = append(result, d)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

type fakeEvent struct{ name string }

func (e fakeEvent) EventName() string { return e.name }

// programmableSender allows per-call response control for retry-style tests.
// Kept local to avoid modifying the shared fake (which is static StatusCode/Err).
type programmableSender struct {
	mu        sync.Mutex
	calls     []webhookfakes.SenderCall
	responder func(attempt int) (int, error)
	preEnter  func() // optional: invoked before gate wait (for concurrency counting)
	attempt   int
	gate      chan struct{} // optional: blocks Send until released
}

func (p *programmableSender) Send(_ context.Context, url string, headers map[string]string, body []byte) (int, error) {
	p.mu.Lock()
	p.calls = append(p.calls, webhookfakes.SenderCall{URL: url, Headers: headers, Body: body})
	a := p.attempt
	p.attempt++
	gate := p.gate
	p.mu.Unlock()
	if p.preEnter != nil {
		p.preEnter()
	}
	if gate != nil {
		<-gate
	}
	if p.responder != nil {
		return p.responder(a)
	}
	return 200, nil
}

func (p *programmableSender) Calls() []webhookfakes.SenderCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]webhookfakes.SenderCall, len(p.calls))
	copy(cp, p.calls)
	return cp
}

// newTestService builds a Service wired with a programmableSender (no HTTP).
// Returns the sender for inspection of recorded calls.
func newTestService(repo webhookdomain.Repository, deliveries webhookdomain.DeliveryRepository, bus shared.EventBus) (*webhook.Service, *programmableSender) {
	sender := &programmableSender{}
	svc := newTestServiceWithSender(repo, deliveries, bus, sender)
	return svc, sender
}

func newTestServiceWithSender(repo webhookdomain.Repository, deliveries webhookdomain.DeliveryRepository, bus shared.EventBus, sender webhookdomain.Sender) *webhook.Service {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := webhook.NewService(repo, deliveries, sender, log, bus)
	svc.SetURLValidator(func(string) error { return nil })
	return svc
}

// waitForCalls polls until sender has at least n recorded calls or deadline.
func waitForCalls(t *testing.T, sender *programmableSender, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(sender.Calls()) >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %d sender calls, did not arrive in time", n)
}

// waitForDeliveries polls until delivery repo has at least n records or deadline.
func waitForDeliveries(t *testing.T, deliveries *fakeDeliveryRepo, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		deliveries.mu.Lock()
		got := len(deliveries.data)
		deliveries.mu.Unlock()
		if got >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %d deliveries, did not arrive in time", n)
}

// --- tests ---

func TestCreate(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	svc, _ := newTestService(repo, deliveries, bus)

	cfg := &webhookdomain.Config{
		Name:    "test",
		URL:     "http://example.com",
		Events:  []string{"media.item.added"},
		Enabled: true,
	}
	if err := svc.Create(context.Background(), cfg); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if cfg.ID == "" {
		t.Fatal("ID not set")
	}
	if cfg.CreatedAt.IsZero() {
		t.Fatal("CreatedAt not set")
	}
}

func TestList(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	svc, _ := newTestService(repo, deliveries, bus)

	ctx := context.Background()
	_ = svc.Create(ctx, &webhookdomain.Config{Name: "a", URL: "http://a.com", Events: []string{"*"}})
	_ = svc.Create(ctx, &webhookdomain.Config{Name: "b", URL: "http://b.com", Events: []string{"*"}})

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
}

func TestDelete(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	svc, _ := newTestService(repo, deliveries, bus)

	ctx := context.Background()
	cfg := &webhookdomain.Config{Name: "x", URL: "http://x.com", Events: []string{"*"}}
	_ = svc.Create(ctx, cfg)

	if err := svc.Delete(ctx, cfg.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list, _ := svc.List(ctx)
	if len(list) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(list))
	}
}

func TestListDeliveries(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	svc, _ := newTestService(repo, deliveries, bus)

	ctx := context.Background()
	_ = deliveries.Create(ctx, &webhookdomain.Delivery{
		ID:        "d1",
		WebhookID: "w1",
		Event:     "media.item.added",
		SentAt:    time.Now(),
	})

	list, err := svc.ListDeliveries(ctx, "w1", 10)
	if err != nil {
		t.Fatalf("ListDeliveries: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}
	if list[0].ID != "d1" {
		t.Fatalf("unexpected delivery ID: %s", list[0].ID)
	}
}

func TestCreate_BlocksPrivateURL(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := webhook.NewService(repo, deliveries, webhookfakes.NewFakeSender(), log, bus)

	cfg := &webhookdomain.Config{
		Name:    "ssrf-test",
		URL:     "http://169.254.169.254/latest/meta-data/",
		Events:  []string{"*"},
		Enabled: true,
	}
	err := svc.Create(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for private IP URL")
	}
}

func TestHandleAny_DeliverToMatchingWebhook(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	svc, sender := newTestService(repo, deliveries, bus)

	ctx := context.Background()
	cfg := &webhookdomain.Config{
		Name:    "test-hook",
		URL:     "http://hook.example",
		Events:  []string{"media.item.added"},
		Enabled: true,
	}
	if err := svc.Create(ctx, cfg); err != nil {
		t.Fatalf("Create: %v", err)
	}

	_ = bus.Publish(ctx, fakeEvent{"media.item.added"})

	waitForCalls(t, sender, 1, 2*time.Second)
	waitForDeliveries(t, deliveries, 1, 2*time.Second)
}

func TestHandleAny_ConcurrencyBounded(t *testing.T) {
	t.Parallel()
	var concurrent atomic.Int32
	var maxSeen atomic.Int32
	gate := make(chan struct{})

	// preEnter increments concurrency BEFORE blocking on gate (gate hold simulates inflight work).
	sender := &programmableSender{
		gate: gate,
		preEnter: func() {
			cur := concurrent.Add(1)
			for {
				old := maxSeen.Load()
				if cur <= old || maxSeen.CompareAndSwap(old, cur) {
					break
				}
			}
		},
		responder: func(_ int) (int, error) {
			concurrent.Add(-1)
			return 200, nil
		},
	}

	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	_ = newTestServiceWithSender(repo, deliveries, bus, sender)

	ctx := context.Background()
	for i := range 15 {
		repo.mu.Lock()
		repo.data = append(repo.data, &webhookdomain.Config{
			ID:      fmt.Sprintf("w%d", i),
			URL:     "http://hook.example",
			Events:  []string{"*"},
			Enabled: true,
		})
		repo.mu.Unlock()
	}

	_ = bus.Publish(ctx, fakeEvent{"test.event"})
	time.Sleep(100 * time.Millisecond)

	if maxSeen.Load() > 10 {
		t.Fatalf("concurrent deliveries exceeded limit: %d > 10", maxSeen.Load())
	}

	close(gate)
	time.Sleep(200 * time.Millisecond)
}

func TestHandleAny_SkipsDisabledWebhook(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	_, sender := newTestService(repo, deliveries, bus)

	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "disabled", URL: "http://hook.example", Events: []string{"*"}, Enabled: false,
	})
	repo.mu.Unlock()

	_ = bus.Publish(context.Background(), fakeEvent{"test.event"})
	time.Sleep(100 * time.Millisecond)

	if n := len(sender.Calls()); n != 0 {
		t.Fatal("disabled webhook should not be called")
	}
}

func TestHandleAny_SkipsNonMatchingEvent(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	_, sender := newTestService(repo, deliveries, bus)

	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "specific", URL: "http://hook.example", Events: []string{"media.item.added"}, Enabled: true,
	})
	repo.mu.Unlock()

	_ = bus.Publish(context.Background(), fakeEvent{"user.created"})
	time.Sleep(100 * time.Millisecond)

	if n := len(sender.Calls()); n != 0 {
		t.Fatal("non-matching event should not trigger delivery")
	}
}

func TestHandleAny_WildcardEventMatch(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	_, sender := newTestService(repo, deliveries, bus)

	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "wildcard", URL: "http://hook.example", Events: []string{"*"}, Enabled: true,
	})
	repo.mu.Unlock()

	_ = bus.Publish(context.Background(), fakeEvent{"anything.goes"})
	waitForCalls(t, sender, 1, 2*time.Second)
}

func TestUpdate(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	svc, _ := newTestService(repo, deliveries, bus)

	ctx := context.Background()
	cfg := &webhookdomain.Config{Name: "orig", URL: "http://orig.com", Events: []string{"*"}, Enabled: true}
	_ = svc.Create(ctx, cfg)

	cfg.Name = "updated"
	if err := svc.Update(ctx, cfg); err != nil {
		t.Fatalf("Update: %v", err)
	}

	list, _ := svc.List(ctx)
	if len(list) != 1 || list[0].Name != "updated" {
		t.Fatalf("expected updated name, got %v", list)
	}
}

func TestHandleAny_DeliveryRecordsError(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	sender := &programmableSender{responder: func(_ int) (int, error) { return http.StatusInternalServerError, nil }}
	svc := newTestServiceWithSender(repo, deliveries, bus, sender)
	svc.SetRetryDelays([]time.Duration{1 * time.Millisecond})

	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "fail", URL: "http://hook.example", Events: []string{"*"}, Enabled: true,
	})
	repo.mu.Unlock()

	_ = bus.Publish(context.Background(), fakeEvent{"test.event"})
	waitForDeliveries(t, deliveries, 1, 2*time.Second)

	deliveries.mu.Lock()
	d := deliveries.data[0]
	deliveries.mu.Unlock()
	if d.Error == "" {
		t.Fatal("expected error in delivery record")
	}
	if d.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", d.StatusCode)
	}
}

// TestHandleAny_RetriesOnFailureThenSucceeds uses programmableSender (per-call response).
// Pure unit test — no HTTP.
func TestHandleAny_RetriesOnFailureThenSucceeds(t *testing.T) {
	t.Parallel()
	sender := &programmableSender{
		responder: func(attempt int) (int, error) {
			if attempt == 0 {
				return http.StatusServiceUnavailable, nil
			}
			return http.StatusOK, nil
		},
	}

	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	svc := newTestServiceWithSender(repo, deliveries, bus, sender)
	svc.SetRetryDelays([]time.Duration{1 * time.Millisecond})

	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "retry", URL: "http://hook.example", Events: []string{"*"}, Enabled: true,
	})
	repo.mu.Unlock()

	_ = bus.Publish(context.Background(), fakeEvent{"test.event"})
	waitForDeliveries(t, deliveries, 1, 2*time.Second)

	if got := len(sender.Calls()); got < 2 {
		t.Fatalf("expected at least 2 attempts, got %d", got)
	}
	deliveries.mu.Lock()
	d := deliveries.data[0]
	deliveries.mu.Unlock()
	if d.Error != "" {
		t.Fatalf("expected no error after success, got %q", d.Error)
	}
	if d.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", d.StatusCode)
	}
}

func TestHandleAny_DeliveryIncludesSignatureHeader(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	_, sender := newTestService(repo, deliveries, bus)

	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "sig", URL: "http://hook.example", Events: []string{"*"}, Enabled: true, Secret: "mysecret",
	})
	repo.mu.Unlock()

	_ = bus.Publish(context.Background(), fakeEvent{"test.event"})
	waitForCalls(t, sender, 1, 2*time.Second)

	gotSig := sender.Calls()[0].Headers["X-Evey-Signature"]
	if gotSig == "" {
		t.Fatal("X-Evey-Signature header not sent")
	}
	if len(gotSig) < 7 || gotSig[:7] != "sha256=" {
		t.Fatalf("unexpected signature format: %q", gotSig)
	}
}

func TestUpdate_BlocksPrivateURL(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := webhook.NewService(repo, deliveries, webhookfakes.NewFakeSender(), log, bus)

	cfg := &webhookdomain.Config{
		ID: "w1", Name: "x", URL: "http://169.254.169.254/meta-data/", Events: []string{"*"}, Enabled: true,
	}
	err := svc.Update(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for private IP URL on Update")
	}
}

func TestHandleAny_CustomHeaders(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	_, sender := newTestService(repo, deliveries, bus)

	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "custom", URL: "http://hook.example", Events: []string{"*"}, Enabled: true,
		Headers: map[string]string{"X-Custom": "hello"},
	})
	repo.mu.Unlock()

	_ = bus.Publish(context.Background(), fakeEvent{"test.event"})
	waitForCalls(t, sender, 1, 2*time.Second)

	gotHeader := sender.Calls()[0].Headers["X-Custom"]
	if gotHeader != "hello" {
		t.Fatalf("expected X-Custom=hello, got %q", gotHeader)
	}
}

func TestHandleAny_PayloadContainsEventAndData(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	_, sender := newTestService(repo, deliveries, bus)

	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "payload-check", URL: "http://hook.example", Events: []string{"*"}, Enabled: true,
	})
	repo.mu.Unlock()

	_ = bus.Publish(context.Background(), fakeEvent{"media.item.added"})
	waitForCalls(t, sender, 1, 2*time.Second)

	body := sender.Calls()[0].Body

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	ev, ok := payload["event"]
	if !ok {
		t.Fatal("payload missing 'event' key")
	}
	if ev != "media.item.added" {
		t.Fatalf("expected event=media.item.added, got %v", ev)
	}
	if _, ok = payload["payload"]; !ok {
		t.Fatal("payload missing 'payload' key")
	}
}

func TestHandleAny_Status299IsSuccess(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	sender := &programmableSender{responder: func(_ int) (int, error) { return 299, nil }}
	svc := newTestServiceWithSender(repo, deliveries, bus, sender)
	svc.SetRetryDelays([]time.Duration{1 * time.Millisecond})

	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "s299", URL: "http://hook.example", Events: []string{"*"}, Enabled: true,
	})
	repo.mu.Unlock()

	_ = bus.Publish(context.Background(), fakeEvent{"test.event"})
	waitForDeliveries(t, deliveries, 1, 2*time.Second)

	if n := len(sender.Calls()); n != 1 {
		t.Fatalf("status 299 should succeed on first attempt, got %d attempts", n)
	}

	deliveries.mu.Lock()
	d := deliveries.data[0]
	deliveries.mu.Unlock()
	if d.Error != "" {
		t.Fatalf("expected no error for 299, got %q", d.Error)
	}
	if d.StatusCode != 299 {
		t.Fatalf("expected status 299, got %d", d.StatusCode)
	}
}

func TestHandleAny_Status300IsFailure(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	sender := &programmableSender{responder: func(_ int) (int, error) { return 300, nil }}
	svc := newTestServiceWithSender(repo, deliveries, bus, sender)
	svc.SetRetryDelays([]time.Duration{1 * time.Millisecond})

	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "s300", URL: "http://hook.example", Events: []string{"*"}, Enabled: true,
	})
	repo.mu.Unlock()

	_ = bus.Publish(context.Background(), fakeEvent{"test.event"})
	waitForDeliveries(t, deliveries, 1, 2*time.Second)

	if n := len(sender.Calls()); n < 2 {
		t.Fatalf("status 300 should trigger retries, got %d attempts", n)
	}

	deliveries.mu.Lock()
	d := deliveries.data[0]
	deliveries.mu.Unlock()
	if d.Error == "" {
		t.Fatal("expected error for status 300")
	}
	if d.StatusCode != 300 {
		t.Fatalf("expected status 300, got %d", d.StatusCode)
	}
}

// TestHandleAny_HTTPErrorRecordsDelivery verifies a transport-level error
// (Sender returns err) produces a delivery record with non-empty Error and
// StatusCode 0. Uses programmableSender (no HTTP).
func TestHandleAny_HTTPErrorRecordsDelivery(t *testing.T) {
	t.Parallel()
	sender := &programmableSender{
		responder: func(_ int) (int, error) {
			return 0, errors.New("connection refused")
		},
	}

	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	svc := newTestServiceWithSender(repo, deliveries, bus, sender)
	svc.SetRetryDelays([]time.Duration{1 * time.Millisecond})

	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "http-err", URL: "http://hook.example", Events: []string{"*"}, Enabled: true,
	})
	repo.mu.Unlock()

	_ = bus.Publish(context.Background(), fakeEvent{"test.event"})
	waitForDeliveries(t, deliveries, 1, 2*time.Second)

	deliveries.mu.Lock()
	d := deliveries.data[0]
	deliveries.mu.Unlock()
	if d.Error == "" {
		t.Fatal("expected error string in delivery record")
	}
	if d.StatusCode != 0 {
		t.Fatalf("expected status 0 (no response), got %d", d.StatusCode)
	}
}

// --- HTTP integration smoke test (kept for adapter wiring sanity) ---
// Renamed with _Integration suffix to distinguish from pure unit tests above.
// Uses real webhooksender + httptest.Server end-to-end.

func TestHandleAny_RealSenderIntegration(t *testing.T) {
	t.Parallel()
	received := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	// Inject a plain client — httptest.Server binds 127.0.0.1 which the
	// default SSRF-protected client (webhooksender.New) would reject.
	svc := webhook.NewService(repo, deliveries, webhooksender.NewWithClient(&http.Client{Timeout: 5 * time.Second}), log, bus)
	svc.SetURLValidator(func(string) error { return nil })

	cfg := &webhookdomain.Config{
		Name: "smoke", URL: srv.URL, Events: []string{"*"}, Enabled: true,
	}
	if err := svc.Create(context.Background(), cfg); err != nil {
		t.Fatalf("Create: %v", err)
	}

	_ = bus.Publish(context.Background(), fakeEvent{"smoke.event"})

	select {
	case <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("real sender did not deliver to test server")
	}
}
