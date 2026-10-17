package webhook_test

import (
	"context"
	"maps"
	"testing"

	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	webhookdomain "github.com/eveyproject/evey/internal/domain/webhook"
)

func createWithHeaders(t *testing.T, headers map[string]string) (*webhookdomain.Config, func(*webhookdomain.Config) map[string]string, func(*webhookdomain.Config) error) {
	t.Helper()
	repo := &fakeWebhookRepo{}
	svc, _ := newTestService(repo, &fakeDeliveryRepo{}, sharedfakes.NewFakeEventBus())
	ctx := context.Background()
	cfg := &webhookdomain.Config{Name: "orig", URL: "http://orig.com", Events: []string{"*"}, Enabled: true, Headers: maps.Clone(headers)}
	if err := svc.Create(ctx, cfg); err != nil {
		t.Fatalf("Create: %v", err)
	}
	stored := func(c *webhookdomain.Config) map[string]string {
		got, err := repo.GetByID(ctx, c.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		return got.Headers
	}
	update := func(c *webhookdomain.Config) error { return svc.Update(ctx, c) }
	return cfg, stored, update
}

// Neither admin UI edits headers: the enable toggle sends name, url, events,
// secret and enabled only. Persisting that verbatim wrote headers=null, so
// toggling a webhook off and on again dropped the Authorization header its
// receiver needs, and every later delivery was refused.
func TestUpdate_NoHeadersSent_KeepsStoredHeaders(t *testing.T) {
	t.Parallel()
	want := map[string]string{"Authorization": "Bearer receiver-token"}
	cfg, stored, update := createWithHeaders(t, want)

	if err := update(&webhookdomain.Config{ID: cfg.ID, Name: "orig", URL: "http://orig.com", Events: []string{"*"}, Enabled: false}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := stored(cfg); !maps.Equal(got, want) {
		t.Fatalf("headers after toggle = %v, want %v", got, want)
	}
}

// Header values are masked on every read like the secret, so a client that
// round-trips what it read sends masks back; those mean "unchanged".
func TestUpdate_MaskedHeaderEcho_KeepsStoredValue(t *testing.T) {
	t.Parallel()
	want := map[string]string{"Authorization": "Bearer receiver-token", "X-Key": "ab"}
	cfg, stored, update := createWithHeaders(t, want)

	echo := map[string]string{"Authorization": maskFor(want["Authorization"]), "X-Key": maskFor(want["X-Key"])}
	if err := update(&webhookdomain.Config{ID: cfg.ID, Name: "orig", URL: "http://orig.com", Events: []string{"*"}, Enabled: true, Headers: echo}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := stored(cfg); !maps.Equal(got, want) {
		t.Fatalf("headers after mask echo = %v, want %v", got, want)
	}
}

// A real new value, a new key and a dropped key are all honoured.
func TestUpdate_ChangedHeaders_AreStored(t *testing.T) {
	t.Parallel()
	cfg, stored, update := createWithHeaders(t, map[string]string{"Authorization": "Bearer old-token", "X-Old": "gone"})

	sent := map[string]string{"Authorization": "Bearer new-token", "X-New": "added"}
	if err := update(&webhookdomain.Config{ID: cfg.ID, Name: "orig", URL: "http://orig.com", Events: []string{"*"}, Enabled: true, Headers: sent}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := stored(cfg); !maps.Equal(got, sent) {
		t.Fatalf("headers after edit = %v, want %v", got, sent)
	}
}
