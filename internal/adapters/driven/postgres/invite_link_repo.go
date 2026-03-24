package postgres

import (
	"context"
	"database/sql"

	"github.com/gravity-zero/evey/internal/domain/user"
)

type InviteLinkRepository struct {
	db *sql.DB
}

func NewInviteLinkRepository(db *sql.DB) *InviteLinkRepository {
	return &InviteLinkRepository{db: db}
}

func (r *InviteLinkRepository) Create(ctx context.Context, link *user.InviteLink) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO invite_links (id, created_by, token, label, max_uses, used_count, is_admin, expires_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		link.ID, link.CreatedBy, link.Token, link.Label, link.MaxUses, link.UsedCount, link.IsAdmin, link.ExpiresAt, link.CreatedAt,
	)
	return err
}

func (r *InviteLinkRepository) GetByToken(ctx context.Context, token string) (*user.InviteLink, error) {
	link := &user.InviteLink{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, created_by, token, label, max_uses, used_count, is_admin, expires_at, created_at
		 FROM invite_links WHERE token = $1`, token,
	).Scan(&link.ID, &link.CreatedBy, &link.Token, &link.Label, &link.MaxUses, &link.UsedCount, &link.IsAdmin, &link.ExpiresAt, &link.CreatedAt)
	if err != nil {
		return nil, err
	}
	return link, nil
}

func (r *InviteLinkRepository) ListByCreator(ctx context.Context, creatorID string) ([]*user.InviteLink, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, created_by, token, label, max_uses, used_count, is_admin, expires_at, created_at
		 FROM invite_links WHERE created_by = $1 ORDER BY created_at`, creatorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*user.InviteLink
	for rows.Next() {
		link := &user.InviteLink{}
		if err := rows.Scan(&link.ID, &link.CreatedBy, &link.Token, &link.Label, &link.MaxUses, &link.UsedCount, &link.IsAdmin, &link.ExpiresAt, &link.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, link)
	}
	return result, rows.Err()
}

func (r *InviteLinkRepository) IncrementUsedCount(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE invite_links SET used_count = used_count + 1 WHERE id = $1`, id)
	return err
}

func (r *InviteLinkRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM invite_links WHERE id = $1`, id)
	return err
}

var _ user.InviteLinkRepository = (*InviteLinkRepository)(nil)
