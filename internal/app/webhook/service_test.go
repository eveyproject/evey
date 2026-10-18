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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/webhooksender"
	"github.com/eveyproject/evey/internal/app/webhook"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	webhookdomain "github.com/eveyproject/evey/internal/domain/webhook"
	webhookfakes "github.com/eveyproject/evey/internal/domain/webhook/fakes"
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
	// Matches the contract every other Repository implementation (postgres,
	// domain/webhook/fakes) honors: a missing id is shared.ErrNotFound, not a
	// nil/nil that callers can't distinguish from "found, empty config".
	return nil, shared.ErrNotFound
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

func (r *fakeDeliveryRepo) Create(ctx context.Context, d *webhookdomain.Delivery) error {
	// Faithful to a real DB driver: a write on an expired/cancelled context is
	// rejected. This is what makes the exhausted-retry audit-loss reproducible.
	if err := ctx.Err(); err != nil {
		return err
	}
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

// maskFor mirrors grpcapi.maskSecret's format without importing the driving
// adapter from an app-layer test (that would be a layering violation even
// from a _test package). It exists purely so these tests can construct "what
// a client received on a prior read and is now echoing back" inputs.
func maskFor(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return s[:4] + "****"
}

// TestUpdate_SecretMaskEcho_KeepsStoredSecret covers the actual data-destruction
// bug: both admin UIs read a webhook (secret always masked, e.g. "supe****"),
// then round-trip that same masked string back on the next Update. Without a
// server-side merge, that overwrites the real secret with the mask, and every
// subsequent delivery is signed with "supe****" - rejected silently by the
// receiver.
func TestUpdate_SecretMaskEcho_KeepsStoredSecret(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	svc, _ := newTestService(repo, deliveries, bus)

	ctx := context.Background()
	realSecret := "supersecretvalue"
	cfg := &webhookdomain.Config{Name: "orig", URL: "http://orig.com", Events: []string{"*"}, Enabled: true, Secret: realSecret}
	if err := svc.Create(ctx, cfg); err != nil {
		t.Fatalf("Create: %v", err)
	}

	update := &webhookdomain.Config{ID: cfg.ID, Name: "updated", URL: "http://orig.com", Events: []string{"*"}, Enabled: true, Secret: maskFor(realSecret)}
	if err := svc.Update(ctx, update); err != nil {
		t.Fatalf("Update: %v", err)
	}

	stored, err := repo.GetByID(ctx, cfg.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.Secret != realSecret {
		t.Fatalf("secret got overwritten by its own mask: got %q, want %q", stored.Secret, realSecret)
	}
}

// A secret of exactly 4 characters is masked whole ("****"), so the echo of it
// must be recognised too: at that boundary the merge compared "****" with
// "abcd****" and wrote the mask over the secret.
func TestUpdate_ShortSecretMaskEcho_KeepsStoredSecret(t *testing.T) {
	t.Parallel()
	for _, realSecret := range []string{"ab", "abcd"} {
		repo := &fakeWebhookRepo{}
		svc, _ := newTestService(repo, &fakeDeliveryRepo{}, sharedfakes.NewFakeEventBus())
		ctx := context.Background()
		cfg := &webhookdomain.Config{Name: "short", URL: "http://orig.com", Events: []string{"*"}, Enabled: true, Secret: realSecret}
		if err := svc.Create(ctx, cfg); err != nil {
			t.Fatalf("Create: %v", err)
		}
		update := &webhookdomain.Config{ID: cfg.ID, Name: "short", URL: "http://orig.com", Events: []string{"*"}, Enabled: true, Secret: "****"}
		if err := svc.Update(ctx, update); err != nil {
			t.Fatalf("Update: %v", err)
		}
		stored, err := repo.GetByID(ctx, cfg.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if stored.Secret != realSecret {
			t.Fatalf("%d-char secret overwritten by its mask echo: got %q", len(realSecret), stored.Secret)
		}
	}
}

// TestUpdate_NewSecret_IsStored covers the legitimate "operator rotates the
// secret" path - must still work after the mask-echo fix.
func TestUpdate_NewSecret_IsStored(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	svc, _ := newTestService(repo, deliveries, bus)

	ctx := context.Background()
	cfg := &webhookdomain.Config{Name: "orig", URL: "http://orig.com", Events: []string{"*"}, Enabled: true, Secret: "old-secret-value"}
	if err := svc.Create(ctx, cfg); err != nil {
		t.Fatalf("Create: %v", err)
	}

	update := &webhookdomain.Config{ID: cfg.ID, Name: "orig", URL: "http://orig.com", Events: []string{"*"}, Enabled: true, Secret: "brand-new-secret"}
	if err := svc.Update(ctx, update); err != nil {
		t.Fatalf("Update: %v", err)
	}

	stored, err := repo.GetByID(ctx, cfg.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.Secret != "brand-new-secret" {
		t.Fatalf("got secret %q, want brand-new-secret", stored.Secret)
	}
}

// TestUpdate_EmptySecret_KeepsStoredSecret: proto3 can't distinguish "field
// absent" from "empty string" on a bare string field, so an empty secret from
// either UI must be treated as "no change", not as "clear it".
func TestUpdate_EmptySecret_KeepsStoredSecret(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	svc, _ := newTestService(repo, deliveries, bus)

	ctx := context.Background()
	cfg := &webhookdomain.Config{Name: "orig", URL: "http://orig.com", Events: []string{"*"}, Enabled: true, Secret: "keep-me-secret"}
	if err := svc.Create(ctx, cfg); err != nil {
		t.Fatalf("Create: %v", err)
	}

	update := &webhookdomain.Config{ID: cfg.ID, Name: "orig", URL: "http://orig.com", Events: []string{"*"}, Enabled: true, Secret: ""}
	if err := svc.Update(ctx, update); err != nil {
		t.Fatalf("Update: %v", err)
	}

	stored, err := repo.GetByID(ctx, cfg.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.Secret != "keep-me-secret" {
		t.Fatalf("got secret %q, want keep-me-secret (unchanged)", stored.Secret)
	}
}

// TestUpdate_EmptyStoredSecret_NewSecretIsStored: a webhook created without a
// secret must still be able to get one via Update.
func TestUpdate_EmptyStoredSecret_NewSecretIsStored(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	svc, _ := newTestService(repo, deliveries, bus)

	ctx := context.Background()
	cfg := &webhookdomain.Config{Name: "orig", URL: "http://orig.com", Events: []string{"*"}, Enabled: true}
	if err := svc.Create(ctx, cfg); err != nil {
		t.Fatalf("Create: %v", err)
	}

	update := &webhookdomain.Config{ID: cfg.ID, Name: "orig", URL: "http://orig.com", Events: []string{"*"}, Enabled: true, Secret: "first-real-secret"}
	if err := svc.Update(ctx, update); err != nil {
		t.Fatalf("Update: %v", err)
	}

	stored, err := repo.GetByID(ctx, cfg.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.Secret != "first-real-secret" {
		t.Fatalf("got secret %q, want first-real-secret", stored.Secret)
	}
}

// TestUpdate_NewSecretContainingAsterisks_IsStoredVerbatim proves the merge
// decision reconstructs the mask from the STORED secret and compares against
// that, rather than doing a naive "contains a star" check - a real secret is
// free to contain asterisks and must not be mistaken for an echoed mask.
func TestUpdate_NewSecretContainingAsterisks_IsStoredVerbatim(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	svc, _ := newTestService(repo, deliveries, bus)

	ctx := context.Background()
	cfg := &webhookdomain.Config{Name: "orig", URL: "http://orig.com", Events: []string{"*"}, Enabled: true, Secret: "old-secret-value"}
	if err := svc.Create(ctx, cfg); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newSecret := "a***b***c"
	update := &webhookdomain.Config{ID: cfg.ID, Name: "orig", URL: "http://orig.com", Events: []string{"*"}, Enabled: true, Secret: newSecret}
	if err := svc.Update(ctx, update); err != nil {
		t.Fatalf("Update: %v", err)
	}

	stored, err := repo.GetByID(ctx, cfg.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.Secret != newSecret {
		t.Fatalf("got secret %q, want %q stored verbatim", stored.Secret, newSecret)
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

// A delivery whose retries exhaust the per-delivery deadline must still record
// its audit row. The delivery context is already expired by the time we persist,
// so the write must run on a detached context - otherwise the most important
// record (the failed/exhausted delivery) is silently lost.
func TestHandleAny_DeliveryRecordedEvenWhenContextExpired(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	sender := &programmableSender{responder: func(_ int) (int, error) { return 0, errors.New("boom") }}
	svc := newTestServiceWithSender(repo, deliveries, bus, sender)
	svc.SetRetryDelays([]time.Duration{time.Millisecond})

	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "exp", URL: "http://hook.example", Events: []string{"*"}, Enabled: true,
	})
	repo.mu.Unlock()

	// Pre-cancel the context: every ctx.Done() check inside the delivery fires
	// immediately, so the audit write hits an already-dead context - the exact
	// condition produced in prod when retries outlast deliveryTimeout.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = bus.Publish(ctx, fakeEvent{"test.event"})

	waitForDeliveries(t, deliveries, 1, 2*time.Second)
	deliveries.mu.Lock()
	d := deliveries.data[0]
	deliveries.mu.Unlock()
	if d.Error == "" {
		t.Fatal("expected the exhausted/failed delivery to record its error")
	}
}

// TestHandleAny_RetriesOnFailureThenSucceeds uses programmableSender (per-call response).
// Pure unit test - no HTTP.
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
	// Inject a plain client - httptest.Server binds 127.0.0.1 which the
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

// Delivery is at-least-once: consumers need a stable occurrence id to
// deduplicate retries. X-Evey-Delivery must be present, identical across every
// retry attempt of the same occurrence, and match the stored Delivery row.
func TestHandleAny_DeliveryIDHeaderStableAcrossRetries(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	sender := &programmableSender{responder: func(attempt int) (int, error) {
		if attempt == 0 {
			return 0, errors.New("transient network blip")
		}
		return 200, nil
	}}
	svc := newTestServiceWithSender(repo, deliveries, bus, sender)
	svc.SetRetryDelays([]time.Duration{time.Millisecond})

	ctx := context.Background()
	cfg := &webhookdomain.Config{
		Name:    "dedup-hook",
		URL:     "http://hook.example",
		Events:  []string{"media.item.added"},
		Enabled: true,
	}
	if err := svc.Create(ctx, cfg); err != nil {
		t.Fatalf("Create: %v", err)
	}

	_ = bus.Publish(ctx, fakeEvent{"media.item.added"})

	waitForCalls(t, sender, 2, 2*time.Second)
	waitForDeliveries(t, deliveries, 1, 2*time.Second)

	calls := sender.Calls()
	first := calls[0].Headers["X-Evey-Delivery"]
	if first == "" {
		t.Fatal("X-Evey-Delivery header missing")
	}
	if retry := calls[1].Headers["X-Evey-Delivery"]; retry != first {
		t.Fatalf("delivery id changed across retries: %q vs %q - consumers can't dedup", first, retry)
	}
	deliveries.mu.Lock()
	storedID := deliveries.data[0].ID
	deliveries.mu.Unlock()
	if storedID != first {
		t.Fatalf("stored delivery id %q != sent header %q", storedID, first)
	}
}

func (f *fakeDeliveryRepo) Purge(_ context.Context, _ time.Time) (int64, error) { return 0, nil }

func TestServiceTest_Success(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	sender := &programmableSender{responder: func(_ int) (int, error) { return http.StatusOK, nil }}
	svc := newTestServiceWithSender(repo, deliveries, bus, sender)

	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "wh-1", URL: "http://hook.example", Events: []string{"*"}, Enabled: true,
	})
	repo.mu.Unlock()

	result, err := svc.Test(context.Background(), "wh-1")
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", result.StatusCode)
	}

	// The test ping must go through the exact same audited path as a real
	// event, marked distinctly so it doesn't get confused with one.
	waitForDeliveries(t, deliveries, 1, 2*time.Second)
	deliveries.mu.Lock()
	d := deliveries.data[0]
	deliveries.mu.Unlock()
	if d.Event != "webhook.test" {
		t.Fatalf("expected delivery Event %q, got %q", "webhook.test", d.Event)
	}
	if d.WebhookID != "wh-1" {
		t.Fatalf("expected delivery WebhookID %q, got %q", "wh-1", d.WebhookID)
	}
}

func TestServiceTest_NotFound(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	svc, _ := newTestService(repo, deliveries, bus)

	_, err := svc.Test(context.Background(), "missing")
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("expected shared.ErrNotFound, got %v", err)
	}
}

func TestServiceTest_DeliveryFailed(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	sender := &programmableSender{responder: func(_ int) (int, error) { return http.StatusServiceUnavailable, nil }}
	svc := newTestServiceWithSender(repo, deliveries, bus, sender)
	svc.SetRetryDelays([]time.Duration{1 * time.Millisecond})

	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "wh-2", URL: "http://hook.example", Events: []string{"*"}, Enabled: true,
	})
	repo.mu.Unlock()

	_, err := svc.Test(context.Background(), "wh-2")
	if !errors.Is(err, webhook.ErrDeliveryFailed) {
		t.Fatalf("expected ErrDeliveryFailed, got %v", err)
	}
	if err.Error() == webhook.ErrDeliveryFailed.Error() {
		t.Fatal("expected the receiver's status to be included in the error message")
	}
}

// A delivery cancelled while it waits before a retry stops there: it neither
// sends again nor sits out the rest of its backoff. Nothing exercised that
// branch (mutation testing reported it uncovered), so a refactor that dropped
// the ctx.Done case would have held a Test call for the full 36 s of retries.
func TestTest_CancelledDuringBackoffStopsRetrying(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	deliveries := &fakeDeliveryRepo{}
	bus := sharedfakes.NewFakeEventBus()
	sender := &programmableSender{responder: func(_ int) (int, error) { return http.StatusInternalServerError, nil }}
	svc := newTestServiceWithSender(repo, deliveries, bus, sender)
	svc.SetRetryDelays([]time.Duration{time.Hour})
	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{ID: "slow", URL: "http://hook.example", Events: []string{"*"}, Enabled: true})
	repo.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := svc.Test(ctx, "slow")
		done <- err
	}()
	waitForCalls(t, sender, 1, 2*time.Second)
	cancel()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
			t.Fatalf("Test returned %v, want the cancellation", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a cancelled delivery kept waiting out its retry backoff")
	}
	if n := len(sender.Calls()); n != 1 {
		t.Fatalf("sent %d times, want the one attempt before the cancellation", n)
	}
}
