package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	appwebhook "github.com/eveyproject/evey/internal/app/webhook"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	webhookdomain "github.com/eveyproject/evey/internal/domain/webhook"
	webhookfakes "github.com/eveyproject/evey/internal/domain/webhook/fakes"
	"github.com/eveyproject/evey/internal/pkg/auth"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
)

// fakeWebhookDeliveryRepo is a minimal in-memory DeliveryRepository - the
// domain/webhook/fakes package has none, only a Config repository fake.
type fakeWebhookDeliveryRepo struct {
	mu   sync.Mutex
	data []*webhookdomain.Delivery
}

func (r *fakeWebhookDeliveryRepo) Create(_ context.Context, d *webhookdomain.Delivery) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = append(r.data, d)
	return nil
}

func (r *fakeWebhookDeliveryRepo) ListByWebhook(_ context.Context, webhookID string, limit int) ([]*webhookdomain.Delivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*webhookdomain.Delivery
	for _, d := range r.data {
		if d.WebhookID == webhookID {
			out = append(out, d)
		}
	}
	return out, nil
}

func (r *fakeWebhookDeliveryRepo) Purge(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

// newWebhookTestMux builds a mux with only the webhook test route registered,
// wired to a real appwebhook.Service (fake repo/sender underneath) so the test
// exercises the actual admin gate + service call, not a stand-in.
func newWebhookTestMux(t *testing.T, isAdmin bool) (*http.ServeMux, string, *webhookfakes.FakeRepository, *webhookfakes.FakeSender) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	gen := auth.NewJWTGenerator(key, time.Minute)
	token, err := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: isAdmin})
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	repo := webhookfakes.NewFakeRepository()
	deliveries := &fakeWebhookDeliveryRepo{}
	sender := webhookfakes.NewFakeSender()
	bus := sharedfakes.NewFakeEventBus()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := appwebhook.NewService(repo, deliveries, sender, log, bus)
	svc.SetURLValidator(func(string) error { return nil })
	svc.SetRetryDelays([]time.Duration{time.Millisecond})

	mux := http.NewServeMux()
	registerWebhookRoutes(mux, svc, gen, ratelimit.New(1000))
	return mux, token, repo, sender
}

func TestWebhookTestRoute_RequiresAdmin(t *testing.T) {
	t.Parallel()
	mux, token, _, _ := newWebhookTestMux(t, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/webhooks/wh-1/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWebhookTestRoute_NoToken(t *testing.T) {
	t.Parallel()
	mux, _, _, _ := newWebhookTestMux(t, true)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/webhooks/wh-1/test", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with no token, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWebhookTestRoute_NotFound(t *testing.T) {
	t.Parallel()
	mux, token, _, _ := newWebhookTestMux(t, true)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/webhooks/missing/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown webhook id, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWebhookTestRoute_Success(t *testing.T) {
	t.Parallel()
	mux, token, repo, sender := newWebhookTestMux(t, true)
	sender.StatusCode = http.StatusOK

	cfg := &webhookdomain.Config{ID: "wh-1", Name: "hook", URL: "http://hook.example", Events: []string{"*"}, Enabled: true}
	if err := repo.Create(context.Background(), cfg); err != nil {
		t.Fatalf("seed repo: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/webhooks/wh-1/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := body["statusCode"]; !ok {
		t.Fatalf("expected statusCode field in response, got %v", body)
	}
	if _, ok := body["durationMs"]; !ok {
		t.Fatalf("expected durationMs field in response, got %v", body)
	}
}

func TestWebhookTestRoute_DeliveryFailure(t *testing.T) {
	t.Parallel()
	mux, token, repo, sender := newWebhookTestMux(t, true)
	sender.StatusCode = http.StatusServiceUnavailable

	cfg := &webhookdomain.Config{ID: "wh-2", Name: "hook", URL: "http://hook.example", Events: []string{"*"}, Enabled: true}
	if err := repo.Create(context.Background(), cfg); err != nil {
		t.Fatalf("seed repo: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/webhooks/wh-2/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for a failed delivery, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["message"] == "" {
		t.Fatal("expected a non-empty, actionable error message")
	}
}
