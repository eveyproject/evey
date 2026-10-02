package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

var _ shared.ShareLinkRepository = (*ShareLinkRepo)(nil)

type ShareLinkRepo struct {
	db *sql.DB
}

func NewShareLinkRepo(db *sql.DB) *ShareLinkRepo {
	return &ShareLinkRepo{db: db}
}

func (r *ShareLinkRepo) Create(ctx context.Context, link *shared.ShareLink) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.share_link.Create")
	defer span.End()
	span.SetAttributes(attribute.String("share_link.id", link.ID))
	if err := link.Validate(); err != nil {
		return fmt.Errorf("invalid share link: %w", err)
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO share_links (id, token, media_item_id, created_by, expires_at, max_views, view_count, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		link.ID, link.Token, link.MediaItemID, link.CreatedBy, link.ExpiresAt, link.MaxViews, link.ViewCount, link.CreatedAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("create share link: %w", err)
	}
	return nil
}

func (r *ShareLinkRepo) GetByID(ctx context.Context, id string) (*shared.ShareLink, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.share_link.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("share_link.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	row := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, token, media_item_id, created_by, expires_at, max_views, view_count, created_at
		 FROM share_links WHERE id = $1`, id)
	link, err := scanShareLink(row)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return link, nil
}

func (r *ShareLinkRepo) GetByToken(ctx context.Context, token string) (*shared.ShareLink, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.share_link.GetByToken")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	row := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, token, media_item_id, created_by, expires_at, max_views, view_count, created_at
		 FROM share_links WHERE token = $1`, token)
	link, err := scanShareLink(row)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return link, nil
}

func (r *ShareLinkRepo) ListByUser(ctx context.Context, userID string) ([]*shared.ShareLink, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.share_link.ListByUser")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", userID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, token, media_item_id, created_by, expires_at, max_views, view_count, created_at
		 FROM share_links WHERE created_by = $1 ORDER BY created_at DESC LIMIT 500`, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("list share links: %w", err)
	}
	defer rows.Close()
	links, err := scanShareLinks(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return links, nil
}

func (r *ShareLinkRepo) ListAll(ctx context.Context) ([]*shared.ShareLink, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.share_link.ListAll")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, token, media_item_id, created_by, expires_at, max_views, view_count, created_at
		 FROM share_links ORDER BY created_at DESC LIMIT 1000`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("list all share links: %w", err)
	}
	defer rows.Close()
	links, err := scanShareLinks(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return links, nil
}

func (r *ShareLinkRepo) ConsumeView(ctx context.Context, token string) (*shared.ShareLink, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.share_link.ConsumeView")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// The limit lives in the WHERE: concurrent spends serialize on the row
	// lock and the loser re-evaluates against the winner's count, so the
	// counter can never pass max_views.
	row := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`UPDATE share_links SET view_count = view_count + 1
		 WHERE token = $1 AND expires_at > NOW()
		   AND (max_views IS NULL OR view_count < max_views)
		 RETURNING id, token, media_item_id, created_by, expires_at, max_views, view_count, created_at`, token)
	link, err := scanShareLink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("consume share view: %w", err)
	}
	return link, nil
}

func (r *ShareLinkRepo) Delete(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.share_link.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("share_link.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM share_links WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("delete share link: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

func (r *ShareLinkRepo) DeleteExpired(ctx context.Context) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.share_link.DeleteExpired")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	result, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM share_links WHERE expires_at < NOW()`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
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
