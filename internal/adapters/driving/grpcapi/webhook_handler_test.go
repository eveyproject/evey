package grpcapi

import (
	"context"
	"log/slog"
	"testing"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appwebhook "github.com/eveyproject/evey/internal/app/webhook"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/domain/webhook"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type noopSender struct{}

func (n *noopSender) Send(_ context.Context, _ string, _ map[string]string, _ []byte) (int, error) {
	return 200, nil
}

type fakeWebhookRepo struct {
	items map[string]*webhook.Config
}

func newFakeWebhookRepo() *fakeWebhookRepo {
	return &fakeWebhookRepo{items: make(map[string]*webhook.Config)}
}

func (r *fakeWebhookRepo) Create(_ context.Context, cfg *webhook.Config) error {
	r.items[cfg.ID] = cfg
	return nil
}

func (r *fakeWebhookRepo) GetByID(_ context.Context, id string) (*webhook.Config, error) {
	c, ok := r.items[id]
	if !ok {
		return nil, context.DeadlineExceeded
	}
	return c, nil
}

func (r *fakeWebhookRepo) List(_ context.Context) ([]*webhook.Config, error) {
	out := make([]*webhook.Config, 0, len(r.items))
	for _, c := range r.items {
		out = append(out, c)
	}
	return out, nil
}

func (r *fakeWebhookRepo) Update(_ context.Context, cfg *webhook.Config) error {
	// Mirrors the Postgres repository: an update that hits no row is NotFound.
	if _, ok := r.items[cfg.ID]; !ok {
		return shared.ErrNotFound
	}
	r.items[cfg.ID] = cfg
	return nil
}

func (r *fakeWebhookRepo) Delete(_ context.Context, id string) error {
	if _, ok := r.items[id]; !ok {
		return shared.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

type fakeDeliveryRepo struct {
	items []*webhook.Delivery
}

func newFakeDeliveryRepo() *fakeDeliveryRepo {
	return &fakeDeliveryRepo{}
}

func (r *fakeDeliveryRepo) Create(_ context.Context, d *webhook.Delivery) error {
	r.items = append(r.items, d)
	return nil
}

func (r *fakeDeliveryRepo) ListByWebhook(_ context.Context, webhookID string, limit int) ([]*webhook.Delivery, error) {
	var out []*webhook.Delivery
	for _, d := range r.items {
		if d.WebhookID == webhookID {
			out = append(out, d)
		}
	}
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

func setupWebhookHandler() (*webhookHandler, *fakeWebhookRepo, *fakeDeliveryRepo) {
	repo := newFakeWebhookRepo()
	deliveries := newFakeDeliveryRepo()
	events := sharedfakes.NewFakeEventBus()
	svc := appwebhook.NewService(repo, deliveries, &noopSender{}, slog.Default(), events)
	svc.SetURLValidator(func(string) error { return nil })

	return &webhookHandler{svc: svc}, repo, deliveries
}

func TestCreateWebhook_Success(t *testing.T) {
	h, _, _ := setupWebhookHandler()
	resp, err := h.CreateWebhook(context.Background(), &eveyv1.CreateWebhookRequest{
		Name:    "test-hook",
		Url:     "https://example.com/hook",
		Events:  []string{"*"},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}
	if resp.Name != "test-hook" {
		t.Fatalf("got name %q, want test-hook", resp.Name)
	}
	if resp.Id == "" {
		t.Fatal("expected non-empty ID")
	}
}

func TestCreateWebhook_InvalidConfig_InvalidArgument(t *testing.T) {
	h, _, _ := setupWebhookHandler()
	// No events subscribed → Config.Validate fails → must surface as
	// InvalidArgument, not a generic Internal error.
	_, err := h.CreateWebhook(context.Background(), &eveyv1.CreateWebhookRequest{
		Name:    "test-hook",
		Url:     "https://example.com/hook",
		Events:  nil,
		Enabled: true,
	})
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
	}
}

func TestListWebhooks_ReturnsItems(t *testing.T) {
	h, _, _ := setupWebhookHandler()
	_, _ = h.CreateWebhook(context.Background(), &eveyv1.CreateWebhookRequest{
		Name: "hook1", Url: "https://a.com", Events: []string{"*"}, Enabled: true,
	})
	_, _ = h.CreateWebhook(context.Background(), &eveyv1.CreateWebhookRequest{
		Name: "hook2", Url: "https://b.com", Events: []string{"*"}, Enabled: true,
	})

	resp, err := h.ListWebhooks(context.Background(), &eveyv1.ListWebhooksRequest{})
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}
	if len(resp.Webhooks) != 2 {
		t.Fatalf("got %d webhooks, want 2", len(resp.Webhooks))
	}
}

func TestUpdateWebhook_Success(t *testing.T) {
	h, repo, _ := setupWebhookHandler()
	created, _ := h.CreateWebhook(context.Background(), &eveyv1.CreateWebhookRequest{
		Name: "original", Url: "https://a.com", Events: []string{"*"}, Enabled: true,
	})

	resp, err := h.UpdateWebhook(context.Background(), &eveyv1.UpdateWebhookRequest{
		Id:      created.Id,
		Name:    "updated",
		Url:     "https://b.com",
		Events:  []string{"library.scanned"},
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("UpdateWebhook: %v", err)
	}
	if resp.Name != "updated" {
		t.Fatalf("got name %q, want updated", resp.Name)
	}
	if resp.Enabled {
		t.Fatal("expected disabled after update")
	}

	// resp is built straight from the request-derived cfg the handler just
	// constructed, so it round-trips even if the service never called
	// repo.Update. Read the repo directly to prove the write actually landed.
	stored, ok := repo.items[created.Id]
	if !ok {
		t.Fatal("expected webhook to still exist in repo after update")
	}
	if stored.Name != "updated" {
		t.Fatalf("repo has name %q, want updated", stored.Name)
	}
	if stored.URL != "https://b.com" {
		t.Fatalf("repo has url %q, want https://b.com", stored.URL)
	}
	if stored.Enabled {
		t.Fatal("repo should have the webhook disabled after update")
	}
}

func TestDeleteWebhook_Success(t *testing.T) {
	h, _, _ := setupWebhookHandler()
	created, _ := h.CreateWebhook(context.Background(), &eveyv1.CreateWebhookRequest{
		Name: "todelete", Url: "https://a.com", Events: []string{"*"}, Enabled: true,
	})

	_, err := h.DeleteWebhook(context.Background(), &eveyv1.DeleteWebhookRequest{Id: created.Id})
	if err != nil {
		t.Fatalf("DeleteWebhook: %v", err)
	}

	resp, _ := h.ListWebhooks(context.Background(), &eveyv1.ListWebhooksRequest{})
	if len(resp.Webhooks) != 0 {
		t.Fatalf("expected 0 webhooks after delete, got %d", len(resp.Webhooks))
	}
}

func TestListWebhookDeliveries_Success(t *testing.T) {
	h, _, deliveryRepo := setupWebhookHandler()
	created, _ := h.CreateWebhook(context.Background(), &eveyv1.CreateWebhookRequest{
		Name: "hook", Url: "https://a.com", Events: []string{"*"}, Enabled: true,
	})

	deliveryRepo.items = append(deliveryRepo.items, &webhook.Delivery{
		ID: "del-1", WebhookID: created.Id, Event: "library.scanned", StatusCode: 200,
	})

	resp, err := h.ListWebhookDeliveries(context.Background(), &eveyv1.ListWebhookDeliveriesRequest{
		WebhookId: created.Id,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("ListWebhookDeliveries: %v", err)
	}
	if len(resp.Deliveries) != 1 {
		t.Fatalf("got %d deliveries, want 1", len(resp.Deliveries))
	}
	if resp.Deliveries[0].Event != "library.scanned" {
		t.Fatalf("got event %q, want library.scanned", resp.Deliveries[0].Event)
	}
}

func (f *fakeDeliveryRepo) Purge(_ context.Context, _ time.Time) (int64, error) { return 0, nil }

// The signing secret never leaves the server whole: empty stays empty, a
// short one is fully masked, a long one keeps a 4-char hint.
func TestMaskSecret(t *testing.T) {
	for in, want := range map[string]string{"": "", "ab": "****", "abcd": "****", "abcde": "abcd****", "whsec_1234567890": "whse****"} {
		if got := maskSecret(in); got != want {
			t.Errorf("maskSecret(%q) = %q, want %q", in, got, want)
		}
	}
}

// Deleting a webhook that is already gone (a double click, a second admin
// tab) is NotFound, not a logged 500.
func TestWebhookHandler_DeleteUnknownIsNotFound(t *testing.T) {
	h, _, _ := setupWebhookHandler()
	_, err := h.DeleteWebhook(context.Background(), &eveyv1.DeleteWebhookRequest{Id: "ghost"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", status.Code(err))
	}
}

// Updating a webhook that was deleted from another tab is NotFound, not a
// logged 500 (the repository reports the missing row).
func TestWebhookHandler_UpdateUnknownIsNotFound(t *testing.T) {
	h, _, _ := setupWebhookHandler()
	_, err := h.UpdateWebhook(context.Background(), &eveyv1.UpdateWebhookRequest{
		Id: "ghost", Name: "x", Url: "https://a.com", Events: []string{"*"}, Enabled: true,
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", status.Code(err))
	}
}

// Custom headers carry the receiver's credentials, so ListWebhooks masks their
// values like the secret, and a UI that echoes the masks back keeps the real
// values stored.
func TestListWebhooks_MasksHeaderValues(t *testing.T) {
	h, repo, _ := setupWebhookHandler()
	ctx := context.Background()
	created, err := h.CreateWebhook(ctx, &eveyv1.CreateWebhookRequest{
		Name: "hook", Url: "https://example.com/hook", Events: []string{"*"}, Enabled: true,
		Headers: map[string]string{"Authorization": "Bearer receiver-token"},
	})
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}
	list, err := h.ListWebhooks(ctx, &eveyv1.ListWebhooksRequest{})
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}
	got := list.Webhooks[0].Headers["Authorization"]
	if got != "Bear****" {
		t.Fatalf("listed Authorization = %q, want the mask", got)
	}
	if _, err = h.UpdateWebhook(ctx, &eveyv1.UpdateWebhookRequest{
		Id: created.Id, Name: "hook", Url: "https://example.com/hook", Events: []string{"*"}, Enabled: false,
		Headers: list.Webhooks[0].Headers,
	}); err != nil {
		t.Fatalf("UpdateWebhook: %v", err)
	}
	stored, err := repo.GetByID(ctx, created.Id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.Headers["Authorization"] != "Bearer receiver-token" {
		t.Fatalf("stored Authorization = %q, the mask was written over it", stored.Headers["Authorization"])
	}
}
