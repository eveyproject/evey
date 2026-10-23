package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type TrustedPublisherRepository struct {
	db *sql.DB
}

func NewTrustedPublisherRepository(db *sql.DB) *TrustedPublisherRepository {
	return &TrustedPublisherRepository{db: db}
}

const trustedPublisherColumns = `fingerprint, name, public_key_pem, seeded, added_at`

func (r *TrustedPublisherRepository) List(ctx context.Context) ([]plugin.TrustedPublisher, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT `+trustedPublisherColumns+` FROM plugin_trusted_publishers ORDER BY added_at, fingerprint LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []plugin.TrustedPublisher
	for rows.Next() {
		var p plugin.TrustedPublisher
		if err := rows.Scan(&p.Fingerprint, &p.Name, &p.PublicKeyPEM, &p.Seeded, &p.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *TrustedPublisherRepository) Get(ctx context.Context, fingerprint string) (*plugin.TrustedPublisher, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var p plugin.TrustedPublisher
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT `+trustedPublisherColumns+` FROM plugin_trusted_publishers WHERE fingerprint = $1`, fingerprint).
		Scan(&p.Fingerprint, &p.Name, &p.PublicKeyPEM, &p.Seeded, &p.AddedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Upsert keeps the original added_at and never clears seeded: re-adding a
// seeded key by hand must not make it removable from the panel.
func (r *TrustedPublisherRepository) Upsert(ctx context.Context, p plugin.TrustedPublisher) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO plugin_trusted_publishers (fingerprint, name, public_key_pem, seeded)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (fingerprint) DO UPDATE SET
		   name = EXCLUDED.name,
		   public_key_pem = EXCLUDED.public_key_pem,
		   seeded = plugin_trusted_publishers.seeded OR EXCLUDED.seeded`,
		p.Fingerprint, p.Name, p.PublicKeyPEM, p.Seeded)
	return err
}

func (r *TrustedPublisherRepository) Delete(ctx context.Context, fingerprint string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	db := txFromCtx(ctx, r.db)
	res, err := db.ExecContext(ctx,
		`DELETE FROM plugin_trusted_publishers WHERE fingerprint = $1 AND seeded = FALSE`, fingerprint)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	var seeded bool
	err = db.QueryRowContext(ctx,
		`SELECT seeded FROM plugin_trusted_publishers WHERE fingerprint = $1`, fingerprint).Scan(&seeded)
	if errors.Is(err, sql.ErrNoRows) {
		return shared.ErrNotFound
	}
	if err != nil {
		return err
	}
	return plugin.ErrSeededPublisher
}

func (r *TrustedPublisherRepository) PruneSeeded(ctx context.Context, keep []string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if keep == nil {
		keep = []string{}
	}
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM plugin_trusted_publishers WHERE seeded = TRUE AND NOT (fingerprint = ANY($1))`, pq.Array(keep))
	return err
}

var _ plugin.TrustedPublisherRepository = (*TrustedPublisherRepository)(nil)
