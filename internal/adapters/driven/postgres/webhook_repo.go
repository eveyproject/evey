package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/webhook"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type WebhookRepository struct {
	db *sql.DB
}

func NewWebhookRepository(db *sql.DB) *WebhookRepository {
	return &WebhookRepository{db: db}
}

func (r *WebhookRepository) Create(ctx context.Context, cfg *webhook.Config) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.webhook.Create")
	defer span.End()
	span.SetAttributes(attribute.String("webhook.id", cfg.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	headersJSON, err := json.Marshal(cfg.Headers)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	events := cfg.Events
	if events == nil {
		events = []string{}
	}
	_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO webhook_configs (id, name, url, events, secret, headers, enabled, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		cfg.ID, cfg.Name, cfg.URL, pq.Array(events), cfg.Secret, headersJSON, cfg.Enabled, cfg.CreatedAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *WebhookRepository) GetByID(ctx context.Context, id string) (*webhook.Config, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.webhook.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("webhook.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	cfg := &webhook.Config{}
	var headersJSON []byte
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, name, url, events, secret, headers, enabled, created_at FROM webhook_configs WHERE id = $1`, id,
	).Scan(&cfg.ID, &cfg.Name, &cfg.URL, pq.Array(&cfg.Events), &cfg.Secret, &headersJSON, &cfg.Enabled, &cfg.CreatedAt)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		// Every other repo here maps a missing row onto the domain error; this
		// one did not, so callers checking errors.Is(err, shared.ErrNotFound)
		// fell through to their 500 branch instead of answering 404.
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal(headersJSON, &cfg.Headers); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "headers"), slog.Any("err", err))
	}
	return cfg, nil
}

func (r *WebhookRepository) List(ctx context.Context) ([]*webhook.Config, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.webhook.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, name, url, events, secret, headers, enabled, created_at FROM webhook_configs ORDER BY created_at DESC LIMIT 1000`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*webhook.Config
	for rows.Next() {
		cfg := &webhook.Config{}
		var headersJSON []byte
		if err := rows.Scan(&cfg.ID, &cfg.Name, &cfg.URL, pq.Array(&cfg.Events), &cfg.Secret, &headersJSON, &cfg.Enabled, &cfg.CreatedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		if err := json.Unmarshal(headersJSON, &cfg.Headers); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "headers"), slog.Any("err", err))
		}
		result = append(result, cfg)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *WebhookRepository) Update(ctx context.Context, cfg *webhook.Config) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.webhook.Update")
	defer span.End()
	span.SetAttributes(attribute.String("webhook.id", cfg.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	headersJSON, err := json.Marshal(cfg.Headers)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	events := cfg.Events
	if events == nil {
		events = []string{}
	}
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE webhook_configs SET name=$1, url=$2, events=$3, secret=$4, headers=$5, enabled=$6 WHERE id=$7`,
		cfg.Name, cfg.URL, pq.Array(events), cfg.Secret, headersJSON, cfg.Enabled, cfg.ID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

func (r *WebhookRepository) Delete(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.webhook.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("webhook.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM webhook_configs WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

type WebhookDeliveryRepository struct {
	db *sql.DB
}

func NewWebhookDeliveryRepository(db *sql.DB) *WebhookDeliveryRepository {
	return &WebhookDeliveryRepository{db: db}
}

func (r *WebhookDeliveryRepository) Create(ctx context.Context, d *webhook.Delivery) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.webhook_delivery.Create")
	defer span.End()
	span.SetAttributes(attribute.String("webhook.id", d.WebhookID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO webhook_deliveries (id, webhook_id, event, status_code, error, sent_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		d.ID, d.WebhookID, d.Event, d.StatusCode, d.Error, d.SentAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *WebhookDeliveryRepository) ListByWebhook(ctx context.Context, webhookID string, limit int) ([]*webhook.Delivery, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.webhook_delivery.ListByWebhook")
	defer span.End()
	span.SetAttributes(attribute.String("webhook.id", webhookID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	q := `SELECT id, webhook_id, event, status_code, error, sent_at FROM webhook_deliveries WHERE webhook_id = $1 ORDER BY sent_at DESC`
	if limit <= 0 {
		limit = 1000
	}
	q += ` LIMIT $2`
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, q, webhookID, limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	result, err := scanDeliveries(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func scanDeliveries(rows *sql.Rows) ([]*webhook.Delivery, error) {
	var result []*webhook.Delivery
	for rows.Next() {
		d := &webhook.Delivery{}
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.Event, &d.StatusCode, &d.Error, &d.SentAt); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

var _ webhook.Repository = (*WebhookRepository)(nil)
var _ webhook.DeliveryRepository = (*WebhookDeliveryRepository)(nil)

// Purge deletes delivery rows older than before. Uses the composite
// (webhook_id, sent_at) index added in 000141 only incidentally - this is a
// full-table range delete on sent_at, cheap because the table is small once the
// sweep has run at least once.
func (r *WebhookDeliveryRepository) Purge(ctx context.Context, before time.Time) (int64, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.webhook.PurgeDeliveries")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM webhook_deliveries WHERE sent_at < $1`, before)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
