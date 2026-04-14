package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/evey-project/evey/internal/domain/shared"
)

var _ shared.ShareLinkRepository = (*ShareLinkRepo)(nil)

type ShareLinkRepo struct {
	db *sql.DB
}

func NewShareLinkRepo(db *sql.DB) *ShareLinkRepo {
	return &ShareLinkRepo{db: db}
}

func (r *ShareLinkRepo) Create(ctx context.Context, link *shared.ShareLink) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO share_links (id, token, media_item_id, created_by, expires_at, max_views, view_count, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		link.ID, link.Token, link.MediaItemID, link.CreatedBy, link.ExpiresAt, link.MaxViews, link.ViewCount, link.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create share link: %w", err)
	}
	return nil
}

func (r *ShareLinkRepo) GetByID(ctx context.Context, id string) (*shared.ShareLink, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	row := r.db.QueryRowContext(ctx,
		`SELECT id, token, media_item_id, created_by, expires_at, max_views, view_count, created_at
		 FROM share_links WHERE id = $1`, id)
	return scanShareLink(row)
}

func (r *ShareLinkRepo) GetByToken(ctx context.Context, token string) (*shared.ShareLink, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	row := r.db.QueryRowContext(ctx,
		`SELECT id, token, media_item_id, created_by, expires_at, max_views, view_count, created_at
		 FROM share_links WHERE token = $1`, token)
	return scanShareLink(row)
}

func (r *ShareLinkRepo) ListByUser(ctx context.Context, userID string) ([]*shared.ShareLink, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, token, media_item_id, created_by, expires_at, max_views, view_count, created_at
		 FROM share_links WHERE created_by = $1 ORDER BY created_at DESC LIMIT 500`, userID)
	if err != nil {
		return nil, fmt.Errorf("list share links: %w", err)
	}
	defer rows.Close()
	return scanShareLinks(rows)
}

func (r *ShareLinkRepo) ListAll(ctx context.Context) ([]*shared.ShareLink, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, token, media_item_id, created_by, expires_at, max_views, view_count, created_at
		 FROM share_links ORDER BY created_at DESC LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("list all share links: %w", err)
	}
	defer rows.Close()
	return scanShareLinks(rows)
}

func (r *ShareLinkRepo) IncrementViewCount(ctx context.Context, token string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`UPDATE share_links SET view_count = view_count + 1 WHERE token = $1`, token)
	if err != nil {
		return fmt.Errorf("increment view count: %w", err)
	}
	return nil
}

func (r *ShareLinkRepo) Delete(ctx context.Context, id string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM share_links WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete share link: %w", err)
	}
	return nil
}

func (r *ShareLinkRepo) DeleteExpired(ctx context.Context) (int, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	result, err := r.db.ExecContext(ctx, `DELETE FROM share_links WHERE expires_at < NOW()`)
	if err != nil {
		return 0, fmt.Errorf("delete expired share links: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

func scanShareLink(row *sql.Row) (*shared.ShareLink, error) {
	var link shared.ShareLink
	err := row.Scan(&link.ID, &link.Token, &link.MediaItemID, &link.CreatedBy,
		&link.ExpiresAt, &link.MaxViews, &link.ViewCount, &link.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan share link: %w", err)
	}
	return &link, nil
}

func scanShareLinks(rows *sql.Rows) ([]*shared.ShareLink, error) {
	var links []*shared.ShareLink
	for rows.Next() {
		var link shared.ShareLink
		if err := rows.Scan(&link.ID, &link.Token, &link.MediaItemID, &link.CreatedBy,
			&link.ExpiresAt, &link.MaxViews, &link.ViewCount, &link.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan share link: %w", err)
		}
		links = append(links, &link)
	}
	return links, rows.Err()
}
