package webhook_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/app/webhook"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/shared/fakes"
	webhookdomain "github.com/gravity-zero/evey/internal/domain/webhook"
	"log/slog"
	"os"
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
	return webhook.NewService(repo, deliveries, log, bus)
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
