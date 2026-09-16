package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/webhook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookRepo_CRUD(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewWebhookRepository(db)
	ctx := context.Background()

	cfg := &webhook.Config{
		ID:        "wh-1",
		Name:      "My Webhook",
		URL:       "https://example.com/hook",
		Events:    []string{"media.item.added", "library.scan.completed"},
		Secret:    "s3cr3t",
		Headers:   map[string]string{"X-Custom": "value"},
		Enabled:   true,
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}

	require.NoError(t, repo.Create(ctx, cfg))

	got, err := repo.GetByID(ctx, cfg.ID)
	require.NoError(t, err)
	assert.Equal(t, cfg.Name, got.Name)
	assert.Equal(t, cfg.Events, got.Events)
	assert.Equal(t, "value", got.Headers["X-Custom"])

	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	cfg.Enabled = false
	cfg.Name = "Updated"
	require.NoError(t, repo.Update(ctx, cfg))

	got, err = repo.GetByID(ctx, cfg.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", got.Name)
	assert.False(t, got.Enabled)

	require.NoError(t, repo.Delete(ctx, cfg.ID))
	list, _ = repo.List(ctx)
	assert.Empty(t, list)
}

func TestWebhookRepo_Delete_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewWebhookRepository(db)
	err := repo.Delete(context.Background(), "wh-missing")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestWebhookRepo_Update_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewWebhookRepository(db)
	err := repo.Update(context.Background(), &webhook.Config{
		ID: "wh-missing", Name: "x", URL: "http://x", Enabled: true,
	})
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestWebhookDeliveryRepo(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	whRepo := postgres.NewWebhookRepository(db)
	delivRepo := postgres.NewWebhookDeliveryRepository(db)
	ctx := context.Background()

	require.NoError(t, whRepo.Create(ctx, &webhook.Config{
		ID: "wh-2", Name: "Test", URL: "http://x.com", Enabled: true,
		CreatedAt: time.Now(),
	}))

	for i, id := range []string{"d1", "d2"} {
		require.NoError(t, delivRepo.Create(ctx, &webhook.Delivery{
			ID:         id,
			WebhookID:  "wh-2",
			Event:      "media.item.added",
			StatusCode: 200 + i,
			SentAt:     time.Now(),
		}))
	}

	deliveries, err := delivRepo.ListByWebhook(ctx, "wh-2", 10)
	require.NoError(t, err)
	assert.Len(t, deliveries, 2)
}

// Delivery retention: rows older than the cutoff go, the recent trail stays.
func TestWebhookDeliveryRepo_Purge(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, postgres.NewWebhookRepository(db).Create(ctx, &webhook.Config{
		ID: "wh-purge", Name: "Purge", URL: "http://x.com", Enabled: true, CreatedAt: time.Now(),
	}))
	delivRepo := postgres.NewWebhookDeliveryRepository(db)
	now := time.Now().UTC()
	for id, at := range map[string]time.Time{"dp-old": now.Add(-40 * 24 * time.Hour), "dp-new": now.Add(-time.Hour)} {
		require.NoError(t, delivRepo.Create(ctx, &webhook.Delivery{ID: id, WebhookID: "wh-purge", Event: "media.item.added", StatusCode: 200, SentAt: at}))
	}

	n, err := delivRepo.Purge(ctx, now.Add(-30*24*time.Hour))
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	left, err := delivRepo.ListByWebhook(ctx, "wh-purge", 10)
	require.NoError(t, err)
	require.Len(t, left, 1)
	assert.Equal(t, "dp-new", left[0].ID)
	n, err = delivRepo.Purge(ctx, now.Add(-30*24*time.Hour))
	require.NoError(t, err)
	assert.Zero(t, n)
}
