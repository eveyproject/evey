package webhook_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/app/webhook"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/shared/fakes"
	webhookdomain "github.com/gravity-zero/evey/internal/domain/webhook"
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

func newTestService(repo webhookdomain.Repository, deliveries webhookdomain.DeliveryRepository, bus shared.EventBus) *webhook.Service {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := webhook.NewService(repo, deliveries, log, bus)
	// Bypass SSRF protection in tests (test server runs on localhost)
	svc.SetURLValidator(func(string) error { return nil })
	return svc
}

// --- tests ---

func TestCreate(t *testing.T) {
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := fakes.NewFakeEventBus()
	svc := newTestService(repo, deliveries, bus)

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
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := fakes.NewFakeEventBus()
	svc := newTestService(repo, deliveries, bus)

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
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := fakes.NewFakeEventBus()
	svc := newTestService(repo, deliveries, bus)

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
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := fakes.NewFakeEventBus()
	svc := newTestService(repo, deliveries, bus)

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
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := fakes.NewFakeEventBus()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := webhook.NewService(repo, deliveries, log, bus)

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
	received := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := fakes.NewFakeEventBus()
	svc := newTestService(repo, deliveries, bus)

	ctx := context.Background()
	cfg := &webhookdomain.Config{
		Name:    "test-hook",
		URL:     srv.URL,
		Events:  []string{"media.item.added"},
		Enabled: true,
	}
	if err := svc.Create(ctx, cfg); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// FakeEventBus dispatches synchronously, but deliver runs in a goroutine.
	_ = bus.Publish(ctx, fakeEvent{"media.item.added"})

	select {
	case <-received:
		// delivered
	case <-time.After(2 * time.Second):
		t.Fatal("webhook not delivered in time")
	}

	// also verify a delivery record was written
	time.Sleep(50 * time.Millisecond)
	deliveries.mu.Lock()
	n := len(deliveries.data)
	deliveries.mu.Unlock()
	if n == 0 {
		t.Fatal("delivery record not written")
	}
}

func TestHandleAny_ConcurrencyBounded(t *testing.T) {
	var concurrent atomic.Int32
	var maxSeen atomic.Int32
	gate := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := concurrent.Add(1)
		for {
			old := maxSeen.Load()
			if cur <= old || maxSeen.CompareAndSwap(old, cur) {
				break
			}
		}
		<-gate
		concurrent.Add(-1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := fakes.NewFakeEventBus()
	_ = newTestService(repo, deliveries, bus)

	ctx := context.Background()
	// create 15 enabled webhooks (more than maxConcurrentDeliveries=10)
	for i := 0; i < 15; i++ {
		repo.mu.Lock()
		repo.data = append(repo.data, &webhookdomain.Config{
			ID:      fmt.Sprintf("w%d", i),
			URL:     srv.URL,
			Events:  []string{"*"},
			Enabled: true,
		})
		repo.mu.Unlock()
	}

	_ = bus.Publish(ctx, fakeEvent{"test.event"})

	// let goroutines start
	time.Sleep(100 * time.Millisecond)

	if maxSeen.Load() > 10 {
		t.Fatalf("concurrent deliveries exceeded limit: %d > 10", maxSeen.Load())
	}

	close(gate)
	time.Sleep(200 * time.Millisecond)
}

func TestHandleAny_SkipsDisabledWebhook(t *testing.T) {
	var called atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := fakes.NewFakeEventBus()
	_ = newTestService(repo, deliveries, bus)

	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "disabled", URL: srv.URL, Events: []string{"*"}, Enabled: false,
	})
	repo.mu.Unlock()

	_ = bus.Publish(context.Background(), fakeEvent{"test.event"})
	time.Sleep(100 * time.Millisecond)

	if called.Load() != 0 {
		t.Fatal("disabled webhook should not be called")
	}
}

func TestHandleAny_SkipsNonMatchingEvent(t *testing.T) {
	var called atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := fakes.NewFakeEventBus()
	_ = newTestService(repo, deliveries, bus)

	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "specific", URL: srv.URL, Events: []string{"media.item.added"}, Enabled: true,
	})
	repo.mu.Unlock()

	_ = bus.Publish(context.Background(), fakeEvent{"user.created"})
	time.Sleep(100 * time.Millisecond)

	if called.Load() != 0 {
		t.Fatal("non-matching event should not trigger delivery")
	}
}

func TestHandleAny_WildcardEventMatch(t *testing.T) {
	received := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := fakes.NewFakeEventBus()
	_ = newTestService(repo, deliveries, bus)

	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "wildcard", URL: srv.URL, Events: []string{"*"}, Enabled: true,
	})
	repo.mu.Unlock()

	_ = bus.Publish(context.Background(), fakeEvent{"anything.goes"})

	select {
	case <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("wildcard webhook not delivered")
	}
}

func TestUpdate(t *testing.T) {
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := fakes.NewFakeEventBus()
	svc := newTestService(repo, deliveries, bus)

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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := fakes.NewFakeEventBus()
	svc := newTestService(repo, deliveries, bus)
	svc.SetRetryDelays([]time.Duration{10 * time.Millisecond})

	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "fail", URL: srv.URL, Events: []string{"*"}, Enabled: true,
	})
	repo.mu.Unlock()

	_ = bus.Publish(context.Background(), fakeEvent{"test.event"})

	deadline := time.After(5 * time.Second)
	for {
		deliveries.mu.Lock()
		n := len(deliveries.data)
		deliveries.mu.Unlock()
		if n > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("delivery record not written after retries")
		case <-time.After(50 * time.Millisecond):
		}
	}

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
